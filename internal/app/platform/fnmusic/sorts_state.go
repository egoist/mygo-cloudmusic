package fnmusic

import backend "netease-music-mygo/internal/platform/fnmusic"

const (
	sortKeyLibrary      = "library"
	sortKeyFavorites    = "favorites"
	sortKeyAlbums       = "albums"
	sortKeyAlbumDetail  = "albumDetail"
	sortKeyArtists      = "artists"
	sortKeyArtistTracks = "artistTracks"
	sortKeyArtistAlbums = "artistAlbums"
	sortKeyGenres       = "genres"
	sortKeyGenreTracks  = "genreTracks"
	sortKeyFolders      = "folders"
)

func (h *Home) restoreSorts() {
	if h == nil || h.state == nil || h.runtime == nil {
		return
	}
	prefs := h.runtime.Sorts()
	c := h.state.Catalog()
	c.LibrarySort = backend.LibrarySortPolicy.Normalize(prefs[sortKeyLibrary])
	c.FavoriteSort = backend.FavoriteSortPolicy.Normalize(prefs[sortKeyFavorites])
	c.AlbumSort = backend.AlbumListSortPolicy.Normalize(prefs[sortKeyAlbums])
	c.AlbumDetailSort = backend.AlbumTrackSortPolicy.Normalize(prefs[sortKeyAlbumDetail])
	c.ArtistSort = backend.ArtistListSortPolicy.Normalize(prefs[sortKeyArtists])
	c.ArtistTrackSort = backend.ArtistTrackSortPolicy.Normalize(prefs[sortKeyArtistTracks])
	c.ArtistAlbumSort = backend.ArtistAlbumSortPolicy.Normalize(prefs[sortKeyArtistAlbums])
	c.GenreSort = backend.GenreListSortPolicy.Normalize(prefs[sortKeyGenres])
	c.GenreTrackSort = backend.GenreTrackSortPolicy.Normalize(prefs[sortKeyGenreTracks])
	if h.folders != nil {
		h.folders.sort = backend.ParseFolderSort(prefs[sortKeyFolders])
	}
	h.sortGen = h.state.Generation()
}

func (h *Home) syncSorts() {
	if h == nil || h.state == nil || h.sortGen == h.state.Generation() {
		return
	}
	h.restoreSorts()
}

func (h *Home) rememberSort(key, value string) {
	if h == nil || h.runtime == nil {
		return
	}
	h.runtime.SetSort(key, value)
}
