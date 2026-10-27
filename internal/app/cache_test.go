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
