package netease

import (
	"net/url"
	"strconv"
)

// SearchType is what a search looks for.
type SearchType int

const (
	SearchSongs     SearchType = 1
	SearchAlbums    SearchType = 10
	SearchArtists   SearchType = 100
	SearchPlaylists SearchType = 1000
	SearchUsers     SearchType = 1002
	SearchPodcasts  SearchType = 1009
)

// SearchResult is a page of what a search found, of the one type it
// looked for, and how many it found in all.
type SearchResult struct {
	Songs     []Track
	Albums    []AlbumProfile
	Artists   []ArtistProfile
	Playlists []Playlist
	Users     []User
	Podcasts  []Podcast
	Total     int
}

// Len is how many the page has.
func (r *SearchResult) Len() int {
	return len(r.Songs) + len(r.Albums) + len(r.Artists) + len(r.Playlists) + len(r.Users) + len(r.Podcasts)
}

// SearchPage returns limit of what matches a query, of a type, from offset.
func SearchPage(query string, kind SearchType, limit, offset int) (SearchResult, error) {
	var res struct {
		Result struct {
			Songs         []Track         `json:"songs"`
			SongCount     int             `json:"songCount"`
			Albums        []AlbumProfile  `json:"albums"`
			AlbumCount    int             `json:"albumCount"`
			Artists       []ArtistProfile `json:"artists"`
			ArtistCount   int             `json:"artistCount"`
			Playlists     []Playlist      `json:"playlists"`
			PlaylistCount int             `json:"playlistCount"`
			Users         []User          `json:"userprofiles"`
			UserCount     int             `json:"userprofileCount"`
			Podcasts      []Podcast       `json:"djRadios"`
			PodcastCount  int             `json:"djRadiosCount"`
		} `json:"result"`
	}
	q := url.Values{"s": {query}, "type": {strconv.Itoa(int(kind))}, "limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}, "total": {"true"}}
	if err := get("/api/cloudsearch/pc", q, &res); err != nil {
		return SearchResult{}, err
	}
	r := res.Result
	// Search marks whole catalogues as taken down that the listener may
	// play, as YOASOBI's for a subscriber: the songs' details say right.
	detailPrivileges(r.Songs)
	return SearchResult{
		Songs: r.Songs, Albums: r.Albums, Artists: r.Artists, Playlists: r.Playlists, Users: r.Users, Podcasts: r.Podcasts,
		Total: r.SongCount + r.AlbumCount + r.ArtistCount + r.PlaylistCount + r.UserCount + r.PodcastCount,
	}, nil
}

// Search returns the songs matching a query.
func Search(query string, limit int) ([]Track, error) {
	res, err := SearchPage(query, SearchSongs, limit, 0)
	return res.Songs, err
}

// AlbumProfile is an album as its page shows it.
type AlbumProfile struct {
	Album
	Artists     []Artist `json:"artists"`
	PublishTime int64    `json:"publishTime"` // Unix milliseconds
	Size        int      `json:"size"`        // how many songs it has
	Description string   `json:"description"`
	Songs       []Track  `json:"-"`
}

// ArtistNames joins the album's artists' names, as its credits read.
func (al *AlbumProfile) ArtistNames() string {
	return (&Track{Artists: al.Artists}).ArtistNames()
}

// AlbumDetail returns an album with its songs.
func AlbumDetail(id int64) (*AlbumProfile, error) {
	var res struct {
		Album AlbumProfile `json:"album"`
		Songs []Track      `json:"songs"`
	}
	if err := get("/api/v1/album/"+strconv.FormatInt(id, 10), nil, &res); err != nil {
		return nil, err
	}
	al := &res.Album
	al.Songs = res.Songs
	detailPrivileges(al.Songs)
	return al, nil
}

// ArtistAlbums returns limit of an artist's albums from offset, the newest
// first, and whether they have more.
func ArtistAlbums(id int64, limit, offset int) ([]AlbumProfile, bool, error) {
	var res struct {
		Albums []AlbumProfile `json:"hotAlbums"`
		More   bool           `json:"more"`
	}
	q := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
	err := get("/api/artist/albums/"+strconv.FormatInt(id, 10), q, &res)
	return res.Albums, res.More, err
}

// User is someone on NetEase, as their page shows them.
type User struct {
	UserID    int64  `json:"userId"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
	Signature string `json:"signature"`
	Followeds int    `json:"followeds"` // their followers
	Follows   int    `json:"follows"`
}

// UserDetail returns who a user is.
func UserDetail(id int64) (*User, error) {
	var res struct {
		Profile User `json:"profile"`
	}
	if err := get("/api/v1/user/detail/"+strconv.FormatInt(id, 10), nil, &res); err != nil {
		return nil, err
	}
	return &res.Profile, nil
}

// legacyTrack is a song in the shape older endpoints still send, as search
// suggestions, events and podcast episodes do.
type legacyTrack struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Artists  []Artist `json:"artists"`
	Album    Album    `json:"album"`
	Duration int64    `json:"duration"`
}

func (t legacyTrack) track() Track {
	return Track{ID: t.ID, Name: t.Name, Artists: t.Artists, Album: t.Album, Duration: t.Duration}
}

// Append adds the next page of a search to r.
func (r *SearchResult) Append(next SearchResult) {
	r.Songs = append(r.Songs, next.Songs...)
	r.Albums = append(r.Albums, next.Albums...)
	r.Artists = append(r.Artists, next.Artists...)
	r.Playlists = append(r.Playlists, next.Playlists...)
	r.Users = append(r.Users, next.Users...)
	r.Podcasts = append(r.Podcasts, next.Podcasts...)
	r.Total = next.Total
}
