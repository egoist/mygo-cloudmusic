package fnmusic

import (
	"context"
	"errors"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform/fnmusic"
	"netease-music-mygo/internal/platform/paging"
)

const (
	PageSize  = 100
	PageAhead = 2
	CardsPage = 60
)

// ListKind describes a Feiniu catalog at the data layer. It does not select a
// UI; the platform home chooses how to present each list.
type ListKind string

const (
	ListLibrary   ListKind = "library"
	ListFavorites ListKind = "favorites"
	ListRecent    ListKind = "recent"
	ListPlaylist  ListKind = "playlist"
	ListAlbum     ListKind = "album"
	ListArtist    ListKind = "artist"
	ListSearch    ListKind = "search"
	ListGenre     ListKind = "genre"
)

// Spec identifies a service list without referring to a root app route.
type Spec struct {
	Kind  ListKind
	Ref   string
	Query string
	Sort  string
}

// List is a lazily loaded Feiniu catalog. Its pager is platform state, not UI
// state, so a view can be rebuilt without losing loaded pages.
type List struct {
	ID         int64
	Spec       Spec
	Kind       string
	HeaderKind string
	Name       string
	Sub        string
	CoverURL   string
	Fetch      func(page, size int) ([]netease.Track, int, error)
	Pages      *paging.Pager[netease.Track]
	Shuffle    bool
}

// View adapts the provider-owned pager to the application-wide list seam.
// The root renderer receives callbacks, never this platform's List type.
func (l *List) View() *appplatform.TrackList {
	if l == nil || l.Pages == nil {
		return nil
	}
	return &appplatform.TrackList{
		ID: l.ID, Kind: l.Kind, Name: l.Name, Sub: l.Sub, Cover: l.Cover(),
		Total: l.Pages.Total, Len: l.Pages.Len, At: l.Pages.At, Around: l.Pages.Around,
		When: l.Pages.When, Retry: func() { l.Pages.Retry() }, PageSize: PageSize,
		Failed: func() bool { return l.Pages.FailedAt(0) }, Error: func() error { return l.Pages.Error() },
	}
}

func (l *List) Cover() string {
	if l == nil {
		return ""
	}
	if l.CoverURL != "" {
		return l.CoverURL
	}
	if t := l.Pages.At(0); t != nil {
		return t.Album.PicURL
	}
	return ""
}

// SetSort changes the order of a loaded list without dropping its geometry.
func (l *List) SetSort(sort string) {
	if l == nil || l.Spec.Sort == sort {
		return
	}
	l.Spec.Sort = sort
	if l.Pages != nil {
		l.Pages.Reload()
	}
}

// Refresh asks for the current list again instead of trusting its loaded
// pages. Retry alone only touches failed pages and leaves cached rows in place.
func (l *List) Refresh() {
	if l == nil || l.Pages == nil {
		return
	}
	l.Pages.Reload()
	l.Pages.At(0)
}

// Catalog owns list and card state for one Feiniu session.
type Catalog struct {
	Session  *State
	Post     func(func())
	OnTracks func([]netease.Track)

	Lists                       map[int64]*List
	Meta                        map[int64]netease.Playlist
	SearchList                  *List
	SearchAlbums, SearchArtists *paging.Pager[netease.Playlist]
	Albums, Artists, Genres     *paging.Pager[netease.Playlist]
	ArtistAlbums                map[int64]*paging.Pager[netease.Playlist]

	LibrarySort, FavoriteSort, AlbumSort, AlbumDetailSort string
	ArtistSort, ArtistTrackSort, ArtistAlbumSort          string
	GenreSort, GenreTrackSort                             string
}

// NewCatalog creates empty platform catalog state.
func NewCatalog(session *State, post func(func())) *Catalog {
	if post == nil {
		post = func(fn func()) { fn() }
	}
	return &Catalog{
		Session: session, Post: post,
		Lists: map[int64]*List{}, Meta: map[int64]netease.Playlist{},
		ArtistAlbums:    map[int64]*paging.Pager[netease.Playlist]{},
		LibrarySort:     fnmusic.LibrarySortPolicy.Default,
		FavoriteSort:    fnmusic.FavoriteSortPolicy.Default,
		AlbumSort:       fnmusic.AlbumListSortPolicy.Default,
		AlbumDetailSort: fnmusic.AlbumTrackSortPolicy.Default,
		ArtistSort:      fnmusic.ArtistListSortPolicy.Default,
		ArtistTrackSort: fnmusic.ArtistTrackSortPolicy.Default,
		ArtistAlbumSort: fnmusic.ArtistAlbumSortPolicy.Default,
		GenreSort:       fnmusic.GenreListSortPolicy.Default,
		GenreTrackSort:  fnmusic.GenreTrackSortPolicy.Default,
	}
}

