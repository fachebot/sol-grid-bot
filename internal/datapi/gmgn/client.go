package gmgn

// GMGN AI API 客户端
// 用于获取GMGN的K线数据、持仓信息、钱包持仓和热门代币数据

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fachebot/sol-grid-bot/internal/charts"
	"github.com/fachebot/sol-grid-bot/internal/config"

	"github.com/enetx/g"
	"github.com/enetx/surf"
	"github.com/google/uuid"
)

const (
	// GMGN AI 基础URL
	gmgnAIBaseURL = "https://gmgn.ai"
)

// fakeDeviceInfo 模拟的设备信息参数
var fakeDeviceInfo = ""

// getDeviceInfo 生成模拟的设备信息
// 用于绕过GMGN的反爬虫检测
func getDeviceInfo() (string, error) {
	deviceId, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}

	deviceInfo := map[string]string{
		"device_id": deviceId.String(),
		"client_id": "gmgn_web_20250820-2734-a756529",
		"from_app":  "gmgn",
		"app_ver":   "20250820-2734-a756529",
		"tz_name":   "Etc/GMT-8",
		"tz_offset": "28800",
		"app_lang":  "zh-CN",
		"os":        "web",
	}
	return encodeURLParams(deviceInfo), nil
}

// encodeURLParams 将参数Map编码为URL查询字符串
func encodeURLParams(params map[string]string) string {
	var pairs []string
	for k, v := range params {
		pairs = append(pairs, k+"="+v)
	}
	return strings.Join(pairs, "&")
}

// init 初始化模块，在包加载时生成设备信息
func init() {
	var err error
	fakeDeviceInfo, err = getDeviceInfo()
	if err != nil {
		panic(err)
	}
}

// Client GMGN API客户端结构体
type Client struct {
	surfClient *surf.Client // HTTP客户端
}

// NewClient 创建新的GMGN API客户端
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

// doRequest 发送HTTP请求到GMGN API
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

// parseGmgnResponse 解析GMGN响应
func (c *Client) parseGmgnResponse(responseBody string) (*gmgnResponse, error) {
	var res gmgnResponse
	if err := json.Unmarshal([]byte(responseBody), &res); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if res.Code != 0 {
		return nil, fmt.Errorf("gmgn api error - code: %d, msg: %s", res.Code, res.Msg)
	}

	return &res, nil
}

// convertToOhlcData 转换GMGN OHLC数据为标准格式
func (c *Client) convertToOhlcData(gmgnData []gmgnOhlc) []charts.Ohlc {
	ohlcs := make([]charts.Ohlc, 0, len(gmgnData))
	for _, item := range gmgnData {
		ohlc := charts.Ohlc{
			Open:   item.Open,
			Close:  item.Close,
			High:   item.High,
			Low:    item.Low,
			Time:   time.Unix(item.Time.IntPart()/1000, 0),
			Volume: item.Volume,
		}
		ohlcs = append(ohlcs, ohlc)
	}
	return ohlcs
}

// FetchTokenCandles 获取代币K线数据
// ctx: 上下文
// token: 代币地址
// to: 结束时间
// period: K线周期 (如 "1m", "5m", "1h", "1d")
// limit: 返回数据条数限制
// 返回: K线数据切片和错误
func (c *Client) FetchTokenCandles(ctx context.Context, token string, to time.Time, period string, limit int) ([]charts.Ohlc, error) {
	intervalD, err := charts.ResolutionToDuration(period)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/token_candles/sol/%s?%s&resolution=%s&from=0&to=%d&limit=%d",
		gmgnAIBaseURL, token, fakeDeviceInfo, period, to.UnixMilli(), limit)
	referer := fmt.Sprintf("%s/sol/token/%s", gmgnAIBaseURL, token)

	response, err := c.doRequest(ctx, url, "GET", nil, referer)
	if err != nil {
		return nil, err
	}

	gmgnResp, err := c.parseGmgnResponse(response)
	if err != nil {
		return nil, err
	}

	var tokenCandles gmgnTokenCandles
	if err := json.Unmarshal(gmgnResp.Data, &tokenCandles); err != nil {
		return nil, fmt.Errorf("failed to parse ohlc data: %w", err)
	}

	result := c.convertToOhlcData(tokenCandles.List)
	result = charts.FillMissingOhlc(result, to, intervalD)

	return result, nil
}

