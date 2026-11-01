package app

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// searchRequest is a search asked of NetEase.
type searchRequest struct {
	query  string
	kind   netease.SearchType
	offset int
}

// fakeSearch replaces searching NetEase with one that reports what it was
// asked, and finds nothing, which lands nowhere without a window.
func fakeSearch(t *testing.T) chan searchRequest {
	asked := make(chan searchRequest, 8)
	searchResults = func(q string, kind netease.SearchType, _, offset int) (netease.SearchResult, error) {
		asked <- searchRequest{q, kind, offset}
		return netease.SearchResult{}, errors.New("offline")
	}
	t.Cleanup(func() { searchResults = netease.SearchPage })
	return asked
}

func wait[T any](t *testing.T, ch chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(time.Second):
		t.Fatalf("%s wasn't asked for", what)
		panic("unreachable")
	}
}

// Enter searches songs for the query typed; a tab searches it for another
// kind, which the next search keeps, and back leaves the search.
func TestSearchKinds(t *testing.T) {
	asked := fakeSearch(t)
	a := newTestApp()
	a.settings = defaultSettings()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Type("春日影")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if got := wait(t, asked, "the search"); got != (searchRequest{"春日影", netease.SearchSongs, 0}) {
		t.Fatalf("searched %+v", got)
	}

	if err := tt.Click("歌手"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := wait(t, asked, "the artists"); got != (searchRequest{"春日影", netease.SearchArtists, 0}) {
		t.Fatalf("searched %+v", got)
	}
	a.search("迷星叫")
	if got := wait(t, asked, "the next search"); got.kind != netease.SearchArtists {
		t.Errorf("the next search looked for kind %d", got.kind)
	}
	if !slices.Equal(a.settings.SearchHistory, []string{"迷星叫", "春日影"}) {
		t.Errorf("history %q", a.settings.SearchHistory)
	}

	tt.Key(0, ui.KeyBack)
	tt.Key(0, ui.KeyBack)
	if a.route.page != pageHome {
		t.Errorf("back went to %+v", a.route)
	}
}

// The tabs over a search's songs switch it to another kind: it crashed,
// the list's state reset as the list built the tabs in its header.
func TestSearchKindFromSongs(t *testing.T) {
	asked := fakeSearch(t)
	a := catalogApp()
	a.open(route{page: pageSearch, query: "MyGO", kind: netease.SearchSongs})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("歌手"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.kind != netease.SearchArtists {
		t.Fatalf("on %+v", a.route)
	}
	if len(asked) > 0 {
		t.Error("the artists found were searched for again")
	}
}

// The 歌单 tab searches playlists, and a playlist it found opens its page.
func TestSearchPlaylists(t *testing.T) {
	asked := fakeSearch(t)
	a := catalogApp()
	a.open(route{page: pageSearch, query: "MyGO", kind: netease.SearchSongs})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := wait(t, asked, "the playlists"); got != (searchRequest{"MyGO", netease.SearchPlaylists, 0}) {
		t.Fatalf("searched %+v", got)
	}

	a.searches[a.route] = &searchData{
		res:  netease.SearchResult{Playlists: []netease.Playlist{{ID: 42, Name: "迷途人偶", TrackCount: 52}}, Total: 1},
		page: paged{loaded: true},
	}
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "迷途人偶"} // so its page loads nothing
	tt.Frame()
	if err := tt.Click("迷途人偶"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pagePlaylist, id: 42}) {
		t.Errorf("clicked through to %+v", a.route)
	}
}

// A search's next page loads once its foot comes into view, from where the
// last ended, and a search done again loads from its start.
func TestSearchLoadsMore(t *testing.T) {
	asked := fakeSearch(t)
	a := newTestApp()
	a.settings = defaultSettings()
	r := route{page: pageSearch, query: "春日影", kind: netease.SearchUsers}
	a.searches[r] = &searchData{
		res:  netease.SearchResult{Users: []netease.User{{UserID: 1, Nickname: "甲"}, {UserID: 2, Nickname: "乙"}}, Total: 5},
		page: paged{loaded: true, more: true},
	}
	a.open(r)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	if got := wait(t, asked, "the next page"); got != (searchRequest{"春日影", netease.SearchUsers, 2}) {
		t.Fatalf("asked for %+v", got)
	}

	a.search("春日影")
	if got := wait(t, asked, "the search again"); got.offset != 0 {
		t.Errorf("searched again from %d", got.offset)
	}
}

