package app

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// These load a podcast and its episodes from NetEase, and subscribe to it,
// and the podcasts the tab picks and the most popular; tests replace them.
var (
	podcastDetail    = netease.PodcastDetail
	podcastPrograms  = netease.PodcastPrograms
	subscribePodcast = netease.SubscribePodcast

	recommendedPodcasts = netease.RecommendedPodcasts
	hotPodcasts         = netease.HotPodcasts
)

// programsPage is how many of a podcast's episodes load at a time.
const programsPage = 30

// podcastData is what the app has of a podcast: what it is, and its
// episodes, the newest first, as far as they have loaded.
type podcastData struct {
	podcast     *netease.Podcast
	loading     bool
	err         string
	programs    []netease.Program
	page        paged
	subscribing bool // a subscription or its end is on its way
	thenPlay    bool // its card's play button waits for it to load
}

func (a *app) loadPodcast(id int64) {
	d := a.podcasts[id]
	if d == nil {
		d = &podcastData{}
		a.podcasts[id] = d
	}
	if !d.page.loaded {
		a.loadPrograms(id)
	}
	if d.podcast != nil || d.loading {
		return
	}
	d.loading, d.err = true, ""
	go func() {
		pod, err := podcastDetail(id)
		a.post(func() {
			d.loading = false
			if err != nil {
				d.err, d.thenPlay = "播客加载失败", false
				return
			}
			d.podcast = pod
			a.playLoaded(d)
		})
	}()
}

// loadPrograms loads the next page of a podcast's episodes.
func (a *app) loadPrograms(id int64) {
	d := a.podcasts[id]
	if d.page.loading {
		return
	}
	d.page.loading, d.page.err = true, ""
	offset := len(d.programs)
	go func() {
		programs, more, err := podcastPrograms(id, programsPage, offset)
		a.post(func() {
			d.page.loading = false
			if err != nil {
				d.page.err, d.thenPlay = "节目加载失败", false
				return
			}
			d.programs = append(d.programs, programs...)
			d.page.loaded, d.page.more = true, more && len(programs) > 0
			// Which the listener likes, for their hearts.
			ids := make([]int64, len(programs))
			for i := range programs {
				ids[i] = programs[i].ID
			}
			a.askProgramLikes(ids)
			a.playLoaded(d)
		})
	}()
}

// playProgram plays the episodes loaded, from the one at i: those with
// audio to play.
func (a *app) playProgram(d *podcastData, i int) {
	var queue []netease.Track
	at := 0
	for j := range d.programs {
		if d.programs[j].MainSong.ID == 0 {
			continue
		}
		if j <= i {
			at = len(queue)
		}
		queue = append(queue, d.programs[j].Track(d.podcast))
	}
	if len(queue) > 0 {
		a.pl.PlayQueue(queue, at)
	}
}

// playPodcast plays a podcast's episodes from the newest, from its card,
// once it and they have loaded.
func (a *app) playPodcast(id int64) {
	a.loadPodcast(id)
	d := a.podcasts[id]
	d.thenPlay = true
	a.playLoaded(d)
}

// playLoaded plays a podcast its card asked to play, once it can.
func (a *app) playLoaded(d *podcastData) {
	if d.thenPlay && d.podcast != nil && d.page.loaded {
		d.thenPlay = false
		a.playProgram(d, 0)
	}
}

// toggleSubscribe subscribes the listener to a podcast, or unsubscribes
// them, at once, going back if NetEase refuses. Signed out, it asks the
// listener to sign in.
func (a *app) toggleSubscribe(d *podcastData) {
	if d.podcast == nil || d.subscribing {
		return
	}
	if a.profile == nil {
		a.openLogin()
		return
	}
	pod := d.podcast
	tab := a.podcastTab
	sub := !pod.Subed
	pod.Subed, d.subscribing = sub, true
	if sub {
		pod.SubCount++
	} else {
		pod.SubCount--
	}
	go func() {
		err := subscribePodcast(pod.ID, sub)
		a.post(func() {
			d.subscribing = false
			switch {
			case err != nil:
				pod.Subed = !sub
				if sub {
					pod.SubCount--
				} else {
					pod.SubCount++
				}
				a.toast = "操作失败，请稍后再试"
			case sub:
				tab.subscribed(pod)
				a.toast = "已订阅"
			default:
				tab.unsubscribed(pod.ID)
				a.toast = "已取消订阅"
			}
		})
	}()
}

