package qqmusic

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/network"
)

// Session is QQ Music's own credential, stored only in this native process and
// the operating system credential store. QQ's global login cookies are transient.
type Session struct {
	MusicID      string        `json:"musicid"`
	MusicKey     string        `json:"musickey"`
	LoginType    int           `json:"loginType"`
	CreatedAt    int64         `json:"musickeyCreateTime,omitempty"`
	ExpiresIn    int64         `json:"keyExpiresIn,omitempty"`
	Nickname     string        `json:"nick,omitempty"`
	AvatarURL    string        `json:"logo,omitempty"`
	EncryptUin   string        `json:"encryptUin,omitempty"`
	OpenID       string        `json:"openid,omitempty"`
	RefreshToken string        `json:"refresh_token,omitempty"`
	UnionID      string        `json:"unionid,omitempty"`
	RefreshKey   string        `json:"refresh_key,omitempty"`
	Device       *wechatDevice `json:"device,omitempty"`
}

var authState struct {
	sync.RWMutex
	session Session
	qr      map[string]qrLogin
}

type qrLogin struct {
	image   []byte
	expires time.Time
	wechat  *wechatQR
}

var authHTTP = newAuthHTTPClient()

func newAuthHTTPClient() *http.Client {
	transport := network.Transport()
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func sessionSnapshot() Session {
	authState.RLock()
	defer authState.RUnlock()
	return authState.session
}
func setSession(s Session) { authState.Lock(); authState.session = s; authState.Unlock() }
func (s Session) valid() bool {
	if s.MusicKey == "" || s.MusicID == "" || (s.LoginType != 2 && s.LoginType != 1) {
		return false
	}
	id, err := strconv.ParseInt(s.MusicID, 10, 64)
	return err == nil && id > 0 && (s.CreatedAt <= 0 || s.ExpiresIn <= 0 || time.Now().Unix() < s.CreatedAt+s.ExpiresIn)
}

func (s Session) canRefresh() bool {
	id, err := strconv.ParseInt(s.MusicID, 10, 64)
	return s.LoginType == 1 && err == nil && id > 0 && s.MusicKey != "" && (s.RefreshKey != "" || s.RefreshToken != "")
}

const qqSessionSecret = "qqmusic-session"

func authSessionFile() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "qqmusic-session"), nil
}

var readPersistedSession = func() ([]byte, error) {
	var b []byte
	var err error
	if mygo.IsDev() {
		var path string
		path, err = authSessionFile()
		if err == nil {
			b, err = os.ReadFile(path)
		}
	} else {
		b, err = mygo.Secrets.Get(qqSessionSecret)
	}
	return b, err
}

// RestoreSession also retains an expired WeChat credential when it can refresh.
func RestoreSession() bool {
	b, err := readPersistedSession()
	var s Session
	if err != nil || json.Unmarshal(b, &s) != nil || (!s.valid() && !s.canRefresh()) {
		return false
	}
	setSession(s)
	return true
}

