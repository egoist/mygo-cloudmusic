package platform

import (
	"context"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
	coreplatform "netease-music-mygo/internal/platform"
	"netease-music-mygo/internal/platform/paging"
	"netease-music-mygo/internal/player"
)

// Shared platform seam.
//
// These interfaces and value types are the only platform-facing contract the
// root app should depend on. Provider adapters translate their own state into
// these values; this package must not expose a concrete provider's protocol
// model or UI state.

// ConnectionConfig is the provider-neutral connection data exchanged by the
// root application and a platform adapter. A provider may interpret Mode or
// AccessCode differently, but that interpretation stays behind its seam.
type ConnectionConfig struct {
	Address    string `json:"address,omitempty"`
	Username   string `json:"username,omitempty"`
	PassHash   string `json:"-"`
	AccessCode string `json:"-"`
	DeviceID   string `json:"deviceID,omitempty"`
	Mode       string `json:"mode,omitempty"`
	HTTPS      bool   `json:"https,omitempty"`
}

// TranscodeMode and TranscodeSettings are deliberately string based. The
// shared app must not import a provider's enum merely to persist a setting or
// render a selector.
type TranscodeMode string

const (
	TranscodeAuto   TranscodeMode = "auto"
	TranscodeNative TranscodeMode = "native"
	TranscodeAlways TranscodeMode = "always"
)

type TranscodeSettings struct {
	Mode    TranscodeMode
	Bitrate int
}

// PlatformSettings is the provider-owned namespace persisted by the root
// settings file. Provider adapters validate and interpret the values.
type PlatformSettings struct {
	Config           ConnectionConfig  `json:"config,omitempty"`
	TranscodeMode    TranscodeMode     `json:"transcodeMode,omitempty"`
	TranscodeBitrate int               `json:"transcodeBitrate,omitempty"`
	LibrarySort      string            `json:"librarySort,omitempty"`
	Sorts            map[string]string `json:"sorts,omitempty"`
	// Quality, Normalize, HotComments and SearchHistory are provider playback
	// preferences, kept in the provider's own namespace. Optional values fall
	// back to the provider's defaults, not to the shared ones.
	Quality       string   `json:"quality,omitempty"`
	Normalize     *bool    `json:"normalize,omitempty"`
	HotComments   bool     `json:"hotComments,omitempty"`
	SearchHistory []string `json:"searchHistory,omitempty"`
}

// Runtime is the lifecycle seam for a platform account. It contains no
// provider package types; adapters translate to their own client/config.
type Runtime interface {
	Config() ConnectionConfig
	SetConfig(ConnectionConfig)
	Configured() bool
	Connected() bool
	Error() error
	Connect(context.Context, ConnectionConfig) error
	Cancel(context.Context) error
	Disconnect(context.Context) error
	FetchArtwork(context.Context, string, int) ([]byte, bool, error)
	Transcode() TranscodeSettings
	SetTranscode(TranscodeSettings)
	TranscodeBitrates() []int
	Sorts() map[string]string
	SetSort(string, string)
}

// Home is the platform-owned navigation and settings shell. The root app
// only hosts it; it does not inspect platform state or route implementations.
type Home interface {
	Sidebar(*ui.Context)
	Content(*ui.Context)
	Settings(*ui.Context)
	Open(Route)
	CanBack() bool
	CanForward() bool
	Back()
	Forward()
	ResetNavigation(Route)
	CloseTransient()
	Route() Route
	SearchText() *string
}

// Adapter is the complete platform-owned context used by the root app.
// Concrete state, protocol models and provider UI remain behind this seam.
type Adapter interface {
	Info() Info
	Home() Home
	Runtime() Runtime
	ConfigurePlayer(*player.Player)
	OwnsTrack(*netease.Track) bool
	Liked(*netease.Track) bool
	Favorite(context.Context, *netease.Track, bool) error
}

// Accent lets a platform brand the shared chrome with its own colour.
type Accent interface {
	AccentColor() (ui.Color, ui.Color)
}

// CommentSource is optional: a platform that serves comments for its own
// tracks implements it, and the root app stops asking NetEase. Read-only
// sources leave liking to the shared NetEase path by staying unimplemented.
type CommentSource interface {
	CommentThread(*netease.Track) string
	Comments(string, int, int) (*netease.Comments, error)
}

// Info is the provider-neutral identity shown by shared platform controls.
// The root app uses it to enumerate and validate adapters; it never needs to
// name a concrete provider package or compare a provider-specific constant.
type Info struct {
	ID        coreplatform.ID
	Name      string
	Comments  bool
	Podcasts  bool
	Quality   bool
	Normalize bool
}

type Route struct {
	Page  string
	ID    int64
	Query string
	Kind  string
}

type PlayerPort interface {
	Current() *netease.Track
	Playing() bool
	Loading() bool
	Err() string
	TogglePlay()
	Next()
	Queue() []netease.Track
	PlayOrder() ([]int, int)
	PlayQueue([]netease.Track, int)
	AppendQueue([]netease.Track)
	PlayList([]netease.Track, int, func(func([]netease.Track)))
	PlayRadio(string, []netease.Track, func(func([]netease.Track)))
	EndRadio()
	Radio() string
	SetShuffle(bool)
}

