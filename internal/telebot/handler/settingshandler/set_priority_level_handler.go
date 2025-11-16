package settingshandler

import (
	"context"
	"fmt"

	"github.com/fachebot/sol-grid-bot/internal/ent/settings"
	"github.com/fachebot/sol-grid-bot/internal/logger"
	"github.com/fachebot/sol-grid-bot/internal/svc"
	"github.com/fachebot/sol-grid-bot/internal/telebot/pathrouter"
	"github.com/fachebot/sol-grid-bot/internal/utils"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type SetPriorityLevelHandler struct {
	botApi *tgbotapi.BotAPI
	svcCtx *svc.ServiceContext
}

func NewSetPriorityLevelHandler(svcCtx *svc.ServiceContext, botApi *tgbotapi.BotAPI) *SetPriorityLevelHandler {
	return &SetPriorityLevelHandler{botApi: botApi, svcCtx: svcCtx}
}

func (h SetPriorityLevelHandler) FormatPath(priorityLevel ...settings.PriorityLevel) string {
	if len(priorityLevel) == 0 {
		return "/settings/priority_level"
	}
	return fmt.Sprintf("/settings/priority_level/%s", priorityLevel[0].String())
}

func (h *SetPriorityLevelHandler) AddRouter(router *pathrouter.Router) {
	router.HandleFunc("/settings/priority_level", h.handle)
	router.HandleFunc("/settings/priority_level/{value}", h.handle)
}

func (h *SetPriorityLevelHandler) handle(ctx context.Context, vars map[string]string, userId int64, update tgbotapi.Update) error {
	// 处理选项列表
	value, ok := vars["value"]
	if !ok {
		text := getSettingsMenuText()
		markup := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("medium", h.FormatPath(settings.PriorityLevelMedium)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("high", h.FormatPath(settings.PriorityLevelHigh)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("veryHigh", h.FormatPath(settings.PriorityLevelVeryHigh)),
			),
		)
		_, err := utils.ReplyMessage(h.botApi, update, text, markup)
		return err
	}

	// 获取用户设置
	record, err := getUserSettings(ctx, h.svcCtx, userId)
	if err != nil {
		logger.Errorf("[SetPriorityLevelHandler] 查询用户设置失败, userId: %d, %v", userId, err)
		return err
	}

	// 更新优先级别
	priorityLevel := settings.PriorityLevel(value)
	if settings.PriorityLevelValidator(priorityLevel) == nil {
		err = h.svcCtx.SettingsModel.UpdatePriorityLevel(ctx, record.ID, priorityLevel)
		if err != nil {
			logger.Errorf("[SetPriorityLevelHandler] 更新 PriorityLevel 配置失败, userId: %d, %v", userId, err)
			return err
		}

		record.PriorityLevel = priorityLevel
	}

	displaySettingsMenu(h.botApi, update, record)
	return nil
}
