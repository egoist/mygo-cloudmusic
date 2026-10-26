package app

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/fnmusic"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

// fnEnv is an app connected to a Feiniu service, which records what it is
// asked, and whose replies are run when the test says, as the main thread
// would.
type fnEnv struct {
	a      *app
	asked  chan string // "path?query" of each request but the login
	posted chan func()
	// routes answer requests for a path, in place of an empty list.
	routes map[string]func(q url.Values) any
}

// drain runs the replies that have come in, and waits for n if n > 0.
func (e *fnEnv) drain(t *testing.T, n int) {
	t.Helper()
	for i := 0; ; i++ {
		select {
		case f := <-e.posted:
			f()
			n--
			continue
		default:
		}
		if n <= 0 {
			return
		}
		select {
		case f := <-e.posted:
			f()
			n--
		case <-time.After(5 * time.Second):
			t.Fatal("no reply from the service")
		}
	}
}

// fnTrack is a song of the service as it is listed.
func fnTrack(guid, name, artist, format string) netease.Track {
	id := fnmusic.ID(guid)
	return netease.Track{ID: id, Name: name, Artists: []netease.Artist{{Name: artist}}, Duration: 200000,
		Album:  netease.Album{ID: fnmusic.ID("album-" + artist), Name: artist + "的专辑"},
		Source: fnmusic.Source, Ref: guid, Format: format, Privilege: &netease.Privilege{ID: id, Pl: 1}}
}

// fnTestApp is an app showing a Feiniu service with a library of three
// songs, as if its first page had come in.
func fnTestApp(t *testing.T) *fnEnv {
	e := &fnEnv{asked: make(chan string, 256), posted: make(chan func(), 256), routes: map[string]func(url.Values) any{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/music/api/v1")
		if path != "/user/password-login" {
			e.asked <- path + "?" + r.URL.RawQuery
		}
		if route := e.routes[path]; route != nil {
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": route(r.URL.Query())})
			return
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if strings.HasPrefix(path, "/favorite-track/") {
			e.asked <- "like " + path + " " + body["trackGUID"]
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"userToken": "t", "list": []any{}, "total": 0}})
	}))
	t.Cleanup(srv.Close)
	client, err := fnmusic.New(fnmusic.Config{Address: srv.URL, Username: "me", PassHash: fnmusic.HashPassword("x")})
	if err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.Source = sourceFn
	a.fn.client = client
	a.fn.sync = func(f func()) { e.posted <- f }
	a.pl.OnShuffle = a.fnShuffleToggled
	a.route = a.home()
	e.a = a
	tracks := []netease.Track{
		fnTrack("a", "晴天", "周杰伦", "flac"),
		fnTrack("b", "七里香", "周杰伦", "m4a"),
		fnTrack("c", "宝船", "DOUDOU", "mp3"),
	}
	e.seed(fnmusic.LibraryID, tracks)
	return e
}

// seed makes the list numbered id one whose first page, of these songs, is
// in: asking the service for another is a failure of the test.
func (e *fnEnv) seed(id int64, tracks []netease.Track) *fnList {
	a := e.a
	l := a.newFnList(fnSpec{typ: listLibrary})
	if id == fnmusic.FavoritesID {
		l = a.newFnList(fnSpec{typ: listFavorites})
	}
	l.pg = a.trackPager(func(int) ([]netease.Track, int, error) { return nil, 0, errors.New("unexpected request") })
	l.pg.land(0, tracks, len(tracks))
	if a.fn.lists == nil {
		a.fn.lists = map[int64]*fnList{}
	}
	a.fn.lists[id] = l
	return l
}

