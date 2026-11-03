package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/platform"
)

// Platform-owned application state.
//
// Keep provider selection, adapter storage and platform-only view state out
// of the upstream app structure. The anonymous field lets existing app code
// use the promoted names while app.go needs only one extension field.
type appPlatformState struct {
	platformID         platform.ID
	platforms          map[platform.ID]appplatform.Adapter
	platformGeneration uint64
	platformCardList   ui.ListState
	platformRoute      appplatform.Route
	platformHeroSince  time.Time
	platformMenuOpen   bool
}
