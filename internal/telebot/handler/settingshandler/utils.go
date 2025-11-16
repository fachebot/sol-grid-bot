package settingshandler

import (
	"context"
	"fmt"
	"strings"

	"github.com/fachebot/sol-grid-bot/internal/ent"
	"github.com/fachebot/sol-grid-bot/internal/ent/settings"
	"github.com/fachebot/sol-grid-bot/internal/svc"
	"github.com/fachebot/sol-grid-bot/internal/utils"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func getSettingsMenuText() string {
	items := []string{
		"1️⃣ *聚合器:* 指定使用的去中心化交易所聚合器",
		"2️⃣ *交易滑点:* 交易允许的价格滑点",
		"3️⃣ *优先级别:* 设置交易的优先级级别",
		"4️⃣ *交易最大重试次数:* 交易失败后最大重试次数",
		"5️⃣ *交易最大Lamports:* 交易中允许使用的最大Lamports数量",
	}

	text := "Solana 网格机器人 | 用户配置"
	text = fmt.Sprintf("%s\n\n%s", text, strings.Join(items, "\n"))
	return text
}

func getUserSettings(ctx context.Context, svcCtx *svc.ServiceContext, userId int64) (*ent.Settings, error) {
	record, err := svcCtx.SettingsModel.FindByUserId(ctx, userId)
	if err == nil {
		return record, nil
	}

	if !ent.IsNotFound(err) {
		return nil, err
	}

	c := svcCtx.Config.Solana
	args := ent.Settings{
		UserId:        userId,
		MaxRetries:    int64(c.MaxRetries),
		SlippageBps:   c.SlippageBps,
		MaxLamports:   c.MaxLamports,
		PriorityLevel: settings.PriorityLevel(c.PriorityLevel),
		DexAggregator: settings.DexAggregator(c.DexAggregator),
	}
	return svcCtx.SettingsModel.Save(ctx, args)
}

func displaySettingsMenu(botApi *tgbotapi.BotAPI, update tgbotapi.Update, record *ent.Settings) error {
	text := getSettingsMenuText()
	sellSlippageBps := float64(record.SlippageBps) / 10000 * 100
	if record.SellSlippageBps != nil {
		sellSlippageBps = float64(*record.SellSlippageBps) / 10000 * 100
	}

	exitSlippageBps := sellSlippageBps
	if record.ExitSlippageBps != nil {
		exitSlippageBps = float64(*record.ExitSlippageBps) / 10000 * 100
	}

	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("聚合器: %s", record.DexAggregator), SetDexAggHandler{}.FormatPath()),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("优先级别: %s", record.PriorityLevel), SetPriorityLevelHandler{}.FormatPath()),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("买入滑点: %v%%", float64(record.SlippageBps)/10000*100), SettingsHomeHandler{}.FormatPath(&SettingsOptionSlippageBps)),
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("卖出滑点: %v%%", sellSlippageBps), SettingsHomeHandler{}.FormatPath(&SettingsOptionSellSlippageBps)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("清仓交易滑点: %v%%", exitSlippageBps), SettingsHomeHandler{}.FormatPath(&SettingsOptionExitSlippageBps)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("交易最大重试次数: %d", record.MaxRetries), SettingsHomeHandler{}.FormatPath(&SettingsOptionMaxRetries)),
		),
		// tgbotapi.NewInlineKeyboardRow(
		// 	tgbotapi.NewInlineKeyboardButtonData(
		// 		fmt.Sprintf("交易最大Lamports: %d", record.MaxLamports), SettingsHomeHandler{}.FormatPath(&SettingsOptionMaxLamports)),
		// ),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ 返回主页", "/home"),
		),
	)
	_, err := utils.ReplyMessage(botApi, update, text, markup)
	return err
}
