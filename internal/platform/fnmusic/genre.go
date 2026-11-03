package fnmusic

// Feiniu genre protocol models and list/detail requests. These types stay in
// the provider backend and are translated before reaching shared app code.

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"netease-music-mygo/internal/netease"
)

// Genre is one service-defined music style. The service calls these values
// genres even when they are user-entered tags or source labels.
type Genre struct {
	GUID       string `json:"guid"`
	Name       string `json:"name"`
	CoverID    string `json:"coverId"`
	TrackCount int    `json:"trackCount"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

// CoverURL returns the platform URL for this genre's optional artwork.
func (g Genre) CoverURL() string { return coverURL(g.CoverID) }

// GenrePage is a page of styles and the order applied by the service.
type GenrePage struct {
	List  []Genre `json:"list"`
	Total int     `json:"total"`
	Sort  string  `json:"sort"`
}

const (
	SortGenreNameAsc        = "name,asc"
	SortGenreNameDesc       = "name,desc"
	SortGenreTrackCountAsc  = "trackCount,asc"
	SortGenreTrackCountDesc = "trackCount,desc"
)

// GenreSortValues returns the sort values accepted by the service, in the
// order used by the platform UI.
func GenreSortValues() []string {
	return []string{
		SortGenreNameDesc,
		SortGenreNameAsc,
		SortGenreTrackCountDesc,
		SortGenreTrackCountAsc,
	}
}

// ParseGenreSort normalizes a persisted style-list sort value.
func ParseGenreSort(sort string) string {
	return GenreListSortPolicy.Normalize(sort)
}

// GenresPage returns one page of styles. The service uses one-based pages;
// callers pass the page number exactly as the API expects.
func (c *Client) GenresPage(sort string, page, size int) (GenrePage, error) {
	if page <= 0 || size <= 0 {
		return GenrePage{}, errors.New("fnmusic: invalid genre page")
	}
	q := url.Values{
		"page": {strconv.Itoa(page)},
		"size": {strconv.Itoa(size)},
	}
	if sort != "" {
		q.Set("sort", ParseGenreSort(sort))
	}
	var out GenrePage
	if err := c.call("GET", "/genre/list", q, nil, &out); err != nil {
		return GenrePage{}, err
	}
	return out, nil
}

// Genre returns the metadata for one style.
func (c *Client) Genre(guid string) (Genre, error) {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return Genre{}, errors.New("fnmusic: empty genre guid")
	}
	var out Genre
	if err := c.call("GET", "/genre/detail", url.Values{"guid": {guid}}, nil, &out); err != nil {
		return Genre{}, err
	}
	return out, nil
}

// KindGenre marks a style card and its song list in the shared models.
const KindGenre = "风格"

// GenreID is the stable app ID of a style's card and list.
func GenreID(guid string) int64 { return ID("genre|" + guid) }

// Card returns the style as a shared card, its song count under the name.
func (g Genre) Card() netease.Playlist {
	name := strings.TrimSpace(g.Name)
	if name == "" {
		name = "未命名风格"
	}
	return netease.Playlist{
		PlatformPlaylist: netease.PlatformPlaylist{Ref: g.GUID, Kind: KindGenre},
		ID:               GenreID(g.GUID), Name: name, CoverURL: g.CoverURL(),
		TrackCount: g.TrackCount, UpdateFreq: fmt.Sprintf("%d 首", g.TrackCount),
	}
}
