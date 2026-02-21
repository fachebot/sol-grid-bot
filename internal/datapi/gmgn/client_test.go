package gmgn

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/fachebot/sol-grid-bot/internal/charts"
	"github.com/shopspring/decimal"
)

func TestParseGmgnResponse(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		wantCode   int
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:     "成功响应",
			response: `{"code": 0, "msg": "success", "data": {}}`,
			wantCode: 0,
			wantErr:  false,
		},
		{
			name:       "错误响应 - 非零code",
			response:   `{"code": 1001, "msg": "invalid parameters", "data": null}`,
			wantCode:   1001,
			wantErr:    true,
			wantErrMsg: "gmgn api error - code: 1001, msg: invalid parameters",
		},
		{
			name:     "无效JSON",
			response: `{"code": 0, "msg": "success"`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			result, err := client.parseGmgnResponse(tt.response)

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
				t.Errorf("code mismatch: got %d, want %d", result.Code, tt.wantCode)
			}
		})
	}
}

func TestConvertToOhlcData(t *testing.T) {
	tests := []struct {
		name     string
		gmgnData []gmgnOhlc
		wantLen  int
	}{
		{
			name:     "空数据",
			gmgnData: []gmgnOhlc{},
			wantLen:  0,
		},
		{
			name: "单条数据",
			gmgnData: []gmgnOhlc{
				{
					Open:   decimal.NewFromFloat(1.5),
					Close:  decimal.NewFromFloat(1.6),
					High:   decimal.NewFromFloat(1.7),
					Low:    decimal.NewFromFloat(1.4),
					Time:   decimal.NewFromInt(1700000000000),
					Volume: decimal.NewFromFloat(1000),
				},
			},
			wantLen: 1,
		},
		{
			name: "多条数据",
			gmgnData: []gmgnOhlc{
				{
					Open:   decimal.NewFromFloat(1.0),
					Close:  decimal.NewFromFloat(1.1),
					High:   decimal.NewFromFloat(1.2),
					Low:    decimal.NewFromFloat(0.9),
					Time:   decimal.NewFromInt(1700000000000),
					Volume: decimal.NewFromFloat(500),
				},
				{
					Open:   decimal.NewFromFloat(1.1),
					Close:  decimal.NewFromFloat(1.2),
					High:   decimal.NewFromFloat(1.3),
					Low:    decimal.NewFromFloat(1.0),
					Time:   decimal.NewFromInt(1700000060000),
					Volume: decimal.NewFromFloat(600),
				},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			result := client.convertToOhlcData(tt.gmgnData)

			if len(result) != tt.wantLen {
				t.Errorf("length mismatch: got %d, want %d", len(result), tt.wantLen)
				return
			}

			for i, ohlc := range result {
				if i >= len(tt.gmgnData) {
					break
				}
				expected := tt.gmgnData[i]
				if !ohlc.Open.Equal(expected.Open) {
					t.Errorf("Open mismatch at index %d: got %v, want %v", i, ohlc.Open, expected.Open)
				}
				if !ohlc.Close.Equal(expected.Close) {
					t.Errorf("Close mismatch at index %d: got %v, want %v", i, ohlc.Close, expected.Close)
				}
				if !ohlc.High.Equal(expected.High) {
					t.Errorf("High mismatch at index %d: got %v, want %v", i, ohlc.High, expected.High)
				}
				if !ohlc.Low.Equal(expected.Low) {
					t.Errorf("Low mismatch at index %d: got %v, want %v", i, ohlc.Low, expected.Low)
				}
				expectedTime := time.Unix(expected.Time.IntPart()/1000, 0)
				if !ohlc.Time.Equal(expectedTime) {
					t.Errorf("Time mismatch at index %d: got %v, want %v", i, ohlc.Time, expectedTime)
				}
			}
		})
	}
}

func TestEncodeURLParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]string
		checkFn func(string) bool
	}{
		{
			name:   "空map",
			params: map[string]string{},
			checkFn: func(result string) bool {
				return result == ""
			},
		},
		{
			name: "单参数",
			params: map[string]string{
				"key1": "value1",
			},
			checkFn: func(result string) bool {
				return result == "key1=value1"
			},
		},
		{
			name: "多参数",
			params: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			checkFn: func(result string) bool {
				return result == "key1=value1&key2=value2" || result == "key2=value2&key1=value1"
			},
		},
		{
			name: "包含特殊字符",
			params: map[string]string{
				"device_id": "abc-123",
				"client_id": "gmgn_web_20250820",
			},
			checkFn: func(result string) bool {
				return result == "client_id=gmgn_web_20250820&device_id=abc-123" ||
					result == "device_id=abc-123&client_id=gmgn_web_20250820"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeURLParams(tt.params)
			if !tt.checkFn(result) {
				t.Errorf("encodeURLParams() = %v", result)
			}
		})
	}
}

