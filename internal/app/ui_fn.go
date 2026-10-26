package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/fnmusic"
	"netease-music-mygo/internal/netease"
)

// The pages and controls of the Feiniu platform, which the account menu at
// the foot of the sidebar switches to from NetEase's. Every list of songs
// or cards here is read from the service a page at a time as it is scrolled
// to: see fnpager.go.

// fnNav lists the pages of the Feiniu platform in the sidebar: the library,
// its albums and artists, the songs liked, then the playlists kept on the
// service.
func (a *app) fnNav(c *ui.Context) {
	if a.fn.client == nil {
		a.navItem(c, iconServer, "连接飞牛音乐", route{page: pageHome})
	} else {
		a.navItem(c, iconNote, "音乐库", route{page: pagePlaylist, id: fnmusic.LibraryID})
		a.navItem(c, iconAlbum, "专辑", route{page: pageFnAlbums})
		a.navItem(c, iconUser, "歌手", route{page: pageFnArtists})
		a.navItem(c, iconHeart, "我喜欢", route{page: pagePlaylist, id: fnmusic.FavoritesID})
		a.navItem(c, iconRadio, "漫游", route{page: pageFnRoam})
	}
	if a.searched.query != "" {
		a.navItem(c, iconSearch, "搜索结果", route{page: pageSearch})
	}
	a.myPlaylists(c, "歌单", a.fn.playlists, false, nil)
}

// fnContent shows the page of the Feiniu platform: its lists, or a way to
// connect it without one.
func (a *app) fnContent(c *ui.Context) {
	if a.fn.client == nil {
		a.fnConnectPage(c)
		return
	}
	switch a.route.page {
	case pageSearch:
		a.fnSearchPage(c)
	case pageFnRoam:
		a.fnRoamPage(c)
	case pageFnFoundAlbums:
		a.fnCardsPage(c, "专辑 · “"+a.searched.query+"”", a.fn.searchAlbums, &a.fn.foundAlbumList, "", "", nil, nil)
	case pageFnFoundArtists:
		a.fnCardsPage(c, "歌手 · “"+a.searched.query+"”", a.fn.searchArtists, &a.fn.foundArtistList, "", "", nil, nil)
	case pageFnAlbums:
		a.fnCardsPage(c, "专辑", a.fnAlbums(), &a.fn.albumList, a.fn.albumSort, fnmusic.AlbumsSize,
			[]sortChoice{{fnmusic.AlbumsSize, "歌曲数"}, {fnmusic.AlbumsRecent, "最近添加"}, {fnmusic.AlbumsName, "名称"}}, a.setFnAlbumSort)
	case pageFnArtists:
		a.fnCardsPage(c, "歌手", a.fnArtists(), &a.fn.artistList, a.fn.artistSort, fnmusic.ArtistsSize,
			[]sortChoice{{fnmusic.ArtistsSize, "歌曲数"}, {fnmusic.ArtistsAlbums, "专辑数"}, {fnmusic.ArtistsName, "名称"}}, a.setFnArtistSort)
	default:
		a.fnListPage(c)
	}
}

// sortChoice is an order a list can be in: the value the service takes, and
// how the listener reads it.
type sortChoice struct{ sort, label string }

// fnListPage shows a list of songs of the service: its header over rows
// that are read as they are scrolled to.
func (a *app) fnListPage(c *ui.Context) {
	l := a.fnListFor(a.route.id)
	if l == nil {
		a.centered(c, "找不到这个列表", nil)
		return
	}
	a.fnSongs(c, l, true, func(c *ui.Context) { a.fnHeader(c, l) })
}