func (c *Catalog) trackPager(fetch func(page int) ([]netease.Track, int, error)) *paging.Pager[netease.Track] {
	p := paging.New(PageSize, PageAhead, c.Post, fetch)
	p.SetLanded(func(_ int, items []netease.Track) {
		if c.OnTracks != nil {
			c.OnTracks(items)
		}
	})
	return p
}

func (c *Catalog) cardPager(fetch func(page int) ([]netease.Playlist, int, error)) *paging.Pager[netease.Playlist] {
	p := paging.New(CardsPage, 1, c.Post, fetch)
	p.SetLanded(func(_ int, items []netease.Playlist) {
		for _, item := range items {
			c.Meta[item.ID] = item
		}
	})
	return p
}

func (c *Catalog) cardFetch(client *fnmusic.Client, gen uint64, fetch func(int) ([]netease.Playlist, int, error)) func(int) ([]netease.Playlist, int, error) {
	return func(page int) ([]netease.Playlist, int, error) {
		if c == nil || c.Session == nil || !c.Session.clientCurrent(client, gen) {
			return nil, 0, context.Canceled
		}
		items, total, err := fetch(page)
		if err == nil && !c.Session.clientCurrent(client, gen) {
			return nil, 0, context.Canceled
		}
		return items, total, err
	}
}

// NewList builds a list from a service-neutral spec.
func (c *Catalog) NewList(spec Spec) *List {
	if c == nil || c.Session == nil {
		return nil
	}
	client, gen, err := c.Session.activeClient(context.Background())
	if err != nil {
		return nil
	}
	l := &List{Spec: spec, ID: listID(spec), Fetch: nil}
	guard := func(fetch func(int, int) ([]netease.Track, int, error)) func(int, int) ([]netease.Track, int, error) {
		return func(page, size int) ([]netease.Track, int, error) {
			if !c.Session.clientCurrent(client, gen) {
				return nil, 0, context.Canceled
			}
			tracks, total, err := fetch(page, size)
			if err == nil && !c.Session.clientCurrent(client, gen) {
				return nil, 0, context.Canceled
			}
			return tracks, total, err
		}
	}
	switch spec.Kind {
	case ListLibrary:
		l.Kind, l.Name = "飞牛音乐", "音乐库"
		l.HeaderKind = "音乐库"
		l.Fetch = guard(func(page, size int) ([]netease.Track, int, error) {
			return client.LibraryPage(l.Spec.Sort, page, size)
		})
	case ListFavorites:
		l.Kind, l.Name = "飞牛音乐", "我喜欢"
		l.HeaderKind = "歌单"
		l.Fetch = guard(func(page, size int) ([]netease.Track, int, error) {
			return client.FavoritesPage(l.Spec.Sort, page, size)
		})
	case ListRecent:
		l.Kind, l.Name = "飞牛音乐", "最近播放"
		l.HeaderKind = "播放记录"
		l.Fetch = guard(client.RecentPage)
	case ListAlbum:
		l.Kind, l.HeaderKind = fnmusic.KindAlbum, "专辑"
		l.Fetch = guard(func(page, size int) ([]netease.Track, int, error) {
			return client.AlbumPage(spec.Ref, l.Spec.Sort, page, size)
		})
	case ListArtist:
		l.Kind, l.HeaderKind = fnmusic.KindArtist, "歌手"
		l.Fetch = guard(func(page, size int) ([]netease.Track, int, error) {
			return client.ArtistPage(spec.Ref, l.Spec.Sort, page, size)
		})
	case ListPlaylist:
		l.Kind, l.HeaderKind = "歌单", "歌单"
		l.Fetch = guard(func(page, size int) ([]netease.Track, int, error) { return client.PlaylistPage(spec.Ref, page, size) })
	case ListGenre:
		l.Kind, l.HeaderKind = fnmusic.KindGenre, "风格"
		l.Fetch = guard(func(page, size int) ([]netease.Track, int, error) {
			res, err := client.GenreTracksPage(spec.Ref, l.Spec.Sort, page, size)
			if err != nil {
				return nil, 0, err
			}
			tracks := make([]netease.Track, len(res.List))
			for i, item := range res.List {
				tracks[i] = item.Track
			}
			return tracks, res.Total, nil
		})
	case ListSearch:
		l.Kind, l.HeaderKind, l.Name = "搜索", "搜索", spec.Query
		l.Fetch = guard(func(page, size int) ([]netease.Track, int, error) { return client.SearchPage(spec.Query, page, size) })
	default:
		return nil
	}
	l.Pages = c.trackPager(func(page int) ([]netease.Track, int, error) { return l.Fetch(page, PageSize) })
	return l
}

