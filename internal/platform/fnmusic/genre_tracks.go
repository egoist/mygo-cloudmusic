package fnmusic

// Feiniu style-song paging and explicit full-load playback requests.

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"netease-music-mygo/internal/netease"
)

// GenreTrack keeps the file metadata a style page displays beside its shared
// track model. It deliberately does not add style-specific fields to
// netease.Track.
type GenreTrack struct {
	Track     netease.Track
	Size      int64
	CreatedAt int64
	UpdatedAt int64
}

// GenreTrackPage is one page returned by the style-song endpoint.
type GenreTrackPage struct {
	List  []GenreTrack
	Total int
	Sort  string
}

const (
	SortGenreTrackCreatedAsc  = "createdAt,asc"
	SortGenreTrackCreatedDesc = "createdAt,desc"
	SortGenreTrackUpdatedAsc  = "updatedAt,asc"
	SortGenreTrackUpdatedDesc = "updatedAt,desc"
	SortGenreTrackTitleAsc    = "title,asc"
	SortGenreTrackTitleDesc   = "title,desc"
)

// ParseGenreTrackSort normalizes a persisted style-song sort value.
func ParseGenreTrackSort(sort string) string {
	return GenreTrackSortPolicy.Normalize(sort)
}

// GenreTrackSortValues returns the service-supported style-song sorts.
func GenreTrackSortValues() []string {
	return []string{
		SortGenreTrackCreatedDesc,
		SortGenreTrackCreatedAsc,
		SortValue(SortFieldArtistName, SortOrderDesc),
		SortValue(SortFieldArtistName, SortOrderAsc),
	}
}

// GenreTracksPage returns one page of songs belonging to a style. The service
// returns the array as data.list, unlike folder-view/list's data.folder.tracks.
// A size of -1 is accepted for the explicit play-all operation.
func (c *Client) GenreTracksPage(guid, sort string, page, size int) (GenreTrackPage, error) {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return GenreTrackPage{}, errors.New("fnmusic: empty genre guid")
	}
	if page <= 0 || (size == 0 || size < -1) {
		return GenreTrackPage{}, errors.New("fnmusic: invalid genre track page")
	}
	q := url.Values{
		"page":      {strconv.Itoa(page)},
		"size":      {strconv.Itoa(size)},
		"genreGUID": {guid},
	}
	if sort != "" {
		q.Set("sort", ParseGenreTrackSort(sort))
	}
	var raw struct {
		List  []rawTrack `json:"list"`
		Total int        `json:"total"`
		Sort  string     `json:"sort"`
	}
	if err := c.call("GET", "/track/genre-detail/list", q, nil, &raw); err != nil {
		return GenreTrackPage{}, err
	}
	out := GenreTrackPage{Total: raw.Total, Sort: raw.Sort}
	if out.Sort == "" {
		out.Sort = ParseGenreTrackSort(sort)
	}
	out.List = make([]GenreTrack, 0, len(raw.List))
	for _, item := range raw.List {
		track, ok := item.track()
		if !ok {
			continue
		}
		out.List = append(out.List, GenreTrack{
			Track: track, Size: item.AudioSpec.Size,
			CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		})
	}
	return out, nil
}

// AllGenreTracks resolves every song for an explicit play-all action. The
// service owns the query limit and returns the result in one response.
func (c *Client) AllGenreTracks(guid, sort string) ([]netease.Track, error) {
	page, err := c.GenreTracksPage(guid, sort, 1, -1)
	if err != nil {
		return nil, err
	}
	out := make([]netease.Track, 0, len(page.List))
	for _, item := range page.List {
		out = append(out, item.Track)
	}
	return out, nil
}
