package player

import (
	"context"
	"time"

	"netease-music-mygo/internal/netease"
)

// maxSkipped is how many songs that cannot play in a row stop the player,
// with the last one's error shown.
const maxSkipped = 3

// AudioStream is the player-owned decoded stream contract exposed to platform
// adapters that supply a decoder. Its source method remains private to this
// package, so external adapters can only receive and close the stream.
type AudioStream = stream

// SegmentedSource is an encoded stream made of ordered pieces. Providers
// adapt their own HLS or chunked response to this interface; the player owns
// only decoder and seek/preroll behavior.
type SegmentedSource interface {
	SegmentCount() int
	SegmentDuration(int) time.Duration
	TotalDuration() time.Duration
	FetchSegment(context.Context, int) ([]byte, error)
	Close()
}

// OpenSegmentedStream decodes a provider's segmented MP3 source through the
// shared player pipeline.
func OpenSegmentedStream(source SegmentedSource) (AudioStream, error) {
	stream, err := openSegmentedStream(source)
	if err != nil && source != nil {
		source.Close()
	}
	return stream, err
}

// Platform extension seam.
//
// Keep provider-neutral player additions here: embedded extension state,
// platform queue helpers, reset behavior and optional resolver hooks. The
// upstream player.go should gain only the smallest possible embedded field or
// stable hook call. Concrete provider protocol types must stay in the
// corresponding platform adapter and must not be imported here.

// PlayList plays the loaded part of a provider list and asks more for the
// remainder as playback approaches its end. It is the shared queue seam for
// platform pagers; the player never knows how a page is fetched.
func (p *Player) PlayList(tracks []netease.Track, start int, more func(func([]netease.Track))) {
	if len(tracks) == 0 {
		return
	}
	p.PlayQueue(tracks, start)
	p.feed = more
	p.refill()
}

// AppendQueue adds songs to the current queue without replacing what is
// playing. Duplicate song IDs are ignored, just as radio refills are.
func (p *Player) AppendQueue(tracks []netease.Track) {
	if p == nil || len(tracks) == 0 {
		return
	}
	p.EndRadio()
	p.appendTracks(tracks)
	p.sync()
}

// Reset stops playback and drops the complete playback context. It is used
// when the application changes music platforms: a queue, radio feed, or
// in-flight load belongs to the platform that created it and must not leak
// into the next one.
func (p *Player) Reset() {
	if p == nil {
		return
	}
	p.stop()
	p.gen++
	p.endRadio()
	p.queue = nil
	p.order = nil
	p.at = -1
	p.track = nil
	p.info = nil
	p.lyrics = nil
	p.lyricsLoaded = false
	p.pending = 0
	p.seeking = false
	p.loading = false
	p.playing = false
	p.err = ""
	p.shuffle = false
	p.repeat = RepeatOff
	p.sync()
}

type PlatformPlayer struct {
	started bool // the current track has been reported as started
	// Stream resolves audio for the active platform. Nil uses NetEase's
	// original resolver, preserving the upstream default path.
	ResolveStream func(*netease.Track, string) (*netease.Stream, error)
	// Artwork and Lyrics resolve optional platform data. Nil keeps the
	// upstream NetEase requests.
	Artwork       func(*netease.Track) ([]byte, error)
	ResolveLyrics func(*netease.Track) (string, string, string, error)
	ResolveAudio  func(*netease.Track, string) (*netease.Stream, AudioStream, error)
	// Started hears a track once, as it first begins to play.
	Started func(*netease.Track)
}

// acceptRefill applies a provider queue refill without making player.go know
// how an exhausted feed should end. Returning false tells the caller that no
// tracks were appended and playback should stop waiting for more.
func (p *Player) acceptRefill(more []netease.Track) bool {
	if len(more) == 0 && p.radio == "" {
		p.feed = nil
		if p.awaiting {
			p.awaiting = false
			p.playing = false
			p.sync()
		}
		return false
	}
	p.appendTracks(more)
	return true
}

// resolveArtwork keeps the provider fallback outside player.go. A nil hook is
// the unchanged NetEase path.
func (p *Player) resolveArtwork(t *netease.Track) ([]byte, error) {
	if p.Artwork != nil {
		return p.Artwork(t)
	}
	return netease.Fetch(netease.Sized(t.Album.PicURL, 600))
}

// resolveLyrics keeps provider lyric lookup behind the same optional hook.
func (p *Player) resolveLyrics(t *netease.Track) (string, string, string, error) {
	if p.ResolveLyrics != nil {
		return p.ResolveLyrics(t)
	}
	return netease.Lyrics(t.ID)
}

// resolveAudio reports whether a provider handled the stream. A nil result
// means that the shared URL/cache decoder should continue with its usual path.
func (p *Player) resolveAudio(t *netease.Track, level string) (*netease.Stream, stream, error, bool) {
	if p.ResolveAudio == nil || t.Source == "" {
		return nil, nil, nil, false
	}
	info, decoded, err := p.ResolveAudio(t, level)
	if decoded == nil && err == nil {
		return nil, nil, nil, false
	}
	return info, decoded, err, true
}

// resolveStream lets a provider supply a signed URL while preserving the
// original NetEase resolver when no hook is installed or the track is native.
func (p *Player) resolveStream(t *netease.Track, level string) (*netease.Stream, error) {
	if p.ResolveStream != nil {
		return p.ResolveStream(t, level)
	}
	return netease.SongURL(t.ID, level)
}

// markStarted reports the current track once, as it first begins to play.
func (p *Player) markStarted() {
	if p.started || p.track == nil || p.Started == nil {
		return
	}
	p.started = true
	p.Started(p.track)
}
