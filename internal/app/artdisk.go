package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo"
)

// Covers keep on disk, as the CDN sent them, so that they show at once the
// next time, without the network: far from the CDN, each is most of a
// second away. A cover's URL names a picture that never changes, so one
// kept is good for good; past artDiskLimit, those shown longest ago go.

// artDiskLimit is how much the covers kept on disk may take: tens of
// thousands of them, a few KB to 50 each.
const artDiskLimit = 200 << 20

// artDir returns where covers are kept, or "" without a cache directory.
func artDir() string {
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "covers")
}

// artPath returns the file of the cover at url, or "" when none are kept.
func (c *artCache) artPath(url string) string {
	if c.dir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:16]))
}

// readArt returns the cover kept at path, shown now: the last to go.
func readArt(path string) ([]byte, bool) {
	if path == "" {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	now := time.Now()
	os.Chtimes(path, now, now)
	return b, true
}

// keepArt keeps the cover b at path, through a file of its own until it is
// all written: the same cover may come in twice at once, for a backdrop.
func keepArt(path string, b []byte) {
	if path == "" {
		return
	}
	dir := filepath.Dir(path)
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	f, err := os.CreateTemp(dir, "*.part")
	if err != nil {
		return
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil || os.Rename(f.Name(), path) != nil {
		os.Remove(f.Name())
	}
}

// pruneArt drops the covers in dir shown longest ago until the rest fit
// limit, and the parts of those an app that quit left.
func pruneArt(dir string, limit int64) {
	entries, _ := os.ReadDir(dir)
	type kept struct {
		path string
		size int64
		used time.Time
	}
	var covers []kept
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if filepath.Ext(path) == ".part" {
			if time.Since(info.ModTime()) > time.Hour {
				os.Remove(path)
			}
			continue
		}
		covers = append(covers, kept{path, info.Size(), info.ModTime()})
		total += info.Size()
	}
	slices.SortFunc(covers, func(a, b kept) int { return a.used.Compare(b.used) })
	for _, k := range covers {
		if total <= limit {
			return
		}
		if os.Remove(k.path) == nil {
			total -= k.size
		}
	}
}
