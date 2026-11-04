package cachemove

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func song(t *testing.T, dir string) {
	t.Helper()
	meta, _ := json.Marshal(netease.Stream{Size: 5, Type: "mp3", Level: "exhigh"})
	write(t, filepath.Join(dir, "123-exhigh.mp3"), []byte("music"))
	write(t, filepath.Join(dir, "123-exhigh.json"), meta)
}

func exists(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	if (err == nil) != want || (err != nil && !errors.Is(err, os.ErrNotExist)) {
		t.Fatalf("stat %s: %v, want exists=%v", path, err, want)
	}
}

// Complete pairs move only after settings are saved; unrelated and partial files stay untouched.
func TestMove(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "old"), filepath.Join(root, "new")
	song(t, source)
	for _, name := range []string{"personal.mp3", "unfinished.part", "notes.txt", "999-exhigh.json"} {
		write(t, filepath.Join(source, name), []byte("keep"))
	}
	modified := time.Unix(1700000000, 0)
	if err := os.Chtimes(filepath.Join(source, "123-exhigh.mp3"), modified, modified); err != nil {
		t.Fatal(err)
	}
	saved := false
	warning, err := Move(source, target, true, func() error {
		exists(t, filepath.Join(source, "123-exhigh.mp3"), true)
		data, err := os.ReadFile(filepath.Join(target, "123-exhigh.mp3"))
		if err != nil || string(data) != "music" {
			t.Fatalf("target before save: %q, %v", data, err)
		}
		saved = true
		return nil
	})
	if err != nil || warning != nil || !saved {
		t.Fatalf("move: %v, %v, saved=%v", err, warning, saved)
	}
	for _, name := range []string{"123-exhigh.mp3", "123-exhigh.json"} {
		exists(t, filepath.Join(source, name), false)
		exists(t, filepath.Join(target, name), true)
	}
	for _, name := range []string{"personal.mp3", "unfinished.part", "notes.txt", "999-exhigh.json"} {
		exists(t, filepath.Join(source, name), true)
		exists(t, filepath.Join(target, name), false)
	}
	info, err := os.Stat(filepath.Join(target, "123-exhigh.mp3"))
	if err != nil || !info.ModTime().Equal(modified) {
		t.Fatalf("lost LRU timestamp: %v, %v", info, err)
	}
}

// A partial copy or failed settings save must never remove the usable old cache.
func TestMoveFailure(t *testing.T) {
	for _, failure := range []string{"copy", "save"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			source, target := filepath.Join(root, "old"), filepath.Join(root, "new")
			song(t, source)
			injected := errors.New("injected failure")
			copies, saves := 0, 0
			copy := func(from, to string) error {
				copies++
				if failure == "copy" && copies == 2 {
					return injected
				}
				return copyFile(from, to)
			}
			_, err := move(source, target, true, func() error {
				saves++
				if failure == "save" {
					return injected
				}
				return nil
			}, copy)
			if !errors.Is(err, injected) {
				t.Fatalf("got %v", err)
			}
			if failure == "copy" && saves != 0 {
				t.Fatal("saved a failed migration")
			}
			exists(t, filepath.Join(source, "123-exhigh.mp3"), true)
			exists(t, filepath.Join(source, "123-exhigh.json"), true)
			exists(t, target, false)
			leftovers, _ := filepath.Glob(filepath.Join(root, ".song-cache-move-*"))
			if len(leftovers) != 0 {
				t.Fatalf("staging files leaked: %v", leftovers)
			}
		})
	}
}

// Directory changes without migration leave all old files where they were.
func TestMoveWithoutMigration(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "old"), filepath.Join(root, "new")
	song(t, source)
	warning, err := Move(source, target, false, func() error { return nil })
	if err != nil || warning != nil {
		t.Fatalf("move: %v %v", err, warning)
	}
	exists(t, filepath.Join(source, "123-exhigh.mp3"), true)
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatalf("target: %v %v", entries, err)
	}
}

// Overlapping paths, existing user files and invalid targets are rejected without saving.
func TestMoveRejectsUnsafeTargets(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "old")
	song(t, source)
	occupied := filepath.Join(root, "occupied")
	write(t, filepath.Join(occupied, "personal.txt"), []byte("keep"))
	blocked := filepath.Join(root, "file")
	write(t, blocked, []byte("not a directory"))
	for _, target := range []string{source, filepath.Join(source, "child"), root, occupied, blocked, filepath.Join(blocked, "child"), "relative"} {
		t.Run(target, func(t *testing.T) {
			if _, err := Move(source, target, true, func() error { t.Fatal("saved invalid path"); return nil }); err == nil {
				t.Fatal("accepted unsafe target")
			}
			exists(t, filepath.Join(source, "123-exhigh.mp3"), true)
			data, err := os.ReadFile(filepath.Join(occupied, "personal.txt"))
			if err != nil || string(data) != "keep" {
				t.Fatal("changed unrelated target file")
			}
		})
	}
}

// Incomplete and malformed pairs are not migrated or deleted.
func TestMoveSkipsIncompleteSongs(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "old"), filepath.Join(root, "new")
	song(t, source)
	write(t, filepath.Join(source, "123-exhigh.mp3"), []byte("shorter than metadata"))
	warning, err := Move(source, target, true, func() error { return nil })
	if err != nil || warning != nil {
		t.Fatalf("move: %v %v", err, warning)
	}
	exists(t, filepath.Join(source, "123-exhigh.mp3"), true)
	exists(t, filepath.Join(target, "123-exhigh.mp3"), false)
}

// A second disk uses the same copy-and-publish path, never a rename of the source.
func TestMoveAcrossVolumes(t *testing.T) {
	parent := os.Getenv("CACHE_MOVE_TEST_TARGET")
	if parent == "" {
		t.Skip("set CACHE_MOVE_TEST_TARGET to a temporary directory on a second disk")
	}
	source := t.TempDir()
	if filepath.VolumeName(source) == filepath.VolumeName(parent) {
		t.Skip("test needs distinct volumes")
	}
	target, err := os.MkdirTemp(parent, "cachemove-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(target) })
	song(t, source)
	warning, err := Move(source, target, true, func() error { return nil })
	if err != nil || warning != nil {
		t.Fatalf("cross-volume move: %v %v", err, warning)
	}
	exists(t, filepath.Join(target, "123-exhigh.mp3"), true)
	exists(t, filepath.Join(source, "123-exhigh.mp3"), false)
}
