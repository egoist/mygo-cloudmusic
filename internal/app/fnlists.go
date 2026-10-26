package app

import (
	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/fnmusic"
	"netease-music-mygo/internal/netease"
)

// fnPageSize is how many songs a request of the service asks for, and
// fnAhead how many pages are read ahead of the row on view: at a reading
// pace of a few rows a second, a hundred rows are half a minute, and one
// page ahead is read well before it is needed.
const (
	fnPageSize  = 100
	fnAhead     = 2
	fnCardsPage = 60
)

// The kinds of list of songs the service has.
const (
	listLibrary   = "library"
	listFavorites = "favorites"
	listPlaylist  = "playlist"
	listAlbum     = "album"
	listArtist    = "artist"
	listSearch    = "search"
)

// fnSpec says which list of songs of the service is meant, in words that
// make it again: what the session keeps of the list a queue is read from.
type fnSpec struct {
	typ  string
	ref  string // the GUID of a playlist, an album or an artist
	q    string // what a search is for
	sort string // the order of the library
}

// fnList is a list of songs of the service, read as it is scrolled to: its
// library, the songs liked, a playlist, an album, an artist, or what a
// search finds.
type fnList struct {
	id   int64
	spec fnSpec
	kind string // what the page calls it: "飞牛音乐", "专辑", "歌手", "歌单", "搜索"
	name string
	sub  string // under its name: an album's artist
	// cover is the picture of an album, an artist or a playlist, which a
	// library or a search has none of: the first song's stands in.
	cover string
	// fetch reads a page of the list, from 1, of size songs.
	fetch   func(page, size int) ([]netease.Track, int, error)
	pg      *pager[netease.Track]
	shuffle bool // a shuffle is being started
}

// Cover returns the picture to show for the list.
func (l *fnList) Cover() string {
	if l.cover != "" {
		return l.cover
	}
	if t := l.pg.at(0); t != nil {
		return t.Album.PicURL
	}
	return ""
}

// trackPager makes a pager of songs, which keeps the hearts of those it
// reads true to what the service says.
func (a *app) trackPager(fetch func(page int) ([]netease.Track, int, error)) *pager[netease.Track] {
	p := newPager(fnPageSize, fnAhead, a.fnPost, fetch)
	p.landed = func(_ int, items []netease.Track) { a.noteHearts(items) }
	return p
}

// noteHearts takes what the service says of which songs are liked.
func (a *app) noteHearts(items []netease.Track) {
	for _, t := range items {
		// A song being liked is the listener's word, not the page's.
		if !a.fn.likeBusy[t.ID] {
			a.setFnLiked(t.ID, t.Fav)
		}
	}
}

// listID is the number the app gives the list a spec names.
func listID(s fnSpec) int64 {
	switch s.typ {
	case listLibrary:
		return fnmusic.LibraryID
	case listFavorites:
		return fnmusic.FavoritesID
	case listAlbum:
		return fnmusic.AlbumID(s.ref)
	case listArtist:
		return fnmusic.ArtistID(s.ref)
	case listPlaylist:
		return fnmusic.ID(s.ref)
	}
	return -3 // a search
}

