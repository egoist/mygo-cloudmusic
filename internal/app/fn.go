package app

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/fnmusic"
	"netease-music-mygo/internal/netease"
)

// The app shows the music of one platform at a time: NetEase's, or the
// songs on the listener's own Feiniu NAS, chosen from the account menu at
// the foot of the sidebar. The lists of the Feiniu platform are read from
// the service a page at a time as they are scrolled to (fnpager.go,
// fnlists.go) under numbers below zero, which NetEase's are not.

const (
	sourceNetease = "netease"
	sourceFn      = "fn"
)

// fnState is what the app knows of the listener's Feiniu music service.
type fnState struct {
	client *fnmusic.Client // nil until connected to
	err    string          // why the service could not be reached, or ""
	// playlists are the listener's, for the sidebar; liked the songs they
	// like, as the pages of songs tell as they come in.
	playlists []netease.Playlist
	liked     map[int64]bool
	likeBusy  map[int64]bool // songs being liked or unliked with the service
	form      fnForm

	lists  map[int64]*fnList          // the lists of songs opened, by number
	meta   map[int64]netease.Playlist // what cards stand for, by number
	search *fnList                    // the songs found for the search shown
	// searchAlbums and searchArtists are the albums and artists it found,
	// and foundAlbumList and foundArtistList where their pages are scrolled.
	searchAlbums, searchArtists     *pager[netease.Playlist]
	foundAlbumList, foundArtistList ui.ListState

	q      *fnQueue // where the queue playing is read from, if from a list
	roamID string   // the roam ID of the song the roam is at

	albums, artists       *pager[netease.Playlist]
	albumSort, artistSort string
	albumList, artistList ui.ListState // where those pages are scrolled
	libSort               string
	// sync, if set, runs what comes back from the service, in place of
	// posting it to the window: for tests, which have none.
	sync func(func())
}

// fnForm is the dialog to connect the service.
type fnForm struct {
	open bool
	// byAddress says the service is reached by its address, rather than
	// its FN ID. address is that, or the FN ID; https is the protocol chosen
	// for an address.
	byAddress                   bool
	https                       bool
	address, user, passwd, code string
	busy                        bool
	err                         string
	gen                         int
}

// onFn reports whether the app shows the Feiniu service's music.
func (a *app) onFn() bool { return a.settings.Source == sourceFn }

// isFnLiked reports whether the listener likes the song with this ID on
// Feiniu. NetEase's likes are handled by ui_like.go.
func (a *app) isFnLiked(id int64) bool { return a.fn.liked[id] }

// fnPost runs fn on the main thread, as what came back from the service.
func (a *app) fnPost(fn func()) {
	if a.fn.sync != nil {
		a.fn.sync(fn)
		return
	}
	a.post(fn)
}

// startFn makes the client of the service the settings name, if any: it
// signs in once it is asked for something, so what plays on picking up
// again has a client at once.
func (a *app) startFn() {
	// The NAS found over FN Connect may be elsewhere on another network.
	mygo.Network.OnChanged(func(mygo.NetworkStatus) {
		if c := a.fn.client; c != nil {
			c.Invalidate()
		}
	})
	cfg := a.settings.Fn
	loadFnSecrets(&cfg)
	if !cfg.Valid() {
		return
	}
	c, err := fnmusic.New(cfg)
	if err != nil {
		a.fn.err = err.Error()
		return
	}
	a.fn.client = c
	fnmusic.SetDefault(c)
	a.loadFnAccount()
}

// loadFnAccount loads what the listener keeps on the service besides its
// songs: their playlists. Failing is how a wrong address or password
// shows, once the service had been fine.
func (a *app) loadFnAccount() {
	c := a.fn.client
	if c == nil {
		return
	}
	go func() {
		lists, err := c.Playlists()
		a.fnPost(func() {
			if a.fn.client != c {
				return
			}
			if err != nil {
				a.fn.err = fnError(err)
				return
			}
			a.fn.err = ""
			a.fn.playlists = lists
			a.loadFnCovers(c, lists)
		})
	}()
}

// fnError says in words why the service failed.
func fnError(err error) string {
	if fnmusic.UserError(err) {
		return err.Error()
	}
	return "无法连接飞牛音乐：" + strings.TrimPrefix(err.Error(), "fnmusic: ")
}

// openFnConnect shows the dialog to connect the service, with what was
// connected before.
func (a *app) openFnConnect() {
	cfg := a.settings.Fn
	byAddress := cfg.Mode != fnmusic.ModeFNConnect && cfg.Address != ""
	address, https := splitScheme(cfg.Address, cfg.HTTPS)
	a.fn.form = fnForm{open: true, byAddress: byAddress, address: address, https: https,
		user: cfg.Username, gen: a.fn.form.gen + 1}
}

// splitScheme takes the scheme off an address the listener typed, which says
// the protocol: https, or http, in place of what was chosen.
func splitScheme(addr string, https bool) (string, bool) {
	addr = strings.TrimSpace(addr)
	switch {
	case strings.HasPrefix(strings.ToLower(addr), "https://"):
		return strings.TrimRight(addr[len("https://"):], "/"), true
	case strings.HasPrefix(strings.ToLower(addr), "http://"):
		return strings.TrimRight(addr[len("http://"):], "/"), false
	}
	return strings.TrimRight(addr, "/"), https
}

