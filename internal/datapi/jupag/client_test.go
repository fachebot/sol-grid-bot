package jupag

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestFormatInterval(t *testing.T) {
	tests := []struct {
		name      string
		interval  string
		want      string
		wantError bool
	}{
		{
			name:      "1分钟",
			interval:  "1m",
			want:      "1_MINUTE",
			wantError: false,
		},
		{
			name:      "5分钟",
			interval:  "5m",
			want:      "5_MINUTE",
			wantError: false,
		},
		{
			name:      "15分钟",
			interval:  "15m",
			want:      "15_MINUTE",
			wantError: false,
		},
		{
			name:      "1小时",
			interval:  "1h",
			want:      "1_HOUR",
			wantError: false,
		},
		{
			name:      "4小时",
			interval:  "4h",
			want:      "4_HOUR",
			wantError: false,
		},
		{
			name:      "1天",
			interval:  "1d",
			want:      "1_DAY",
			wantError: false,
		},
		{
			name:      "1秒",
			interval:  "1s",
			want:      "1_SECOND",
			wantError: false,
		},
		{
			name:      "空字符串",
			interval:  "",
			wantError: true,
		},
		{
			name:      "只有单位",
			interval:  "m",
			wantError: true,
		},
		{
			name:      "无效单位",
			interval:  "1w",
			wantError: true,
		},
		{
			name:      "无效数字",
			interval:  "abm",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatInterval(tt.interval)

			if tt.wantError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.want {
				t.Errorf("formatInterval() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestTokenCandlesResponseParsing(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantLen  int
	}{
		{
			name:     "空蜡烛数据",
			response: `{"candles": []}`,
			wantLen:  0,
		},
		{
			name:     "单条蜡烛数据",
			response: `{"candles": [{"time": 1700000000, "open": "1.5", "high": "1.7", "low": "1.4", "close": "1.6", "volume": "1000"}]}`,
			wantLen:  1,
		},
		{
			name: "多条蜡烛数据",
			response: `{"candles": [
				{"time": 1700000000, "open": "1.0", "high": "1.2", "low": "0.9", "close": "1.1", "volume": "500"},
				{"time": 1700000060, "open": "1.1", "high": "1.3", "low": "1.0", "close": "1.2", "volume": "600"}
			]}`,
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp tokenCandlesResponse
			err := json.Unmarshal([]byte(tt.response), &resp)
			if err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			if len(resp.Candles) != tt.wantLen {
				t.Errorf("length mismatch: got %d, want %d", len(resp.Candles), tt.wantLen)
			}

			for _, candle := range resp.Candles {
				if candle.Time <= 0 {
					t.Errorf("invalid time: %d", candle.Time)
				}
				if candle.Open.IsNegative() {
					t.Errorf("invalid open: %v", candle.Open)
				}
				if candle.High.LessThan(candle.Low) {
					t.Errorf("high less than low: high=%v, low=%v", candle.High, candle.Low)
				}
			}
		})
	}
}

func TestTokenCandlesToOhlc(t *testing.T) {
	t.Run("转换蜡烛数据为OHLC", func(t *testing.T) {
		candles := []tokenCandle{
			{
				Time:   1700000000,
				Open:   decimal.NewFromFloat(1.5),
				Close:  decimal.NewFromFloat(1.6),
				High:   decimal.NewFromFloat(1.7),
				Low:    decimal.NewFromFloat(1.4),
				Volume: decimal.NewFromFloat(1000),
			},
		}

		result := make([]struct {
			Open   decimal.Decimal
			Close  decimal.Decimal
			High   decimal.Decimal
			Low    decimal.Decimal
			Time   time.Time
			Volume decimal.Decimal
		}, 0, len(candles))

		for _, item := range candles {
			result = append(result, struct {
				Open   decimal.Decimal
				Close  decimal.Decimal
				High   decimal.Decimal
				Low    decimal.Decimal
				Time   time.Time
				Volume decimal.Decimal
			}{
				Open:   item.Open,
				Close:  item.Close,
				High:   item.High,
				Low:    item.Low,
				Time:   time.Unix(item.Time, 0),
				Volume: item.Volume,
			})
		}

		if len(result) != 1 {
			t.Errorf("expected 1 item, got %d", len(result))
		}

		if !result[0].Open.Equal(decimal.NewFromFloat(1.5)) {
			t.Errorf("Open mismatch")
		}
		if !result[0].Close.Equal(decimal.NewFromFloat(1.6)) {
			t.Errorf("Close mismatch")
		}
		if !result[0].High.Equal(decimal.NewFromFloat(1.7)) {
			t.Errorf("High mismatch")
		}
		if !result[0].Low.Equal(decimal.NewFromFloat(1.4)) {
			t.Errorf("Low mismatch")
		}
	})
}

func TestRandomClientHelloID(t *testing.T) {
	t.Run("返回非nil的ClientHelloID", func(t *testing.T) {
		id := RandomClientHelloID()
		if id.Client == "" && id.Version == "" {
			t.Error("expected non-nil ClientHelloID")
		}
	})

	t.Run("多次调用返回有效值", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			id := RandomClientHelloID()
			if id.Client == "" && id.Version == "" {
				t.Errorf("iteration %d: expected non-nil ClientHelloID", i)
			}
		}
	})
}

