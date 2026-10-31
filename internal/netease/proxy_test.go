package netease

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The configured proxy carries NetEase API and CDN requests, while updates
// and other traffic keep the transport's environment proxy behavior.
func TestProxyRoutesOnlyNetEase(t *testing.T) {
	t.Cleanup(func() { _, _ = SetProxy("") })
	proxy, err := SetProxy("127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	if proxy != "http://127.0.0.1:8080" {
		t.Fatalf("normalized proxy %q", proxy)
	}
	for _, target := range []string{
		"https://music.163.com/api/personalized/playlist",
		"https://interfacepc.music.163.com/eapi/song/enhance/player/url/v1",
		"https://p1.music.126.net/cover.jpg",
		"https://m701.music.126.net/song.flac",
	} {
		req, _ := http.NewRequest("GET", target, nil)
		got, err := HTTPTransport.Proxy(req)
		if err != nil || got == nil || got.String() != proxy {
			t.Errorf("proxy for %s = %v, %v", target, got, err)
		}
	}
	for _, target := range []string{"https://github.com/egoist/mygo-cloudmusic", "https://163.com.example.com/song"} {
		req, _ := http.NewRequest("GET", target, nil)
		got, err := HTTPTransport.Proxy(req)
		want, wantErr := http.ProxyFromEnvironment(req)
		if (err == nil) != (wantErr == nil) || (got == nil) != (want == nil) || got != nil && want != nil && got.String() != want.String() {
			t.Errorf("proxy for %s = %v, %v, want environment %v, %v", target, got, err, want, wantErr)
		}
	}
}

// Clearing the address restores the environment proxy behavior.
func TestEmptyProxyRestoresEnvironment(t *testing.T) {
	t.Cleanup(func() { _, _ = SetProxy("") })
	if _, err := SetProxy("http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if proxy, err := SetProxy(""); err != nil || proxy != "" {
		t.Fatalf("cleared proxy = %q, %v", proxy, err)
	}
	req, _ := http.NewRequest("GET", "https://music.163.com/api", nil)
	got, err := HTTPTransport.Proxy(req)
	want, wantErr := http.ProxyFromEnvironment(req)
	if (err == nil) != (wantErr == nil) || (got == nil) != (want == nil) || got != nil && want != nil && got.String() != want.String() {
		t.Fatalf("proxy after clearing = %v, %v, want environment %v, %v", got, err, want, wantErr)
	}
}

// HTTP, HTTPS, and SOCKS5 proxy addresses are accepted without rewriting
// their schemes.
func TestProxySchemes(t *testing.T) {
	t.Cleanup(func() { _, _ = SetProxy("") })
	for _, raw := range []string{"http://127.0.0.1:8080", "https://127.0.0.1:8080", "socks5://127.0.0.1:1080"} {
		proxy, err := SetProxy(raw)
		if err != nil || proxy != raw {
			t.Errorf("proxy %q = %q, %v", raw, proxy, err)
		}
	}
}

// Rejecting an invalid proxy leaves the last working configuration intact.
func TestInvalidProxyDoesNotReplaceWorkingProxy(t *testing.T) {
	t.Cleanup(func() { _, _ = SetProxy("") })
	proxy, err := SetProxy("http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"ftp://127.0.0.1:8080", "http://", "http://127.0.0.1:8080/path"} {
		if _, err := SetProxy(raw); err == nil {
			t.Errorf("accepted invalid proxy %q", raw)
		}
	}
	req, _ := http.NewRequest("GET", "https://music.163.com/api", nil)
	got, err := HTTPTransport.Proxy(req)
	if err != nil || got == nil || got.String() != proxy {
		t.Fatalf("proxy after invalid input = %v, %v", got, err)
	}
}

// Cover downloads go through the configured proxy rather than connecting
// directly to the CDN.
func TestFetchUsesProxy(t *testing.T) {
	t.Cleanup(func() { _, _ = SetProxy("") })
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() != "http://p1.music.126.net/cover.jpg" {
			t.Errorf("proxy received %s", r.URL)
		}
		_, _ = w.Write([]byte("cover"))
	}))
	defer proxy.Close()
	if _, err := SetProxy(proxy.URL); err != nil {
		t.Fatal(err)
	}
	body, err := Fetch("http://p1.music.126.net/cover.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "cover" {
		t.Fatalf("fetched %q", body)
	}
}
