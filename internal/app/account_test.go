package app

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Clicking the account row drops its menu, which takes the focus, on the
// platform shown, so the arrows walk its items, and Escape or a second click puts it away.
func TestAccountMenu(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1, Nickname: "小明"}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()

	if err := tt.Click("小明"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.accountOpen || !tt.HasText("退出登录") {
		t.Fatalf("menu not open: %v", a.accountOpen)
	}
	if !tt.Focused("网易云音乐") {
		t.Fatal("the platform shown has no focus")
	}
	// Down walks the other platform, the listener's page, settings, then out.
	for _, want := range []string{"飞牛音乐", "我的主页", "设置…", "退出登录"} {
		tt.Key(0, ui.KeyDown)
		tt.Frame()
		if !tt.Focused(want) {
			t.Fatalf("Down did not move to %s", want)
		}
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.accountOpen {
		t.Fatal("Escape left the menu open")
	}

	tt.Click("小明")
	tt.Frame()
	tt.Click("小明")
	tt.Frame()
	if a.accountOpen {
		t.Fatal("a second click left the menu open")
	}
}

// 我的主页 in the account menu opens the page of who is signed in and puts
// the menu away.
func TestAccountMenuOpensOwnPage(t *testing.T) {
	detail, playlists := userDetail, userPlaylists
	defer func() { userDetail, userPlaylists = detail, playlists }()
	userDetail = func(id int64) (*netease.User, error) { return &netease.User{UserID: id, Nickname: "小明"}, nil }
	userPlaylists = func(int64) ([]netease.Playlist, []netease.Playlist, error) { return nil, nil, nil }

	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 42, Nickname: "小明"}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	tt.Click("小明")
	tt.Frame()
	if err := tt.Click("我的主页"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pageUser || a.route.id != 42 {
		t.Fatalf("route = %+v, want the page of user 42", a.route)
	}
	if a.accountOpen {
		t.Fatal("the menu stayed open")
	}
}

// fakeSubscriptions answers for the artists and podcasts the listener
// follows, and tells on each time they are asked for.
func fakeSubscriptions(t *testing.T) chan struct{} {
	asked := make(chan struct{}, 8)
	ars, pods := followedArtists, subscribedPodcasts
	t.Cleanup(func() { followedArtists, subscribedPodcasts = ars, pods })
	followedArtists = func() ([]netease.ArtistProfile, error) { asked <- struct{}{}; return nil, nil }
	subscribedPodcasts = func() ([]netease.Podcast, error) { return nil, nil }
	return asked
}

// ownPageApp is signed in as user 9, whose page has what they follow.
func ownPageApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 9, Nickname: "听歌的人"}
	user := netease.User{UserID: 9, Nickname: "听歌的人", Signature: "在音乐里，寻找同路的人。", Follows: 12, Followeds: 24}
	a.users[9] = &userData{
		user:      &user,
		playlists: []netease.Playlist{{ID: 30, Name: "通勤路上", TrackCount: 40}},
		artists:   []netease.ArtistProfile{{Artist: netease.Artist{ID: 7, Name: "MyGO!!!!!"}}, {Artist: netease.Artist{ID: 10, Name: "Ave Mujica"}}},
		podcasts: []netease.Podcast{
			{ID: 5, Name: "夜航电台", DJ: netease.User{Nickname: "主播"}, ProgramCount: 120, Description: "每晚陪你说说话。"},
			{ID: 6, Name: "通勤十分钟", ProgramCount: 48},
		},
		artistsShown: true, podcastsShown: true,
	}
	return a
}

// The listener's own page asks again for what they follow each time it
// shows, as they may have followed someone since, and a followed artist
// opens their page.
func TestOwnPageFollows(t *testing.T) {
	asked := fakeSubscriptions(t)
	a := ownPageApp()
	a.open(route{page: pageUser, id: 9})
	wait(t, asked, "the artists followed")
	tt := ui.NewTester(a.view, 1200, 1200)
	tt.Frame()
	if err := tt.Click("Ave Mujica"); err != nil {
		t.Fatal(err)
	}
	if a.route.page != pageArtist || a.route.id != 10 {
		t.Fatalf("route = %+v, want the page of artist 10", a.route)
	}
	a.users[9].subsLoading = 0 // what was asked for has landed
	a.back()
	wait(t, asked, "the artists followed, again")
}

// Someone else's page doesn't ask for what the listener follows, nor shows
// it.
func TestOthersPageHidesFollows(t *testing.T) {
	asked := fakeSubscriptions(t)
	a := ownPageApp()
	a.profile = &netease.Profile{UserID: 1}
	a.open(route{page: pageUser, id: 9})
	tt := ui.NewTester(a.view, 1200, 1200)
	tt.Frame()
	if len(asked) > 0 {
		t.Fatal("asked for what the listener follows")
	}
	if err := tt.Click("Ave Mujica"); err == nil {
		t.Fatal("the listener's artists showed on someone else's page")
	}
}

// The listener's own page asks for no frames once shown.
func TestOwnPageFrameBudget(t *testing.T) {
	fakeSubscriptions(t)
	a := ownPageApp()
	a.open(route{page: pageUser, id: 9})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Error("the page asks for frames while idle")
	}
}

// With NM_FRAMES set, saves the listener's own page, scrolled to what they
// follow, in the light and the dark.
func TestOwnPageFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	fakeSubscriptions(t)
	for _, dark := range []bool{false, true} {
		a := ownPageApp()
		a.open(route{page: pageUser, id: 9})
		tt := ui.NewTester(a.view, 1200, 1200)
		tt.SetDark(dark)
		tt.Advance(time.Second)
		tt.Frame()
		name := "user-own.png"
		if dark {
			name = "user-own-dark.png"
		}
		f, _ := os.Create(filepath.Join(dir, name))
		png.Encode(f, tt.Image())
		f.Close()
	}
}

// With NM_FRAMES set, saves the foot of the sidebar with the account menu
// open, light and dark.
func TestAccountMenuFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, dark := range []bool{false, true} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.profile = &netease.Profile{UserID: 1, Nickname: "小明"}
		tt := ui.NewTester(a.view, 1000, 600)
		tt.SetDark(dark)
		tt.Frame()
		tt.Click("小明")
		tt.Advance(time.Second)
		tt.Frame()
		img := tt.Image()
		s := float32(img.Bounds().Dx()) / 1000
		crop := image.Rect(0, int(400*s), int((sidebarWidth+20)*s), img.Bounds().Dy())
		out := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
		draw.Draw(out, out.Bounds(), img, crop.Min, draw.Src)
		name := "account-light.png"
		if dark {
			name = "account-dark.png"
		}
		f, _ := os.Create(filepath.Join(dir, name))
		png.Encode(f, out)
		f.Close()
	}
}
