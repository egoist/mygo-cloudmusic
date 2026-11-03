package qqmusic

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// WeChat uses synthetic Android identifiers; no local hardware is inspected.
type wechatDevice struct {
	AndroidID string `json:"androidId"`
	OpenUDID  string `json:"openUdid"`
	IMEI      string `json:"imei"`
	Qimei     string `json:"qimei"`
	Qimei36   string `json:"qimei36"`
	UID       string `json:"uid"`
	SID       string `json:"sid"`
}

func (d *wechatDevice) comm() map[string]any {
	comm := map[string]any{"ct": 11, "cv": 14090008, "v": 14090008, "chid": "10003505", "tmeAppID": "qqmusic", "QIMEI": d.Qimei, "QIMEI36": d.Qimei36, "OpenUDID": d.OpenUDID, "udid": d.OpenUDID, "OpenUDID2": d.OpenUDID, "aid": d.AndroidID, "os_ver": "10", "phonetype": "MI 6", "devicelevel": "29", "newdevicelevel": "29", "rom": "xiaomi/iarim/sagit:10/eomam.200122.001/2001221:user/release-keys"}
	if d.UID != "" {
		comm["uid"], comm["sid"] = d.UID, d.SID
	}
	return comm
}

func authenticatedComm(s Session) map[string]any {
	comm := map[string]any{"ct": 11, "cv": 14090008, "v": 14090008, "chid": "10003505", "tmeAppID": "qqmusic"}
	if s.Device != nil {
		comm = s.Device.comm()
	}
	comm["qq"], comm["authst"], comm["tmeLoginType"] = s.MusicID, s.MusicKey, s.LoginType
	return comm
}

func deviceDigits(n int) (string, error) {
	var b strings.Builder
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		b.WriteByte('0' + byte(v.Int64()))
	}
	return b.String(), nil
}

