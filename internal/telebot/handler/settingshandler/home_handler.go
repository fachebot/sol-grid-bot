package settingshandler

import (
	"context"
	"fmt"
	"strconv"

	"github.com/fachebot/sol-grid-bot/internal/cache"
	"github.com/fachebot/sol-grid-bot/internal/ent"
	"github.com/fachebot/sol-grid-bot/internal/logger"
	"github.com/fachebot/sol-grid-bot/internal/svc"
	"github.com/fachebot/sol-grid-bot/internal/telebot/pathrouter"
	"github.com/fachebot/sol-grid-bot/internal/utils"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/shopspring/decimal"
)

type SettingsOption int

var (
	SettingsOptionMaxRetries      SettingsOption = 1
	SettingsOptionSlippageBps     SettingsOption = 2
	SettingsOptionMaxLamports     SettingsOption = 3
	SettingsOptionPriorityLevel   SettingsOption = 4
	SettingsOptionDexAggregator   SettingsOption = 5
	SettingsOptionSellSlippageBps SettingsOption = 6
	SettingsOptionExitSlippageBps SettingsOption = 7
)

func InitRoutes(svcCtx *svc.ServiceContext, botApi *tgbotapi.BotAPI, router *pathrouter.Router) {
	NewSettingsHomeHandler(svcCtx, botApi).AddRouter(router)
	NewSetDexAggHandler(svcCtx, botApi).AddRouter(router)
	NewSetPriorityLevelHandler(svcCtx, botApi).AddRouter(router)
}

type SettingsHomeHandler struct {
	botApi *tgbotapi.BotAPI
	svcCtx *svc.ServiceContext
}

func NewSettingsHomeHandler(svcCtx *svc.ServiceContext, botApi *tgbotapi.BotAPI) *SettingsHomeHandler {
	return &SettingsHomeHandler{botApi: botApi, svcCtx: svcCtx}
}

func (h SettingsHomeHandler) FormatPath(option *SettingsOption) string {
	if option == nil {
		return "/settings"
	}
	return fmt.Sprintf("/settings/set/%d", *option)
}

func (h *SettingsHomeHandler) AddRouter(router *pathrouter.Router) {
	router.HandleFunc("/settings", h.handle)
	router.HandleFunc("/settings/set/{option}", h.handle)
}

func (h *SettingsHomeHandler) handle(ctx context.Context, vars map[string]string, userId int64, update tgbotapi.Update) error {
	record, err := getUserSettings(ctx, h.svcCtx, userId)
	if err != nil {
		logger.Errorf("[SettingsHomeHandler] 查询用户设置失败, userId: %d, %v", userId, err)
		return err
	}

	option, ok := vars["option"]
	if !ok {
		return displaySettingsMenu(h.botApi, update, record)
	}

	optionValue, err := strconv.Atoi(option)
	if err != nil {
		return err
	}

	switch SettingsOption(optionValue) {
	case SettingsOptionSlippageBps:
		return h.handleSlippageBps(ctx, update, record)
	case SettingsOptionMaxRetries:
		return h.handleMaxRetries(ctx, update, record)
	case SettingsOptionMaxLamports:
		return h.handleMaxLamports(ctx, update, record)
	case SettingsOptionSellSlippageBps:
		return h.handleSellSlippageBps(ctx, update, record)
	case SettingsOptionExitSlippageBps:
		return h.handleExitSlippageBps(ctx, update, record)
	}

	return nil
}

