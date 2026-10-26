package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// copyText puts text on the clipboard; tests replace it.
var copyText = func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) }

// songMenu is three dots that open the menu of tr.
func (a *app) songMenu(c *ui.Context, tr *netease.Track, size float32, color ui.Color) ui.Element {
	b := iconButton(c, iconMore, size, color, "更多")
	b.Menu(a.songItems(tr, 0))
	return b
}

// songItems builds the menu of what can be done with tr, behind its dots
// or a right click on its row: copying its title, its title with its
// artists, or the link to its page, opening its artists' or album's, and,
// signed in, adding it to a playlist or taking it out of pid, the playlist
// it is listed in, if set.
func (a *app) songItems(tr *netease.Track, pid int64) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		if tr.ProgramID != 0 {
			a.programItems(m, tr)
			return
		}
		if m.Item("复制歌名").Chosen() {
			copyText(tr.Name)
		}
		if m.Item("复制歌名和歌手").Chosen() {
			copyText(tr.Name + " - " + tr.ArtistNames())
		}
		if tr.URL() != "" {
			m.Separator()
			if m.Item("复制链接").Chosen() {
				copyText(tr.URL())
			}
		}
		// Its artists' and album's pages, those NetEase has.
		linked := tr.Album.ID != 0
		for _, ar := range tr.Artists {
			linked = linked || ar.ID != 0
		}
		if linked {
			m.Separator()
		}
		for _, ar := range tr.Artists {
			if ar.ID != 0 && m.Item("查看歌手："+ar.Name).Chosen() {
				a.openArtist(ar.ID)
			}
		}
		if tr.Album.ID != 0 && m.Item("查看专辑："+tr.Album.Name).Chosen() {
			a.openPage(route{page: pageAlbum, id: tr.Album.ID})
		}
		a.playlistItems(m, tr, pid)
	}
}

// programItems builds the menu of an episode: copying its title or the
// link to its page, or opening its podcast's.
func (a *app) programItems(m *ui.Menu, tr *netease.Track) {
	if m.Item("复制节目名").Chosen() {
		copyText(tr.Name)
	}
	if m.Item("复制链接").Chosen() {
		copyText(tr.URL())
	}
	m.Separator()
	if m.Item("查看播客").Chosen() {
		a.openPage(route{page: pagePodcast, id: tr.PodcastID})
	}
}
