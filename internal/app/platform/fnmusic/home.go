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

func (h *Home) SearchText() *string {
	if h == nil {
		return nil
	}
	return &h.search
}

func (h *Home) Open(route Route) {
	if route.Page == "" {
		route.Page = PageLibrary
	}
	h.route = route
	if route.Page == PageSearch && h.state != nil {
		if err := h.state.Catalog().Search(route.Query); err != nil {
			h.state.SetError(err)
		}
	}
}

func (h *Home) Content(c *ui.Context) {
	if h == nil || h.host == nil || h.state == nil {
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