func (h *SettingsHomeHandler) handleSlippageBps(ctx context.Context, update tgbotapi.Update, record *ent.Settings) error {
	// 步骤1
	if update.CallbackQuery != nil {
		chatId := update.CallbackQuery.Message.Chat.ID
		text := "🌳 填写买入交易允许的价格滑点\n\n💵 例如: 10｜代表 10% , 单位是 %"
		c := tgbotapi.NewMessage(chatId, text)
		c.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true}

		msg, err := h.botApi.Send(c)
		if err != nil {
			logger.Debugf("[SettingsHomeHandler] 发送消息失败, %v", err)
			return err
		}

		route := cache.RouteInfo{Path: h.FormatPath(&SettingsOptionSlippageBps), Context: update.CallbackQuery.Message}
		h.svcCtx.MessageCache.SetRoute(chatId, msg.MessageID, route)

		return nil
	}

	// 步骤2
	if update.Message != nil {
		chatId := update.Message.Chat.ID
		deleteMessages := []int{update.Message.MessageID}
		if update.Message.ReplyToMessage != nil {
			deleteMessages = append(deleteMessages, update.Message.ReplyToMessage.MessageID)
		}
		utils.DeleteMessages(h.botApi, chatId, deleteMessages, 0)

		// 检查输入滑点
		d, err := decimal.NewFromString(update.Message.Text)
		if err != nil || d.LessThanOrEqual(decimal.Zero) {
			utils.SendMessageAndDelayDeletion(h.botApi, chatId, "⚠️ 请输入有效数字", 1)
			return nil
		} else if d.GreaterThan(decimal.NewFromInt(20)) {
			utils.SendMessageAndDelayDeletion(h.botApi, chatId, "⚠️ 滑点最大不能超过20%", 1)
			return nil
		}

		slippageBps := int(d.Div(decimal.NewFromInt(100)).Mul(decimal.NewFromInt(10000)).IntPart())
		if slippageBps == record.SlippageBps {
			return nil
		}

		// 发送成功提示
		text := "✅ 配置修改成功"
		err = h.svcCtx.SettingsModel.UpdateSlippageBps(ctx, record.ID, slippageBps)
		if err == nil {
			record.SlippageBps = slippageBps
		} else {
			text = "❌ 配置修改失败, 请稍后重试"
			logger.Errorf("[SettingsHomeHandler] 更新配置[SlippageBps]失败, %v", err)
		}
		utils.SendMessageAndDelayDeletion(h.botApi, chatId, text, 1)

		// 更新用户界面
		if update.Message.ReplyToMessage == nil {
			return displaySettingsMenu(h.botApi, update, record)
		} else {
			route, ok := h.svcCtx.MessageCache.GetRoute(chatId, update.Message.ReplyToMessage.MessageID)
			if ok && route.Context != nil {
				return displaySettingsMenu(h.botApi, tgbotapi.Update{Message: route.Context}, record)
			}
			return displaySettingsMenu(h.botApi, update, record)
		}
	}

	return nil
}

func (h *SettingsHomeHandler) handleSellSlippageBps(ctx context.Context, update tgbotapi.Update, record *ent.Settings) error {
	// 步骤1
	if update.CallbackQuery != nil {
		chatId := update.CallbackQuery.Message.Chat.ID
		text := "🌳 填写卖出交易允许的价格滑点\n\n💵 例如: 10｜代表 10% , 单位是 %"
		c := tgbotapi.NewMessage(chatId, text)
		c.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true}

		msg, err := h.botApi.Send(c)
		if err != nil {
			logger.Debugf("[SettingsHomeHandler] 发送消息失败, %v", err)
			return err
		}

		route := cache.RouteInfo{Path: h.FormatPath(&SettingsOptionSellSlippageBps), Context: update.CallbackQuery.Message}
		h.svcCtx.MessageCache.SetRoute(chatId, msg.MessageID, route)

		return nil
	}

	// 步骤2
	if update.Message != nil {
		chatId := update.Message.Chat.ID
		deleteMessages := []int{update.Message.MessageID}
		if update.Message.ReplyToMessage != nil {
			deleteMessages = append(deleteMessages, update.Message.ReplyToMessage.MessageID)
		}
		utils.DeleteMessages(h.botApi, chatId, deleteMessages, 0)

		// 检查输入滑点
		d, err := decimal.NewFromString(update.Message.Text)
		if err != nil || d.LessThanOrEqual(decimal.Zero) {
			utils.SendMessageAndDelayDeletion(h.botApi, chatId, "⚠️ 请输入有效数字", 1)
			return nil
		} else if d.GreaterThan(decimal.NewFromInt(20)) {
			utils.SendMessageAndDelayDeletion(h.botApi, chatId, "⚠️ 滑点最大不能超过20%", 1)
			return nil
		}

		slippageBps := int(d.Div(decimal.NewFromInt(100)).Mul(decimal.NewFromInt(10000)).IntPart())
		if (record.SellSlippageBps == nil && slippageBps == 0) || (record.SellSlippageBps != nil && slippageBps == *record.SellSlippageBps) {
			return nil
		}

		// 发送成功提示
		text := "✅ 配置修改成功"
		err = h.svcCtx.SettingsModel.UpdateSellSlippageBps(ctx, record.ID, slippageBps)
		if err == nil {
			record.SellSlippageBps = &slippageBps
		} else {
			text = "❌ 配置修改失败, 请稍后重试"
			logger.Errorf("[SettingsHomeHandler] 更新配置[SellSlippageBps]失败, %v", err)
		}
		utils.SendMessageAndDelayDeletion(h.botApi, chatId, text, 1)

		// 更新用户界面
		if update.Message.ReplyToMessage == nil {
			return displaySettingsMenu(h.botApi, update, record)
		} else {
			route, ok := h.svcCtx.MessageCache.GetRoute(chatId, update.Message.ReplyToMessage.MessageID)
			if ok && route.Context != nil {
				return displaySettingsMenu(h.botApi, tgbotapi.Update{Message: route.Context}, record)
			}
			return displaySettingsMenu(h.botApi, update, record)
		}
	}

	return nil
}

