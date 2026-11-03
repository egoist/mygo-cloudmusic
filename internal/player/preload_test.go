package player

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// songServer serves the song b as the stream of each track asked for, and
// counts the requests each track got.
func songServer(t *testing.T, b []byte) (*httptest.Server, func(id int64) int) {
	t.Helper()
	var mu sync.Mutex
	hits := map[int64]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/"), 10, 64)
		mu.Lock()
		hits[id]++
		mu.Unlock()
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv, func(id int64) int {
		mu.Lock()
		defer mu.Unlock()
		return hits[id]
	}
}

// serveURL makes songURL answer with a stream of the song id at srv.
func serveURL(t *testing.T, srv *httptest.Server, size int) {
	t.Helper()
	old := songURL
	songURL = func(id int64, level string) (*netease.Stream, error) {
		return &netease.Stream{URL: fmt.Sprintf("%s/%d", srv.URL, id), Size: int64(size), Level: level, Type: "mp3"}, nil
	}
	t.Cleanup(func() { songURL = old })
}

// pump runs what the player posts on the test's goroutine, until done is
// true: a player only changes as its posts run.
func pump(t *testing.T, ch chan func(), what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("waited for %s", what)
		}
		select {
		case fn := <-ch:
			fn()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// newLoadPlayer returns a player of tracks, its songs in a cache, with the
// sound off: it plays for real, as the load does.
func newLoadPlayer(t *testing.T, cache *Cache, tracks ...netease.Track) (chan func(), *Player) {
	t.Helper()
	ch := make(chan func(), 64)
	p := New(func(fn func()) { ch <- fn })
	p.Cache = cache
	p.volume = 0
	t.Cleanup(func() {
		p.stop()
		p.dropAhead()
		// Nothing new starts once the player has moved on, but what came
		// in, and what plays, let go of their files on other goroutines: a
		// moment after, the audio closes, and a fetch posts. The test's
		// cache cannot go while they hold it.
		p.gen++
		time.Sleep(150 * time.Millisecond)
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if !run(ch) && !comingIn(cache) {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	})
	return ch, p
}

// run runs what the player posted, and reports whether anything was.
func run(ch chan func()) bool {
	any := false
	for {
		select {
		case fn := <-ch:
			fn()
			any = true
		default:
			return any
		}
	}
}

// comingIn reports whether a song still comes into the cache's directory.
func comingIn(c *Cache) bool {
	if c == nil {
		return false
	}
	parts, _ := filepath.Glob(filepath.Join(c.dir, "*.part"))
	return len(parts) > 0
}

// song returns the song the tests play: five seconds of MP3.
func song(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The track after this one is fetched as this one plays, so that it plays
// at once without asking for it again, and says it plays rather than that
// it still loads: the row kept showing 正在缓冲… while it played.
func TestPrefetchNext(t *testing.T) {
	b := song(t)
	srv, hits := songServer(t, b)
	serveURL(t, srv, len(b))
	c := NewCache(t.TempDir(), 1<<30)
	ch, p := newLoadPlayer(t, c, netease.Track{ID: 1, Duration: 5000}, netease.Track{ID: 2, Duration: 5000})
	p.PlayQueue([]netease.Track{{ID: 1, Duration: 5000}, {ID: 2, Duration: 5000}}, 0)

	pump(t, ch, "the next track fetched ahead", func() bool { return p.ahead != nil && hits(2) == 1 })
	if n := hits(2); n != 1 {
		t.Fatalf("asked for the next track %d times", n)
	}
	// Fetched, it is kept, which is what makes it play at once.
	pump(t, ch, "the next track kept", func() bool {
		dl, _, ok := c.lookup(2, "exhigh")
		if ok {
			dl.Close()
		}
		return ok
	})
	asked := hits(2)
	p.Next()
	pump(t, ch, "the next track playing", func() bool { return p.out != nil && p.Current().ID == 2 })
	if n := hits(2); n != asked {
		t.Errorf("asked for the track fetched ahead again: %d requests", n-asked)
	}
	if p.Loading() {
		t.Error("still says it loads, as it plays")
	}
}

// A track that does not load is asked for again, rather than skipped as if
// it could not play: a slow or dropped connection is not an unavailable
// song.
func TestLoadRetries(t *testing.T) {
	b := song(t)
	srv, _ := songServer(t, b)
	old := songURL
	var n atomic.Int64
	songURL = func(id int64, level string) (*netease.Stream, error) {
		if n.Add(1) <= 2 {
			return nil, errors.New("connection dropped")
		}
		return &netease.Stream{URL: fmt.Sprintf("%s/%d", srv.URL, id), Size: int64(len(b)), Level: level, Type: "mp3"}, nil
	}
	defer func() { songURL = old }()
	c := NewCache(t.TempDir(), 1<<30)
	ch, p := newLoadPlayer(t, c, netease.Track{ID: 1, Duration: 5000}, netease.Track{ID: 2, Duration: 5000})
	p.PlayQueue([]netease.Track{{ID: 1, Duration: 5000}, {ID: 2, Duration: 5000}}, 0)

	pump(t, ch, "the track to load", func() bool { return p.out != nil })
	if id := p.Current().ID; id != 1 {
		t.Errorf("skipped to track %d", id)
	}
	if n := n.Load(); n < 3 {
		t.Errorf("asked %d times", n)
	}
}

// A track that does not load at all stops there, with what went wrong on
// show, rather than playing the one after it.
func TestLoadFailureDoesNotSkip(t *testing.T) {
	b := song(t)
	_, hits := songServer(t, b)
	old := songURL
	songURL = func(id int64, level string) (*netease.Stream, error) {
		return nil, errors.New("connection dropped")
	}
	defer func() { songURL = old }()
	c := NewCache(t.TempDir(), 1<<30)
	ch, p := newLoadPlayer(t, c, netease.Track{ID: 1, Duration: 5000}, netease.Track{ID: 2, Duration: 5000})
	p.PlayQueue([]netease.Track{{ID: 1, Duration: 5000}, {ID: 2, Duration: 5000}}, 0)

	pump(t, ch, "the load to give up", func() bool { return !p.loading && p.err != "" })
	if id := p.Current().ID; id != 1 {
		t.Errorf("skipped to track %d", id)
	}
	if p.Playing() {
		t.Error("plays nothing, as if playing")
	}
	if n := hits(2); n != 0 {
		t.Errorf("asked for the next track %d times", n)
	}
}

// cutServer serves the song, cut off halfway, as a flaky connection does.
func cutServer(t *testing.T, b []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.Write(b[:len(b)/2])
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if hj, ok := w.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				conn.Close()
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// cutStream returns what played of a song whose download was cut off
// halfway, as a stream of the player's: a song of id, from url.
func cutStream(t *testing.T, c *Cache, id int64, url string, size int) stream {
	t.Helper()
	dl := c.download(id, &netease.Stream{URL: url, Size: int64(size), Level: "exhigh", Type: "mp3"})
	dl.waitFor(int64(size))
	if dl.Err() == nil {
		t.Fatal("the cut download ended whole")
	}
	s, err := openStream(dl, 5000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() }) // what came of it, closed as a stream of the player's would be
	return s
}

// A download cut off as it plays is asked for again, and the song goes on
// from there: it used to end where the bytes did, halfway through, and the
// next track played.
func TestCutOffRetries(t *testing.T) {
	b := song(t)
	whole, _ := songServer(t, b)
	cut := cutServer(t, b)
	old := songURL
	songURL = func(id int64, level string) (*netease.Stream, error) {
		return &netease.Stream{URL: fmt.Sprintf("%s/%d", whole.URL, id), Size: int64(len(b)), Level: level, Type: "mp3"}, nil
	}
	defer func() { songURL = old }()

	c := NewCache(t.TempDir(), 1<<30)
	ch, p := newLoadPlayer(t, c, netease.Track{ID: 1, Duration: 5000}, netease.Track{ID: 2, Duration: 5000})
	p.Cue([]netease.Track{{ID: 1, Duration: 5000}, {ID: 2, Duration: 5000}})
	p.stream = cutStream(t, c, 1, cut.URL, len(b))

	p.cutOff() // what plays ended, and the download is why
	if id := p.Current().ID; id != 1 {
		t.Fatalf("moved on to track %d", id)
	}
	if p.tries != 1 {
		t.Errorf("tried %d times", p.tries)
	}
	pump(t, ch, "the track again", func() bool { return p.out != nil })
	if id := p.Current().ID; id != 1 {
		t.Errorf("plays track %d", id)
	}
}

// A song whose download keeps breaking off is left where it stopped, with
// what went wrong on show: its tries used to start over each time it loaded
// again, so a song that kept breaking off was retried for as long as the
// listener waited.
func TestCutOffStops(t *testing.T) {
	b := song(t)
	whole, _ := songServer(t, b)
	cut := cutServer(t, b)
	old := songURL
	songURL = func(id int64, level string) (*netease.Stream, error) {
		return &netease.Stream{URL: fmt.Sprintf("%s/%d", whole.URL, id), Size: int64(len(b)), Level: level, Type: "mp3"}, nil
	}
	defer func() { songURL = old }()

	c := NewCache(t.TempDir(), 1<<30)
	ch, p := newLoadPlayer(t, c, netease.Track{ID: 1, Duration: 5000}, netease.Track{ID: 2, Duration: 5000})
	p.Cue([]netease.Track{{ID: 1, Duration: 5000}, {ID: 2, Duration: 5000}})
	// Broken off, asked for again, and broken off again: once for every try
	// it gets, and then it is left where it stopped.
	for i := int64(1); i <= loadTries+1; i++ {
		gen := p.gen
		p.stream = cutStream(t, c, 100+i, cut.URL, len(b))
		p.cutOff()
		if p.err != "" {
			break // left where it stopped
		}
		pump(t, ch, "the track again", func() bool { return p.gen != gen })
	}

	if p.Playing() {
		t.Error("plays on, as it cannot be had")
	}
	if p.err == "" {
		t.Error("does not say what went wrong")
	}
	if id := p.Current().ID; id != 1 {
		t.Errorf("moved on to track %d", id)
	}
}

// A song whose bytes all came in plays on to the next one when it ends.
func TestSongEndPlaysNext(t *testing.T) {
	b := song(t)
	srv, _ := songServer(t, b)
	serveURL(t, srv, len(b))
	c := NewCache(t.TempDir(), 1<<30)
	ch, p := newLoadPlayer(t, c, netease.Track{ID: 1, Duration: 5000}, netease.Track{ID: 2, Duration: 5000})
	p.Cue([]netease.Track{{ID: 1, Duration: 5000}, {ID: 2, Duration: 5000}})
	s, err := openStream(downloaded(b), 5000)
	if err != nil {
		t.Fatal(err)
	}
	p.stream = s

	p.cutOff()
	pump(t, ch, "the next track", func() bool { return p.out != nil && p.Current().ID == 2 })
}

// The track after this one is fetched only once this one has come in whole:
// fetched sooner, it would take the bytes of the song being played.
func TestPrefetchWaitsForThisOne(t *testing.T) {
	b := song(t)
	rest := make(chan struct{})
	var mu sync.Mutex
	hits := map[int64]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/"), 10, 64)
		mu.Lock()
		hits[id]++
		mu.Unlock()
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		if id != 1 {
			w.Write(b)
			return
		}
		// Half of it, and the rest only when the test lets it come.
		w.Write(b[:len(b)/2])
		w.(http.Flusher).Flush()
		<-rest
		w.Write(b[len(b)/2:])
	}))
	t.Cleanup(srv.Close)
	serveURL(t, srv, len(b))
	c := NewCache(t.TempDir(), 1<<30)
	ch, p := newLoadPlayer(t, c, netease.Track{ID: 1, Duration: 5000}, netease.Track{ID: 2, Duration: 5000})
	p.PlayQueue([]netease.Track{{ID: 1, Duration: 5000}, {ID: 2, Duration: 5000}}, 0)

	pump(t, ch, "the track playing", func() bool { return p.out != nil })
	mu.Lock()
	n := hits[2]
	mu.Unlock()
	if n != 0 {
		t.Errorf("asked for the next track %d times, as this one still comes in", n)
	}
	close(rest)
	pump(t, ch, "the next track fetched ahead", func() bool { return p.ahead != nil })
}
