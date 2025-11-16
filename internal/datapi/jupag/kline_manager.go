package jupag

import (
	"context"
	"time"

	"github.com/fachebot/sol-grid-bot/internal/charts"
	"github.com/fachebot/sol-grid-bot/internal/logger"

	"github.com/shopspring/decimal"
)

const (
	maxCandelsLimit = 1000
)

type KlineManager struct {
	ctx      context.Context
	cancel   context.CancelFunc
	stopChan chan struct{}

	client             *Client
	subscriber         *JupagSubscriber
	candles            int
	resolution         string
	resolutionDuration time.Duration
	tokenOhlcsMap      map[string][]charts.Ohlc

	tokenOhlcsChan chan charts.TokenOhlcs
}

func NewKlineManager(client *Client, subscriber *JupagSubscriber, resolution string, candles int) *KlineManager {
	if candles > maxCandelsLimit {
		candles = maxCandelsLimit
	}

	ctx, cancel := context.WithCancel(context.Background())

	resolutionDuration, err := charts.ResolutionToDuration(resolution)
	if err != nil {
		logger.Fatalf("[KlineManager] 无效的 resolution 配置, value: %v, %v", resolution, err)
	}

	return &KlineManager{
		ctx:                ctx,
		cancel:             cancel,
		client:             client,
		subscriber:         subscriber,
		candles:            candles,
		resolution:         resolution,
		resolutionDuration: resolutionDuration,
		tokenOhlcsMap:      make(map[string][]charts.Ohlc),
	}
}

func (m *KlineManager) Stop() {
	if m.stopChan == nil {
		return
	}

	logger.Infof("[KlineManager] 准备停止服务")

	m.cancel()

	<-m.stopChan

	close(m.stopChan)
	m.stopChan = nil

	if m.tokenOhlcsChan != nil {
		close(m.tokenOhlcsChan)
		m.tokenOhlcsChan = nil
	}

	logger.Infof("[KlineManager] 服务已经停止")
}

func (m *KlineManager) Start() {
	if m.stopChan != nil {
		return
	}

	m.stopChan = make(chan struct{})
	logger.Infof("[KlineManager] 开始运行服务")
	go m.run()
}

func (m *KlineManager) Subscribe(assets []string) error {
	return m.subscriber.SubscribeTrades(assets)
}

func (m *KlineManager) Unsubscribe(assets []string) error {
	return m.subscriber.UnsubscribeTrades(assets)
}

func (m *KlineManager) GetOhlcsChan() <-chan charts.TokenOhlcs {
	if m.tokenOhlcsChan == nil {
		m.tokenOhlcsChan = make(chan charts.TokenOhlcs, 1024)
	}
	return m.tokenOhlcsChan
}

func (m *KlineManager) run() {
	tradeChan := m.subscriber.GetTradeChan()

	for {
		select {
		case <-m.ctx.Done():
			m.stopChan <- struct{}{}
			return
		case trades := <-tradeChan:
			tokenOhlcsMap := make(map[string][]charts.Ohlc)
			for _, trade := range trades {
				ohlcs, ok := m.updateOhlcs(trade)
				if !ok {
					continue
				}
				tokenOhlcsMap[trade.Asset] = ohlcs
			}

			for token, ohlcs := range tokenOhlcsMap {
				if m.tokenOhlcsChan != nil {
					select {
					case m.tokenOhlcsChan <- charts.TokenOhlcs{Token: token, Ohlcs: ohlcs}:
					default:
						logger.Warnf("[KlineManager] 分发 Ohlcs 数据, channel 已满. token: %+v", token)
					}
				}
			}
		}
	}
}

func (m *KlineManager) trimOhlcs(ohlcs []charts.Ohlc) []charts.Ohlc {
	if len(ohlcs) <= maxCandelsLimit {
		return ohlcs
	}

	copy(ohlcs, ohlcs[len(ohlcs)-maxCandelsLimit:])
	ohlcs = ohlcs[:maxCandelsLimit]

	return ohlcs
}

func (m *KlineManager) updateOhlcs(data Trade) ([]charts.Ohlc, bool) {
	ohlcs := m.tokenOhlcsMap[data.Asset]

	// 初始化或重置切片
	if data.first {
		ohlcs = ohlcs[:0]
	}

	// 数据加载逻辑封装
	loadData := func() bool {
		newOhlcs, err := m.client.FetchTokenCandles(m.ctx, data.Asset, data.Timestamp, m.resolution, m.candles)
		if err != nil {
			logger.Errorf("[KlineManager] 获取数据失败: %s", err)
			return false
		}
		ohlcs = m.trimOhlcs(newOhlcs)
		m.tokenOhlcsMap[data.Asset] = ohlcs
		return true
	}

	// 首次加载
	if len(ohlcs) == 0 {
		logger.Infof("[KlineManager] 首次获取K线数据, token: %s", data.Asset)
		if loadData() {
			return ohlcs, true
		}
		return nil, false
	}

	// 填充数据
	last := ohlcs[len(ohlcs)-1]
	n := int(data.Timestamp.Sub(last.Time) / m.resolutionDuration)
	for range n {
		last.Open = last.Close
		last.High = last.Close
		last.Low = last.Close
		last.Volume = decimal.Zero
		last.Time = last.Time.Add(m.resolutionDuration)
		ohlcs = append(ohlcs, last)
	}

	// 增量更新
	update := &ohlcs[len(ohlcs)-1]
	update.Close = data.UsdPrice
	if update.Close.LessThan(update.Low) {
		update.Low = update.Close
	}
	if update.Close.GreaterThan(update.High) {
		update.High = update.Close
	}
	update.Volume = update.Volume.Add(data.UsdVolume)

	// 更新缓存数据
	ohlcs = m.trimOhlcs(ohlcs)
	m.tokenOhlcsMap[data.Asset] = ohlcs

	return ohlcs, true
}
