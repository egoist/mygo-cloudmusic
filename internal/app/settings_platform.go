package app

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/platform"
)

// Platform settings extension.
//
// Keep the active provider and provider-owned settings namespace together so
// the upstream settings structure does not accumulate provider fields. The
// provider-specific values remain neutral seam types; validation and
// interpretation belong to the provider adapter.
type platformSettings struct {
	Platform string                       `json:"platform,omitempty"`
	FnMusic  appplatform.PlatformSettings `json:"fnmusic,omitempty"`
	QQMusic  appplatform.PlatformSettings `json:"qqmusic,omitempty"`
}

// qqQuality keeps only the levels QQ can actually stream.
func qqQuality(level string) string {
	switch level {
	case "standard", "exhigh", "lossless":
		return level
	case "higher":
		return "exhigh"
	case "hires":
		return "lossless"
	default:
		return "exhigh"
	}
}

func (a *app) qualityLevel() string {
	if a.platformID == platform.QQMusicID {
		if q := a.settings.QQMusic.Quality; q != "" {
			return q
		}
		return "exhigh"
	}
	return a.settings.Quality
}

func (a *app) setQualityLevel(level string) {
	if a.platformID == platform.QQMusicID {
		a.settings.QQMusic.Quality = qqQuality(level)
		return
	}
	a.settings.Quality = level
}

func (a *app) normalizeEnabled() bool {
	if a.platformID == platform.QQMusicID {
		if v := a.settings.QQMusic.Normalize; v != nil {
			return *v
		}
		return true
	}
	return a.settings.Normalize
}

func (a *app) setNormalize(on bool) {
	if a.platformID == platform.QQMusicID {
		a.settings.QQMusic.Normalize = &on
		return
	}
	a.settings.Normalize = on
}

func (a *app) hotCommentsEnabled() bool {
	if a.platformID == platform.QQMusicID {
		return a.settings.QQMusic.HotComments
	}
	return a.settings.HotComments
}

func (a *app) setHotComments(on bool) {
	if a.platformID == platform.QQMusicID {
		a.settings.QQMusic.HotComments = on
		return
	}
	a.settings.HotComments = on
}

func (a *app) platformQualityLevels() []struct{ level, label string } {
	if a.platformID == platform.QQMusicID {
		return []struct{ level, label string }{
			{"standard", "标准（128k）"},
			{"exhigh", "极高（320k）"},
			{"lossless", "无损（FLAC）"},
		}
	}
	return qualityLevels
}

func (a *app) platformSettingsGroup(c *ui.Context, group func(string, func()), row func(string, string, func())) {
	group("平台", func() {
		row("音乐平台", "切换后主页和平台设置使用独立的上下文", func() {
			infos := a.platformInfos()
			labels := make([]string, len(infos))
			for i, info := range infos {
				labels[i] = info.Name
			}
			chosen := labels[0]
			for i, info := range infos {
				if info.ID == a.platformID {
					chosen = labels[i]
				}
			}
			if ui.Select(c.Key("platform"), &chosen, labels).Label("音乐平台").Width(150).Changed() {
				for _, info := range infos {
					if info.Name == chosen {
						a.setPlatform(info.ID)
						break
					}
				}
			}
		})
	})
}

// setPlatform switches the complete platform context. Shared playback and
// window surfaces stay alive, so features such as the macOS notch, lyrics and
// desktop lyrics keep the upstream implementation unchanged.
func (a *app) setPlatform(id platform.ID) {
	if !a.hasPlatform(id) {
		id = platform.NeteaseID
	}
	if a.platformID == id {
		return
	}
	if a.pl != nil {
		a.pl.Reset()
	}
	a.discardSession()
	if old := a.activePlatform(); old != nil {
		if home := old.Home(); home != nil {
			home.CloseTransient()
		}
		if rt := old.Runtime(); rt != nil {
			_ = rt.Cancel(context.Background())
		}
	}
	a.platformID = id
	a.platformGeneration++
	a.configurePlatformPlayer()
	a.settings.Platform = string(id)
	a.history, a.forward = nil, nil
	a.nowPlaying = false
	a.trackList, a.platformCardList = ui.ListState{}, ui.ListState{}
	a.pageScroll, a.navScroll = ui.ScrollState{}, ui.ScrollState{}
	a.platformHeroSince = time.Time{}
	a.comments = commentState{}
	a.platformMenuOpen = false
	a.platformRoute = appplatform.Route{}
	a.scrolled = route{}
	if home := a.activeHome(); home != nil {
		home.ResetNavigation(appplatform.Route{Page: "library"})
		if rt := a.activeRuntime(); rt != nil && rt.Configured() && !rt.Connected() {
			go func() { _ = rt.Connect(context.Background(), rt.Config()) }()
		}
	} else {
		a.route = route{page: pageHome}
		a.boot()
		a.load(a.route)
	}
	a.settings.save()
	a.redraw()
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
}
