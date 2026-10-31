package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// searchResults and searchSuggest search NetEase; tests replace them.
var (
	searchResults = netease.SearchPage
	searchSuggest = netease.SearchSuggest
)

// searchLimit is how many results load at a time.
const searchLimit = 60

// searchKinds are the kinds of search, as their tabs show them.
var searchKinds = []struct {
	kind        netease.SearchType
	label, unit string
}{
	{netease.SearchSongs, "单曲", "首单曲"},
	{netease.SearchArtists, "歌手", "位歌手"},
	{netease.SearchAlbums, "专辑", "张专辑"},
	{netease.SearchPlaylists, "歌单", "个歌单"},
	{netease.SearchUsers, "用户", "位用户"},
	{netease.SearchPodcasts, "播客", "个播客"},
}

// searchData is what a search found, as far as its pages have loaded.
type searchData struct {
	res  netease.SearchResult
	page paged
}

// search searches for q, of the kind last searched for.
func (a *app) search(q string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return
	}
	kind := a.searched.kind
	if kind == 0 {
		kind = netease.SearchSongs
	}
	a.searchFor(route{page: pageSearch, query: q, kind: kind})
}

// searchFor opens the page of a search, remembering its query. Searched
// for again, it loads anew.
func (a *app) searchFor(r route) {
	a.rememberSearch(r.query)
	a.dismissSuggestions()
	if r == a.route {
		a.nowPlaying = false
		a.loadSearch(r, true)
		return
	}
	a.open(r)
}

// loadSearch loads the first page of a search, unless it has, or again.
func (a *app) loadSearch(r route, again bool) {
	if a.searches[r] != nil && !again {
		return
	}
	d := &searchData{}
	a.searches[r] = d
	a.moreResults(r, d)
}

// moreResults loads the next page of a search. A search done again has
// new data, so the last one's pages land where nothing shows them.
func (a *app) moreResults(r route, d *searchData) {
	if d.page.loading {
		return
	}
	d.page.loading, d.page.err = true, ""
	offset := d.res.Len()
	go func() {
		res, err := searchResults(r.query, r.kind, searchLimit, offset)
		a.post(func() {
			d.page.loading = false
			if err != nil {
				d.page.err = "搜索失败，请稍后再试"
				return
			}
			d.res.Append(res)
			// A server can claim more while sending none.
			d.page.loaded, d.page.more = true, res.Len() > 0 && d.res.Len() < d.res.Total
		})
	}()
}

// searchHistoryLimit is how many searches the history keeps.
const searchHistoryLimit = 20

// rememberSearch puts q first in the history of searches, once.
func (a *app) rememberSearch(q string) {
	history := []string{q}
	for _, s := range a.settings.SearchHistory {
		if s != q && len(history) < searchHistoryLimit {
			history = append(history, s)
		}
	}
	a.settings.SearchHistory = history
	a.settings.save()
}

func (a *app) clearSearchHistory() {
	a.settings.SearchHistory = nil
	a.settings.save()
}

func (a *app) searchPage(c *ui.Context) {
	r := a.route
	d := a.searches[r]
	header := func(c *ui.Context) { a.searchHeader(c, d) }
	more := func() { a.moreResults(r, d) }
	if r.kind == netease.SearchSongs {
		if !d.page.loaded && d.page.loading {
			a.trackSkeleton(c, a.pageShimmer(c), true, header)
			return
		}
		a.trackRows(c, d.res.Songs, true, header, func(c *ui.Context) {
			a.pageFoot(c, &d.page, more)
			a.searchEmpty(c, d)
		})
		return
	}
	a.scrollPage(c, func() {
		header(c)
		res := &d.res
		switch r.kind {
		case netease.SearchArtists:
			ui.Grid(c).Columns(a.columns(c, 132)).GapX(20).GapY(28).Children(func() {
				for i := range res.Artists {
					a.artistCard(c.Key(res.Artists[i].ID), &res.Artists[i])
				}
			})
		case netease.SearchAlbums:
			a.albumGrid(c, res.Albums)
		case netease.SearchPlaylists:
			ui.Grid(c).Columns(a.columns(c, 156)).GapX(20).GapY(28).Children(func() {
				for i := range res.Playlists {
					a.foundPlaylistCard(c.Key(res.Playlists[i].ID), &res.Playlists[i])
				}
			})
		case netease.SearchUsers:
			for i := range res.Users {
				a.userRow(c.Key(res.Users[i].UserID), &res.Users[i])
			}
		case netease.SearchPodcasts:
			for i := range res.Podcasts {
				a.podcastRow(c.Key(res.Podcasts[i].ID), &res.Podcasts[i])
			}
		}
		if !d.page.loaded && d.page.loading {
			a.playlistCardsSkeleton(c, a.pageShimmer(c), a.columns(c, 156), 156, false)
			return
		}
		a.pageFoot(c, &d.page, more)
		a.searchEmpty(c, d)
	})
}

func (a *app) searchEmpty(c *ui.Context, d *searchData) {
	if d.page.loaded && d.res.Len() == 0 {
		a.empty(c, "没有找到相关结果")
	}
}

// searchHeader is what was searched for, how many it found, and the tabs
// of what it may look for instead.
func (a *app) searchHeader(c *ui.Context, d *searchData) {
	r := a.route
	sub := "搜索"
	at := 0
	for i, k := range searchKinds {
		if k.kind == r.kind {
			at = i
			if d.page.loaded {
				sub = fmt.Sprintf("搜索 · 找到 %d %s", d.res.Total, k.unit)
			}
		}
	}
	a.heading(c, "“"+r.query+"”", sub)
	ui.Row(c).Margin(0, 0, 18, 0).Children(func() {
		labels := make([]string, len(searchKinds))
		for i, k := range searchKinds {
			labels[i] = k.label
		}
		if segmented(c.Key("kind"), &at, labels...) {
			r.kind = searchKinds[at].kind
			a.show(r)
		}
	})
}

// artistCard is an artist's round photo over their name, opening their page.
func (a *app) artistCard(c *ui.Context, ar *netease.ArtistProfile) {
	p := a.pal
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).FocusRing(false).Label(ar.Name)
	hovered := card.Hovered()
	card.Children(func() {
		photo := ar.Avatar
		if photo == "" {
			photo = ar.PicURL
		}
		art := ui.Box(c).AspectRatio(1).Radius(999)
		if hovered {
			art.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			art.Shadow(0, 2, 8, -2, p.cardShade)
		}
		art.Children(func() { a.coverFill(c, photo, 400, 999) })
		ui.Text(c, ar.Name).FontSize(13).FontWeight(500).SingleLine().AlignSelf(ui.Center)
	})
	if card.Clicked() {
		a.open(route{page: pageArtist, id: ar.ID})
	}
}

// foundPlaylistCard is a playlist a search found: its card, with how many
// songs it has and who made it under its name, as strangers' playlists
// are told apart by their makers.
func (a *app) foundPlaylistCard(c *ui.Context, pl *netease.Playlist) {
	sub := fmt.Sprintf("%d 首", pl.TrackCount)
	if pl.Creator.Nickname != "" {
		sub += " · " + pl.Creator.Nickname
	}
	if a.coverCard(c, pl.Name, pl.Cover(), pl.PlayCount, sub, func() { a.loadPlaylist(pl.ID, true) }) {
		a.open(route{page: pagePlaylist, id: pl.ID})
	}
}
