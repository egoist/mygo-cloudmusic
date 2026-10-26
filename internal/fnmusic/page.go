package fnmusic

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"netease-music-mygo/internal/netease"
)

// The service lists everything a page at a time, in an order the caller
// names: a page asks for 1-based page number and size, and says how many
// there are in all, which is what a list loaded as it is scrolled needs.

// page is a page of a list of songs.
type page struct {
	List  []rawTrack `json:"list"`
	Total int        `json:"total"`
}

// The orders the lists can be asked for in: what the service takes, and
// echoes back as the one it used (it falls back to its own for any other).
const (
	SortRecent = "createdAt,desc" // the library: newest first
	SortTitle  = "title,asc"

	AlbumsRecent = "newTrackAddedAt,desc"
	AlbumsName   = "name,asc"
	AlbumsSize   = "trackCount,desc" // most songs first, singles last

	ArtistsSize   = "trackCount,desc"
	ArtistsName   = "name,asc"
	ArtistsAlbums = "albumCount,desc"
)

// tracks reads a page of songs from path, and how many the list has.
func (c *Client) tracks(path string, q url.Values, pg, size int) ([]netease.Track, int, error) {
	v := url.Values{"page": {strconv.Itoa(pg)}, "size": {strconv.Itoa(size)}}
	for k, vs := range q {
		v[k] = vs
	}
	var p page
	if err := c.call("GET", path, v, nil, &p); err != nil {
		return nil, 0, err
	}
	out := make([]netease.Track, 0, len(p.List))
	for _, r := range p.List {
		if t, ok := r.track(); ok {
			out = append(out, t)
		}
	}
	return out, p.Total, nil
}

// LibraryPage returns a page of the library's songs.
func (c *Client) LibraryPage(sort string, pg, size int) ([]netease.Track, int, error) {
	return c.tracks("/track/list", url.Values{"sort": {sort}}, pg, size)
}

// FavoritesPage returns a page of the songs the listener likes.
func (c *Client) FavoritesPage(pg, size int) ([]netease.Track, int, error) {
	return c.tracks("/favorite-track/list", nil, pg, size)
}

// PlaylistPage returns a page of the songs of a playlist.
func (c *Client) PlaylistPage(ref string, pg, size int) ([]netease.Track, int, error) {
	return c.tracks("/track/playlist-detail/list", url.Values{"playlistGUID": {ref}}, pg, size)
}

// AlbumPage returns a page of the songs of an album, in the order of its
// disc.
func (c *Client) AlbumPage(ref string, pg, size int) ([]netease.Track, int, error) {
	return c.tracks("/track/album-detail/list", url.Values{"albumGUID": {ref}}, pg, size)
}

// ArtistPage returns a page of the songs of an artist.
func (c *Client) ArtistPage(ref string, pg, size int) ([]netease.Track, int, error) {
	return c.tracks("/track/artist-detail/list", url.Values{"artistGUID": {ref}}, pg, size)
}

// SearchPage returns a page of the songs the service finds for q, best
// first: it searches, as the web player does.
func (c *Client) SearchPage(q string, pg, size int) ([]netease.Track, int, error) {
	return c.tracks("/search/track", url.Values{"q": {q}}, pg, size)
}

// Kinds of what a card is of, which Playlist.Kind says.
const (
	KindAlbum  = "专辑"
	KindArtist = "歌手"
)

// AlbumID and ArtistID are the app's numbers for an album or an artist,
// from their GUIDs: apart from each other's and from playlists'.
func AlbumID(guid string) int64  { return ID("album|" + guid) }
func ArtistID(guid string) int64 { return ID("artist|" + guid) }

// names joins the names of people as credits read.
func joinNames(list []struct {
	Name string `json:"name"`
}) string {
	var out []string
	for _, a := range list {
		if s := strings.TrimSpace(a.Name); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, ", ")
}

