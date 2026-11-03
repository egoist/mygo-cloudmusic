package qqmusic

import (
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
)

// LoginError retains the cause for classification without putting temporary
// authorization URLs or credentials in the error's printable representation.
type LoginError struct {
	Step string
	Err  error
}

type loginHTTPError int

func (e loginHTTPError) Error() string { return fmt.Sprintf("HTTP %d", int(e)) }

func (e *LoginError) Error() string { return LoginErrorMessage(e) }
func (e *LoginError) Unwrap() error { return e.Err }

func loginFailure(step string, err error) error {
	if err == nil {
		return nil
	}
	var failure *LoginError
	if errors.As(err, &failure) {
		return err
	}
	return &LoginError{Step: step, Err: err}
}

func loginStep(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "login"
	}
	switch u.Path {
	case "/ssl/ptqrshow":
		return "qr-show"
	case "/tme/trpc/proxy":
		return "device-bootstrap"
	case "/ssl/ptqrlogin":
		return "qr-poll"
	case "/check_sig":
		return "check-sig"
	case "/oauth2.0/login_jump":
		return "login-jump"
	case "/oauth2.0/authorize":
		return "authorize"
	case "/cgi-bin/musicu.fcg":
		return "qq-login"
	}
	return "login"
}

// LoginErrorMessage is safe to show in the fixed-width native login dialog.
func LoginErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var response *loginResponseError
	if errors.As(err, &response) {
		return response.message()
	}
	stage := "登录"
	var failure *LoginError
	if errors.As(err, &failure) {
		switch failure.Step {
		case "qr-show":
			stage = "二维码获取"
		case "device-bootstrap":
			stage = "登录设备初始化"
		case "qr-poll":
			stage = "扫码状态查询"
		case "qr-image":
			stage = "二维码图片解析"
		case "check-sig":
			stage = "QQ 授权校验"
		case "login-jump":
			stage = "QQ 授权跳转"
		case "authorize":
			stage = "QQ 授权确认"
		case "qq-login":
			stage = "QQ 音乐登录"
		}
	}
	var status loginHTTPError
	if errors.As(err, &status) {
		return fmt.Sprintf("%s服务返回 HTTP %d，请刷新二维码重试", stage, int(status))
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return stage + "服务无法连接，请检查网络后刷新二维码"
	}
	var certificate x509.UnknownAuthorityError
	var hostname x509.HostnameError
	if errors.As(err, &certificate) || errors.As(err, &hostname) {
		return stage + "证书校验失败，请检查网络后重试"
	}
	var network net.Error
	if errors.As(err, &network) {
		if network.Timeout() {
			return stage + "超时，请刷新二维码重试"
		}
		if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return stage + "连接失败（N201），请检查网络后重试"
		}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return stage + "连接被中断，请刷新二维码重试"
	}
	if failure != nil && failure.Step == "qq-login" {
		return "QQ 音乐登录响应异常（L200）"
	}
	return stage + "失败，请刷新二维码重试"
}
