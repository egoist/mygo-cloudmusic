package player

import (
	"errors"
	"math"
	"math/rand/v2"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// asker says where a track streams from, at a level.
type asker func(id int64, level string) (*netease.Stream, error)

// songURL is where a track streams from: what the API says, or what the
// tests serve. A load takes it here, on the main thread, as its goroutine
// outlives the call that started it.
var songURL asker = netease.SongURL

type RepeatMode int

const (
	RepeatOff RepeatMode = iota
	RepeatAll
	RepeatOne
)

// Player plays a queue of tracks. Its methods run on the main thread;
// loading happens on goroutines, whose results post brings back there.
type Player struct {
	post func(func())
	// Quality returns the stream level to load, as the settings say.
	Quality func() string
	// Normalize reports whether to even out the loudness of tracks, as
	// the settings say.
	Normalize func() bool
	// Cache keeps the songs played, if set.
	Cache *Cache
	// Changed, if set, hears that what plays, where it is or whether it
	// plays changed.
	Changed func()

	queue   []netease.Track
	order   []int // the queue's indices in play order: shuffled or not
	at      int   // where in order the current track is, -1 for none
	shuffle bool
	repeat  RepeatMode
	volume  float64

	// A radio goes on with the tracks its feed gives, as the queue nears
	// its end, for as long as nothing else is played.
	radio    string
	feed     func(got func([]netease.Track))
	radioGen int  // counts radios, to drop what an old one gives
	feeding  bool // the feed is asked
	awaiting bool // at the queue's end, for what the feed gives

	track   *netease.Track
	out     *audio.Player
	stream  stream
	info    *netease.Stream // what the current track streams
	fade    fader
	gain    float64 // the current track's normalizing gain
	loading bool
	playing bool // what the listener wants: false while paused
	err     string
	gen     int // counts loads, to drop the results of stale ones

	// ahead is the next track's download, fetched while this one plays,
	// so that it starts at once when this one ends: nil without one.
	ahead *ahead
	// tries counts the times the current track was tried again, as its load
	// failed, or its download was cut off: it starts over with another
	// track, so that a song that keeps breaking off is left where it is.
	tries int

	lyrics       []lyrics.Line
	lyricsLoaded bool

	// pending holds a seek asked for while the track loads.
	pending time.Duration
	// A seek playing fades out, then lands, then fades in: meanwhile the
	// position is where it goes.
	seeking bool
	seekTo  time.Duration
	seekGen int
}

// ahead is a track's download, fetched while the one before it plays: the
// cache keeps it, and it is read from there when it plays.
type ahead struct {
	id    int64 // the track's
	level string
	dl    *download
	info  *netease.Stream
}

func New(post func(func())) *Player {
	p := &Player{post: post, at: -1, volume: 0.8, gain: 1}
	// Music, mixed far enough ahead to play on while macOS keeps the app
	// waiting, as it does while its appearance changes.
	audio.SetLatency(audio.LatencyPlayback)
	// The media keys, and the controls of Control Center.
	mygo.NowPlaying.OnCommand(func(cmd mygo.MediaCommand, pos time.Duration) {
		switch cmd {
		case mygo.MediaPlay, mygo.MediaPause:
			if (cmd == mygo.MediaPlay) != p.playing {
				p.TogglePlay()
			}
		case mygo.MediaNext:
			p.Next()
		case mygo.MediaPrevious:
			p.Prev()
		case mygo.MediaSeek:
			p.Seek(pos)
		}
		p.post(func() {})
	})
	return p
}

// sync shows the system, and Changed, where playback is.
func (p *Player) sync() {
	if p.Changed != nil {
		defer p.Changed()
	}
	if p.track == nil {
		mygo.NowPlaying.Clear()
		return
	}
	state := mygo.PlaybackPaused
	if p.playing {
		state = mygo.PlaybackPlaying
	}
	mygo.NowPlaying.SetPlayback(state, p.Position())
}

// announce shows the system the track, and its cover once loaded.
func (p *Player) announce(t *netease.Track, artwork []byte) {
	state := mygo.PlaybackPaused
	if p.playing {
		state = mygo.PlaybackPlaying
	}
	mygo.NowPlaying.Set(mygo.NowPlayingInfo{
		Title:    t.Name,
		Artist:   t.ArtistNames(),
		Album:    t.Album.Name,
		Artwork:  artwork,
		Duration: t.Length(),
		Position: p.Position(),
		State:    state,
		Commands: mygo.MediaPlay | mygo.MediaPause | mygo.MediaNext | mygo.MediaPrevious | mygo.MediaSeek,
	})
}

func (p *Player) ended() {
	if p.repeat == RepeatOne {
		p.out.Seek(0)
		p.out.Play()
		return
	}
	if p.at+1 >= len(p.order) && p.feed != nil {
		p.awaitFeed()
		return
	}
	if p.at+1 >= len(p.order) && p.repeat == RepeatOff {
		p.playing = false
		p.sync()
		return
	}
	p.Next()
}

// PlayQueue replaces the queue with tracks and plays the one at start.
func (p *Player) PlayQueue(tracks []netease.Track, start int) {
	if len(tracks) == 0 {
		return
	}
	p.endRadio()
	start = min(max(start, 0), len(tracks)-1)
	p.queue = append([]netease.Track(nil), tracks...)
	p.order = make([]int, len(p.queue))
	for i := range p.order {
		p.order[i] = i
	}
	if p.shuffle {
		p.shuffleOrder(start)
		p.load(0)
		return
	}
	p.load(start)
}

// PlayRadio plays tracks in their order, then goes on with those feed
// gives as the queue nears its end, until something else is played. The
// radio's name says which it is. feed calls got on the main thread.
func (p *Player) PlayRadio(name string, tracks []netease.Track, feed func(got func([]netease.Track))) {
	if len(tracks) == 0 {
		return
	}
	shuffle := p.shuffle
	p.shuffle = false
	p.PlayQueue(tracks, 0)
	p.shuffle = shuffle
	p.AttachRadio(name, feed)
}

// RadioFrom makes the track playing the first of a radio's queue, which
// goes on with tracks, then with those feed gives: without starting it
// over.
func (p *Player) RadioFrom(name string, tracks []netease.Track, feed func(got func([]netease.Track))) {
	if p.track == nil {
		return
	}
	p.queue, p.order, p.at = []netease.Track{*p.track}, []int{0}, 0
	p.appendTracks(tracks)
	p.AttachRadio(name, feed)
}

// AttachRadio makes the queue the radio name's, which feed goes on with,
// as after restoring a session that played it.
func (p *Player) AttachRadio(name string, feed func(got func([]netease.Track))) {
	p.endRadio()
	p.radio, p.feed = name, feed
	p.refill()
}

// EndRadio stops the radio playing: the song playing plays on, but the
// songs the radio queued after it go, and it adds no more.
func (p *Player) EndRadio() {
	if p.radio == "" && p.feed == nil {
		return
	}
	awaiting := p.awaiting
	p.endRadio()
	if p.at >= 0 && p.at+1 < len(p.order) {
		kept := make([]netease.Track, p.at+1)
		for i, k := range p.order[:p.at+1] {
			kept[i] = p.queue[k]
		}
		p.queue, p.order = kept, make([]int, len(kept))
		for i := range p.order {
			p.order[i] = i
		}
	}
	if awaiting {
		// At its end, waiting for more: nothing plays on.
		p.playing = false
		p.sync()
	}
}

func (p *Player) endRadio() {
	p.radioGen++
	p.radio, p.feed, p.feeding, p.awaiting = "", nil, false, false
}

// Radio returns the name of the radio playing, or "".
func (p *Player) Radio() string { return p.radio }

// refill asks the radio for more once the queue is near its end.
func (p *Player) refill() {
	if p.feed == nil || p.feeding || p.at < len(p.order)-2 {
		return
	}
	p.feeding = true
	gen := p.radioGen
	p.feed(func(more []netease.Track) {
		if gen != p.radioGen {
			return
		}
		p.feeding = false
		p.appendTracks(more)
		switch {
		case !p.awaiting:
		case p.at+1 < len(p.order):
			p.awaiting = false
			p.load(p.at + 1)
		default:
			// The radio has nothing more: it stops at its end.
			p.awaiting, p.playing = false, false
			p.err = "电台暂时没有更多歌曲"
			p.sync()
		}
	})
}

// awaitFeed waits at the end of a radio's queue for what its feed gives.
func (p *Player) awaitFeed() {
	p.awaiting = true
	p.refill()
}

// appendTracks adds tracks to the end of the queue, but those in it.
func (p *Player) appendTracks(tracks []netease.Track) {
	in := make(map[int64]bool, len(p.queue))
	for _, t := range p.queue {
		in[t.ID] = true
	}
	for _, t := range tracks {
		if !in[t.ID] {
			in[t.ID] = true
			p.order = append(p.order, len(p.queue))
			p.queue = append(p.queue, t)
		}
	}
}

// shuffleOrder shuffles the order, with first first.
func (p *Player) shuffleOrder(first int) {
	rand.Shuffle(len(p.order), func(i, j int) { p.order[i], p.order[j] = p.order[j], p.order[i] })
	for i, v := range p.order {
		if v == first {
			p.order[0], p.order[i] = p.order[i], p.order[0]
		}
	}
}

func (p *Player) ToggleShuffle() {
	p.shuffle = !p.shuffle
	if len(p.order) == 0 {
		return
	}
	cur := p.order[max(p.at, 0)]
	if p.shuffle {
		p.shuffleOrder(cur)
		p.at = 0
	} else {
		for i := range p.order {
			p.order[i] = i
		}
		p.at = cur
	}
}

func (p *Player) CycleRepeat() { p.repeat = (p.repeat + 1) % 3 }

// Current returns the track playing or loading, or nil.
func (p *Player) Current() *netease.Track { return p.track }

// Queue returns the tracks queued, in the order they were given.
func (p *Player) Queue() []netease.Track { return p.queue }

// Playing reports what the listener wants: false while paused.
func (p *Player) Playing() bool { return p.playing }

// Loading reports whether the current track is still loading.
func (p *Player) Loading() bool { return p.loading || p.awaiting }

// Ready reports whether the current track has loaded, to play.
func (p *Player) Ready() bool { return p.out != nil }

// Err returns why the current track did not play, or "".
func (p *Player) Err() string { return p.err }

// Stream returns what the current track streams, once loaded, or nil.
func (p *Player) Stream() *netease.Stream { return p.info }

func (p *Player) Shuffle() bool      { return p.shuffle }
func (p *Player) Repeat() RepeatMode { return p.repeat }

// Lyrics returns the current track's lyrics, and whether they have loaded.
func (p *Player) Lyrics() ([]lyrics.Line, bool) { return p.lyrics, p.lyricsLoaded }

// Downloaded returns the share of the current track downloaded so far, or
// false before it has started to.
func (p *Player) Downloaded() (float64, bool) {
	if p.stream == nil {
		return 0, false
	}
	dl, _ := p.stream.source()
	return dl.Progress(), true
}

// Audio is the current track's sound decoded apart from what plays, as
// OpenAudio opens it: interleaved stereo at its rate, read as it
// downloads.
type Audio interface {
	SampleRate() int
	Channels() int
	Read(dst []float32) (int, error)
	// SetPosition goes to a frame, of SampleRate a second.
	SetPosition(frame int64) error
	Close() error
}

// OpenAudio opens the current track's sound again, for decoding apart
// from what plays, as for transcribing an episode. It reads the same
// download, waiting for what has not come in, and fails once the track
// changes. It works on any goroutine once opened; close it when done.
func (p *Player) OpenAudio() (Audio, error) {
	if p.stream == nil || p.track == nil {
		return nil, errors.New("player: nothing loaded")
	}
	dl, _ := p.stream.source()
	return openShared(dl, p.track.Duration)
}

// PlayOrder returns the queue's indices in play order, and where the
// current track is among them, -1 for none.
func (p *Player) PlayOrder() ([]int, int) { return p.order, p.at }

func (p *Player) Next() {
	if len(p.order) == 0 {
		return
	}
	at := p.at + 1
	if at >= len(p.order) {
		if p.feed != nil {
			p.stop()
			p.awaitFeed()
			return
		}
		at = 0
	}
	p.load(at)
}

// Prev plays the track before this one, however far this one has played:
// once a few seconds in, going back to its start only kept the listener
// pressing twice.
func (p *Player) Prev() {
	if len(p.order) == 0 {
		return
	}
	at := p.at - 1
	if at < 0 {
		at = len(p.order) - 1
	}
	p.load(at)
}

// Jump plays the queue's track i.
func (p *Player) Jump(i int) {
	for k, v := range p.order {
		if v == i {
			p.load(k)
			return
		}
	}
}

// TogglePlay pauses, fading out, or plays, fading in.
func (p *Player) TogglePlay() {
	if p.out == nil {
		switch {
		case p.loading:
			// It plays once loaded, or not.
			p.playing = !p.playing
			p.sync()
		case len(p.order) > 0:
			p.tries = 0
			p.loadSkipping(max(p.at, 0), 0, true, p.pending)
		}
		return
	}
	out := p.out
	if p.playing {
		p.playing = false
		p.fade.fade(0, fadeOut, func() {
			p.post(func() {
				if p.out == out && !p.playing {
					out.Pause()
					p.sync()
				}
			})
		})
		p.sync()
		return
	}
	p.playing = true
	if !p.seeking && !out.Playing() {
		if out.Position() >= out.Duration()-time.Second {
			out.Seek(0)
		}
		out.Play()
	}
	p.fade.fade(1, fadeIn, nil)
	// A session picked up again was loaded paused: its next track may
	// still be fetched while this one plays.
	p.prefetch(p.at + 1)
	p.sync()
}

// seekGap is how long a seek waits for what plays to fade out, which the
// audio player does over 5 ms, as it pauses: within the next buffer or
// two it sends the system.
const seekGap = 30 * time.Millisecond

// Seek moves to t. Playing, it pauses and plays again around the jump,
// which the audio player fades over a few milliseconds, rather than
// cutting from one sound to the other with a click.
func (p *Player) Seek(t time.Duration) {
	if p.out == nil {
		p.pending = t
		return
	}
	out := p.out
	// Paused, it jumps at once; but a seek on its way paused it only for
	// the jump, and this one takes its place, playing on from there.
	if !out.Playing() && !p.seeking {
		out.Seek(t)
		p.sync()
		return
	}
	p.seekGen++
	gen := p.seekGen
	p.seeking, p.seekTo = true, t
	out.Pause()
	time.AfterFunc(seekGap, func() {
		p.post(func() {
			if gen != p.seekGen || p.out != out {
				return
			}
			p.seeking = false
			out.Seek(t)
			if p.playing || !p.fade.silent() {
				out.Play()
			}
			p.sync()
		})
	})
	p.sync()
}

func (p *Player) SetVolume(v float64) {
	p.volume = v
	p.applyGain()
}

// applyGain sets the volume of what plays: the listener's, which the ear
// hears in powers, and the track's gain, if normalizing.
func (p *Player) applyGain() {
	g := p.volume * p.volume
	if p.Normalize != nil && p.Normalize() {
		g *= p.gain
	}
	p.fade.setGain(g)
}

// normPreamp lifts normalized tracks, which NetEase brings to the quiet
// loudness of ReplayGain, nearer to how loud music plays elsewhere.
const normPreamp = 4

// normGain is the gain that brings a track as loud as others, as NetEase
// measured it, but never so loud that its peaks would clip.
func normGain(st *netease.Stream) float64 {
	if st == nil || st.Gain == 0 && st.Peak == 0 {
		return 1
	}
	g := math.Pow(10, (st.Gain+normPreamp)/20)
	if st.Peak > 0 {
		g = min(g, 1/st.Peak)
	}
	return g
}

func (p *Player) Position() time.Duration {
	switch {
	case p.out == nil:
		return p.pending
	case p.seeking:
		return p.seekTo
	}
	return p.out.Position()
}

func (p *Player) Duration() time.Duration {
	if p.track == nil {
		return 0
	}
	return p.track.Length()
}

// Levels returns the spectrum of what plays now.
func (p *Player) Levels() ([Bands]float32, bool) {
	if p.stream == nil || p.out == nil || !p.playing {
		return [Bands]float32{}, false
	}
	_, spec := p.stream.source()
	frame := int64(p.Position().Seconds() * float64(p.stream.SampleRate()))
	return spec.at(frame)
}

func (p *Player) stop() {
	if out := p.out; out != nil {
		// Paused first, it fades out over a few milliseconds, rather than
		// stopping with a click.
		out.Pause()
		time.AfterFunc(100*time.Millisecond, out.Close)
	}
	p.out, p.stream, p.info = nil, nil, nil
	p.seeking = false
	p.fade.attach(nil, 1)
}

// load plays the track at order[at], skipping those that cannot play.
func (p *Player) load(at int) {
	p.tries = 0
	p.loadSkipping(at, 0, true, 0)
}

// loadSkipping loads the track at order[at], to play or not, from from,
// skipping those that cannot play: skipped of them so far.
func (p *Player) loadSkipping(at, skipped int, play bool, from time.Duration) {
	t := p.queue[p.order[at]]
	p.stop()
	p.gen++
	gen := p.gen
	// What was fetched ahead plays at once, unless it is of another level
	// than the one asked for now.
	level := p.level()
	ahead := p.takeAhead(t.ID, level)
	ask := songURL
	p.at = at
	p.track = &t
	p.loading, p.playing, p.err = true, play, ""
	p.awaiting = false
	p.lyrics, p.lyricsLoaded = nil, false
	p.pending = from
	p.announce(&t, nil)
	p.refill()

	go func() {
		cover, err := netease.Fetch(netease.Sized(t.Album.PicURL, 600))
		if err == nil {
			p.post(func() {
				if gen == p.gen {
					p.announce(&t, cover)
				}
			})
		}
	}()

	go func() {
		lrc, trans, yrc, err := netease.Lyrics(t.ID)
		var lines []lyrics.Line
		if err == nil {
			lines = lyrics.Build(lrc, trans, yrc)
		}
		p.post(func() {
			if gen == p.gen {
				p.lyrics = lines
				p.lyricsLoaded = true
			}
		})
	}()
	go func() {
		info, s, err := p.open(t, level, ahead, ask)
		p.post(func() {
			if gen != p.gen {
				if s != nil {
					s.Close()
				}
				return
			}
			p.loading = false
			if err != nil {
				p.failed(at, skipped, t, from, err)
				return
			}
			p.stream, p.info = s, info
			p.gain = normGain(info)
			p.out = audio.NewPlayer(s)
			// From its middle, as a session picked up again, it fades in.
			lvl := 1.0
			if p.pending > 0 {
				lvl = 0
			}
			p.fade.attach(p.out, lvl)
			p.applyGain()
			p.out.OnEnd(func() {
				p.post(func() {
					if gen == p.gen {
						p.cutOff()
					}
				})
			})
			if p.pending > 0 {
				p.out.Seek(p.pending)
				p.pending = 0
			}
			if p.playing {
				p.out.Play()
				p.fade.fade(1, fadeIn, nil)
			}
			// The one after it is fetched while this one plays, so that
			// it starts at once when this one ends.
			p.prefetch(at + 1)
			p.sync()
		})
	}()
}

// failed handles a track that could not be loaded: tried again if it may
// come now, skipped if it cannot play at all, and else left where it is
// for the listener to play again.
func (p *Player) failed(at, skipped int, t netease.Track, from time.Duration, err error) {
	if err != netease.ErrUnavailable {
		// What did not come now may come again: a stream URL gone stale,
		// or a connection dropped, is asked for again a moment later.
		if p.tries < loadTries {
			p.retry(at, skipped, from)
			return
		}
		p.err = "加载失败，请重试"
		p.playing = false
		p.sync()
		return
	}
	if t.ProgramID != 0 {
		p.err = "这期节目需要购买才能收听"
	} else {
		p.err = "这首歌需要 VIP 才能播放"
	}
	// Skip what cannot play, unless nothing in the queue can.
	if skipped+1 < len(p.order) {
		next := at + 1
		if next >= len(p.order) {
			next = 0
		}
		p.tries = 0
		p.loadSkipping(next, skipped+1, p.playing, 0)
		return
	}
	p.playing = false
	p.sync()
}

// loadTries is how many times a track is tried again before it is left for
// the listener to play again.
const loadTries = 2

// retry loads the track at at again in a moment, still loading as it
// waits: a stream URL gone stale, or a connection dropped, may work once
// more.
func (p *Player) retry(at, skipped int, from time.Duration) {
	p.tries++
	p.loading = true
	gen, tries := p.gen, p.tries
	time.AfterFunc(retryGap(tries), func() {
		p.post(func() {
			if gen == p.gen {
				p.loadSkipping(at, skipped, p.playing, from)
			}
		})
	})
	p.sync()
}

// retryGap is how long the try after n tries waits: a moment longer each
// time, for a network that may be back by then.
func retryGap(n int) time.Duration { return (400 << (n - 1)) * time.Millisecond }

// cutOff runs as what plays comes to its end: the song's, or the
// download's, cut off, which is played again from where it stopped.
func (p *Player) cutOff() {
	if p.stream == nil || p.stream.cut() == nil {
		p.ended()
		return
	}
	if p.tries < loadTries {
		p.retry(p.at, 0, p.Position())
		return
	}
	// It cannot be had: stop where it stopped, for the listener to play
	// again.
	p.err = "播放中断，请重试"
	p.playing = false
	p.pending = p.Position()
	p.stop()
	p.sync()
}

// prefetch fetches the track at order[at] once this one has come in whole,
// into the cache, so that it starts at once when it is played: what it
// cannot fetch is fetched again then.
func (p *Player) prefetch(at int) {
	if at < 0 || at >= len(p.order) || !p.playing {
		p.dropAhead()
		return
	}
	t := p.queue[p.order[at]]
	level := p.level()
	if a := p.ahead; a != nil && a.id == t.ID && a.level == level {
		return // fetched already, or coming in
	}
	p.dropAhead()
	gen := p.gen
	ask := songURL
	// What plays comes in first: fetching the one after it sooner would
	// take the bytes of the song being played, which is what stutters.
	now := p.stream
	go func() {
		if now != nil {
			if playing, _ := now.source(); playing != nil {
				playing.waitFor(playing.size)
				if playing.Err() != nil {
					// Let go of, not come in whole: it is not this one's
					// turn to be fetched ahead.
					return
				}
			}
		}
		info, err := ask(t.ID, level)
		if err != nil || !p.Cache.Holds(info) {
			// Not a song the cache would keep: fetching it now would
			// spend its bytes for nothing, as it would come in vain.
			return
		}
		dl := p.Cache.download(t.ID, info)
		p.post(func() {
			if gen != p.gen || p.ahead != nil {
				dl.Close()
				return
			}
			p.ahead = &ahead{id: t.ID, level: level, dl: dl, info: info}
		})
	}()
}

// takeAhead returns the track fetched ahead, if it plays as id at level,
// and lets go of what was fetched for another track, or another level.
func (p *Player) takeAhead(id int64, level string) *ahead {
	a := p.ahead
	p.ahead = nil
	if a == nil {
		return nil
	}
	if a.id != id || a.level != level {
		a.dl.Close()
		return nil
	}
	return a
}

// dropAhead lets go of the track fetched ahead, if any.
func (p *Player) dropAhead() {
	a := p.ahead
	p.ahead = nil
	if a != nil {
		a.dl.Close()
	}
}

// level returns the stream level to load, as the settings say.
func (p *Player) level() string {
	if p.Quality != nil {
		return p.Quality()
	}
	return "exhigh"
}

// open opens track t's audio at level: from what was fetched ahead of it,
// kept in the cache, or else streamed into it, and kept once it has all
// come in. ask is where its URL comes from, taken on the main thread.
func (p *Player) open(t netease.Track, level string, ahead *ahead, ask asker) (*netease.Stream, stream, error) {
	dl, info, err := p.fetch(t, level, ahead, ask)
	if err != nil {
		return nil, nil, err
	}
	s, err := openStream(dl, t.Duration)
	if err != nil {
		dl.Close()
		return nil, nil, err
	}
	return info, s, nil
}

// fetch returns the download of track t at level, and what it streams: the
// one fetched ahead, the one the cache keeps, or one started now.
func (p *Player) fetch(t netease.Track, level string, ahead *ahead, ask asker) (*download, *netease.Stream, error) {
	if ahead != nil {
		if ahead.dl.Err() == nil {
			return ahead.dl, ahead.info, nil
		}
		// Cut off before it played: asked for again, from its start.
		ahead.dl.Close()
	}
	dl, info, ok := p.Cache.lookup(t.ID, level)
	if ok {
		return dl, info, nil
	}
	info, err := ask(t.ID, level)
	if err != nil {
		return nil, nil, err
	}
	return p.Cache.download(t.ID, info), info, nil
}

// Session is where playback was left, to pick up again.
type Session struct {
	Queue    []netease.Track `json:"queue"`
	Order    []int           `json:"order"`
	At       int             `json:"at"`
	Shuffle  bool            `json:"shuffle"`
	Repeat   RepeatMode      `json:"repeat"`
	Position int64           `json:"position"` // milliseconds
	Radio    string          `json:"radio,omitempty"`
}

// Session returns where playback is, or false with nothing queued.
func (p *Player) Session() (Session, bool) {
	if p.track == nil || p.at < 0 {
		return Session{}, false
	}
	return Session{
		Queue:    p.queue,
		Order:    p.order,
		At:       p.at,
		Shuffle:  p.shuffle,
		Repeat:   p.repeat,
		Position: p.Position().Milliseconds(),
		Radio:    p.radio,
	}, true
}

// Restore picks up a session where it was left, paused, and loads its
// track to play at once when asked.
func (p *Player) Restore(s Session) {
	n := len(s.Queue)
	if n == 0 || len(s.Order) != n || s.At < 0 || s.At >= n {
		return
	}
	seen := make([]bool, n)
	for _, i := range s.Order {
		if i < 0 || i >= n || seen[i] {
			return
		}
		seen[i] = true
	}
	p.queue, p.order = s.Queue, s.Order
	p.shuffle, p.repeat = s.Shuffle, s.Repeat%3
	p.tries = 0
	p.loadSkipping(s.At, 0, false, time.Duration(s.Position)*time.Millisecond)
}
