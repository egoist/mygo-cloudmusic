package qqmusic

import (
	"context"
	"strconv"

	"github.com/egoist/mygo/ui"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
	backend "netease-music-mygo/internal/platform/qqmusic"
	"netease-music-mygo/internal/player"
)

const Source = "qqmusic"

type Adapter struct {
	state   *State
	runtime appplatform.Runtime
	home    *Home
}

var _ appplatform.Adapter = (*Adapter)(nil)

func NewAdapter(host Host, settings *appplatform.PlatformSettings, save ...func()) *Adapter {
	state := NewState()
	runtime := NewRuntime(state, settings, save...)
	return &Adapter{state: state, runtime: runtime, home: NewHome(state, host, runtime)}
}

func (a *Adapter) Home() appplatform.Home {
	if a == nil {
		return nil
	}
	return a.home
}

func (a *Adapter) Info() appplatform.Info {
	// QQ plays up to FLAC and ships loudness data with its streams, so the
	// shared quality and normalisation rows apply.
	return appplatform.Info{ID: platform.QQMusicID, Name: "QQ 音乐", Comments: true, Quality: true, Normalize: true}
}

// AccentColor is QQ Music's green, which its pages and controls share.
func (a *Adapter) AccentColor() (ui.Color, ui.Color) {
	return ui.Hex("#30a46c"), ui.Hex("#ffffff")
}

func (a *Adapter) Runtime() appplatform.Runtime {
	if a == nil {
		return nil
	}
	return a.runtime
}

func (a *Adapter) OwnsTrack(tr *netease.Track) bool {
	return tr != nil && tr.Source == Source
}

func (a *Adapter) Liked(tr *netease.Track) bool {
	return a != nil && tr != nil && a.state.Liked(tr.ID)
}

func (a *Adapter) Favorite(ctx context.Context, tr *netease.Track, liked bool) error {
	if a == nil {
		return backend.ErrUnavailable
	}
	return a.state.SetFavorite(ctx, tr, liked)
}

// CommentThread names a QQ track's comment thread: the song id itself.
func (a *Adapter) CommentThread(tr *netease.Track) string {
	if tr == nil {
		return ""
	}
	return strconv.FormatInt(tr.ID, 10)
}

// Comments reads a page of a QQ track's comments. QQ does not support liking
// them through this client, so the shared pane shows counts only.
func (a *Adapter) Comments(thread string, offset, limit int) (*netease.Comments, error) {
	page, err := backend.ThreadComments(thread, offset, limit)
	if err != nil {
		return nil, err
	}
	return qqComments(page), nil
}

func (a *Adapter) ConfigurePlayer(p *player.Player) {
	if p == nil || a == nil || a.state == nil {
		return
	}
	p.ResolveStream = func(tr *netease.Track, level string) (*netease.Stream, error) {
		if tr == nil {
			return nil, netease.ErrUnavailable
		}
		if tr.Source != Source {
			return netease.SongURL(tr.ID, level)
		}
		s, err := backend.SongURL(tr.ID, level)
		if err != nil {
			return nil, err
		}
		return &netease.Stream{URL: s.URL, Size: s.Size, Type: s.Type, Level: s.Level, Gain: s.Gain, Peak: s.Peak}, nil
	}
	p.Artwork = func(tr *netease.Track) ([]byte, error) {
		if tr == nil {
			return nil, netease.ErrUnavailable
		}
		if tr.Source != Source {
			return netease.Fetch(netease.Sized(tr.Album.PicURL, 600))
		}
		return backend.Fetch(backend.Sized(tr.Album.PicURL, 600))
	}
	p.ResolveLyrics = func(tr *netease.Track) (string, string, string, error) {
		if tr == nil {
			return "", "", "", netease.ErrUnavailable
		}
		if tr.Source != Source {
			return netease.Lyrics(tr.ID)
		}
		return backend.Lyrics(tr.ID)
	}
	p.Started = func(tr *netease.Track) {
		if a.home != nil {
			a.home.Started(tr)
		}
	}
}
