package fnmusic

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"netease-music-mygo/internal/netease"
)

// The service transcodes what the app cannot decode (M4A, APE, WMA, DSD…)
// and cuts the songs of a CUE sheet out of their one file: asked for a
// track and a codec, it makes an HLS stream of it, which this puts back
// together as the MP3 the player decodes.

// Native says whether the player decodes a kind of file as it is.
func Native(format string) bool { return format == "mp3" || format == "flac" }

// Transcoded reports whether a track has to be transcoded to play: of a
// kind of file the player does not decode, or one of a CUE sheet's.
func Transcoded(t *netease.Track) bool { return t.Cue || !Native(t.Format) }

// transcode asks the service for an HLS stream of the track as codec, and
// returns its address.
func (c *Client) transcode(ref, codec string) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	payload := map[string]any{"guid": ref, "output": map[string]any{"codec": codec, "channel": 2}}
	if err := c.call("POST", "/track/transcode", nil, payload, &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", errors.New("fnmusic: the service gave no stream")
	}
	return c.absolute(out.URL)
}

// quit frees the transcoding the service keeps for a track, if it still
// does: it ends by itself in time, so failing is no matter.
func (c *Client) quit(ref string) {
	c.call("POST", "/track/transcode/quit", nil, map[string]string{"guid": ref}, nil)
}

// absolute makes the address the service gave, which is from its API on,
// one to ask for.
func (c *Client) absolute(rel string) (string, error) {
	if strings.HasPrefix(rel, "http://") || strings.HasPrefix(rel, "https://") {
		return rel, nil
	}
	ep, err := c.endpoint()
	if err != nil {
		return "", err
	}
	// Where a reverse proxy mounts the service is before /music/api/v1.
	prefix := strings.TrimSuffix(ep.base.Path, apiPath)
	u := url.URL{Scheme: ep.base.Scheme, Host: ep.base.Host, Path: prefix}
	return u.String() + "/" + strings.TrimLeft(rel, "/"), nil
}

// Segment is a piece of a transcoded stream: where it is, and how long it
// plays.
type Segment struct {
	URL      string
	Duration time.Duration
}

// HLS is a track as the service transcodes it to MP3: a list of pieces,
// each of which can be had alone, so that a seek asks for the piece it
// lands in, not for all that come before it.
type HLS struct {
	Segments []Segment
	Total    time.Duration

	c    *Client
	ref  string
	once sync.Once
}

// TranscodeHLS has the service transcode the track to MP3, and returns the
// pieces of the result. The service lists them all at once, with the ENDLIST
// of a stream that is whole; one still being made is waited for.
func (c *Client) TranscodeHLS(ctx context.Context, t *netease.Track) (*HLS, error) {
	u, err := c.transcode(t.Ref, "mp3")
	if err != nil {
		return nil, fmt.Errorf("转码失败：%w", trimFn(err))
	}
	deadline := time.Now().Add(hlsPatience)
	for {
		body, err := c.fetch(ctx, u)
		if err != nil {
			return nil, err
		}
		pl, err := parsePlaylist(string(body), u)
		if err != nil {
			return nil, err
		}
		if pl.variant != "" {
			u = pl.variant
			continue
		}
		if pl.end && len(pl.segments) > 0 {
			h := &HLS{c: c, ref: t.Ref}
			for i, s := range pl.segments {
				h.Segments = append(h.Segments, Segment{URL: s, Duration: pl.durations[i]})
				h.Total += pl.durations[i]
			}
			return h, nil
		}
		if time.Now().After(deadline) {
			return nil, errors.New("转码超时")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(hlsPoll):
		}
	}
}

func trimFn(err error) error {
	return errors.New(strings.TrimPrefix(err.Error(), "fnmusic: "))
}

// Fetch returns the audio of piece i, as the MP3 frames it holds.
func (h *HLS) Fetch(ctx context.Context, i int) ([]byte, error) {
	if i < 0 || i >= len(h.Segments) {
		return nil, io.EOF
	}
	b, err := h.c.fetch(ctx, h.Segments[i].URL)
	if err != nil {
		return nil, err
	}
	return audioOf(b), nil
}

// Release lets the service go of the transcoding it keeps, which it ends by
// itself in time: for when the track is let go of.
func (h *HLS) Release() { h.once.Do(func() { go h.c.quit(h.ref) }) }

// get asks for something at an address of the service, signed as the
// service takes it, and signed in again once if the session ended.
func (c *Client) get(ctx context.Context, raw string) (*http.Response, error) {
	for retry := false; ; retry = true {
		token, err := c.session()
		if err != nil {
			return nil, err
		}
		res, err := c.send(func() (*http.Request, error) { return c.signedGet(ctx, raw, token) })
		if err != nil {
			return nil, err
		}
		if (res.StatusCode == 401 || res.StatusCode == 403) && !retry {
			res.Body.Close()
			if err := c.refresh(token); err != nil {
				return nil, err
			}
			continue
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			return nil, fmt.Errorf("fnmusic: %s", res.Status)
		}
		return res, nil
	}
}