func (h *SettingsHomeHandler) handleExitSlippageBps(ctx context.Context, update tgbotapi.Update, record *ent.Settings) error {
	// 步骤1
	if update.CallbackQuery != nil {
		chatId := update.CallbackQuery.Message.Chat.ID
		text := "🌳 填写清仓交易允许的价格滑点\n\n💵 例如: 10｜代表 10% , 单位是 %"
		c := tgbotapi.NewMessage(chatId, text)
		c.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true}

		msg, err := h.botApi.Send(c)
		if err != nil {
			logger.Debugf("[SettingsHomeHandler] 发送消息失败, %v", err)
			return err
		}

		route := cache.RouteInfo{Path: h.FormatPath(&SettingsOptionExitSlippageBps), Context: update.CallbackQuery.Message}
		h.svcCtx.MessageCache.SetRoute(chatId, msg.MessageID, route)

		return nil
	}

	// 步骤2
	if update.Message != nil {
		chatId := update.Message.Chat.ID
		deleteMessages := []int{update.Message.MessageID}
		if update.Message.ReplyToMessage != nil {
			deleteMessages = append(deleteMessages, update.Message.ReplyToMessage.MessageID)
		}
		utils.DeleteMessages(h.botApi, chatId, deleteMessages, 0)

		// 检查输入滑点
		d, err := decimal.NewFromString(update.Message.Text)
		if err != nil || d.LessThanOrEqual(decimal.Zero) {
			utils.SendMessageAndDelayDeletion(h.botApi, chatId, "⚠️ 请输入有效数字", 1)
			return nil
		} else if d.GreaterThan(decimal.NewFromInt(20)) {
			utils.SendMessageAndDelayDeletion(h.botApi, chatId, "⚠️ 滑点最大不能超过20%", 1)
			return nil
		}

		slippageBps := int(d.Div(decimal.NewFromInt(100)).Mul(decimal.NewFromInt(10000)).IntPart())
		if (record.ExitSlippageBps == nil && slippageBps == 0) || (record.ExitSlippageBps != nil && slippageBps == *record.ExitSlippageBps) {
			return nil
		}

		// 发送成功提示
		text := "✅ 配置修改成功"
		err = h.svcCtx.SettingsModel.UpdateExitSlippageBps(ctx, record.ID, slippageBps)
		if err == nil {
			record.ExitSlippageBps = &slippageBps
		} else {
			text = "❌ 配置修改失败, 请稍后重试"
			logger.Errorf("[SettingsHomeHandler] 更新配置[ExitSlippageBps]失败, %v", err)
		}
		utils.SendMessageAndDelayDeletion(h.botApi, chatId, text, 1)

		// 更新用户界面
		if update.Message.ReplyToMessage == nil {
			return displaySettingsMenu(h.botApi, update, record)
		} else {
			route, ok := h.svcCtx.MessageCache.GetRoute(chatId, update.Message.ReplyToMessage.MessageID)
			if ok && route.Context != nil {
				return displaySettingsMenu(h.botApi, tgbotapi.Update{Message: route.Context}, record)
			}
			return displaySettingsMenu(h.botApi, update, record)
		}
	}

	return nil
}

