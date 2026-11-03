package fnmusic

import (
	"net/url"

	"netease-music-mygo/internal/netease"
)

// Roaming is the service's own random play: it picks the songs, one after
// another, each asked for with the roam ID of the one before.

type roamStep struct {
	RoamID string   `json:"roamId"`
	Track  rawTrack `json:"track"`
}

// RoamStart begins a roam, returning its first song and that song's roam
// ID, which asks for the next.
func (c *Client) RoamStart() (netease.Track, string, error) {
	var res struct {
		Current *roamStep `json:"current"`
	}
	if err := c.call("GET", "/track/roam-start", url.Values{"deviceId": {c.cfg.DeviceID}}, nil, &res); err != nil {
		return netease.Track{}, "", err
	}
	return c.roamTrack(res.Current)
}

// RoamNext returns the song the service picks to follow the one with roam
// ID after, and its own roam ID.
func (c *Client) RoamNext(after string) (netease.Track, string, error) {
	var res struct {
		Current *roamStep `json:"current"`
	}
	q := url.Values{"deviceId": {c.cfg.DeviceID}, "relativeRoamId": {after}}
	if err := c.call("GET", "/track/roam-next", q, nil, &res); err != nil {
		return netease.Track{}, "", err
	}
	return c.roamTrack(res.Current)
}

func (c *Client) roamTrack(s *roamStep) (netease.Track, string, error) {
	if s == nil {
		return netease.Track{}, "", errNoRoam
	}
	t, ok := s.Track.track()
	if !ok {
		return netease.Track{}, "", errNoRoam
	}
	return t, s.RoamID, nil
}