// newFnList makes the list spec names, nil if there is no service or no
// such kind. The page it is for sets its name and cover.
func (a *app) newFnList(s fnSpec) *fnList {
	c := a.fn.client
	if c == nil {
		return nil
	}
	l := &fnList{id: listID(s), spec: s}
	switch s.typ {
	case listLibrary:
		sort := s.sort
		if sort == "" {
			sort = fnmusic.SortRecent
		}
		l.kind, l.name = "飞牛音乐", "音乐库"
		l.fetch = func(pg, size int) ([]netease.Track, int, error) { return c.LibraryPage(sort, pg, size) }
	case listFavorites:
		l.kind, l.name = "飞牛音乐", "我喜欢"
		l.fetch = func(pg, size int) ([]netease.Track, int, error) { return c.FavoritesPage(pg, size) }
	case listAlbum:
		l.kind = fnmusic.KindAlbum
		l.fetch = func(pg, size int) ([]netease.Track, int, error) { return c.AlbumPage(s.ref, pg, size) }
	case listArtist:
		l.kind = fnmusic.KindArtist
		l.fetch = func(pg, size int) ([]netease.Track, int, error) { return c.ArtistPage(s.ref, pg, size) }
	case listPlaylist:
		l.kind = "歌单"
		l.fetch = func(pg, size int) ([]netease.Track, int, error) { return c.PlaylistPage(s.ref, pg, size) }
	case listSearch:
		l.kind, l.name = "搜索", s.q
		l.fetch = func(pg, size int) ([]netease.Track, int, error) { return c.SearchPage(s.q, pg, size) }
	default:
		return nil
	}
	l.pg = a.trackPager(func(pg int) ([]netease.Track, int, error) { return l.fetch(pg, fnPageSize) })
	return l
}

// fnListFor returns the list of songs numbered id, made when first asked
// for, or nil if it is of nothing known: a card not seen.
func (a *app) fnListFor(id int64) *fnList {
	if l := a.fn.lists[id]; l != nil {
		return l
	}
	var spec fnSpec
	var m netease.Playlist
	switch id {
	case fnmusic.LibraryID:
		spec = fnSpec{typ: listLibrary, sort: a.fn.libSort}
	case fnmusic.FavoritesID:
		spec = fnSpec{typ: listFavorites}
	default:
		var ok bool
		if m, ok = a.fnMeta(id); !ok {
			return nil
		}
		spec = fnSpec{ref: m.Ref}
		switch m.Kind {
		case fnmusic.KindAlbum:
			spec.typ = listAlbum
		case fnmusic.KindArtist:
			spec.typ = listArtist
		default:
			spec.typ = listPlaylist
		}
	}
	l := a.newFnList(spec)
	if l == nil {
		return nil
	}
	if m.Name != "" {
		l.name, l.sub, l.cover = m.Name, m.Sub, m.Cover()
	}
	if a.fn.lists == nil {
		a.fn.lists = map[int64]*fnList{}
	}
	a.fn.lists[id] = l
	return l
}

// fnMeta returns what the card or playlist numbered id stands for.
func (a *app) fnMeta(id int64) (netease.Playlist, bool) {
	if m, ok := a.fn.meta[id]; ok {
		return m, true
	}
	for _, pl := range a.fn.playlists {
		if pl.ID == id {
			return pl, true
		}
	}
	return netease.Playlist{}, false
}

// openFnList makes sure the list numbered id is on its way, and plays it
// from its first song once that is in, if asked: the play button of a
// card.
func (a *app) openFnList(id int64, thenPlay bool) {
	l := a.fnListFor(id)
	if l == nil {
		a.plErr[id] = "找不到这个列表"
		return
	}
	l.pg.at(0)
	if thenPlay {
		a.playFirst(l)
	}
}

// playFirst plays the list from its first song, once it is in.
func (a *app) playFirst(l *fnList) {
	l.pg.when(0, func(ok bool) {
		if !ok {
			a.toast = "加载失败，请稍后再试"
			return
		}
		a.playFn(l, 0)
	})
}

// playAllFn plays the list in order from its first song, however the
// player was set to shuffle.
func (a *app) playAllFn(l *fnList) {
	a.pl.SetShuffle(false)
	a.playFirst(l)
}

// setFnLibSort lists the library in another order, from its start.
func (a *app) setFnLibSort(sort string) {
	if a.fn.libSort == sort || a.fn.libSort == "" && sort == fnmusic.SortRecent {
		return
	}
	a.fn.libSort = sort
	delete(a.fn.lists, fnmusic.LibraryID)
	a.resetListScroll()
}

