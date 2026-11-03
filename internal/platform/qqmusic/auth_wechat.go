package qqmusic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"
)

const wechatAppID = "wx48db31d50e334801"
const wechatConnect = "https://open.weixin.qq.com/connect/qrconnect"

type wechatQR struct {
	client  *http.Client
	uuid    string
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	scanned bool
	device  *wechatDevice
}

var wechatUUID = regexp.MustCompile(`(?:uuid=|/connect/qrcode/)([A-Za-z0-9_-]+)`)
var wechatStatus = regexp.MustCompile(`window\.wx_errcode\s*=\s*(\d+)\s*;\s*window\.wx_code\s*=\s*'([^'\r\n]*)'`)

func wechatSend(qr *wechatQR, raw, referer string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(qr.ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	res, err := qr.client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, loginHTTPError(res.StatusCode)
	}
	return authBytes(res, limit)
}

// WechatQRKey keeps the web cookie jar private to one QR login.
func WechatQRKey() (key string, err error) {
	defer func() { err = loginFailure("qr-show", err) }()
	device, err := bootstrapWechatDevice()
	if err != nil {
		return "", err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", err
	}
	client := *authHTTP
	client.Jar, client.Timeout = jar, 20*time.Second
	ctx, cancel := context.WithCancel(context.Background())
	qr := &wechatQR{client: &client, ctx: ctx, cancel: cancel, device: device}
	defer func() {
		if err != nil {
			cancel()
		}
	}()
	q := url.Values{"appid": {wechatAppID}, "redirect_uri": {"https://y.qq.com/portal/wx_redirect.html?login_type=2&surl=https://y.qq.com/"}, "response_type": {"code"}, "scope": {"snsapi_login"}, "state": {"STATE"}, "href": {"https://y.qq.com/mediastyle/music_v17/src/css/popup_wechat.css#wechat_redirect"}}
	page, err := wechatSend(qr, wechatConnect+"?"+q.Encode(), "", 256<<10)
	if err != nil {
		return "", err
	}
	match := wechatUUID.FindSubmatch(page)
	if match == nil {
		return "", errors.New("qqmusic: missing WeChat QR identifier")
	}
	qr.uuid = string(match[1])
	b, err := wechatSend(qr, "https://open.weixin.qq.com/connect/qrcode/"+qr.uuid, wechatConnect, 64<<10)
	if err != nil {
		return "", err
	}
	if _, _, err = image.DecodeConfig(bytes.NewReader(b)); err != nil {
		return "", &loginResponseError{Kind: "qr-image"}
	}
	key, err = randomHex(24)
	if err != nil {
		return "", err
	}
	authState.Lock()
	defer authState.Unlock()
	if authState.qr == nil {
		authState.qr = make(map[string]qrLogin)
	}
	for old, login := range authState.qr {
		if time.Now().After(login.expires) {
			if login.wechat != nil {
				login.wechat.cancel()
			}
			delete(authState.qr, old)
		}
	}
	authState.qr[key] = qrLogin{image: b, expires: time.Now().Add(120 * time.Second), wechat: qr}
	return key, nil
}

