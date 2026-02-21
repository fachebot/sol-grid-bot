package jupag

// Jupiter 交易数据订阅者
// 通过WebSocket实时接收代币交易数据

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fachebot/sol-grid-bot/internal/config"
	"github.com/fachebot/sol-grid-bot/internal/logger"

	"github.com/gorilla/websocket"
	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
)

const (
	reconnectInitial = 1 * time.Second  // 初始重连间隔
	reconnectMax     = 30 * time.Second // 最大重连间隔
)

// JupagSubscriber Jupiter WebSocket订阅者结构体
type JupagSubscriber struct {
	ctx            context.Context     // 上下文
	cancel         context.CancelFunc  // 取消函数
	stopChan       chan struct{}       // 停止信号通道
	url            string              // WebSocket URL
	conn           *websocket.Conn     // WebSocket连接
	proxy          config.Sock5Proxy   // SOCK5代理配置
	reconnect      chan struct{}       // 重连信号通道
	tradeChan      chan []Trade        // 交易数据通道
	mutex          sync.Mutex          // 互斥锁
	assets         map[string]struct{} // 订阅的代币列表
	messageCounter map[string]int      // 消息计数器
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

// NewJupagSubscriber 创建新的Jupiter订阅者
// proxy: SOCK5代理配置
func NewJupagSubscriber(proxy config.Sock5Proxy) *JupagSubscriber {
	ctx, cancel := context.WithCancel(context.Background())
	subscriber := &JupagSubscriber{
		ctx:            ctx,
		cancel:         cancel,
		url:            "wss://trench-stream.jup.ag/ws",
		proxy:          proxy,
		reconnect:      make(chan struct{}, 1),
		assets:         make(map[string]struct{}),
		messageCounter: make(map[string]int),
	}
	return subscriber
}

// Stop 停止订阅者服务
func (subscriber *JupagSubscriber) Stop() {
	logger.Infof("[JupagSubscriber] 准备停止服务")

	subscriber.cancel()

	if subscriber.conn != nil {
		subscriber.conn.Close()
	}

	<-subscriber.stopChan

	close(subscriber.stopChan)
	subscriber.stopChan = nil

	if subscriber.tradeChan != nil {
		close(subscriber.tradeChan)
		subscriber.tradeChan = nil
	}

	logger.Infof("[JupagSubscriber] 服务已经停止")
}

// Start 启动订阅者服务
func (subscriber *JupagSubscriber) Start() {
	if subscriber.stopChan != nil {
		return
	}

	subscriber.stopChan = make(chan struct{})

	if subscriber.conn == nil {
		logger.Infof("[JupagSubscriber] 开始运行服务")
		go subscriber.run()
	}
}

// WaitUntilConnected 等待连接建立
func (subscriber *JupagSubscriber) WaitUntilConnected() {
	for subscriber.conn == nil {
		time.Sleep(time.Second * 1)
	}
}

// GetTradeChan 获取交易数据通道
func (subscriber *JupagSubscriber) GetTradeChan() <-chan []Trade {
	if subscriber.tradeChan == nil {
		subscriber.tradeChan = make(chan []Trade, 1024)
	}
	return subscriber.tradeChan
}

// SubscribeTrades 订阅代币交易数据
// assets: 代币地址列表
func (subscriber *JupagSubscriber) SubscribeTrades(assets []string) error {
	allAssets := make([]string, 0)
	subscriber.mutex.Lock()
	for _, asset := range assets {
		subscriber.assets[asset] = struct{}{}
	}
	for asset := range subscriber.assets {
		allAssets = append(allAssets, asset)
	}
	subscriber.mutex.Unlock()

	if len(assets) == 0 {
		return nil
	}
	if subscriber.conn == nil {
		return fmt.Errorf("[JupagSubscriber] 连接未建立")
	}

	logger.Debugf("[JupagSubscriber] 订阅Trades, assets: %+v", assets)

	payload := map[string]any{
		"type":   "subscribe:txns",
		"assets": allAssets,
	}
	err := subscriber.conn.WriteJSON(payload)

	return err
}

// UnsubscribeTrades 取消订阅代币交易数据
// assets: 代币地址列表
func (subscriber *JupagSubscriber) UnsubscribeTrades(assets []string) error {
	if len(assets) == 0 {
		return nil
	}
	if subscriber.conn == nil {
		return fmt.Errorf("[JupagSubscriber] 连接未建立")
	}

	logger.Debugf("[JupagSubscriber] 取消订阅Trades, assets: %+v", assets)

	payload := map[string]any{
		"type":   "unsubscribe:txns",
		"assets": assets,
	}
	err := subscriber.conn.WriteJSON(payload)

	if err == nil {
		subscriber.mutex.Lock()
		for _, asset := range assets {
			delete(subscriber.assets, asset)
		}
		subscriber.mutex.Unlock()
	}

	return err
}

// run 主运行循环，处理重连逻辑
func (subscriber *JupagSubscriber) run() {
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
				logger.Infof("[JupagSubscriber] 重新建立连接...")
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
func (subscriber *JupagSubscriber) connect() {
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

	headers := make(http.Header)
	headers.Set("origin", "https://jup.ag")
	headers.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")
	headers.Set("accept-language", "zh-CN,zh;q=0.9")
	headers.Set("cache-control", "no-cache")
	headers.Set("pragma", "no-cache")
	headers.Set("accept-encoding", "gzip, deflate, br, zstd")

	conn, _, err := dialer.Dial(subscriber.url, headers)
	if err != nil {
		logger.Errorf("[JupagSubscriber] 连接失败, %v", err)
		subscriber.scheduleReconnect()
		return
	}

	subscriber.conn = conn
	subscriber.messageCounter = make(map[string]int)
	logger.Infof("[JupagSubscriber] 连接已建立")

	assets := make([]string, 0)
	subscriber.mutex.Lock()
	for asset := range subscriber.assets {
		assets = append(assets, asset)
	}
	subscriber.mutex.Unlock()
	subscriber.SubscribeTrades(assets)

	go subscriber.readMessages()
}

// heartbeat 发送心跳包保持连接
func (subscriber *JupagSubscriber) heartbeat(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			msg := `{"method":"ping"}`
			if err := subscriber.conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				logger.Errorf("[JupagSubscriber] 发送心跳消息失败, %v", err)
				return
			}

			duration := time.Second * 20
			timer.Reset(duration)
		case <-ctx.Done():
			return
		}
	}
}

