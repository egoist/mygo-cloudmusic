package app

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// With NM_FRAMES set, saves themed confirmation dialogs and long paths for visual inspection.
func TestCacheDirectoryConfirmationFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, scene := range []struct {
		name          string
		dark          bool
		width, height int
		target        string
		err           string
	}{
		{"light", false, 560, 720, `D:\Program Files\MyGo 云音乐`, ""},
		{"dark", true, 560, 720, `D:\Program Files\MyGo 云音乐`, ""},
		{"narrow", false, 480, 420, `D:\音乐与播客\缓存文件\网易云音乐\MyGO\这是一个较长的歌曲缓存目录`, ""},
		{"error", false, 480, 420, `D:\` + strings.Repeat(`很长的音乐缓存目录\`, 12), "新缓存目录必须为空，请选择其他目录。"},
	} {
		a, _, _ := cacheTestApp(t)
		a.cacheConfirm = cacheDirectoryConfirmation{open: true, directory: scene.target, target: scene.target, err: scene.err}
		tt := ui.NewTester(func(c *ui.Context) {
			theme := ui.LightTheme()
			if scene.dark {
				theme = ui.DarkTheme()
			}
			c.SetTheme(theme)
			a.settingsView(c)
		}, scene.width, scene.height)
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, "cache-confirm-"+scene.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}