// FetchTokenHolders 获取代币持有者列表
// ctx: 上下文
// token: 代币地址
// 返回: 持有者信息列表和错误
func (c *Client) FetchTokenHolders(ctx context.Context, token string) ([]*HolderInfo, error) {
	url := fmt.Sprintf("%s/vas/api/v1/token_holders/sol/%s?%s&limit=100&cost=20orderby=amount_percentage&direction=desc",
		gmgnAIBaseURL, token, fakeDeviceInfo)
	referer := fmt.Sprintf("%s/sol/token/%s", gmgnAIBaseURL, token)

	response, err := c.doRequest(ctx, url, "GET", nil, referer)
	if err != nil {
		return nil, err
	}

	gmgnResp, err := c.parseGmgnResponse(response)
	if err != nil {
		return nil, err
	}

	var holders HolderInfoList
	if err := json.Unmarshal(gmgnResp.Data, &holders); err != nil {
		return nil, fmt.Errorf("failed to parse holder list: %w", err)
	}

	return holders.List, nil
}

// FetchWalletHoldings 获取钱包持仓列表
// ctx: 上下文
// wallet: 钱包地址
// 返回: 持仓信息列表和错误
func (c *Client) FetchWalletHoldings(ctx context.Context, wallet string) ([]*WalletHolding, error) {
	url := fmt.Sprintf("%s/api/v1/wallet_holdings/sol/%s?%s&limit=50&orderby=last_active_timestamp&direction=desc&showsmall=false&sellout=false&hide_airdrop=true&hide_abnormal=false",
		gmgnAIBaseURL, wallet, fakeDeviceInfo)
	referer := fmt.Sprintf("%s/sol/address/%s", gmgnAIBaseURL, wallet)

	response, err := c.doRequest(ctx, url, "GET", nil, referer)
	if err != nil {
		return nil, err
	}

	gmgnResp, err := c.parseGmgnResponse(response)
	if err != nil {
		return nil, err
	}

	var holdings WalletHoldings
	if err := json.Unmarshal(gmgnResp.Data, &holdings); err != nil {
		return nil, fmt.Errorf("failed to parse wallet holder list: %w", err)
	}

	return holdings.Holdings, nil
}

// FetchTrendingToken1H 获取1小时内热门代币
// ctx: 上下文
// tokenFilter: 代币过滤条件
// 返回: 热门代币列表和错误
func (c *Client) FetchTrendingToken1H(ctx context.Context, tokenFilter TokenFilter) (*TrendingTokens, error) {
	params := []string{
		"orderby=renowned_count",
		"direction=desc",
		"filters[]=frozen",
		"filters[]=burn",
		"filters[]=distribed",
		"platforms[]=pump",
		"platforms[]=pumpamm",
		"platforms[]=moonshot",
		"platforms[]=raydium",
		"platforms[]=meteora",
		"platforms[]=fluxbeam",
		"platforms[]=orca",
		"platforms[]=ray_launchpad",
		"platforms[]=boop",
		"platforms[]=letsbonk",
		fmt.Sprintf("min_created=%dm", tokenFilter.MinCreatedMinutes),
		fmt.Sprintf("max_created=%dm", tokenFilter.MaxCreatedMinutes),
		fmt.Sprintf("min_marketcap=%v", tokenFilter.MinMarketcap),
		fmt.Sprintf("max_marketcap=%v", tokenFilter.MaxMarketcap),
		fmt.Sprintf("min_holder_count=%d", tokenFilter.MinHolderCount),
		fmt.Sprintf("min_swaps=%d", tokenFilter.MinSwaps1H),
		fmt.Sprintf("min_volume=%v", tokenFilter.MinVolume1H),
	}

	referer := "https://gmgn.ai/trend?chain=sol"
	url := fmt.Sprintf("%s/defi/quotation/v1/rank/sol/swaps/1h?%s&%s", gmgnAIBaseURL, fakeDeviceInfo, strings.Join(params, "&"))

	response, err := c.doRequest(ctx, url, "GET", nil, referer)
	if err != nil {
		return nil, err
	}

	gmgnResp, err := c.parseGmgnResponse(response)
	if err != nil {
		return nil, err
	}

	var trendingTokens TrendingTokens
	if err := json.Unmarshal(gmgnResp.Data, &trendingTokens); err != nil {
		return nil, fmt.Errorf("failed to parse trending token: %w", err)
	}

	return &trendingTokens, nil
}
