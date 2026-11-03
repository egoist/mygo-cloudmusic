package qqmusic

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"testing"
	"time"
)

// A WeChat scan retains its cursor and cookie jar, then exchanges the code with the bootstrapped device.
func TestWechatQRLogin(t *testing.T) {
	var imageBytes bytes.Buffer
	_ = png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 37, 37)))
	polls, exchanges := 0, 0
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Host + r.URL.Path {
		case "api.tencentmusic.com/tme/trpc/proxy":
			if r.Header.Get("method") != "GetQimei" || r.Header.Get("User-Agent") != "QQMusic" {
				t.Error("missing QIMEI headers")
			}
			var body struct {
				Params map[string]string `json:"qimeiParams"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			p := body.Params
			if p["key"] == "" || p["params"] == "" || p["time"] == "" || p["nonce"] == "" || p["sign"] != protocolMD5(p["key"], p["params"], p["time"]+"000", p["nonce"], "ZdJqM15EeO2zWc08", p["extra"]) {
				t.Error("invalid QIMEI signed envelope")
			}
			io.WriteString(w, `{"code":0,"data":"{\"code\":0,\"data\":{\"q16\":\"device-q16\",\"q36\":\"device-q36\"}}"}`)
		case "open.weixin.qq.com/connect/qrconnect":
			if r.URL.Query().Get("appid") != wechatAppID {
				t.Error("incorrect WeChat app id")
			}
			http.SetCookie(w, &http.Cookie{Name: "wx_session", Value: "web-cookie", Domain: "weixin.qq.com", Path: "/"})
			io.WriteString(w, `<img src="/connect/qrcode/uuid-fixture"><a href="https://open.weixin.qq.com/connect/confirm?uuid=uuid-fixture">`)
		case "open.weixin.qq.com/connect/qrcode/uuid-fixture":
			w.Write(imageBytes.Bytes())
		case "lp.open.weixin.qq.com/connect/l/qrconnect":
			if r.Header.Get("Cookie") != "wx_session=web-cookie" {
				t.Error("lost WeChat web cookie")
			}
			if polls >= 2 && r.URL.Query().Get("last") != "404" {
				t.Error("lost scanned cursor")
			}
			statuses := []string{"408", "404", "408", "405"}
			io.WriteString(w, "window.wx_errcode="+statuses[polls]+";window.wx_code='private-code';")
			polls++
		case "u.y.qq.com/cgi-bin/musicu.fcg":
			if r.Header.Get("Cookie") != "" {
				t.Error("WeChat cookie leaked to musicu")
			}
			var body struct {
				Comm map[string]any `json:"comm"`
				Req  struct {
					Module, Method string
					Param          map[string]any
				} `json:"req"`
				Req0 struct{ Module, Method string } `json:"req_0"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Comm["QIMEI"] != "device-q16" || body.Comm["QIMEI36"] != "device-q36" {
				t.Error("lost bootstrapped QIMEI")
			}
			if body.Req0.Method == "GetSession" {
				io.WriteString(w, `{"code":0,"req_0":{"code":0,"data":{"session":{"uid":123,"sid":"session-sid"}}}}`)
				return
			}
			exchanges++
			if body.Req.Module != "music.login.LoginServer" || body.Req.Method != "Login" || body.Req.Param["strAppid"] != wechatAppID || body.Req.Param["code"] != "private-code" || body.Comm["tmeLoginType"] != float64(1) || body.Comm["uid"] != "123" || body.Comm["sid"] != "session-sid" {
				t.Error("invalid WeChat credential exchange")
			}
			io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":0,"str_musicid":"1152921504606846976","musickey":"wechat-key","openid":"open","refresh_key":"refresh","nick":"Listener"}}}`)
		default:
			t.Errorf("unexpected request %s%s", r.Host, r.URL.Path)
			w.WriteHeader(500)
		}
	})
	old := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = old })
	setSession(Session{MusicID: "10002", MusicKey: "prior-account", LoginType: 2})
	key, err := WechatQRKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{QRWaiting, QRScanned, QRScanned, QRConfirmed} {
		code, _, _, _, err := QRCheck(key)
		if err != nil || code != want {
			t.Fatalf("check = %d %v, want %d", code, err, want)
		}
	}
	s := sessionSnapshot()
	if exchanges != 1 || !s.valid() || s.MusicID != "1152921504606846976" || s.LoginType != 1 || s.Device == nil || s.RefreshKey != "refresh" {
		t.Fatal("WeChat session lost its account or refresh data")
	}
	encoded, _ := json.Marshal(s)
	var restored Session
	if json.Unmarshal(encoded, &restored) != nil || !restored.valid() || restored.Device.Qimei != "device-q16" {
		t.Fatal("WeChat session did not survive persistence")
	}
}

