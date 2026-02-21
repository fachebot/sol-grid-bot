package okxweb3

// OKX Web3 API 客户端
// 用于获取OKX DEX的K线数据

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/fachebot/sol-grid-bot/internal/charts"
	"github.com/fachebot/sol-grid-bot/internal/config"

	"github.com/enetx/g"
	"github.com/enetx/surf"
	"github.com/shopspring/decimal"
)

const (
	// OKX Web3 API 基础URL
	okxBaseURL = "https://web3.okx.com"
)

// Client OKX Web3 API 客户端结构体
type Client struct {
	proxy      config.Sock5Proxy // SOCK5 代理配置
	surfClient *surf.Client      // HTTP 客户端
}

// NewClient 创建新的OKX Web3 API客户端
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
		proxy:      proxy,
		surfClient: cli,
	}
}

// doRequest 发送HTTP请求到OKX API
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

// parseOkxResponse 解析OKX响应
func (c *Client) parseOkxResponse(responseBody string) (*okxResponse, error) {
	var res okxResponse
	if err := json.Unmarshal([]byte(responseBody), &res); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if res.Code != "0" {
		return nil, fmt.Errorf("okx api error - code: %s, msg: %s", res.Code, res.Msg)
	}

	return &res, nil
}

// FetchTokenCandles 获取代币K线数据
// ctx: 上下文
// token: 代币地址
// to: 结束时间
// interval: K线周期 (如 "1m", "5m", "1h", "1d")
// limit: 返回数据条数限制
// 返回: K线数据切片和错误
func (c *Client) FetchTokenCandles(ctx context.Context, token string, to time.Time, interval string, limit int) ([]charts.Ohlc, error) {
	intervalD, err := charts.ResolutionToDuration(interval)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/priapi/v5/dex/token/market/dex-token-hlc-candles?chainId=501&address=%s&after=%d&bar=%s&limit=%d&t=%d",
		okxBaseURL, token, to.UnixMilli(), interval, limit, time.Now().Unix())

	response, err := c.doRequest(ctx, url, "GET", nil, "https://web3.okx.com/")
	if err != nil {
		return nil, err
	}

	okxResp, err := c.parseOkxResponse(response)
	if err != nil {
		return nil, err
	}

	var tokenCandles [][]decimal.Decimal
	if err := json.Unmarshal(okxResp.Data, &tokenCandles); err != nil {
		return nil, fmt.Errorf("failed to parse ohlc data: %w", err)
	}

	result := make([]charts.Ohlc, 0, len(tokenCandles))
	for _, data := range tokenCandles {
		if len(data) < 8 {
			return nil, fmt.Errorf("failed to parse ohlc data: %+v", data)
		}

		result = append(result, charts.Ohlc{
			Open:   data[1],
			Close:  data[2],
			High:   data[3],
			Low:    data[4],
			Time:   time.UnixMilli(data[0].IntPart()),
			Volume: data[6],
		})
	}

	slices.Reverse(result)
	result = charts.FillMissingOhlc(result, to, intervalD)

	return result, nil
}