func listID(spec Spec) int64 {
	switch spec.Kind {
	case ListLibrary:
		return fnmusic.LibraryID
	case ListFavorites:
		return fnmusic.FavoritesID
	case ListRecent:
		return fnmusic.RecentID
	case ListAlbum:
		return fnmusic.AlbumID(spec.Ref)
	case ListArtist:
		return fnmusic.ArtistID(spec.Ref)
	case ListPlaylist:
		return fnmusic.ID(spec.Ref)
	case ListGenre:
		return fnmusic.GenreID(spec.Ref)
	default:
		return -3
	}
}

// ListFor returns or constructs the list represented by id.
func (c *Catalog) ListFor(id int64) *List {
	return c.ListForRoute(id, "", "")
}

// ListForRoute resolves a card using the provider-neutral route metadata.
// Cards carry a stable app ID for rendering, while Ref preserves the service
// identifier needed when the card's metadata has not reached Catalog.Meta
// yet (for example immediately after a reconnect).
func (c *Catalog) ListForRoute(id int64, kind, ref string) *List {
	if c == nil {
		return nil
	}
	if l := c.Lists[id]; l != nil {
		l.SetSort(c.listSort(l))
		return l
	}
	var spec Spec
	var meta netease.Playlist
	switch id {
	case fnmusic.LibraryID:
		spec = Spec{Kind: ListLibrary, Sort: c.LibrarySort}
	case fnmusic.FavoritesID:
		spec = Spec{Kind: ListFavorites, Sort: c.FavoriteSort}
	case fnmusic.RecentID:
		spec = Spec{Kind: ListRecent}
	default:
		var ok bool
		meta, ok = c.Meta[id]
		if !ok && ref != "" {
			meta = netease.Playlist{ID: id, Ref: ref, Kind: kind}
			ok = true
		}
		if !ok && c.Session != nil {
			for _, pl := range c.Session.Playlists() {
				if pl.ID == id {
					meta, ok = pl, true
					break
				}
			}
		}
		if !ok {
			return nil
		}
		// Album and artist routes require the provider GUID. Never turn a
		// partially resolved card into a playlist request: that endpoint can
		// return an unrelated list and was the source of the card crash path.
		if meta.Ref == "" {
			return nil
		}
		spec.Ref = meta.Ref
		switch meta.Kind {
		case fnmusic.KindAlbum:
			spec.Kind, spec.Sort = ListAlbum, c.AlbumDetailSort
		case fnmusic.KindArtist:
			spec.Kind, spec.Sort = ListArtist, c.ArtistTrackSort
		case fnmusic.KindGenre:
			spec.Kind, spec.Sort = ListGenre, c.GenreTrackSort
		default:
			spec.Kind = ListPlaylist
		}
	}
	l := c.NewList(spec)
	if l == nil {
		return nil
	}
	if meta.Name != "" {
		l.Name, l.Sub, l.CoverURL = meta.Name, meta.Sub, meta.Cover()
	}
	c.Lists[id] = l
	return l
}

func (c *Catalog) listSort(l *List) string {
	if l == nil {
		return ""
	}
	switch l.Spec.Kind {
	case ListLibrary:
		return fnmusic.LibrarySortPolicy.Normalize(c.LibrarySort)
	case ListFavorites:
		return fnmusic.FavoriteSortPolicy.Normalize(c.FavoriteSort)
	case ListAlbum:
		return fnmusic.AlbumTrackSortPolicy.Normalize(c.AlbumDetailSort)
	case ListArtist:
		return fnmusic.ArtistTrackSortPolicy.Normalize(c.ArtistTrackSort)
	case ListGenre:
		return fnmusic.GenreTrackSortPolicy.Normalize(c.GenreTrackSort)
	default:
		return ""
	}
}

// FavoriteChanged drops or updates the cached favorites list after a
// successful like change.
func (c *Catalog) FavoriteChanged(id int64, liked bool) {
	if c == nil {
		return
	}
	if !liked && c.RemoveFavorite(id) {
		return
	}
	c.Refresh(fnmusic.FavoritesID)
}