// fnSearchPage shows what the service finds for the search in sections: the
// artists, then the albums, each a row of cards that opens in full, then the
// songs, which are read as they are scrolled to.
func (a *app) fnSearchPage(c *ui.Context) {
	l := a.fn.search
	if l == nil {
		a.centered(c, "没有找到相关内容", nil)
		return
	}
	songs, albums, artists := l.pg, a.fn.searchAlbums, a.fn.searchArtists
	// Each of the three is asked for, so that they are read together.
	songs.at(0)
	albums.at(0)
	artists.at(0)
	if songs.total < 0 && !songs.failedAt(0) {
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, true, func(c *ui.Context) { a.heading(c, "“"+l.name+"”", "搜索") })
		return
	}
	if songs.failedAt(0) {
		a.centered(c, fnError(fmt.Errorf("%s", songs.err)), songs.retry)
		return
	}
	if songs.len() == 0 && albums.len() == 0 && artists.len() == 0 && albums.total >= 0 && artists.total >= 0 {
		top, bottom := a.insets(c)
		ui.Column(c).Grow(1).Padding(top+pageGap, pagePad, bottom, pagePad).Children(func() {
			a.heading(c, "“"+l.name+"”", "搜索")
			a.centered(c, "没有找到相关内容", nil)
		})
		return
	}
	a.listTracks(c, songs.len(),
		func(i int) *netease.Track { return songs.at(i) },
		func(i int) { a.playFn(l, i) },
		func(c *ui.Context, i int) { a.fnPendingRow(c, songs, i) },
		true, func(c *ui.Context) {
			a.heading(c, "“"+l.name+"”", "搜索")
			a.cardSection(c, "歌手", artists, pageFnFoundArtists)
			a.cardSection(c, "专辑", albums, pageFnFoundAlbums)
			if songs.len() > 0 {
				a.section(c, "歌曲")
			}
		})
}

// cardWidth returns how many cards fit across a page, and how wide each is.
func (a *app) cardWidth(c *ui.Context) (int, float32) {
	const gap = 20
	w, _ := c.Size()
	cols := a.columns(c, 168)
	return cols, (w - sidebarWidth - 2*pagePad - gap*float32(cols-1)) / float32(cols)
}

// cardSection is a row of the first cards of p under a title, with a button
// to see them all if there are more than the row holds. It shows nothing
// until p is in, or if it has none.
func (a *app) cardSection(c *ui.Context, title string, p *pager[netease.Playlist], all pageKind) {
	if p == nil || p.len() == 0 {
		return
	}
	cols, card := a.cardWidth(c)
	ui.Row(c.Key(title)).AlignItems(ui.Center).Margin(34, 0, 14, 0).Children(func() {
		ui.Text(c, title).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Grow(1)
		if p.len() > cols {
			b := ui.ButtonBase(c.Key("all")).PaddingX(8).Height(26).Radius(7).AlignItems(ui.Center).FocusRing(false).
				Label("查看全部" + title)
			b.Background(b.AnimateColor("bg", map[bool]ui.Color{true: a.pal.hover, false: ui.Transparent}[b.Hovered()], ui.Animation{Duration: 120 * time.Millisecond}))
			b.Children(func() {
				ui.Text(c, fmt.Sprintf("全部 %d", p.len())).FontSize(12.5).FontWeight(500).TextColor(a.pal.muted)
			})
			if b.Clicked() {
				a.open(route{page: all})
			}
		}
	})
	ui.Row(c.Key(title + "-cards")).Gap(20).AlignItems(ui.Start).Children(func() {
		for k := 0; k < min(cols, p.len()); k++ {
			if pl := p.at(k); pl != nil {
				ui.Box(c.Key(k)).Width(card).Children(func() { a.playlistCard(c, pl) })
			}
		}
	})
}

// fnRoamPage is the page of the service's roam: its own random play.
func (a *app) fnRoamPage(c *ui.Context) {
	p := a.pal
	top, bottom := a.insets(c)
	ui.Column(c).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).Children(func() {
		a.heading(c, "漫游", "飞牛音乐")
		on := a.pl.Radio() == fnRoamRadio
		ui.Text(c, "由 NAS 为你随机挑选曲库里的歌，一首接一首，不用整库加载。").FontSize(14).TextColor(p.muted).LineHeight(1.5)
		if tr := a.pl.Current(); on && tr != nil {
			ui.Text(c, "正在播放："+tr.Name+" · "+tr.ArtistNames()).FontSize(14).FontWeight(500).Margin(18, 0, 0, 0)
		}
		ui.Row(c).Margin(22, 0, 0, 0).Children(func() {
			switch {
			case on:
				if pillButton(c.Key("roam"), iconX, "退出漫游", p.hover.Over(p.bg), p.text).Clicked() {
					a.exitRadio()
				}
			default:
				label := "开始漫游"
				if a.radioStarting == fnRoamRadio {
					label = "正在开启…"
				}
				if pillButton(c.Key("roam"), iconRadio, label, p.accent, p.onAccent).Clicked() {
					a.startRoam()
				}
			}
		})
	})
}

