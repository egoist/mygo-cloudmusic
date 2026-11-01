package netease

import (
	"fmt"
	"net/url"
	"strconv"
)

// SubscribePodcast subscribes the listener to a podcast, or unsubscribes
// them.
func SubscribePodcast(id int64, subscribe bool) error {
	return subscription("djradio", subscribe, map[string]any{"id": strconv.FormatInt(id, 10)})
}

// FollowArtist follows an artist for the listener, or unfollows them.
// Through the web API: as the desktop client, through eapi, following is
// refused with -460, 检测到您的网络环境存在风险, though unfollowing is not.
func FollowArtist(id int64, follow bool) error {
	path := "/api/artist/unsub"
	if follow {
		path = "/api/artist/sub"
	}
	form := url.Values{"artistId": {strconv.FormatInt(id, 10)}, "artistIds": {fmt.Sprintf("[%d]", id)}}
	return post(path, form, &struct{}{})
}

func subscription(kind string, sub bool, data map[string]any) error {
	path := "/api/" + kind + "/unsub"
	if sub {
		path = "/api/" + kind + "/sub"
	}
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := eapiPost(path, data, &res); err != nil {
		return err
	}
	if res.Code != 200 {
		return fmt.Errorf("netease: %s: code %d %s", path, res.Code, res.Message)
	}
	return nil
}

// FollowedArtists returns the artists the listener follows, those followed
// last first.
func FollowedArtists() ([]ArtistProfile, error) {
	var out []ArtistProfile
	for offset := 0; ; offset += subPage {
		var res struct {
			Data    []ArtistProfile `json:"data"`
			HasMore bool            `json:"hasMore"`
		}
		if err := get("/api/artist/sublist", subQuery(offset), &res); err != nil {
			return nil, err
		}
		out = append(out, res.Data...)
		if !res.HasMore || len(res.Data) == 0 {
			return out, nil
		}
	}
}

// SubscribedPodcasts returns the podcasts the listener subscribes to, those
// subscribed to last first.
func SubscribedPodcasts() ([]Podcast, error) {
	var out []Podcast
	for offset := 0; ; offset += subPage {
		var res struct {
			Podcasts []Podcast `json:"djRadios"`
			HasMore  bool      `json:"hasMore"`
		}
		if err := get("/api/djradio/get/subed", subQuery(offset), &res); err != nil {
			return nil, err
		}
		out = append(out, res.Podcasts...)
		if !res.HasMore || len(res.Podcasts) == 0 {
			return out, nil
		}
	}
}

// subPage is how many artists or podcasts the listener follows NetEase
// sends at a time.
const subPage = 100

func subQuery(offset int) url.Values {
	return url.Values{"limit": {strconv.Itoa(subPage)}, "offset": {strconv.Itoa(offset)}, "total": {"true"}}
}

// SubscribePlaylist saves a playlist for the listener, or unsaves it.
func SubscribePlaylist(id int64, subscribe bool) error {
	path := "/api/playlist/unsubscribe"
	if subscribe {
		path = "/api/playlist/subscribe"
	}
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := eapiPost(path, map[string]any{"id": strconv.FormatInt(id, 10)}, &res); err != nil {
		return err
	}
	if res.Code != 200 {
		return fmt.Errorf("netease: %s: code %d %s", path, res.Code, res.Message)
	}
	return nil
}
