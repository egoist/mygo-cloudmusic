package app

import (
	"errors"
	"os"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/fnmusic"
	"netease-music-mygo/internal/netease"
)

// The listener's playlists on the Feiniu service: making them, renaming them,
// giving them a cover, deleting them, and putting songs in them or taking
// songs out. They are the playlists of the service's account, so that what
// is done here is seen by the service's own player too.

// pickCover asks the listener for a picture to make a playlist's cover, and
// returns its bytes, or nil if they chose none; tests replace it.
var pickCover = func() ([]byte, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:   "选择歌单封面",
		Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "webp", "gif"}}},
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	info, err := os.Stat(paths[0])
	if err != nil {
		return nil, err
	}
	if info.Size() > fnmusic.MaxCoverBytes {
		return nil, fnmusic.ErrBadCover
	}
	return os.ReadFile(paths[0])
}

// fnPlaylist returns the listener's playlist numbered id.
func (a *app) fnPlaylist(id int64) (netease.Playlist, bool) {
	for _, pl := range a.fn.playlists {
		if pl.ID == id {
			return pl, true
		}
	}
	return netease.Playlist{}, false
}

// changeFnPlaylist makes change of the playlist numbered id, wherever the
// app has it: the sidebar's, what its card stands for, and its list.
func (a *app) changeFnPlaylist(id int64, change func(pl *netease.Playlist)) {
	for i := range a.fn.playlists {
		if a.fn.playlists[i].ID == id {
			change(&a.fn.playlists[i])
		}
	}
	if m, ok := a.fn.meta[id]; ok {
		change(&m)
		a.fn.meta[id] = m
	}
	if l := a.fn.lists[id]; l != nil {
		var pl netease.Playlist
		pl.Name, pl.CoverURL = l.name, l.cover
		change(&pl)
		l.name, l.cover = pl.Name, pl.CoverURL
	}
}

// newFnPlaylist asks for the name of a playlist to make, and then puts songs
// in it, if any.
func (a *app) newFnPlaylist(songs []netease.Track) {
	if a.fn.client == nil {
		a.toast = "还没有连接飞牛音乐"
		return
	}
	a.naming = nameDialog{open: true, fn: true, fnAdd: songs}
}

// startFnRename asks for a new name for the playlist.
func (a *app) startFnRename(pl netease.Playlist) {
	a.naming = nameDialog{open: true, fn: true, name: pl.Name, rename: pl.ID}
}

// submitFnName makes the playlist named, or renames one: at once in the app,
// as the service may refuse, when renaming.
func (a *app) submitFnName() {
	d := &a.naming
	name := strings.TrimSpace(d.name)
	c := a.fn.client
	if name == "" || d.busy || c == nil {
		return
	}
	if d.rename != 0 {
		id := d.rename
		d.open = false
		pl, ok := a.fnPlaylist(id)
		if !ok || pl.Name == name {
			return
		}
		old := pl.Name
		a.changeFnPlaylist(id, func(p *netease.Playlist) { p.Name = name })
		go func() {
			err := c.RenamePlaylist(pl.Ref, name)
			a.fnPost(func() {
				if err != nil {
					a.changeFnPlaylist(id, func(p *netease.Playlist) { p.Name = old })
					a.toast = "重命名失败，请稍后再试"
				}
			})
		}()
		return
	}
	d.busy = true
	add, from := d.fnAdd, d.fnList
	go func() {
		pl, err := c.CreatePlaylist(name)
		a.fnPost(func() {
			a.naming.busy = false
			if err != nil {
				a.toast = "创建歌单失败，请稍后再试"
				return
			}
			a.naming.open = false
			// First, as the service lists the newest.
			a.fn.playlists = slices.Insert(a.fn.playlists, 0, pl)
			a.open(route{page: pagePlaylist, id: pl.ID})
			if len(add) > 0 {
				a.addToFnPlaylist(pl, add)
			}
			if from != nil {
				a.addListToFnPlaylist(from, pl)
			}
		})
	}()
}

// confirmFnDelete asks whether to delete the playlist, which can't be undone.
func (a *app) confirmFnDelete(pl netease.Playlist) {
	a.deleting = deleteDialog{open: true, id: pl.ID, name: pl.Name, fn: true}
}

// deleteFnChosen deletes the playlist asked about, at once in the app and
// then with the service, which may refuse: it comes back then. The songs in
// it stay in the library.
func (a *app) deleteFnChosen() {
	id := a.deleting.id
	a.deleting.open = false
	i := slices.IndexFunc(a.fn.playlists, func(pl netease.Playlist) bool { return pl.ID == id })
	c := a.fn.client
	if i < 0 || c == nil {
		return
	}
	pl := a.fn.playlists[i]
	a.fn.playlists = slices.Delete(slices.Clone(a.fn.playlists), i, i+1)
	// Leave no way back to its page, and leave it for the page before.
	gone := func(r route) bool { return r.page == pagePlaylist && r.id == id }
	a.history = slices.DeleteFunc(a.history, gone)
	a.forward = slices.DeleteFunc(a.forward, gone)
	if gone(a.route) {
		r := a.home()
		if n := len(a.history); n > 0 {
			r, a.history = a.history[n-1], a.history[:n-1]
		}
		a.show(r)
	}
	go func() {
		err := c.DeletePlaylist(pl.Ref)
		a.fnPost(func() {
			if err != nil {
				a.fn.playlists = slices.Insert(a.fn.playlists, min(i, len(a.fn.playlists)), pl)
				a.toast = "删除失败，请稍后再试"
				return
			}
			delete(a.fn.lists, id)
			a.toast = "已删除歌单"
		})
	}()
}

