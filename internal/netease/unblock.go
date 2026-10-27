package netease

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// The source matching and provider adapters are modified Go ports of the
// corresponding UnblockNeteaseMusic server flow; see third_party notices.

const alternateSourceTimeout = 15 * time.Second

type alternateAudio struct {
	url  string
	br   int
	size int64
}

type alternateProvider func(context.Context, Track, string) (alternateAudio, error)

// fallbackSongURL tries alternate providers only after NetEase cannot supply
// a full stream. The direct endpoint adapter follows UNM's pyncmd provider.
func fallbackSongURL(id int64, level string, cause error) (*Stream, error) {
	if !graySongFallbackEnabled() {
		return nil, cause
	}

	ctx, cancel := context.WithTimeout(context.Background(), alternateSourceTimeout)
	defer cancel()

	results := make(chan alternateAudio, 3)
	go func() {
		audio, err := gdStudioAudio(ctx, Track{ID: id}, level)
		if err != nil {
			audio = alternateAudio{}
		}
		results <- audio
	}()
	go func() {
		tracks, err := SongDetails([]int64{id})
		if err != nil || len(tracks) == 0 {
			results <- alternateAudio{}
			results <- alternateAudio{}
			return
		}
		track := tracks[0]
		for _, provider := range []alternateProvider{kugouAudio, kuwoAudio} {
			go func(provider alternateProvider) {
				audio, err := provider(ctx, track, level)
				if err == nil {
					results <- audio
					return
				}
				results <- alternateAudio{}
			}(provider)
		}
	}()

	for range 3 {
		select {
		case audio := <-results:
			if audio.url != "" {
				stream, err := streamFromAlternate(audio, level)
				if err == nil {
					return stream, nil
				}
			}
		case <-ctx.Done():
			return nil, ErrGraySongUnavailable
		}
	}
	return nil, ErrGraySongUnavailable
}

func streamFromAlternate(audio alternateAudio, requested string) (*Stream, error) {
	u, err := url.Parse(audio.url)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("netease: alternate provider returned an invalid URL")
	}
	ext := strings.ToLower(path.Ext(u.Path))
	typ := "mp3"
	if ext == ".flac" {
		typ = "flac"
	} else if ext != "" && ext != ".mp3" {
		return nil, fmt.Errorf("netease: unsupported alternate format %s", ext)
	} else if normalizedBR(audio.br) >= 900000 {
		typ = "flac"
	}
	audio.br = normalizedBR(audio.br)
	if typ == "mp3" && audio.br >= 900000 {
		audio.br = 320000
	} else if typ == "flac" && audio.br > 0 && audio.br < 900000 {
		audio.br = 999000
	}
	if audio.br == 0 {
		if typ == "flac" {
			audio.br = 999000
		} else if requested == "lossless" || requested == "hires" {
			requested = "exhigh"
		}
	}
	level := alternateLevel(audio.br, requested)
	return &Stream{URL: audio.url, Size: audio.size, Type: typ, Level: level}, nil
}

func alternateLevel(br int, requested string) string {
	br = normalizedBR(br)
	switch {
	case br >= 900000:
		return "lossless"
	case br >= 256000:
		return "exhigh"
	case br >= 160000:
		return "higher"
	case br > 0:
		return "standard"
	default:
		return requested
	}
}

func normalizedBR(br int) int {
	if br > 0 && br < 1000 {
		return br * 1000
	}
	return br
}

func gdStudioAudio(ctx context.Context, track Track, level string) (alternateAudio, error) {
	br := "320"
	if Lossless(level) {
		br = "999"
	}
	q := url.Values{
		"types":  {"url"},
		"source": {"netease"},
		"id":     {strconv.FormatInt(track.ID, 10)},
		"br":     {br},
	}
	var response struct {
		URL  string `json:"url"`
		BR   int    `json:"br"`
		Size int64  `json:"size"`
	}
	if err := alternateJSON(ctx, "https://music-api.gdstudio.xyz/api.php?"+q.Encode(), nil, &response); err != nil {
		return alternateAudio{}, err
	}
	if response.URL == "" || response.BR <= 0 {
		return alternateAudio{}, ErrUnavailable
	}
	return alternateAudio{url: response.URL, br: normalizedBR(response.BR), size: response.Size}, nil
}

type kugouResult struct {
	Hash     string `json:"hash"`
	HQHash   string `json:"320hash"`
	SQHash   string `json:"sqhash"`
	Name     string `json:"songname"`
	Duration int64  `json:"duration"`
	AlbumID  string `json:"album_id"`
}

