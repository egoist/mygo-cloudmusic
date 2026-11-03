package qqmusic

import "time"

// View models retained for the native player.
type Podcast struct {
	ID                                  int64
	Name, PicURL, Description, Category string
	DJ                                  User
	ProgramCount                        int
	SubCount                            int64
	PlayCount                           float64
	Subed                               bool
	RcmdText, LastProgram               string
}
type Program struct {
	ID                   int64
	Name, CoverURL       string
	Duration, CreateTime int64
	Description          string
	MainSong             struct{ ID int64 }
}

func (p *Program) Track(pod *Podcast) Track {
	cover := p.CoverURL
	if cover == "" {
		cover = pod.PicURL
	}
	return Track{ID: p.MainSong.ID, Name: p.Name, Duration: p.Duration, ProgramID: p.ID, PodcastID: pod.ID, Description: p.Description, Album: Album{Name: pod.Name, PicURL: cover}, Artists: []Artist{{Name: pod.DJ.Nickname}}}
}

type Event struct {
	ID, Time int64
	User     User
	Pictures []struct{ SquareURL string }
	Info     struct{ LikedCount, CommentCount, ShareCount int }
	Content  EventContent
}
type EventContent struct {
	Msg       string
	Song      *Track
	Album     *AlbumProfile
	Artist    *ArtistProfile
	Playlist  *Playlist
	Video     bool
	Forwarded string
}
type Play struct {
	Track  Track
	At     time.Time
	Device string
}
type Comment struct {
	ID       int64
	RemoteID string
	User     struct {
		ID                  int64
		Nickname, AvatarURL string
	}
	Content    string
	Time       int64
	LikedCount int
	Liked      bool
	IPLocation struct{ Location string }
	BeReplied  []struct {
		User    struct{ Nickname string }
		Content string
	}
}
type Comments struct {
	Hot, Latest []Comment
	Total       int
	More        bool
}

const (
	FMDefault  = "DEFAULT"
	FMFamiliar = "FAMILIAR"
	FMExplore  = "EXPLORE"
)

// QQ does not expose these NetEase contracts. The QQ UI omits their entries.
func FMTrash(int64) error                                      { return ErrUnsupported }
func HeartMode(int64, int64, int64) ([]Track, error)           { return nil, ErrUnsupported }
func LikeComment(string, int64, bool) error                    { return ErrUnsupported }
func RecentPlays(int) ([]Play, error)                          { return nil, ErrUnsupported }
func FollowArtist(int64, bool) error                           { return ErrUnsupported }
func FollowedArtists() ([]ArtistProfile, error)                { return nil, ErrUnsupported }
func Events(int64, int, int64) ([]Event, int64, bool, error)   { return nil, 0, false, ErrUnsupported }
func SubscribePodcast(int64, bool) error                       { return ErrUnsupported }
func SubscribedPodcasts() ([]Podcast, error)                   { return nil, ErrUnsupported }
func PodcastDetail(int64) (*Podcast, error)                    { return nil, ErrUnsupported }
func PodcastPrograms(int64, int, int) ([]Program, bool, error) { return nil, false, ErrUnsupported }
func RecommendedPodcasts() ([]Podcast, error)                  { return nil, ErrUnsupported }
func HotPodcasts(int, int) ([]Podcast, bool, error)            { return nil, false, ErrUnsupported }
func LikeProgram(int64, bool) error                            { return ErrUnsupported }
func ProgramsLiked([]int64) (map[int64]bool, error)            { return nil, ErrUnsupported }