// addToFnPlaylist puts songs in the playlist. The service puts them first,
// and takes one that is in already for one to move there, so there is no
// telling the listener it is in.
func (a *app) addToFnPlaylist(pl netease.Playlist, songs []netease.Track) {
	c := a.fn.client
	if c == nil || len(songs) == 0 {
		return
	}
	refs := make([]string, len(songs))
	for i, t := range songs {
		refs[i] = t.Ref
	}
	go func() {
		err := c.AddToPlaylist(pl.Ref, refs)
		a.fnPost(func() {
			if err != nil {
				a.toast = "添加失败，请稍后再试"
				return
			}
			a.toast = "已添加到歌单"
			// Read anew when shown, from its start, where the songs are.
			delete(a.fn.lists, pl.ID)
			if a.route.page == pagePlaylist && a.route.id == pl.ID {
				a.resetListScroll()
			}
		})
	}()
}

// removeFromFnPlaylist takes the song at position i out of the playlist's
// list l, at once in the app and then with the service, which may refuse: the
// list is read anew then.
func (a *app) removeFromFnPlaylist(l *fnList, i int) {
	c := a.fn.client
	tr := l.pg.at(i)
	if c == nil || tr == nil || l.spec.typ != listPlaylist {
		return
	}
	ref, id := tr.Ref, l.id
	l.pg.removeAt(i)
	go func() {
		err := c.RemoveFromPlaylist(l.spec.ref, []string{ref})
		a.fnPost(func() {
			if err != nil {
				delete(a.fn.lists, id)
				a.toast = "移除失败，请稍后再试"
			}
		})
	}()
}

// newFnPlaylistFrom asks for the name of a playlist to make of the songs of
// the album or the artist l, read when it is named.
func (a *app) newFnPlaylistFrom(l *fnList) {
	if a.fn.client == nil {
		return
	}
	a.naming = nameDialog{open: true, fn: true, fnList: l}
}

// addListToFnPlaylist puts every song of the album or the artist in the
// playlist: the one time the whole of a list is read, and it is a short one.
func (a *app) addListToFnPlaylist(l *fnList, pl netease.Playlist) {
	l.pg.whole(func(ok bool) {
		if !ok || l.pg.len() == 0 {
			a.toast = "添加失败，请稍后再试"
			return
		}
		a.addToFnPlaylist(pl, slices.Clone(l.pg.items))
	})
}

// changeFnCover asks for a picture and makes it the playlist's cover.
func (a *app) changeFnCover(pl netease.Playlist) {
	c := a.fn.client
	if c == nil {
		return
	}
	pick := pickCover
	go func() {
		picture, err := pick()
		if err == nil && picture == nil {
			return // none chosen
		}
		var id string
		if err == nil {
			id, err = c.SetPlaylistCover(pl.Ref, pl.Name, picture)
		}
		a.fnPost(func() {
			if err != nil {
				a.toast = "更换封面失败，请稍后再试"
				if fnmusic.UserError(err) || errors.Is(err, fnmusic.ErrBadCover) {
					a.toast = err.Error()
				}
				return
			}
			url := fnmusic.CoverURL(id)
			a.changeFnPlaylist(pl.ID, func(p *netease.Playlist) { p.CoverURL = url })
			a.toast = "已更换封面"
		})
	}()
}

// loadFnCovers gives the playlists that have no cover of their own the
// cover of their first song, as a playlist shows in the sidebar and on
// its page: one song asked for of each.
func (a *app) loadFnCovers(c *fnmusic.Client, lists []netease.Playlist) {
	for _, pl := range lists {
		if pl.CoverURL != "" {
			continue
		}
		go func() {
			songs, _, err := c.PlaylistPage(pl.Ref, 1, 1)
			if err != nil || len(songs) == 0 || songs[0].Album.PicURL == "" {
				return
			}
			a.fnPost(func() {
				if a.fn.client != c {
					return
				}
				// Only where there is still none: one the listener has
				// chosen since stays.
				a.changeFnPlaylist(pl.ID, func(p *netease.Playlist) {
					if p.CoverURL == "" {
						p.CoverURL = songs[0].Album.PicURL
					}
				})
			})
		}()
	}
}

// fnPlaylistItems are the items of a Feiniu song's menu that put it in the
// listener's playlists, or take it out of pid, the one it is listed in.
func (a *app) fnPlaylistItems(m *ui.Menu, tr *netease.Track, pid int64) {
	if a.fn.client == nil {
		return
	}
	m.Separator()
	m.Submenu("添加到歌单", func(m *ui.Menu) {
		if m.Item("新建歌单…").Chosen() {
			a.newFnPlaylist([]netease.Track{*tr})
		}
		var to []netease.Playlist
		for _, pl := range a.fn.playlists {
			if pl.ID != pid {
				to = append(to, pl)
			}
		}
		if len(to) > 0 {
			m.Separator()
		}
		for _, pl := range to {
			if m.Item(pl.Name).Chosen() {
				a.addToFnPlaylist(pl, []netease.Track{*tr})
			}
		}
	})
	if l := a.fn.lists[pid]; l != nil && l.spec.typ == listPlaylist {
		if m.Item("从歌单中移除").Chosen() {
			// The song's place in the list, among those read.
			if i := l.pg.indexOf(func(t *netease.Track) bool { return t.Ref == tr.Ref }); i >= 0 {
				a.removeFromFnPlaylist(l, i)
			}
		}
	}
}
