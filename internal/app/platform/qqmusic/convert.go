package qqmusic

import (
	"strconv"
	"time"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform/paging"
	backend "netease-music-mygo/internal/platform/qqmusic"
)

func track(t backend.Track) netease.Track {
	artists := make([]netease.Artist, len(t.Artists))
	for i, a := range t.Artists {
		artists[i] = netease.Artist{ID: a.ID, Name: a.Name}
	}
	ref := t.MID
	if ref == "" {
		ref = strconv.FormatInt(t.ID, 10)
	}
	out := netease.Track{
		PlatformTrack: netease.PlatformTrack{Source: Source, Ref: ref},
		ID:            t.ID, Name: t.Name, Artists: artists,
		Album:    netease.Album{ID: t.Album.ID, Name: t.Album.Name, PicURL: t.Album.PicURL},
		Duration: t.Duration, Fee: t.Fee, Alias: t.Alias, Reason: t.Reason,
		ProgramID: t.ProgramID, PodcastID: t.PodcastID, Description: t.Description,
	}
	if t.Privilege != nil {
		out.Privilege = &netease.Privilege{ID: t.Privilege.ID, St: t.Privilege.St, Pl: t.Privilege.Pl}
	}
	return out
}

func tracks(in []backend.Track) []netease.Track {
	out := make([]netease.Track, len(in))
	for i := range in {
		out[i] = track(in[i])
	}
	return out
}

func playlist(p backend.Playlist) netease.Playlist {
	out := netease.Playlist{
		PlatformPlaylist: netease.PlatformPlaylist{Ref: strconv.FormatInt(p.ID, 10), Kind: "playlist"},
		ID:               p.ID, Name: p.Name, CoverURL: p.CoverURL, PicURL: p.PicURL,
		Description: p.Description, PlayCount: p.PlayCount, TrackCount: p.TrackCount,
		UpdateFreq: p.UpdateFreq, SpecialType: p.SpecialType, Privacy: p.Privacy,
		Tracks: tracks(p.Tracks),
	}
	out.Creator.UserID, out.Creator.Nickname = p.Creator.UserID, p.Creator.Nickname
	for _, line := range p.Preview {
		out.Preview = append(out.Preview, netease.ChartLine{First: line.First, Second: line.Second})
	}
	return out
}

func trackIDs(in []netease.Track) []int64 {
	out := make([]int64, len(in))
	for i, tr := range in {
		out[i] = tr.ID
	}
	return out
}

func playlistCover(p netease.Playlist) string {
	if p.CoverURL != "" {
		return p.CoverURL
	}
	return p.PicURL
}

func playlists(in []backend.Playlist) []netease.Playlist {
	out := make([]netease.Playlist, len(in))
	for i := range in {
		out[i] = playlist(in[i])
	}
	return out
}

func albumCard(a backend.AlbumProfile) netease.Playlist {
	sub := a.ArtistNames()
	if a.PublishTime > 0 {
		year := time.UnixMilli(a.PublishTime).Format("2006")
		if sub != "" {
			sub += " · "
		}
		sub += year
	}
	return netease.Playlist{
		PlatformPlaylist: netease.PlatformPlaylist{Ref: a.MID, Kind: "album", Sub: a.ArtistNames()},
		ID:               a.ID, Name: a.Name, CoverURL: a.PicURL, TrackCount: a.Size,
		UpdateFreq: sub,
	}
}

func albumCards(in []backend.AlbumProfile) []netease.Playlist {
	out := make([]netease.Playlist, len(in))
	for i := range in {
		out[i] = albumCard(in[i])
	}
	return out
}

func artistCard(a backend.ArtistProfile) netease.Playlist {
	cover := a.PicURL
	if cover == "" {
		cover = a.Avatar
	}
	return netease.Playlist{
		PlatformPlaylist: netease.PlatformPlaylist{Ref: a.MID, Kind: "artist"},
		ID:               a.ID, Name: a.Name, CoverURL: cover, TrackCount: a.MusicSize,
		UpdateFreq: strconv.Itoa(a.MusicSize) + " 首",
	}
}

func artistCards(in []backend.ArtistProfile) []netease.Playlist {
	out := make([]netease.Playlist, len(in))
	for i := range in {
		out[i] = artistCard(in[i])
	}
	return out
}

func qqComments(in *backend.Comments) *netease.Comments {
	if in == nil {
		return &netease.Comments{}
	}
	return &netease.Comments{Hot: qqCommentList(in.Hot), Latest: qqCommentList(in.Latest), Total: in.Total, More: in.More}
}

func qqCommentList(in []backend.Comment) []netease.Comment {
	if len(in) == 0 {
		return nil
	}
	out := make([]netease.Comment, len(in))
	for i, cm := range in {
		out[i] = netease.Comment{
			ID:         cm.ID,
			Content:    cm.Content,
			Time:       cm.Time,
			LikedCount: cm.LikedCount,
			Liked:      cm.Liked,
		}
		out[i].User.ID = cm.User.ID
		out[i].User.Nickname = cm.User.Nickname
		out[i].User.AvatarURL = cm.User.AvatarURL
		out[i].IPLocation.Location = cm.IPLocation.Location
		for _, r := range cm.BeReplied {
			out[i].BeReplied = append(out[i].BeReplied, struct {
				User struct {
					Nickname string `json:"nickname"`
				} `json:"user"`
				Content string `json:"content"`
			}{User: struct {
				Nickname string `json:"nickname"`
			}{Nickname: r.User.Nickname}, Content: r.Content})
		}
	}
	return out
}

func staticTracks(in []netease.Track) *appplatform.TrackList {
	total := len(in)
	return &appplatform.TrackList{
		Total: func() int { return total },
		Len:   func() int { return total },
		At: func(i int) *netease.Track {
			if i < 0 || i >= total {
				return nil
			}
			return &in[i]
		},
		PageSize: max(total, 1),
	}
}

// lazyTracks wraps a pager as the shared list, which loads pages as rows
// come into view.
func lazyTracks(p *paging.Pager[netease.Track], pageSize int) *appplatform.TrackList {
	return &appplatform.TrackList{
		Total: p.Total, Len: p.Len, At: p.At, Around: p.Around, When: p.When,
		Retry: p.Retry, Failed: func() bool { return p.FailedAt(0) },
		Error: p.Error, PageSize: pageSize,
	}
}

func staticCards(in []netease.Playlist) *paging.Pager[netease.Playlist] {
	p := paging.New[netease.Playlist](max(len(in), 1), 0, nil, nil)
	p.Land(0, in, len(in))
	return p
}
