package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
		raw, _ := io.ReadAll(r.Body)
		if r.Method == "POST" && path != "/user/password-login" {
			e.asked <- "POST " + path + " " + string(raw)
		}
		if route := e.routes[path]; route != nil {
			v := route(r.URL.Query())
			if err, ok := v.(error); ok {
				// What the service refuses.
				json.NewEncoder(w).Encode(map[string]any{"code": 1, "msg": err.Error()})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": v})
			return
		}
		var body map[string]any
		json.Unmarshal(raw, &body)
		if strings.HasPrefix(path, "/favorite-track/") {
			e.asked <- "like " + path + " " + fmt.Sprint(body["trackGUID"])
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
	// An address has its protocol chosen, and may have an access code too.
	if _, ok := tt.Find("访问码（可选）"); !ok {
		t.Error("no field for the access code of an address")
	}
	if _, ok := tt.Find("FN Connect 访问码（可选）"); ok {
		t.Error("the access code of FN Connect is asked for an address")
	}
	if a.fn.form.https {
		t.Fatal("the protocol does not start as http")
	}
	if err := tt.Click("https"); err != nil {
		t.Fatal(err)
	}
	if !a.fn.form.https {
		t.Error("the switch did not choose https")
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

// The scheme typed in an address is the protocol, and the address is kept
// without it; with none typed, the protocol chosen holds.
func TestSplitScheme(t *testing.T) {
	for _, c := range []struct {
		in    string
		https bool
		want  string
		wantS bool
	}{
		{"192.168.1.10:5666", false, "192.168.1.10:5666", false},
		{"192.168.1.10:5667", true, "192.168.1.10:5667", true},
		{"https://172.16.1.10:18154", false, "172.16.1.10:18154", true},
		{"HTTP://nas.example.com/", true, "nas.example.com", false},
		{" https://nas.example.com/proxy/ ", false, "nas.example.com/proxy", true},
		{"nas.example.com", true, "nas.example.com", true},
	} {
		got, https := splitScheme(c.in, c.https)
		if got != c.want || https != c.wantS {
			t.Errorf("splitScheme(%q, %v) = %q, %v; want %q, %v", c.in, c.https, got, https, c.want, c.wantS)
		}
	}
}

// The search is a visit, not a place of the sidebar's: its entry is there
// while the results are shown and goes when the listener leaves them, which
// it did not, ever, before; and emptying the search box leaves the search for
// the page before it.
func TestSearchIsAVisit(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	if _, ok := tt.Find("搜索结果"); ok {
		t.Fatal("an entry for a search not made")
	}

	a.query = "周"
	tt.Frame()
	a.search("周")
	tt.Frame()
	if _, ok := tt.Find("搜索结果"); !ok {
		t.Error("no entry while the results are shown")
	}
	// Leaving for another page takes the entry with it, and back brings both.
	if err := tt.Click("专辑"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("搜索结果"); ok {
		t.Error("the entry stays after the listener left the results")
	}
	a.back()
	tt.Frame()
	if a.route.page != pageSearch {
		t.Fatalf("back did not return to the results: %+v", a.route)
	}
	if _, ok := tt.Find("搜索结果"); !ok {
		t.Error("no entry when back to the results")
	}

	// Emptying the box leaves them, for the page the search was made from.
	a.query = ""
	tt.Frame()
	if want := (route{page: pagePlaylist, id: fnmusic.LibraryID}); a.route != want {
		t.Errorf("after emptying the box: %+v, want %+v", a.route, want)
	}
	if _, ok := tt.Find("搜索结果"); ok {
		t.Error("the entry stays after the box was emptied")
	}
}

// A search made from a search is the same visit: going back leaves the
// search, not the one before it.
func TestSearchAgainDoesNotStack(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	a.search("一")
	a.search("二")
	a.search("三")
	if len(a.history) != 1 {
		t.Fatalf("%d pages of history after three searches", len(a.history))
	}
	a.back()
	if a.route.page != pagePlaylist {
		t.Errorf("back from the searches: %+v", a.route)
	}
}

// asks drains what the service was asked, and returns those that begin with
// prefix.
func (e *fnEnv) asks(prefix string) []string {
	var out []string
	for len(e.asked) > 0 {
		if s := <-e.asked; strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}

// playlistsTestApp is an app showing the Feiniu platform, with two playlists
// of the listener's, one of them on show with three songs in it.
func playlistsTestApp(t *testing.T) (*fnEnv, *fnList) {
	e := fnTestApp(t)
	a := e.a
	a.fn.playlists = []netease.Playlist{
		{ID: fnmusic.ID("p1"), Ref: "p1", Name: "通勤"},
		{ID: fnmusic.ID("p2"), Ref: "p2", Name: "睡前"},
	}
	l := a.newFnList(fnSpec{typ: listPlaylist, ref: "p1"})
	l.name = "通勤"
	l.pg = a.trackPager(func(int) ([]netease.Track, int, error) { return nil, 0, errors.New("unexpected request") })
	l.pg.land(0, []netease.Track{fnTrack("a", "晴天", "周杰伦", "flac"), fnTrack("b", "七里香", "周杰伦", "mp3"), fnTrack("c", "宝船", "DOUDOU", "mp3")}, 3)
	a.fn.lists[l.id] = l
	a.route = route{page: pagePlaylist, id: l.id}
	return e, l
}

// A playlist is made from the name given, and shown, and the songs it was
// made for go in it; it is the first of the listener's, as the service lists
// the newest.
func TestFnCreatePlaylist(t *testing.T) {
	e, _ := playlistsTestApp(t)
	a := e.a
	e.routes["/playlist/create"] = func(url.Values) any { return map[string]any{"guid": "p3", "name": "通勤"} }
	a.newFnPlaylist([]netease.Track{fnTrack("a", "晴天", "周杰伦", "flac")})
	if !a.naming.open || !a.naming.fn {
		t.Fatal("the name is not asked for")
	}
	a.naming.name = "  跑步  "
	a.submitName()
	e.drain(t, 1)
	if a.naming.open || a.fn.playlists[0].Ref != "p3" || len(a.fn.playlists) != 3 {
		t.Fatalf("dialog open %v, playlists %+v", a.naming.open, a.fn.playlists)
	}
	if a.route != (route{page: pagePlaylist, id: fnmusic.ID("p3")}) {
		t.Errorf("route %+v", a.route)
	}
	e.drain(t, 2) // the songs added, and the first page of the new playlist
	posts := e.asks("POST /playlist/")
	if len(posts) != 2 || !strings.Contains(posts[0], `"name":"跑步"`) ||
		!strings.Contains(posts[1], "add-track") || !strings.Contains(posts[1], `"guid":"p3"`) || !strings.Contains(posts[1], `"trackGUIDs":["a"]`) {
		t.Errorf("asked %q", posts)
	}
	// A name that is blank asks nothing.
	a.newFnPlaylist(nil)
	a.naming.name = "   "
	a.submitName()
	if len(e.asks("POST /playlist/create")) != 0 || a.naming.busy {
		t.Error("a blank name was sent")
	}
}

// Renaming shows at once, and is taken back if the service refuses.
func TestFnRenamePlaylist(t *testing.T) {
	e, l := playlistsTestApp(t)
	a := e.a
	pl, _ := a.fnPlaylist(l.id)
	a.startFnRename(pl)
	if a.naming.name != "通勤" || a.naming.rename != l.id {
		t.Fatalf("dialog %+v", a.naming)
	}
	a.naming.name = "上班"
	a.submitName()
	if got, _ := a.fnPlaylist(l.id); got.Name != "上班" || l.name != "上班" {
		t.Errorf("not renamed at once: %q, list %q", got.Name, l.name)
	}
	e.drain(t, 1)
	if posts := e.asks("POST /playlist/edit"); len(posts) != 1 || !strings.Contains(posts[0], `"guid":"p1"`) || !strings.Contains(posts[0], `"name":"上班"`) {
		t.Errorf("asked %q", posts)
	}
	// Refused: the name before comes back, which is the last that took.
	e.routes["/playlist/edit"] = func(url.Values) any { return errors.New("no") }
	a.startFnRename(pl)
	a.naming.name = "又改"
	a.submitName()
	if got, _ := a.fnPlaylist(l.id); got.Name != "又改" {
		t.Fatalf("not renamed at once: %q", got.Name)
	}
	e.drain(t, 1)
	if got, _ := a.fnPlaylist(l.id); got.Name != "上班" || l.name != "上班" || a.toast == "" {
		t.Errorf("name %q, list %q, toast %q", got.Name, l.name, a.toast)
	}
}

// Deleting asks first, takes the playlist out of the sidebar at once and the
// listener from its page, and brings it back if the service refuses.
func TestFnDeletePlaylist(t *testing.T) {
	e, l := playlistsTestApp(t)
	a := e.a
	a.history = []route{{page: pageFnAlbums}, a.route}
	pl, _ := a.fnPlaylist(l.id)
	a.confirmFnDelete(pl)
	if !a.deleting.open || !a.deleting.fn || a.deleting.name != "通勤" {
		t.Fatalf("not asked: %+v", a.deleting)
	}
	a.deleteChosen()
	if len(a.fn.playlists) != 1 || a.route.page != pageFnAlbums || len(a.history) != 0 {
		t.Errorf("playlists %d, route %+v, history %v", len(a.fn.playlists), a.route, a.history)
	}
	e.drain(t, 2) // the delete's, and the albums page it left for asking for its own
	if posts := e.asks("POST /playlist/delete"); len(posts) != 1 || !strings.Contains(posts[0], `"guid":"p1"`) {
		t.Errorf("asked %q", posts)
	}
	// Refused: it comes back where it was.
	e.routes["/playlist/delete"] = func(url.Values) any { return errors.New("no") }
	pl2, _ := a.fnPlaylist(fnmusic.ID("p2"))
	a.confirmFnDelete(pl2)
	a.deleteChosen()
	e.drain(t, 1)
	if len(a.fn.playlists) != 1 || a.fn.playlists[0].Ref != "p2" || a.toast == "" {
		t.Errorf("playlists %+v, toast %q", a.fn.playlists, a.toast)
	}
}

// From a song's menu in the library, a song goes in a playlist of the
// listener's, or a new one; and the menu of a playlist's own song offers
// taking it out, and not putting it in the playlist it is in.
func TestFnSongMenuPlaylists(t *testing.T) {
	e, l := playlistsTestApp(t)
	a := e.a
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	// In a playlist, the one it is in is not offered, the other is.
	if err := tt.ChooseMenuItem("添加到歌单", "通勤"); err == nil {
		t.Error("offered to add the song to the playlist it is in")
	}
	if err := tt.ChooseMenuItem("添加到歌单", "睡前"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	e.drain(t, 1)
	if posts := e.asks("POST /playlist/add-track"); len(posts) != 1 || !strings.Contains(posts[0], `"guid":"p2"`) || !strings.Contains(posts[0], `"trackGUIDs":["c"]`) {
		t.Errorf("asked %q", posts)
	}
	// And taken out of this one: at once, and the list is a song shorter.
	if err := tt.RightClick("七里香"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("从歌单中移除"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if l.pg.len() != 2 {
		t.Errorf("the list has %d songs", l.pg.len())
	}
	e.drain(t, 1)
	if posts := e.asks("POST /playlist/remove-track"); len(posts) != 1 || !strings.Contains(posts[0], `"guid":"p1"`) || !strings.Contains(posts[0], `"trackGUIDs":["b"]`) {
		t.Errorf("asked %q", posts)
	}
}

// Taking a song out that the service will not is taken back: the list is
// read again, with it.
func TestFnRemoveRefused(t *testing.T) {
	e, l := playlistsTestApp(t)
	a := e.a
	e.routes["/playlist/remove-track"] = func(url.Values) any { return errors.New("no") }
	a.removeFromFnPlaylist(l, 0)
	if l.pg.len() != 2 {
		t.Fatalf("not out at once: %d", l.pg.len())
	}
	e.drain(t, 1)
	if a.fn.lists[l.id] != nil || a.toast == "" {
		t.Errorf("the list was kept: %v, toast %q", a.fn.lists[l.id] != nil, a.toast)
	}
}

// Every song of an album goes in a playlist in one request; the album is the
// one list read whole, which is short.
func TestFnAddAlbumToPlaylist(t *testing.T) {
	e, _ := playlistsTestApp(t)
	a := e.a
	album := a.newFnList(fnSpec{typ: listAlbum, ref: "alb"})
	album.pg = a.trackPager(func(int) ([]netease.Track, int, error) { return nil, 0, errors.New("unexpected request") })
	album.pg.land(0, []netease.Track{fnTrack("x1", "一", "甲", "mp3"), fnTrack("x2", "二", "甲", "mp3")}, 2)
	pl, _ := a.fnPlaylist(fnmusic.ID("p2"))
	a.addListToFnPlaylist(album, pl)
	e.drain(t, 1)
	if posts := e.asks("POST /playlist/add-track"); len(posts) != 1 || !strings.Contains(posts[0], `"trackGUIDs":["x1","x2"]`) {
		t.Errorf("asked %q", posts)
	}
}

// A picture chosen is uploaded and made the playlist's cover, which then shows
// wherever the playlist does; none chosen asks nothing, and a file that is no
// picture is told so.
func TestFnPlaylistCover(t *testing.T) {
	e, l := playlistsTestApp(t)
	a := e.a
	real := pickCover
	defer func() { pickCover = real }()

	pickCover = func() ([]byte, error) { return nil, nil }
	pl, _ := a.fnPlaylist(l.id)
	a.changeFnCover(pl)
	time.Sleep(50 * time.Millisecond)
	if len(e.asked) != 0 || len(e.posted) != 0 {
		t.Error("a cover not chosen was asked of the service")
	}

	pickCover = func() ([]byte, error) { return []byte("not a picture"), nil }
	a.changeFnCover(pl)
	e.drain(t, 1)
	if a.toast != fnmusic.ErrBadCover.Error() {
		t.Errorf("toast %q", a.toast)
	}

	png := append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, make([]byte, 64)...)
	pickCover = func() ([]byte, error) { return png, nil }
	e.routes["/static/cover/playlist"] = func(url.Values) any { return map[string]any{"coverId": "playlist_c1"} }
	a.changeFnCover(pl)
	e.drain(t, 1)
	if got, _ := a.fnPlaylist(l.id); got.CoverURL != "fnmusic://cover/playlist_c1" || l.cover != got.CoverURL {
		t.Errorf("cover %q, list %q", got.CoverURL, l.cover)
	}
	// The name goes with it, which the service wants for any edit.
	if posts := e.asks("POST /playlist/edit"); len(posts) != 1 || !strings.Contains(posts[0], `"name":"通勤"`) || !strings.Contains(posts[0], `"coverId":"playlist_c1"`) {
		t.Errorf("asked %q", posts)
	}
}

// A playlist with no cover of its own wears its first song's, in the sidebar
// and on its page; one that has its own keeps it.
func TestFnPlaylistsWearTheirFirstSongsCover(t *testing.T) {
	e, _ := playlistsTestApp(t)
	a := e.a
	e.routes["/track/playlist-detail/list"] = func(q url.Values) any {
		return map[string]any{"total": 1, "list": []map[string]any{{"guid": "s", "title": "歌", "coverId": "track_1", "audioSpec": map[string]any{"format": "mp3"}}}}
	}
	lists := []netease.Playlist{
		{ID: fnmusic.ID("p1"), Ref: "p1", Name: "通勤"},
		{ID: fnmusic.ID("p2"), Ref: "p2", Name: "睡前", CoverURL: "fnmusic://cover/playlist_own"},
	}
	a.fn.playlists = lists
	a.loadFnCovers(a.fn.client, lists)
	e.drain(t, 1)
	if a.fn.playlists[0].CoverURL != "fnmusic://cover/track_1" || a.fn.playlists[1].CoverURL != "fnmusic://cover/playlist_own" {
		t.Errorf("covers %q, %q", a.fn.playlists[0].CoverURL, a.fn.playlists[1].CoverURL)
	}
}

// A song of the Feiniu platform is told to the service as it starts, which is
// what puts it in the play history there; a song of NetEase's is not, nor is
// anything told without a service. A failure is no matter.
func TestFnReportsWhatStarts(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	a.fn.lists[fnmusic.RecentID] = e.seed(fnmusic.RecentID, nil)
	tr := fnTrack("g9", "晴天", "周杰伦", "flac")
	a.reportStart(&tr)
	e.drain(t, 1)
	posts := e.asks("POST /event/report")
	if len(posts) != 1 || !strings.Contains(posts[0], `"eventType":"track_play"`) || !strings.Contains(posts[0], `"trackGUID":"g9"`) {
		t.Fatalf("asked %q", posts)
	}
	// The history as shown was read before: it is read anew when next shown.
	if a.fn.lists[fnmusic.RecentID] != nil {
		t.Error("the history shown is the old one")
	}

	other := netease.Track{ID: 5, Name: "宝船"}
	a.reportStart(&other)
	time.Sleep(50 * time.Millisecond)
	if len(e.asks("POST /event/report")) != 0 {
		t.Error("a song of NetEase's was told to the Feiniu service")
	}
	a.fn.client = nil
	a.reportStart(&tr) // no service: nothing, and no panic
}

// Reading the play history goes by pages as any list, and is the last played
// first; a song taken out goes at once, and the service is told.
func TestFnRecentPage(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	e.routes["/play-history/list"] = func(q url.Values) any {
		return map[string]any{"total": 2, "list": []map[string]any{
			{"guid": "r2", "title": "刚听的", "audioSpec": map[string]any{"format": "mp3"}},
			{"guid": "r1", "title": "先听的", "audioSpec": map[string]any{"format": "mp3"}},
		}}
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("最近播放"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	e.drain(t, 1)
	tt.Frame()
	if a.route != (route{page: pagePlaylist, id: fnmusic.RecentID}) {
		t.Fatalf("route %+v", a.route)
	}
	l := a.fn.lists[fnmusic.RecentID]
	if l == nil || l.name != "最近播放" || l.pg.len() != 2 {
		t.Fatalf("list %+v", l)
	}
	if _, ok := tt.Find("刚听的"); !ok {
		t.Error("the history is not shown")
	}
	if asked := e.asks("/play-history/list"); len(asked) != 1 || !strings.Contains(asked[0], "page=1") {
		t.Errorf("asked %q", asked)
	}

	// Taken out of the history: at once, and the service is told.
	if err := tt.RightClick("刚听的"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("从最近播放中移除"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if l.pg.len() != 1 {
		t.Errorf("the history has %d songs", l.pg.len())
	}
	e.drain(t, 1)
	if posts := e.asks("POST /play-history/delete"); len(posts) != 1 || !strings.Contains(posts[0], `"trackGUIDs":["r2"]`) {
		t.Errorf("asked %q", posts)
	}
}

// The roam picks songs the queue has already: it is asked on from them, as
// the queue does not take a song twice and a radio that was given nothing
// stops. And a chain the service no longer has is begun anew, as the service's
// own app does.
func TestFnRoamSkipsWhatIsQueued(t *testing.T) {
	e := fnTestApp(t)
	a := e.a
	step := func(id string) map[string]any {
		return map[string]any{"roamId": "id-" + id, "track": map[string]any{"guid": id, "title": id, "audioSpec": map[string]any{"format": "mp3"}}}
	}
	a.pl.Cue([]netease.Track{fnTrack("r1", "r1", "x", "mp3"), fnTrack("r2", "r2", "x", "mp3")})
	// After r1 comes r2, which is queued; after that, r3.
	e.routes["/track/roam-next"] = func(q url.Values) any {
		if q.Get("relativeRoamId") == "id-r1" {
			return map[string]any{"current": step("r2")}
		}
		return map[string]any{"current": step("r3")}
	}
	a.fn.roamID = "id-r1"
	var got []netease.Track
	a.roamFeed(func(more []netease.Track) { got = more })
	for len(got) == 0 {
		e.drain(t, 1)
	}
	if got[0].Ref != "r3" || a.fn.roamID != "id-r3" {
		t.Errorf("gave %q, roam at %q", got[0].Ref, a.fn.roamID)
	}

	// A chain gone: the roam begins anew.
	e.routes["/track/roam-next"] = func(url.Values) any { return errors.New("expired") }
	e.routes["/track/roam-start"] = func(url.Values) any { return map[string]any{"current": step("n1")} }
	got = nil
	a.fn.roamID = "id-stale"
	a.roamFeed(func(more []netease.Track) { got = more })
	for len(got) == 0 {
		e.drain(t, 1)
	}
	if got[0].Ref != "n1" || a.fn.roamID != "id-n1" {
		t.Errorf("gave %q, roam at %q", got[0].Ref, a.fn.roamID)
	}
}
