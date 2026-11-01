package app

import (
	appplatform "netease-music-mygo/internal/app/platform"
	appfnmusic "netease-music-mygo/internal/app/platform/fnmusic"
	"netease-music-mygo/internal/platform"
)

// Concrete adapter registration.
//
// This is the root application's only bootstrap point that names a concrete
// provider package. Keep provider construction here; the rest of the app
// should communicate through internal/app/platform interfaces.

// newPlatformAdapters is the only root bootstrap point that names a concrete
// adapter. All later app code uses the platform seam and can remain unchanged
// when another provider is added.
func newPlatformAdapters(a *app) {
	if a == nil {
		return
	}
	if a.platforms == nil {
		a.platforms = map[platform.ID]appplatform.Adapter{}
	}
	a.platforms[platform.FnmusicID] = appfnmusic.NewAdapter(platformHost{a: a}, &a.settings.FnMusic, func() { _ = a.settings.save() })
}