// resetListScroll puts the page's list back at its start.
//
// It is not done at once: the controls that ask for it (the sort of a list
// is chosen from its header) are rows of the very list that is put back,
// which cannot be, in the middle of its own click. content does it at the
// start of the next frame.
func (a *app) resetListScroll() { a.scrollReset = true }

// refreshFn reads the list numbered id again, from its start.
func (a *app) refreshFn(id int64) {
	delete(a.fn.lists, id)
	a.resetListScroll()
	if id == fnmusic.LibraryID || id == fnmusic.FavoritesID {
		return
	}
	a.loadFnAccount()
}

// fnSearch finds what the service has for q: songs, albums and artists, each
// by the service, which searches its library as the web player does. There
// is no search of our own to be had: the library is not here whole.
func (a *app) fnSearch(q string) {
	a.searched = route{page: pageSearch, query: q, kind: netease.SearchSongs}
	c := a.fn.client
	if c == nil {
		return
	}
	a.fn.search = a.newFnList(fnSpec{typ: listSearch, q: q})
	a.fn.searchAlbums = a.cardPager(func(pg int) ([]netease.Playlist, int, error) {
		return c.AlbumsPage(fnmusic.SearchAlbumsPath, "", q, pg, fnCardsPage)
	})
	a.fn.searchArtists = a.cardPager(func(pg int) ([]netease.Playlist, int, error) {
		return c.ArtistsPage(fnmusic.SearchArtistsPath, "", q, pg, fnCardsPage)
	})
	a.fn.foundAlbumList, a.fn.foundArtistList = ui.ListState{}, ui.ListState{}
}

// fnAlbums and fnArtists return the pagers of the cards of the albums and
// the artists, made when first asked for. A card puts what it stands for
// where its page can open it.
func (a *app) fnAlbums() *pager[netease.Playlist] {
	if a.fn.albums == nil && a.fn.client != nil {
		c, sort := a.fn.client, a.fn.albumSort
		a.fn.albums = a.cardPager(func(pg int) ([]netease.Playlist, int, error) {
			return c.AlbumsPage(fnmusic.AlbumsPath, sort, "", pg, fnCardsPage)
		})
	}
	return a.fn.albums
}

func (a *app) fnArtists() *pager[netease.Playlist] {
	if a.fn.artists == nil && a.fn.client != nil {
		c, sort := a.fn.client, a.fn.artistSort
		a.fn.artists = a.cardPager(func(pg int) ([]netease.Playlist, int, error) {
			return c.ArtistsPage(fnmusic.ArtistsPath, sort, "", pg, fnCardsPage)
		})
	}
	return a.fn.artists
}

func (a *app) cardPager(fetch func(page int) ([]netease.Playlist, int, error)) *pager[netease.Playlist] {
	p := newPager(fnCardsPage, 1, a.fnPost, fetch)
	p.landed = func(_ int, items []netease.Playlist) {
		if a.fn.meta == nil {
			a.fn.meta = map[int64]netease.Playlist{}
		}
		for _, pl := range items {
			a.fn.meta[pl.ID] = pl
		}
	}
	return p
}

// setFnAlbumSort and setFnArtistSort list the albums or the artists in
// another order, from the start.
func (a *app) setFnAlbumSort(sort string) {
	if a.fn.albumSort != sort {
		a.fn.albumSort, a.fn.albums = sort, nil
		a.scrollReset = true
	}
}

func (a *app) setFnArtistSort(sort string) {
	if a.fn.artistSort != sort {
		a.fn.artistSort, a.fn.artists = sort, nil
		a.scrollReset = true
	}
}

// forgetFnLists drops every list read of the service: it is another, or
// none.
func (a *app) forgetFnLists() {
	a.fn.lists, a.fn.meta, a.fn.search = nil, nil, nil
	a.fn.albums, a.fn.artists, a.fn.searchAlbums, a.fn.searchArtists = nil, nil, nil, nil
}
