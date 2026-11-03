package qqmusic

import (
	"encoding/json"
	"strconv"
	"strings"
)

type loginResponseError struct {
	Kind                         string
	Gateway, Business, LoginType int
}

func (e *loginResponseError) Error() string { return e.message() }
func (e *loginResponseError) message() string {
	switch e.Kind {
	case "qr-signature":
		return "QQ 未返回二维码签名（Q201），请刷新重试"
	case "qr-image":
		return "登录服务返回的二维码图片无效（Q202），请刷新重试"
	case "qr-key":
		return "二维码已失效（Q203），请刷新重试"
	case "rejected":
		return "QQ 音乐拒绝登录（服务码 " + strconv.Itoa(e.Gateway) + "/" + strconv.Itoa(e.Business) + "）"
	case "json":
		return "QQ 音乐登录响应无法解析（L201）"
	case "status":
		return "QQ 音乐未返回登录状态（L202）"
	case "data":
		return "QQ 音乐未返回登录数据（L203）"
	case "id":
		return "QQ 音乐返回的账号标识无效（L204）"
	case "key":
		return "QQ 音乐未返回登录凭据（L205）"
	case "type":
		return "QQ 音乐返回未支持的登录类型（" + strconv.Itoa(e.LoginType) + "）"
	case "expired":
		return "QQ 音乐返回的登录凭据已过期，请重新扫码"
	}
	return "QQ 音乐登录响应无效（L200）"
}

// The reference accepts numeric strings for optional expiry metadata; optional
// fields of an unavailable type do not invalidate the actual login credential.
func loginOptionalInt(raw json.RawMessage) int64 {
	var value json.Number
	if json.Unmarshal(raw, &value) == nil {
		if n, err := value.Int64(); err == nil {
			return n
		}
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		n, _ := strconv.ParseInt(text, 10, 64)
		return n
	}
	return 0
}

func decodeQQLoginResponse(body []byte) (Session, error) {
	return decodeLoginResponse(body, 2)
}

func decodeLoginResponse(body []byte, expectedType int) (Session, error) {
	var result struct {
		Code *int `json:"code"`
		Req  *struct {
			Code *int            `json:"code"`
			Data json.RawMessage `json:"data"`
		} `json:"req"`
	}
	if json.Unmarshal(body, &result) != nil {
		return Session{}, &loginResponseError{Kind: "json"}
	}
	if result.Code == nil || result.Req == nil || result.Req.Code == nil {
		return Session{}, &loginResponseError{Kind: "status"}
	}
	if *result.Code != 0 || *result.Req.Code != 0 {
		return Session{}, &loginResponseError{Kind: "rejected", Gateway: *result.Code, Business: *result.Req.Code}
	}
	var data object
	if json.Unmarshal(result.Req.Data, &data) != nil || data == nil {
		return Session{}, &loginResponseError{Kind: "data"}
	}
	id := ""
	if expectedType == 1 {
		id = text(data, "str_musicid")
	}
	if id == "" {
		id = text(data, "musicid")
	}
	if id == "" {
		id = strings.TrimSpace(string(data["musicid"]))
	}
	idNumber, idErr := strconv.ParseInt(id, 10, 64)
	if idErr != nil || idNumber <= 0 {
		return Session{}, &loginResponseError{Kind: "id"}
	}
	key := text(data, "musickey")
	if key == "" {
		return Session{}, &loginResponseError{Kind: "key"}
	}
	loginType := int(loginOptionalInt(data["loginType"]))
	if loginType == 0 && expectedType == 1 {
		loginType = 1
	}
	if loginType != expectedType {
		return Session{}, &loginResponseError{Kind: "type", LoginType: loginType}
	}
	s := Session{MusicID: id, MusicKey: key, LoginType: loginType, CreatedAt: loginOptionalInt(data["musickeyCreateTime"]), ExpiresIn: loginOptionalInt(data["keyExpiresIn"]), Nickname: text(data, "nick"), AvatarURL: text(data, "logo"), EncryptUin: text(data, "encryptUin")}
	s.OpenID, s.RefreshToken, s.UnionID, s.RefreshKey = text(data, "openid"), text(data, "refresh_token"), text(data, "unionid"), text(data, "refresh_key")
	if !s.valid() {
		return Session{}, &loginResponseError{Kind: "expired"}
	}
	return s, nil
}
