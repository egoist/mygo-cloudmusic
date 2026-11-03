package network

import (
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/http/httpproxy"
)

var readSystemProxy = systemProxy

// Transport keeps Go's HTTP implementation while honoring the desktop user's
// configured proxy. Explicit environment proxy settings retain precedence.
func Transport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = requestProxy
	return t
}

func requestProxy(req *http.Request) (*url.URL, error) {
	env := httpproxy.FromEnvironment()
	if hasEnvironmentProxy(req.URL.Scheme, env) {
		return env.ProxyFunc()(req.URL)
	}
	server, bypass := readSystemProxy()
	return proxyFor(req.URL, server, bypass, env)
}

func hasEnvironmentProxy(scheme string, env *httpproxy.Config) bool {
	return scheme == "https" && env.HTTPSProxy != "" || scheme == "http" && env.HTTPProxy != ""
}

func proxyFor(target *url.URL, server, bypass string, env *httpproxy.Config) (*url.URL, error) {
	if hasEnvironmentProxy(target.Scheme, env) {
		return env.ProxyFunc()(target)
	}
	if bypassSystemProxy(target, bypass) {
		return nil, nil
	}
	selected := proxyServer(server, target.Scheme)
	if selected == "" {
		return nil, nil
	}
	// A Windows HTTPS mapping is normally an HTTP CONNECT proxy, not an
	// HTTPS connection to the proxy itself. Preserve an explicitly given scheme.
	if !strings.Contains(selected, "://") {
		selected = "http://" + selected
	}
	config := httpproxy.Config{HTTPProxy: selected, HTTPSProxy: selected, NoProxy: env.NoProxy, CGI: env.CGI}
	return config.ProxyFunc()(target)
}

func proxyServer(servers, scheme string) string {
	var fallback string
	for _, entry := range strings.FieldsFunc(servers, func(r rune) bool { return r == ';' || r == ' ' || r == '\t' }) {
		key, value, mapped := strings.Cut(entry, "=")
		if mapped {
			if strings.EqualFold(key, scheme) {
				return value
			}
		} else if fallback == "" {
			fallback = entry
		}
	}
	return fallback
}

func bypassSystemProxy(target *url.URL, bypass string) bool {
	host := strings.ToLower(target.Hostname())
	for _, entry := range strings.Split(bypass, ";") {
		pattern := strings.ToLower(strings.TrimSpace(entry))
		if pattern == "" {
			continue
		}
		if pattern == "<local>" {
			if net.ParseIP(host) == nil && !strings.Contains(host, ".") && !strings.Contains(host, ":") {
				return true
			}
			continue
		}
		if matched, _ := path.Match(pattern, host); matched {
			return true
		}
		if matched, _ := path.Match(pattern, strings.ToLower(target.Host)); matched {
			return true
		}
	}
	return false
}
