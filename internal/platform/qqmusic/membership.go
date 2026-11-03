package qqmusic

import "errors"

// Membership describes the account, not whether a particular song can play.
type Membership string

const (
	MembershipUnknown Membership = "unknown"
	MembershipFree    Membership = "free"
	MembershipGreen   Membership = "green"
	MembershipSuper   Membership = "super"
)

// UserMembership reads the signed-in account's membership flags. iSuperVip is
// also set for green diamond accounts, so only HugeVip identifies super VIP.
func UserMembership() (Membership, error) {
	s := sessionSnapshot()
	if s.MusicID == "" || s.MusicKey == "" {
		return MembershipUnknown, errors.New("请先登录 QQ 音乐")
	}
	var data struct {
		Info map[string]struct {
			VIP  *int `json:"iVipFlag"`
			Huge *int `json:"HugeVip"`
		} `json:"infoMap"`
	}
	if err := rpc("userInfo.VipQueryServer", "SRFVipQuery_V2", map[string]any{"uin_list": []string{s.MusicID}}, &data); err != nil {
		return MembershipUnknown, err
	}
	info, ok := data.Info[s.MusicID]
	if !ok {
		return MembershipUnknown, errors.New("QQ 音乐未返回会员状态")
	}
	if info.Huge == nil {
		return MembershipUnknown, nil
	}
	if *info.Huge == 1 {
		return MembershipSuper, nil
	}
	if *info.Huge != 0 || info.VIP == nil {
		return MembershipUnknown, nil
	}
	switch *info.VIP {
	case 0:
		return MembershipFree, nil
	case 1:
		return MembershipGreen, nil
	default:
		return MembershipUnknown, nil
	}
}
