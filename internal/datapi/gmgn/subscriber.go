package gmgn

// GMGN 行情数据订阅者
// 通过WebSocket实时接收代币K线数据

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fachebot/sol-grid-bot/internal/charts"
	"github.com/fachebot/sol-grid-bot/internal/config"
	"github.com/fachebot/sol-grid-bot/internal/logger"
	"github.com/fachebot/sol-grid-bot/internal/utils"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	utls "github.com/refraction-networking/utls"
	"github.com/shopspring/decimal"
	"golang.org/x/net/proxy"
)

const (
	reconnectInitial = 1 * time.Second  // 初始重连间隔
	reconnectMax     = 30 * time.Second // 最大重连间隔
)

// Ticker K线数据 ticker
type Ticker struct {
	Token string      // 代币地址
	First bool        // 是否为首次接收
	Ohlc  charts.Ohlc // K线数据
}

// channelMessage WebSocket通道消息结构体
type channelMessage struct {
	Channel string          `json:"channel"` // 通道名称
	Data    json.RawMessage `json:"data"`    // 消息数据
}

// klineChannelData K线通道数据
type klineChannelData struct {
	N string          `json:"n"` // 代币名称
	A string          `json:"a"` // 代币地址
	I string          `json:"i"` // 周期
	O decimal.Decimal `json:"o"` // 开盘价
	H decimal.Decimal `json:"h"` // 最高价
	L decimal.Decimal `json:"l"` // 最低价
	C decimal.Decimal `json:"c"` // 收盘价
	V decimal.Decimal `json:"v"` // 成交量
	T int64           `json:"t"` // 时间戳
}

// QuotationSubscriber GMGN行情订阅者结构体
type QuotationSubscriber struct {
	ctx      context.Context    // 上下文
	cancel   context.CancelFunc // 取消函数
	stopChan chan struct{}      // 停止信号通道

	conn           *websocket.Conn   // WebSocket连接
	url            string            // WebSocket URL
	resolution     string            // K线周期
	tokenAddresses sync.Map          // 订阅的代币地址集合
	proxy          config.Sock5Proxy // SOCK5代理配置
	reconnect      chan struct{}     // 重连信号通道

	tickerChan     chan Ticker    // K线数据通道
	messageCounter map[string]int // 消息计数器
}

// NewQuotationSubscriber 创建新的GMGN行情订阅者
// resolution: K线周期 (如 "1m", "5m", "1h")
// tokenAddresses: 初始订阅的代币地址列表
// proxy: SOCK5代理配置
func NewQuotationSubscriber(
	resolution string,
	tokenAddresses []string,
	proxy config.Sock5Proxy,
) (*QuotationSubscriber, error) {
	ctx, cancel := context.WithCancel(context.Background())
	subscriber := &QuotationSubscriber{
		url:            "wss://ws.gmgn.ai/quotation",
		resolution:     resolution,
		proxy:          proxy,
		reconnect:      make(chan struct{}, 1),
		ctx:            ctx,
		cancel:         cancel,
		messageCounter: make(map[string]int),
	}

	for _, tokenAddress := range tokenAddresses {
		subscriber.tokenAddresses.Store(tokenAddress, true)
	}
	return subscriber, nil
}

// netDialTLSContext 创建自定义TLS连接的Dialer
// 支持SOCK5代理和TLS指纹模拟
func netDialTLSContext(ctx context.Context, network, addr string, sock5Proxy string) (net.Conn, error) {
	serverName := addr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		serverName = host
	}

	spec, err := utls.UTLSIdToSpec(RandomClientHelloID())
	if err != nil {
		return nil, err
	}
	for _, ext := range spec.Extensions {
		alpnExt, ok := ext.(*utls.ALPNExtension)
		if !ok {
			continue
		}

		alpnExt.AlpnProtocols = []string{"http/1.1"}
	}

	var conn net.Conn
	if sock5Proxy == "" {
		conn, err = new(net.Dialer).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
	} else {
		dialer, err := proxy.SOCKS5(network, sock5Proxy, nil, proxy.Direct)
		if err != nil {
			return nil, err
		}

		conn, err = dialer.Dial(network, addr)
		if err != nil {
			return nil, err
		}
	}

	config := &utls.Config{
		InsecureSkipVerify: true,
		ServerName:         serverName,
	}

	client := utls.UClient(conn, config, utls.HelloCustom)
	if err = client.ApplyPreset(&spec); err != nil {
		return nil, err
	}

	return client, nil
}