// RemoveFavorite takes a song out of the loaded favorites list without
// refetching every page.
func (c *Catalog) RemoveFavorite(id int64) bool {
	if c == nil {
		return false
	}
	l := c.Lists[fnmusic.FavoritesID]
	if l == nil || l.Pages == nil {
		return false
	}
	index := l.Pages.IndexOf(func(tr *netease.Track) bool { return tr != nil && tr.ID == id })
	if index < 0 {
		return false
	}
	l.Pages.RemoveAt(index)
	return true
}

// Refresh drops one list so the next view asks the service again.
func (c *Catalog) Refresh(id int64) {
	if c == nil {
		return
	}
	delete(c.Lists, id)
}

// Search creates the song, album and artist catalogs for one query.
func (c *Catalog) Search(query string) error {
	if c == nil || c.Session == nil || c.Session.Client() == nil {
		return errors.New("fnmusic: not connected")
	}
	client, gen, err := c.Session.activeClient(context.Background())
	if err != nil {
		return err
	}
	c.SearchList = c.NewList(Spec{Kind: ListSearch, Query: query})
	c.SearchAlbums = c.cardPager(c.cardFetch(client, gen, func(page int) ([]netease.Playlist, int, error) {
		return client.AlbumsPage(fnmusic.SearchAlbumsPath, "", query, page, CardsPage)
	}))
	c.SearchArtists = c.cardPager(c.cardFetch(client, gen, func(page int) ([]netease.Playlist, int, error) {
		return client.ArtistsPage(fnmusic.SearchArtistsPath, "", query, page, CardsPage)
	}))
	return nil
}

func (c *Catalog) AlbumsList() *paging.Pager[netease.Playlist] {
	if c.Albums == nil && c.Session != nil && c.Session.Client() != nil {
		client := c.Session.Client()
		_, gen, _ := c.Session.activeClient(context.Background())
		c.Albums = c.cardPager(c.cardFetch(client, gen, func(page int) ([]netease.Playlist, int, error) {
			return client.AlbumsPage(fnmusic.AlbumsPath, fnmusic.AlbumListSortPolicy.Normalize(c.AlbumSort), "", page, CardsPage)
		}))
	}
	return c.Albums
}

func (c *Catalog) ArtistsList() *paging.Pager[netease.Playlist] {
	if c.Artists == nil && c.Session != nil && c.Session.Client() != nil {
		client := c.Session.Client()
		_, gen, _ := c.Session.activeClient(context.Background())
		c.Artists = c.cardPager(c.cardFetch(client, gen, func(page int) ([]netease.Playlist, int, error) {
			return client.ArtistsPage(fnmusic.ArtistsPath, fnmusic.ArtistListSortPolicy.Normalize(c.ArtistSort), "", page, CardsPage)
		}))
	}
	return c.Artists
}

// ArtistAlbumsList is the albums of one artist, in the order of
// ArtistAlbumSort.
func (c *Catalog) ArtistAlbumsList(id int64, ref string) *paging.Pager[netease.Playlist] {
	if c == nil || ref == "" || c.Session == nil || c.Session.Client() == nil {
		return nil
	}
	if c.ArtistAlbums == nil {
		c.ArtistAlbums = map[int64]*paging.Pager[netease.Playlist]{}
	}
	if p := c.ArtistAlbums[id]; p != nil {
		return p
	}
	client := c.Session.Client()
	_, gen, _ := c.Session.activeClient(context.Background())
	p := c.cardPager(c.cardFetch(client, gen, func(page int) ([]netease.Playlist, int, error) {
		return client.ArtistAlbumsPage(ref, fnmusic.ArtistAlbumSortPolicy.Normalize(c.ArtistAlbumSort), page, CardsPage)
	}))
	c.ArtistAlbums[id] = p
	return p
}

// GenresList is the style cards, in the order of GenreSort.
func (c *Catalog) GenresList() *paging.Pager[netease.Playlist] {
	if c.Genres == nil && c.Session != nil && c.Session.Client() != nil {
		client := c.Session.Client()
		_, gen, _ := c.Session.activeClient(context.Background())
		c.Genres = c.cardPager(c.cardFetch(client, gen, func(page int) ([]netease.Playlist, int, error) {
			res, err := client.GenresPage(fnmusic.ParseGenreSort(c.GenreSort), page, CardsPage)
			if err != nil {
				return nil, 0, err
			}
			cards := make([]netease.Playlist, 0, len(res.List))
			for _, g := range res.List {
				if g.GUID != "" {
					cards = append(cards, g.Card())
				}
			}
			return cards, res.Total, nil
		}))
	}
	return c.Genres
}
