package platform

import (
	"context"
	"errors"
	"net/http"

	"netease-music-mygo/internal/netease"
)

// Shared provider seam.
//
// Providers translate their service data into the models used by the shared
// player and views. Keep this package provider-neutral; concrete protocol
// clients and UI state belong in their provider packages.

// ID identifies a music platform in settings and in a platform session.
type ID string

const (
	// NeteaseID is the upstream default platform.
	NeteaseID ID = "netease"
	// FnmusicID identifies the fnOS/Feiniu Music platform.
	FnmusicID ID = "fnmusic"
	// QQMusicID identifies the QQ Music platform.
	QQMusicID ID = "qqmusic"
)

// ListKind describes a list a provider can page through. The values are
// deliberately service-neutral; a provider maps them to its own endpoints.
type ListKind string

const (
	ListLibrary   ListKind = "library"
	ListFavorites ListKind = "favorites"
	ListRecent    ListKind = "recent"
	ListPlaylist  ListKind = "playlist"
	ListAlbum     ListKind = "album"
	ListArtist    ListKind = "artist"
	ListAlbums    ListKind = "albums"
	ListArtists   ListKind = "artists"
)

// PageRequest selects one page of a provider list. Page is zero based at the
// seam; adapters convert it when their service uses one based pages.
type PageRequest struct {
	Kind  ListKind
	Ref   string
	Query string
	Sort  string
	Page  int
	Size  int
}

// Page is the common result for song and card lists. A provider fills only
// the fields relevant to the requested kind and always reports Total when
// the service provides it.
type Page struct {
	Tracks    []netease.Track
	Playlists []netease.Playlist
	Total     int
}

// SearchKind selects the result type returned by Search.
type SearchKind ListKind

const (
	SearchSongs   SearchKind = "songs"
	SearchAlbums  SearchKind = "albums"
	SearchArtists SearchKind = "artists"
)

// Lyrics is the common lyric payload. Translation and word-timed lyrics are
// optional because not every service supplies them.
type Lyrics struct {
	LRC         string
	Translation string
	YRC         string
}

// Provider is the small public interface used by the app's platform seam.
// Connection, playlist editing, likes and transcoding are optional
// capabilities below, so a caller does not need to type-switch on a platform
// name to discover what it supports.
type Provider interface {
	ID() ID
	Name() string

	Page(context.Context, PageRequest) (Page, error)
	Search(context.Context, string, SearchKind, int, int) (Page, error)
	Stream(context.Context, *netease.Track) (*netease.Stream, error)
	Lyrics(context.Context, *netease.Track) (Lyrics, error)
	Cover(context.Context, string, int) ([]byte, error)
}

// Connector is implemented by providers that have a sign-in or connection
// lifecycle, such as a local music server.
type Connector interface {
	Provider
	Connect(context.Context) error
	Close() error
	Connected() bool
}

// Liker is an optional capability for platforms with a liked-song list.
type Liker interface {
	SetLiked(context.Context, *netease.Track, bool) error
}

// PlaylistEditor is an optional capability for platforms whose playlists can
// be changed by the user.
type PlaylistEditor interface {
	CreatePlaylist(context.Context, string) (netease.Playlist, error)
	RenamePlaylist(context.Context, netease.Playlist, string) error
	DeletePlaylist(context.Context, netease.Playlist) error
	AddToPlaylist(context.Context, netease.Playlist, []netease.Track) error
	RemoveFromPlaylist(context.Context, netease.Playlist, []netease.Track) error
}

// Transcoder is an optional capability. The returned stream uses the shared
// player stream model; the concrete HLS/session implementation stays inside
// the provider adapter.
type Transcoder interface {
	ShouldTranscode(*netease.Track) bool
	TranscodedStream(context.Context, *netease.Track, int) (*netease.Stream, error)
}

// ErrUnsupported is returned when a provider cannot represent a requested
// operation. It lets the UI hide an unsupported capability instead of
// guessing from a platform ID.
var ErrUnsupported = errors.New("platform: unsupported operation")

// HTTPHeaders is an optional capability for a stream that needs signed
// request headers. It is kept separate from Provider so ordinary providers
// remain simple.
type HTTPHeaders interface {
	StreamHeaders(*netease.Track) http.Header
}
