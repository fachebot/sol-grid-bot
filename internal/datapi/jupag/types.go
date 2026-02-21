package jupag

// Jupiter 数据类型定义

import (
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"
)

// Trade 交易记录结构体
type Trade struct {
	first         bool            // 是否为首次接收
	Timestamp     time.Time       `json:"timestamp"`     // 时间戳
	Asset         string          `json:"asset"`         // 代币地址
	Type          string          `json:"type"`          // 交易类型 (buy/sell)
	UsdPrice      decimal.Decimal `json:"usdPrice"`      // USD价格
	UsdVolume     decimal.Decimal `json:"usdVolume"`     // USD交易量
	NativeVolume  decimal.Decimal `json:"nativeVolume"`  // 代币交易量
	TraderAddress string          `json:"traderAddress"` // 交易者地址
	TxHash        string          `json:"txHash"`        // 交易哈希
	Amount        decimal.Decimal `json:"amount"`        // 交易数量
	IsMev         bool            `json:"isMev"`         // 是否为MEV交易
	IsValidPrice  bool            `json:"isValidPrice"`  // 价格是否有效
	PoolId        string          `json:"poolId"`        // 池子ID
}

// Message WebSocket消息结构体
type Message struct {
	Type string          `json:"type"` // 消息类型
	Data json.RawMessage `json:"data"` // 消息数据
}

// tokenCandle K线数据结构体
type tokenCandle struct {
	Time   int64           `json:"time"`   // 时间戳
	Open   decimal.Decimal `json:"open"`   // 开盘价
	High   decimal.Decimal `json:"high"`   // 最高价
	Low    decimal.Decimal `json:"low"`    // 最低价
	Close  decimal.Decimal `json:"close"`  // 收盘价
	Volume decimal.Decimal `json:"volume"` // 成交量
}

// tokenCandlesResponse K线数据响应结构体
type tokenCandlesResponse struct {
	Candles []tokenCandle `json:"candles,omitempty"` // K线数组
}
