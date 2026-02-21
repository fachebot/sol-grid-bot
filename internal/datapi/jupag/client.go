package jupag

// Jupiter Aggregator API 客户端
// 用于获取JupiterDEX的交易数据和K线数据

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fachebot/sol-grid-bot/internal/charts"
	"github.com/fachebot/sol-grid-bot/internal/config"

	"github.com/enetx/g"
	"github.com/enetx/surf"
)

const (
	// Jupiter Data API 基础URL
	jupagBaseURL = "https://datapi.jup.ag"
)

// Client Jupiter API客户端结构体
type Client struct {
	surfClient *surf.Client // HTTP客户端
}

// NewClient 创建新的Jupiter API客户端
// proxy: SOCK5 代理配置
func NewClient(proxy config.Sock5Proxy) *Client {
	var cli *surf.Client

	if proxy.Enable {
		proxyURL := g.String(fmt.Sprintf("socks5://%s:%d", proxy.Host, proxy.Port))
		cli = surf.NewClient().
			Builder().
			Impersonate().
			Chrome().
			Proxy(proxyURL).
			Build().
			Unwrap()
	} else {
		cli = surf.NewClient().
			Builder().
			Impersonate().
			Chrome().
			Build().
			Unwrap()
	}

	return &Client{
		surfClient: cli,
	}
}

// doRequest 发送HTTP请求到Jupiter API
// ctx: 上下文
// url: 请求URL
// method: HTTP方法 (GET/POST)
// bodyJson: 请求体JSON数据
// referer: Referer头
// 返回: 响应体字符串和错误
func (c *Client) doRequest(ctx context.Context, url, method string, bodyJson any, referer string) (string, error) {
	headers := map[string]string{
		"accept":          "application/json, text/plain, */*",
		"accept-language": "zh-CN,zh;q=0.9",
		"accept-encoding": "gzip,deflate,br",
	}
	if referer != "" {
		headers["referer"] = referer
	}

	rawURL := g.String(url)
	var respResult g.Result[*surf.Response]

	if bodyJson != nil {
		bodyBytes, _ := json.Marshal(bodyJson)
		switch method {
		case "POST":
			respResult = c.surfClient.Post(rawURL).Body(bodyBytes).WithContext(ctx).AddHeaders(headers).Do()
		default:
			respResult = c.surfClient.Get(rawURL).WithContext(ctx).AddHeaders(headers).Do()
		}
	} else {
		switch method {
		case "POST":
			respResult = c.surfClient.Post(rawURL).WithContext(ctx).AddHeaders(headers).Do()
		default:
			respResult = c.surfClient.Get(rawURL).WithContext(ctx).AddHeaders(headers).Do()
		}
	}

	if respResult.IsErr() {
		return "", fmt.Errorf("request failed: %w", respResult.Err())
	}

	resp := respResult.Ok()
	statusCode := int(resp.StatusCode)
	if statusCode < 200 || statusCode >= 300 {
		return "", fmt.Errorf("http status: %d", statusCode)
	}

	body := resp.Body.String().Unwrap()
	return body.Std(), nil
}

// FetchTokenCandles 获取代币K线数据
// ctx: 上下文
// token: 代币地址
// to: 结束时间
// period: K线周期 (如 "1m", "5m", "1h", "1d")
// limit: 返回数据条数限制
// 返回: K线数据切片和错误
func (c *Client) FetchTokenCandles(ctx context.Context, token string, to time.Time, period string, limit int) ([]charts.Ohlc, error) {
	interval, err := formatInterval(period)
	if err != nil {
		return nil, err
	}

	intervalD, err := charts.ResolutionToDuration(period)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v2/charts/%s?interval=%s&to=%d&candles=%d&type=price",
		jupagBaseURL, token, interval, to.UnixMilli(), limit)

	response, err := c.doRequest(ctx, url, http.MethodGet, nil, "https://jup.ag/")
	if err != nil {
		return nil, err
	}

	var tokenCandles tokenCandlesResponse
	if err := json.Unmarshal([]byte(response), &tokenCandles); err != nil {
		return nil, fmt.Errorf("failed to parse ohlc data: %w", err)
	}

	result := make([]charts.Ohlc, 0, len(tokenCandles.Candles))
	for _, item := range tokenCandles.Candles {
		result = append(result, charts.Ohlc{
			Open:   item.Open,
			Close:  item.Close,
			High:   item.High,
			Low:    item.Low,
			Time:   time.Unix(item.Time, 0),
			Volume: item.Volume,
		})
	}

	result = charts.FillMissingOhlc(result, to, intervalD)

	return result, nil
}