func newWechatDevice() (*wechatDevice, error) {
	a, err := randomHex(8)
	if err != nil {
		return nil, err
	}
	u, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	imei, err := deviceDigits(14)
	if err != nil {
		return nil, err
	}
	sum := 0
	for i, c := range imei {
		n := int(c - '0')
		if i%2 == 1 {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
	}
	return &wechatDevice{AndroidID: a, OpenUDID: u, IMEI: imei + strconv.Itoa((10-sum%10)%10)}, nil
}

func protocolMD5(parts ...string) string {
	h := md5.Sum([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(h[:])
}

func qimeiEnvelope(d *wechatDevice, now time.Time) ([]byte, http.Header, error) {
	const appKey = "0AND0HD6FE4HY80F"
	const modulus = "c4231830a2eb5fc2827170641e79d80fec51bda9a22e4b4ab37d1f205a4ae44d928cda25879f66a3429051663312a127faf8a246bdaaf63918417e90d7c95b5908aa6a2d0f852e4a6770294a548ac1c2fe8f1f252fb826f4ac86ab9a00e7ce47d002a56e7c4b51eb889acc60ca6adbc9f72e81f4d31b1dd7464805264530ab1d"
	cryptKey, err := randomHex(8)
	if err != nil {
		return nil, nil, err
	}
	nonce, err := randomHex(8)
	if err != nil {
		return nil, nil, err
	}
	first, err := deviceDigits(6)
	if err != nil {
		return nil, nil, err
	}
	second, err := deviceDigits(9)
	if err != nil {
		return nil, nil, err
	}
	dated := map[int]bool{1: true, 2: true, 13: true, 14: true, 17: true, 18: true, 21: true, 22: true, 25: true, 26: true, 29: true, 30: true, 33: true, 34: true, 37: true, 38: true}
	var beacon strings.Builder
	for i := 1; i <= 40; i++ {
		v := now.UTC().Format("2006-01") + "-01" + first + "." + second
		if i == 3 {
			v = "0000000000000000"
		} else if i == 4 {
			v = strings.ReplaceAll(cryptKey, "0", "1")
		} else if !dated[i] {
			v, err = deviceDigits(4)
			if err != nil {
				return nil, nil, err
			}
		}
		fmt.Fprintf(&beacon, "k%d:%s;", i, v)
	}
	reserved, _ := json.Marshal(map[string]any{"harmony": "0", "clone": "0", "containe": "", "oz": "UhYmelwouA+V2nPWbOvLTgN2/m8jwGB+yUB5v9tysQg=", "oo": "Xecjt+9S1+f8Pz2VLSxgpw==", "kelong": "0", "uptimes": now.UTC().Add(-time.Hour).Format("2006-01-02 15:04:05"), "multiUser": "0", "bod": "Xiaomi", "dv": "sagit", "firstLevel": "", "manufact": "Xiaomi", "name": "MI 6", "host": "se.infra", "kernel": "Linux 5.4.0-54-generic (android-build@google.com)"})
	payload, _ := json.Marshal(map[string]any{"androidId": d.AndroidID, "platformId": 1, "appKey": appKey, "appVersion": "14.9.0.8", "beaconIdSrc": beacon.String(), "brand": "Xiaomi", "channelId": "10003505", "cid": "", "imei": d.IMEI, "imsi": "", "mac": "", "model": "MI 6", "networkType": "unknown", "oaid": "", "osVersion": "Android 10,level 29", "qimei": "", "qimei36": "", "sdkVersion": "1.2.13.6", "targetSdkVersion": "33", "audit": "", "userId": "{}", "packageId": "com.tencent.qqmusic", "deviceType": "Phone", "sdkName": "", "reserved": string(reserved)})
	n, _ := new(big.Int).SetString(modulus, 16)
	wrapped, err := rsa.EncryptPKCS1v15(rand.Reader, &rsa.PublicKey{N: n, E: 65537}, []byte(cryptKey))
	if err != nil {
		return nil, nil, err
	}
	block, err := aes.NewCipher([]byte(cryptKey))
	if err != nil {
		return nil, nil, err
	}
	pad := aes.BlockSize - len(payload)%aes.BlockSize
	payload = append(payload, bytes.Repeat([]byte{byte(pad)}, pad)...)
	ciphertext := make([]byte, len(payload))
	cipher.NewCBCEncrypter(block, []byte(cryptKey)).CryptBlocks(ciphertext, payload)
	key, params := base64.StdEncoding.EncodeToString(wrapped), base64.StdEncoding.EncodeToString(ciphertext)
	ts := strconv.FormatInt(now.Unix(), 10)
	extra := `{"appKey":"` + appKey + `"}`
	body, err := json.Marshal(map[string]any{"app": 0, "os": 1, "qimeiParams": map[string]any{"key": key, "params": params, "time": ts, "nonce": nonce, "sign": protocolMD5(key, params, strconv.FormatInt(now.Unix()*1000, 10), nonce, "ZdJqM15EeO2zWc08", extra), "extra": extra}})
	headers := http.Header{"Method": {"GetQimei"}, "Service": {"trpc.tme_datasvr.qimeiproxy.QimeiProxy"}, "Appid": {"qimei_qq_android"}, "Sign": {protocolMD5("qimei_qq_androidpzAuCmaFAaFaHrdakPjLIEqKrGnSOOvH", ts)}, "Timestamp": {ts}, "Content-Type": {"application/json"}}
	headers.Set("User-Agent", "QQMusic")
	return body, headers, err
}

func embeddedObject(raw json.RawMessage) object {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		raw = []byte(text)
	}
	var obj object
	_ = json.Unmarshal(raw, &obj)
	return obj
}

func bootstrapWechatDevice() (*wechatDevice, error) {
	d, err := newWechatDevice()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for attempt := 0; attempt < 2; attempt++ {
		body, headers, err := qimeiEnvelope(d, now)
		if err != nil {
			return nil, err
		}
		res, err := authSend(http.MethodPost, "https://api.tencentmusic.com/tme/trpc/proxy", bytes.NewReader(body), headers)
		if err != nil {
			return nil, err
		}
		serverTime, _ := http.ParseTime(res.Header.Get("Date"))
		b, err := authBytes(res, 64<<10)
		if err != nil {
			return nil, err
		}
		outer := embeddedObject(b)
		inner := embeddedObject(outer["data"])
		data := embeddedObject(inner["data"])
		d.Qimei, d.Qimei36 = text(data, "q16"), text(data, "q36")
		if d.Qimei != "" && d.Qimei36 != "" {
			break
		}
		if attempt == 0 && !serverTime.IsZero() && now.Sub(serverTime).Abs() > time.Minute {
			now = serverTime
			continue
		}
		return nil, errors.New("qqmusic: device bootstrap did not return QIMEI")
	}
	var data struct {
		Session struct {
			UID json.RawMessage `json:"uid"`
			SID json.RawMessage `json:"sid"`
		} `json:"session"`
	}
	if err := rpcWithComm("music.getSession.session", "GetSession", map[string]any{"uid": "", "vkey": 0, "caller": 0}, &data, d.comm(), map[string]string{"Cookie": ""}); err != nil {
		return nil, loginFailure("device-bootstrap", err)
	}
	value := func(b json.RawMessage) string {
		var s string
		if json.Unmarshal(b, &s) == nil {
			return s
		}
		if len(b) > 0 && string(b) != "null" {
			return string(b)
		}
		return ""
	}
	d.UID, d.SID = value(data.Session.UID), value(data.Session.SID)
	if d.UID == "" || d.SID == "" {
		return nil, errors.New("qqmusic: missing device session")
	}
	return d, nil
}
