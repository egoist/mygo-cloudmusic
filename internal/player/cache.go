package player

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"netease-music-mygo/internal/netease"
)

// Cache keeps the songs played on disk, so that they play again at once,
// and without the network: each as <id>-<level>.mp3 or .flac, with what
// its stream was beside it in <id>-<level>.json. Once they take more than
// the limit, those played longest ago go first.
type Cache struct {
	dir string

	mu    sync.Mutex
	limit int64 // in bytes; 0 keeps nothing
}

// NewCache returns a cache in dir of limit bytes.
func NewCache(dir string, limit int64) *Cache {
	return &Cache{dir: dir, limit: limit}
}

// SetLimit changes how much the cache may take, and drops what is over.
func (c *Cache) SetLimit(limit int64) {
	c.mu.Lock()
	c.limit = limit
	c.mu.Unlock()
	c.prune("")
}

func cacheKey(id int64, level string) string { return fmt.Sprintf("%d-%s", id, level) }

// lookup returns a song kept at level or a better one, with its stream.
func (c *Cache) lookup(id int64, level string) ([]byte, *netease.Stream, bool) {
	if c == nil {
		return nil, nil, false
	}
	at := max(slices.Index(netease.Levels, level), 0)
	for _, lv := range netease.Levels[at:] {
		key := cacheKey(id, lv)
		meta, err := os.ReadFile(filepath.Join(c.dir, key+".json"))
		if err != nil {
			continue
		}
		var st netease.Stream
		if json.Unmarshal(meta, &st) != nil {
			continue
		}
		path := filepath.Join(c.dir, key+"."+st.Type)
		b, err := os.ReadFile(path)
		if err != nil || int64(len(b)) != st.Size {
			continue
		}
		// Played now: the last to go.
		now := time.Now()
		os.Chtimes(path, now, now)
		return b, &st, true
	}
	return nil, nil, false
}

// store keeps a song, whole, of the stream st, and drops what is over the
// limit then.
func (c *Cache) store(id int64, st *netease.Stream, b []byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	limit := c.limit
	c.mu.Unlock()
	// A song of more than a quarter of it would push out too many.
	if int64(len(b)) > limit/4 {
		return
	}
	kept := *st
	if kept.Type == "" {
		kept.Type = "mp3"
	}
	kept.Size = int64(len(b))
	key := cacheKey(id, kept.Level)
	meta, _ := json.Marshal(kept)
	if os.MkdirAll(c.dir, 0o700) != nil {
		return
	}
	// Written aside, then moved in place: a song half written is no song.
	path := filepath.Join(c.dir, key+"."+kept.Type)
	if os.WriteFile(path+".part", b, 0o600) != nil || os.Rename(path+".part", path) != nil {
		os.Remove(path + ".part")
		return
	}
	os.WriteFile(filepath.Join(c.dir, key+".json"), meta, 0o600)
	c.prune(path)
}

// Size returns how many bytes the songs kept take.
func (c *Cache) Size() int64 {
	var n int64
	for _, f := range c.songs() {
		n += f.size
	}
	return n
}

// Clear drops every song kept.
func (c *Cache) Clear() {
	for _, f := range c.songs() {
		c.remove(f.path)
	}
}

type cachedSong struct {
	path string
	size int64
	used time.Time
}

func (c *Cache) songs() []cachedSong {
	entries, _ := os.ReadDir(c.dir)
	var out []cachedSong
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if ext != ".mp3" && ext != ".flac" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, cachedSong{filepath.Join(c.dir, e.Name()), info.Size(), info.ModTime()})
	}
	return out
}

func (c *Cache) remove(path string) {
	os.Remove(path)
	os.Remove(strings.TrimSuffix(path, filepath.Ext(path)) + ".json")
}

// prune drops the songs played longest ago until the rest fit the limit,
// but keep, the song just kept.
func (c *Cache) prune(keep string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	songs := c.songs()
	slices.SortFunc(songs, func(a, b cachedSong) int { return a.used.Compare(b.used) })
	var total int64
	for _, f := range songs {
		total += f.size
	}
	for _, f := range songs {
		if total <= c.limit {
			return
		}
		if f.path != keep {
			c.remove(f.path)
			total -= f.size
		}
	}
}