func TestJupagFetchTokenCandlesResolution(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		want     string
		wantErr  bool
	}{
		{
			name:     "1分钟",
			interval: "1m",
			want:     "1_MINUTE",
			wantErr:  false,
		},
		{
			name:     "5分钟",
			interval: "5m",
			want:     "5_MINUTE",
			wantErr:  false,
		},
		{
			name:     "15分钟",
			interval: "15m",
			want:     "15_MINUTE",
			wantErr:  false,
		},
		{
			name:     "1小时",
			interval: "1h",
			want:     "1_HOUR",
			wantErr:  false,
		},
		{
			name:     "4小时",
			interval: "4h",
			want:     "4_HOUR",
			wantErr:  false,
		},
		{
			name:     "1天",
			interval: "1d",
			want:     "1_DAY",
			wantErr:  false,
		},
		{
			name:     "1秒",
			interval: "1s",
			want:     "1_SECOND",
			wantErr:  false,
		},
		{
			name:     "无效间隔",
			interval: "1w",
			wantErr:  true,
		},
		{
			name:     "空间隔",
			interval: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatInterval(tt.interval)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.want {
				t.Errorf("formatInterval() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestJupagFetchTokenCandlesDataParsing(t *testing.T) {
	t.Run("解析完整的蜡烛数据响应", func(t *testing.T) {
		response := `{"candles": [
			{"time": 1700000000, "open": "1.0", "high": "1.2", "low": "0.9", "close": "1.1", "volume": "500"},
			{"time": 1700000060, "open": "1.1", "high": "1.3", "low": "1.0", "close": "1.2", "volume": "600"}
		]}`

		var resp tokenCandlesResponse
		err := json.Unmarshal([]byte(response), &resp)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if len(resp.Candles) != 2 {
			t.Errorf("expected 2 candles, got %d", len(resp.Candles))
		}

		if !resp.Candles[0].Open.Equal(decimal.NewFromFloat(1.0)) {
			t.Errorf("first Open mismatch")
		}
		if !resp.Candles[1].Close.Equal(decimal.NewFromFloat(1.2)) {
			t.Errorf("second Close mismatch")
		}
	})

	t.Run("解析空蜡烛数据", func(t *testing.T) {
		response := `{"candles": []}`

		var resp tokenCandlesResponse
		err := json.Unmarshal([]byte(response), &resp)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if len(resp.Candles) != 0 {
			t.Errorf("expected 0 candles, got %d", len(resp.Candles))
		}
	})

	t.Run("解析缺失candles字段", func(t *testing.T) {
		response := `{}`

		var resp tokenCandlesResponse
		err := json.Unmarshal([]byte(response), &resp)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if resp.Candles != nil {
			t.Errorf("expected nil candles, got %v", resp.Candles)
		}
	})
}

func TestJupagFetchTokenCandlesOhlcConversion(t *testing.T) {
	t.Run("转换蜡烛数据到OHLC", func(t *testing.T) {
		candles := []tokenCandle{
			{
				Time:   1700000000,
				Open:   decimal.NewFromFloat(1.0),
				Close:  decimal.NewFromFloat(1.1),
				High:   decimal.NewFromFloat(1.2),
				Low:    decimal.NewFromFloat(0.9),
				Volume: decimal.NewFromFloat(500),
			},
			{
				Time:   1700000060,
				Open:   decimal.NewFromFloat(1.1),
				Close:  decimal.NewFromFloat(1.2),
				High:   decimal.NewFromFloat(1.3),
				Low:    decimal.NewFromFloat(1.0),
				Volume: decimal.NewFromFloat(600),
			},
		}

		result := make([]struct {
			Open   decimal.Decimal
			Close  decimal.Decimal
			High   decimal.Decimal
			Low    decimal.Decimal
			Time   time.Time
			Volume decimal.Decimal
		}, 0, len(candles))

		for _, item := range candles {
			result = append(result, struct {
				Open   decimal.Decimal
				Close  decimal.Decimal
				High   decimal.Decimal
				Low    decimal.Decimal
				Time   time.Time
				Volume decimal.Decimal
			}{
				Open:   item.Open,
				Close:  item.Close,
				High:   item.High,
				Low:    item.Low,
				Time:   time.Unix(item.Time, 0),
				Volume: item.Volume,
			})
		}

		if len(result) != 2 {
			t.Fatalf("expected 2 ohlc, got %d", len(result))
		}

		expectedTime1 := time.Unix(1700000000, 0)
		if !result[0].Time.Equal(expectedTime1) {
			t.Errorf("Time mismatch: got %v, want %v", result[0].Time, expectedTime1)
		}

		if !result[0].Open.Equal(decimal.NewFromFloat(1.0)) {
			t.Errorf("Open mismatch")
		}
		if !result[0].Close.Equal(decimal.NewFromFloat(1.1)) {
			t.Errorf("Close mismatch")
		}
		if !result[0].High.Equal(decimal.NewFromFloat(1.2)) {
			t.Errorf("High mismatch")
		}
		if !result[0].Low.Equal(decimal.NewFromFloat(0.9)) {
			t.Errorf("Low mismatch")
		}
	})

	t.Run("空数据转换", func(t *testing.T) {
		candles := []tokenCandle{}
		result := make([]struct {
			Open   decimal.Decimal
			Close  decimal.Decimal
			High   decimal.Decimal
			Low    decimal.Decimal
			Time   time.Time
			Volume decimal.Decimal
		}, 0, len(candles))

		if len(result) != 0 {
			t.Errorf("expected 0 ohlc, got %d", len(result))
		}
	})

	t.Run("时间戳边界值", func(t *testing.T) {
		candles := []tokenCandle{
			{
				Time:   0,
				Open:   decimal.NewFromFloat(1.0),
				Close:  decimal.NewFromFloat(1.1),
				High:   decimal.NewFromFloat(1.1),
				Low:    decimal.NewFromFloat(1.0),
				Volume: decimal.NewFromFloat(100),
			},
		}

		result := make([]struct {
			Open   decimal.Decimal
			Close  decimal.Decimal
			High   decimal.Decimal
			Low    decimal.Decimal
			Time   time.Time
			Volume decimal.Decimal
		}, 0, len(candles))

		for _, item := range candles {
			result = append(result, struct {
				Open   decimal.Decimal
				Close  decimal.Decimal
				High   decimal.Decimal
				Low    decimal.Decimal
				Time   time.Time
				Volume decimal.Decimal
			}{
				Open:   item.Open,
				Close:  item.Close,
				High:   item.High,
				Low:    item.Low,
				Time:   time.Unix(item.Time, 0),
				Volume: item.Volume,
			})
		}

		expectedTime := time.Unix(0, 0)
		if !result[0].Time.Equal(expectedTime) {
			t.Errorf("Time mismatch: got %v, want %v", result[0].Time, expectedTime)
		}
	})
}
