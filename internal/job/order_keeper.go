package job

import (
	"context"
	"fmt"
	"time"

	"github.com/fachebot/sol-grid-bot/internal/ent"
	"github.com/fachebot/sol-grid-bot/internal/ent/grid"
	"github.com/fachebot/sol-grid-bot/internal/ent/order"
	"github.com/fachebot/sol-grid-bot/internal/logger"
	"github.com/fachebot/sol-grid-bot/internal/model"
	"github.com/fachebot/sol-grid-bot/internal/strategy"
	"github.com/fachebot/sol-grid-bot/internal/svc"
	"github.com/fachebot/sol-grid-bot/internal/utils"
	"github.com/fachebot/sol-grid-bot/internal/utils/format"
	"github.com/fachebot/sol-grid-bot/internal/utils/solanautil"

	"github.com/shopspring/decimal"
)

type OrderKeeper struct {
	ctx      context.Context
	cancel   context.CancelFunc
	stopChan chan struct{}
	svcCtx   *svc.ServiceContext
}

func NewOrderKeeper(svcCtx *svc.ServiceContext) *OrderKeeper {
	ctx, cancel := context.WithCancel(context.Background())
	return &OrderKeeper{
		ctx:    ctx,
		cancel: cancel,
		svcCtx: svcCtx,
	}
}

func (keeper *OrderKeeper) Stop() {
	if keeper.stopChan == nil {
		return
	}

	logger.Infof("[OrderKeeper] 准备停止服务")

	keeper.cancel()

	<-keeper.stopChan
	close(keeper.stopChan)
	keeper.stopChan = nil

	logger.Infof("[OrderKeeper] 服务已经停止")
}

func (keeper *OrderKeeper) Start() {
	if keeper.stopChan != nil {
		return
	}

	keeper.stopChan = make(chan struct{})
	logger.Infof("[OrderKeeper] 开始运行服务")
	go keeper.run()
}

func (keeper *OrderKeeper) run() {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			keeper.handlePolling()
			duration := time.Millisecond * 1000
			timer.Reset(duration)
		case <-keeper.ctx.Done():
			keeper.stopChan <- struct{}{}
			return

		}
	}
}

func (keeper *OrderKeeper) sendNotification(ord *ent.Order, text string, force bool) {
	w, err := keeper.svcCtx.WalletModel.FindByAccount(keeper.ctx, ord.Account)
	if err != nil {
		logger.Errorf("[OrderKeeper] 查询钱包信息失败, account: %s, %v", ord.Account, err)
		return
	}

	if ord.StrategyId != "" {
		s, err := keeper.svcCtx.StrategyModel.FindByUserIdGUID(keeper.ctx, w.UserId, ord.StrategyId)
		if err != nil {
			logger.Errorf("[OrderKeeper] 查询策略信息失败, userId: %d, strategyId: %s, %v", w.UserId, ord.StrategyId, err)
			return
		}

		if !force && !s.EnablePushNotification {
			return
		}
	}

	if w.UserId == 0 {
		logger.Warnf("[OrderKeeper] 用户未绑定Telegram账号, 无法发送通知")
		return
	}

	_, err = utils.SendMessage(keeper.svcCtx.BotApi, w.UserId, text)
	if err != nil {
		logger.Warnf("[OrderKeeper] 发送电报通知失败, userId: %d, text: %s, %v", w.UserId, text, err)
		return
	}
}

func (keeper *OrderKeeper) handleRetryExit(ord *ent.Order) {
	// 查询策略
	record, err := keeper.svcCtx.StrategyModel.FindByGUID(keeper.ctx, ord.StrategyId)
	if err != nil {
		logger.Errorf("[OrderKeeper] 查询策略信息失败, account: %s, strategy: %s, %v", ord.Account, ord.StrategyId, err)
		return
	}

	keeper.sendNotification(ord, fmt.Sprintf("♻️ 正在尝试重新清仓 *%s* 代币失败", ord.Symbol), true)

	// 卖出代币
	orderArgs, err := strategy.SellToken(keeper.ctx, keeper.svcCtx, record, "重新清仓", &ord.InAmount, nil, true)
	if err != nil {
		logger.Errorf("[OrderKeeper] 尝试重新清仓失败, strategy: %s, token: %s, %v", ord.StrategyId, ord.Symbol, err)
		keeper.sendNotification(ord, fmt.Sprintf("❌ 尝试重新清仓 *%s* 代币失败，请手动清仓", ord.Symbol), true)
		return
	}
	orderArgs.GridBuyCost = ord.GridBuyCost

	// 保存订单记录
	err = utils.Tx(keeper.ctx, keeper.svcCtx.DbClient, func(tx *ent.Tx) error {
		_, err = model.NewOrderModel(tx.Order).Save(keeper.ctx, orderArgs)
		return err
	})
	if err != nil {
		logger.Errorf("[OrderKeeper] 保存订单记录失败, order: %+v, %v", orderArgs, err)
	}
}

