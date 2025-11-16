package jupag

import (
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"
)

type Trade struct {
	first         bool
	Timestamp     time.Time       `json:"timestamp"`
	Asset         string          `json:"asset"`
	Type          string          `json:"type"`
	UsdPrice      decimal.Decimal `json:"usdPrice"`
	UsdVolume     decimal.Decimal `json:"usdVolume"`
	NativeVolume  decimal.Decimal `json:"nativeVolume"`
	TraderAddress string          `json:"traderAddress"`
	TxHash        string          `json:"txHash"`
	Amount        decimal.Decimal `json:"amount"`
	IsMev         bool            `json:"isMev"`
	IsValidPrice  bool            `json:"isValidPrice"`
	PoolId        string          `json:"poolId"`
}

type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type tokenCandle struct {
	Time   int64           `json:"time"`
	Open   decimal.Decimal `json:"open"`
	High   decimal.Decimal `json:"high"`
	Low    decimal.Decimal `json:"low"`
	Close  decimal.Decimal `json:"close"`
	Volume decimal.Decimal `json:"volume"`
}

type tokenCandlesResponse struct {
	Candles []tokenCandle `json:"candles,omitempty"`
}
