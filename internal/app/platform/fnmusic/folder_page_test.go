package fnmusic

import (
	"testing"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/fnmusic"
)

// A folder's breadcrumbs start at the library holding it, matching the longest root path.
func TestFolderCrumbsStartAtTheirLibrary(t *testing.T) {
	f := newFolderViewState(nil)
	f.roots = []backend.SharedLibrary{
		{Name: "音乐", Path: "/vol1/music"},
		{Name: "", Path: "/vol1/music-hd/"},
	}
	got := f.folderCrumbs("/vol1/music-hd/Adele/25")
	want := []folderCrumb{{Name: "music-hd", Path: "/vol1/music-hd"}, {Name: "Adele", Path: "/vol1/music-hd/Adele"}}
	if len(got) != len(want) {
		t.Fatalf("crumbs = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("crumbs = %#v, want %#v", got, want)
		}
	}
	if crumbs := f.folderCrumbs("/vol1/music"); len(crumbs) != 0 {
		t.Fatalf("a library has no folders over it: %#v", crumbs)
	}
	if crumbs := f.folderCrumbs("/elsewhere/a"); crumbs != nil {
		t.Fatalf("a folder outside every library has no crumbs: %#v", crumbs)
	}
}

// Changing the order drops every cached folder so none is shown in the old order.
func TestFolderSortDropsCachedPages(t *testing.T) {
	h := NewHome(NewState(), nil)
	h.host = nopHost{}
	f := h.folderState()
	f.pages["/a"] = backend.FolderPage{Path: "/a"}
	h.setFolderSort(backend.SortFolderCreatedDesc)
	if len(f.pages) != 0 || f.sort != backend.SortFolderCreatedDesc {
		t.Fatalf("after sort: pages=%v sort=%q", f.pages, f.sort)
	}
}

// nopHost satisfies Host for logic that only asks for a redraw.
type nopHost struct{ Host }

func (nopHost) Invalidate() {}

// Playing a folder takes the songs' lengths and formats from the folders
// already shown: playable-tracks leaves them out, and its songs showed 0:00
// with a progress bar that never moved.
func TestPlayFolderFillsTracksFromLoadedPages(t *testing.T) {
	h := NewHome(NewState(), nil)
	f := h.folderState()
	f.pages["/a"] = backend.FolderPage{Path: "/a", Tracks: []backend.FolderTrack{{Track: netease.Track{Ref: "r1", Duration: 198632, Format: "flac", Album: netease.Album{Name: "专辑"}}}}}
	tracks := []netease.Track{{Ref: "r1"}, {Ref: "r2"}}
	h.fillFolderTracks(tracks)
	if tracks[0].Duration != 198632 || tracks[0].Format != "flac" || tracks[0].Album.Name != "专辑" {
		t.Fatalf("filled = %+v", tracks[0])
	}
	if tracks[1].Duration != 0 {
		t.Fatalf("an unknown song was filled: %+v", tracks[1])
	}
}
