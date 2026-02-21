package jupag

// Jupiter 工具函数

import (
	"fmt"
	"math/rand"
	"strconv"

	utls "github.com/refraction-networking/utls"
)

// 支持的TLS客户端HelloID列表
// 用于模拟不同浏览器的TLS指纹
var (
	clientHelloIDs = []utls.ClientHelloID{
		utls.HelloChrome_Auto,
		utls.HelloFirefox_Auto,
		utls.HelloEdge_Auto,
		utls.HelloSafari_Auto,
		utls.Hello360_Auto,
		utls.HelloQQ_Auto,
	}
)

// RandomClientHelloID 随机选择一个TLS客户端HelloID
// 用于模拟随机浏览器的TLS指纹
func RandomClientHelloID() utls.ClientHelloID {
	return clientHelloIDs[rand.Intn(len(clientHelloIDs))]
}

// formatInterval 将周期字符串转换为Jupiter API格式
// 例如: "1m" -> "1_MINUTE", "1h" -> "1_HOUR"
func formatInterval(interval string) (string, error) {
	if len(interval) < 2 {
		return "", fmt.Errorf("invalid interval format: %q (expected format: <number><unit> where unit is s, m, h, d)", interval)
	}

	unit := interval[len(interval)-1]
	nStr := interval[:len(interval)-1]

	n, err := strconv.Atoi(nStr)
	if err != nil {
		return "", fmt.Errorf("invalid number part %q: %v", nStr, err)
	}

	switch unit {
	case 's':
		return fmt.Sprintf("%d_SECOND", n), nil
	case 'm':
		return fmt.Sprintf("%d_MINUTE", n), nil
	case 'h':
		return fmt.Sprintf("%d_HOUR", n), nil
	case 'd':
		return fmt.Sprintf("%d_DAY", n), nil
	default:
		return "", fmt.Errorf("invalid unit %q (expected s, m, h, d)", string(unit))
	}
}
