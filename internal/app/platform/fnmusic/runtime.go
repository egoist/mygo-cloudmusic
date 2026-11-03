package fnmusic

import (
	"context"

	appplatform "netease-music-mygo/internal/app/platform"
	backend "netease-music-mygo/internal/platform/fnmusic"
)

// RuntimeAdapter translates the generic application seam into Feiniu's
// protocol configuration and lifecycle. No Feiniu client type escapes this
// package.
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

func (r *RuntimeAdapter) Config() appplatform.ConnectionConfig {
	if r == nil || r.state == nil {
		return appplatform.ConnectionConfig{}
	}
	return toConnectionConfig(r.state.Config())
}

func (r *RuntimeAdapter) SetConfig(cfg appplatform.ConnectionConfig) {
	if r == nil || r.state == nil {
		return
	}
	r.state.SetConfig(fromConnectionConfig(cfg))
}

func (r *RuntimeAdapter) Configured() bool {
	if r == nil {
		return false
	}
	return fromConnectionConfig(r.Config()).Valid()
}

func (r *RuntimeAdapter) Connected() bool { return r != nil && r.state != nil && r.state.Connected() }

func (r *RuntimeAdapter) Error() error {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.Error()
}

func (r *RuntimeAdapter) Connect(ctx context.Context, cfg appplatform.ConnectionConfig) error {
	if r == nil || r.state == nil {
		return backend.ErrSignedOut
	}
	b := fromConnectionConfig(cfg)
	defer r.state.finishConnecting()
	if err := r.state.Connect(ctx, b); err != nil {
		return err
	}
	// State.Connect publishes the client and completes its login phase. Keep
	// the loading surface until the sidebar catalog has also landed.
	r.state.setConnecting(r.state.Generation(), true)
	canonical := toConnectionConfig(r.state.Config())
	if r.settings != nil {
		r.settings.Config = canonical
	}
	if r.save != nil {
		r.save()
	}
	_ = SaveCredentials(r.state.Config())
	if _, err := r.state.LoadPlaylists(ctx); err != nil && err != context.Canceled {
		// Login succeeded; keep the account connected and surface catalog
		// failure through the normal platform status instead of leaving the
		// connection dialog stuck open.
		r.state.SetError(err)
	}
	return nil
}

func (r *RuntimeAdapter) Cancel(context.Context) error {
	if r != nil && r.state != nil {
		r.state.Cancel()
	}
	return nil
}

func (r *RuntimeAdapter) Disconnect(ctx context.Context) error {
	if r == nil || r.state == nil {
		return nil
	}
	err := r.state.Close(ctx)
	DeleteCredentials()
	if r.settings != nil {
		r.settings.Config = appplatform.ConnectionConfig{}
	}
	if r.save != nil {
		r.save()
	}
	return err
}

func (r *RuntimeAdapter) FetchArtwork(ctx context.Context, raw string, size int) ([]byte, bool, error) {
	if r == nil || r.state == nil {
		return nil, false, nil
	}
	return r.state.FetchArtwork(ctx, raw, size)
}

func (r *RuntimeAdapter) Transcode() appplatform.TranscodeSettings {
	if r == nil || r.settings == nil {
		return appplatform.TranscodeSettings{Mode: appplatform.TranscodeAuto}
	}
	return appplatform.TranscodeSettings{Mode: r.settings.TranscodeMode, Bitrate: r.settings.TranscodeBitrate}
}

func (r *RuntimeAdapter) SetTranscode(v appplatform.TranscodeSettings) {
	if r == nil || r.settings == nil {
		return
	}
	mode := v.Mode
	if mode != appplatform.TranscodeAuto && mode != appplatform.TranscodeNative && mode != appplatform.TranscodeAlways {
		mode = appplatform.TranscodeAuto
	}
	b := v.Bitrate
	if !backend.ValidTranscodeBitrate(b) {
		b = 0
	}
	r.settings.TranscodeMode, r.settings.TranscodeBitrate = mode, b
	if r.save != nil {
		r.save()
	}
}

func (r *RuntimeAdapter) TranscodeBitrates() []int {
	return append([]int(nil), backend.TranscodeBitrates...)
}

func (r *RuntimeAdapter) Sorts() map[string]string {
	if r == nil || r.settings == nil || len(r.settings.Sorts) == 0 {
		return nil
	}
	out := make(map[string]string, len(r.settings.Sorts))
	for key, value := range r.settings.Sorts {
		out[key] = value
	}
	return out
}

func (r *RuntimeAdapter) SetSort(key, value string) {
	if r == nil || r.settings == nil || key == "" {
		return
	}
	if r.settings.Sorts == nil {
		r.settings.Sorts = map[string]string{}
	}
	if r.settings.Sorts[key] == value {
		return
	}
	r.settings.Sorts[key] = value
	if r.save != nil {
		r.save()
	}
}

func toConnectionConfig(c backend.Config) appplatform.ConnectionConfig {
	return appplatform.ConnectionConfig{Address: c.Address, Username: c.Username, PassHash: c.PassHash, AccessCode: c.AccessCode, DeviceID: c.DeviceID, Mode: c.Mode, HTTPS: c.HTTPS}
}

func fromConnectionConfig(c appplatform.ConnectionConfig) backend.Config {
	return backend.Config{Address: c.Address, Username: c.Username, PassHash: c.PassHash, AccessCode: c.AccessCode, DeviceID: c.DeviceID, Mode: c.Mode, HTTPS: c.HTTPS}
}