// Stop 停止订阅者服务
func (subscriber *QuotationSubscriber) Stop() {
	logger.Infof("[QuotationSubscriber] 准备停止服务")

	subscriber.cancel()

	if subscriber.conn != nil {
		subscriber.conn.Close()
	}

	<-subscriber.stopChan

	close(subscriber.stopChan)
	subscriber.stopChan = nil

	if subscriber.tickerChan != nil {
		close(subscriber.tickerChan)
		subscriber.tickerChan = nil
	}

	logger.Infof("[QuotationSubscriber] 服务已经停止")
}

// Start 启动订阅者服务
func (subscriber *QuotationSubscriber) Start() {
	if subscriber.stopChan != nil {
		return
	}

	subscriber.stopChan = make(chan struct{})

	if subscriber.conn == nil {
		logger.Infof("[QuotationSubscriber] 开始运行服务")
		go subscriber.run()
	}
}

// WaitUntilConnected 等待连接建立
func (subscriber *QuotationSubscriber) WaitUntilConnected() {
	for subscriber.conn == nil {
		time.Sleep(time.Second * 1)
	}
}

// GetTickerChan 获取K线数据通道
func (subscriber *QuotationSubscriber) GetTickerChan() <-chan Ticker {
	if subscriber.tickerChan == nil {
		subscriber.tickerChan = make(chan Ticker, 1024)
	}
	return subscriber.tickerChan
}

// Subscribe 订阅代币K线数据
// tokenAddresses: 代币地址列表
func (subscriber *QuotationSubscriber) Subscribe(tokenAddresses []string) error {
	for _, tokenAddress := range tokenAddresses {
		subscriber.tokenAddresses.Store(tokenAddress, true)
	}

	allTokenAddresses := make([]string, 0)
	subscriber.tokenAddresses.Range(func(k any, v any) bool {
		allTokenAddresses = append(allTokenAddresses, k.(string))
		return true
	})

	if subscriber.conn != nil {
		err := subscriber.sendSubscribe(allTokenAddresses)
		if err != nil {
			return err
		}
	}

	return nil
}

// Unsubscribe 取消订阅代币K线数据
// tokenAddresses: 代币地址列表
func (subscriber *QuotationSubscriber) Unsubscribe(tokenAddresses []string) error {
	for _, tokenAddress := range tokenAddresses {
		subscriber.tokenAddresses.Delete(tokenAddress)
	}

	allTokenAddresses := make([]string, 0)
	subscriber.tokenAddresses.Range(func(k any, v any) bool {
		allTokenAddresses = append(allTokenAddresses, k.(string))
		return true
	})

	if subscriber.conn != nil {
		err := subscriber.sendSubscribe(allTokenAddresses)
		if err != nil {
			return err
		}
	}

	return nil
}

// sendSubscribe 发送订阅请求到WebSocket服务器
func (subscriber *QuotationSubscriber) sendSubscribe(tokenAddresses []string) error {
	if subscriber.conn == nil {
		return fmt.Errorf("[QuotationSubscriber] 连接未建立")
	}

	if len(tokenAddresses) == 0 {
		return nil
	}

	logger.Debugf("[QuotationSubscriber] 订阅代币K线, %+v", tokenAddresses)

	data := make([]map[string]any, 0)
	for _, tokenAddress := range tokenAddresses {
		data = append(data, map[string]any{
			"chain":     "sol",
			"addresses": tokenAddress,
			"interval":  subscriber.resolution,
		})
	}

	payload := map[string]any{
		"action":  "subscribe",
		"id":      uuid.NewString(),
		"channel": "kline",
		"data":    data,
	}

	return subscriber.conn.WriteJSON(payload)
}

// run 主运行循环，处理重连逻辑
func (subscriber *QuotationSubscriber) run() {
	subscriber.connect()

	reconnectDelay := reconnectInitial
loop:
	for {
		select {
		case <-subscriber.ctx.Done():
			break loop
		case <-subscriber.reconnect:
			select {
			case <-subscriber.ctx.Done():
				break loop
			case <-time.After(reconnectDelay):
				logger.Infof("[QuotationSubscriber] 重新建立连接...")
				subscriber.connect()

				reconnectDelay *= 2
				if reconnectDelay > reconnectMax {
					reconnectDelay = reconnectMax
				}
			}
		}
	}

	subscriber.stopChan <- struct{}{}
}