func checkWechatQR(key string, login qrLogin) (code int, msg, nickname, avatar string, err error) {
	defer func() { err = loginFailure("qr-poll", err) }()
	qr := login.wechat
	qr.mu.Lock()
	defer qr.mu.Unlock()
	q := url.Values{"uuid": {qr.uuid}, "_": {strconv.FormatInt(time.Now().UnixMilli(), 10)}}
	if qr.scanned {
		q.Set("last", "404")
	}
	b, err := wechatSend(qr, "https://lp.open.weixin.qq.com/connect/l/qrconnect?"+q.Encode(), "https://open.weixin.qq.com/", 16384)
	if err != nil {
		// An unchanged QR may hold the poll open past the transport deadline.
		var network interface{ Timeout() bool }
		if errors.As(err, &network) && network.Timeout() {
			if qr.scanned {
				return QRScanned, "请在微信上确认登录", "", "", nil
			}
			return QRWaiting, "", "", "", nil
		}
		return 0, "", "", "", err
	}
	match := wechatStatus.FindSubmatch(b)
	if match == nil {
		return 0, "", "", "", errors.New("qqmusic: malformed WeChat QR status")
	}
	switch string(match[1]) {
	case "408":
		if qr.scanned {
			return QRScanned, "请在微信上确认登录", "", "", nil
		}
		return QRWaiting, "", "", "", nil
	case "404":
		qr.scanned = true
		return QRScanned, "请在微信上确认登录", "", "", nil
	case "402", "403":
		CancelQR(key)
		return QRExpired, "二维码已失效", "", "", nil
	case "405":
		if len(match[2]) == 0 {
			return 0, "", "", "", errors.New("qqmusic: missing WeChat authorization code")
		}
		s, err := exchangeWechatSession(map[string]any{"code": string(match[2]), "strAppid": wechatAppID}, nil, qr.device)
		if err != nil {
			return 0, "", "", "", err
		}
		if !finishQR(key, s) {
			return QRExpired, "二维码已失效", "", "", nil
		}
		qr.cancel()
		return QRConfirmed, "登录成功", s.Nickname, s.AvatarURL, nil
	default:
		return 0, "", "", "", fmt.Errorf("qqmusic: WeChat QR status %s", match[1])
	}
}

func exchangeWechatSession(param map[string]any, prior *Session, device *wechatDevice) (session Session, err error) {
	defer func() { err = loginFailure("qq-login", err) }()
	comm := device.comm()
	comm["tmeLoginType"] = 1
	if prior != nil {
		comm["qq"], comm["authst"] = prior.MusicID, prior.MusicKey
	}
	body, err := json.Marshal(map[string]any{"comm": comm, "req": map[string]any{"module": "music.login.LoginServer", "method": "Login", "param": param}})
	if err != nil {
		return Session{}, err
	}
	headers := http.Header{"Content-Type": {"application/json"}, "User-Agent": {UserAgent}, "Referer": {"https://y.qq.com/"}}
	if prior != nil {
		headers.Set("Cookie", "uin="+prior.MusicID+"; qqmusic_uin="+prior.MusicID+"; qm_keyst="+prior.MusicKey+"; qqmusic_key="+prior.MusicKey)
	}
	res, err := authSend(http.MethodPost, musicuURL, bytes.NewReader(body), headers)
	if err != nil {
		return Session{}, err
	}
	b, err := authBytes(res, 16384)
	if err != nil {
		return Session{}, err
	}
	s, err := decodeLoginResponse(b, 1)
	s.Device = device
	return s, err
}

func refreshWechatSession(s Session) (Session, error) {
	device := s.Device
	if device == nil {
		var err error
		device, err = bootstrapWechatDevice()
		if err != nil {
			return Session{}, err
		}
	}
	refreshed, err := exchangeWechatSession(map[string]any{"openid": s.OpenID, "refresh_token": s.RefreshToken, "str_musicid": s.MusicID, "musickey": s.MusicKey, "unionid": s.UnionID, "refresh_key": s.RefreshKey, "loginMode": 2}, &s, device)
	if err != nil {
		return Session{}, err
	}
	if refreshed.OpenID == "" {
		refreshed.OpenID = s.OpenID
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = s.RefreshToken
	}
	if refreshed.UnionID == "" {
		refreshed.UnionID = s.UnionID
	}
	if refreshed.RefreshKey == "" {
		refreshed.RefreshKey = s.RefreshKey
	}
	if refreshed.Nickname == "" {
		refreshed.Nickname = s.Nickname
	}
	if refreshed.AvatarURL == "" {
		refreshed.AvatarURL = s.AvatarURL
	}
	if refreshed.EncryptUin == "" {
		refreshed.EncryptUin = s.EncryptUin
	}
	return refreshed, nil
}
