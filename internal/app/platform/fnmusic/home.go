package fnmusic

import (
	"context"

	"github.com/egoist/mygo/ui"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
)

// Home owns the Feiniu navigation and page selection. It deliberately does
// not know the root app's route type or render implementation.
type Home struct {
	state   *State
	host    Host
	runtime appplatform.Runtime
	route   Route
	history []Route
	forward []Route

	// form survives rebuilds of the settings window, as text input state must
	// belong to the platform view rather than a local render closure.
	formAddress, formUser, formPassword string
	formCode                            string
	formMode                            string
	formHTTPS                           bool
	connectionOpen                      bool
	connectionBusy                      bool
	connectionError                     string
	connectionGen                       uint64
	accountOpen                         bool
	search                              string
	navScroll                           ui.ScrollState
	nameOpen                            bool
	deleteOpen                          bool
	name                                string
	rename                              *netease.Playlist
	delete                              *netease.Playlist
	addTracks                           []netease.Track
	addList                             *List
}

func (h *Home) postCurrent(gen uint64, fn func()) {
	if h == nil || h.host == nil {
		return
	}
	h.host.Post(func() {
		if h.state == nil || h.state.Generation() != gen {
			return
		}
		fn()
	})
}

func NewHome(state *State, host Host, runtimes ...appplatform.Runtime) *Home {
	var runtime appplatform.Runtime
	if len(runtimes) > 0 {
		runtime = runtimes[0]
	}
	if runtime == nil {
		runtime = NewRuntime(state, nil)
	}
	h := &Home{state: state, host: host, runtime: runtime, route: Route{Page: PageLibrary}}
	if state != nil {
		cfg := toConnectionConfig(state.Config())
		h.formAddress, h.formUser, h.formCode = cfg.Address, cfg.Username, cfg.AccessCode
		h.formMode, h.formHTTPS = cfg.Mode, cfg.HTTPS
	}
	return h
}

func (h *Home) State() *State { return h.state }

func (h *Home) Route() Route { return h.route }

func (h *Home) CanBack() bool    { return h != nil && len(h.history) > 0 }
func (h *Home) CanForward() bool { return h != nil && len(h.forward) > 0 }

// ResetNavigation selects a platform page without carrying navigation from a
// previous connection or platform session.
func (h *Home) ResetNavigation(route Route) {
	if h == nil {
		return
	}
	if route.Page == "" {
		route.Page = PageLibrary
	}
	h.history, h.forward = nil, nil
	h.openRoute(route)
}

func (h *Home) SearchText() *string {
	if h == nil {
		return nil
	}
	return &h.search
}

func (h *Home) Open(route Route) {
	if h == nil {
		return
	}
	if route.Page == "" {
		route.Page = PageLibrary
	}
	if route == h.route {
		return
	}
	h.history = append(h.history, h.route)
	h.forward = nil
	h.openRoute(route)
}

func (h *Home) openRoute(route Route) {
	h.route = route
	if route.Page == PageSearch && h.state != nil {
		if err := h.state.Catalog().Search(route.Query); err != nil {
			h.state.SetError(err)
		}
	}
}

func (h *Home) Back() {
	if h == nil || len(h.history) == 0 {
		return
	}
	n := len(h.history) - 1
	route := h.history[n]
	h.history = h.history[:n]
	h.forward = append(h.forward, h.route)
	h.openRoute(route)
}

func (h *Home) Forward() {
	if h == nil || len(h.forward) == 0 {
		return
	}
	n := len(h.forward) - 1
	route := h.forward[n]
	h.forward = h.forward[:n]
	h.history = append(h.history, h.route)
	h.openRoute(route)
}

func (h *Home) Content(c *ui.Context) {
	if h == nil || h.host == nil || h.state == nil {
		return
	}
	if h.state.Connecting() {
		h.connectionLoading(c)
		h.dialogs(c)
		return
	}
	if !h.state.Connected() {
		h.connectionPage(c)
		return
	}
	switch h.route.Page {
	case PageSearch, PageSearchAlbums, PageSearchArtists, PageAlbums, PageArtists:
		h.cardsPage(c)
	case PageRoam:
		h.roamPage(c)
	default:
		h.listPage(c)
	}
	h.dialogs(c)
}

func (h *Home) connectionLoading(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Spinner(c).Label("正在连接飞牛音乐")
		ui.Text(c, "正在加载音乐库和歌单…").FontSize(13).TextColor(t.TextMuted)
	})
}

func (h *Home) RequestFavorite(tr netease.Track, liked bool) {
	if h.state == nil {
		return
	}
	gen := h.state.Generation()
	go func() {
		err := h.state.SetFavorite(context.Background(), &tr, liked)
		h.postCurrent(gen, func() {
			if err != nil {
				h.host.Toast("操作失败，请稍后再试")
			}
		})
	}()
}
