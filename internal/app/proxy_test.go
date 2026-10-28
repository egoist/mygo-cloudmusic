package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Entering a proxy in Settings saves and applies the normalized address.
func TestProxySetting(t *testing.T) {
	t.Cleanup(func() { _, _ = netease.SetProxy("") })
	a := newTestApp()
	a.settings = defaultSettings()
	tt := ui.NewTester(a.settingsView, 560, 720)
	tt.Frame()
	if err := tt.Click("代理地址"); err != nil {
		t.Fatal(err)
	}
	tt.Type("127.0.0.1:8080")
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	if a.settings.Proxy != "http://127.0.0.1:8080" {
		t.Fatalf("saved proxy %q", a.settings.Proxy)
	}
	if a.proxyErr != "" {
		t.Fatalf("proxy error %q", a.proxyErr)
	}
}
