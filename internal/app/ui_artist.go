package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// artistLinks shows a track's artists as its credits read, each a link to
// their page in color, brightening to hover under the pointer, and then,
// with album, a link to its album. An artist NetEase has no page for,
// credited by name alone, is plain text. An episode's host and podcast,
// in its artists' and album's place, link to the podcast's page.
func (a *app) artistLinks(c *ui.Context, tr *netease.Track, color, hover ui.Color, album ...bool) ui.Element {
	podcast := route{page: pagePodcast, id: tr.PodcastID}
	return ui.RichText(c).TextColor(color).Children(func() {
		for i, ar := range tr.Artists {
			if i > 0 {
				ui.Text(c, ", ")
			}
			switch {
			case tr.PodcastID != 0:
				a.link(c.Key(i), ar.Name, color, hover, podcast)
			case ar.ID != 0:
				a.link(c.Key(i), ar.Name, color, hover, route{page: pageArtist, id: ar.ID})
			default:
				ui.Text(c, ar.Name)
			}
		}
		if len(album) > 0 && album[0] && tr.Album.Name != "" {
			ui.Text(c, " — ")
			switch {
			case tr.PodcastID != 0:
				a.link(c.Key("album"), tr.Album.Name, color, hover, podcast)
			case tr.Album.ID != 0:
				a.link(c.Key("album"), tr.Album.Name, color, hover, route{page: pageAlbum, id: tr.Album.ID})
			default:
				ui.Text(c, tr.Album.Name)
			}
		}
	})
}

// link is text in a RichText that opens a page.
func (a *app) link(c *ui.Context, text string, color, hover ui.Color, r route) {
	link := ui.Link(c, text, "").TextColor(color)
	if link.Hovered() {
		link.TextColor(hover)
	}
	// Once built: a song's row is in a list, which a new page resets.
	link.OnClick(func() { a.openPage(r) })
}

// openPage opens a page in the main window, from wherever a link to it
// shows: over Now Playing, which closes, or the notch.
func (a *app) openPage(r route) {
	if pr, ok := a.platformPage(r); ok {
		if home := a.activeHome(); home != nil {
			home.Open(pr)
			a.showMain()
			a.redraw()
			return
		}
	}
	a.open(r)
	a.showMain()
	a.redraw()
}

// openArtist opens an artist's page in the main window.
func (a *app) openArtist(id int64) { a.openPage(route{page: pageArtist, id: id}) }

// These load an artist, their songs, albums and events from NetEase, and
// follow them; tests replace them.
var (
	artistDetail      = netease.ArtistDetail
	artistLatestSongs = netease.ArtistLatestSongs
	albumsBy          = netease.ArtistAlbums
	accountEvents     = netease.Events
	followArtist      = netease.FollowArtist
)

// artistData is what the app has of an artist: who they are, with their
// most popular songs, and their newest songs, albums and events once
// asked for.
type artistData struct {
	profile       *netease.ArtistProfile
	loading       bool
	err           string
	latest        []netease.Track
	latestLoading bool
	latestErr     string
	albums        []netease.AlbumProfile
	albumsPage    paged
	events        []netease.Event
	eventsPage    paged
	eventsBefore  int64 // the time of the last event loaded, the next page's start
	following     bool  // a follow or unfollow is on its way
}

// What an artist's page lists: their songs, the most popular or the
// newest, their albums, or their events.
const (
	artistHot = iota
	artistLatest
	artistAlbums
	artistEvents
)

// latestCount is how many of an artist's newest songs their page lists;
// albumsPage and eventsPage how many albums and events load at a time.
const (
	latestCount = 100
	albumsPage  = 30
	eventsPage  = 20
)

func (a *app) artist(id int64) *artistData {
	d := a.artists[id]
	if d == nil {
		d = &artistData{}
		a.artists[id] = d
	}
	return d
}

// loadArtist loads an artist's page, and what it lists.
func (a *app) loadArtist(id int64) {
	d := a.artist(id)
	switch a.artistTab {
	case artistLatest:
		a.loadLatest(id)
	case artistAlbums:
		if !d.albumsPage.loaded {
			a.loadAlbums(id)
		}
	}
	if d.profile != nil || d.loading {
		if a.artistTab == artistEvents && !d.eventsPage.loaded {
			a.loadEvents(id)
		}
		return
	}
	d.loading, d.err = true, ""
	go func() {
		ar, err := artistDetail(id)
		a.post(func() {
			d.loading = false
			if err != nil {
				d.err = "歌手加载失败"
				return
			}
			d.profile = ar
			// Their events are their account's, which their profile names.
			if a.artistTab == artistEvents && a.route == (route{page: pageArtist, id: id}) {
				a.loadEvents(id)
			}
		})
	}()
}

