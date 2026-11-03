package qqmusic

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"testing"
)

// A TLS close before sending HTTP reconnects once without changing the signed URL.
func TestAuthReconnectsBeforeRequest(t *testing.T) {
	old := authHTTP
	t.Cleanup(func() { authHTTP = old })
	requests := 0
	authHTTP = &http.Client{Transport: authRoundTripper(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Host != "ssl.ptlogin2.graph.qq.com" || r.URL.Query().Get("ptsigx") != "fake" {
			t.Error("changed signed authorization target")
		}
		if requests == 1 {
			return nil, io.EOF
		}
		return &http.Response{StatusCode: 302, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}
	res, err := authSend("GET", "https://ssl.ptlogin2.graph.qq.com/check_sig?ptsigx=fake", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if requests != 2 {
		t.Fatalf("requests=%d", requests)
	}
}

// Established connections, POSTs and repeated TLS failures must not replay one-time authorization.
func TestAuthDoesNotReplayAuthorization(t *testing.T) {
	for _, test := range []struct {
		name, method string
		connected    bool
		want         int
	}{
		{"connected-get", "GET", true, 1},
		{"oauth-post", "POST", false, 1},
		{"repeated-tls-close", "GET", false, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			old := authHTTP
			t.Cleanup(func() { authHTTP = old })
			requests := 0
			authHTTP = &http.Client{Transport: authRoundTripper(func(r *http.Request) (*http.Response, error) {
				requests++
				if test.connected {
					httptrace.ContextClientTrace(r.Context()).GotConn(httptrace.GotConnInfo{})
				}
				return nil, io.EOF
			})}
			_, err := authSend(test.method, "https://graph.qq.com/oauth2.0/authorize?code=fake", nil, nil)
			if !errors.Is(err, io.EOF) || requests != test.want {
				t.Fatalf("requests=%d err=%v", requests, err)
			}
		})
	}
}