// TrackList is a provider-neutral lazy list. A platform supplies callbacks
// around its pager; the root only asks for rows and page boundaries.
type TrackList struct {
	ID       int64
	Kind     string
	Name     string
	Sub      string
	Cover    string
	Total    func() int
	Len      func() int
	At       func(int) *netease.Track
	Around   func(int) ([]netease.Track, int, int)
	When     func(int, func(bool))
	Retry    func()
	Failed   func() bool
	Error    func() error
	PageSize int
}

type AccountAction struct {
	Icon      *ui.SVG
	Label     string
	Shortcut  string
	Separator bool
	Danger    bool
	Action    func()
}

type TrackView struct {
	Title          string
	Sub            string
	Cover          string
	Total          int
	OnPlay         func(int)
	OnShuffle      func()
	OnRefresh      func()
	Header         func(*ui.Context)
	HeaderSkeleton func(*ui.Context)
	Menu           func(*ui.Menu, *netease.Track, int64)
	Liked          func(int64) bool
	Favorite       func(netease.Track, bool)
	ShowAlbum      bool
	Footer         func(*ui.Context)
}

type SearchView struct {
	Play      func(int)
	PlayCard  func(int64)
	TrackMenu func(*ui.Menu, *netease.Track, int64)
	Liked     func(int64) bool
	Favorite  func(netease.Track, bool)
	OpenList  func(int64)
	OpenCards func(string)
}

type SearchResults struct {
	Query   string
	Songs   *TrackList
	Albums  *paging.Pager[netease.Playlist]
	Artists *paging.Pager[netease.Playlist]
}

type CardView struct {
	Title          string
	Sub            string
	Count          int
	Header         func(*ui.Context)
	HeaderSkeleton func(*ui.Context)
	Sort           []SortChoice
	Current        string
	OnSort         func(string)
	OnOpen         func(int64)
	OnPlay         func(int64)
	// Meta reserves the card's secondary line in its loading skeleton.
	Meta bool
	// Round draws an artist's round photo over the name, with no play.
	Round bool
	// NoPlay leaves the card to opening only, as album grids do.
	NoPlay bool
	// Placeholder is shown when a card has no cover at all.
	Placeholder *ui.SVG
	// Art draws the cover of a card that has none, into its square.
	Art func(p *ui.Painter, r ui.Rect, card *netease.Playlist, radius float32)
	// Footer draws under the cards, for pages that load more.
	Footer func(*ui.Context)
}

type SortChoice struct{ Value, Label string }

// Host contains only application-wide controls and rendering primitives.
type Host interface {
	Post(func())
	Invalidate()
	Navigate(Route)
	CurrentPlatform() coreplatform.ID
	ShowMain()
	OpenSettings()
	Toast(string)
	Player() PlayerPort
	PlatformSearchField(*ui.Context, string, func(string))
	PlatformOptions() []Info
	PlatformSwitcher(*ui.Context)
	NavItem(*ui.Context, *ui.SVG, string, Route)
	PlaylistHeader(*ui.Context, func())
	PlaylistItem(*ui.Context, string, netease.Playlist, bool, func(*ui.Menu), func())
	NowPlaying(*ui.Context, *netease.Track)
	RenderTracks(*ui.Context, *TrackList, TrackView)
	RenderCards(*ui.Context, *paging.Pager[netease.Playlist], CardView)
	RenderSearch(*ui.Context, SearchResults, SearchView)
	TrackSkeleton(*ui.Context, TrackView)
	CardsSkeleton(*ui.Context, CardView)
	SearchSkeleton(*ui.Context)
	// PageList is the scrolling page list under the shared top and player
	// bars, with the same insets and edge marks as the track list.
	PageList(c *ui.Context, key string, state *ui.ListState, rows int, row func(int))
	// TrackRow is one song row of the shared list; view supplies its play,
	// menu and heart actions.
	TrackRow(c *ui.Context, index int, tr *netease.Track, view TrackView)
	// Bone and TextBone are blocks of the shared skeleton, shimmering in
	// step with the page's: one as big as it is laid out, one a line of
	// text at size.
	Bone(c *ui.Context, radius float32) ui.Element
	TextBone(c *ui.Context, size, lineHeight float32) ui.Element
	Cover(*ui.Context, string, float32, float32) ui.Element
	// CoverBitmap is a cover's bitmap, nil until it loads, for drawing it
	// inside a record's label.
	CoverBitmap(string) *ui.Bitmap
	// ArtworkBackdrop is a cover's blurred bitmap, for large page backdrops.
	// It is nil until the cover loads.
	ArtworkBackdrop(string) *ui.Bitmap
	// ArtworkColors are the deep, rich and glow tones a large backdrop
	// paints from the cover's dominant colour.
	ArtworkColors(string) (ui.Color, ui.Color, ui.Color, bool)
	PillButton(*ui.Context, string, *ui.SVG, string, bool) ui.Element
	MoreButton(*ui.Context, string, func(*ui.Menu)) ui.Element
	Segmented(*ui.Context, string, *int, ...string) bool
	AccountRow(*ui.Context, *ui.SVG, string, string, string, *bool, []AccountAction)
	CopyText(string)
	// SearchHistory is the shared search history, newest first.
	SearchHistory() []string
	RememberSearch(string)
	SwitchPlatform(coreplatform.ID)
	SharedSettings(*ui.Context)
}