func (a *app) loadLatest(id int64) {
	d := a.artist(id)
	if d.latest != nil || d.latestLoading {
		return
	}
	d.latestLoading, d.latestErr = true, ""
	go func() {
		songs, err := artistLatestSongs(id, latestCount)
		a.post(func() {
			d.latestLoading = false
			if err != nil {
				d.latestErr = "歌曲加载失败"
				return
			}
			d.latest = songs
		})
	}()
}

// loadAlbums loads the next page of an artist's albums.
func (a *app) loadAlbums(id int64) {
	d := a.artist(id)
	if d.albumsPage.loading {
		return
	}
	d.albumsPage.loading, d.albumsPage.err = true, ""
	offset := len(d.albums)
	go func() {
		albums, more, err := albumsBy(id, albumsPage, offset)
		a.post(func() {
			d.albumsPage.loading = false
			if err != nil {
				d.albumsPage.err = "专辑加载失败"
				return
			}
			d.albums = append(d.albums, albums...)
			d.albumsPage.loaded, d.albumsPage.more = true, more && len(albums) > 0
		})
	}()
}

// loadEvents loads the next page of an artist's events, once their profile
// says whose they are.
func (a *app) loadEvents(id int64) {
	d := a.artist(id)
	if d.profile == nil || d.profile.AccountID == 0 || d.eventsPage.loading {
		return
	}
	d.eventsPage.loading, d.eventsPage.err = true, ""
	uid, before := d.profile.AccountID, d.eventsBefore
	if !d.eventsPage.loaded {
		before = -1
	}
	go func() {
		events, last, more, err := accountEvents(uid, eventsPage, before)
		a.post(func() {
			d.eventsPage.loading = false
			if err != nil {
				d.eventsPage.err = "动态加载失败"
				return
			}
			d.events = append(d.events, events...)
			d.eventsPage.loaded = true
			// A page that starts no earlier would load forever.
			d.eventsPage.more = more && len(events) > 0 && (before == -1 || last < before)
			d.eventsBefore = last
		})
	}()
}

// toggleFollow follows an artist, or unfollows them, at once, going back if
// NetEase refuses. Signed out, it asks the listener to sign in.
func (a *app) toggleFollow(id int64) {
	d := a.artist(id)
	if d.profile == nil || d.following {
		return
	}
	if a.profile == nil {
		a.openLogin()
		return
	}
	ar := d.profile
	follow := !ar.Followed
	ar.Followed, d.following = follow, true
	go func() {
		err := followArtist(id, follow)
		a.post(func() {
			d.following = false
			switch {
			case err != nil:
				ar.Followed = !follow
				a.toast = "操作失败，请稍后再试"
			case follow:
				a.toast = "已关注"
			default:
				a.toast = "已取消关注"
			}
		})
	}()
}

// artistPage shows who an artist is over their songs, the most popular or
// the newest, their albums or their events.
func (a *app) artistPage(c *ui.Context) {
	id := a.route.id
	d := a.artist(id)
	if d.profile == nil {
		if !d.loading {
			a.centered(c, d.err, func() { a.loadArtist(id) })
			return
		}
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, true, func(c *ui.Context) { a.artistHeaderSkeleton(c, s) })
		return
	}
	header := func(c *ui.Context) { a.artistHeader(c, d) }
	switch a.artistTab {
	case artistAlbums:
		a.scrollPage(c, func() {
			header(c)
			a.albumGrid(c, d.albums)
			a.pageFoot(c, &d.albumsPage, func() { a.loadAlbums(id) })
			if d.albumsPage.loaded && len(d.albums) == 0 {
				a.empty(c, "暂无专辑")
			}
		})
		return
	case artistEvents:
		a.scrollPage(c, func() {
			header(c)
			a.artistEvents(c, d)
		})
		return
	}
	songs := d.profile.Songs
	if a.artistTab == artistLatest {
		switch {
		case d.latest != nil:
			songs = d.latest
		case d.latestLoading:
			a.trackSkeleton(c, a.pageShimmer(c), true, header)
			return
		default:
			songs = nil
			header = func(c *ui.Context) {
				a.artistHeader(c, d)
				ui.Column(c).Padding(40, 0).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, d.latestErr).TextColor(a.pal.muted).FontSize(13)
					if ui.Button(c.Key("retry"), "重试").Clicked() {
						a.loadLatest(d.profile.ID)
					}
				})
			}
		}
	}
	a.trackRows(c, songs, true, header)
}

