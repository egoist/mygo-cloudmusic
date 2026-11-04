package player

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// Quiescing cancels an active writer and prevents late loads from writing to the old cache.
func TestCacheQuiesceWriter(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/late" {
			w.Write([]byte("song"))
			return
		}
		w.Write([]byte("song"))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()
	c := NewCache(t.TempDir(), 1<<20)
	st := &netease.Stream{URL: srv.URL, Size: 100, Type: "mp3", Level: "exhigh"}
	dl := c.download(1, st)
	<-started
	dl.waitFor(4)
	unblock := c.Quiesce()
	defer unblock()
	dl.mu.Lock()
	open := dl.file != nil
	dl.mu.Unlock()
	if open {
		t.Fatal("writer still holds a cache file")
	}
	if entries, err := os.ReadDir(c.dir); err != nil || len(entries) != 0 {
		t.Fatalf("unfinished file remains: %v %v", entries, err)
	}
	late := c.download(1, &netease.Stream{URL: srv.URL + "/late", Size: 4, Type: "mp3", Level: "exhigh"})
	late.waitFor(4)
	late.halt()
	if entries, err := os.ReadDir(c.dir); err != nil || len(entries) != 0 {
		t.Fatalf("stale load wrote old cache: %v %v", entries, err)
	}
}

func seedCache(t *testing.T, c *Cache, id int64, data []byte) {
	t.Helper()
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	key := cacheKey(id, "exhigh")
	meta, _ := json.Marshal(netease.Stream{Size: int64(len(data)), Type: "mp3", Level: "exhigh"})
	for name, b := range map[string][]byte{key + ".mp3": data, key + ".json": meta} {
		if err := os.WriteFile(filepath.Join(c.dir, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// Shared decoders release old file handles too, while clearing/pruning cannot race migration.
func TestCacheQuiesceSharedReader(t *testing.T) {
	c := NewCache(t.TempDir(), 1<<20)
	seedCache(t, c, 1, []byte("music"))
	dl, _, ok := c.lookup(1, "exhigh")
	if !ok || !dl.share() {
		t.Fatal("could not open shared cached song")
	}
	unblock := c.Quiesce()
	c.Clear()
	c.SetLimit(0)
	if _, _, ok := c.lookup(1, "exhigh"); ok {
		t.Fatal("lookup bypassed quiescence")
	}
	from, to := filepath.Join(c.dir, "1-exhigh.mp3"), filepath.Join(c.dir, "held.mp3")
	if err := os.Rename(from, to); err != nil {
		t.Fatalf("old file still in use: %v", err)
	}
	if err := os.Rename(to, from); err != nil {
		t.Fatal(err)
	}
	dl.mu.Lock()
	if dl.file != nil || dl.n != 0 {
		t.Error("shared reader still accesses old cache")
	}
	dl.mu.Unlock()
	dl.release()
	unblock()
	restored, _, ok := c.lookup(1, "exhigh")
	if !ok {
		t.Fatal("old cache did not recover after rollback")
	}
	restored.Close()
}

type offlineTransport struct{}

func (offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network disabled in cache suspension test")
}

func waitCachePlayer(t *testing.T, p *Player, posts chan func()) {
	t.Helper()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for !p.Ready() || !p.lyricsLoaded {
		select {
		case fn := <-posts:
			fn()
		case <-timeout.C:
			t.Fatal("cached player did not resume")
		}
	}
}

// Both playing and paused tracks resume at the preserved position using the new cache.
func TestSuspendCacheResume(t *testing.T) {
	old := http.DefaultTransport
	http.DefaultTransport = offlineTransport{}
	defer func() { http.DefaultTransport = old }()
	data, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	for _, playing := range []bool{false, true} {
		posts := make(chan func(), 16)
		p := New(func(fn func()) { posts <- fn })
		p.Cache = NewCache(t.TempDir(), 1<<20)
		seedCache(t, p.Cache, 1, data)
		p.Cue([]netease.Track{{ID: 1, Duration: 10000}})
		p.loadSkipping(0, 0, playing, 250*time.Millisecond)
		waitCachePlayer(t, p, posts)
		resume := p.SuspendCache()
		position := p.Position()
		if p.Ready() || p.Playing() != playing || position < 250*time.Millisecond {
			t.Fatal("did not suspend at current position")
		}
		p.Cache.Quiesce()
		p.Cache = NewCache(t.TempDir(), 1<<20)
		seedCache(t, p.Cache, 1, data)
		resume()
		waitCachePlayer(t, p, posts)
		delta := p.Position() - position
		if p.Playing() != playing || delta < 0 || delta > 200*time.Millisecond {
			t.Fatalf("resume playing=%v at %v, wanted playing=%v near %v", p.Playing(), p.Position(), playing, position)
		}
		p.out.Close()
	}
}

// Queue, seek and pause commands during migration are deferred rather than starting new loads.
func TestSuspendCacheDefersControls(t *testing.T) {
	old := http.DefaultTransport
	http.DefaultTransport = offlineTransport{}
	defer func() { http.DefaultTransport = old }()
	data, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	posts := make(chan func(), 16)
	p := New(func(fn func()) { posts <- fn })
	p.Cache = NewCache(t.TempDir(), 1<<20)
	seedCache(t, p.Cache, 1, data)
	seedCache(t, p.Cache, 2, data)
	p.Cue([]netease.Track{{ID: 1, Duration: 10000}, {ID: 2, Duration: 10000}})
	p.loadSkipping(0, 0, true, 0)
	waitCachePlayer(t, p, posts)
	resume := p.SuspendCache()
	unblock := p.Cache.Quiesce()
	p.Next()
	p.Seek(500 * time.Millisecond)
	p.TogglePlay()
	if p.Ready() || p.Current().ID != 2 || p.Position() != 500*time.Millisecond {
		t.Fatal("controls were not deferred")
	}
	unblock()
	resume()
	waitCachePlayer(t, p, posts)
	defer p.out.Close()
	if p.Playing() || p.Current().ID != 2 || p.Position() != 500*time.Millisecond {
		t.Fatal("deferred controls were lost")
	}
}
