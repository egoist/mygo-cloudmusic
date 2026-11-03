package qqmusic

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

type authRoundTripper func(*http.Request) (*http.Response, error)

func (fn authRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func fakeAuthServer(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	base, _ := url.Parse(server.URL)
	old := authHTTP
	authHTTP = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: authRoundTripper(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		u := *r.URL
		clone.URL = &u
		clone.Host = r.URL.Host
		clone.URL.Scheme = base.Scheme
		clone.URL.Host = base.Host
		return http.DefaultTransport.RoundTrip(clone)
	})}
	authState.Lock()
	prior := authState.session
	priorQR := authState.qr
	authState.session = Session{}
	authState.qr = nil
	authState.Unlock()
	t.Cleanup(func() {
		authHTTP = old
		authState.Lock()
		authState.session = prior
		authState.qr = priorQR
		authState.Unlock()
	})
}

// A failed fetch must not poison later QR creation or return the previous QR's bytes.
func TestQRRefreshAfterFetchFailure(t *testing.T) {
	var pngBytes bytes.Buffer
	_ = png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 37, 37)))
	requests := 0
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "qrsig", Value: fmt.Sprintf("fresh-sig-%d", requests), Domain: "ptlogin2.qq.com", Path: "/"})
		_, _ = w.Write(pngBytes.Bytes())
	})
	if _, err := QRKey(); err == nil {
		t.Fatal("failed request looked successful")
	}
	var previous string
	for i := 0; i < 2; i++ {
		key, err := QRKey()
		if err != nil {
			t.Fatal(err)
		}
		if key == previous {
			t.Fatal("refresh reused the old QR key")
		}
		image, err := QRImage(key)
		if err != nil || !bytes.Equal(image, pngBytes.Bytes()) {
			t.Fatal("fresh QR image was not retained")
		}
		previous = key
	}
}

// QQ's PNG and qrsig must survive the complete OAuth exchange without leaking global cookies to musicu.
func TestQQQRLogin(t *testing.T) {
	const sig = "fake-qrsig-value_01"
	const oauthCode = "ABCDEF0123456789ABCDEF0123456789"
	var imageBytes bytes.Buffer
	_ = png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 37, 37)))
	polls := 0
	exchanges := 0
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Host + r.URL.Path {
		case "xui.ptlogin2.qq.com/ssl/ptqrshow":
			if r.URL.Query().Get("appid") != "716027609" || r.URL.Query().Get("pt_3rd_aid") != "100497308" {
				t.Error("incorrect QQ application IDs")
			}
			http.SetCookie(w, &http.Cookie{Name: "qrsig", Value: sig, Domain: "ptlogin2.qq.com", Path: "/"})
			w.Write(imageBytes.Bytes())
		case "xui.ptlogin2.qq.com/ssl/ptqrlogin":
			if r.Header.Get("Cookie") != "qrsig="+sig || r.URL.Query().Get("ptqrtoken") != strconv.FormatUint(uint64(hash33(sig, 0)), 10) {
				t.Error("poll lost QR authentication")
			}
			polls++
			switch polls {
			case 1:
				io.WriteString(w, `ptuiCB('66','0','','0','waiting','')`)
			case 2:
				io.WriteString(w, `ptuiCB('67','0','','0','scanned','')`)
			default:
				io.WriteString(w, `ptuiCB('0','0','https://ssl.ptlogin2.graph.qq.com/check_sig?ptsigx=fake','0','ok','nick')`)
			}
		case "ssl.ptlogin2.graph.qq.com/check_sig":
			for _, c := range []http.Cookie{{Name: "p_uin", Value: "o0010001"}, {Name: "p_skey", Value: "fake-pskey"}, {Name: "pt_oauth_token", Value: "fake-oauth"}} {
				c.Domain = "graph.qq.com"
				c.Path = "/"
				http.SetCookie(w, &c)
			}
			w.Header().Set("Location", loginJump)
			w.WriteHeader(302)
		case "graph.qq.com/oauth2.0/login_jump":
			w.WriteHeader(200)
		case "graph.qq.com/oauth2.0/authorize":
			if r.Method != "POST" {
				t.Error("authorization is not POST")
			}
			_ = r.ParseForm()
			if r.Form.Get("g_tk") != strconv.FormatUint(uint64(hash33("fake-pskey", 5381)), 10) || !strings.Contains(r.Header.Get("Cookie"), "p_skey=fake-pskey") {
				t.Error("lost graph authentication")
			}
			if len(r.Form.Get("state")) != 32 {
				t.Error("state is missing")
			}
			w.Header().Set("Location", "https://y.qq.com/portal/wx_redirect.html?code="+oauthCode+"&state="+r.Form.Get("state"))
			w.WriteHeader(302)
		case "u.y.qq.com/cgi-bin/musicu.fcg":
			exchanges++
			if r.Header.Get("Cookie") != "" {
				t.Error("global QQ cookies leaked to musicu")
			}
			var body struct {
				Req struct {
					Module string `json:"module"`
					Method string `json:"method"`
					Param  struct {
						Code string `json:"code"`
					} `json:"param"`
				} `json:"req"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Req.Module != "QQConnectLogin.LoginServer" || body.Req.Method != "QQLogin" || body.Req.Param.Code != oauthCode {
				t.Errorf("incorrect credential request: %+v", body)
			}
			io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":10001,"musickey":"fake-musickey","loginType":2,"nick":"Listener","logo":"https://example.test/avatar"}}}`)
		default:
			t.Errorf("unexpected request %s%s", r.Host, r.URL)
			w.WriteHeader(500)
		}
	})
	key, err := QRKey()
	if err != nil {
		t.Fatal(err)
	}
	b, err := QRImage(key)
	if err != nil || !bytes.Equal(b, imageBytes.Bytes()) {
		t.Fatalf("lost original QR PNG: %v", err)
	}
	for _, want := range []int{QRWaiting, QRScanned, QRConfirmed} {
		code, _, _, _, err := QRCheck(key)
		if err != nil || code != want {
			t.Fatalf("QRCheck = %d, %v; want %d", code, err, want)
		}
	}
	if exchanges != 1 {
		t.Fatalf("credential exchanges = %d", exchanges)
	}
	s := sessionSnapshot()
	if s.MusicID != "10001" || s.MusicKey != "fake-musickey" || s.LoginType != 2 {
		t.Fatalf("bad session: id=%s type=%d", s.MusicID, s.LoginType)
	}
}

