package qqmusic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

const loudnessLifetime = 30 * 24 * time.Hour

var loudnessCacheFile = func() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "qqmusic-loudness.json"), nil
}

type loudnessEntry struct {
	Volume Loudness  `json:"volume"`
	At     time.Time `json:"at"`
}

// The cache is independent of the audio cache: clearing or disabling song
// downloads need not discard measurements, or make cached songs go unbalanced.
type loudnessCache struct {
	sync.Mutex
	loaded  bool
	path    string
	entries map[string]loudnessEntry
}

var songLoudnessCache = &loudnessCache{}

func (c *loudnessCache) load() {
	if c.loaded {
		return
	}
	c.loaded = true
	c.entries = make(map[string]loudnessEntry)
	path, err := loudnessCacheFile()
	if err != nil {
		return
	}
	c.path = path
	if b, err := os.ReadFile(path); err == nil {
		// A lost cache can always be fetched again.
		var entries map[string]loudnessEntry
		if json.Unmarshal(b, &entries) == nil && entries != nil {
			c.entries = entries
		}
	}
}

func (c *loudnessCache) get(mid string, now time.Time) (Loudness, bool) {
	c.Lock()
	defer c.Unlock()
	c.load()
	entry, ok := c.entries[mid]
	if !ok || now.Sub(entry.At) >= loudnessLifetime || entry.Volume.Peak < 0 {
		return Loudness{}, false
	}
	return entry.Volume, true
}

func (c *loudnessCache) put(volumes map[string]Loudness, now time.Time) {
	if len(volumes) == 0 {
		return
	}
	c.Lock()
	defer c.Unlock()
	c.load()
	for mid, volume := range volumes {
		if mid != "" {
			c.entries[mid] = loudnessEntry{Volume: volume, At: now}
		}
	}
	if c.path == "" {
		return
	}
	if b, err := json.Marshal(c.entries); err == nil {
		if os.MkdirAll(filepath.Dir(c.path), 0o700) == nil {
			_ = os.WriteFile(c.path, b, 0o600)
		}
	}
}

// SongLoudness returns the standard master's measurements, also for a song
// whose audio is already on disk. Failure is optional metadata, not a reason
// to stop playback; callers should use unity gain when it is unavailable.
func SongLoudness(t Track) (Loudness, error) {
	if t.MID == "" {
		if known, ok := knownTrack(t.ID); ok {
			t = known
		}
	}
	volumes, err := SongLoudnesses([]Track{t})
	if t.MID == "" {
		if known, ok := knownTrack(t.ID); ok {
			t = known
		}
	}
	return volumes[t.MID], err
}

// SongLoudnesses fetches uncached songs in anonymous musicu batches of at most
// 30. Successful measurements remain available if another song in the batch
// fails. An absent volume on a valid song means no measurement (0 dB).
func SongLoudnesses(tracks []Track) (map[string]Loudness, error) {
	volumes := make(map[string]Loudness, len(tracks))
	missing := make([]Track, 0, len(tracks))
	seen := make(map[string]bool, len(tracks))
	now := time.Now()
	for _, track := range tracks {
		if track.MID == "" {
			if known, ok := knownTrack(track.ID); ok {
				track = known
			}
		}
		key := track.MID
		if key == "" {
			key = "id:" + strconv.FormatInt(track.ID, 10)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		if track.MID != "" {
			if volume, ok := songLoudnessCache.get(track.MID, now); ok {
				volumes[track.MID] = volume
				continue
			}
		}
		missing = append(missing, track)
	}
	var failures []error
	for len(missing) > 0 {
		n := min(len(missing), 30)
		batch, err := fetchSongLoudnesses(missing[:n])
		for mid, volume := range batch {
			volumes[mid] = volume
		}
		songLoudnessCache.put(batch, now)
		if err != nil {
			failures = append(failures, err)
		}
		missing = missing[n:]
	}
	return volumes, errors.Join(failures...)
}

func fetchSongLoudnesses(tracks []Track) (map[string]Loudness, error) {
	const module = "music.pf_song_detail_svr"
	payload := map[string]any{"comm": map[string]any{"ct": 24, "cv": 0, "uin": "0", "format": "json"}}
	methods := make([]string, len(tracks))
	for i, track := range tracks {
		method := "get_song_detail_yqq"
		param := map[string]any{"song_mid": track.MID}
		if track.MID == "" {
			if track.ID <= 0 {
				return nil, errors.New("qqmusic: song ID or MID missing for loudness")
			}
			method = "get_song_detail"
			param = map[string]any{"song_id": track.ID}
		}
		methods[i] = method
		payload["req_"+strconv.Itoa(i)] = map[string]any{"module": module, "method": method, "param": param}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, musicuURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://y.qq.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	b, err = readHTTP(req)
	if err != nil {
		return nil, err
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("qqmusic: malformed loudness response: %w", err)
	}
	var code *int
	if json.Unmarshal(response["code"], &code) != nil || code == nil {
		return nil, errors.New("qqmusic: missing loudness gateway status")
	}
	if *code != 0 {
		return nil, &ProtocolError{GlobalCode: *code, Module: module, Method: methods[0]}
	}
	volumes := make(map[string]Loudness, len(tracks))
	var failures []error
	for i, track := range tracks {
		var reply struct {
			Code    *int `json:"code"`
			Subcode int  `json:"subcode"`
			Data    struct {
				Track object `json:"track_info"`
			} `json:"data"`
		}
		err := json.Unmarshal(response["req_"+strconv.Itoa(i)], &reply)
		switch {
		case err != nil || reply.Code == nil:
			failures = append(failures, errors.New("qqmusic: missing loudness RPC status"))
			continue
		case *reply.Code != 0 || reply.Subcode != 0:
			status := *reply.Code
			if status == 0 {
				status = reply.Subcode
			}
			failures = append(failures, &ProtocolError{GlobalCode: *code, Code: status, Module: module, Method: methods[i]})
			continue
		case reply.Data.Track == nil:
			failures = append(failures, errors.New("qqmusic: missing loudness track"))
			continue
		}
		mid := text(reply.Data.Track, "mid", "songmid", "songMid")
		id := number(reply.Data.Track, "id", "songid", "songId")
		if mid == "" || track.MID != "" && mid != track.MID || track.MID == "" && id != track.ID {
			failures = append(failures, errors.New("qqmusic: loudness song identity mismatch"))
			continue
		}
		detail := convertTrack(reply.Data.Track)
		if raw := reply.Data.Track["volume"]; len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && detail.Volume == nil {
			failures = append(failures, errors.New("qqmusic: malformed loudness measurement"))
			continue
		}
		volume := Loudness{}
		if detail.Volume != nil {
			volume = *detail.Volume
		}
		volumes[detail.MID] = volume
	}
	return volumes, errors.Join(failures...)
}