func TestGmgnResponseDataParsing(t *testing.T) {
	t.Run("解析TokenCandles数据", func(t *testing.T) {
		response := gmgnResponse{
			Code: 0,
			Msg:  "success",
			Data: json.RawMessage(`{"list":[{"open":"1.5","close":"1.6","high":"1.7","low":"1.4","time":"1700000000000","volume":"1000"}]}`),
		}

		var tokenCandles gmgnTokenCandles
		err := json.Unmarshal(response.Data, &tokenCandles)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if len(tokenCandles.List) != 1 {
			t.Errorf("expected 1 item, got %d", len(tokenCandles.List))
		}

		if !tokenCandles.List[0].Open.Equal(decimal.NewFromFloat(1.5)) {
			t.Errorf("Open mismatch")
		}
	})

	t.Run("解析HolderInfo数据", func(t *testing.T) {
		response := gmgnResponse{
			Code: 0,
			Msg:  "success",
			Data: json.RawMessage(`{"list":[{"address":"abc123","amount_cur":"100.5","usd_value":"50.25"}]}`),
		}

		var holders HolderInfoList
		err := json.Unmarshal(response.Data, &holders)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if len(holders.List) != 1 {
			t.Errorf("expected 1 holder, got %d", len(holders.List))
		}

		if holders.List[0].Address != "abc123" {
			t.Errorf("address mismatch")
		}
	})

	t.Run("解析WalletHoldings数据", func(t *testing.T) {
		response := gmgnResponse{
			Code: 0,
			Msg:  "success",
			Data: json.RawMessage(`{"holdings":[{"token":{"address":"token123","symbol":"TEST"},"balance":"10.5","usd_value":"5.25"}]}`),
		}

		var holdings WalletHoldings
		err := json.Unmarshal(response.Data, &holdings)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if len(holdings.Holdings) != 1 {
			t.Errorf("expected 1 holding, got %d", len(holdings.Holdings))
		}

		if holdings.Holdings[0].Token.Symbol != "TEST" {
			t.Errorf("symbol mismatch")
		}
	})

	t.Run("解析TrendingTokens数据", func(t *testing.T) {
		response := gmgnResponse{
			Code: 0,
			Msg:  "success",
			Data: json.RawMessage(`{"rank":[{"id":1,"address":"token123","symbol":"TEST","price":"0.5","market_cap":"1000000"}]}`),
		}

		var trendingTokens TrendingTokens
		err := json.Unmarshal(response.Data, &trendingTokens)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if len(trendingTokens.Rank) != 1 {
			t.Errorf("expected 1 token, got %d", len(trendingTokens.Rank))
		}

		if trendingTokens.Rank[0].Symbol != "TEST" {
			t.Errorf("symbol mismatch")
		}
	})
}

