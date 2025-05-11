package proxy

import (
	"net/http/httputil"
	"net/url"
)

func NewReverseProxy(target string) *httputil.ReverseProxy {
	targetURL, _ := url.Parse(target)
	return httputil.NewSingleHostReverseProxy(targetURL)
}
