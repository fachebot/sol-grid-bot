package okxweb3

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/fachebot/sol-grid-bot/internal/charts"
	"github.com/shopspring/decimal"
)

func TestParseOkxResponse(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		wantCode   string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:     "成功响应",
			response: `{"code": "0", "msg": "success", "data": []}`,
			wantCode: "0",
			wantErr:  false,
		},
		{
			name:       "错误响应 - 非零code",
			response:   `{"code": "1001", "msg": "invalid parameters", "data": null}`,
			wantCode:   "1001",
			wantErr:    true,
			wantErrMsg: "okx api error - code: 1001, msg: invalid parameters",
		},
		{
			name:     "无效JSON",
			response: `{"code": "0", "msg": "success"`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			result, err := client.parseOkxResponse(tt.response)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if tt.wantErrMsg != "" && err.Error() != tt.wantErrMsg {
					t.Errorf("error message mismatch: got %s, want %s", err.Error(), tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result.Code != tt.wantCode {
				t.Errorf("code mismatch: got %s, want %s", result.Code, tt.wantCode)
			}
		})
	}
}

func TestOkxResponseDataParsing(t *testing.T) {
	t.Run("解析TokenCandles数据", func(t *testing.T) {
		response := okxResponse{
			Code: "0",
			Msg:  "success",
			Data: json.RawMessage(`[[1700000000000, "1.5", "1.6", "1.7", "1.4", "1000", "500", "2000"]]`),
		}

		var tokenCandles [][]decimal.Decimal
		err := json.Unmarshal(response.Data, &tokenCandles)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if len(tokenCandles) != 1 {
			t.Errorf("expected 1 item, got %d", len(tokenCandles))
		}

		if len(tokenCandles[0]) < 8 {
			t.Errorf("expected at least 8 fields, got %d", len(tokenCandles[0]))
		}
	})

	t.Run("解析空数组数据", func(t *testing.T) {
		response := okxResponse{
			Code: "0",
			Msg:  "success",
			Data: json.RawMessage(`[]`),
		}

		var tokenCandles [][]decimal.Decimal
		err := json.Unmarshal(response.Data, &tokenCandles)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if len(tokenCandles) != 0 {
			t.Errorf("expected 0 items, got %d", len(tokenCandles))
		}
	})

	t.Run("解析多条蜡烛数据", func(t *testing.T) {
		response := okxResponse{
			Code: "0",
			Msg:  "success",
			Data: json.RawMessage(`[
				[1700000000000, "1.0", "1.1", "1.2", "0.9", "500", "100", "1000"],
				[1700000060000, "1.1", "1.2", "1.3", "1.0", "600", "200", "1500"]
			]`),
		}

		var tokenCandles [][]decimal.Decimal
		err := json.Unmarshal(response.Data, &tokenCandles)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if len(tokenCandles) != 2 {
			t.Errorf("expected 2 items, got %d", len(tokenCandles))
		}

		for i, candle := range tokenCandles {
			if len(candle) < 8 {
				t.Errorf("item %d: expected at least 8 fields, got %d", i, len(candle))
			}
		}
	})
}

func TestOkxOhlcConversion(t *testing.T) {
	t.Run("转换OKX数据为OHLC", func(t *testing.T) {
		tokenCandles := [][]decimal.Decimal{
			{
				decimal.NewFromInt(1700000000000),
				decimal.NewFromFloat(1.5),
				decimal.NewFromFloat(1.6),
				decimal.NewFromFloat(1.7),
				decimal.NewFromFloat(1.4),
				decimal.NewFromFloat(1000),
				decimal.NewFromFloat(500),
				decimal.NewFromFloat(2000),
			},
		}

		result := make([]struct {
			Open   decimal.Decimal
			Close  decimal.Decimal
			High   decimal.Decimal
			Low    decimal.Decimal
			Volume decimal.Decimal
		}, 0, len(tokenCandles))

		for _, data := range tokenCandles {
			if len(data) < 8 {
				continue
			}

			result = append(result, struct {
				Open   decimal.Decimal
				Close  decimal.Decimal
				High   decimal.Decimal
				Low    decimal.Decimal
				Volume decimal.Decimal
			}{
				Open:   data[1],
				Close:  data[2],
				High:   data[3],
				Low:    data[4],
				Volume: data[6],
			})
		}

		if len(result) != 1 {
			t.Errorf("expected 1 item, got %d", len(result))
		}

		if !result[0].Open.Equal(decimal.NewFromFloat(1.5)) {
			t.Errorf("Open mismatch: got %v, want %v", result[0].Open, decimal.NewFromFloat(1.5))
		}
		if !result[0].Close.Equal(decimal.NewFromFloat(1.6)) {
			t.Errorf("Close mismatch: got %v, want %v", result[0].Close, decimal.NewFromFloat(1.6))
		}
		if !result[0].High.Equal(decimal.NewFromFloat(1.7)) {
			t.Errorf("High mismatch: got %v, want %v", result[0].High, decimal.NewFromFloat(1.7))
		}
		if !result[0].Low.Equal(decimal.NewFromFloat(1.4)) {
			t.Errorf("Low mismatch: got %v, want %v", result[0].Low, decimal.NewFromFloat(1.4))
		}
		if !result[0].Volume.Equal(decimal.NewFromFloat(500)) {
			t.Errorf("Volume mismatch: got %v, want %v", result[0].Volume, decimal.NewFromFloat(500))
		}
	})
}