// fnSongs lists the songs of l. Before the first page is in, it is a
// skeleton; if that failed, a message with a way to try again.
func (a *app) fnSongs(c *ui.Context, l *fnList, headerCovers bool, header func(c *ui.Context)) {
	p := l.pg
	if p.at(0) == nil && p.total < 0 {
		if p.failedAt(0) {
			a.centered(c, fnError(fmt.Errorf("%s", p.err)), p.retry)
			return
		}
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, true, func(c *ui.Context) { a.playlistHeaderSkeleton(c, s) })
		return
	}
	if p.len() == 0 {
		top, bottom := a.insets(c)
		ui.Column(c).Grow(1).Padding(top+pageGap, pagePad, bottom, pagePad).Children(func() {
			header(c)
			a.centered(c, "这里还没有歌曲", nil)
		})
		return
	}
	a.listTracks(c, p.len(),
		func(i int) *netease.Track { return p.at(i) },
		func(i int) { a.playFn(l, i) },
		func(c *ui.Context, i int) { a.fnPendingRow(c, p, i) },
		true, header)
}

// fnPendingRow is the row of a song not in yet: blocks, shimmering, or if
// its page failed, a message that a click tries again.
func (a *app) fnPendingRow(c *ui.Context, p *pager[netease.Track], i int) {
	if p.failedAt(i) {
		b := ui.ButtonBase(c.Key(i)).Height(54).PaddingX(12).Radius(10).AlignItems(ui.Center).FocusRing(false).Label("重试")
		b.Children(func() {
			ui.Text(c, "加载失败，点击重试").FontSize(13).TextColor(a.pal.muted)
		})
		if b.Clicked() {
			p.retry()
		}
		return
	}
	s := a.pageShimmer(c)
	wide := func() bool { w, _ := c.Size(); return w-sidebarWidth > 820 }()
	ui.Row(c.Key(i)).Height(54).Shrink(0).PaddingX(12).Gap(14).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(26).Center().Children(func() { s.text(c, 13, 1.3).Width(12) })
		s.bone(c, 6).Size(38, 38)
		ui.Column(c).Grow(3).Basis(0).MinWidth(0).Gap(3).Children(func() {
			s.text(c, 14, 1.3).WidthPercent(boneWidth(i) * 0.8)
			s.text(c, 12.5, 1.3).WidthPercent(boneWidth(i+5) * 0.55)
		})
		if wide {
			ui.Box(c).Grow(2).Basis(0).MinWidth(0).Children(func() {
				s.text(c, 12.5, 1.3).WidthPercent(boneWidth(i+3) * 0.8)
			})
		}
		ui.Box(c).Size(32, 32)
		ui.Row(c).Width(44).Justify(ui.End).Children(func() { s.text(c, 12.5, 1.3).Width(30) })
	})
}

// fnHeader is the header of a list of songs: its cover, what it is, its
// name, how many songs it has, and the buttons that play it.
func (a *app) fnHeader(c *ui.Context, l *fnList) {
	p := a.pal
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		ui.Box(c).Size(212, 212).Radius(14).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFill(c, l.Cover(), 600, 14)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, l.kind).FontSize(12).FontWeight(600).TextColor(p.accent)
			ui.Text(c, l.name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			var meta []string
			if l.sub != "" {
				meta = append(meta, l.sub)
			}
			if l.pg.total >= 0 {
				meta = append(meta, fmt.Sprintf("%d 首", l.pg.total))
			}
			if len(meta) > 0 {
				ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(p.muted)
			}
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
					a.playAllFn(l)
				}
				label := "随机播放"
				if l.shuffle {
					label = "正在准备…"
				}
				if pillButton(c.Key("shuffle"), iconShuffle, label, p.hover.Over(p.bg), p.text).Clicked() {
					a.shuffleFn(l)
				}
				more := iconButton(c.Key("more"), iconMore, 18, p.muted, "更多")
				more.Menu(func(m *ui.Menu) {
					if l.id == fnmusic.LibraryID {
						for _, o := range []sortChoice{{fnmusic.SortRecent, "按最近添加排序"}, {fnmusic.SortTitle, "按歌名排序"}} {
							if m.Item(o.label).Chosen() {
								a.setFnLibSort(o.sort)
							}
						}
						m.Separator()
					}
					if m.Item("刷新").Chosen() {
						a.refreshFn(l.id)
					}
				})
			})
		})
	})
}

