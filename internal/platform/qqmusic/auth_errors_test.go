package qqmusic

import (
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
)

// A failed scan-confirmation request must never print its temporary authorization parameters.
func TestLoginErrorRedactsAuthorizationURL(t *testing.T) {
	cause := &url.Error{Op: "Get", URL: "https://ssl.ptlogin2.graph.qq.com/check_sig?ptsigx=private-signature&uin=private-account", Err: io.EOF}
	err := loginFailure("check-sig", cause)
	for _, message := range []string{err.Error(), LoginErrorMessage(err), LoginErrorMessage(cause)} {
		for _, secret := range []string{"https://", "ptsigx", "private-signature", "private-account"} {
			if strings.Contains(message, secret) {
				t.Errorf("login error disclosed authorization data: %q", message)
			}
		}
	}
	if !errors.Is(err, io.EOF) {
		t.Fatal("lost network failure cause")
	}
	if !strings.Contains(LoginErrorMessage(err), "QQ 授权") {
		t.Fatal("lost failed login step")
	}
}

// Transport failures must be distinguishable from rejected or malformed credential responses.
func TestLoginErrorClassifiesTransportFailure(t *testing.T) {
	cause := &url.Error{Op: "Post", URL: "https://u.y.qq.com/cgi-bin/musicu.fcg?private=fake", Err: errors.New("TLS handshake failure")}
	message := LoginErrorMessage(loginFailure("qq-login", cause))
	if !strings.Contains(message, "N201") || strings.Contains(message, "private=") {
		t.Fatalf("unexpected login diagnostic: %s", message)
	}
}
