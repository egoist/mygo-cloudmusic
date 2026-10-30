package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/speech"
)

// captionsPane shows the episode playing's captions as lyrics show, in
// Now Playing: written down on the device as it plays. Without the model,
// it offers to download it.
func (a *app) captionsPane(c *ui.Context, tr *netease.Track, h float32) {
	pl := a.pl
	cs := &a.captions
	if !a.modelReady() {
		a.modelCard(c)
		return
	}
	if !a.settings.Captions {
		a.captionsNote(c, "字幕已关闭", "开启字幕", func() {
			a.settings.Captions = true
			a.settings.save()
			a.syncCaptions()
		})
		return
	}
	// Until the player says the episode changed, what the app has is the
	// last one's: none of it shows.
	var lines []lyrics.Line
	if cs.program == tr.ProgramID {
		lines = cs.lines
	}
	pos := pl.Position()
	if len(lines) == 0 {
		switch {
		case cs.program != tr.ProgramID:
			a.captionsNote(c, "正在识别…", "", nil)
		case cs.failed == cs.program:
			a.captionsNote(c, "字幕生成失败", "", nil)
		case a.captionsWritten():
			a.captionsNote(c, "这期节目没有识别到语音", "", nil)
		default:
			a.captionsNote(c, "正在识别…", "", nil)
		}
		return
	}
	cur := lyrics.Index(lines, pos)
	// A line stays lit through a pause, but not where nothing is written
	// down yet: after a seek ahead, until the words there are.
	if cur >= 0 && pos >= lines[cur].End && a.captionsWriting(pos) {
		cur = -1
	}
	if pl.Playing() {
		if i := lyrics.Index(lines, pos); i+1 < len(lines) {
			c.After(lines[i+1].At - pos + 20*time.Millisecond)
		}
	}
	if a.lyricTrack != tr.ID {
		a.lyricTrack, a.lyricAt = tr.ID, -2
		a.lyricScroll, a.lyricGlide = ui.ScrollState{}, lyricGlide{}
		a.lyricRecenter = 0
	}
	lag := a.lyricGlide.step(c, &a.lyricScroll)
	scroll := ui.Scroll(c.Key("captions")).Grow(1).TrackScroll(&a.lyricScroll).Padding(0, 8, h*0.5, 0)
	a.timedLines(c, scroll, lines, pos, cur, lag, linesStyle{size: 24, weight: 700, lineHeight: 1.4}, func() {
		if cs.work != nil {
			ui.Text(c, "正在识别…").FontSize(14).FontWeight(600).TextColor(white.Alpha(0.35)).Padding(16, 12)
		}
	})
}

// captionsNote says how the captions are, in the middle of the pane, with
// a button if there is something to do.
func (a *app) captionsNote(c *ui.Context, text, button string, act func()) {
	ui.Column(c).Grow(1).Center().Gap(18).Children(func() {
		ui.Text(c, text).FontSize(22).FontWeight(700).TextColor(white.Alpha(0.4))
		if button != "" && pillButton(c.Key("act"), nil, button, white.Alpha(0.14), white).Clicked() {
			act()
		}
	})
}

// modelCard offers to download the model that writes captions, and shows
// how far its download is.
func (a *app) modelCard(c *ui.Context) {
	m := &a.captions.model
	ui.Column(c).Grow(1).Justify(ui.Center).Gap(14).Padding(0, 12, 48, 12).MaxWidth(440).Children(func() {
		ui.Text(c, "为播客生成字幕").FontSize(24).FontWeight(800).LetterSpacing(-0.3)
		ui.Text(c, fmt.Sprintf("下载语音识别模型（约 %s）后，节目会在本机边播边转写成字幕，像歌词一样随播放滚动，点一句即可跳到那里。音频不会上传。", megabytes(speech.ModelSize()))).
			FontSize(14).LineHeight(1.6).TextColor(white.Alpha(0.6))
		if m.downloading {
			share := float32(0)
			if m.total > 0 {
				share = float32(m.done) / float32(m.total)
			}
			ui.Box(c).Height(6).Radius(3).Background(white.Alpha(0.14)).Margin(6, 0, 0, 0).Children(func() {
				ui.Box(c).Height(6).Radius(3).WidthPercent(share * 100).Background(white.Alpha(0.85))
			})
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, fmt.Sprintf("正在下载 %s / %s", megabytes(m.done), megabytes(m.total))).FontSize(12.5).FontFeatures("tnum").TextColor(white.Alpha(0.5))
				ui.Spacer(c)
				if pillButton(c.Key("cancel"), nil, "取消", white.Alpha(0.14), white).Clicked() {
					a.cancelModelDownload()
				}
			})
			return
		}
		if m.err != "" {
			ui.Text(c, m.err).FontSize(13).TextColor(ui.Hex("#ff8a80"))
		}
		label := "下载模型"
		if m.err != "" {
			label = "重试"
		}
		ui.Row(c).Margin(6, 0, 0, 0).Children(func() {
			if pillButton(c.Key("download"), nil, label, white, ui.Hex("#141416")).Clicked() {
				a.startModelDownload()
			}
		})
	})
}

// megabytes says n bytes in megabytes, or gigabytes once there are many.
func megabytes(n int64) string {
	if n >= 1<<30 {
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%d MB", n>>20)
}

// captionSettings are the settings' rows for captions.
func (a *app) captionSettings(row func(label, desc string, control func()), c *ui.Context) {
	m := &a.captions.model
	s := &a.settings
	row("播客字幕", "播放节目时在本机转写语音，生成可滚动的字幕；转写时会多用一些电量", func() {
		if ui.Switch(c.Key("captions"), &s.Captions).Label("播客字幕").Disabled(!a.modelReady()).Changed() {
			a.changed()
			a.syncCaptions()
		}
	})
	var desc string
	switch {
	case m.downloading:
		desc = fmt.Sprintf("正在下载 %s / %s", megabytes(m.done), megabytes(m.total))
	case a.modelReady():
		desc = "Qwen3-ASR，已下载，占用 " + megabytes(speech.ModelSize())
	default:
		desc = "Qwen3-ASR，约 " + megabytes(speech.ModelSize()) + "，下载后才能生成字幕"
	}
	row("语音识别模型", desc, func() {
		switch {
		case m.downloading:
			if ui.Button(c.Key("model"), "取消").Clicked() {
				a.cancelModelDownload()
			}
		case a.modelReady():
			if ui.Button(c.Key("model"), "删除").Clicked() {
				a.deleteModel()
			}
		default:
			if ui.Button(c.Key("model"), "下载").Clicked() {
				a.startModelDownload()
			}
		}
	})
}
