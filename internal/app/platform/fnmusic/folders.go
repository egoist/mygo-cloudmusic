package fnmusic

import (
	"path"
	"strings"

	"github.com/egoist/mygo/ui"

	backend "netease-music-mygo/internal/platform/fnmusic"
)

// folderViewState is the Feiniu-only state of the folder pages. Each folder
// is a page of its own, at Route{Page: PageFolders, Query: its path}, so back
// and forward walk the folders as they walk any other page. A folder's
// contents are cached by its path and asked for when it is first shown.
type folderViewState struct {
	home *Home

	generation uint64
	roots      []backend.SharedLibrary
	rootsBusy  bool
	rootsErr   error
	pages      map[string]backend.FolderPage
	loading    map[string]bool
	errs       map[string]error
	lists      map[string]*ui.ListState
	sort       string
}

func newFolderViewState(h *Home) *folderViewState {
	f := &folderViewState{home: h, sort: backend.SortFolderTitleAsc}
	f.reset(0)
	return f
}

func (h *Home) folderState() *folderViewState {
	if h == nil {
		return nil
	}
	if h.folders == nil {
		h.folders = newFolderViewState(h)
	}
	return h.folders
}

func (f *folderViewState) reset(gen uint64) {
	if f == nil {
		return
	}
	f.generation = gen
	f.roots, f.rootsBusy, f.rootsErr = nil, false, nil
	f.pages = map[string]backend.FolderPage{}
	f.loading = map[string]bool{}
	f.errs = map[string]error{}
	f.lists = map[string]*ui.ListState{}
}

// syncFolders drops what an earlier session loaded.
func (h *Home) syncFolders() *folderViewState {
	f := h.folderState()
	if f == nil || h.state == nil {
		return f
	}
	if gen := h.state.Generation(); f.generation != gen {
		f.reset(gen)
	}
	return f
}

// listState keeps each folder's scroll position, so going back to a folder
// lands where it was left.
func (f *folderViewState) listState(key string) *ui.ListState {
	if st := f.lists[key]; st != nil {
		return st
	}
	st := &ui.ListState{}
	f.lists[key] = st
	return st
}

func (h *Home) ensureFolderRoots() {
	f := h.syncFolders()
	if f == nil || h.state == nil || !h.state.Connected() || !h.state.FolderViewEnabled() {
		return
	}
	if f.rootsBusy || f.roots != nil || f.rootsErr != nil {
		return
	}
	client := h.state.Client()
	if client == nil {
		return
	}
	f.rootsBusy = true
	gen := f.generation
	go func() {
		roots, err := client.SharedLibraries()
		h.postCurrent(gen, func() {
			f.rootsBusy = false
			if err != nil {
				f.rootsErr = err
				return
			}
			if roots == nil {
				roots = []backend.SharedLibrary{}
			}
			f.roots = roots
		})
	}()
}

func (h *Home) loadFolderPage(folderPath string) {
	f := h.syncFolders()
	if f == nil || h.state == nil || folderPath == "" || f.loading[folderPath] || f.errs[folderPath] != nil {
		return
	}
	if _, ok := f.pages[folderPath]; ok {
		return
	}
	client := h.state.Client()
	if client == nil {
		return
	}
	gen := f.generation
	f.loading[folderPath] = true
	sort := backend.ParseFolderSort(f.sort)
	go func() {
		page, err := client.FolderPage(folderPath, sort)
		h.postCurrent(gen, func() {
			delete(f.loading, folderPath)
			if err != nil {
				f.errs[folderPath] = err
				return
			}
			if page.Path == "" {
				page.Path = folderPath
			}
			f.pages[folderPath] = page
		})
	}()
}

// refreshFolder asks for a folder's contents again, keeping what shows until
// they come.
func (h *Home) refreshFolder(folderPath string) {
	f := h.syncFolders()
	if f == nil {
		return
	}
	if folderPath == "" {
		f.roots, f.rootsErr = nil, nil
		h.ensureFolderRoots()
	} else {
		delete(f.errs, folderPath)
		old, had := f.pages[folderPath]
		delete(f.pages, folderPath)
		h.loadFolderPage(folderPath)
		if had {
			// The old contents stay on screen while the new ones load.
			f.pages[folderPath] = old
			f.loading[folderPath] = true
		}
	}
	h.host.Invalidate()
}

// setFolderSort orders every folder anew; the cached ones are dropped.
func (h *Home) setFolderSort(sort string) {
	f := h.syncFolders()
	if f == nil || f.sort == sort {
		return
	}
	f.sort = sort
	f.pages = map[string]backend.FolderPage{}
	f.errs = map[string]error{}
	h.host.Invalidate()
}

// openFolder shows a folder as a page of its own.
func (h *Home) openFolder(folderPath string) {
	h.host.Navigate(Route{Page: PageFolders, Query: folderPath})
}

// folderRoot is the library a folder is in.
func (f *folderViewState) folderRoot(folderPath string) (backend.SharedLibrary, bool) {
	var best backend.SharedLibrary
	found := false
	for _, root := range f.roots {
		p := strings.TrimRight(root.Path, "/")
		if p == "" {
			continue
		}
		if (folderPath == p || strings.HasPrefix(folderPath, p+"/")) && len(p) > len(strings.TrimRight(best.Path, "/")) {
			best, found = root, true
		}
	}
	return best, found
}

func rootName(root backend.SharedLibrary) string {
	if name := strings.TrimSpace(root.Name); name != "" {
		return name
	}
	return path.Base(strings.TrimRight(root.Path, "/"))
}

// folderCrumb is one folder over the one shown, from its library down.
type folderCrumb struct{ Name, Path string }

// folderCrumbs are the folders over folderPath, its library first.
func (f *folderViewState) folderCrumbs(folderPath string) []folderCrumb {
	root, ok := f.folderRoot(folderPath)
	if !ok {
		return nil
	}
	base := strings.TrimRight(root.Path, "/")
	if folderPath == base {
		return nil
	}
	crumbs := []folderCrumb{{Name: rootName(root), Path: base}}
	rest := strings.Split(strings.Trim(strings.TrimPrefix(folderPath, base), "/"), "/")
	at := base
	for _, part := range rest[:len(rest)-1] {
		at += "/" + part
		crumbs = append(crumbs, folderCrumb{Name: part, Path: at})
	}
	return crumbs
}

func folderSortLabel(sort string) string {
	switch sort {
	case backend.SortFolderTitleDesc:
		return "名称（Z-A）"
	case backend.SortFolderCreatedAsc:
		return "最早添加"
	case backend.SortFolderCreatedDesc:
		return "最近添加"
	case backend.SortFolderUpdatedAsc:
		return "最早修改"
	case backend.SortFolderUpdatedDesc:
		return "最近修改"
	default:
		return "名称（A-Z）"
	}
}
