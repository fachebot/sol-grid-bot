package svc

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"

	"github.com/fachebot/sol-grid-bot/internal/cache"
	"github.com/fachebot/sol-grid-bot/internal/config"
	"github.com/fachebot/sol-grid-bot/internal/datapi/gmgn"
	"github.com/fachebot/sol-grid-bot/internal/datapi/jupag"
	"github.com/fachebot/sol-grid-bot/internal/datapi/okxweb3"
	"github.com/fachebot/sol-grid-bot/internal/engine"
	"github.com/fachebot/sol-grid-bot/internal/ent"
	"github.com/fachebot/sol-grid-bot/internal/logger"
	"github.com/fachebot/sol-grid-bot/internal/model"
	"github.com/fachebot/sol-grid-bot/internal/utils"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/net/proxy"
)

type ServiceContext struct {
	Config           *config.Config
	HashEncoder      *utils.HashEncoder
	Engine           *engine.StrategyEngine
	DbClient         *ent.Client
	BotApi           *tgbotapi.BotAPI
	BotUserInfo      *tgbotapi.User
	SolanaRpc        *rpc.Client
	OkxClient        *okxweb3.Client
	GmgnClient       *gmgn.Client
	JupagClient      *jupag.Client
	TransportProxy   *http.Transport
	LookuptableCache *cache.LookuptableCache
	MessageCache     *cache.MessageCache
	TokenMetaCache   *cache.TokenMetaCache
	GridModel        *model.GridModel
	OrderModel       *model.OrderModel
	SettingsModel    *model.SettingsModel
	StrategyModel    *model.StrategyModel
	WalletModel      *model.WalletModel
}

func NewServiceContext(c *config.Config, strategyEngine *engine.StrategyEngine) *ServiceContext {
	// 创建hash编码器
	salt := os.Getenv("GRIDBOT_HASH_SALT")
	if salt == "" {
		salt = "8wKzxf51vQJT5n=bM6e?z)6B]XiDXcMdE]=>GiXm"
		logger.Debugf("环境变量 GRIDBOT_HASH_SALT 未设置")
	}
	hashEncoder, err := utils.NewHashEncoder(salt)
	if err != nil {
		logger.Fatalf("创建Hash编码器失败, %v", err)
	}

	// 创建数据库连接
	client, err := ent.Open("sqlite3", "file:data/sqlite.db?mode=rwc&_journal_mode=WAL&_fk=1")
	if err != nil {
		logger.Fatalf("打开数据库失败, %v", err)
	}
	if err := client.Schema.Create(context.Background()); err != nil {
		logger.Fatalf("创建数据库Schema失败, %v", err)
	}

	// 创建SOCKS5代理
	var transportProxy *http.Transport
	if c.Sock5Proxy.Enable {
		socks5Proxy := fmt.Sprintf("%s:%d", c.Sock5Proxy.Host, c.Sock5Proxy.Port)
		dialer, err := proxy.SOCKS5("tcp", socks5Proxy, nil, proxy.Direct)
		if err != nil {
			logger.Fatalf("创建SOCKS5代理失败, %v", err)
		}

		transportProxy = &http.Transport{
			Dial:            dialer.Dial,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	// 创建电报机器人
	tgHttpClient := new(http.Client)
	if transportProxy != nil {
		tgHttpClient.Transport = transportProxy
	}
	botApi, err := tgbotapi.NewBotAPIWithClient(c.TelegramBot.ApiToken, tgbotapi.APIEndpoint, tgHttpClient)
	if err != nil {
		logger.Fatalf("创建电报机器人失败, %v", err)
	}
	botApi.Debug = c.TelegramBot.Debug

	botUserInfo, err := botApi.GetMe()
	if err != nil {
		logger.Fatalf("获取电报机器人信息失败, %v", err)
	}

	// 创建SolanaRpc客户端
	rpcHttpClient := new(http.Client)
	if transportProxy != nil {
		rpcHttpClient.Transport = transportProxy
	}
	opts := &jsonrpc.RPCClientOpts{HTTPClient: rpcHttpClient}
	solanaRpc := rpc.NewWithCustomRPCClient(jsonrpc.NewClientWithOpts(c.Solana.RpcUrl, opts))

	svcCtx := &ServiceContext{
		Config:           c,
		HashEncoder:      hashEncoder,
		Engine:           strategyEngine,
		DbClient:         client,
		BotApi:           botApi,
		BotUserInfo:      &botUserInfo,
		SolanaRpc:        solanaRpc,
		OkxClient:        okxweb3.NewClient(c.Sock5Proxy),
		GmgnClient:       gmgn.NewClient(c.Sock5Proxy, c.ZenRows),
		JupagClient:      jupag.NewClient(c.Sock5Proxy),
		TransportProxy:   transportProxy,
		LookuptableCache: cache.NewLookuptableCache(solanaRpc),
		MessageCache:     cache.NewMessageCache(),
		TokenMetaCache:   cache.NewTokenMetaCache(solanaRpc),
		GridModel:        model.NewGridModel(client.Grid),
		OrderModel:       model.NewOrderModel(client.Order),
		SettingsModel:    model.NewSettingsModel(client.Settings),
		StrategyModel:    model.NewStrategyModel(client.Strategy),
		WalletModel:      model.NewWalletModel(client.Wallet),
	}
	return svcCtx
}

func (svcCtx *ServiceContext) Close() {
	if err := svcCtx.DbClient.Close(); err != nil {
		logger.Errorf("关闭数据库失败, %v", err)
	}

	if err := svcCtx.SolanaRpc.Close(); err != nil {
		logger.Errorf("关闭Solana RPC失败, %v", err)
	}
}
