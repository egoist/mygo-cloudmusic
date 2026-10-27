package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/player"
)

// newSongCache returns the cache of songs played, in the app's cache
// directory, or nil without one.
func newSongCache(mb int) *player.Cache {
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return nil
	}
	return player.NewCache(filepath.Join(dir, "songs"), int64(mb)<<20)
}

// applyCacheLimit gives the cache the limit the settings say. With none,
// the covers kept on disk go too, and no more are kept.
func (a *app) applyCacheLimit() {
	songs, art := a.pl.Cache, a.art
	limit := int64(a.settings.CacheMB) << 20
	art.off = limit == 0
	go func() {
		if songs != nil {
			songs.SetLimit(limit)
		}
		if limit == 0 {
			art.clear()
		}
		a.post(a.measureCache)
	}()
}

// measureCache finds how much the songs kept take, and every cache with
// them, on a goroutine, for the settings to show.
func (a *app) measureCache() {
	songs, dirs := a.pl.Cache, a.cacheDirs()
	go func() {
		var kept int64
		if songs != nil {
			kept = songs.Size()
		}
		all := kept
		for _, dir := range dirs {
			all += dirSize(dir)
		}
		a.post(func() {
			a.cacheUsed, a.cacheAll = kept, all
			if a.settingsWin != nil {
				a.settingsWin.Invalidate()
			}
		})
	}()
}

// cacheDirs returns the caches kept beside the songs: the covers, and the
// transcripts of episodes.
func (a *app) cacheDirs() []string {
	var dirs []string
	if a.art.dir != "" {
		dirs = append(dirs, a.art.dir)
	}
	if dir, err := transcriptDir(); err == nil {
		dirs = append(dirs, dir)
	}
	return dirs
}

// dirSize returns how many bytes the files in dir take.
func dirSize(dir string) int64 {
	var n int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

// clearCache drops every cache: the songs kept, the covers and the
// transcripts. The episode playing, as the song, keeps its transcript
// again as it goes on.
func (a *app) clearCache() {
	a.cacheUsed, a.cacheAll = 0, 0
	songs, art := a.pl.Cache, a.art
	transcripts, err := transcriptDir()
	go func() {
		if songs != nil {
			songs.Clear()
		}
		art.clear()
		if err == nil {
			os.RemoveAll(transcripts)
		}
		a.post(a.measureCache)
	}()
}

// cacheUsage says how much a cache of n bytes takes, once measured.
func cacheUsage(n int64) string {
	switch {
	case n < 0:
		return "正在计算已用空间…"
	case n >= 1<<30:
		return fmt.Sprintf("已用 %.1f GB", float64(n)/(1<<30))
	default:
		return fmt.Sprintf("已用 %d MB", n>>20)
	}
}
