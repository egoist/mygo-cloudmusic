package fnmusic

import (
	"fmt"
	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/fnmusic"
	"netease-music-mygo/internal/platform/paging"

	"github.com/egoist/mygo/ui"
)

// cardsHeader is the compact header used by album and artist collections.
// Detail lists use listHeader, which has the three-line playlist layout.
func (h *Home) cardsHeader(c *ui.Context, title string, total int) {
	t := c.Theme()
	ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
		ui.Text(c, title).FontSize(30).FontWeight(700).LetterSpacing(-0.6)
		if total >= 0 {
			ui.Text(c, fmt.Sprintf("共 %d 个", total)).FontSize(13).TextColor(t.TextMuted)
		}
	})
}

func (h *Home) openCard(id int64) {
	if h == nil || h.state == nil || h.host == nil {
		return
	}
	h.host.Navigate(h.cardRoute(id))
}

func (h *Home) cardRoute(id int64) Route {
	r := Route{Page: PagePlaylist, ID: id}
	c := h.state.Catalog()
	if c == nil {
		return r
	}
	if meta := c.Meta[id]; meta.ID != 0 {
		r.Kind, r.Query = meta.Kind, meta.Ref
	} else {
		// Search result cards may be opened before the pager's landed callback
		// updates Meta; inspect the already loaded card pages directly.
		for _, p := range []*paging.Pager[netease.Playlist]{c.SearchAlbums, c.SearchArtists, c.Albums, c.Artists} {
			if p == nil {
				continue
			}
			if i := p.IndexOf(func(pl *netease.Playlist) bool { return pl != nil && pl.ID == id }); i >= 0 {
				if pl := p.Peek(i); pl != nil {
					r.Kind, r.Query = pl.Kind, pl.Ref
					break
				}
			}
		}
	}
	return r
}

// cardRouteFrom keeps the provider identity that was rendered with a card.
// Resolving a card by ID alone is unsafe while a page is still landing: the
// catalog metadata may not have been published yet, and an album/artist can
// otherwise fall back to the playlist endpoint.
func (h *Home) cardRouteFrom(p *paging.Pager[netease.Playlist], id int64) Route {
	if p != nil {
		if i := p.IndexOf(func(pl *netease.Playlist) bool { return pl != nil && pl.ID == id }); i >= 0 {
			if pl := p.Peek(i); pl != nil {
				return Route{Page: PagePlaylist, ID: pl.ID, Kind: pl.Kind, Query: pl.Ref}
			}
		}
	}
	return h.cardRoute(id)
}

func (h *Home) openCardFrom(p *paging.Pager[netease.Playlist], id int64) {
	h.host.Navigate(h.cardRouteFrom(p, id))
}

func (h *Home) playCardFrom(p *paging.Pager[netease.Playlist], id int64) {
	if h == nil || h.state == nil || h.host == nil {
		return
	}
	c := h.state.Catalog()
	if c == nil {
		return
	}
	r := h.cardRouteFrom(p, id)
	l := c.ListForRoute(id, r.Kind, r.Query)
	if l == nil || l.Pages == nil {
		return
	}
	gen := h.state.Generation()
	l.Pages.When(0, func(ok bool) {
		if ok && h.state.Generation() == gen {
			PlayList(h.host.Player(), l, 0)
		}
	})
}

func (h *Home) playCard(id int64) { h.playCardFrom(nil, id) }

func (h *Home) cardsPage(c *ui.Context) {
	switch h.route.Page {
	case PageSearch:
		catalog := h.state.Catalog()
		var songs *appplatform.TrackList
		if catalog.SearchList != nil {
			songs = catalog.SearchList.View()
		}
		h.host.RenderSearch(c, appplatform.SearchResults{Query: h.route.Query, Songs: songs, Albums: catalog.SearchAlbums, Artists: catalog.SearchArtists}, SearchView{Play: func(i int) {
			if catalog.SearchList != nil {
				PlayList(h.host.Player(), catalog.SearchList, i)
			}
		}, PlayCard: func(id int64) { h.playCardFrom(nil, id) }, TrackMenu: h.trackMenu, Liked: h.state.Liked, Favorite: h.RequestFavorite, OpenList: h.openCard, OpenCards: func(kind string) {
			page := PageSearchArtists
			if kind == "albums" {
				page = PageSearchAlbums
			}
			h.host.Navigate(Route{Page: page, Query: h.route.Query})
		}})
	case PageSearchAlbums:
		cards := h.state.Catalog().SearchAlbums
		if cards == nil {
			h.host.RenderCards(c, nil, CardView{Title: "专辑 · “" + h.route.Query + "”"})
			break
		}
		h.host.RenderCards(c, cards, CardView{Title: "专辑 · “" + h.route.Query + "”", Count: cards.Total(), Header: func(c *ui.Context) { h.cardsHeader(c, "专辑 · “"+h.route.Query+"”", cards.Total()) }, OnOpen: func(id int64) { h.openCardFrom(cards, id) }, OnPlay: func(id int64) { h.playCardFrom(cards, id) }})
	case PageSearchArtists:
		cards := h.state.Catalog().SearchArtists
		if cards == nil {
			h.host.RenderCards(c, nil, CardView{Title: "歌手 · “" + h.route.Query + "”"})
			break
		}
		h.host.RenderCards(c, cards, CardView{Title: "歌手 · “" + h.route.Query + "”", Count: cards.Total(), Header: func(c *ui.Context) { h.cardsHeader(c, "歌手 · “"+h.route.Query+"”", cards.Total()) }, OnOpen: func(id int64) { h.openCardFrom(cards, id) }, OnPlay: func(id int64) { h.playCardFrom(cards, id) }})
	case PageAlbums:
		cards := h.state.Catalog().AlbumsList()
		if cards == nil {
			h.host.RenderCards(c, nil, CardView{Title: "专辑"})
			break
		}
		h.host.RenderCards(c, cards, CardView{Title: "专辑", Count: cards.Total(), Header: func(c *ui.Context) { h.cardsHeader(c, "专辑", cards.Total()) }, Sort: []SortChoice{{Value: backend.AlbumsSize, Label: "歌曲数"}, {Value: backend.AlbumsRecent, Label: "最近添加"}, {Value: backend.AlbumsName, Label: "名称"}}, Current: h.state.Catalog().AlbumSort, OnSort: func(v string) { h.state.Catalog().AlbumSort = v; cards.Reload(); h.host.Invalidate() }, OnOpen: func(id int64) { h.openCardFrom(cards, id) }, OnPlay: func(id int64) { h.playCardFrom(cards, id) }})
	case PageArtists:
		cards := h.state.Catalog().ArtistsList()
		if cards == nil {
			h.host.RenderCards(c, nil, CardView{Title: "歌手"})
			break
		}
		h.host.RenderCards(c, cards, CardView{Title: "歌手", Count: cards.Total(), Header: func(c *ui.Context) { h.cardsHeader(c, "歌手", cards.Total()) }, Sort: []SortChoice{{Value: backend.ArtistsSize, Label: "歌曲数"}, {Value: backend.ArtistsAlbums, Label: "专辑数"}, {Value: backend.ArtistsName, Label: "名称"}}, Current: h.state.Catalog().ArtistSort, OnSort: func(v string) { h.state.Catalog().ArtistSort = v; cards.Reload(); h.host.Invalidate() }, OnOpen: func(id int64) { h.openCardFrom(cards, id) }, OnPlay: func(id int64) { h.playCardFrom(cards, id) }})
	}
}