// fnCardsPage shows the albums or the artists as cards, as many across as
// fit, read as they are scrolled to: a library has thousands. The order
// they come in is the listener's to choose.
func (a *app) fnCardsPage(c *ui.Context, title string, p *pager[netease.Playlist], state *ui.ListState, sort, def string, choices []sortChoice, choose func(string)) {
	top, bottom := a.insets(c)
	if p == nil {
		a.centered(c, "还没有连接飞牛音乐", nil)
		return
	}
	if sort == "" {
		sort = def
	}
	if p.at(0) == nil && p.total < 0 {
		if p.failedAt(0) {
			a.centered(c, fnError(fmt.Errorf("%s", p.err)), p.retry)
			return
		}
		s := a.pageShimmer(c)
		ui.Scroll(c.Key("fn-cards-loading")).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).Children(func() {
			a.heading(c, title, "飞牛音乐")
			a.playlistCardsSkeleton(c, s, a.columns(c, 168)*3, 168, false)
		})
		return
	}
	const gap = 20
	cols, card := a.cardWidth(c)
	total := p.len()
	rows := (total + cols - 1) / cols
	a.underTop, a.underBottom = true, true
	ui.List(c.Key(a.route), state, rows+1, func(i int) {
		if i == 0 {
			ui.Column(c).Children(func() {
				a.startMark(c)
				a.heading(c, title, fmt.Sprintf("飞牛音乐 · %d", total))
				if len(choices) > 0 {
					a.sortBar(c, sort, choices, choose)
				}
			})
			return
		}
		if i == rows {
			defer a.endMark(c)
		}
		ui.Row(c).Gap(gap).AlignItems(ui.Start).Margin(0, 0, 28, 0).Children(func() {
			for k := (i - 1) * cols; k < min(i*cols, total); k++ {
				ui.Box(c.Key(k)).Width(card).Children(func() {
					if pl := p.at(k); pl != nil {
						a.playlistCard(c, pl)
						return
					}
					a.cardPending(c, p, k)
				})
			}
		})
	}).Grow(1).Padding(top+pageGap, pagePad, bottom+32, pagePad).ScrollbarInsets(top, 0, bottom, 0)
}

// cardPending is the card of an album or artist not in yet.
func (a *app) cardPending(c *ui.Context, p *pager[netease.Playlist], i int) {
	if p.failedAt(i) {
		b := ui.ButtonBase(c).Column().Gap(10).Center().AspectRatio(1).Radius(10).Background(a.pal.hover).FocusRing(false).Label("重试")
		b.Children(func() { ui.Text(c, "加载失败，点击重试").FontSize(12).TextColor(a.pal.muted) })
		if b.Clicked() {
			p.retry()
		}
		return
	}
	s := a.pageShimmer(c)
	ui.Column(c).Gap(10).AlignItems(ui.Stretch).Children(func() {
		s.bone(c, 10).AspectRatio(1)
		ui.Column(c).Children(func() {
			s.text(c, 13, 1.35).WidthPercent(92)
			s.text(c, 13, 1.35).WidthPercent(boneWidth(i))
		})
	})
}

// sortBar is the choice of the order a page of cards comes in.
func (a *app) sortBar(c *ui.Context, sort string, choices []sortChoice, choose func(string)) {
	labels := make([]string, len(choices))
	at := 0
	for i, ch := range choices {
		labels[i] = ch.label
		if ch.sort == sort {
			at = i
		}
	}
	ui.Row(c).Margin(-6, 0, 18, 0).AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Text(c, "排序").FontSize(12).TextColor(a.pal.muted)
		if segmented(c.Key("sort"), &at, labels...) {
			choose(choices[at].sort)
		}
	})
}

