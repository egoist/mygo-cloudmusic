package netease

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
)

var configuredProxy atomic.Pointer[url.URL]

// HTTPTransport carries NetEase API, image, and audio requests. Other
// packages can use it without routing their non-NetEase requests through
// the configured proxy.
var HTTPTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	fromEnvironment := t.Proxy
	t.Proxy = func(req *http.Request) (*url.URL, error) {
		if proxy := configuredProxy.Load(); proxy != nil && neteaseHost(req.URL.Hostname()) {
			return proxy, nil
		}
		return fromEnvironment(req)
	}
	return t
}()

// SetProxy sets the proxy for NetEase requests and returns its normalized
// URL. An empty address restores the transport's environment proxy behavior.
func SetProxy(raw string) (string, error) {
	proxy, err := parseProxy(raw)
	if err != nil {
		return "", err
	}
	configuredProxy.Store(proxy)
	HTTPTransport.CloseIdleConnections()
	if proxy == nil {
		return "", nil
	}
	return proxy.String(), nil
}

func parseProxy(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("invalid proxy URL")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	switch u.Scheme {
	case "http", "https", "socks5":
	default:
		return nil, errors.New("proxy must use http, https, or socks5")
	}
	u.Path = ""
	return u, nil
}

func neteaseHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	for _, domain := range []string{"163.com", "126.net"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}