func TestGmgnFetchTokenCandlesResolution(t *testing.T) {
	tests := []struct {
		name    string
		period  string
		wantDur time.Duration
		wantErr bool
	}{
		{
			name:    "1分钟",
			period:  "1m",
			wantDur: time.Minute,
			wantErr: false,
		},
		{
			name:    "5分钟",
			period:  "5m",
			wantDur: 5 * time.Minute,
			wantErr: false,
		},
		{
			name:    "15分钟",
			period:  "15m",
			wantDur: 15 * time.Minute,
			wantErr: false,
		},
		{
			name:    "1小时",
			period:  "1h",
			wantDur: time.Hour,
			wantErr: false,
		},
		{
			name:    "4小时",
			period:  "4h",
			wantDur: 4 * time.Hour,
			wantErr: false,
		},
		{
			name:    "1天",
			period:  "1d",
			wantDur: 24 * time.Hour,
			wantErr: false,
		},
		{
			name:    "无效周期",
			period:  "1w",
			wantErr: true,
		},
		{
			name:    "空周期",
			period:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := charts.ResolutionToDuration(tt.period)

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

func TestGmgnFetchTokenCandlesDataParsing(t *testing.T) {
	t.Run("解析完整的蜡烛数据响应", func(t *testing.T) {
		response := `{"code": 0, "msg": "success", "data": {"list": [
			{"open": "1.0", "close": "1.1", "high": "1.2", "low": "0.9", "time": "1700000000000", "volume": "500"},
			{"open": "1.1", "close": "1.2", "high": "1.3", "low": "1.0", "time": "1700000060000", "volume": "600"}
		]}}`

		client := &Client{}
		parsed, err := client.parseGmgnResponse(response)
		if err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		var tokenCandles gmgnTokenCandles
		if err := json.Unmarshal(parsed.Data, &tokenCandles); err != nil {
			t.Fatalf("failed to parse data: %v", err)
		}

		if len(tokenCandles.List) != 2 {
			t.Errorf("expected 2 candles, got %d", len(tokenCandles.List))
		}

		if !tokenCandles.List[0].Open.Equal(decimal.NewFromFloat(1.0)) {
			t.Errorf("first Open mismatch")
		}
		if !tokenCandles.List[1].Close.Equal(decimal.NewFromFloat(1.2)) {
			t.Errorf("second Close mismatch")
		}
	})

	t.Run("解析空蜡烛数据", func(t *testing.T) {
		response := `{"code": 0, "msg": "success", "data": {"list": []}}`

		client := &Client{}
		parsed, err := client.parseGmgnResponse(response)
		if err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		var tokenCandles gmgnTokenCandles
		if err := json.Unmarshal(parsed.Data, &tokenCandles); err != nil {
			t.Fatalf("failed to parse data: %v", err)
		}

		if len(tokenCandles.List) != 0 {
			t.Errorf("expected 0 candles, got %d", len(tokenCandles.List))
		}
	})

	t.Run("解析带错误码的响应", func(t *testing.T) {
		response := `{"code": 1001, "msg": "invalid token", "data": null}`

		client := &Client{}
		_, err := client.parseGmgnResponse(response)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
	})
}

func TestGmgnFetchTokenCandlesOhlcConversion(t *testing.T) {
	t.Run("转换GMGN蜡烛数据到OHLC", func(t *testing.T) {
		gmgnData := []gmgnOhlc{
			{
				Open:   decimal.NewFromFloat(1.0),
				Close:  decimal.NewFromFloat(1.1),
				High:   decimal.NewFromFloat(1.2),
				Low:    decimal.NewFromFloat(0.9),
				Time:   decimal.NewFromInt(1700000000000),
				Volume: decimal.NewFromFloat(500),
			},
			{
				Open:   decimal.NewFromFloat(1.1),
				Close:  decimal.NewFromFloat(1.2),
				High:   decimal.NewFromFloat(1.3),
				Low:    decimal.NewFromFloat(1.0),
				Time:   decimal.NewFromInt(1700000060000),
				Volume: decimal.NewFromFloat(600),
			},
		}

		client := &Client{}
		result := client.convertToOhlcData(gmgnData)

		if len(result) != 2 {
			t.Fatalf("expected 2 ohlc, got %d", len(result))
		}

		expectedTime1 := time.Unix(1700000000000/1000, 0)
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
		client := &Client{}
		result := client.convertToOhlcData([]gmgnOhlc{})
		if len(result) != 0 {
			t.Errorf("expected 0 ohlc, got %d", len(result))
		}
	})

	t.Run("高精度时间戳转换", func(t *testing.T) {
		gmgnData := []gmgnOhlc{
			{
				Open:   decimal.NewFromFloat(1.0),
				Close:  decimal.NewFromFloat(1.1),
				High:   decimal.NewFromFloat(1.1),
				Low:    decimal.NewFromFloat(1.0),
				Time:   decimal.NewFromFloat(1700000000123.5),
				Volume: decimal.NewFromFloat(100),
			},
		}

		client := &Client{}
		result := client.convertToOhlcData(gmgnData)

		expectedTime := time.Unix(1700000000123/1000, 0)
		if !result[0].Time.Equal(expectedTime) {
			t.Errorf("Time mismatch: got %v, want %v", result[0].Time, expectedTime)
		}
	})
}

func TestGmgnFetchTokenCandlesURLConstruction(t *testing.T) {
	t.Run("构建FetchTokenCandles URL", func(t *testing.T) {
		token := "So11111111111111111111111111111111111111112"
		period := "15m"
		to := time.Unix(1700000000, 0)
		limit := 100

		url := gmgnAIBaseURL + "/api/v1/token_candles/sol/" + token + "?" + fakeDeviceInfo + "&resolution=" + period + "&from=0&to=" + fmt.Sprintf("%d", to.UnixMilli()) + "&limit=" + fmt.Sprintf("%d", limit)

		if url == "" {
			t.Error("expected non-empty URL")
		}
		if len(url) < 100 {
			t.Errorf("URL seems too short: %s", url)
		}
	})
}
