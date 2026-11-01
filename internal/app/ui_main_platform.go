package app

import (
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	appplatform "netease-music-mygo/internal/app/platform"
)

// platformPageTop draws navigation for a platform-owned route. The upstream
// history stack must remain reserved for the NetEase pages, so the active
// platform supplies its own neutral back/forward state through Home.
func (a *app) platformPageTop(c *ui.Context) bool {
	home := a.activeHome()
	if home == nil {
		return false
	}
	top, _ := a.insets(c)
	edge := ui.Box(c.Key("platform-top-edge")).Absolute().Top(0).Left(0).Right(0).Height(top).
		DragWindow().Material(glass.ScrollEdge{Hard: true, Background: a.pal.bg})
	edge.Opacity(edgeShown(edge, a.underTop))
	if !home.CanBack() && !home.CanForward() {
		return true
	}
	ui.Row(c).Absolute().Top(0).Left(20).Height(topBar(c)).AlignItems(ui.Center).Children(func() {
		back := iconButton(c.Key("platform-back"), iconBack, 18, a.pal.muted, "返回").Disabled(!home.CanBack())
		if !home.CanBack() {
			back.Opacity(0.35)
		}
		if back.Clicked() {
			home.Back()
			a.platformRoute = appplatform.Route{}
			a.redraw()
		}
		if home.CanForward() && iconButton(c.Key("platform-forward"), iconForward, 18, a.pal.muted, "前进").Clicked() {
			home.Forward()
			a.platformRoute = appplatform.Route{}
			a.redraw()
		}
	})
	return true
}

// Main view platform extension seam.
//
// Keep the upstream sidebar and content renderers intact. A platform-owned
// home is selected here through the neutral app seam and takes over only the
// two shared slots.

func (a *app) platformSidebar(c *ui.Context) {
	if home := a.activeHome(); home != nil {
		top := float32(14)
		if c.TitleBar().Left > 0 {
			top = topBar(c) + 4
		}
		ui.Column(c).Fill().Padding(top, 14, 14, 14).DragWindow().Children(func() {
			home.Sidebar(c)
		})
		return
	}
	a.sidebar(c)
}

func (a *app) platformContent(c *ui.Context) {
	if home := a.activeHome(); home != nil {
		// A platform page can navigate from inside a ui.List row callback. Do
		// not reset that ListState from the callback itself: MyGo is still
		// holding the current list frame. Reset it at the next build instead.
		route := home.Route()
		if route != a.platformRoute {
			a.trackList, a.platformCardList = ui.ListState{}, ui.ListState{}
			a.pageScroll = ui.ScrollState{}
			a.platformHeroSince = time.Time{}
			a.underTop, a.underBottom = false, false
			a.platformRoute = route
		}
		home.Content(c)
		return
	}
	a.platformRoute = appplatform.Route{}
	a.content(c)
}
