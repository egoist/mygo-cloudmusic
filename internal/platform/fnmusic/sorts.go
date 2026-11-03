package fnmusic

import "strings"

// Sort orders are sent as "<field>,<asc|desc>". The policies here describe
// which fields each endpoint accepts and the order the web player uses when
// the caller has no preference yet.

const (
	SortOrderAsc  = "asc"
	SortOrderDesc = "desc"

	SortFieldTitle        = "title"
	SortFieldCreatedAt    = "createdAt"
	SortFieldUpdatedAt    = "updatedAt"
	SortFieldFavoriteAt   = "favoriteAt"
	SortFieldTrackNo      = "trackNo"
	SortFieldReleaseYear  = "releaseYear"
	SortFieldName         = "name"
	SortFieldArtistName   = "artistName"
	SortFieldTrackCount   = "trackCount"
	SortFieldAlbumCount   = "albumCount"
	SortFieldAlbumAddedAt = "newTrackAddedAt"
)

type SortPolicy struct {
	Default string
	Fields  []string
}

func SortValue(field, order string) string { return field + "," + order }

func SplitSort(sort string) (field, order string, ok bool) {
	parts := strings.Split(strings.TrimSpace(sort), ",")
	if len(parts) != 2 || (parts[1] != SortOrderAsc && parts[1] != SortOrderDesc) {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (p SortPolicy) Normalize(sort string) string {
	field, _, ok := SplitSort(sort)
	if !ok {
		return p.Default
	}
	for _, allowed := range p.Fields {
		if field == allowed {
			return strings.TrimSpace(sort)
		}
	}
	return p.Default
}

var (
	FavoriteSortPolicy = SortPolicy{Default: SortValue(SortFieldFavoriteAt, SortOrderDesc), Fields: []string{
		SortFieldFavoriteAt, SortFieldTitle,
	}}
	LibrarySortPolicy = SortPolicy{Default: SortValue(SortFieldCreatedAt, SortOrderDesc), Fields: []string{
		SortFieldCreatedAt, SortFieldTitle,
	}}
	AlbumListSortPolicy = SortPolicy{Default: SortValue(SortFieldAlbumAddedAt, SortOrderDesc), Fields: []string{
		SortFieldAlbumAddedAt, SortFieldReleaseYear, SortFieldName, SortFieldArtistName, SortFieldTrackCount,
	}}
	AlbumTrackSortPolicy = SortPolicy{Default: SortValue(SortFieldTrackNo, SortOrderAsc), Fields: []string{
		SortFieldTrackNo, SortFieldTitle,
	}}
	ArtistListSortPolicy = SortPolicy{Default: SortValue(SortFieldName, SortOrderDesc), Fields: []string{
		SortFieldName, SortFieldTrackCount, SortFieldAlbumCount,
	}}
	ArtistTrackSortPolicy = SortPolicy{Default: SortValue(SortFieldCreatedAt, SortOrderDesc), Fields: []string{
		SortFieldCreatedAt,
	}}
	ArtistAlbumSortPolicy = SortPolicy{Default: SortValue(SortFieldAlbumAddedAt, SortOrderDesc), Fields: []string{
		SortFieldAlbumAddedAt, SortFieldReleaseYear, SortFieldName, SortFieldTrackCount,
	}}
	GenreListSortPolicy = SortPolicy{Default: SortValue(SortFieldName, SortOrderDesc), Fields: []string{
		SortFieldName, SortFieldTrackCount,
	}}
	GenreTrackSortPolicy = SortPolicy{Default: SortValue(SortFieldCreatedAt, SortOrderDesc), Fields: []string{
		SortFieldCreatedAt, SortFieldArtistName,
	}}
)