// submitFn signs in with what the dialog holds, and connects the
// service if it takes it.
func (a *app) submitFn() {
	f := &a.fn.form
	if f.busy {
		return
	}
	cfg := fnmusic.Config{
		Address:    strings.TrimSpace(f.address),
		Username:   strings.TrimSpace(f.user),
		PassHash:   fnmusic.HashPassword(f.passwd),
		DeviceID:   a.settings.Fn.DeviceID,
		AccessCode: strings.TrimSpace(f.code),
	}
	if f.byAddress {
		// A scheme typed in the address is the protocol, and is shown as
		// chosen.
		cfg.Address, cfg.HTTPS = splitScheme(cfg.Address, f.https)
		f.address, f.https = cfg.Address, cfg.HTTPS
	} else {
		cfg.Mode = fnmusic.ModeFNConnect
	}
	if cfg.Address == "" || cfg.Username == "" || f.passwd == "" {
		what := "FN ID"
		if f.byAddress {
			what = "服务器地址"
		}
		f.err = "请填写" + what + "、用户名和密码"
		return
	}
	c, err := fnmusic.New(cfg)
	if err != nil {
		f.err = "服务器地址无效"
		if cfg.Mode == fnmusic.ModeFNConnect {
			f.err = err.Error()
		}
		return
	}
	f.busy, f.err = true, ""
	gen := f.gen
	go func() {
		// The address is checked first, which says what is wrong with it:
		// an access code the NAS wants, a protocol it does not speak.
		err := c.Connect()
		if err == nil {
			err = c.Login()
		}
		a.post(func() {
			f := &a.fn.form
			if gen != f.gen {
				return
			}
			f.busy = false
			if err != nil {
				f.err = fnError(err)
				return
			}
			f.open, f.passwd, f.code = false, "", ""
			a.connectedFn(c)
		})
	}()
}

// connectedFn makes c the service the app shows.
func (a *app) connectedFn(c *fnmusic.Client) {
	a.settings.Fn = c.Config()
	a.settings.save()
	if err := saveFnSecrets(a.settings.Fn); err != nil {
		a.toast = "无法保存飞牛音乐的登录信息，下次启动需重新连接"
	}
	a.fn = fnState{client: c, form: a.fn.form}
	fnmusic.SetDefault(c)
	a.loadFnAccount()
	a.useSource(sourceFn)
	// Already there, the pages are of the new service.
	a.show(a.home())
}

// disconnectFn forgets the service and its login.
func (a *app) disconnectFn() {
	c := a.fn.client
	a.fn = fnState{form: a.fn.form}
	fnmusic.SetDefault(nil)
	a.settings.Fn = fnmusic.Config{DeviceID: a.settings.Fn.DeviceID}
	deleteFnSecrets()
	if a.onFn() {
		a.show(a.route)
	}
	a.settings.save()
	if c != nil {
		go c.Logout()
	}
}

// useSource shows the music of src, from its first page.
func (a *app) useSource(src string) {
	if a.settings.Source == src {
		return
	}
	a.settings.Source = src
	a.settings.save()
	a.history, a.forward = nil, nil
	a.nowPlaying = false
	a.query, a.queryWas = "", ""
	a.searched = route{}
	a.fn.search = nil
	a.show(a.home())
	a.redraw()
}

// home is the page a platform starts on: for Feiniu's, its library.
func (a *app) home() route {
	if a.onFn() {
		return route{page: pagePlaylist, id: fnmusic.LibraryID}
	}
	return route{page: pageHome}
}

// toggleFnLike likes a song of the service, or unlikes it, at once in the
// app and then with the service, which may refuse: the heart goes back then.
func (a *app) toggleFnLike(tr *netease.Track) {
	c := a.fn.client
	if c == nil {
		a.toast = "还没有连接飞牛音乐"
		return
	}
	if a.fn.liked == nil {
		a.fn.liked = map[int64]bool{}
	}
	if a.fn.likeBusy == nil {
		a.fn.likeBusy = map[int64]bool{}
	}
	id, ref, like := tr.ID, tr.Ref, !a.fn.liked[tr.ID]
	a.setFnLiked(id, like)
	a.fn.likeBusy[id] = true
	go func() {
		err := c.SetFavorite(ref, like)
		a.fnPost(func() {
			delete(a.fn.likeBusy, id)
			if err != nil {
				a.setFnLiked(id, !like)
				a.toast = "操作失败，请稍后再试"
				return
			}
			// The songs liked changed: read anew when shown. On show,
			// the song stays in its list until it is left.
			if a.route.page != pagePlaylist || a.route.id != fnmusic.FavoritesID {
				delete(a.fn.lists, fnmusic.FavoritesID)
			}
		})
	}()
}

func (a *app) setFnLiked(id int64, like bool) {
	if a.fn.liked == nil {
		a.fn.liked = map[int64]bool{}
	}
	if like {
		a.fn.liked[id] = true
	} else {
		delete(a.fn.liked, id)
	}
}