// signedGet makes a request for raw, an address of the service, with its
// session, and the signature that goes with the path from its API on.
func (c *Client) signedGet(ctx context.Context, raw, token string) (*http.Request, error) {
	ep, err := c.endpoint()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	if i := strings.LastIndex(req.URL.Path, apiPath); i >= 0 {
		nonce := fmt.Sprintf("%06d", 100000+randomN(900000))
		req.Header.Set("authx", signature(req.URL.Path[i:], readQuery(req.URL.Query()), nonce, time.Now().UnixMilli()))
	}
	req.Header.Set("User-Agent", netease.UserAgent)
	cookies := []string{"music-token=" + url.QueryEscape(token)}
	if ep.relay {
		cookies = append(cookies, "mode=relay")
	}
	req.Header.Set("Cookie", strings.Join(cookies, "; "))
	c.accessHeaders(req.Header)
	return req, nil
}

// How long to wait for the service to make a stream it is still making,
// and how long between asking.
var (
	hlsPoll     = time.Second
	hlsPatience = 90 * time.Second
)

// fetch returns what is at address u.
func (c *Client) fetch(ctx context.Context, u string) ([]byte, error) {
	res, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return io.ReadAll(io.LimitReader(res.Body, 64<<20))
}

type playlist struct {
	segments  []string
	durations []time.Duration
	variant   string // the first stream of a master playlist
	end       bool   // the stream is all there
}

// parsePlaylist reads an m3u8 at address base: the pieces of the audio, as
// addresses, and whether the playlist is complete. The initialisation
// segment of an fMP4 stream is left out: it holds no audio.
func parsePlaylist(text, base string) (playlist, error) {
	if !strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") {
		return playlist{}, errors.New("fnmusic: 转码返回的不是播放列表")
	}
	b, err := url.Parse(base)
	if err != nil {
		return playlist{}, err
	}
	resolve := func(ref string) string {
		r, err := url.Parse(ref)
		if err != nil {
			return ""
		}
		return b.ResolveReference(r).String()
	}
	var pl playlist
	master := false
	var dur time.Duration
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF"):
			master = true
		case line == "#EXT-X-ENDLIST":
			pl.end = true
		case strings.HasPrefix(line, "#EXTINF:"):
			sec, _, _ := strings.Cut(strings.TrimPrefix(line, "#EXTINF:"), ",")
			f, _ := strconv.ParseFloat(strings.TrimSpace(sec), 64)
			dur = time.Duration(f * float64(time.Second))
		case strings.HasPrefix(line, "#"):
		case master && pl.variant == "":
			pl.variant = resolve(line)
		case !master:
			if s := resolve(line); s != "" {
				pl.segments = append(pl.segments, s)
				pl.durations = append(pl.durations, dur)
				dur = 0
			}
		}
	}
	// With no piece yet, it is not made yet: it is asked for again.
	return pl, nil
}

// audioOf takes the audio out of a piece of a stream, whatever it is
// wrapped in: an MPEG transport stream, a fragment of MP4, or the audio
// itself.
func audioOf(b []byte) []byte {
	switch {
	case len(b) >= 189 && b[0] == 0x47 && b[188] == 0x47:
		return tsAudio(b)
	case len(b) >= 8 && isBox(b[4:8]):
		return mp4Audio(b)
	}
	return b
}

func isBox(t []byte) bool {
	switch string(t) {
	case "ftyp", "styp", "moof", "mdat", "moov", "sidx":
		return true
	}
	return false
}

// mp4Audio joins the media data of a fragment of MP4: for audio of MP3, the
// samples are its frames, as they come.
func mp4Audio(b []byte) []byte {
	var out []byte
	for len(b) >= 8 {
		size := int(binary.BigEndian.Uint32(b))
		typ := string(b[4:8])
		head := 8
		if size == 1 && len(b) >= 16 {
			size, head = int(binary.BigEndian.Uint64(b[8:])), 16
		}
		if size == 0 || size > len(b) {
			size = len(b)
		}
		if size < head {
			break
		}
		if typ == "mdat" {
			out = append(out, b[head:size]...)
		}
		b = b[size:]
	}
	return out
}

// tsAudio takes the audio out of an MPEG transport stream: of the first
// stream whose packets begin with a PES header of audio (stream id C0 to
// DF), without its header.
func tsAudio(b []byte) []byte {
	var out []byte
	pid := -1
	for p := 0; p+188 <= len(b); p += 188 {
		pkt := b[p : p+188]
		if pkt[0] != 0x47 {
			continue
		}
		start := pkt[1]&0x40 != 0
		id := int(pkt[1]&0x1f)<<8 | int(pkt[2])
		at := 4
		switch pkt[3] >> 4 & 3 {
		case 0, 2: // no payload
			continue
		case 3: // an adaptation field first
			at = 5 + int(pkt[4])
		}
		if at >= 188 {
			continue
		}
		payload := pkt[at:]
		if start && len(payload) >= 9 && bytes.Equal(payload[:3], []byte{0, 0, 1}) {
			if pid < 0 && payload[3] >= 0xC0 && payload[3] <= 0xDF {
				pid = id
			}
			if id != pid {
				continue
			}
			if n := 9 + int(payload[8]); n <= len(payload) {
				out = append(out, payload[n:]...)
			}
			continue
		}
		if id == pid {
			out = append(out, payload...)
		}
	}
	return out
}
