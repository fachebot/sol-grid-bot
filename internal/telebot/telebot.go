package telebot

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/fachebot/sol-grid-bot/internal/logger"
	"github.com/fachebot/sol-grid-bot/internal/svc"
	"github.com/fachebot/sol-grid-bot/internal/telebot/handler/positionhandler"
	"github.com/fachebot/sol-grid-bot/internal/telebot/handler/settingshandler"
	"github.com/fachebot/sol-grid-bot/internal/telebot/handler/strategyhandler"
	"github.com/fachebot/sol-grid-bot/internal/telebot/handler/wallethandler"
	"github.com/fachebot/sol-grid-bot/internal/telebot/pathrouter"
	"github.com/fachebot/sol-grid-bot/internal/utils"
	"github.com/fachebot/sol-grid-bot/internal/utils/solanautil"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TeleBot struct {
	ctx      context.Context
	cancel   context.CancelFunc
	stopChan chan struct{}
	svcCtx   *svc.ServiceContext
	botApi   *tgbotapi.BotAPI
	router   *pathrouter.Router
}

func NewTeleBot(svcCtx *svc.ServiceContext) (*TeleBot, error) {
	ctx, cancel := context.WithCancel(context.Background())
	botService := &TeleBot{
		ctx:    ctx,
		cancel: cancel,
		svcCtx: svcCtx,
		botApi: svcCtx.BotApi,
		router: pathrouter.NewRouter(),
	}

	botService.initRoutes()
	return botService, nil
}

func (s *TeleBot) initRoutes() {
	s.router.HandleFunc("/home", func(
		ctx context.Context,
		vars map[string]string,
		userId int64,
		update tgbotapi.Update,
	) error {
		return s.handleHome(userId, update)
	})

	positionhandler.InitRoutes(s.svcCtx, s.botApi, s.router)
	settingshandler.InitRoutes(s.svcCtx, s.botApi, s.router)
	strategyhandler.InitRoutes(s.svcCtx, s.botApi, s.router)
	wallethandler.InitRoutes(s.svcCtx, s.botApi, s.router)
}

func (s *TeleBot) Stop() {
	if s.stopChan == nil {
		return
	}

	logger.Infof("[TeleBot] 准备停止服务")

	s.botApi.StopReceivingUpdates()
	s.cancel()

	<-s.stopChan
	close(s.stopChan)
	s.stopChan = nil

	logger.Infof("[TeleBot] 服务已经停止")
}

func (s *TeleBot) Start() {
	if s.stopChan != nil {
		return
	}

	s.stopChan = make(chan struct{})
	logger.Infof("[TeleBot] 开始运行服务")
	go s.run()
}

func (s *TeleBot) handleHome(userId int64, update tgbotapi.Update) error {
	// 确保生成账户
	w, err := wallethandler.GetUserWallet(s.ctx, s.svcCtx, userId)
	if err != nil {
		return err
	}

	// 查询账户余额
	balance, err := solanautil.GetBalance(s.ctx, s.svcCtx.SolanaRpc, w.Account)
	if err != nil {
		return err
	}

	// 查询USDC余额
	usdcBalance, decimals, err := solanautil.GetTokenBalance(s.ctx, s.svcCtx.SolanaRpc, solanautil.USDC, w.Account)
	if err != nil {
		usdcBalance = big.NewInt(0)
	}

	// 回复首页菜单
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📈 策略", "/strategy"),
			tgbotapi.NewInlineKeyboardButtonData("📊 仓位", "/position"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💳 钱包", "/wallet"),
			tgbotapi.NewInlineKeyboardButtonData("⚙️ 设置", "/settings"),
		),
	)
	text := fmt.Sprintf("Solana 网格机器人 | 盈利如春雨, 润物无声, 渐丰收! \n\n💳 我的钱包:\n`%s`\n\n💰    SOL余额: `%s`\n💰 USDC余额: `%s`",
		w.Account, solanautil.ParseSOL(balance).Truncate(5), solanautil.ParseUnits(usdcBalance, decimals).Truncate(5))
	text = text + fmt.Sprintf("\n\n[OKX](https://web3.okx.com/zh-hant/portfolio/%s/analysis?chainIndex=501) | [GMGN](https://gmgn.ai/sol/address/%s) | [Solscan](https://solscan.io/account/%s)", w.Account, w.Account, w.Account)
	_, err = utils.ReplyMessage(s.botApi, update, text, markup)
	if err != nil {
		logger.Debugf("[TeleBot] 处理主页失败, %v", err)
	}

	return nil
}

