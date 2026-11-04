package app

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo"
	"netease-music-mygo/internal/cachemove"
)

var openCacheDirectory = mygo.Dialog.Open
var postCacheDirectory = func(a *app, fn func()) { a.post(fn) }

// changeCacheDirectory suspends playback now and moves files off the UI thread.
// Settings are saved on the main thread, preserving edits made during copying.
func (a *app) changeCacheDirectory(directory string, migrate bool) error {
	if a.cacheMoving {
		return fmt.Errorf("正在更改缓存目录，请稍候")
	}
	source, err := songCacheDirectory(a.settings.CacheDir)
	if err != nil {
		return err
	}
	target, err := songCacheDirectory(directory)
	if err != nil {
		return err
	}
	if err := cachemove.Validate(source, target); err != nil {
		return err
	}
	a.cacheMoving, a.cacheTarget, a.cacheError = true, target, ""
	cache := a.pl.Cache
	resume := a.pl.SuspendCache()
	go func() {
		unblock := cache.Quiesce()
		warning, err := cachemove.Move(source, target, migrate, func() error {
			saved := make(chan error, 1)
			postCacheDirectory(a, func() {
				next := a.settings
				next.CacheDir = directory
				err := next.save()
				if err == nil {
					a.settings = next
				}
				saved <- err
			})
			return <-saved
		})
		if err != nil {
			unblock()
		}
		postCacheDirectory(a, func() {
			a.cacheMoving, a.cacheTarget = false, ""
			if err != nil {
				a.cacheError = "更改缓存目录失败，仍使用原目录：" + err.Error()
			} else {
				a.pl.Cache = newSongCache(a.settings.CacheMB, a.settings.CacheDir)
				if warning != nil {
					a.cacheError = "新目录已启用，但部分旧缓存未能删除：" + warning.Error()
				}
			}
			resume()
			a.cacheUsed = -1
			a.measureCache()
			if a.settingsWin != nil {
				a.settingsWin.Invalidate()
			}
		})
	}()
	return nil
}

func (a *app) chooseCacheDirectory() {
	if a.cacheMoving {
		return
	}
	paths, err := openCacheDirectory(mygo.OpenDialogOptions{
		Parent: a.settingsWin, Title: "选择空文件夹作为歌曲缓存目录", Directory: true, CreateDirectories: true,
	})
	if err != nil {
		a.cacheError = err.Error()
		return
	}
	if len(paths) == 0 {
		return
	}
	a.confirmCacheDirectory(filepath.Clean(paths[0]))
}

func (a *app) confirmCacheDirectory(directory string) {
	if a.cacheMoving {
		return
	}
	target, err := songCacheDirectory(directory)
	if err != nil {
		a.cacheError = err.Error()
		return
	}
	a.cacheConfirm = cacheDirectoryConfirmation{open: true, directory: directory, target: target}
}

func (a *app) submitCacheDirectory(migrate bool) {
	d := &a.cacheConfirm
	if err := a.changeCacheDirectory(d.directory, migrate); err != nil {
		d.err = err.Error()
		return
	}
	d.open = false
}

func (a *app) cacheDirectoryDescription() string {
	dir, err := songCacheDirectory(a.settings.CacheDir)
	if err != nil {
		return err.Error()
	}
	desc := dir
	if a.cacheMoving {
		desc += "\n正在更改为：" + a.cacheTarget + "\n完成后自动恢复播放，请勿退出应用"
	}
	if a.cacheError != "" {
		desc += "\n" + a.cacheError
	}
	return desc
}