func kugouAudio(ctx context.Context, track Track, level string) (alternateAudio, error) {
	keyword := trackKeyword(track)
	if keyword == "" {
		return alternateAudio{}, ErrUnavailable
	}
	q := url.Values{"keyword": {keyword}, "page": {"1"}, "pagesize": {"10"}}
	var response struct {
		Data struct {
			Info []kugouResult `json:"info"`
		} `json:"data"`
	}
	if err := alternateJSON(ctx, "http://mobilecdn.kugou.com/api/v3/search/song?"+q.Encode(), nil, &response); err != nil {
		return alternateAudio{}, err
	}
	song, ok := chooseAlternate(response.Data.Info, track.Duration, func(s kugouResult) int64 { return s.Duration * 1000 })
	if !ok {
		return alternateAudio{}, ErrUnavailable
	}
	hashes := []struct {
		hash string
		br   int
	}{{song.HQHash, 320000}, {song.Hash, 128000}}
	if Lossless(level) {
		hashes = append([]struct {
			hash string
			br   int
		}{{song.SQHash, 999000}}, hashes...)
	}
	for _, candidate := range hashes {
		if candidate.hash == "" {
			continue
		}
		digest := md5.Sum([]byte(candidate.hash + "kgcloudv2"))
		params := url.Values{
			"key":      {hex.EncodeToString(digest[:])},
			"hash":     {candidate.hash},
			"appid":    {"1005"},
			"pid":      {"2"},
			"cmd":      {"25"},
			"behavior": {"play"},
			"album_id": {song.AlbumID},
		}
		var play struct {
			URL []string `json:"url"`
		}
		if err := alternateJSON(ctx, "http://trackercdn.kugou.com/i/v2/?"+params.Encode(), nil, &play); err == nil && len(play.URL) > 0 && play.URL[0] != "" {
			return alternateAudio{url: play.URL[0], br: candidate.br}, nil
		}
	}
	return alternateAudio{}, ErrUnavailable
}

type kuwoResult struct {
	ID       string `json:"MUSICRID"`
	Duration int64  `json:"DURATION"`
}

func kuwoAudio(ctx context.Context, track Track, _ string) (alternateAudio, error) {
	keyword := strings.ReplaceAll(trackKeyword(track), " - ", " ")
	if keyword == "" {
		return alternateAudio{}, ErrUnavailable
	}
	q := url.Values{
		"correct": {"1"}, "vipver": {"1"}, "stype": {"comprehensive"},
		"encoding": {"utf8"}, "rformat": {"json"}, "mobi": {"1"},
		"show_copyright_off": {"1"}, "searchapi": {"6"}, "all": {keyword},
	}
	var response struct {
		Content []json.RawMessage `json:"content"`
	}
	if err := alternateJSON(ctx, "http://search.kuwo.cn/r.s?"+q.Encode(), nil, &response); err != nil {
		return alternateAudio{}, err
	}
	if len(response.Content) < 2 {
		return alternateAudio{}, ErrUnavailable
	}
	var page struct {
		MusicPage struct {
			AbsList []kuwoResult `json:"abslist"`
		} `json:"musicpage"`
	}
	if err := json.Unmarshal(response.Content[1], &page); err != nil {
		return alternateAudio{}, err
	}
	song, ok := chooseAlternate(page.MusicPage.AbsList, track.Duration, func(s kuwoResult) int64 { return s.Duration * 1000 })
	if !ok || song.ID == "" {
		return alternateAudio{}, ErrUnavailable
	}
	params := url.Values{"type": {"convert_url"}, "format": {"mp3"}, "response": {"url"}, "rid": {"MUSIC_" + strings.TrimPrefix(song.ID, "MUSIC_")}}
	result, err := alternateText(ctx, "http://antiserver.kuwo.cn/anti.s?"+params.Encode(), map[string]string{"User-Agent": "okhttp/3.10.0"})
	if err != nil || !strings.HasPrefix(result, "http") {
		return alternateAudio{}, ErrUnavailable
	}
	return alternateAudio{url: strings.TrimSpace(result), br: 320000}, nil
}

func trackKeyword(track Track) string {
	name := strings.TrimSpace(track.Name)
	if name == "" {
		return ""
	}
	artists := make([]string, 0, min(len(track.Artists), 3))
	for _, artist := range track.Artists[:min(len(track.Artists), 3)] {
		if artist.Name != "" {
			artists = append(artists, artist.Name)
		}
	}
	if len(artists) == 0 {
		return name
	}
	return name + " - " + strings.Join(artists, " / ")
}

func chooseAlternate[T any](items []T, duration int64, length func(T) int64) (T, bool) {
	var zero T
	if len(items) == 0 {
		return zero, false
	}
	if duration <= 0 {
		return items[0], true
	}
	for _, item := range items[:min(len(items), 5)] {
		if d := length(item); d > 0 && absMillis(d-duration) < 5_000 {
			return item, true
		}
	}
	return zero, false
}

func absMillis(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func alternateJSON(ctx context.Context, target string, headers map[string]string, out any) error {
	body, err := alternateRequest(ctx, target, headers)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func alternateText(ctx context.Context, target string, headers map[string]string) (string, error) {
	body, err := alternateRequest(ctx, target, headers)
	return string(body), err
}

func alternateRequest(ctx context.Context, target string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/121 Safari/537.36")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	response, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("alternate source: %s", response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 2<<20))
}