// An OAuth callback with the wrong state must never be exchanged for a music credential.
func TestQQLoginRejectsWrongState(t *testing.T) {
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/check_sig":
			for _, name := range []string{"p_uin", "p_skey", "pt_oauth_token"} {
				http.SetCookie(w, &http.Cookie{Name: name, Value: "fake", Domain: "graph.qq.com"})
			}
			w.Header().Set("Location", loginJump)
			w.WriteHeader(302)
		case "/oauth2.0/login_jump":
			w.WriteHeader(200)
		case "/oauth2.0/authorize":
			w.Header().Set("Location", "https://y.qq.com/portal/wx_redirect.html?code=ABCDEF0123456789ABCDEF0123456789&state=wrong")
			w.WriteHeader(302)
		default:
			t.Error("unexpected credential request after state mismatch")
			w.WriteHeader(500)
		}
	})
	if _, err := exchangePtlogin("https://ssl.ptlogin2.graph.qq.com/check_sig"); err == nil {
		t.Fatal("accepted wrong OAuth state")
	}
}

// Redirects outside the QQ authorization flow and executable JSONP are rejected as data.
func TestQQLoginRejectsInvalidProtocol(t *testing.T) {
	for _, body := range []string{`alert(1)`, `ptuiCB('66')`, `ptuiCB('66','0','','0','waiting','');alert(1)`, `ptuiCB('66','0','','0','waiting','',)`} {
		if _, err := parsePtui(body); err == nil {
			t.Errorf("accepted malformed response %q", body)
		}
	}
	args, err := parsePtui(`ptuiCB('67','0','','0','it\'s scanned','\u4f60\x21');`)
	if err != nil || args[4] != "it's scanned" || args[5] != "你!" {
		t.Fatalf("string escapes = %v %v", args, err)
	}
	for _, raw := range []string{"https://evil.test/check_sig", "http://ssl.ptlogin2.graph.qq.com/check_sig", "https://ssl.ptlogin2.graph.qq.com:443/check_sig"} {
		if _, err := trustedLoginURL(raw, "ssl.ptlogin2.graph.qq.com", "/check_sig"); err == nil {
			t.Error(fmt.Sprintf("accepted redirect %s", raw))
		}
	}
}

// An explicitly rejected restored key must be discarded so anonymous requests do not keep using it.
func TestAccountDiscardsRejectedCredential(t *testing.T) {
	for _, code := range []int{1000, 104400, 104401} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": code, "data": map[string]any{}}})
			})
			oldHTTP := httpClient
			httpClient = authHTTP
			t.Cleanup(func() { httpClient = oldHTTP })
			oldDelete := deletePersistedSession
			deleted := false
			deletePersistedSession = func() { deleted = true }
			t.Cleanup(func() { deletePersistedSession = oldDelete })
			setSession(Session{MusicID: "10001", MusicKey: "rejected", LoginType: 2})
			profile, err := Account()
			if err != nil || profile != nil || sessionSnapshot().MusicKey != "" || !deleted {
				t.Fatalf("rejected account was retained: profile=%v err=%v deleted=%v", profile, err, deleted)
			}
		})
	}
}

// A temporary network failure must preserve the credential for a later retry.
func TestAccountPreservesCredentialOnNetworkFailure(t *testing.T) {
	oldHTTP := httpClient
	oldSession := sessionSnapshot()
	t.Cleanup(func() { httpClient = oldHTTP; setSession(oldSession) })
	httpClient = &http.Client{Transport: authRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("temporary network failure")
	})}
	setSession(Session{MusicID: "10001", MusicKey: "still-valid", LoginType: 2})
	if _, err := Account(); err == nil {
		t.Fatal("network failure was hidden")
	}
	if sessionSnapshot().MusicKey != "still-valid" {
		t.Fatal("network failure discarded the credential")
	}
}
