package userkey

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

type IP struct {
	v string
	t string
}

func (ip IP) Value() string {
	return ip.v
}

func (ip IP) Type() string {
	return ip.t
}

func ReqToIP(r *http.Request) (IP, error) {
	t := "IP"
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ips := strings.Split(forwarded, ", ")
		if len(ips) > 0 {
			return IP{v: ips[0], t: t}, nil
		}
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		if r.RemoteAddr != "" {
			return IP{v: r.RemoteAddr, t: t}, err
		}
		return IP{}, fmt.Errorf("%s header not found", t)
	}

	if ip != "" {
		return IP{v: ip, t: t}, nil
	}
	return IP{}, fmt.Errorf("%s header not found", t)

}