func (s *TeleBot) handleUpdate(update tgbotapi.Update) {
	// 获取用户ID
	var chat *tgbotapi.Chat
	if update.Message != nil {
		chat = update.Message.Chat
	} else if update.ChannelPost != nil {
		chat = update.ChannelPost.Chat
	} else if update.EditedMessage != nil {
		chat = update.EditedMessage.Chat
	} else if update.CallbackQuery != nil {
		chat = update.CallbackQuery.Message.Chat
	} else {
		return
	}

	userId := chat.ID
	logger.Debugf("[TeleBot] 收到新消息, chat: %d, username: %s, title: %s, type: %s",
		chat.ID, chat.UserName, chat.Title, chat.Type)

	if chat.Type != "private" {
		return
	}
	if !s.svcCtx.Config.TelegramBot.IsWhiteListUser(userId) {
		utils.SendMessage(s.botApi, userId, "🚫 非白名单用户, 不允许使用此机器人")
		return
	}

	// 处理文本消息
	if update.Message != nil {
		if update.Message.IsCommand() && update.Message.Text == "/start" {
			err := s.handleHome(userId, update)
			if err != nil {
				logger.Debugf("[TeleBot] 处理主页失败, %v", err)
			}
			return
		}

		if update.Message.IsCommand() && strings.HasPrefix(update.Message.Text, "/start quick ") {
			token := strings.TrimLeft(update.Message.Text, "/start quick ")
			path := strategyhandler.QuickStartStrategyHandler{}.FormatPath(token)
			err := s.router.Execute(s.ctx, path, userId, update)
			if err != nil {
				logger.Debugf("[TeleBot] 处理路由失败, path: %s, %v", path, err)
			}
			return
		}

		if update.Message.ReplyToMessage != nil {
			chatId := update.Message.ReplyToMessage.Chat.ID
			messageID := update.Message.ReplyToMessage.MessageID
			route, ok := s.svcCtx.MessageCache.GetRoute(chatId, messageID)
			if ok {
				err := s.router.Execute(s.ctx, route.Path, userId, update)
				if err != nil {
					logger.Debugf("[TeleBot] 处理路由失败, path: %s, %v", route.Path, err)
				}
			}
		}

		return
	}

	// 处理回调查询
	if update.CallbackQuery != nil {
		err := s.router.Execute(s.ctx, update.CallbackQuery.Data, userId, update)
		if err == nil {
			cb := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
			if _, err = s.botApi.Request(cb); err != nil {
				logger.Debugf("[TeleBot] 回答 CallbackQuery 失败, id: %s, %v", update.CallbackQuery.ID, err)
			}
		} else {
			logger.Errorf("[TeleBot] 处理 CallbackQuery 失败, %v", err)
			cb := tgbotapi.NewCallbackWithAlert(update.CallbackQuery.ID, "操作失败, 请稍后再试")
			if _, err = s.botApi.Request(cb); err != nil {
				logger.Debugf("[TeleBot] 回答 CallbackQuery 失败, id: %s, %v", update.CallbackQuery.ID, err)
			}
		}
	}
}

func (s *TeleBot) run() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 5
	updates := s.botApi.GetUpdatesChan(u)

	for {
		select {
		case <-s.ctx.Done():
			logger.Infof("[TeleBot] 上下文已取消")

			s.stopChan <- struct{}{}

			return
		case update := <-updates:
			s.handleUpdate(update)
		}
	}
}