// readMessages 读取并处理WebSocket消息
func (subscriber *JupagSubscriber) readMessages() {
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
			logger.Errorf("[JupagSubscriber] 读取出错, %v", err)
			subscriber.scheduleReconnect()
			return
		}

		logger.Debugf("[JupagSubscriber] 收到新消息, %s", message)

		var payload Message
		if err = json.Unmarshal(message, &payload); err != nil {
			logger.Errorf("[JupagSubscriber] 解析消息失败, message: %s, %v", message, err)
			continue
		}

		switch payload.Type {
		case "actions":
			if subscriber.tradeChan != nil {
				var trades []Trade
				if err = json.Unmarshal(payload.Data, &trades); err != nil {
					logger.Errorf("[JupagSubscriber] 解析Trades失败, message: %s, %v", message, err)
					continue
				}

				for idx, trade := range trades {
					count, ok := subscriber.messageCounter[trade.Asset]
					if !ok {
						count = 0
					}

					trades[idx].first = count == 0
					subscriber.messageCounter[trade.Asset] = count + 1
				}

				select {
				case subscriber.tradeChan <- trades:
					logger.Debugf("[JupagSubscriber] 分发 Trades 数据, trades: %+v", len(trades))
				default:
					logger.Warnf("[JupagSubscriber] 分发 Trades 数据, channel 已满. trades: %+v", len(trades))
				}
			}
		}
	}
}

// scheduleReconnect 安排重连
func (subscriber *JupagSubscriber) scheduleReconnect() {
	if subscriber.ctx.Err() == nil {
		select {
		case subscriber.reconnect <- struct{}{}:
		default:
		}
	}
}