// A song of the Feiniu service is liked with the service, without a
// NetEase login: the heart once asked the listener to sign in to NetEase
// and did nothing else.
func TestFnLikeNeedsNoNeteaseLogin(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	tr := a.fn.lists[fnmusic.LibraryID].pg.items[0]
	a.pl.Cue([]netease.Track{tr})
	a.pl.SetLyrics(nil)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()

	if err := tt.Click("喜欢"); err != nil {
		t.Fatal(err)
	}
	if !a.fn.liked[tr.ID] {
		t.Fatal("the heart did not fill at once")
	}
	if a.login.open {
		t.Error("asked for a NetEase login")
	}
	for {
		select {
		case got := <-e.asked:
			if got == "like /favorite-track/create a" {
				e.drain(t, 1)
				if a.fn.likeBusy[tr.ID] {
					t.Error("still being liked after the service answered")
				}
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the service was not asked")
		}
	}
}

// The platform is switched from the account menu at the foot of the
// sidebar, and the whole app shows the one chosen: Feiniu's starts on its
// library, NetEase's on its home, and neither keeps the other's history.
func TestSwitchPlatformFromAccountMenu(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	a.settings.Source = sourceNetease
	a.route = a.home()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()

	if err := tt.Click("账户"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("飞牛音乐"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.onFn() || a.route != (route{page: pagePlaylist, id: fnmusic.LibraryID}) || a.accountOpen {
		t.Fatalf("source %q, route %+v, menu open %v", a.settings.Source, a.route, a.accountOpen)
	}
	if _, ok := tt.Find("晴天"); !ok {
		t.Error("the library is not shown")
	}
	// The menu is there on the other platform, to go back from.
	if err := tt.Click("me"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("网易云音乐"); err != nil {
		t.Fatal(err)
	}
	if a.onFn() || a.route.page != pageHome || len(a.history) != 0 {
		t.Errorf("source %q, route %+v, %d pages of history", a.settings.Source, a.route, len(a.history))
	}
}

// With no one signed in to the platform shown, the account row still opens
// the menu: the platform is switched from there, and it is where to sign in.
func TestAccountMenuWithoutAccount(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	a.fn.client = nil
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("飞牛音乐"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("连接飞牛音乐…"); !ok {
		t.Error("no way to connect in the menu")
	}
	if _, ok := tt.Find("网易云音乐"); !ok {
		t.Error("no way back to NetEase in the menu")
	}
}

// A click plays a song of the library from there on, a kind of file the
// player does not decode too: the service transcodes it.
func TestFnRowClickPlays(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()

	if err := tt.Click("七里香"); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.Ref != "b" {
		t.Fatalf("a song to be transcoded did not play: %+v", cur)
	}
	if err := tt.Click("宝船"); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.Ref != "c" {
		t.Fatalf("playing %+v", cur)
	}
}

// Searching on the Feiniu platform asks the service, which searches: the
// app does not filter the library it has, which it does not have whole.
func TestFnSearchAsksTheService(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	a.fnSearch("周杰伦 晴")
	if a.fn.search == nil || a.fn.search.name != "周杰伦 晴" {
		t.Fatalf("search %+v", a.fn.search)
	}
	a.fn.search.pg.at(0)
	var got string
	for got == "" {
		select {
		case s := <-e.asked:
			if strings.HasPrefix(s, "/search/track") {
				got = s
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the service was not asked to search")
		}
	}
	if !strings.Contains(got, "q="+"%E5%91%A8") || !strings.Contains(got, "page=1") || !strings.Contains(got, "size=100") {
		t.Errorf("asked %q", got)
	}
}

// A song is asked for a page at a time, as it is scrolled to: opening the
// library asks for its first page, and not for the rest of it.
func TestFnLibraryOpensOnOnePage(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	delete(a.fn.lists, fnmusic.LibraryID)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	e.drain(t, 1)
	tt.Frame()
	var tracks int
	for len(e.asked) > 0 {
		if s := <-e.asked; strings.HasPrefix(s, "/track/list") {
			tracks++
			if !strings.Contains(s, "page=1") || !strings.Contains(s, "sort=createdAt%2Cdesc") {
				t.Errorf("asked %q", s)
			}
		}
	}
	if tracks != 1 {
		t.Errorf("%d requests for songs to show an empty library", tracks)
	}
}

// libraryOf makes the service answer the library's pages, and the songs at
// single positions, with n songs named by their place: "歌0" to "歌n-1".
func (e *fnEnv) libraryOf(n int) {
	e.routes["/track/list"] = func(q url.Values) any {
		page, _ := strconv.Atoi(q.Get("page"))
		size, _ := strconv.Atoi(q.Get("size"))
		// As the service does: a page past its 2000th is its first.
		if page > 2000 {
			page = 1
		}
		var list []map[string]any
		for i := (page - 1) * size; i < min(page*size, n); i++ {
			list = append(list, map[string]any{"guid": "g" + strconv.Itoa(i), "title": "歌" + strconv.Itoa(i),
				"audioSpec": map[string]any{"format": "mp3"}})
		}
		return map[string]any{"total": n, "list": list}
	}
}

// listOf is a list of the library, as a page of it opens.
func (e *fnEnv) listOf(n int) *fnList {
	e.libraryOf(n)
	l := e.a.newFnList(fnSpec{typ: listLibrary})
	e.a.fn.lists = map[int64]*fnList{fnmusic.LibraryID: l}
	return l
}

// What gives a queue the rest of a long list in order: the pages after what
// it was started from, as it asks, and nothing at the list's end.
func TestFnQueueGoesOnWithPages(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	l := e.listOf(250)
	l.pg.at(0)
	e.drain(t, 1)
	q := newFnQueue(l, false)
	q.next = fnPageSize
	feed := a.fnFeed(q)
	var songs [][]netease.Track
	for i := range 3 {
		feed(func(more []netease.Track) { songs = append(songs, more) })
		if i < 2 {
			e.drain(t, 1) // a page is read; at the list's end, nothing is
		}
	}
	if len(songs) != 3 || len(songs[0]) != 100 || len(songs[1]) != 50 || songs[2] != nil {
		t.Fatalf("pages given: %d", len(songs))
	}
	if songs[0][0].Name != "歌100" || songs[1][0].Name != "歌200" {
		t.Errorf("not in order: %q, %q", songs[0][0].Name, songs[1][0].Name)
	}
	if q.next != 250 {
		t.Errorf("next %d", q.next)
	}
}

// At random a queue asks for the songs at positions of a shuffled order, each
// in a small page of its own, a few at a time: every song once, and the song
// at each position the very one the order says, in a library so long that the
// service would answer the page of a position past its 2000th with its first;
// and none of the library read whole.
func TestFnRandomQueue(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	const n = 5000
	l := e.listOf(n)
	q := newFnQueue(l, true)
	q.seed = 7
	perm := rand.New(rand.NewPCG(7, 0x9e3779b97f4a7c15)).Perm(n)
	feed := a.fnFeed(q)
	seen := map[string]bool{}
	var got [][]netease.Track
	for i := range 3 {
		feed(func(more []netease.Track) { got = append(got, more) })
		for len(got) <= i {
			e.drain(t, 1)
		}
	}
	if len(got) != 3 {
		t.Fatalf("%d batches", len(got))
	}
	total := 0
	for b, batch := range got {
		if len(batch) != fnBatch {
			t.Errorf("a batch of %d", len(batch))
		}
		for k, tr := range batch {
			total++
			if seen[tr.Ref] {
				t.Errorf("%s twice", tr.Ref)
			}
			seen[tr.Ref] = true
			if want := "g" + strconv.Itoa(perm[b*fnBatch+k]); tr.Ref != want {
				t.Errorf("song %d of the order is %s, want %s", b*fnBatch+k, tr.Ref, want)
			}
		}
	}
	if total != 3*fnBatch || q.next != 3*fnBatch {
		t.Errorf("%d songs, next %d", total, q.next)
	}
	// Not in the order of the library.
	inOrder := true
	for i, tr := range got[0] {
		if tr.Ref != "g"+strconv.Itoa(i) {
			inOrder = false
		}
	}
	if inOrder {
		t.Error("the first batch is the library's first songs")
	}
	// Each song was asked for in a small page, and the library not read
	// whole: the one page of the usual size is the first, for the length.
	small, pages := 0, 0
	for len(e.asked) > 0 {
		s := <-e.asked
		if !strings.HasPrefix(s, "/track/list") {
			continue
		}
		if strings.Contains(s, "size=100") {
			pages++
		} else {
			small++
		}
	}
	if small < 3*fnBatch || pages > 1 {
		t.Errorf("%d songs asked in small pages, %d pages of the usual size", small, pages)
	}
	// The same seed makes the same order, which is how a restart goes on.
	q2 := newFnQueue(l, true)
	q2.seed = 7
	q2.next = fnBatch
	var again []netease.Track
	a.fnFeed(q2)(func(more []netease.Track) { again = more })
	for len(again) == 0 {
		e.drain(t, 1)
	}
	if again[0].Ref != got[1][0].Ref {
		t.Errorf("from the seed and where it had got to: %s, was %s", again[0].Ref, got[1][0].Ref)
	}
}

// What the session keeps of a queue makes it again: the list, the order, how
// far it had got.
func TestFnOriginRoundTrip(t *testing.T) {
	e := fnTestApp(t)
	l := e.a.newFnList(fnSpec{typ: listAlbum, ref: "alb"})
	q := newFnQueue(l, true)
	q.seed, q.next = 42, 30
	var o fnOrigin
	if err := json.Unmarshal([]byte(q.origin()), &o); err != nil {
		t.Fatal(err)
	}
	if o != (fnOrigin{T: listAlbum, Ref: "alb", Shuffle: true, Seed: 42, Next: 30}) {
		t.Errorf("%+v", o)
	}
	if got := listID(fnSpec{typ: listAlbum, ref: "alb"}); got != fnmusic.AlbumID("alb") || got != l.id {
		t.Errorf("list number %d", got)
	}
}

// After a restart a queue read from a list goes on reading it from where it
// was, and what was playing plays on: the queue is no longer cut at what
// was loaded.
func TestFnRestoreContinuesTheList(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	e.libraryOf(300)
	a.pl.Restore(playerSession(fnTrack("g150", "歌150", "x", "mp3")))
	raw, _ := json.Marshal(fnOrigin{T: listLibrary, Next: 200})
	a.restoreFnOrigin(string(raw))
	// The first page, for the length of the list, then the one it goes on at.
	for a.fn.q.next != 300 {
		e.drain(t, 1)
	}
	// The queue now holds the song it was playing and the page after.
	if !a.pl.HasOrigin() || a.fn.q == nil || a.fn.q.next != 300 {
		t.Fatalf("has origin %v, queue %+v", a.pl.HasOrigin(), a.fn.q)
	}
	if q := a.pl.Queue(); len(q) != 101 || q[1].Name != "歌200" {
		t.Errorf("queue of %d, then %q", len(q), q[1].Name)
	}
	// And keeps its place in the session, at the next restart.
	s, _ := a.pl.Session()
	var o fnOrigin
	json.Unmarshal([]byte(s.Origin), &o)
	if o.Next != 300 || o.T != listLibrary {
		t.Errorf("session origin %+v", o)
	}
}

// Shuffle switched on while a list plays makes the queue the song playing and
// songs of the list at random; switched off, the list in order from that song
// again.
func TestFnShuffleToggleOverAList(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	l := e.listOf(500)
	l.pg.at(0)
	e.drain(t, 1)
	l.pg.at(120)
	for l.pg.state[1] != pageHave {
		e.drain(t, 1)
	}
	a.playFn(l, 120)
	if !a.pl.HasOrigin() || a.pl.Current() == nil || a.pl.Current().Name != "歌120" {
		t.Fatalf("playing %+v", a.pl.Current())
	}
	a.pl.ToggleShuffle()
	if !a.pl.Shuffle() || !a.fn.q.shuffle || len(a.pl.Queue()) != 1 && a.pl.Queue()[0].Name != "歌120" {
		t.Fatalf("shuffle %v, queue of %d", a.pl.Shuffle(), len(a.pl.Queue()))
	}
	a.pl.ToggleShuffle()
	// In order, from the song after the one playing, which is at position
	// 120: the queue goes on with 歌121, as it read on at once.
	if q := a.pl.Queue(); a.pl.Shuffle() || a.fn.q.shuffle || len(q) < 2 || q[0].Name != "歌120" || q[1].Name != "歌121" {
		t.Errorf("shuffle %v, queue %v", a.pl.Shuffle(), a.pl.Queue())
	}
}

func playerSession(tr netease.Track) player.Session {
	return player.Session{Queue: []netease.Track{tr}, Order: []int{0}, At: 0}
}

// The roam is the service's: its songs one after another, each asked for with
// the roam ID of the one before; and it is a radio, as NetEase's FM is.
func TestFnRoam(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	step := func(id, title string) map[string]any {
		return map[string]any{"roamId": id, "track": map[string]any{"guid": id, "title": title, "audioSpec": map[string]any{"format": "mp3"}}}
	}
	var after atomic.Value
	e.routes["/track/roam-start"] = func(url.Values) any { return map[string]any{"current": step("r1", "一")} }
	e.routes["/track/roam-next"] = func(q url.Values) any {
		after.Store(q.Get("relativeRoamId"))
		return map[string]any{"current": step("r2", "二")}
	}
	a.startRoam()
	e.drain(t, 1)
	if a.pl.Radio() != fnRoamRadio || a.pl.Current() == nil || a.pl.Current().Name != "一" || a.fn.roamID != "r1" {
		t.Fatalf("radio %q, playing %+v, roam %q", a.pl.Radio(), a.pl.Current(), a.fn.roamID)
	}
	// The queue is a song short of its end: it asked for the next.
	e.drain(t, 1)
	if q := a.pl.Queue(); len(q) != 2 || q[1].Name != "二" || after.Load() != "r1" || a.fn.roamID != "r2" {
		t.Errorf("queue %v, asked after %v, roam %q", q, after.Load(), a.fn.roamID)
	}
	if radioLabel(fnRoamRadio) != "飞牛漫游" {
		t.Error("the radio has no name")
	}
}

// The hearts are drawn from what the service says of the songs as they
// come in, so no list of the songs liked has to be read to start with; but
// not over the listener's word on a song they are liking.
func TestFnHeartsComeWithTheSongs(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	liked := fnTrack("l", "喜欢的", "甲", "mp3")
	liked.Fav = true
	busy := fnTrack("n", "刚点的", "甲", "mp3")
	a.fn.likeBusy = map[int64]bool{busy.ID: true}
	a.setFnLiked(busy.ID, true)
	l := e.seed(fnmusic.FavoritesID, nil)
	l.pg.land(0, []netease.Track{liked, busy}, 2)
	if !a.isFnLiked(liked.ID) {
		t.Error("a song the service says is liked is not")
	}
	if !a.isFnLiked(busy.ID) {
		t.Error("a page took back what the listener just did")
	}
}

// The albums are cards read a page at a time, which open the songs of the
// album, in a list of their own, when clicked; and the order they come in
// is the listener's to choose, from the page.
func TestFnAlbumsPage(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	a.fn.albums = a.cardPager(func(int) ([]netease.Playlist, int, error) { return nil, 0, errors.New("unexpected request") })
	cards := []netease.Playlist{
		{ID: fnmusic.AlbumID("x"), Ref: "x", Kind: fnmusic.KindAlbum, Name: "叶惠美", Sub: "周杰伦", TrackCount: 11},
		{ID: fnmusic.AlbumID("y"), Ref: "y", Kind: fnmusic.KindAlbum, Name: "单曲", Sub: "某人", TrackCount: 1},
	}
	a.fn.albums.land(0, cards, 2)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()

	if err := tt.Click("专辑"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pageFnAlbums {
		t.Fatalf("route %+v", a.route)
	}
	if err := tt.Click("叶惠美"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	l := a.fn.lists[fnmusic.AlbumID("x")]
	if a.route.page != pagePlaylist || l == nil || l.kind != fnmusic.KindAlbum || l.name != "叶惠美" || l.sub != "周杰伦" {
		t.Fatalf("route %+v, list %+v", a.route, l)
	}
	e.drain(t, 1)
	found := false
	for len(e.asked) > 0 {
		if s := <-e.asked; strings.HasPrefix(s, "/track/album-detail/list") && strings.Contains(s, "albumGUID=x") {
			found = true
		}
	}
	if !found {
		t.Error("the songs of the album were not asked for")
	}

	// Another order starts the cards over, from the service's word on it.
	if err := tt.Click("专辑"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	old := a.fn.albums
	tt.Frame()
	if err := tt.Click("名称"); err != nil {
		t.Fatal(err)
	}
	if a.fn.albumSort != fnmusic.AlbumsName || a.fn.albums == old {
		t.Errorf("sort %q, the cards of the old order kept: %v", a.fn.albumSort, a.fn.albums == old)
	}
}

// The connect dialog takes an FN ID first, and an address from its switch;
// without what it needs it says so, rather than asking the service.
func TestFnConnectDialogModes(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	a.fn.client = nil
	a.openFnConnect()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()

	if a.fn.form.byAddress {
		t.Fatal("the dialog does not start with the FN ID")
	}
	if _, ok := tt.Find("FN Connect 访问码（可选）"); !ok {
		t.Error("no field for the access code")
	}
	if err := tt.Click("地址"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.fn.form.byAddress {
		t.Fatal("the switch did not choose the address")
	}
	if _, ok := tt.Find("FN Connect 访问码（可选）"); ok {
		t.Error("an access code is asked for an address")
	}
	a.submitFn()
	if a.fn.form.err == "" || a.fn.form.busy {
		t.Errorf("empty form: err %q, busy %v", a.fn.form.err, a.fn.form.busy)
	}
}

// idleFrames counts the frames a view asks for over a second at rest.
func idleFrames(tt *ui.Tester) int {
	tt.Advance(time.Second)
	tt.Frame()
	asked := 0
	for range 60 {
		tt.Advance(time.Second / 60)
		if tt.FramePending() {
			asked++
			tt.Frame()
		}
	}
	return asked
}

// A list of the Feiniu platform whose pages are in asks for no more frames
// than a playlist of NetEase's: only the rows still being read shimmer, and
// a list read as it is scrolled to must not make an idle page busy.
func TestFnListFrameBudget(t *testing.T) {
	n := newTestApp()
	n.settings = defaultSettings()
	n.playlists[5] = &netease.Playlist{ID: 5, Name: "x", Tracks: []netease.Track{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}, {ID: 3, Name: "c"}}}
	n.route = route{page: pagePlaylist, id: 5}
	base := idleFrames(ui.NewTester(n.view, 1200, 800))

	e := fnTestApp(t)
	if got := idleFrames(ui.NewTester(e.a.view, 1200, 800)); got > base {
		t.Errorf("a library on show: %d frames a second, a playlist of NetEase's %d", got, base)
	}
}

// A search shows the artists and the albums the service finds as rows of
// cards, each with a way to see them all, and the songs under them.
func TestFnSearchSections(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	card := func(i int, name string) map[string]any {
		return map[string]any{"guid": name + strconv.Itoa(i), "name": name + strconv.Itoa(i), "trackCount": 3}
	}
	var asked atomic.Value
	e.routes["/search/album"] = func(q url.Values) any {
		asked.Store(q.Get("q"))
		var l []map[string]any
		for i := range 12 {
			l = append(l, card(i, "专辑"))
		}
		return map[string]any{"total": 12, "list": l}
	}
	e.routes["/search/artist"] = func(url.Values) any {
		return map[string]any{"total": 1, "list": []map[string]any{card(0, "歌手")}}
	}
	e.routes["/search/track"] = func(url.Values) any {
		return map[string]any{"total": 1, "list": []map[string]any{{"guid": "s1", "title": "七里香", "audioSpec": map[string]any{"format": "mp3"}}}}
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	a.search("周")
	for i := 0; i < 8; i++ {
		tt.Advance(time.Second)
		tt.Frame()
		e.drain(t, 0)
		if a.fn.search != nil && a.fn.search.pg.total >= 0 && a.fn.searchAlbums.total >= 0 && a.fn.searchArtists.total >= 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.drain(t, 0)
	tt.Frame()
	if asked.Load() != "周" {
		t.Errorf("the albums were asked for %v", asked.Load())
	}
	for _, want := range []string{"歌手", "专辑", "歌曲", "七里香", "歌手0", "专辑0"} {
		if _, ok := tt.Find(want); !ok && !tt.HasText(want) {
			t.Errorf("no %q on the search page", want)
		}
	}
	// One row of the albums shows; there are more, so all of them are a click away.
	if _, ok := tt.Find("专辑5"); ok {
		t.Error("the row of albums holds more than a row")
	}
	if err := tt.Click("查看全部专辑"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pageFnFoundAlbums {
		t.Fatalf("route %+v", a.route)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if _, ok := tt.Find("专辑5"); !ok {
		t.Error("the albums found are not all on their page")
	}
}

// The roam has a page and a place in the sidebar, and leaving it is as
// leaving any radio.
func TestFnRoamPage(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("漫游"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pageFnRoam {
		t.Fatalf("route %+v", a.route)
	}
	if _, ok := tt.Find("开始漫游"); !ok {
		t.Error("no way to start the roam")
	}
}