func (keeper *OrderKeeper) handleCloseOrder(ord *ent.Order, tokenBalanceChanges map[string]solanautil.TokenBalanceChange) {
	// 计算最终价格
	cost := decimal.Zero
	var finalPrice, outAmount decimal.Decimal
	switch ord.Type {
	case order.TypeBuy:
		v, ok := tokenBalanceChanges[ord.Token]
		if ok && !v.Change.Equal(decimal.Zero) {
			finalPrice = ord.InAmount.Div(v.Change)
		}
		outAmount = v.Change
	case order.TypeSell:
		v, ok := tokenBalanceChanges[solanautil.USDC]
		if ok && !ord.InAmount.Equal(decimal.Zero) {
			finalPrice = v.Change.Div(ord.InAmount)
		}
		outAmount = v.Change

		if ord.GridBuyCost != nil {
			cost = *ord.GridBuyCost
		} else if ord.GridId != nil {
			g, err := keeper.svcCtx.GridModel.FindByGuid(keeper.ctx, *ord.GridId)
			if err == nil {
				cost = g.Amount
			} else {
				logger.Errorf("[OrderKeeper] 查询网格信息失败, guid: %s, %v", *ord.GridId, err)
			}
		}
	}

	// 获取策略信息
	s, err := keeper.svcCtx.StrategyModel.FindByGUID(keeper.ctx, ord.StrategyId)
	if err != nil {
		logger.Errorf("[OrderKeeper] 查询策略信息失败, guid: %s, %v", ord.StrategyId, err)
	}

	// 更新订单状态
	err = utils.Tx(keeper.ctx, keeper.svcCtx.DbClient, func(tx *ent.Tx) error {
		if ord.GridId != nil {
			switch ord.Type {
			case order.TypeBuy:
				err := model.NewGridModel(tx.Grid).SetBoughtStatus(
					keeper.ctx, *ord.GridId, finalPrice, outAmount)
				if err != nil {
					return err
				}
			case order.TypeSell:
				_, err := model.NewGridModel(tx.Grid).DeleteByGuid(keeper.ctx, *ord.GridId)
				if err != nil {
					return err
				}
			}
		}

		if !cost.IsZero() {
			profit := outAmount.Sub(cost)
			err = model.NewOrderModel(tx.Order).UpdateProfit(keeper.ctx, ord.ID, profit)
			if err != nil {
				return err
			}
		}

		err = model.NewOrderModel(tx.Order).SetOrderClosedStatus(keeper.ctx, ord.ID, finalPrice, outAmount)
		if err != nil {
			return err
		}

		if s != nil && ord.GridId != nil && s.FirstOrderId == nil {
			err = model.NewStrategyModel(tx.Strategy).UpdateFirstOrderId(keeper.ctx, s.ID, &ord.ID)
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		logger.Errorf("[OrderKeeper] 设置订单 closed 状态失败, id: %d, hash: %s, %v", ord.ID, ord.TxHash, err)
		return
	}
	logger.Infof("[OrderKeeper] 设置订单 closed 状态, id: %d, type: %s, finalPrice: %s, outAmount: %s, hash: %s",
		ord.ID, ord.Type, finalPrice, outAmount, ord.TxHash)

	// 发送电报通知
	switch ord.Type {
	case order.TypeBuy:
		usdcChange, ok := tokenBalanceChanges[solanautil.USDC]
		if !ok {
			usdcChange = solanautil.TokenBalanceChange{}
		}
		text := fmt.Sprintf("🟢 网格 `#%d` 买入 %sU [%s](https://gmgn.ai/sol/token/%s) 💰 余额: %sU [>>](https://solscan.io/tx/%s)",
			*ord.GridNumber, usdcChange.Change.Abs().Truncate(2), ord.Symbol, ord.Token, usdcChange.Post.Truncate(2), ord.TxHash)
		keeper.sendNotification(ord, text, false)
	case order.TypeSell:
		if ord.GridId != nil {
			usdcChange, ok := tokenBalanceChanges[solanautil.USDC]
			if !ok {
				usdcChange = solanautil.TokenBalanceChange{}
			}
			text := fmt.Sprintf("🔴 网格 `#%d` 卖出 %sU [%s](https://gmgn.ai/sol/token/%s) 💰 余额: %sU [>>](https://solscan.io/tx/%s)",
				*ord.GridNumber, usdcChange.Change.Abs().Truncate(2), ord.Symbol, ord.Token, usdcChange.Post.Truncate(2), ord.TxHash)
			keeper.sendNotification(ord, text, false)
		} else {
			text := fmt.Sprintf("✅ 清仓 *%s* 代币成功, 成交价格: %s, 💰 金额: %sU [>>](https://solscan.io/tx/%s)",
				ord.Symbol, format.Price(finalPrice, 5), outAmount.Truncate(2), ord.TxHash)
			keeper.sendNotification(ord, text, true)
		}
	}
}

func (keeper *OrderKeeper) handleRejectOrder(ord *ent.Order, _ map[string]solanautil.TokenBalanceChange, reason string) {
	err := utils.Tx(keeper.ctx, keeper.svcCtx.DbClient, func(tx *ent.Tx) error {
		if ord.GridId != nil {
			if ord.Type == order.TypeBuy {
				_, err := model.NewGridModel(tx.Grid).DeleteByGuid(keeper.ctx, *ord.GridId)
				if err != nil {
					return err
				}
			} else {
				err := model.NewGridModel(tx.Grid).UpdateStatusByGuid(keeper.ctx, *ord.GridId, grid.StatusBought)
				if err != nil {
					return err
				}
			}
		}

		return model.NewOrderModel(tx.Order).SetOrderRejectedStatus(keeper.ctx, ord.ID, reason)
	})
	if err != nil {
		logger.Errorf("[OrderKeeper] 设置订单 rejected 状态失败, id: %d, hash: %s, %v", ord.ID, ord.TxHash, err)
		return
	}
	logger.Infof("[OrderKeeper] 设置订单 rejected 状态, id: %d, hash: %s, reason: %s", ord.ID, ord.TxHash, reason)

	// 发送失败通知
	switch ord.Type {
	case order.TypeBuy:
		keeper.sendNotification(ord, fmt.Sprintf("❌ 网格 `#%d` 买入 %sU [%s](https://gmgn.ai/sol/token/%s), 原因: 流动性不足或者滑点问题 [>>](https://solscan.io/tx/%s)",
			*ord.GridNumber, ord.InAmount.Truncate(2), ord.Symbol, ord.Token, ord.TxHash), false)
	case order.TypeSell:
		if ord.GridId != nil {
			keeper.sendNotification(ord, fmt.Sprintf("❌ 网格 `#%d` 卖出 %s [%s](https://gmgn.ai/sol/token/%s) 失败, 原因: 流动性不足或者滑点问题 [>>](https://solscan.io/tx/%s)",
				*ord.GridNumber, ord.InAmount, ord.Symbol, ord.Token, ord.TxHash), false)
		} else {
			keeper.sendNotification(ord, fmt.Sprintf("❌ 清仓 *%s* 代币失败, 原因: 流动性不足或者滑点问题 [>>](https://solscan.io/tx/%s)", ord.Symbol, ord.TxHash), true)
		}
	}

	// 重试清仓操作
	if ord.Type == order.TypeSell && ord.GridId == nil {
		keeper.handleRetryExit(ord)
	}
}

func (keeper *OrderKeeper) handlePolling() {
	// 获取订单列表
	orders, err := keeper.svcCtx.OrderModel.FindPendingOrders(keeper.ctx, 100)
	if err != nil {
		logger.Errorf("[OrderKeeper] 获取订单列表失败, %v", err)
	}
	if len(orders) == 0 {
		return
	}

	// 检查交易状态
	now := time.Now()
	openOrders := make([]*ent.Order, 0)
	tokenBalanceChanges := make(map[int]map[string]solanautil.TokenBalanceChange)

	for _, item := range orders {
		changes, err := solanautil.GetTokenBalanceChanges(
			keeper.ctx, keeper.svcCtx.SolanaRpc, item.TxHash, item.Account)
		if err != nil {
			// 交易是否失败
			if solanautil.IsProgramError(err) {
				keeper.handleRejectOrder(item, changes, err.Error())
				continue
			}

			// 交易是否超时
			if err == solanautil.ErrTxNotFound {
				if now.Sub(item.CreateTime) > time.Minute*2 {
					keeper.handleRejectOrder(item, changes, "timeout")
				}
				continue
			}

			logger.Errorf("[OrderKeeper] 获取余额变化失败, hash: %s, %v", item.TxHash, err)
			continue
		}

		openOrders = append(openOrders, item)
		tokenBalanceChanges[item.ID] = changes
	}
	if len(openOrders) == 0 {
		return
	}

	// 更新订单信息
	for _, item := range openOrders {
		changes, ok := tokenBalanceChanges[item.ID]
		if !ok {
			continue
		}
		keeper.handleCloseOrder(item, changes)
	}
}
