package app

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo"
	"netease-music-mygo/internal/cachemove"
)

var openCacheDirectory = mygo.Dialog.Open
var askCacheDirectory = mygo.Dialog.Message

type cacheDirectoryChange struct {
	Directory string `json:"directory"`
	Migrate   bool   `json:"migrate"`
}

// applyCacheDirectoryChange runs before the player can read or write a cache.
func (a *app) applyCacheDirectoryChange() {
	change := a.settings.CacheChange
	if change == nil {
		return
	}
	source, err := songCacheDirectory(a.settings.CacheDir)
	if err != nil {
		a.cacheError = err.Error()
		return
	}
	target, err := songCacheDirectory(change.Directory)
	if err != nil {
		a.cacheError = err.Error()
		return
	}
	next := a.settings
	next.CacheDir, next.CacheChange = change.Directory, nil
	warning, err := cachemove.Move(source, target, change.Migrate, next.save)
	if err != nil {
		a.cacheError = "更改缓存目录失败，仍使用原目录：" + err.Error()
		return
	}
	a.settings = next
	if warning != nil {
		a.cacheError = "新目录已启用，但部分旧缓存未能删除：" + warning.Error()
	}
}

// queueCacheDirectory saves only a request: no active cache files are moved.
func (a *app) queueCacheDirectory(directory string, migrate bool) error {
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
	next := a.settings
	next.CacheChange = &cacheDirectoryChange{Directory: directory, Migrate: migrate}
	if err := next.save(); err != nil {
		return err
	}
	a.settings = next
	a.cacheError = ""
	return nil
}

func (a *app) cancelCacheDirectoryChange() error {
	next := a.settings
	next.CacheChange = nil
	if err := next.save(); err != nil {
		return err
	}
	a.settings = next
	a.cacheError = ""
	return nil
}

func (a *app) chooseCacheDirectory() {
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
	target, err := songCacheDirectory(directory)
	if err != nil {
		a.cacheError = err.Error()
		return
	}
	result, err := askCacheDirectory(mygo.MessageOptions{
		Parent: a.settingsWin, Type: mygo.MessageQuestion, Title: "更改歌曲缓存目录",
		Message: "下次启动时使用新目录，是否迁移已有歌曲缓存？",
		Detail:  fmt.Sprintf("新目录：%s\n迁移成功并保存设置后才清理旧歌曲缓存。不迁移则保留旧文件。请选择空目录；迁移期间请勿打开另一个应用实例。", target),
		Buttons: []string{"取消", "迁移并更改", "仅更改目录"}, DefaultButton: 1, CancelButton: 0,
	})
	if err != nil {
		a.cacheError = err.Error()
		return
	}
	if result.Button != 1 && result.Button != 2 {
		return
	}
	if err := a.queueCacheDirectory(directory, result.Button == 1); err != nil {
		a.cacheError = err.Error()
	}
}

func (a *app) cacheDirectoryDescription() string {
	dir, err := songCacheDirectory(a.settings.CacheDir)
	if err != nil {
		return err.Error()
	}
	desc := dir
	if change := a.settings.CacheChange; change != nil {
		target, err := songCacheDirectory(change.Directory)
		if err == nil {
			desc += "\n下次启动时更改为：" + target
		}
	}
	if a.cacheError != "" {
		desc += "\n" + a.cacheError
	}
	return desc
}