// fnConnectPage asks the listener to connect their Feiniu music service,
// where there is nothing to show before.
func (a *app) fnConnectPage(c *ui.Context) {
	p := a.pal
	top, bottom := a.insets(c)
	ui.Column(c).Grow(1).Padding(top, pagePad, bottom, pagePad).Center().Gap(10).Children(func() {
		ui.Box(c).Size(64, 64).Radius(18).Background(p.hover).Center().Children(func() {
			ui.Icon(c, iconServer).Size(28, 28).TextColor(p.muted)
		})
		ui.Text(c, "连接飞牛音乐").FontSize(20).FontWeight(700).Margin(8, 0, 0, 0)
		ui.Text(c, "播放存放在飞牛 NAS 上的音乐库、歌单和你喜欢的歌曲").FontSize(13).TextColor(p.muted)
		ui.Text(c, "支持 FN ID（FN Connect）与局域网地址；MP3、FLAC 直接播放，其他格式由 NAS 转码").FontSize(12).TextColor(p.faint)
		if a.fn.err != "" {
			ui.Text(c, a.fn.err).FontSize(12.5).TextColor(p.accent)
		}
		ui.Box(c).Height(6)
		if pillButton(c.Key("connect"), iconServer, "连接", p.accent, p.onAccent).Clicked() {
			a.openFnConnect()
		}
	})
}

// fnDialog asks for where the Feiniu music service is, and who is
// listening: the account of the service on the NAS, not an fnOS one.
func (a *app) fnDialog(c *ui.Context) {
	p := a.pal
	f := &a.fn.form
	ui.DialogBase(c.Key("fn-connect"), &f.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(380).Padding(24).Radius(18).Gap(14).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Text(c, "连接飞牛音乐").FontSize(17).FontWeight(700)
		sub := "输入 FN ID，在家里或外出都能连接；账号是 NAS 上飞牛音乐的账号"
		if f.byAddress {
			sub = "填写飞牛 NAS 上音乐应用的地址与账号"
		}
		ui.Text(c, sub).FontSize(12.5).TextColor(p.muted).LineHeight(1.4).Margin(-8, 0, 0, 0)
		mode := 0
		if f.byAddress {
			mode = 1
		}
		if segmentedIn(c.Key("mode"), &mode, 332, "FN ID", "地址") {
			f.byAddress, f.err = mode == 1, ""
		}
		submit := false
		field := func(key, label, hint string, value *string, secret bool) {
			ui.Column(c.Key(key)).Gap(5).Children(func() {
				ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(p.muted)
				in := ui.TextInput(c.Key("in"), value).Placeholder(hint).Label(label).FontSize(14)
				if secret {
					in.Password()
				}
				if key == "address" {
					in.AutoFocus()
				}
				if in.Submitted() {
					submit = true
				}
			})
		}
		if f.byAddress {
			field("address", "服务器地址", "例如 192.168.1.10:5666", &f.address, false)
		} else {
			field("address", "FN ID", "例如 mynas01", &f.address, false)
		}
		field("user", "用户名", "", &f.user, false)
		field("passwd", "密码", "", &f.passwd, true)
		if !f.byAddress {
			field("code", "FN Connect 访问码（可选）", "NAS 开启了访问码时填写", &f.code, true)
		}
		if f.err != "" {
			ui.Text(c, f.err).FontSize(12.5).TextColor(p.accent)
		}
		ui.Row(c).Gap(10).Justify(ui.End).Margin(4, 0, 0, 0).Children(func() {
			if pillButton(c.Key("cancel"), nil, "取消", p.hover.Over(p.bg), p.text).Clicked() {
				f.open = false
			}
			action := "连接"
			if f.busy {
				action = "正在连接…"
			}
			ok := pillButton(c.Key("ok"), nil, action, p.accent, p.onAccent)
			if f.busy {
				ok.Disabled(true).Opacity(0.5)
			}
			if ok.Clicked() {
				submit = true
			}
		})
		if submit {
			a.submitFn()
		}
	})
}