func (h *SettingsHomeHandler) handleMaxRetries(ctx context.Context, update tgbotapi.Update, record *ent.Settings) error {
	// 步骤1
	if update.CallbackQuery != nil {
		chatId := update.CallbackQuery.Message.Chat.ID
		text := "🌳 填写交易失败后最大重试次数"
		c := tgbotapi.NewMessage(chatId, text)
		c.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true}

		msg, err := h.botApi.Send(c)
		if err != nil {
			logger.Debugf("[SettingsHomeHandler] 发送消息失败, %v", err)
			return err
		}

		route := cache.RouteInfo{Path: h.FormatPath(&SettingsOptionMaxRetries), Context: update.CallbackQuery.Message}
		h.svcCtx.MessageCache.SetRoute(chatId, msg.MessageID, route)

		return nil
	}

	// 步骤2
	if update.Message != nil {
		chatId := update.Message.Chat.ID
		deleteMessages := []int{update.Message.MessageID}
		if update.Message.ReplyToMessage != nil {
			deleteMessages = append(deleteMessages, update.Message.ReplyToMessage.MessageID)
		}
		utils.DeleteMessages(h.botApi, chatId, deleteMessages, 0)

		// 检查输入
		maxRetries, err := strconv.Atoi(update.Message.Text)
		if err != nil || maxRetries < 0 {
			utils.SendMessageAndDelayDeletion(h.botApi, chatId, "⚠️ 请输入有效数字", 1)
			return nil
		} else if maxRetries > 10 {
			utils.SendMessageAndDelayDeletion(h.botApi, chatId, "⚠️ 最大重试次数不能大于10", 1)
			return nil
		}

		// 发送成功提示
		text := "✅ 配置修改成功"
		err = h.svcCtx.SettingsModel.UpdateMaxRetries(ctx, record.ID, int64(maxRetries))
		if err == nil {
			record.MaxRetries = int64(maxRetries)
		} else {
			text = "❌ 配置修改失败, 请稍后重试"
			logger.Errorf("[SettingsHomeHandler] 更新配置[MaxRetries]失败, %v", err)
		}
		utils.SendMessageAndDelayDeletion(h.botApi, chatId, text, 1)

		// 更新用户界面
		if update.Message.ReplyToMessage == nil {
			return displaySettingsMenu(h.botApi, update, record)
		} else {
			route, ok := h.svcCtx.MessageCache.GetRoute(chatId, update.Message.ReplyToMessage.MessageID)
			if ok && route.Context != nil {
				return displaySettingsMenu(h.botApi, tgbotapi.Update{Message: route.Context}, record)
			}
			return displaySettingsMenu(h.botApi, update, record)
		}
	}

	return nil
}

func (h *SettingsHomeHandler) handleMaxLamports(ctx context.Context, update tgbotapi.Update, record *ent.Settings) error {
	// 步骤1
	if update.CallbackQuery != nil {
		chatId := update.CallbackQuery.Message.Chat.ID
		text := "🌳 填写交易中允许使用的最大Lamports数量, 建议值: `50000000`"
		c := tgbotapi.NewMessage(chatId, text)
		c.ParseMode = tgbotapi.ModeMarkdown
		c.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true}

		msg, err := h.botApi.Send(c)
		if err != nil {
			logger.Debugf("[SettingsHomeHandler] 发送消息失败, %v", err)
			return err
		}

		route := cache.RouteInfo{Path: h.FormatPath(&SettingsOptionMaxLamports), Context: update.CallbackQuery.Message}
		h.svcCtx.MessageCache.SetRoute(chatId, msg.MessageID, route)

		return nil
	}

	// 步骤2
	if update.Message != nil {
		chatId := update.Message.Chat.ID
		deleteMessages := []int{update.Message.MessageID}
		if update.Message.ReplyToMessage != nil {
			deleteMessages = append(deleteMessages, update.Message.ReplyToMessage.MessageID)
		}
		utils.DeleteMessages(h.botApi, chatId, deleteMessages, 0)

		// 检查输入
		maxLamports, err := strconv.Atoi(update.Message.Text)
		if err != nil || maxLamports <= 0 {
			utils.SendMessageAndDelayDeletion(h.botApi, chatId, "⚠️ 请输入有效数字", 1)
			return nil
		}

		// 发送成功提示
		text := "✅ 配置修改成功"
		err = h.svcCtx.SettingsModel.UpdateMaxLamports(ctx, record.ID, int64(maxLamports))
		if err == nil {
			record.MaxLamports = int64(maxLamports)
		} else {
			text = "❌ 配置修改失败, 请稍后重试"
			logger.Errorf("[SettingsHomeHandler] 更新配置[MaxLamports]失败, %v", err)
		}
		utils.SendMessageAndDelayDeletion(h.botApi, chatId, text, 1)

		// 更新用户界面
		if update.Message.ReplyToMessage == nil {
			return displaySettingsMenu(h.botApi, update, record)
		} else {
			route, ok := h.svcCtx.MessageCache.GetRoute(chatId, update.Message.ReplyToMessage.MessageID)
			if ok && route.Context != nil {
				return displaySettingsMenu(h.botApi, tgbotapi.Update{Message: route.Context}, record)
			}
			return displaySettingsMenu(h.botApi, update, record)
		}
	}

	return nil
}
