package qqmusic

import appplatform "netease-music-mygo/internal/app/platform"

type Page = string

const (
	PageHome        Page = "home"
	PageDaily       Page = "daily"
	PageFM          Page = "fm"
	PageCharts      Page = "charts"
	PagePlaylist    Page = "playlist"
	PageAlbum       Page = "album"
	PageArtist      Page = "artist"
	PageSearch      Page = "search"
	PagePurchased   Page = "purchased"
	PageLiked       Page = "liked"
	PageUser        Page = "user"
	PageLikedAlbums Page = "liked-albums"
)

type Route = appplatform.Route
type PlayerPort = appplatform.PlayerPort
type Host interface{ appplatform.Host }
type AccountAction = appplatform.AccountAction
type TrackView = appplatform.TrackView
type SearchView = appplatform.SearchView
type SearchResults = appplatform.SearchResults
type CardView = appplatform.CardView
type SortChoice = appplatform.SortChoice
type Runtime = appplatform.Runtime