func SaveSession() error {
	s := sessionSnapshot()
	if !s.valid() {
		return errors.New("qqmusic: no valid session")
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return writePersistedSession(b)
}

var writePersistedSession = func(b []byte) error {
	if !mygo.IsDev() {
		return mygo.Secrets.Set(qqSessionSecret, b)
	}
	path, err := authSessionFile()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

func ClearSession() {
	setSession(Session{})
	deletePersistedSession()
}

var deletePersistedSession = func() {
	if !mygo.IsDev() {
		mygo.Secrets.Delete(qqSessionSecret)
		return
	}
	if path, err := authSessionFile(); err == nil {
		_ = os.Remove(path)
	}
}

func Logout() { ClearSession() }

// Account uses the credential's ID: GetLoginUserInfo does not return one.
func Account() (*Profile, error) {
	s := sessionSnapshot()
	if !s.valid() {
		if !s.canRefresh() {
			return nil, nil
		}
		var err error
		s, err = refreshWechatSession(s)
		if err != nil {
			return nil, err
		}
		setSession(s)
		if err = SaveSession(); err != nil {
			return nil, err
		}
	}
	var data struct {
		Info map[string]json.RawMessage `json:"info"`
	}
	err := rpc("music.UserInfo.userInfoServer", "GetLoginUserInfo", map[string]any{}, &data)
	var rejected *ProtocolError
	if s.LoginType == 1 && errors.As(err, &rejected) && rejected.GlobalCode == 0 && (rejected.Code == 1000 || rejected.Code == 104400 || rejected.Code == 104401) {
		s, err = refreshWechatSession(s)
		if err == nil {
			setSession(s)
			if err = SaveSession(); err != nil {
				return nil, err
			}
			err = rpc("music.UserInfo.userInfoServer", "GetLoginUserInfo", map[string]any{}, &data)
		}
	}
	if err != nil {
		var rejected *ProtocolError
		if errors.As(err, &rejected) && rejected.GlobalCode == 0 && (rejected.Code == 1000 || rejected.Code == 104400 || rejected.Code == 104401) {
			ClearSession()
			return nil, nil
		}
		return nil, err
	}
	id, _ := strconv.ParseInt(s.MusicID, 10, 64)
	p := &Profile{UserID: id, Nickname: s.Nickname, AvatarURL: s.AvatarURL}
	for _, key := range []string{"nick", "nickname"} {
		var v string
		if json.Unmarshal(data.Info[key], &v) == nil && v != "" {
			p.Nickname = v
			break
		}
	}
	for _, key := range []string{"headurl", "logo", "avatarUrl", "avatar"} {
		var v string
		if json.Unmarshal(data.Info[key], &v) == nil && v != "" {
			p.AvatarURL = v
			break
		}
	}
	if p.Nickname == "" {
		p.Nickname = s.MusicID
	}
	return p, nil
}

const (
	QRExpired   = 800
	QRWaiting   = 801
	QRScanned   = 802
	QRConfirmed = 803
)

const loginJump = "https://graph.qq.com/oauth2.0/login_jump"

func authSend(method, raw string, body io.Reader, headers http.Header) (*http.Response, error) {
	step := loginStep(raw)
	req, err := http.NewRequest(method, raw, body)
	if err != nil {
		return nil, loginFailure(step, err)
	}
	req.Header = headers
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0")
	}
	res, err := doAuthRequest(req)
	if err != nil {
		return nil, loginFailure(step, err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 400 {
		res.Body.Close()
		return nil, loginFailure(step, loginHTTPError(res.StatusCode))
	}
	return res, nil
}

func authBytes(res *http.Response, limit int64) ([]byte, error) {
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err == nil && int64(len(b)) > limit {
		err = errors.New("qqmusic: oversized login response")
	}
	return b, err
}
func hash33(s string, initial uint32) uint32 {
	h := initial
	for _, c := range s {
		h = h*33 + uint32(c)
	}
	return h & 0x7fffffff
}
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
func trustedLoginURL(raw, host, path string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != host || u.Path != path || u.User != nil || u.Fragment != "" {
		return nil, errors.New("qqmusic: invalid login redirect")
	}
	return u, nil
}
func authCookie(res *http.Response, name, domain string) string {
	var value string
	for _, c := range res.Cookies() {
		if c.Name == name && strings.TrimPrefix(strings.ToLower(c.Domain), ".") == domain {
			value = c.Value
			if c.MaxAge < 0 {
				value = ""
			}
		}
	}
	return value
}

// QRKey retrieves QQ's original PNG. Re-encoding a web URL cannot reproduce it.
func QRKey() (key string, err error) {
	defer func() { err = loginFailure("qr-show", err) }()
	nonce, err := randomHex(8)
	if err != nil {
		return "", err
	}
	q := url.Values{"appid": {"716027609"}, "e": {"2"}, "l": {"M"}, "s": {"3"}, "d": {"72"}, "v": {"4"}, "t": {nonce}, "daid": {"383"}, "pt_3rd_aid": {"100497308"}, "u1": {loginJump}}
	res, err := authSend("GET", "https://xui.ptlogin2.qq.com/ssl/ptqrshow?"+q.Encode(), nil, nil)
	if err != nil {
		return "", err
	}
	key = authCookie(res, "qrsig", "ptlogin2.qq.com")
	b, err := authBytes(res, 64*1024)
	if err != nil {
		return "", err
	}
	if res.StatusCode != 200 || key == "" {
		return "", &loginResponseError{Kind: "qr-signature"}
	}
	if _, err = png.DecodeConfig(bytes.NewReader(b)); err != nil {
		return "", &loginResponseError{Kind: "qr-image"}
	}
	authState.Lock()
	defer authState.Unlock()
	if authState.qr == nil {
		authState.qr = make(map[string]qrLogin)
	}
	for k, v := range authState.qr {
		if time.Now().After(v.expires) {
			delete(authState.qr, k)
		}
	}
	authState.qr[key] = qrLogin{image: b, expires: time.Now().Add(120 * time.Second)}
	return key, nil
}

func QRImage(key string) ([]byte, error) {
	authState.RLock()
	defer authState.RUnlock()
	q, ok := authState.qr[key]
	if !ok {
		return nil, &loginResponseError{Kind: "qr-key"}
	}
	return bytes.Clone(q.image), nil
}

// CancelQR releases a closed or replaced login so its result cannot sign in later.
func CancelQR(key string) {
	authState.Lock()
	defer authState.Unlock()
	if qr, ok := authState.qr[key]; ok && qr.wechat != nil {
		qr.wechat.cancel()
	}
	delete(authState.qr, key)
}

func finishQR(key string, s Session) bool {
	authState.Lock()
	defer authState.Unlock()
	if _, ok := authState.qr[key]; !ok {
		return false
	}
	authState.session = s
	delete(authState.qr, key)
	return true
}

// JSONP is parsed as data; no JavaScript runtime is part of the client.
var ptuiString = regexp.MustCompile(`^'((?:[^'\\\r\n]|\\(?:['\\nrt]|x[0-9a-fA-F]{2}|u[0-9a-fA-F]{4}))*)'\s*`)

func parsePtui(body string) ([]string, error) {
	body = strings.TrimSpace(body)
	body = strings.TrimSuffix(body, ";")
	if !strings.HasPrefix(body, "ptuiCB(") || !strings.HasSuffix(body, ")") {
		return nil, errors.New("qqmusic: malformed QR status")
	}
	rest := strings.TrimSpace(body[7 : len(body)-1])
	var values []string
	for rest != "" {
		m := ptuiString.FindStringSubmatch(rest)
		if m == nil {
			return nil, errors.New("qqmusic: malformed QR status")
		}
		var value strings.Builder
		for i := 0; i < len(m[1]); i++ {
			if m[1][i] != '\\' {
				value.WriteByte(m[1][i])
				continue
			}
			i++
			switch m[1][i] {
			case 'n':
				value.WriteByte('\n')
			case 'r':
				value.WriteByte('\r')
			case 't':
				value.WriteByte('\t')
			case 'x', 'u':
				n := 2
				if m[1][i] == 'u' {
					n = 4
				}
				v, _ := strconv.ParseUint(m[1][i+1:i+1+n], 16, 16)
				value.WriteRune(rune(v))
				i += n
			default:
				value.WriteByte(m[1][i])
			}
		}
		values = append(values, value.String())
		rest = strings.TrimSpace(rest[len(m[0]):])
		if rest == "" {
			break
		}
		if rest[0] != ',' || len(values) >= 7 {
			return nil, errors.New("qqmusic: malformed QR status")
		}
		rest = strings.TrimSpace(rest[1:])
		if rest == "" {
			return nil, errors.New("qqmusic: malformed QR status")
		}
	}
	if len(values) < 6 || len(values) > 7 {
		return nil, errors.New("qqmusic: malformed QR status")
	}
	return values, nil
}

func QRCheck(key string) (code int, msg, nickname, avatar string, err error) {
	authState.RLock()
	qr, ok := authState.qr[key]
	authState.RUnlock()
	if !ok || time.Now().After(qr.expires) {
		return QRExpired, "二维码已失效", "", "", nil
	}
	if qr.wechat != nil {
		return checkWechatQR(key, qr)
	}
	q := url.Values{"u1": {loginJump}, "from_ui": {"1"}, "aid": {"716027609"}, "daid": {"383"}, "pt_3rd_aid": {"100497308"}, "ptqrtoken": {strconv.FormatUint(uint64(hash33(key, 0)), 10)}}
	res, err := authSend("GET", "https://xui.ptlogin2.qq.com/ssl/ptqrlogin?"+q.Encode(), nil, http.Header{"Cookie": {"qrsig=" + key}})
	if err != nil {
		return 0, "", "", "", err
	}
	b, err := authBytes(res, 16384)
	if err != nil {
		return 0, "", "", "", err
	}
	args, err := parsePtui(string(b))
	if err != nil {
		return 0, "", "", "", err
	}
	switch args[0] {
	case "66":
		return QRWaiting, "", "", "", nil
	case "67":
		return QRScanned, "请在手机 QQ 上确认登录", "", "", nil
	case "65":
		return QRExpired, "二维码已失效", "", "", nil
	case "0":
	default:
		return 0, args[4], "", "", fmt.Errorf("qqmusic: QR status %s", args[0])
	}
	s, err := exchangePtlogin(args[2])
	if err != nil {
		return 0, "", "", "", err
	}
	if !finishQR(key, s) {
		return QRExpired, "二维码已失效", "", "", nil
	}
	return QRConfirmed, "登录成功", s.Nickname, s.AvatarURL, nil
}

func exchangePtlogin(raw string) (session Session, err error) {
	step := "check-sig"
	defer func() { err = loginFailure(step, err) }()
	empty := Session{}
	u, err := trustedLoginURL(raw, "ssl.ptlogin2.graph.qq.com", "/check_sig")
	if err != nil {
		return empty, err
	}
	res, err := authSend("GET", u.String(), nil, nil)
	if err != nil {
		return empty, err
	}
	res.Body.Close()
	if res.StatusCode != 302 {
		return empty, errors.New("qqmusic: check_sig did not redirect")
	}
	jump, err := trustedLoginURL(res.Header.Get("Location"), "graph.qq.com", "/oauth2.0/login_jump")
	if err != nil {
		return empty, err
	}
	var cookies []string
	var skey string
	for _, name := range []string{"p_uin", "p_skey", "pt_oauth_token"} {
		v := authCookie(res, name, "graph.qq.com")
		if v == "" {
			return empty, errors.New("qqmusic: incomplete QQ authorization cookies")
		}
		cookies = append(cookies, name+"="+v)
		if name == "p_skey" {
			skey = v
		}
	}
	step = "login-jump"
	res, err = authSend("GET", jump.String(), nil, nil)
	if err != nil {
		return empty, err
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		return empty, errors.New("qqmusic: login_jump failed")
	}
	state, err := randomHex(16)
	if err != nil {
		return empty, err
	}
	uiID, err := randomHex(16)
	if err != nil {
		return empty, err
	}
	uiID = uiID[:8] + "-" + uiID[8:12] + "-" + uiID[12:16] + "-" + uiID[16:20] + "-" + uiID[20:]
	form := url.Values{"response_type": {"code"}, "client_id": {"100497308"}, "redirect_uri": {"https://y.qq.com/portal/wx_redirect.html?login_type=1&surl=https://y.qq.com/"}, "scope": {"get_user_info,get_app_friends"}, "state": {state}, "switch": {""}, "from_ptlogin": {"1"}, "src": {"1"}, "update_auth": {"1"}, "openapi": {"1010_1030"}, "g_tk": {strconv.FormatUint(uint64(hash33(skey, 5381)), 10)}, "auth_time": {strconv.FormatInt(time.Now().UnixMilli(), 10)}, "ui": {uiID}}
	step = "authorize"
	res, err = authSend("POST", "https://graph.qq.com/oauth2.0/authorize", strings.NewReader(form.Encode()), http.Header{"Content-Type": {"application/x-www-form-urlencoded"}, "Cookie": {strings.Join(cookies, "; ")}})
	if err != nil {
		return empty, err
	}
	res.Body.Close()
	if res.StatusCode != 302 {
		return empty, errors.New("qqmusic: QQ authorization did not redirect")
	}
	back, err := trustedLoginURL(res.Header.Get("Location"), "y.qq.com", "/portal/wx_redirect.html")
	if err != nil {
		return empty, err
	}
	codes, states := back.Query()["code"], back.Query()["state"]
	if len(codes) != 1 || !regexp.MustCompile(`^[A-F0-9]{32}$`).MatchString(codes[0]) || len(states) != 1 || states[0] != state {
		return empty, errors.New("qqmusic: invalid QQ authorization state or code")
	}
	body, _ := json.Marshal(map[string]any{"comm": map[string]any{"g_tk": 5381, "platform": "yqq", "ct": 24, "cv": 0}, "req": map[string]any{"module": "QQConnectLogin.LoginServer", "method": "QQLogin", "param": map[string]any{"code": codes[0]}}})
	step = "qq-login"
	res, err = authSend("POST", "https://u.y.qq.com/cgi-bin/musicu.fcg", bytes.NewReader(body), http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
	if err != nil {
		return empty, err
	}
	b, err := authBytes(res, 16384)
	if err != nil {
		return empty, err
	}
	return decodeQQLoginResponse(b)
}
