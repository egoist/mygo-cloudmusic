package qqmusic

import (
	"context"

	appplatform "netease-music-mygo/internal/app/platform"
	backend "netease-music-mygo/internal/platform/qqmusic"
)

type RuntimeAdapter struct {
	state    *State
	settings *appplatform.PlatformSettings
	save     func()
}

func NewRuntime(state *State, settings *appplatform.PlatformSettings, save ...func()) appplatform.Runtime {
	var persist func()
	if len(save) > 0 {
		persist = save[0]
	}
	return &RuntimeAdapter{state: state, settings: settings, save: persist}
}

// QQ Music works anonymously and signs in with a QR code, so there is no
// connection form to configure.
func (r *RuntimeAdapter) Config() appplatform.ConnectionConfig   { return appplatform.ConnectionConfig{} }
func (r *RuntimeAdapter) SetConfig(appplatform.ConnectionConfig) {}
func (r *RuntimeAdapter) Configured() bool                       { return true }
func (r *RuntimeAdapter) Connected() bool                        { return true }
func (r *RuntimeAdapter) Error() error {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.Error()
}
func (r *RuntimeAdapter) Connect(context.Context, appplatform.ConnectionConfig) error { return nil }
func (r *RuntimeAdapter) Cancel(context.Context) error                                { return nil }
func (r *RuntimeAdapter) Disconnect(context.Context) error {
	backend.Logout()
	if r != nil && r.state != nil {
		r.state.SetProfile(nil)
		r.state.SetPlaylists(nil)
		r.state.SetLikedIDs(nil)
	}
	return nil
}
func (r *RuntimeAdapter) FetchArtwork(_ context.Context, raw string, size int) ([]byte, bool, error) {
	if raw == "" {
		return nil, true, nil
	}
	b, err := backend.Fetch(backend.Sized(raw, size))
	return b, true, err
}
func (r *RuntimeAdapter) Transcode() appplatform.TranscodeSettings {
	return appplatform.TranscodeSettings{Mode: appplatform.TranscodeAuto}
}
func (r *RuntimeAdapter) SetTranscode(appplatform.TranscodeSettings) {}
func (r *RuntimeAdapter) TranscodeBitrates() []int                   { return nil }
func (r *RuntimeAdapter) Sorts() map[string]string                   { return nil }
func (r *RuntimeAdapter) SetSort(string, string)                     {}