func (a *app) podcastPage(c *ui.Context) {
	p := a.pal
	id := a.route.id
	d := a.podcasts[id]
	if d.podcast == nil {
		if !d.loading {
			a.centered(c, d.err, func() { a.loadPodcast(id) })
			return
		}
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, true, func(c *ui.Context) { a.playlistHeaderSkeleton(c, s) })
		return
	}
	pod := d.podcast
	cur := a.pl.Current()
	a.scrollPage(c, func() {
		ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 8, 0).Children(func() {
			ui.Box(c).Size(212, 212).Radius(14).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
				a.coverFill(c, pod.PicURL, 600, 14)
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
				kind := "播客"
				if pod.Category != "" {
					kind += " · " + pod.Category
				}
				ui.Text(c, kind).FontSize(12).FontWeight(600).TextColor(p.accent)
				ui.Text(c, pod.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
				ui.RichText(c).FontSize(13).TextColor(p.muted).Children(func() {
					if pod.DJ.UserID != 0 {
						a.link(c.Key("dj"), pod.DJ.Nickname, p.muted, p.text, route{page: pageUser, id: pod.DJ.UserID})
						ui.Text(c, " · ")
					}
					ui.Text(c, fmt.Sprintf("%d 期 · %s 人订阅", pod.ProgramCount, count(int(pod.SubCount))))
				})
				if desc := strings.TrimSpace(pod.Description); desc != "" {
					ui.Text(c, strings.ReplaceAll(desc, "\n", " ")).FontSize(12.5).TextColor(p.muted).MaxLines(2).LineHeight(1.45).Margin(2, 0, 0, 0)
				}
				ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
					if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
						a.playProgram(d, 0)
					}
					icon, label := iconPlus, "订阅"
					if pod.Subed {
						icon, label = iconCheck, "已订阅"
					}
					if pillButton(c.Key("subscribe"), icon, label, p.hover.Over(p.bg), p.text).Clicked() {
						a.toggleSubscribe(d)
					}
				})
			})
		})
		a.section(c, "节目")
		for i := range d.programs {
			a.programRow(c.Key(d.programs[i].ID), d, i, cur)
		}
		a.pageFoot(c, &d.page, func() { a.loadPrograms(id) })
		if d.page.loaded && len(d.programs) == 0 {
			a.empty(c, "暂无节目")
		}
	})
}

// programRow is an episode in its podcast's list, which a click plays.
func (a *app) programRow(c *ui.Context, d *podcastData, i int, cur *netease.Track) {
	p := a.pal
	pr := &d.programs[i]
	playing := cur != nil && cur.ProgramID == pr.ID
	row := ui.ButtonBase(c).Height(60).PaddingX(12).Gap(14).Radius(8).AlignItems(ui.Center).FocusRing(false).
		Label(pr.Name).Disabled(pr.MainSong.ID == 0)
	switch {
	case playing:
		row.Background(p.accent.Alpha(0.08))
	case row.Hovered():
		row.Background(p.hover)
	}
	row.Children(func() {
		cover := pr.CoverURL
		if cover == "" {
			cover = d.podcast.PicURL
		}
		a.cover(c, cover, 96, 40, 6)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			title := ui.Text(c, pr.Name).FontSize(13.5).FontWeight(500).SingleLine()
			if playing {
				title.TextColor(p.accent)
			}
			ui.Text(c, time.UnixMilli(pr.CreateTime).Format("2006-01-02")).FontSize(12).TextColor(p.muted)
		})
		if playing {
			a.nowPlayingBars(c)
		}
		// A liked episode's heart shows; the others' as the pointer is on them.
		ui.Box(c).Size(32, 32).Center().Children(func() {
			if a.likedPrograms[pr.ID] || row.Hovered() {
				tr := pr.Track(d.podcast)
				a.likeButton(c.Key("like"), &tr, 15, p.muted)
			}
		})
		ui.Text(c, clock(time.Duration(pr.Duration)*time.Millisecond)).FontSize(12).FontFeatures("tnum").TextColor(p.muted)
	})
	if row.Clicked() {
		a.playProgram(d, i)
	}
}

// podcastRow is a podcast in a list, opening its page.
func (a *app) podcastRow(c *ui.Context, pod *netease.Podcast) {
	p := a.pal
	row := ui.ButtonBase(c).Padding(10, 12).Gap(14).Radius(10).AlignItems(ui.Center).FocusRing(false).Label(pod.Name)
	if row.Hovered() {
		row.Background(p.hover)
	}
	row.Children(func() {
		a.cover(c, pod.PicURL, 160, 64, 8)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, pod.Name).FontSize(14).FontWeight(600).SingleLine()
			meta := []string{fmt.Sprintf("%d 期", pod.ProgramCount)}
			if pod.DJ.Nickname != "" {
				meta = append([]string{pod.DJ.Nickname}, meta...)
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(12.5).TextColor(p.muted).SingleLine()
			if pod.Description != "" {
				ui.Text(c, pod.Description).FontSize(12.5).TextColor(p.muted).SingleLine()
			}
		})
	})
	if row.Clicked() {
		a.open(route{page: pagePodcast, id: pod.ID})
	}
}

// programPane is what the episode playing is about, in Now Playing, in
// place of a song's lyrics.
func (a *app) programPane(c *ui.Context, tr *netease.Track) {
	ui.Scroll(c.Key(tr.ProgramID)).Grow(1).Padding(4, 12, 48, 0).Children(func() {
		desc := strings.TrimSpace(tr.Description)
		if desc == "" {
			desc = "这期节目没有简介"
		}
		ui.Text(c, desc).FontSize(15).LineHeight(1.7).TextColor(white.Alpha(0.75))
	})
}