// A closed QR must never publish a credential returned by an already running exchange.
func TestCanceledQRDoesNotSignIn(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "lp.open.weixin.qq.com" {
			io.WriteString(w, `window.wx_errcode=405;window.wx_code='code';`)
			return
		}
		close(started)
		<-release
		io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"str_musicid":"10001","musickey":"key"}}}`)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	authState.Lock()
	authState.qr = map[string]qrLogin{"closed": {expires: time.Now().Add(time.Minute), wechat: &wechatQR{client: authHTTP, uuid: "uuid", ctx: ctx, cancel: cancel, device: &wechatDevice{}}}}
	authState.Unlock()
	result := make(chan int, 1)
	go func() { code, _, _, _, _ := QRCheck("closed"); result <- code }()
	<-started
	CancelQR("closed")
	close(release)
	if <-result != QRExpired || sessionSnapshot().MusicKey != "" {
		t.Fatal("closed QR signed in")
	}
}

// A rejected WeChat key must refresh with all credential fields before retrying the profile read.
func TestWechatAccountRefreshesCredential(t *testing.T) {
	oldWrite := writePersistedSession
	var persisted Session
	writePersistedSession = func(b []byte) error { return json.Unmarshal(b, &persisted) }
	t.Cleanup(func() { writePersistedSession = oldWrite })
	reads, refreshes := 0, 0
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Comm map[string]any                 `json:"comm"`
			Req  struct{ Param map[string]any } `json:"req"`
			Req0 struct{ Method string }        `json:"req_0"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Req0.Method == "GetLoginUserInfo" {
			reads++
			if reads == 1 {
				io.WriteString(w, `{"code":0,"req_0":{"code":1000,"data":{}}}`)
				return
			}
			if body.Comm["authst"] != "new-key" {
				t.Error("profile retry used rejected key")
			}
			io.WriteString(w, `{"code":0,"req_0":{"code":0,"data":{"info":{"nick":"Listener"}}}}`)
			return
		}
		refreshes++
		if r.Header.Get("Cookie") != "uin=10001; qqmusic_uin=10001; qm_keyst=old-key; qqmusic_key=old-key" {
			t.Error("refresh lost credential cookie")
		}
		p := body.Req.Param
		if p["openid"] != "open" || p["refresh_token"] != "token" || p["str_musicid"] != "10001" || p["musickey"] != "old-key" || p["unionid"] != "union" || p["refresh_key"] != "refresh" || p["loginMode"] != float64(2) {
			t.Error("incomplete refresh credential")
		}
		io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"str_musicid":"10001","musickey":"new-key"}}}`)
	})
	old := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = old })
	setSession(Session{MusicID: "10001", MusicKey: "old-key", LoginType: 1, OpenID: "open", RefreshToken: "token", UnionID: "union", RefreshKey: "refresh", Device: &wechatDevice{Qimei: "q16", Qimei36: "q36"}})
	p, err := Account()
	if err != nil || p == nil || p.UserID != 10001 || reads != 2 || refreshes != 1 || sessionSnapshot().MusicKey != "new-key" {
		t.Fatalf("refresh failed: profile=%v err=%v reads=%d refreshes=%d", p, err, reads, refreshes)
	}
	if persisted.MusicKey != "new-key" || persisted.RefreshKey != "refresh" {
		t.Fatal("refreshed credential was not persisted")
	}
}

// Expired WeChat keys with refresh material must survive restore and renew before any account read.
func TestExpiredWechatSessionRestoresAndRefreshes(t *testing.T) {
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Req  struct{ Param map[string]any } `json:"req"`
			Req0 struct{ Method string }        `json:"req_0"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Req0.Method == "GetLoginUserInfo" {
			io.WriteString(w, `{"code":0,"req_0":{"code":0,"data":{"info":{"nick":"renewed"}}}}`)
			return
		}
		if body.Req.Param["loginMode"] != float64(2) {
			t.Error("expired key was not refreshed")
		}
		io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"str_musicid":"10001","musickey":"renewed-key","refresh_key":"rotated-refresh"}}}`)
	})
	oldHTTP := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = oldHTTP })
	oldRead, oldWrite := readPersistedSession, writePersistedSession
	stored, _ := json.Marshal(Session{MusicID: "10001", MusicKey: "expired-key", LoginType: 1, CreatedAt: 1, ExpiresIn: 1, RefreshKey: "refresh", Device: &wechatDevice{Qimei: "q16", Qimei36: "q36"}})
	readPersistedSession = func() ([]byte, error) { return stored, nil }
	writePersistedSession = func(b []byte) error { stored = bytes.Clone(b); return nil }
	t.Cleanup(func() { readPersistedSession, writePersistedSession = oldRead, oldWrite })
	if !RestoreSession() {
		t.Fatal("refreshable expired credential was discarded")
	}
	p, err := Account()
	if err != nil || p == nil || p.Nickname != "renewed" {
		t.Fatalf("expired key renewal: %v %v", p, err)
	}
	var renewed Session
	_ = json.Unmarshal(stored, &renewed)
	if !renewed.valid() || renewed.MusicKey != "renewed-key" || renewed.RefreshKey != "rotated-refresh" {
		t.Fatal("rotated credential was not saved")
	}
	readPersistedSession = func() ([]byte, error) {
		return []byte(`{"musicid":"10001","musickey":"old","loginType":2,"musickeyCreateTime":1,"keyExpiresIn":1,"refresh_key":"ignored"}`), nil
	}
	if RestoreSession() {
		t.Fatal("QQ restore behavior unexpectedly changed")
	}
}
