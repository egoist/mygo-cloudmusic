package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// The FM page offers to start the FM, and heart mode.
func TestFMPage(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.route = route{page: pageFM}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	for _, s := range []string{"私人 FM", "推荐", "熟悉", "探索", "开始收听", "心动模式", "登录后开启"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q in %q", s, tt.Texts())
		}
	}
}

// Playing the FM, its page shows the song, with the FM's controls.
func TestFMPagePlaying(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1, Nickname: "听众"}
	a.pl.Cue([]netease.Track{{ID: 1, Name: "晚风", Artists: []netease.Artist{{Name: "陈婧霏"}}}, {ID: 2, Name: "下一首"}})
	a.pl.AttachRadio(fmRadio(netease.FMExplore), func(got func([]netease.Track)) { got(nil) })
	a.route = route{page: pageFM}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	for _, s := range []string{"晚风", "私人 FM · 探索"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q in %q", s, tt.Texts())
		}
	}
	for _, label := range []string{"不喜欢", "下一首", "播放/暂停"} {
		if _, ok := tt.Find(label); !ok {
			t.Errorf("no %q button", label)
		}
	}
}

// Now Playing's comments: the hot ones, the latest, and their count on
// the tab.
func TestCommentsPane(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tr := netease.Track{ID: 9, Name: "晴天", Duration: 269000}
	a.pl.Cue([]netease.Track{tr})
	var hot, latest netease.Comment
	hot.ID, hot.Content, hot.LikedCount = 1, "从前从前有个人爱你很久", 123456
	hot.User.Nickname = "路人甲"
	latest.ID, latest.Content, latest.Time = 2, "晴天", time.Now().Add(-3*time.Minute).UnixMilli()
	latest.User.Nickname = "路人乙"
	latest.IPLocation.Location = "江苏"
	latest.BeReplied = append(latest.BeReplied, struct {
		User struct {
			Nickname string `json:"nickname"`
		} `json:"user"`
		Content string `json:"content"`
	}{Content: "晴天吗"})
	a.comments = commentState{track: tr.ID, hot: []netease.Comment{hot}, latest: []netease.Comment{latest}, total: 19702, loaded: true}
	a.nowPlaying, a.npPane = true, paneComments
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	for _, s := range []string{"评论 2.0万", "精彩评论", "最新评论 2.0万", "从前从前有个人爱你很久", "路人甲", "12.3万", "3分钟前 · 江苏", "没有更多评论了"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q in %q", s, tt.Texts())
		}
	}
}

func TestCommentTime(t *testing.T) {
	now := time.Date(2026, 10, 25, 22, 0, 0, 0, time.Local)
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{now.Add(-20 * time.Second), "刚刚"},
		{now.Add(-42 * time.Minute), "42分钟前"},
		{now.Add(-5 * time.Hour), "17:00"},
		{now.Add(-23 * time.Hour), "昨天 23:00"},
		{time.Date(2026, 3, 8, 9, 0, 0, 0, time.Local), "3月8日"},
		{time.Date(2021, 4, 10, 9, 0, 0, 0, time.Local), "2021年4月10日"},
	} {
		if got := commentTime(c.t, now); got != c.want {
			t.Errorf("%v: %q, want %q", c.t, got, c.want)
		}
	}
}

// Settings offer the lossless levels, loudness normalization and the cache.
func TestSettingsPlayback(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.cacheUsed = 300 << 20
	tt := ui.NewTester(a.settingsView, 560, 1200)
	tt.Frame()
	for _, s := range []string{"音量均衡", "歌曲缓存", "清除缓存", "2 GB"} {
		if !tt.HasText(s) {
			t.Errorf("missing %q in %q", s, tt.Texts())
		}
	}
	if !tt.HasText("听过的歌曲保存在本机，再次播放无需等待，也不耗流量；已用 300 MB") {
		t.Errorf("no cache usage in %q", tt.Texts())
	}
}