func TestMessageGetChannel(t *testing.T) {
	tests := []struct {
		name     string
		msg      Message
		expected string
	}{
		{
			name: "有效channel",
			msg: Message{
				Arg: map[string]any{
					"channel": "trade",
				},
			},
			expected: "trade",
		},
		{
			name: "无效channel类型",
			msg: Message{
				Arg: map[string]any{
					"channel": 123,
				},
			},
			expected: "",
		},
		{
			name: "无channel",
			msg: Message{
				Arg: map[string]any{},
			},
			expected: "",
		},
		{
			name:     "nil arg",
			msg:      Message{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.msg.GetChannel()
			if result != tt.expected {
				t.Errorf("GetChannel() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMessageGetTokenAddress(t *testing.T) {
	tests := []struct {
		name     string
		msg      Message
		expected string
	}{
		{
			name: "有效tokenAddress",
			msg: Message{
				Arg: map[string]any{
					"tokenAddress": "So11111111111111111111111111111111111111112",
				},
			},
			expected: "So11111111111111111111111111111111111111112",
		},
		{
			name: "无效tokenAddress类型",
			msg: Message{
				Arg: map[string]any{
					"tokenAddress": []string{"abc"},
				},
			},
			expected: "",
		},
		{
			name: "无tokenAddress",
			msg: Message{
				Arg: map[string]any{},
			},
			expected: "",
		},
		{
			name:     "nil arg",
			msg:      Message{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.msg.GetTokenAddress()
			if result != tt.expected {
				t.Errorf("GetTokenAddress() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestOkxFetchTokenCandlesResolution(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		wantDur  time.Duration
		wantErr  bool
	}{
		{
			name:     "1分钟",
			interval: "1m",
			wantDur:  time.Minute,
			wantErr:  false,
		},
		{
			name:     "5分钟",
			interval: "5m",
			wantDur:  5 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "15分钟",
			interval: "15m",
			wantDur:  15 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "1小时",
			interval: "1h",
			wantDur:  time.Hour,
			wantErr:  false,
		},
		{
			name:     "4小时",
			interval: "4h",
			wantDur:  4 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "1天",
			interval: "1d",
			wantDur:  24 * time.Hour,
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
			result, err := charts.ResolutionToDuration(tt.interval)

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

			if result != tt.wantDur {
				t.Errorf("ResolutionToDuration() = %v, want %v", result, tt.wantDur)
			}
		})
	}
}

func TestOkxFetchTokenCandlesDataParsing(t *testing.T) {
	t.Run("解析完整的蜡烛数据响应", func(t *testing.T) {
		response := okxResponse{
			Code: "0",
			Msg:  "success",
			Data: json.RawMessage(`[
				[1700000000000, "1.0", "1.1", "1.2", "0.9", "500", "100", "1000"],
				[1700000060000, "1.1", "1.2", "1.3", "1.0", "600", "200", "1500"]
			]`),
		}

		var tokenCandles [][]decimal.Decimal
		err := json.Unmarshal(response.Data, &tokenCandles)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if len(tokenCandles) != 2 {
			t.Errorf("expected 2 candles, got %d", len(tokenCandles))
		}

		if len(tokenCandles[0]) < 8 {
			t.Errorf("expected at least 8 fields in first candle, got %d", len(tokenCandles[0]))
		}

		if !tokenCandles[0][1].Equal(decimal.NewFromFloat(1.0)) {
			t.Errorf("first Open mismatch")
		}
		if !tokenCandles[1][2].Equal(decimal.NewFromFloat(1.2)) {
			t.Errorf("second Close mismatch")
		}
	})

	t.Run("解析空蜡烛数据", func(t *testing.T) {
		response := okxResponse{
			Code: "0",
			Msg:  "success",
			Data: json.RawMessage(`[]`),
		}

		var tokenCandles [][]decimal.Decimal
		err := json.Unmarshal(response.Data, &tokenCandles)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if len(tokenCandles) != 0 {
			t.Errorf("expected 0 candles, got %d", len(tokenCandles))
		}
	})

	t.Run("解析字段不足的蜡烛数据", func(t *testing.T) {
		response := okxResponse{
			Code: "0",
			Msg:  "success",
			Data: json.RawMessage(`[
				["1700000000000"]
			]`),
		}

		var tokenCandles [][]decimal.Decimal
		err := json.Unmarshal(response.Data, &tokenCandles)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if len(tokenCandles) != 1 {
			t.Errorf("expected 1 candle, got %d", len(tokenCandles))
		}

		if len(tokenCandles[0]) != 1 {
			t.Errorf("expected 1 field, got %d", len(tokenCandles[0]))
		}
	})

	t.Run("解析带错误码的响应", func(t *testing.T) {
		client := &Client{}
		_, err := client.parseOkxResponse(`{"code": "1001", "msg": "invalid token", "data": null}`)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
	})
}

func TestOkxFetchTokenCandlesOhlcConversion(t *testing.T) {
	t.Run("转换OKX蜡烛数据到OHLC", func(t *testing.T) {
		tokenCandles := [][]decimal.Decimal{
			{
				decimal.NewFromInt(1700000000000),
				decimal.NewFromFloat(1.0),
				decimal.NewFromFloat(1.1),
				decimal.NewFromFloat(1.2),
				decimal.NewFromFloat(0.9),
				decimal.NewFromFloat(500),
				decimal.NewFromFloat(100),
				decimal.NewFromFloat(1000),
			},
			{
				decimal.NewFromInt(1700000060000),
				decimal.NewFromFloat(1.1),
				decimal.NewFromFloat(1.2),
				decimal.NewFromFloat(1.3),
				decimal.NewFromFloat(1.0),
				decimal.NewFromFloat(600),
				decimal.NewFromFloat(200),
				decimal.NewFromFloat(1500),
			},
		}

		result := make([]struct {
			Open   decimal.Decimal
			Close  decimal.Decimal
			High   decimal.Decimal
			Low    decimal.Decimal
			Volume decimal.Decimal
		}, 0, len(tokenCandles))

		for _, data := range tokenCandles {
			if len(data) < 8 {
				continue
			}

			result = append(result, struct {
				Open   decimal.Decimal
				Close  decimal.Decimal
				High   decimal.Decimal
				Low    decimal.Decimal
				Volume decimal.Decimal
			}{
				Open:   data[1],
				Close:  data[2],
				High:   data[3],
				Low:    data[4],
				Volume: data[6],
			})
		}

		if len(result) != 2 {
			t.Fatalf("expected 2 ohlc, got %d", len(result))
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
		if !result[0].Volume.Equal(decimal.NewFromFloat(100)) {
			t.Errorf("Volume mismatch: got %v, want %v", result[0].Volume, decimal.NewFromFloat(100))
		}
	})

	t.Run("空数据转换", func(t *testing.T) {
		tokenCandles := [][]decimal.Decimal{}

		result := make([]struct {
			Open   decimal.Decimal
			Close  decimal.Decimal
			High   decimal.Decimal
			Low    decimal.Decimal
			Volume decimal.Decimal
		}, 0, len(tokenCandles))

		if len(result) != 0 {
			t.Errorf("expected 0 ohlc, got %d", len(result))
		}
	})

	t.Run("数据字段不足时跳过", func(t *testing.T) {
		tokenCandles := [][]decimal.Decimal{
			{
				decimal.NewFromInt(1700000000000),
				decimal.NewFromFloat(1.0),
			},
		}

		result := make([]struct {
			Open   decimal.Decimal
			Close  decimal.Decimal
			High   decimal.Decimal
			Low    decimal.Decimal
			Volume decimal.Decimal
		}, 0, len(tokenCandles))

		for _, data := range tokenCandles {
			if len(data) < 8 {
				continue
			}

			result = append(result, struct {
				Open   decimal.Decimal
				Close  decimal.Decimal
				High   decimal.Decimal
				Low    decimal.Decimal
				Volume decimal.Decimal
			}{
				Open:   data[1],
				Close:  data[2],
				High:   data[3],
				Low:    data[4],
				Volume: data[6],
			})
		}

		if len(result) != 0 {
			t.Errorf("expected 0 ohlc (skipped due to insufficient fields), got %d", len(result))
		}
	})

	t.Run("高精度时间戳转换", func(t *testing.T) {
		tokenCandles := [][]decimal.Decimal{
			{
				decimal.NewFromFloat(1700000000123.5),
				decimal.NewFromFloat(1.0),
				decimal.NewFromFloat(1.1),
				decimal.NewFromFloat(1.1),
				decimal.NewFromFloat(1.0),
				decimal.NewFromFloat(100),
				decimal.NewFromFloat(50),
				decimal.NewFromFloat(200),
			},
		}

		result := make([]struct {
			Open   decimal.Decimal
			Close  decimal.Decimal
			High   decimal.Decimal
			Low    decimal.Decimal
			Volume decimal.Decimal
		}, 0, len(tokenCandles))

		for _, data := range tokenCandles {
			if len(data) < 8 {
				continue
			}

			result = append(result, struct {
				Open   decimal.Decimal
				Close  decimal.Decimal
				High   decimal.Decimal
				Low    decimal.Decimal
				Volume decimal.Decimal
			}{
				Open:   data[1],
				Close:  data[2],
				High:   data[3],
				Low:    data[4],
				Volume: data[6],
			})
		}

		if len(result) != 1 {
			t.Fatalf("expected 1 ohlc, got %d", len(result))
		}
	})
}
