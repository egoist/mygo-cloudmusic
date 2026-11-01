package fnmusic

import (
	"fmt"
	backend "netease-music-mygo/internal/platform/fnmusic"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (h *Home) CloseTransient() {
	if h == nil {
		return
	}
	h.accountOpen = false
	h.connectionGen++
	h.connectionOpen, h.connectionBusy, h.connectionError = false, false, ""
	h.nameOpen, h.deleteOpen = false, false
}

func (h *Home) Sidebar(c *ui.Context) {
	if h == nil || h.host == nil || h.state == nil {
		return
	}
	h.host.PlatformSearchField(c, "搜索歌曲、歌手、播客", func(query string) {
		if query = strings.TrimSpace(query); query != "" {
			h.Open(Route{Page: PageSearch, Query: query, Kind: "songs"})
			h.host.Navigate(h.route)
		}
	})
	ui.Scroll(c.Key("fnmusic-nav")).TrackScroll(&h.navScroll).Grow(1).Margin(0, -6).Padding(0, 6).Gap(4).Children(func() {
		if h.state.Connecting() {
			h.sidebarLoading(c)
		} else if !h.state.Connected() {
			h.host.NavItem(c, iconServer, "连接飞牛音乐", Route{Page: PageLibrary})
		} else {
			h.host.NavItem(c, iconNote, "音乐库", Route{Page: PageLibrary})
			h.host.NavItem(c, iconAlbum, "专辑", Route{Page: PageAlbums})
			h.host.NavItem(c, iconUser, "歌手", Route{Page: PageArtists})
			h.host.NavItem(c, iconHeart, "我喜欢", Route{Page: PagePlaylist, ID: backend.FavoritesID})
			h.host.NavItem(c, iconClock, "最近播放", Route{Page: PagePlaylist, ID: backend.RecentID})
			h.host.NavItem(c, iconRadio, "漫游", Route{Page: PageRoam})
		}
		if h.route.Page == PageSearch || h.route.Page == PageSearchAlbums || h.route.Page == PageSearchArtists {
			h.host.NavItem(c, iconSearch, "搜索结果", h.route)
		}
		if h.state.Connected() {
			h.host.PlaylistHeader(c, func() { h.RequestNewPlaylist(nil) })
			for _, pl := range h.state.Playlists() {
				r := Route{Page: PagePlaylist, ID: pl.ID}
				h.host.PlaylistItem(c, fmt.Sprintf("fnmusic-playlist-%d", pl.ID), pl, h.route == r, func(m *ui.Menu) { h.playlistMenu(m, pl) }, func() {
					h.host.Navigate(r)
				})
			}
		}
	})
	if player := h.host.Player(); player != nil {
		if tr := player.Current(); tr != nil {
			h.host.NowPlaying(c, tr)
		}
	}
	cfg := toConnectionConfig(h.state.Config())
	username, status := cfg.Username, "未连接"
	if h.state.Connecting() {
		if username == "" {
			username = "飞牛音乐"
		}
		status = "正在连接…"
	}
	if username == "" {
		username = "飞牛音乐（未连接）"
	}
	connected := h.state.Connected()
	if connected {
		if username == "飞牛音乐（未连接）" {
			username = "飞牛音乐"
		}
		status = "飞牛音乐"
		if err := h.state.Error(); err != nil {
			status = "连接失败"
		}
	} else if err := h.state.Error(); err != nil {
		status = "连接失败"
	}
	actions := h.accountActions()
	h.host.AccountRow(c, iconServer, username, status, &h.accountOpen, actions)
	h.host.PlatformSwitcher(c)
}

// sidebarLoading keeps the configured platform layout stable while the first
// connection and playlist request are in flight. Showing the connect CTA here
// made the sidebar flash to a different navigation tree on every launch.
func (h *Home) sidebarLoading(c *ui.Context) {
	t := c.Theme()
	for i := 0; i < 5; i++ {
		ui.Row(c.Key(fmt.Sprintf("fnmusic-nav-loading-%d", i))).Height(34).PaddingX(10).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(17, 17).Radius(5).Background(t.TextMuted.Alpha(0.16))
			ui.Box(c).Height(12).Width(72 + float32(i%3)*20).Radius(6).Background(t.TextMuted.Alpha(0.16))
		})
	}
}
