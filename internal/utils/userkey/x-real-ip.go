package userkey

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
)

type XRealIP struct {
	v string
	t string
}

func (xip XRealIP) Value() string {
	return xip.v
}

func (xip XRealIP) Type() string {
	return xip.t
}

func NewXRealIP() XRealIP {
	return XRealIP{
		v: generateRandomIP(),
		t: "X-Real-IP",
	}
}

func ReqToXRealIp(r *http.Request) (XRealIP, error) {
	t := "X-Real-IP"
	ip := r.Header.Get(t)
	if ip == "" {
		return NewXRealIP(), fmt.Errorf("%s header not found", t)
	}

	return XRealIP{v: ip, t: t}, nil
}

// GenerateRandomIP генерирует случайный IPv4-адрес
func generateRandomIP() string {
	octets := make([]byte, 4)
	for i := range octets {
		switch i {
		case 0:
			octets[i] = byte(rand.Intn(223) + 1)
		case 1:
			octets[i] = byte(rand.Intn(254) + 1)
		default:
			octets[i] = byte(rand.Intn(255))
		}
	}

	return net.IPv4(octets[0], octets[1], octets[2], octets[3]).String()
}
