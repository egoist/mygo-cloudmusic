package fnmusic

import (
	"context"
	"time"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
	backend "netease-music-mygo/internal/platform/fnmusic"
	"netease-music-mygo/internal/player"
)

// Adapter owns one complete Feiniu context. The root app stores only this
// interface, so provider state and UI do not leak into shared app files.
type Adapter struct {
	state   *State
	runtime appplatform.Runtime
	home    *Home
}

var _ appplatform.Adapter = (*Adapter)(nil)
var _ appplatform.Runtime = (*RuntimeAdapter)(nil)

func NewAdapter(host Host, settings *appplatform.PlatformSettings, save ...func()) *Adapter {
	state := NewState()
	if settings != nil {
		cfg := fromConnectionConfig(settings.Config)
		LoadCredentials(&cfg)
		state.SetConfig(cfg)
		if cfg.Valid() {
			// Mark the restored session as loading before the first window
			// frame. The root starts the actual connection just after the
			// window is created, so this prevents a configured account from
			// flashing the connection form once at startup.
			state.setConnecting(state.Generation(), true)
		}
	}
	if host != nil {
		state.SetPost(host.Post)
	}
	var persist func()
	if len(save) > 0 {
		persist = save[0]
	}
	runtime := NewRuntime(state, settings, persist)
	return &Adapter{state: state, runtime: runtime, home: NewHome(state, host, runtime)}
}

func (a *Adapter) Home() appplatform.Home {
	if a == nil {
		return nil
	}
	return a.home
}

func (a *Adapter) Info() appplatform.Info {
	return appplatform.Info{ID: platform.FnmusicID, Name: "飞牛音乐", Comments: false, Podcasts: false, Quality: false, Normalize: false}
}

func (a *Adapter) Runtime() appplatform.Runtime {
	if a == nil {
		return nil
	}
	return a.runtime
}

func (a *Adapter) OwnsTrack(tr *netease.Track) bool {
	return tr != nil && tr.Source == backend.Source
}

func (a *Adapter) Liked(tr *netease.Track) bool {
	return a != nil && a.state != nil && a.state.LikedTrack(tr)
}

func (a *Adapter) Favorite(ctx context.Context, tr *netease.Track, liked bool) error {
	if a == nil || a.state == nil {
		return backend.ErrSignedOut
	}
	return a.state.SetFavorite(ctx, tr, liked)
}

// ConfigurePlayer installs Feiniu resolvers on the shared player. Keeping the
// resolver here means the root app does not import Feiniu protocol details;
// NetEase tracks continue through the player's original path.
func ConfigurePlayer(p *player.Player, s *State, mode func() backend.TranscodeMode, bitrate func() int) {
	if p == nil {
		return
	}
	p.ResolveAudio = func(tr *netease.Track, level string) (*netease.Stream, player.AudioStream, error) {
		if tr == nil || tr.Source != backend.Source || s == nil {
			return nil, nil, nil
		}
		client := s.Client()
		if client == nil {
			return nil, nil, nil
		}
		m := backend.TranscodeAuto
		if mode != nil && mode() != "" {
			m = mode()
		}
		b := 0
		if bitrate != nil {
			b = bitrate()
		}
		return openFeiniuAudio(client, tr, m, b)
	}
	p.ResolveStream = func(tr *netease.Track, level string) (*netease.Stream, error) {
		if tr == nil {
			return nil, netease.ErrUnavailable
		}
		if tr.Source != backend.Source || s == nil {
			return netease.SongURL(tr.ID, level)
		}
		client := s.Client()
		if client == nil {
			return nil, backend.ErrSignedOut
		}
		return client.Stream(tr)
	}
	p.Artwork = func(tr *netease.Track) ([]byte, error) {
		if tr == nil {
			return nil, netease.ErrUnavailable
		}
		if tr.Source != backend.Source || s == nil {
			return netease.Fetch(netease.Sized(tr.Album.PicURL, 600))
		}
		provider := s.Provider()
		if provider == nil {
			return nil, backend.ErrSignedOut
		}
		return provider.Cover(context.Background(), tr.Album.PicURL, 600)
	}
	p.ResolveLyrics = func(tr *netease.Track) (string, string, string, error) {
		if tr == nil {
			return "", "", "", netease.ErrUnavailable
		}
		if tr.Source != backend.Source || s == nil {
			return netease.Lyrics(tr.ID)
		}
		provider := s.Provider()
		if provider == nil {
			return "", "", "", backend.ErrSignedOut
		}
		l, err := provider.Lyrics(context.Background(), tr)
		return l.LRC, l.Translation, l.YRC, err
	}
}

// openFeiniuAudio keeps the provider's transcode policy beside its client.
// The shared player receives only the segmented source interface and owns all
// decoding and seeking behavior.
func openFeiniuAudio(client *backend.Client, tr *netease.Track, mode backend.TranscodeMode, bitrate int) (*netease.Stream, player.AudioStream, error) {
	if backend.ShouldTranscode(tr, mode) {
		h, err := client.TranscodeHLSWithBitrate(context.Background(), tr, bitrate)
		if err != nil {
			return nil, nil, err
		}
		s, err := player.OpenSegmentedStream(hlsSource{h})
		if err != nil {
			return nil, nil, err
		}
		return &netease.Stream{Type: "mp3", Level: "exhigh"}, s, nil
	}
	if !backend.Native(tr.Format) || tr.Cue {
		return nil, nil, backend.NativePlaybackError(tr)
	}
	return nil, nil, nil
}

// hlsSource is the small provider-to-player translation for Feiniu HLS.
type hlsSource struct{ h *backend.HLS }

func (s hlsSource) SegmentCount() int                   { return len(s.h.Segments) }
func (s hlsSource) SegmentDuration(i int) time.Duration { return s.h.Segments[i].Duration }
func (s hlsSource) TotalDuration() time.Duration        { return s.h.Total }
func (s hlsSource) FetchSegment(ctx context.Context, i int) ([]byte, error) {
	return s.h.Fetch(ctx, i)
}
func (s hlsSource) Close() { s.h.Release() }

// Configure installs this adapter's playback hooks without exposing State to
// the root application.
func (a *Adapter) ConfigurePlayer(p *player.Player) {
	if a == nil || a.state == nil {
		return
	}
	ConfigurePlayer(p, a.state, func() backend.TranscodeMode {
		v := a.runtime.Transcode().Mode
		switch v {
		case appplatform.TranscodeNative:
			return backend.TranscodeNative
		case appplatform.TranscodeAlways:
			return backend.TranscodeAlways
		default:
			return backend.TranscodeAuto
		}
	}, func() int { return a.runtime.Transcode().Bitrate })
}
