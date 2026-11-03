package qqmusic

import (
	"errors"
	"strings"
	"testing"
)

// Optional numeric metadata encoded as strings used to reject an otherwise valid QQ credential.
func TestQQLoginAcceptsNumericMetadata(t *testing.T) {
	for _, test := range []struct {
		metadata         string
		created, expires int64
	}{
		{`"musickeyCreateTime":0,"keyExpiresIn":259200`, 0, 259200},
		{`"musickeyCreateTime":"0","keyExpiresIn":"259200"`, 0, 259200},
		{`"musickeyCreateTime":"1760000000","keyExpiresIn":0`, 1760000000, 0},
	} {
		body := `{"code":0,"req":{"code":0,"data":{"musicid":"10001","musickey":"private-key","loginType":2,` + test.metadata + `,"nick":null}}}`
		s, err := decodeQQLoginResponse([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if s.MusicID != "10001" || s.CreatedAt != test.created || s.ExpiresIn != test.expires {
			t.Fatal("lost login metadata")
		}
	}
}

// A provider refusal must retain its public business codes instead of collapsing into generic failure.
func TestQQLoginReportsRejectionCodes(t *testing.T) {
	_, err := decodeQQLoginResponse([]byte(`{"code":0,"req":{"code":1000,"data":{"musicid":0,"musickey":""}}}`))
	var response *loginResponseError
	if !errors.As(err, &response) || response.Kind != "rejected" || response.Business != 1000 {
		t.Fatalf("missing rejection metadata: %v", err)
	}
	if !strings.Contains(LoginErrorMessage(loginFailure("qq-login", err)), "0/1000") {
		t.Fatal("provider code hidden")
	}
}

// Missing status or credential fields must not look successful or expose response values in an error.
func TestQQLoginRejectsIncompleteResponse(t *testing.T) {
	for _, test := range []struct{ body, kind string }{
		{`{"req":{"code":0,"data":{}}}`, "status"},
		{`{"code":0,"req":{"code":0}}`, "data"},
		{`{"code":0,"req":{"code":0,"data":{"musickey":"private-key"}}}`, "id"},
		{`{"code":0,"req":{"code":0,"data":{"musicid":10001}}}`, "key"},
		{`{"code":0,"req":{"code":0,"data":{"musicid":10001,"musickey":"private-key","loginType":6}}}`, "type"},
	} {
		_, err := decodeQQLoginResponse([]byte(test.body))
		var response *loginResponseError
		if !errors.As(err, &response) || response.Kind != test.kind {
			t.Fatalf("wanted %s, got %v", test.kind, err)
		}
		if strings.Contains(err.Error(), "private-key") {
			t.Fatal("printed credential")
		}
	}
}
