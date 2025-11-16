package gmgn

import (
	"math/rand"

	utls "github.com/refraction-networking/utls"
)

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

func RandomClientHelloID() utls.ClientHelloID {
	return clientHelloIDs[rand.Intn(len(clientHelloIDs))]
}
