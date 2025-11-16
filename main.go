package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/fachebot/sol-grid-bot/internal/config"
	"github.com/fachebot/sol-grid-bot/internal/datapi/gmgn"
	"github.com/fachebot/sol-grid-bot/internal/datapi/jupag"
	"github.com/fachebot/sol-grid-bot/internal/datapi/okxweb3"
	"github.com/fachebot/sol-grid-bot/internal/engine"
	"github.com/fachebot/sol-grid-bot/internal/job"
	"github.com/fachebot/sol-grid-bot/internal/logger"
	"github.com/fachebot/sol-grid-bot/internal/strategy"
	"github.com/fachebot/sol-grid-bot/internal/svc"
	"github.com/fachebot/sol-grid-bot/internal/telebot"
)

var configFile = flag.String("f", "etc/config.yaml", "the config file")

func startAllStrategy(svcCtx *svc.ServiceContext, strategyEngine *engine.StrategyEngine) {
	offset := 0
	const limit = 100

	for {
		data, err := svcCtx.StrategyModel.FindAllActive(context.TODO(), offset, limit)
		if err != nil {
			logger.Fatalf("[startAllStrategy] 加载活跃的策略列表失败, %v", err)
		}

		if len(data) == 0 {
			break
		}

		strategyList := make([]engine.Strategy, 0)
		for _, item := range data {
			s := strategy.NewGridStrategy(svcCtx, item)
			strategyList = append(strategyList, s)
		}

		err = strategyEngine.StartStrategy(strategyList)
		if err != nil {
			logger.Fatalf("[startAllStrategy] 开始策略失败, %v", err)
		}

		offset = offset + len(data)
	}
}

func main() {
	flag.Parse()

	// 读取配置文件
	c, err := config.LoadFromFile(*configFile)
	if err != nil {
		logger.Fatalf("读取配置文件失败, %s", err)
	}

	// 创建数据目录
	if _, err := os.Stat("data"); os.IsNotExist(err) {
		err := os.Mkdir("data", 0755)
		if err != nil {
			logger.Fatalf("创建数据目录失败, %s", err)
		}
	}

	// 运行K线管理器
	const candles = 329
	const resolution = "1m"
	var quotationSubscriber job.Job
	var klineManager engine.KlineManager
	switch c.Datapi {
	case "okx":
		subscriber := okxweb3.NewOkxSubscriber(resolution, c.Sock5Proxy)
		subscriber.Start()
		subscriber.WaitUntilConnected()

		jupagClient := okxweb3.NewClient(c.Sock5Proxy)
		klineManager = okxweb3.NewKlineManager(jupagClient, subscriber, candles)
		klineManager.Start()

		quotationSubscriber = subscriber
	case "jupag":
		subscriber := jupag.NewJupagSubscriber(c.Sock5Proxy)
		subscriber.Start()
		subscriber.WaitUntilConnected()

		jupagClient := jupag.NewClient(c.Sock5Proxy)
		klineManager = jupag.NewKlineManager(jupagClient, subscriber, resolution, candles)
		klineManager.Start()

		quotationSubscriber = subscriber
	default:
		subscriber, err := gmgn.NewQuotationSubscriber(resolution, nil, c.Sock5Proxy)
		if err != nil {
			logger.Fatalf("创建报价订阅器失败, %s", err)
		}
		subscriber.Start()
		subscriber.WaitUntilConnected()

		gmgnClient := gmgn.NewClient(c.Sock5Proxy, c.ZenRows)
		klineManager = gmgn.NewKlineManager(gmgnClient, subscriber, candles)
		klineManager.Start()

		quotationSubscriber = subscriber
	}

	// 运行策略引擎
	strategyEngine := engine.NewStrategyEngine(klineManager)
	strategyEngine.Start()

	// 创建服务上下文
	svcCtx := svc.NewServiceContext(c, strategyEngine)

	// 运行订单Keeper
	orderKeeper := job.NewOrderKeeper(svcCtx)
	orderKeeper.Start()

	// 运行机器人服务
	botService, err := telebot.NewTeleBot(svcCtx)
	if err != nil {
		logger.Fatalf("创建机器人服务失败, %s", err)
	}
	botService.Start()

	// 开始所有策略
	startAllStrategy(svcCtx, strategyEngine)

	// 等待程序退出
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch

	botService.Stop()
	strategyEngine.Stop()
	klineManager.Stop()
	quotationSubscriber.Stop()
	orderKeeper.Stop()
	// loreMonitor.Stop()

	svcCtx.Close()
	logger.Infof("服务已停止")
}