// AlbumsPage returns a page of the albums, as cards: the album's name, its
// artist and how many songs it has.
func (c *Client) AlbumsPage(path, sort, q string, pg, size int) ([]netease.Playlist, int, error) {
	v := url.Values{"page": {strconv.Itoa(pg)}, "size": {strconv.Itoa(size)}}
	if sort != "" {
		v.Set("sort", sort)
	}
	if q != "" {
		v.Set("q", q)
	}
	var res struct {
		List []struct {
			GUID       string `json:"guid"`
			Name       string `json:"name"`
			CoverID    string `json:"coverId"`
			TrackCount int    `json:"trackCount"`
			Artists    []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"list"`
		Total int `json:"total"`
	}
	if err := c.call("GET", path, v, nil, &res); err != nil {
		return nil, 0, err
	}
	out := make([]netease.Playlist, 0, len(res.List))
	for _, a := range res.List {
		if a.GUID == "" {
			continue
		}
		sub := joinNames(a.Artists)
		out = append(out, netease.Playlist{
			ID: AlbumID(a.GUID), Ref: a.GUID, Kind: KindAlbum, Name: a.Name, CoverURL: coverURL(a.CoverID),
			TrackCount: a.TrackCount, Sub: sub, Creator: struct {
				UserID   int64  `json:"userId"`
				Nickname string `json:"nickname"`
			}{Nickname: sub},
		})
	}
	return out, res.Total, nil
}

// ArtistsPage returns a page of the artists, as cards: the artist's name
// and how many albums and songs they have.
func (c *Client) ArtistsPage(path, sort, q string, pg, size int) ([]netease.Playlist, int, error) {
	v := url.Values{"page": {strconv.Itoa(pg)}, "size": {strconv.Itoa(size)}}
	if sort != "" {
		v.Set("sort", sort)
	}
	if q != "" {
		v.Set("q", q)
	}
	var res struct {
		List []struct {
			GUID       string `json:"guid"`
			Name       string `json:"name"`
			CoverID    string `json:"coverId"`
			TrackCount int    `json:"trackCount"`
			AlbumCount int    `json:"albumCount"`
		} `json:"list"`
		Total int `json:"total"`
	}
	if err := c.call("GET", path, v, nil, &res); err != nil {
		return nil, 0, err
	}
	out := make([]netease.Playlist, 0, len(res.List))
	for _, a := range res.List {
		if a.GUID == "" {
			continue
		}
		sub := strconv.Itoa(a.TrackCount) + " 首歌"
		if a.AlbumCount > 0 {
			sub = strconv.Itoa(a.AlbumCount) + " 张专辑 · " + sub
		}
		out = append(out, netease.Playlist{
			ID: ArtistID(a.GUID), Ref: a.GUID, Kind: KindArtist, Name: a.Name, CoverURL: coverURL(a.CoverID),
			TrackCount: a.TrackCount, Sub: sub,
		})
	}
	return out, res.Total, nil
}

// The addresses of the lists of cards.
const (
	AlbumsPath        = "/album/list"
	ArtistsPath       = "/artist/list"
	SearchAlbumsPath  = "/search/album"
	SearchArtistsPath = "/search/artist"
)

// RecentPage returns a page of the songs played, the last first: the
// listener's play history on the service, which the web player and the other
// apps of the NAS share.
func (c *Client) RecentPage(pg, size int) ([]netease.Track, int, error) {
	return c.tracks("/play-history/list", nil, pg, size)
}

// RemoveFromHistory takes songs out of the play history. They stay in the
// library.
func (c *Client) RemoveFromHistory(songs []string) error {
	return c.call("POST", "/play-history/delete", nil, map[string]any{"trackGUIDs": songs}, nil)
}

// ReportPlay tells the service a song was played, as every player of it
// does as a song starts: it goes in the play history, in front, where one
// that was in already is moved to. The service does not know what a roam
// plays, nor what is played by the address of a file, so it is told.
func (c *Client) ReportPlay(song string) error {
	event := map[string]any{
		"eventType":  "track_play",
		"occurredAt": time.Now().UnixMilli(),
		"payload":    map[string]string{"trackGUID": song},
	}
	return c.call("POST", "/event/report", nil, map[string]any{"events": []any{event}}, nil)
}
