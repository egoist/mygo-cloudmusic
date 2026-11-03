package qqmusic

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

type Artist struct {
	ID   int64  `json:"id"`
	MID  string `json:"mid,omitempty"`
	Name string `json:"name"`
}
type Album struct {
	ID     int64  `json:"id"`
	MID    string `json:"mid,omitempty"`
	Name   string `json:"name"`
	PicURL string `json:"picUrl"`
}
type Track struct {
	ID          int64      `json:"id"`
	MID         string     `json:"mid,omitempty"`
	MediaMID    string     `json:"mediaMid,omitempty"`
	Size128     int64      `json:"size128,omitempty"`
	Size320     int64      `json:"size320,omitempty"`
	SizeFLAC    int64      `json:"sizeFlac,omitempty"`
	Name        string     `json:"name"`
	Artists     []Artist   `json:"ar"`
	Album       Album      `json:"al"`
	Duration    int64      `json:"dt"`
	Fee         int        `json:"fee"`
	Alias       []string   `json:"alia"`
	Reason      string     `json:"reason"`
	Privilege   *Privilege `json:"privilege"`
	ProgramID   int64      `json:"programId,omitempty"`
	PodcastID   int64      `json:"podcastId,omitempty"`
	Description string     `json:"description,omitempty"`
	Volume      *Loudness  `json:"volume,omitempty"`
	// NeedsDetails keeps a sparse search result from supplying an incomplete
	// playback filename, including after restoring a saved queue.
	NeedsDetails bool `json:"needsDetails,omitempty"`
}

// Loudness is QQ's measured gain in dB, sample peak and loudness range for
// the standard master. The source-specific vf groups are not interchangeable.
type Loudness struct {
	Gain float64 `json:"gain"`
	Peak float64 `json:"peak"`
	LRA  float64 `json:"lra"`
}
type Privilege struct {
	ID int64 `json:"id"`
	St int   `json:"st"`
	Pl int   `json:"pl"`
}

func (t *Track) ArtistNames() string {
	names := make([]string, len(t.Artists))
	for i, a := range t.Artists {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}
func (t *Track) Playable() bool {
	return t.Privilege == nil || t.Privilege.St >= 0 && t.Privilege.Pl > 0
}
func (t *Track) Length() time.Duration { return time.Duration(t.Duration) * time.Millisecond }
func (t *Track) URL() string {
	if t.MID != "" {
		return "https://y.qq.com/n/ryqq/songDetail/" + t.MID
	}
	return "https://y.qq.com/n/ryqq/songDetail/" + strconv.FormatInt(t.ID, 10)
}
func (t *Track) Thread() string {
	if t.ProgramID > 0 {
		return "A_DJ_1_" + strconv.FormatInt(t.ProgramID, 10)
	}
	return strconv.FormatInt(t.ID, 10)
}

type Playlist struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	CoverURL    string  `json:"coverImgUrl"`
	PicURL      string  `json:"picUrl"`
	Description string  `json:"description"`
	PlayCount   float64 `json:"playCount"`
	TrackCount  int     `json:"trackCount"`
	UpdateFreq  string  `json:"updateFrequency"`
	SpecialType int     `json:"specialType"`
	Privacy     int     `json:"privacy"`
	Creator     struct {
		UserID   int64  `json:"userId"`
		Nickname string `json:"nickname"`
	} `json:"creator"`
	Tracks   []Track `json:"tracks"`
	TrackIDs []struct {
		ID int64 `json:"id"`
	} `json:"trackIds"`
	Preview []ChartLine `json:"-"`
}
type ChartLine struct {
	First  string `json:"first"`
	Second string `json:"second"`
}

func (p *Playlist) Cover() string {
	if p.CoverURL != "" {
		return p.CoverURL
	}
	return p.PicURL
}

type ArtistProfile struct {
	Artist
	Avatar    string   `json:"img1v1Url"`
	PicURL    string   `json:"picUrl"`
	Alias     []string `json:"alias"`
	Trans     string   `json:"trans"`
	BriefDesc string   `json:"briefDesc"`
	MusicSize int      `json:"musicSize"`
	AlbumSize int      `json:"albumSize"`
	Followed  bool     `json:"followed"`
	AccountID int64    `json:"accountId"`
	Songs     []Track  `json:"-"`
}
type AlbumProfile struct {
	Album
	Artists     []Artist `json:"artists"`
	PublishTime int64    `json:"publishTime"`
	Size        int      `json:"size"`
	Description string   `json:"description"`
	Songs       []Track  `json:"-"`
}

func (a *AlbumProfile) ArtistNames() string { return (&Track{Artists: a.Artists}).ArtistNames() }

type Profile struct {
	UserID    int64  `json:"userId"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
}
type User struct {
	UserID    int64  `json:"userId"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
	Signature string `json:"signature"`
	Followeds int    `json:"followeds"`
	Follows   int    `json:"follows"`
}
type Stream struct {
	URL   string  `json:"-"`
	Size  int64   `json:"size"`
	Type  string  `json:"type"`
	Level string  `json:"level"`
	Gain  float64 `json:"gain"`
	Peak  float64 `json:"peak"`
}

var ErrUnavailable = errors.New("QQ 音乐未提供此歌曲的播放链接，可能需要登录或相应会员权限")
var Levels = []string{"standard", "higher", "exhigh", "lossless", "hires"}

func Lossless(level string) bool { return level == "lossless" || level == "hires" }

type SearchType int

const (
	SearchSongs    SearchType = 1
	SearchAlbums   SearchType = 10
	SearchArtists  SearchType = 100
	SearchUsers    SearchType = 1002
	SearchPodcasts SearchType = 1009
)

type SearchResult struct {
	Songs    []Track
	Albums   []AlbumProfile
	Artists  []ArtistProfile
	Users    []User
	Podcasts []Podcast
	Total    int
	Source   SearchSource
	// NextOffset follows the upstream page boundary after decoding filters.
	NextOffset int
}

func (r *SearchResult) Len() int {
	return len(r.Songs) + len(r.Albums) + len(r.Artists) + len(r.Users) + len(r.Podcasts)
}
func (r *SearchResult) Append(n SearchResult) {
	r.Songs = append(r.Songs, n.Songs...)
	r.Albums = append(r.Albums, n.Albums...)
	r.Artists = append(r.Artists, n.Artists...)
	r.Users = append(r.Users, n.Users...)
	r.Podcasts = append(r.Podcasts, n.Podcasts...)
	r.Total = n.Total
	if n.Source != SearchAuto {
		r.Source = n.Source
	}
	if n.NextOffset > 0 {
		r.NextOffset = n.NextOffset
	}
}

type Suggestions struct {
	Songs     []Track
	Artists   []Artist
	Albums    []AlbumProfile
	Playlists []Playlist
	Order     []string
}
