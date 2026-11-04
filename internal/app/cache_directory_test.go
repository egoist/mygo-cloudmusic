package app

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

func cacheTestApp(t *testing.T) (*app, string, string) {
	t.Helper()
	root := t.TempDir()
	source, target := filepath.Join(root, "old"), filepath.Join(root, "new")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = source
	a.settings.path = filepath.Join(root, "settings.json")
	return a, source, target
}

func readCacheSettings(t *testing.T, path string) settings {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s settings
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// Choosing a directory queues a persisted request, without moving an active cache; cancel undoes it.
func TestCacheDirectoryControls(t *testing.T) {
	a, source, target := cacheTestApp(t)
	oldOpen, oldAsk := openCacheDirectory, askCacheDirectory
	defer func() { openCacheDirectory, askCacheDirectory = oldOpen, oldAsk }()
	openCacheDirectory = func(mygo.OpenDialogOptions) ([]string, error) { return []string{target}, nil }
	askCacheDirectory = func(mygo.MessageOptions) (mygo.MessageResult, error) { return mygo.MessageResult{Button: 1}, nil }
	tt := ui.NewTester(a.settingsView, 800, 1800)
	if err := tt.Click("更改歌曲缓存目录"); err != nil {
		t.Fatal(err)
	}
	s := readCacheSettings(t, a.settings.path)
	if s.CacheDir != source || s.CacheChange == nil || s.CacheChange.Directory != target || !s.CacheChange.Migrate {
		t.Fatalf("request not saved: %+v", s)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("moved before restart: %v", err)
	}
	if err := tt.Click("取消更改"); err != nil {
		t.Fatal(err)
	}
	s = readCacheSettings(t, a.settings.path)
	if s.CacheChange != nil || s.CacheDir != source {
		t.Fatalf("cancel changed active directory: %+v", s)
	}
}

// Canceling either native dialog leaves the current path and pending request unchanged.
func TestCacheDirectoryDialogCancel(t *testing.T) {
	for _, cancelPicker := range []bool{true, false} {
		a, source, target := cacheTestApp(t)
		oldOpen, oldAsk := openCacheDirectory, askCacheDirectory
		openCacheDirectory = func(mygo.OpenDialogOptions) ([]string, error) {
			if cancelPicker {
				return nil, nil
			}
			return []string{target}, nil
		}
		askCacheDirectory = func(mygo.MessageOptions) (mygo.MessageResult, error) { return mygo.MessageResult{Button: 0}, nil }
		a.chooseCacheDirectory()
		openCacheDirectory, askCacheDirectory = oldOpen, oldAsk
		if a.settings.CacheDir != source || a.settings.CacheChange != nil {
			t.Fatal("cancel changed settings")
		}
	}
}

// A new process applies migration before playback, saves the new path and clears the request.
func TestCacheDirectoryRestart(t *testing.T) {
	a, source, target := cacheTestApp(t)
	meta, _ := json.Marshal(netease.Stream{Size: 5, Type: "mp3", Level: "exhigh"})
	for name, data := range map[string][]byte{"123-exhigh.mp3": []byte("music"), "123-exhigh.json": meta} {
		if err := os.WriteFile(filepath.Join(source, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.queueCacheDirectory(target, true); err != nil {
		t.Fatal(err)
	}
	b := newTestApp()
	b.settings = readCacheSettings(t, a.settings.path)
	b.settings.path = a.settings.path
	b.applyCacheDirectoryChange()
	if b.cacheError != "" || b.settings.CacheDir != target || b.settings.CacheChange != nil {
		t.Fatalf("restart failed: %s %+v", b.cacheError, b.settings)
	}
	saved := readCacheSettings(t, a.settings.path)
	if saved.CacheDir != target || saved.CacheChange != nil {
		t.Fatalf("new path not persisted: %+v", saved)
	}
	if _, err := os.Stat(filepath.Join(target, "123-exhigh.mp3")); err != nil {
		t.Fatal(err)
	}
	b.pl.Cache = newSongCache(b.settings.CacheMB, b.settings.CacheDir)
	if b.pl.Cache == nil || b.pl.Cache.Size() != 5 {
		t.Fatal("player did not use the migrated cache")
	}
	if _, err := os.Stat(filepath.Join(source, "123-exhigh.mp3")); !os.IsNotExist(err) {
		t.Fatalf("old cache remains: %v", err)
	}
}

// A target filled after scheduling fails safely and its request can still be canceled.
func TestCacheDirectoryRestartFailure(t *testing.T) {
	a, source, target := cacheTestApp(t)
	if err := a.queueCacheDirectory(target, true); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "personal.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.applyCacheDirectoryChange()
	if a.cacheError == "" || a.settings.CacheDir != source || a.settings.CacheChange == nil {
		t.Fatal("failure lost old setting or pending request")
	}
	if err := a.cancelCacheDirectoryChange(); err != nil {
		t.Fatal(err)
	}
	if a.settings.CacheChange != nil {
		t.Fatal("could not cancel failed migration")
	}
}

// A settings write failure cannot enqueue an unsaved directory change.
func TestCacheDirectorySaveFailure(t *testing.T) {
	a, source, target := cacheTestApp(t)
	a.settings.path = source // a directory cannot be replaced by settings.json
	if err := a.queueCacheDirectory(target, true); err == nil {
		t.Fatal("ignored settings save failure")
	}
	if a.settings.CacheDir != source || a.settings.CacheChange != nil {
		t.Fatal("applied unsaved settings")
	}
}

// With NM_FRAMES set, saves the settings with a pending move for visual inspection.
func TestCacheDirectoryFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	a, _, target := cacheTestApp(t)
	a.settings.CacheChange = &cacheDirectoryChange{Directory: target, Migrate: true}
	tt := ui.NewTester(a.settingsView, 560, 1000)
	tt.Frame()
	f, err := os.Create(filepath.Join(dir, "cache-directory.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