// scrollPage scrolls a page of cards and rows, inset as one of tracks is:
// a tab of a page that lists tracks puts its header in the same place.
func (a *app) scrollPage(c *ui.Context, content func()) {
	top, bottom := a.insets(c)
	ui.Scroll(c.Key(a.route)).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		content()
	})
}

// empty says a list has nothing in it.
func (a *app) empty(c *ui.Context, msg string) {
	ui.Text(c, msg).FontSize(13).TextColor(a.pal.muted).Padding(40, 0).AlignSelf(ui.Center)
}

// artistPhotoSize is the width of an artist's round photo on their page.
const artistPhotoSize = 212

func (a *app) artistHeader(c *ui.Context, d *artistData) {
	p := a.pal
	ar := d.profile
	// What the buttons play: the songs listed, or else the most popular.
	songs := ar.Songs
	if a.artistTab == artistLatest && d.latest != nil {
		songs = d.latest
	}
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 8, 0).Children(func() {
		photo := ar.Avatar
		if photo == "" {
			photo = ar.PicURL
		}
		ui.Box(c).Size(artistPhotoSize, artistPhotoSize).Radius(artistPhotoSize/2).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFill(c, photo, 600, artistPhotoSize/2)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, "歌手").FontSize(12).FontWeight(600).TextColor(p.accent)
			ui.Text(c, ar.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			var meta []string
			if aka := distinct(append([]string{ar.Trans}, ar.Alias...)); len(aka) > 0 {
				meta = append(meta, strings.Join(aka, " / "))
			}
			if ar.MusicSize > 0 {
				meta = append(meta, fmt.Sprintf("%d 首歌曲", ar.MusicSize))
			}
			if ar.AlbumSize > 0 {
				meta = append(meta, fmt.Sprintf("%d 张专辑", ar.AlbumSize))
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(p.muted)
			if d := strings.TrimSpace(ar.BriefDesc); d != "" {
				ui.Text(c, strings.ReplaceAll(d, "\n", " ")).FontSize(12.5).TextColor(p.muted).MaxLines(2).LineHeight(1.45).Margin(2, 0, 0, 0)
			}
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
					if a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.pl.PlayQueue(songs, firstPlayable(songs))
				}
				if pillButton(c.Key("shuffle"), iconShuffle, "随机播放", p.hover.Over(p.bg), p.text).Clicked() {
					if !a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.pl.PlayQueue(songs, firstPlayable(songs))
				}
				icon, label := iconPlus, "关注"
				if ar.Followed {
					icon, label = iconCheck, "已关注"
				}
				if pillButton(c.Key("follow"), icon, label, p.hover.Over(p.bg), p.text).Clicked() {
					a.toggleFollow(ar.ID)
				}
			})
		})
	})
	ui.Row(c).AlignItems(ui.Center).Margin(34, 0, 14, 0).Children(func() {
		ui.Text(c, artistTabTitles[a.artistTab]).FontSize(19).FontWeight(700).LetterSpacing(-0.3)
		ui.Spacer(c)
		if segmented(c.Key("tab"), &a.artistTab, "热门", "最新", "专辑", "动态") {
			a.loadArtist(ar.ID)
		}
	})
}

var artistTabTitles = [...]string{"歌曲", "歌曲", "专辑", "动态"}

func (a *app) artistHeaderSkeleton(c *ui.Context, s shimmer) {
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 8, 0).Children(func() {
		s.bone(c, artistPhotoSize/2).Size(artistPhotoSize, artistPhotoSize)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			s.text(c, 12, 1.3).Width(24)
			s.text(c, 30, 1.2).WidthPercent(32)
			s.text(c, 13, 1.3).Width(160)
			ui.Column(c).Margin(2, 0, 0, 0).Children(func() {
				s.text(c, 12.5, 1.45).WidthPercent(78)
				s.text(c, 12.5, 1.45).WidthPercent(52)
			})
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				s.bone(c, 18).Size(92, 36)
				s.bone(c, 18).Size(120, 36)
			})
		})
	})
	a.section(c, "歌曲")
}

// distinct returns ss without empty strings, nor any after its first: an
// artist's translated name is often among their aliases too.
func distinct(ss []string) []string {
	var out []string
	for _, s := range ss {
		if s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}
