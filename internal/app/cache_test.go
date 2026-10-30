package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// 清除 in the settings clears every cache, not the songs alone: the covers
// and the transcripts of episodes go too.
func TestClearAllCaches(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathCache, dir)
	defer mygo.App.SetPath(mygo.PathCache, "")
	a := newTestApp()
	a.settings = defaultSettings()
	a.art.dir = artDir()
	a.pl.Cache = newSongCache(a.settings.CacheMB)
	a.cacheAll = 3
	files := []string{"songs/1-exhigh.mp3", "covers/0123456789abcdef", "transcripts/1.json"}
	for _, f := range files {
		path := filepath.Join(dir, f)
		if os.MkdirAll(filepath.Dir(path), 0o700) != nil || os.WriteFile(path, []byte("x"), 0o600) != nil {
			t.Fatal("can't write", path)
		}
	}
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	if err := tt.Click("清除"); err != nil {
		t.Fatal(err)
	}
	// It clears on a goroutine.
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		left := 0
		for _, f := range files {
			if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
				left++
			}
		}
		if left == 0 {
			return
		}
	}
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("%s still kept", f)
		}
	}
}

// With 歌曲缓存 turned off, the covers kept on disk go, and those that load
// after aren't kept: they were, up to 200 MB, whatever it said.
func TestCacheOffKeepsNoCovers(t *testing.T) {
	cover, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	defer func(f func(string) ([]byte, error)) { fetchCover = f }(fetchCover)
	fetchCover = func(string) ([]byte, error) { return cover, nil }
	a := newTestApp()
	a.settings = defaultSettings()
	posted := make(chan func(), 8)
	a.art = newArtCache(func(f func()) { posted <- f }, t.TempDir())
	kept := filepath.Join(a.art.dir, "0123456789abcdef")
	if err := os.WriteFile(kept, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Not through the select: what it changes applies the appearance too,
	// which waits for a main loop the test hasn't got.
	a.settings.CacheMB = 0
	a.applyCacheLimit()
	// It clears on a goroutine.
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(a.art.dir); err != nil {
			break
		}
	}
	if _, err := os.Stat(kept); err == nil {
		t.Error("cover kept before still there")
	}
	a.art.get("cover", false)
	(<-posted)()
	if _, err := os.Stat(a.art.dir); err == nil {
		t.Error("cover loaded after kept on disk")
	}
}
