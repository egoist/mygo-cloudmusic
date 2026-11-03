package qqmusic

import (
	"encoding/json"
	"testing"
)

// Membership follows HugeVip and iVipFlag without mistaking green or unknown flags for another tier.
func TestUserMembership(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags map[string]any
		want  Membership
	}{
		{"free", map[string]any{"HugeVip": 0, "iVipFlag": 0, "iSuperVip": 1}, MembershipFree},
		{"green", map[string]any{"HugeVip": 0, "iVipFlag": 1, "iSuperVip": 1}, MembershipGreen},
		{"super", map[string]any{"HugeVip": 1, "iVipFlag": 1}, MembershipSuper},
		{"missing huge", map[string]any{"iVipFlag": 1}, MembershipUnknown},
		{"missing vip", map[string]any{"HugeVip": 0}, MembershipUnknown},
		{"unknown huge", map[string]any{"HugeVip": 2, "iVipFlag": 0}, MembershipUnknown},
		{"unknown vip", map[string]any{"HugeVip": 0, "iVipFlag": 2}, MembershipUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeLibraryModuleRPC(t, func(module, method string, param map[string]json.RawMessage) any {
				if module != "userInfo.VipQueryServer" || method != "SRFVipQuery_V2" || string(param["uin_list"]) != `["10001"]` {
					t.Errorf("unexpected membership request %s/%s %v", module, method, param)
				}
				return map[string]any{"infoMap": map[string]any{"10001": tc.flags}}
			})
			got, err := UserMembership()
			if err != nil || got != tc.want {
				t.Fatalf("membership = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

// An absent account entry or malformed flag must not become a free membership.
func TestUserMembershipInvalidResponse(t *testing.T) {
	for _, data := range []any{
		map[string]any{"infoMap": map[string]any{}},
		map[string]any{"infoMap": map[string]any{"10001": map[string]any{"HugeVip": "0", "iVipFlag": 0}}},
	} {
		fakeLibraryRPC(t, func(string, map[string]json.RawMessage) any { return data })
		if got, err := UserMembership(); err == nil || got != MembershipUnknown {
			t.Fatalf("invalid response returned %q, %v", got, err)
		}
	}
}

// Signed-out membership queries never contact the service.
func TestUserMembershipRequiresSession(t *testing.T) {
	fakeLibraryRPC(t, func(string, map[string]json.RawMessage) any {
		t.Error("sent a signed-out membership request")
		return nil
	})
	setSession(Session{})
	if got, err := UserMembership(); err == nil || got != MembershipUnknown {
		t.Fatalf("signed-out membership = %q, %v", got, err)
	}
}
