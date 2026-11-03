package fnmusic

import appplatform "netease-music-mygo/internal/app/platform"

// Page identifies a route owned by a platform UI. The root app translates it
// to its shared route type; the platform package never imports that type.
type Page = string

const (
	PageLibrary       Page = "library"
	PageSearch        Page = "search"
	PageSearchAlbums  Page = "search-albums"
	PageSearchArtists Page = "search-artists"
	PageAlbums        Page = "albums"
	PageArtists       Page = "artists"
	PageArtistAlbums  Page = "artist-albums"
	PagePlaylist      Page = "playlist"
	PageRoam          Page = "roam"
	PageFolders       Page = "folders"
	PageGenres        Page = "genres"
)

// Route is the small navigation value exchanged between a platform view and
// the root app. It deliberately contains no UI elements or app internals.
type Route = appplatform.Route

// PlayerPort is the shared playback seam used by platform queue actions.
// The concrete player remains in the root app.
type PlayerPort = appplatform.PlayerPort

// Host is the lifecycle seam for platform state and views. It exposes shared
// drawing primitives and application actions; the platform package never
// receives *app or a collection of root callbacks.
type Host interface {
	appplatform.Host
}

type AccountAction = appplatform.AccountAction

// TrackView describes a platform list without exposing root app routes or UI
// list state to the platform package.
type TrackView = appplatform.TrackView

type SearchView = appplatform.SearchView

// CardView describes an album, artist or playlist card page.
type CardView = appplatform.CardView

type SortChoice = appplatform.SortChoice

// Runtime is kept as an alias for platform-owned code. The interface itself
// lives in the shared app seam and contains no Feiniu protocol types.
type Runtime = appplatform.Runtime
