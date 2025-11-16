package strategyhandler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/fachebot/sol-grid-bot/internal/ent"
	"github.com/fachebot/sol-grid-bot/internal/ent/order"
	"github.com/fachebot/sol-grid-bot/internal/logger"
	"github.com/fachebot/sol-grid-bot/internal/svc"
	"github.com/fachebot/sol-grid-bot/internal/telebot/pathrouter"
	"github.com/fachebot/sol-grid-bot/internal/utils"
	"github.com/fachebot/sol-grid-bot/internal/utils/format"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type StrategyTradesHandler struct {
	botApi *tgbotapi.BotAPI
	svcCtx *svc.ServiceContext
}

func NewStrategyTradesHandler(svcCtx *svc.ServiceContext, botApi *tgbotapi.BotAPI) *StrategyTradesHandler {
	return &StrategyTradesHandler{botApi: botApi, svcCtx: svcCtx}
}

func (h StrategyTradesHandler) FormatPath(guid string, page int) string {
	return fmt.Sprintf("/strategy/trades/%s/%d", guid, page)
}

func (h *StrategyTradesHandler) AddRouter(router *pathrouter.Router) {
	router.HandleFunc("/strategy/trades/{uuid}/{page:[0-9]+}", h.handle)
}

func (h *StrategyTradesHandler) handle(ctx context.Context, vars map[string]string, userId int64, update tgbotapi.Update) error {
	guid, ok := vars["uuid"]
	if !ok {
		return nil
	}

	var page int
	val, ok := vars["page"]
	if !ok {
		page = 1
	} else {
		n, err := strconv.Atoi((val))
		if err != nil {
			page = 1
		} else {
			page = n
		}
	}

	if page < 1 {
		return nil
	}

	// 查询策略信息
	record, err := h.svcCtx.StrategyModel.FindByUserIdGUID(ctx, userId, guid)
	if err != nil {
		if ent.IsNotFound(err) {
			return DisplayStrategyList(ctx, h.svcCtx, h.botApi, userId, update, 1)
		}
		logger.Errorf("[StrategyDetailsHandler] 查询策略失败, id: %s, %v", guid, err)
		return nil
	}

	// 查询钱包交易记录
	const limit = 10
	offset := (page - 1) * limit
	orders, total, err := h.svcCtx.OrderModel.FindOrdersByStrategyId(ctx, guid, offset, limit)
	if err != nil {
		logger.Errorf("[StrategyDetailsHandler] 查询用户订单列表失败, userId: %d, strategy: %s, %v", userId, guid, err)
		return nil
	}

	totalPage := total / limit
	if total%limit != 0 {
		totalPage += 1
	}

	if page > totalPage {
		page = totalPage
		offset := (page - 1) * limit
		orders, total, err = h.svcCtx.OrderModel.FindOrdersByStrategyId(ctx, guid, offset, limit)
		if err != nil {
			logger.Errorf("[StrategyDetailsHandler] 查询用户订单列表失败, userId: %d, strategy: %s, %v", userId, guid, err)
			return nil
		}
	}

	// 多页翻页功能
	var pageButtons []tgbotapi.InlineKeyboardButton
	if total > limit {
		nextPage := page + 1
		previousPage := page - 1
		if previousPage < 1 {
			page = 1
			previousPage = 0
		}
		if nextPage > totalPage {
			page = totalPage
			nextPage = 0
		}
		pageButtons = []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("⬅️ 上一页", h.FormatPath(guid, previousPage)),
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d/%d", page, totalPage), h.FormatPath(guid, 0)),
			tgbotapi.NewInlineKeyboardButtonData("➡️ 下一页", h.FormatPath(guid, nextPage)),
		}
	}

	// 生成交易记录
	items := make([]string, 0)
	for _, item := range orders {
		var status string
		switch item.Status {
		case order.StatusPending:
			status = "❓"
		case order.StatusClosed:
			status = "✅"
		case order.StatusRejected:
			status = "❌"
		}
		finalPrice := format.Price(item.FinalPrice, 5)

		if item.Type == order.TypeBuy {
			if item.GridNumber != nil {
				items = append(items, fmt.Sprintf("*%s* 🟢 买入`#%d` %sU, 价格 %s %s [>>](https://solscan.io/tx/%s)",
					utils.FormaDate(item.CreateTime), *item.GridNumber, item.InAmount.Truncate(2), finalPrice, status, item.TxHash))
			}
		} else if item.Type == order.TypeSell {
			if item.GridNumber == nil {
				items = append(items, fmt.Sprintf("*%s* 🔴 清仓 %sU, 价格 %s %s [>>](https://solscan.io/tx/%s)",
					utils.FormaDate(item.CreateTime), item.OutAmount.Truncate(2), finalPrice, status, item.TxHash))
			} else {
				items = append(items, fmt.Sprintf("*%s* 🔴 卖出`#%d` %sU, 价格 %s %s [>>](https://solscan.io/tx/%s)",
					utils.FormaDate(item.CreateTime), *item.GridNumber, item.OutAmount.Truncate(2), finalPrice, status, item.TxHash))
			}
		}
	}
	text := fmt.Sprintf("Solana 网格机器人 | *%s* 交易记录\n\n", strings.TrimRight(record.Symbol, "\u0000"))
	text = text + strings.Join(items, "\n\n")

	var rows [][]tgbotapi.InlineKeyboardButton
	if len(pageButtons) > 0 {
		rows = append(rows, pageButtons)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ 返回上级", StrategyDetailsHandler{}.FormatPath(guid)),
		tgbotapi.NewInlineKeyboardButtonData("⏪ 返回主页", "/home"),
	))
	markup := tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err = utils.ReplyMessage(h.botApi, update, text, markup)
	return err
}
