package netease

import (
	"context"
	"errors"
	"strconv"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
)

// NetEase provider adapter.
//
// Keep the upstream NetEase implementation as the default source of
// behavior. This package only translates it to the shared provider seam and
// must not introduce provider-specific logic into the root app.

// Provider is the default music platform used by the app.
type Provider struct{}

var _ platform.Provider = Provider{}

// New returns the stateless upstream provider.
func New() Provider { return Provider{} }

func (Provider) ID() platform.ID { return platform.NeteaseID }
func (Provider) Name() string    { return "网易云音乐" }

func check(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// Page serves playlist tracks from the existing playlist detail endpoint.
// Other list types remain unsupported until their upstream data source has a
// stable page contract.
func (Provider) Page(ctx context.Context, req platform.PageRequest) (platform.Page, error) {
	if err := check(ctx); err != nil {
		return platform.Page{}, err
	}
	if req.Page < 0 || req.Size <= 0 {
		return platform.Page{}, errors.New("platform/netease: invalid page")
	}
	if req.Kind != platform.ListPlaylist {
		return platform.Page{}, platform.ErrUnsupported
	}
	id, err := strconv.ParseInt(req.Ref, 10, 64)
	if err != nil || id == 0 {
		return platform.Page{}, errors.New("platform/netease: invalid playlist")
	}
	pl, err := netease.PlaylistDetail(id)
	if err != nil {
		return platform.Page{}, err
	}
	start := req.Page * req.Size
	total := max(len(pl.Tracks), len(pl.TrackIDs))
	if start >= len(pl.Tracks) {
		return platform.Page{Total: total}, nil
	}
	end := min(start+req.Size, len(pl.Tracks))
	return platform.Page{Tracks: pl.Tracks[start:end], Total: total}, nil
}

func (Provider) Search(ctx context.Context, query string, kind platform.SearchKind, page, size int) (platform.Page, error) {
	if err := check(ctx); err != nil {
		return platform.Page{}, err
	}
	if page < 0 || size <= 0 {
		return platform.Page{}, errors.New("platform/netease: invalid page")
	}
	var searchKind netease.SearchType
	switch kind {
	case platform.SearchSongs:
		searchKind = netease.SearchSongs
	case platform.SearchAlbums:
		searchKind = netease.SearchAlbums
	case platform.SearchArtists:
		searchKind = netease.SearchArtists
	default:
		return platform.Page{}, platform.ErrUnsupported
	}
	res, err := netease.SearchPage(query, searchKind, size, page*size)
	if err != nil {
		return platform.Page{}, err
	}
	return platform.Page{Tracks: res.Songs, Playlists: res.Playlists, Total: res.Total}, nil
}

func (Provider) Stream(ctx context.Context, tr *netease.Track) (*netease.Stream, error) {
	if err := check(ctx); err != nil {
		return nil, err
	}
	if tr == nil {
		return nil, errors.New("platform/netease: nil track")
	}
	return netease.SongURL(tr.ID, "exhigh")
}

func (Provider) Lyrics(ctx context.Context, tr *netease.Track) (platform.Lyrics, error) {
	if err := check(ctx); err != nil {
		return platform.Lyrics{}, err
	}
	if tr == nil {
		return platform.Lyrics{}, errors.New("platform/netease: nil track")
	}
	lrc, translation, yrc, err := netease.Lyrics(tr.ID)
	return platform.Lyrics{LRC: lrc, Translation: translation, YRC: yrc}, err
}

func (Provider) Cover(ctx context.Context, raw string, size int) ([]byte, error) {
	if err := check(ctx); err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, errors.New("platform/netease: empty cover")
	}
	return netease.Fetch(netease.Sized(raw, size))
}