// connect 建立WebSocket连接
func (subscriber *QuotationSubscriber) connect() {
	headers := make(http.Header)
	headers.Set("origin", "https://gmgn.ai")
	headers.Set("user-agent", utils.RandomUserAgent())
	headers.Set("accept-language", "zh-CN,zh;q=0.9")
	headers.Set("cache-control", "no-cache")
	headers.Set("pragma", "no-cache")
	headers.Set("accept-encoding", "gzip, deflate, br, zstd")

	proxy := ""
	if subscriber.proxy.Enable {
		proxy = fmt.Sprintf("%s:%d", subscriber.proxy.Host, subscriber.proxy.Port)
	}
	dialer := &websocket.Dialer{
		NetDial: func(network, addr string) (net.Conn, error) {
			return netDialTLSContext(subscriber.ctx, network, addr, proxy)
		},
		NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return netDialTLSContext(ctx, network, addr, proxy)
		},
		NetDialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return netDialTLSContext(ctx, network, addr, proxy)
		},
		HandshakeTimeout:  45 * time.Second,
		EnableCompression: true,
	}

	conn, _, err := dialer.DialContext(subscriber.ctx, subscriber.url, headers)
	if err != nil {
		logger.Errorf("[QuotationSubscriber] 连接失败, %v", err)
		subscriber.scheduleReconnect()
		return
	}

	subscriber.conn = conn
	subscriber.messageCounter = make(map[string]int)
	logger.Infof("[QuotationSubscriber] 连接已建立")

	tokenAddresses := make([]string, 0)
	subscriber.tokenAddresses.Range(func(k any, v any) bool {
		tokenAddresses = append(tokenAddresses, k.(string))
		return true
	})
	if len(tokenAddresses) > 0 {
		if err := subscriber.sendSubscribe(tokenAddresses); err != nil {
			logger.Errorf("[QuotationSubscriber] 订阅失败, %v", err)
			conn.Close()
			subscriber.scheduleReconnect()
			return
		}
		logger.Infof("[QuotationSubscriber] 订阅代币: %v", tokenAddresses)
	}

	go subscriber.readMessages()
}

// heartbeat 发送心跳包保持连接
func (subscriber *QuotationSubscriber) heartbeat(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			msg := fmt.Sprintf(`{"action":"heartbeat","client_ts":%d}`, time.Now().UnixMilli())
			if err := subscriber.conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				logger.Errorf("[QuotationSubscriber] 发送心跳消息失败, %v", err)
				return
			}

			duration := time.Second * 60
			timer.Reset(duration)
		case <-ctx.Done():
			return
		}
	}
}

// readMessages 读取并处理WebSocket消息
func (subscriber *QuotationSubscriber) readMessages() {
	defer subscriber.conn.Close()

	ctx, cancel := context.WithCancel(subscriber.ctx)
	defer cancel()
	go subscriber.heartbeat(ctx)

	for {
		_, message, err := subscriber.conn.ReadMessage()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			logger.Errorf("[QuotationSubscriber] 读取出错, %v", err)
			subscriber.scheduleReconnect()
			return
		}

		logger.Debugf("[QuotationSubscriber] 收到新消息, %s", message)

		var msg channelMessage
		if err = json.Unmarshal(message, &msg); err != nil {
			logger.Errorf("[QuotationSubscriber] 解析消息失败, message: %s, %v", message, err)
			continue
		}

		if msg.Channel == "kline" {
			if subscriber.tickerChan != nil {
				var klines []klineChannelData
				if err = json.Unmarshal(msg.Data, &klines); err != nil {
					logger.Errorf("[QuotationSubscriber] 解析 kline 失败, message: %s, %v", string(msg.Data), err)
					continue
				}

				for _, kline := range klines {
					count, ok := subscriber.messageCounter[kline.A]
					if !ok {
						count = 0
					}

					ticker := Ticker{
						Token: kline.A,
						First: count == 0,
						Ohlc: charts.Ohlc{
							Open:   kline.O,
							Close:  kline.C,
							High:   kline.H,
							Low:    kline.L,
							Time:   time.Unix(kline.T, 0),
							Volume: kline.V,
						},
					}

					subscriber.messageCounter[kline.A] = count + 1

					select {
					case subscriber.tickerChan <- ticker:
						logger.Debugf("[QuotationSubscriber] 分发 Ticker 数据, %+v", ticker)
					default:
						logger.Warnf("[QuotationSubscriber] 分发 Ticker 数据, channel 已满. %+v", ticker)
					}
				}
			}
		}
	}
}

// scheduleReconnect 安排重连
func (subscriber *QuotationSubscriber) scheduleReconnect() {
	if subscriber.ctx.Err() == nil {
		subscriber.conn = nil
		select {
		case subscriber.reconnect <- struct{}{}:
		default:
		}
	}
}
