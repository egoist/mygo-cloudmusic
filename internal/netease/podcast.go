package netease

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Podcast is a channel of episodes, which NetEase calls a radio.
type Podcast struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	PicURL       string  `json:"picUrl"`
	Description  string  `json:"desc"`
	Category     string  `json:"category"`
	DJ           User    `json:"dj"`
	ProgramCount int     `json:"programCount"`
	SubCount     int64   `json:"subCount"`
	PlayCount    float64 `json:"playCount"`
	Subed        bool    `json:"subed"`    // by the listener signed in
	RcmdText     string  `json:"rcmdtext"` // a line on why to listen, in lists
	// LastProgram is the name of its newest episode, in the listener's
	// subscriptions.
	LastProgram string `json:"lastProgramName"`
}

// Program is an episode of a podcast.
type Program struct {
	ID          int64       `json:"id"`
	Name        string      `json:"name"`
	CoverURL    string      `json:"coverUrl"`
	Duration    int64       `json:"duration"`   // milliseconds
	CreateTime  int64       `json:"createTime"` // Unix milliseconds
	Description string      `json:"description"`
	MainSong    legacyTrack `json:"mainSong"` // its audio
}

// Track is the episode as it plays, in a podcast: its audio's ID, which
// streams, under the episode's name, cover and host.
func (p *Program) Track(pod *Podcast) Track {
	cover := p.CoverURL
	if cover == "" {
		cover = pod.PicURL
	}
	t := Track{
		ID: p.MainSong.ID, Name: p.Name, Duration: p.Duration,
		Album:     Album{Name: pod.Name, PicURL: cover},
		ProgramID: p.ID, PodcastID: pod.ID, Description: p.Description,
	}
	if pod.DJ.Nickname != "" {
		t.Artists = []Artist{{Name: pod.DJ.Nickname}}
	}
	return t
}

// PodcastDetail returns a podcast.
func PodcastDetail(id int64) (*Podcast, error) {
	var res struct {
		Data *Podcast `json:"data"`
	}
	if err := get("/api/djradio/v2/get", url.Values{"id": {strconv.FormatInt(id, 10)}}, &res); err != nil {
		return nil, err
	}
	if res.Data == nil || res.Data.ID != id {
		return nil, fmt.Errorf("netease: no podcast %d", id)
	}
	return res.Data, nil
}

// PodcastPrograms returns limit of a podcast's episodes from offset, the
// newest first, and whether it has more.
func PodcastPrograms(id int64, limit, offset int) ([]Program, bool, error) {
	var res struct {
		Programs []Program `json:"programs"`
		More     bool      `json:"more"`
	}
	q := url.Values{"radioId": {strconv.FormatInt(id, 10)}, "limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}, "asc": {"false"}}
	err := get("/api/dj/program/byradio", q, &res)
	return res.Programs, res.More, err
}

// RecommendedPodcasts returns the podcasts NetEase picks to listen to.
func RecommendedPodcasts() ([]Podcast, error) {
	var res struct {
		Podcasts []Podcast `json:"djRadios"`
	}
	err := get("/api/djradio/recommend/v1", nil, &res)
	return res.Podcasts, err
}

// HotPodcasts returns limit of the most popular podcasts from offset, and
// whether there are more.
func HotPodcasts(limit, offset int) ([]Podcast, bool, error) {
	var res struct {
		Podcasts []Podcast `json:"djRadios"`
		HasMore  bool      `json:"hasMore"`
	}
	q := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
	err := get("/api/djradio/hot/v1", q, &res)
	return res.Podcasts, res.HasMore, err
}

func programThread(id int64) string { return "A_DJ_1_" + strconv.FormatInt(id, 10) }

// LikeProgram likes an episode for the listener, or unlikes it.
func LikeProgram(id int64, like bool) error {
	path := "/api/resource/unlike"
	if like {
		path = "/api/resource/like"
	}
	return eapiOK(path, map[string]any{"threadId": programThread(id)})
}

// ProgramsLiked returns which of the episodes ids the listener likes.
// NetEase lists no episodes a listener likes, but answers of each whether
// they do.
func ProgramsLiked(ids []int64) (map[int64]bool, error) {
	var res struct {
		Code int `json:"code"`
		Data []struct {
			ID    int64 `json:"resourceId"`
			Liked bool  `json:"liked"`
		} `json:"data"`
	}
	list := make([]string, len(ids))
	for i, id := range ids {
		list[i] = strconv.FormatInt(id, 10)
	}
	// Resource type 1 is an episode, whose thread is A_DJ_1.
	data := map[string]any{"resourceType": "1", "resourceIds": "[" + strings.Join(list, ",") + "]"}
	if err := eapiPost("/api/resource/commentInfo/list", data, &res); err != nil {
		return nil, err
	}
	if res.Code != 200 {
		return nil, fmt.Errorf("netease: episodes liked: code %d", res.Code)
	}
	liked := make(map[int64]bool, len(res.Data))
	for _, d := range res.Data {
		liked[d.ID] = d.Liked
	}
	return liked, nil
}