// podcastTab is what the podcasts tab lists: the podcasts the listener
// subscribes to, NetEase's picks, and the most popular, as far as they have
// loaded.
type podcastTab struct {
	subs    []netease.Podcast // the latest subscribed first
	picks   []netease.Podcast
	hot     []netease.Podcast
	hotPage paged
	hotAt   int // how far into NetEase's list the next page starts
	loading bool
	err     string
}

func (a *app) loadPodcastTab() {
	t := a.podcastTab
	if t.hotPage.loaded || t.loading {
		return
	}
	t.loading, t.err = true, ""
	signedIn := a.profile != nil
	go func() {
		var subs []netease.Podcast
		if signedIn {
			subs, _ = subscribedPodcasts()
		}
		picks, err1 := recommendedPodcasts()
		hot, more, err2 := hotPodcasts(programsPage, 0)
		a.post(func() {
			t.loading = false
			if err1 != nil && err2 != nil {
				t.err = "无法连接网易云音乐，请检查网络"
				return
			}
			t.subs, t.picks = subs, picks
			t.addHot(hot)
			t.hotPage = paged{loaded: true, more: more && len(hot) > 0}
			if err2 != nil {
				t.hotPage.err = "热门播客加载失败"
			}
		})
	}()
}

// loadHotPodcasts loads the next page of the most popular podcasts.
func (a *app) loadHotPodcasts() {
	t := a.podcastTab
	if t.hotPage.loading {
		return
	}
	t.hotPage.loading, t.hotPage.err = true, ""
	offset := t.hotAt
	go func() {
		hot, more, err := hotPodcasts(programsPage, offset)
		a.post(func() {
			t.hotPage.loading = false
			if err != nil {
				t.hotPage.err = "热门播客加载失败"
				return
			}
			t.addHot(hot)
			t.hotPage.more = more && len(hot) > 0
		})
	}()
}

// addHot lists a page of the most popular podcasts after those listed,
// but those already listed: NetEase's pages overlap, a podcast often
// coming again a page or two later.
func (t *podcastTab) addHot(page []netease.Podcast) {
	t.hotAt += len(page)
	for _, pod := range page {
		if !slices.ContainsFunc(t.hot, func(p netease.Podcast) bool { return p.ID == pod.ID }) {
			t.hot = append(t.hot, pod)
		}
	}
}

// subscribed lists a podcast the listener just subscribed to first, once
// the tab has loaded what they subscribe to.
func (t *podcastTab) subscribed(pod *netease.Podcast) {
	if !t.hotPage.loaded {
		return
	}
	t.unsubscribed(pod.ID)
	t.subs = append([]netease.Podcast{*pod}, t.subs...)
}

// unsubscribed takes a podcast out of the listener's subscriptions.
func (t *podcastTab) unsubscribed(id int64) {
	t.subs = slices.DeleteFunc(t.subs, func(p netease.Podcast) bool { return p.ID == id })
}

func (a *app) podcastsPage(c *ui.Context) {
	t := a.podcastTab
	if !t.hotPage.loaded && !t.loading {
		a.centered(c, t.err, a.loadPodcastTab)
		return
	}
	top, bottom := a.insets(c)
	ui.Scroll(c.Key("podcasts")).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		a.heading(c, "播客", "")
		cols := a.columns(c, 150)
		if !t.hotPage.loaded {
			s := a.pageShimmer(c)
			a.section(c, "精选播客").Margin(0, 0, 14, 0)
			a.playlistCardsSkeleton(c, s, cols, 150, true)
			a.section(c, "热门播客")
			a.playlistCardsSkeleton(c, s, cols*2, 150, true)
			return
		}
		// The first list starts right under the heading.
		first := true
		list := func(title string, pods []netease.Podcast) {
			if len(pods) == 0 {
				return
			}
			if s := a.section(c, title); first {
				s.Margin(0, 0, 14, 0)
			}
			first = false
			ui.Grid(c.Key(title)).Columns(cols).GapX(20).GapY(28).Children(func() {
				for i := range pods {
					a.podcastCard(c.Key(pods[i].ID), &pods[i])
				}
			})
		}
		list("我的订阅", t.subs)
		list("精选播客", t.picks)
		list("热门播客", t.hot)
		a.pageFoot(c, &t.hotPage, a.loadHotPodcasts)
	})
}

// podcastCard is a podcast's cover over its name and, for those the
// listener subscribes to, its newest episode; for the others, why to
// listen.
func (a *app) podcastCard(c *ui.Context, pod *netease.Podcast) {
	sub := pod.LastProgram
	if sub == "" {
		sub = pod.RcmdText
	}
	if sub == "" {
		sub = pod.DJ.Nickname
	}
	id := pod.ID
	if a.coverCard(c, pod.Name, pod.PicURL, pod.PlayCount, sub, func() { a.playPodcast(id) }) {
		a.open(route{page: pagePodcast, id: id})
	}
}
