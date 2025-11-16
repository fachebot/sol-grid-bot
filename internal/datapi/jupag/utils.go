package jupag

import (
	"fmt"
	"math/rand"
	"strconv"

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