// A search before, from the empty field's popup, searches again, clicked
// or chosen with the arrows; it was lost when its option was keyed anew.
func TestSearchHistoryChoice(t *testing.T) {
	for _, keys := range []bool{false, true} {
		asked := fakeSearch(t)
		a := newTestApp()
		a.settings = defaultSettings()
		a.settings.SearchHistory = []string{"春日影", "迷星叫"}
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		if err := tt.Click("搜索"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if keys {
			tt.Key(0, ui.KeyDown)
			tt.Key(0, ui.KeyDown)
			tt.Key(0, ui.KeyEnter)
		} else if err := tt.Click("迷星叫"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if got := wait(t, asked, "the search"); got.query != "迷星叫" || a.query != "迷星叫" {
			t.Fatalf("keys %v: searched %+v, field %q", keys, got, a.query)
		}
		if !slices.Equal(a.settings.SearchHistory, []string{"迷星叫", "春日影"}) {
			t.Errorf("keys %v: history %q", keys, a.settings.SearchHistory)
		}
	}
}

// Typing asks for suggestions once it pauses, and a suggested album opens
// its page, remembering the query.
func TestSuggestionOpensPage(t *testing.T) {
	asked := make(chan string, 4)
	searchSuggest = func(q string) (netease.Suggestions, error) { asked <- q; return netease.Suggestions{}, nil }
	defer func() { searchSuggest = netease.SearchSuggest }()
	a := newTestApp()
	a.settings = defaultSettings()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Click("搜索")
	tt.Type("迷跡")
	tt.Frame()
	if q := wait(t, asked, "suggestions"); q != "迷跡" {
		t.Fatalf("suggestions for %q", q)
	}
	// As they land, which post drops without a window.
	a.suggestions.loading = false
	a.suggestions.items = suggestionList(netease.Suggestions{
		Albums: []netease.AlbumProfile{{Album: netease.Album{ID: 8, Name: "迷跡波"}, Artists: []netease.Artist{{Name: "MyGO!!!!!"}}}},
		Order:  []string{"albums"},
	})
	tt.Frame()
	if err := tt.Click("专辑：迷跡波"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pageAlbum, id: 8}) || !slices.Equal(a.settings.SearchHistory, []string{"迷跡"}) {
		t.Fatalf("opened %+v, history %q", a.route, a.settings.SearchHistory)
	}
}

// An artist's 专辑 and 动态 load their first page as they are chosen, the
// events from the artist's own account; an album opens its page.
func TestArtistAlbumsAndEvents(t *testing.T) {
	albums, events := make(chan int, 4), make(chan [2]int64, 4)
	albumsBy = func(id int64, _, offset int) ([]netease.AlbumProfile, bool, error) {
		albums <- offset
		return nil, false, errors.New("offline")
	}
	accountEvents = func(uid int64, _ int, before int64) ([]netease.Event, int64, bool, error) {
		events <- [2]int64{uid, before}
		return nil, 0, false, errors.New("offline")
	}
	albumDetail = func(int64) (*netease.AlbumProfile, error) { return nil, errors.New("offline") }
	defer func() {
		albumsBy, accountEvents, albumDetail = netease.ArtistAlbums, netease.Events, netease.AlbumDetail
	}()
	a := newTestApp()
	a.settings = defaultSettings()
	d := &artistData{profile: &netease.ArtistProfile{Artist: netease.Artist{ID: 12, Name: "乙"}, AccountID: 99}}
	a.artists[12] = d
	a.open(route{page: pageArtist, id: 12})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.Click("专辑"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if offset := wait(t, albums, "the albums"); offset != 0 {
		t.Fatalf("albums from %d", offset)
	}
	d.albums = []netease.AlbumProfile{{Album: netease.Album{ID: 8, Name: "迷跡波"}}}
	d.albumsPage = paged{loaded: true}
	tt.Frame()
	if err := tt.Click("迷跡波"); err != nil {
		t.Fatal(err)
	}
	if a.route != (route{page: pageAlbum, id: 8}) {
		t.Fatalf("the album opened %+v", a.route)
	}

	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if err := tt.Click("动态"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := wait(t, events, "the events"); got != [2]int64{99, -1} {
		t.Fatalf("events of %d before %d", got[0], got[1])
	}
	if len(albums) > 0 {
		t.Error("the albums were asked for again")
	}
}

// 关注 follows an artist at once, and asks NetEase to; a second click
// waits for the first.
func TestFollowArtist(t *testing.T) {
	asked := make(chan bool, 4)
	followArtist = func(id int64, follow bool) error { asked <- follow; return nil }
	defer func() { followArtist = netease.FollowArtist }()
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1}
	a.artists[12] = &artistData{profile: &netease.ArtistProfile{Artist: netease.Artist{ID: 12, Name: "乙"}}}
	a.open(route{page: pageArtist, id: 12})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("关注"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.artists[12].profile.Followed || !wait(t, asked, "the follow") {
		t.Fatal("not followed")
	}
	tt.Click("已关注")
	if len(asked) > 0 || !a.artists[12].profile.Followed {
		t.Error("unfollowed before the follow landed")
	}
}

// A click on an episode plays the podcast's episodes from it, those with
// audio, and Now Playing shows what it is about in place of lyrics.
func TestPlayEpisode(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	pod := &netease.Podcast{ID: 5, Name: "夜航", DJ: netease.User{UserID: 6, Nickname: "主播"}}
	ep := func(id, audio int64, name string) netease.Program {
		p := netease.Program{ID: id, Name: name, Duration: 600000, Description: name + "的简介"}
		p.MainSong.ID = audio
		return p
	}
	a.podcasts[5] = &podcastData{podcast: pod, programs: []netease.Program{ep(1, 11, "第三期"), ep(2, 0, "第二期"), ep(3, 13, "第一期")}, page: paged{loaded: true}}
	a.open(route{page: pagePodcast, id: 5})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("第一期"); err != nil {
		t.Fatal(err)
	}
	cur := a.pl.Current()
	if cur == nil || cur.ID != 13 || cur.ProgramID != 3 || cur.PodcastID != 5 || len(a.pl.Queue()) != 2 {
		t.Fatalf("playing %+v of %d", cur, len(a.pl.Queue()))
	}
	a.pl.SetLyrics(nil)
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("节目简介"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if !a.nowPlaying || a.npPane != paneProgram || !tt.HasText("第一期的简介") {
		t.Fatalf("Now Playing open %v on pane %d", a.nowPlaying, a.npPane)
	}
}

// An episode's host in Now Playing opens its podcast's page, closing Now
// Playing, though that is the page it covers: it did nothing, as the page
// was open.
func TestEpisodeOpensPodcast(t *testing.T) {
	a := catalogApp()
	a.open(route{page: pagePodcast, id: 5})
	d := a.podcasts[5]
	a.playProgram(d, 0)
	a.pl.SetLyrics(nil)
	a.nowPlaying, a.npPane = true, paneProgram
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	// The host, as Now Playing credits them, not the podcast's.
	r, ok := tt.Find(d.podcast.DJ.Nickname)
	if !ok {
		t.Fatal("no host")
	}
	tt.ClickAt(r.X+4, r.Y+r.H/2)
	tt.Frame()
	if a.nowPlaying || a.route != (route{page: pagePodcast, id: 5}) {
		t.Fatalf("Now Playing open %v over %+v", a.nowPlaying, a.route)
	}
}

// The sidebar's 播客 opens the podcasts tab, which asks for the first page
// of the most popular podcasts.
func TestPodcastsTabLoads(t *testing.T) {
	asked := make(chan int, 4)
	hotPodcasts = func(_, offset int) ([]netease.Podcast, bool, error) {
		asked <- offset
		return nil, false, errors.New("offline")
	}
	recommendedPodcasts = func() ([]netease.Podcast, error) { return nil, errors.New("offline") }
	defer func() { hotPodcasts, recommendedPodcasts = netease.HotPodcasts, netease.RecommendedPodcasts }()
	a := newTestApp()
	a.settings = defaultSettings()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("播客"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pagePodcasts {
		t.Fatalf("opened %+v", a.route)
	}
	if got := wait(t, asked, "the popular podcasts"); got != 0 {
		t.Fatalf("asked from %d", got)
	}
}

// A podcast's card in the tab opens its page; the button on its cover plays
// its newest episode instead, and leaves the tab open.
func TestPodcastCard(t *testing.T) {
	a := catalogApp()
	a.open(route{page: pagePodcasts})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	r, ok := tt.Find("夜航电台")
	if !ok {
		t.Fatal("no card")
	}
	// The button shows under the pointer, in the corner of the square
	// cover.
	x, y := r.X+r.W-29, r.Y+r.W-29
	tt.Move(x, y)
	tt.Advance(time.Second)
	tt.Frame()
	tt.ClickAt(x, y)
	tt.Frame()
	if cur := a.pl.Current(); cur == nil || cur.ProgramID != 40 || a.route.page != pagePodcasts {
		t.Fatalf("playing %+v on %+v", cur, a.route)
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.W/2)
	tt.Frame()
	if a.route != (route{page: pagePodcast, id: 5}) {
		t.Fatalf("opened %+v", a.route)
	}
}

// A page of popular podcasts lists only those not listed yet, and the next
// starts past all of it. NetEase's pages overlap: the tab listed podcasts
// twice, two cards under one key sharing their state.
func TestHotPodcastsOverlap(t *testing.T) {
	pods := func(ids ...int64) []netease.Podcast {
		var out []netease.Podcast
		for _, id := range ids {
			out = append(out, netease.Podcast{ID: id})
		}
		return out
	}
	var tab podcastTab
	tab.addHot(pods(1, 2, 3))
	tab.addHot(pods(4, 2, 5))
	var ids []int64
	for _, p := range tab.hot {
		ids = append(ids, p.ID)
	}
	if !slices.Equal(ids, []int64{1, 2, 3, 4, 5}) || tab.hotAt != 6 {
		t.Fatalf("listed %v, next from %d", ids, tab.hotAt)
	}
}

// A song row's album opens its page, not playing the song as the row's
// click would.
func TestAlbumLinkInRow(t *testing.T) {
	albumDetail = func(int64) (*netease.AlbumProfile, error) { return nil, errors.New("offline") }
	defer func() { albumDetail = netease.AlbumDetail }()
	a := newTestApp()
	a.settings = defaultSettings()
	song := netease.Track{ID: 2, Name: "起风了", Artists: []netease.Artist{{ID: 13, Name: "丙"}}, Album: netease.Album{ID: 8, Name: "迷跡波"}, Duration: 200000}
	a.artists[13] = &artistData{profile: &netease.ArtistProfile{Artist: netease.Artist{ID: 13, Name: "丙"}, Songs: []netease.Track{song}}}
	a.open(route{page: pageArtist, id: 13})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	// Its name, at the start of the column it fills.
	r, ok := tt.Find("迷跡波")
	if !ok {
		t.Fatal("no album")
	}
	tt.ClickAt(r.X+8, r.Y+r.H/2)
	tt.Frame()
	if a.route != (route{page: pageAlbum, id: 8}) || a.pl.Current() != nil {
		t.Fatalf("opened %+v, playing %+v", a.route, a.pl.Current())
	}
}

// catalogApp is an app with an artist, album, user and podcast loaded, and
// searches for each kind, to look at.
func catalogApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	day := time.Date(2026, 10, 1, 20, 0, 0, 0, time.Local).UnixMilli()
	band := netease.Artist{ID: 7, Name: "MyGO!!!!!"}
	al := netease.Album{ID: 8, Name: "迷跡波"}
	var songs []netease.Track
	for i, name := range []string{"春日影", "迷星叫", "壱雫空", "碧天伴走", "音一会", "栞"} {
		songs = append(songs, netease.Track{ID: int64(i + 1), Name: name, Artists: []netease.Artist{band}, Album: al, Duration: int64(210000 + 16000*i)})
	}
	var albums []netease.AlbumProfile
	for i, name := range []string{"迷跡波", "跡暖空", "壱雫空", "音一会", "迷星叫", "焚音打"} {
		albums = append(albums, netease.AlbumProfile{Album: netease.Album{ID: int64(20 + i), Name: name}, Artists: []netease.Artist{band}, PublishTime: day})
	}
	a.artists[7] = &artistData{
		profile: &netease.ArtistProfile{Artist: band, Alias: []string{"マイゴ"}, MusicSize: 38, AlbumSize: 9, BriefDesc: "迷いながらでも、前へ。", AccountID: 70, Songs: songs},
		albums:  albums, albumsPage: paged{loaded: true},
		events: []netease.Event{
			{ID: 1, Time: day, User: netease.User{Nickname: "MyGO!!!!!"}, Content: netease.EventContent{Msg: "新专辑《迷跡波》上线了！", Album: &albums[0]}},
			{ID: 2, Time: day - 86400000, User: netease.User{Nickname: "MyGO!!!!!"}, Content: netease.EventContent{Msg: "谢谢大家来看演出", Forwarded: "听歌的人：现场太好了"}},
		},
		eventsPage: paged{loaded: true},
	}
	a.albums[8] = &albumData{album: &netease.AlbumProfile{Album: al, Artists: []netease.Artist{band}, PublishTime: day, Description: "MyGO!!!!! 的第一张专辑。", Songs: songs}}
	user := netease.User{UserID: 9, Nickname: "听歌的人", Signature: "在音乐里，寻找同路的人。", Follows: 12, Followeds: 24}
	a.users[9] = &userData{user: &user, playlists: []netease.Playlist{{ID: 30, Name: "通勤路上", TrackCount: 40}, {ID: 31, Name: "睡前", TrackCount: 12}}}
	pod := &netease.Podcast{ID: 5, Name: "夜航电台", Category: "情感", DJ: user, ProgramCount: 120, SubCount: 34000, Description: "每晚陪你说说话。"}
	var programs []netease.Program
	for i := range 4 {
		p := netease.Program{ID: int64(40 + i), Name: fmt.Sprintf("第 %d 期：晚安", 120-i), Duration: 1800000, CreateTime: day - int64(i)*7*86400000}
		p.MainSong.ID = int64(50 + i)
		programs = append(programs, p)
	}
	a.podcasts[5] = &podcastData{podcast: pod, programs: programs, page: paged{loaded: true}}
	var hot []netease.Podcast
	for i, name := range []string{"深夜书店", "城市漫游", "慢慢说", "周末早餐", "通勤十分钟", "老唱片"} {
		hot = append(hot, netease.Podcast{ID: int64(60 + i), Name: name, DJ: netease.User{Nickname: "主播"}, RcmdText: "陪你度过每个夜晚", PlayCount: float64(1e6 * (i + 1))})
	}
	subbed := *pod
	subbed.LastProgram = programs[0].Name
	a.podcastTab = &podcastTab{subs: []netease.Podcast{subbed}, picks: hot[:3], hot: hot, hotPage: paged{loaded: true}}
	for _, res := range []struct {
		kind netease.SearchType
		res  netease.SearchResult
	}{
		{netease.SearchSongs, netease.SearchResult{Songs: songs, Total: 38}},
		{netease.SearchArtists, netease.SearchResult{Artists: []netease.ArtistProfile{*a.artists[7].profile, {Artist: netease.Artist{ID: 10, Name: "Ave Mujica"}}}, Total: 2}},
		{netease.SearchAlbums, netease.SearchResult{Albums: albums, Total: 6}},
		{netease.SearchUsers, netease.SearchResult{Users: []netease.User{user, {UserID: 10, Nickname: "夏日来信"}}, Total: 2}},
		{netease.SearchPodcasts, netease.SearchResult{Podcasts: []netease.Podcast{*pod}, Total: 1}},
	} {
		a.searches[route{page: pageSearch, query: "MyGO", kind: res.kind}] = &searchData{res: res.res, page: paged{loaded: true}}
	}
	return a
}

// catalogScenes are the pages of catalogApp.
var catalogScenes = []struct {
	name string
	r    route
	tab  int
}{
	{"search-songs", route{page: pageSearch, query: "MyGO", kind: netease.SearchSongs}, 0},
	{"search-artists", route{page: pageSearch, query: "MyGO", kind: netease.SearchArtists}, 0},
	{"search-albums", route{page: pageSearch, query: "MyGO", kind: netease.SearchAlbums}, 0},
	{"search-users", route{page: pageSearch, query: "MyGO", kind: netease.SearchUsers}, 0},
	{"search-podcasts", route{page: pageSearch, query: "MyGO", kind: netease.SearchPodcasts}, 0},
	{"artist", route{page: pageArtist, id: 7}, artistHot},
	{"artist-albums", route{page: pageArtist, id: 7}, artistAlbums},
	{"artist-events", route{page: pageArtist, id: 7}, artistEvents},
	{"album", route{page: pageAlbum, id: 8}, 0},
	{"user", route{page: pageUser, id: 9}, 0},
	{"podcast", route{page: pagePodcast, id: 5}, 0},
	{"podcasts", route{page: pagePodcasts}, 0},
}

// With NM_FRAMES set, saves the pages of searches, artists, albums, users
// and podcasts in the light and the dark.
func TestCatalogFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, s := range catalogScenes {
		for _, dark := range []bool{false, true} {
			a := catalogApp()
			a.artistTab = s.tab
			a.open(s.r)
			tt := ui.NewTester(a.view, 1200, 800)
			tt.SetDark(dark)
			tt.Advance(time.Second)
			tt.Frame()
			name := s.name
			if dark {
				name += "-dark"
			}
			f, _ := os.Create(filepath.Join(dir, "catalog-"+name+".png"))
			png.Encode(f, tt.Image())
			f.Close()
		}
	}
}

// The pages of searches, artists, albums, users and podcasts ask for no
// frames once shown.
func TestCatalogFrameBudget(t *testing.T) {
	for _, s := range catalogScenes {
		a := catalogApp()
		a.artistTab = s.tab
		a.open(s.r)
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Advance(time.Second)
		tt.Frame()
		tt.Advance(time.Second)
		if tt.FramePending() {
			t.Errorf("%s asks for frames while idle", s.name)
		}
	}
}
