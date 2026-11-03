package network

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"golang.org/x/net/http/httpproxy"
)

// Desktop proxy selection must honor protocol mappings, bypass lists and explicit environment choices.
func TestProxySelection(t *testing.T) {
	for _, test := range []struct{ name, target, server, bypass, envHTTPS, noProxy, want string }{
		{"shared", "https://xui.ptlogin2.qq.com/ssl/ptqrshow", "proxy.test:8080", "", "", "", "http://proxy.test:8080"},
		{"https-map", "https://graph.qq.com/", "http=plain.test:80;https=secure.test:81", "", "", "", "http://secure.test:81"},
		{"unmapped-direct", "https://graph.qq.com/", "http=plain.test:80", "", "", "", ""},
		{"environment-first", "https://graph.qq.com/", "system.test:80", "*.qq.com", "http://env.test:81", "", "http://env.test:81"},
		{"environment-bypass", "https://graph.qq.com/", "system.test:80", "", "http://env.test:81", ".qq.com", ""},
		{"system-bypass", "https://graph.qq.com/", "system.test:80", "*.qq.com", "", "", ""},
		{"local-name", "http://intranet/", "system.test:80", "<local>", "", "", ""},
		{"private-wildcard", "http://192.168.1.2/", "system.test:80", "192.168.*", "", "", ""},
		{"system-with-no-proxy", "https://graph.qq.com/", "system.test:80", "", "", "graph.qq.com", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			u, err := url.Parse(test.target)
			if err != nil {
				t.Fatal(err)
			}
			proxy, err := proxyFor(u, test.server, test.bypass, &httpproxy.Config{HTTPSProxy: test.envHTTPS, NoProxy: test.noProxy})
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if proxy != nil {
				got = proxy.String()
			}
			if got != test.want {
				t.Fatalf("proxy=%q want=%q", got, test.want)
			}
		})
	}
}

// A request must actually reach the configured desktop proxy when no proxy environment exists.
func TestRequestUsesSystemProxy(t *testing.T) {
	for _, name := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy", "REQUEST_METHOD"} {
		t.Setenv(name, "")
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Host != "qq.invalid" {
			t.Errorf("unexpected proxy target %s", r.URL.Host)
		}
		_, _ = io.WriteString(w, "proxy reached")
	}))
	defer server.Close()
	old := readSystemProxy
	t.Cleanup(func() { readSystemProxy = old })
	readSystemProxy = func() (string, string) { return server.URL, "" }
	transport := Transport()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	response, err := client.Get("http://qq.invalid/qr")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "proxy reached" || requests != 1 {
		t.Fatalf("requests=%d body=%q err=%v", requests, body, err)
	}
}
