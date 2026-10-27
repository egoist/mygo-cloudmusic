package netease

import (
	"fmt"
	"net/url"
	"strconv"
)

// FM modes: what the personal FM picks songs by.
const (
	FMDefault  = "DEFAULT"  // the listener's taste
	FMFamiliar = "FAMILIAR" // songs like those they know
	FMExplore  = "EXPLORE"  // songs new to them
)

// PersonalFM returns the next few songs of the listener's personal FM, in
// mode. It asks the desktop client's API, which carries the session.
func PersonalFM(mode string) ([]Track, error) {
	var res struct {
		Code int `json:"code"`
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	data := map[string]any{"mode": mode, "limit": 3}
	if err := eapiPost("/api/v1/radio/get", data, &res); err != nil {
		return nil, err
	}
	if res.Code != 200 {
		return nil, fmt.Errorf("netease: personal FM: code %d", res.Code)
	}
	// The radio names its songs' fields as the older APIs do: their
	// details say them, with what the listener may play.
	ids := make([]int64, len(res.Data))
	for i, d := range res.Data {
		ids[i] = d.ID
	}
	return SongDetails(ids)
}

// FMTrash tells the personal FM the listener does not like a song, which
// it plays no more.
func FMTrash(id int64) error {
	var res struct {
		Code int `json:"code"`
	}
	data := map[string]any{"songId": strconv.FormatInt(id, 10), "alg": "RT", "time": "25"}
	if err := eapiPost("/api/radio/trash/add", data, &res); err != nil {
		return err
	}
	if res.Code != 200 {
		return fmt.Errorf("netease: FM trash: code %d", res.Code)
	}
	return nil
}

// HeartMode returns songs like one of the listener's liked songs, by the
// heart mode (心动模式) of their liked songs, the playlist pid: from song
// on, picked after start, the song the mode began with.
func HeartMode(song, start, pid int64) ([]Track, error) {
	var res struct {
		Code int `json:"code"`
		Data []struct {
			SongInfo *Track `json:"songInfo"`
		} `json:"data"`
	}
	data := map[string]any{
		"songId":       strconv.FormatInt(song, 10),
		"startMusicId": strconv.FormatInt(start, 10),
		"playlistId":   strconv.FormatInt(pid, 10),
		"type":         "fromPlayOne",
		"count":        1,
	}
	if err := eapiPost("/api/playmode/intelligence/list", data, &res); err != nil {
		return nil, err
	}
	if res.Code != 200 {
		return nil, fmt.Errorf("netease: heart mode: code %d", res.Code)
	}
	var tracks []Track
	for _, d := range res.Data {
		if d.SongInfo != nil {
			tracks = append(tracks, *d.SongInfo)
		}
	}
	detailPrivileges(tracks)
	return tracks, nil
}

// Comment is what a listener wrote about a song or an episode.
type Comment struct {
	ID   int64 `json:"commentId"`
	User struct {
		ID        int64  `json:"userId"`
		Nickname  string `json:"nickname"`
		AvatarURL string `json:"avatarUrl"`
	} `json:"user"`
	Content    string `json:"content"`
	Time       int64  `json:"time"` // Unix milliseconds
	LikedCount int    `json:"likedCount"`
	Liked      bool   `json:"liked"` // by the listener signed in
	IPLocation struct {
		Location string `json:"location"`
	} `json:"ipLocation"`
	// BeReplied holds the comment this one replies to.
	BeReplied []struct {
		User struct {
			Nickname string `json:"nickname"`
		} `json:"user"`
		Content string `json:"content"`
	} `json:"beReplied"`
}

// Comments are a page of a song's or an episode's comments.
type Comments struct {
	// Hot are the comments most liked, on the first page only.
	Hot    []Comment `json:"hotComments"`
	Latest []Comment `json:"comments"`
	Total  int       `json:"total"`
	More   bool      `json:"more"`
}

// ThreadComments returns limit of the latest comments of a thread, a
// track's (see Track.Thread), from offset, and on the first page its hot
// comments.
func ThreadComments(thread string, offset, limit int) (*Comments, error) {
	var res Comments
	q := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
	if err := get("/api/v1/resource/comments/"+thread, q, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// LikeComment likes a comment of a thread for the listener, or unlikes it.
func LikeComment(thread string, id int64, like bool) error {
	path := "/api/v1/comment/unlike"
	if like {
		path = "/api/v1/comment/like"
	}
	return eapiOK(path, map[string]any{"threadId": thread, "commentId": strconv.FormatInt(id, 10)})
}
