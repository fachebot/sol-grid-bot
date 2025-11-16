package strategy

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/fachebot/sol-grid-bot/internal/ent"
	"github.com/fachebot/sol-grid-bot/internal/ent/order"
	"github.com/fachebot/sol-grid-bot/internal/logger"
	"github.com/fachebot/sol-grid-bot/internal/svc"
	"github.com/fachebot/sol-grid-bot/internal/swap"
	"github.com/fachebot/sol-grid-bot/internal/utils/solanautil"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

func isDowntrend(trending []int) bool {
	if len((trending)) < 2 {
		return false
	}

	return trending[len(trending)-2] > trending[len(trending)-1]
}

func encodeGridTrend(trending []int) string {
	if len(trending) == 0 {
		return ""
	}

	slice := lo.Map(trending, func(item int, idx int) string {
		return strconv.Itoa(item)
	})
	return strings.Join(slice, ":")
}

func decodeGridTrend(trending *string) []int {
	if trending == nil {
		return nil
	}

	slice := strings.Split(*trending, ":")
	if len(slice) == 0 {
		return nil
	}

	result := make([]int, 0)
	for _, item := range slice {
		v, err := strconv.Atoi(item)
		if err == nil {
			result = append(result, v)
		}
	}

	return result
}

func updateGridTrend(trending []int, gridNumber int) []int {
	if len(trending) == 0 {
		return append([]int{}, gridNumber)
	}

	if trending[len(trending)-1] == gridNumber {
		return append([]int{}, trending...)
	}

	trending = append([]int{}, trending...)
	trending = append(trending, gridNumber)
	if len(trending) > 2 {
		trending = trending[len(trending)-2:]
	}
	return trending
}

func isMinGridNumber(gridRecords []*ent.Grid, gridNumber int) bool {
	for _, item := range gridRecords {
		if item.GridNumber < gridNumber {
			return false
		}
	}
	return true
}

func SellToken(ctx context.Context, svcCtx *svc.ServiceContext, strategyRecord *ent.Strategy, title string, uiSellAmount, minSellPrice *decimal.Decimal, exit bool) (ent.Order, error) {
	// 获取用户钱包
	w, err := svcCtx.WalletModel.FindByUserId(ctx, strategyRecord.UserId)
	if err != nil {
		logger.Errorf("[GridStrategy] %s - 获取用户钱包失败, userId: %d, %v", title, strategyRecord.UserId, err)
		return ent.Order{}, err
	}

	// 获取代币余额
	tokenBalance, decimals, err := solanautil.GetTokenBalance(ctx, svcCtx.SolanaRpc, strategyRecord.Token, w.Account)
	if err != nil {
		logger.Debugf("[GridStrategy] %s - 获取代币余额失败, token: %s, %v", title, strategyRecord.Token, err)
		return ent.Order{}, err
	}
	uiTokenBalance := solanautil.ParseUnits(tokenBalance, decimals)
	if uiSellAmount == nil {
		uiSellAmount = &uiTokenBalance
	} else if uiSellAmount.GreaterThan(uiTokenBalance) {
		uiSellAmount = &uiTokenBalance
		logger.Warnf("[GridStrategy] %s - 卖出数量大于当前余额, strategy: %s, token: %s, quantity: %v, balance: %v",
			title, strategyRecord.GUID, strategyRecord.Token, uiSellAmount, uiTokenBalance)
	}

	// 获取报价
	sellAmount := solanautil.FormatUnits(*uiSellAmount, decimals)
	swapService := swap.NewSwapService(svcCtx, w.UserId)
	tx, err := swapService.Quote(ctx, strategyRecord.Token, solanautil.USDC, sellAmount, exit)
	if err != nil {
		logger.Errorf("[GridStrategy] %s - 获取报价失败, in: %s, out: USDC, amount: %s, %v", title, strategyRecord.Symbol, uiSellAmount, err)
		return ent.Order{}, err
	}

	uiOutAmount := solanautil.ParseUnits(tx.OutAmount(), solanautil.USDCDecimals)
	quotePrice := uiOutAmount.Div(*uiSellAmount)
	if minSellPrice != nil && quotePrice.LessThan(*minSellPrice) {
		logger.Debugf("[GridStrategy] %s - 报价高于底价, 取消交易, token: %s, quotePrice: %s, bottomPrice: %s", title, strategyRecord.Symbol, quotePrice, *minSellPrice)
		return ent.Order{}, errors.New("price too low")
	}

	// 发送交易
	hash, err := tx.Swap(ctx)
	if err != nil {
		logger.Errorf("[GridStrategy] %s - 发送交易失败, user: %d, inToken: %s, inputAmount: %s, outAmount: %s, hash: %s, %v",
			title, w.UserId, strategyRecord.Symbol, uiSellAmount, uiOutAmount, hash, err)
		return ent.Order{}, err
	}

	logger.Infof("[GridStrategy] %s - 提交交易成功, user: %d, inToken: %s, inputAmount: %s, outAmount: %s, hash: %s",
		title, w.UserId, strategyRecord.Symbol, uiSellAmount, uiOutAmount, hash)

	// 订单记录
	orderArgs := ent.Order{
		Account:    tx.Signer(),
		Token:      strategyRecord.Token,
		Symbol:     strategyRecord.Symbol,
		StrategyId: strategyRecord.GUID,
		Type:       order.TypeSell,
		Price:      quotePrice,
		FinalPrice: quotePrice,
		InAmount:   *uiSellAmount,
		OutAmount:  uiOutAmount,
		Status:     order.StatusPending,
		TxHash:     hash,
	}
	return orderArgs, nil
}
