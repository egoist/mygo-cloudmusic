package app

import "github.com/egoist/mygo/ui"

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
		home.Content(c)
		return
	}
	a.content(c)
}
