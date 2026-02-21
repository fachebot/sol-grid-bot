package okxweb3

// OKX Web3 工具函数

import (
	"math/rand"

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
