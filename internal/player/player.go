package player

import (
	"math/rand/v2"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	_ "github.com/egoist/mygo/audio/mp3"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

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

	queue   []netease.Track
	order   []int // the queue's indices in play order: shuffled or not
	at      int   // where in order the current track is, -1 for none
	shuffle bool
	repeat  RepeatMode
	volume  float64

	track   *netease.Track
	out     *audio.Player
	stream  *mp3Stream
	loading bool
	playing bool // what the listener wants: false while paused
	err     string
	gen     int // counts loads, to drop the results of stale ones

	lyrics       []lyrics.Line
	lyricsLoaded bool

	// pending holds a seek asked for while the track loads.
	pending time.Duration
}

func New(post func(func())) *Player {
	p := &Player{post: post, at: -1, volume: 0.8}
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

// sync shows the system where playback is.
func (p *Player) sync() {
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
func (p *Player) Loading() bool { return p.loading }

// Ready reports whether the current track has loaded, to play.
func (p *Player) Ready() bool { return p.out != nil }

// Err returns why the current track did not play, or "".
func (p *Player) Err() string { return p.err }

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
	return p.stream.dl.Progress(), true
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
		at = 0
	}
	p.load(at)
}

func (p *Player) Prev() {
	if len(p.order) == 0 {
		return
	}
	// As every player does: the start of this track, then the one before.
	if p.Position() > 3*time.Second {
		p.Seek(0)
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

func (p *Player) TogglePlay() {
	if p.out == nil {
		if p.track == nil && len(p.order) > 0 {
			p.load(max(p.at, 0))
		}
		return
	}
	if p.playing {
		p.out.Pause()
		p.playing = false
	} else {
		if !p.out.Playing() && p.out.Position() >= p.out.Duration()-time.Second {
			p.out.Seek(0)
		}
		p.out.Play()
		p.playing = true
	}
	p.sync()
}

func (p *Player) Seek(t time.Duration) {
	if p.out == nil {
		p.pending = t
		return
	}
	p.out.Seek(t)
	if p.playing {
		p.out.Play()
	}
	p.sync()
}

func (p *Player) SetVolume(v float64) {
	p.volume = v
	if p.out != nil {
		p.out.SetVolume(v * v) // the ear hears loudness in powers
	}
}

func (p *Player) Position() time.Duration {
	if p.out == nil {
		return p.pending
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
	frame := int64(p.out.Position().Seconds() * float64(p.stream.rate))
	return p.stream.spec.at(frame)
}

func (p *Player) stop() {
	if p.out != nil {
		p.out.Close()
	}
	p.out, p.stream = nil, nil
}

// load plays the track at order[at], skipping those that cannot play.
func (p *Player) load(at int) {
	p.loadSkipping(at, 0)
}

func (p *Player) loadSkipping(at, skipped int) {
	p.stop()
	p.gen++
	gen := p.gen
	p.at = at
	t := p.queue[p.order[at]]
	p.track = &t
	p.loading, p.playing, p.err = true, true, ""
	p.lyrics, p.lyricsLoaded = nil, false
	p.pending = 0
	p.announce(&t, nil)

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
		level := "exhigh"
		if p.Quality != nil {
			level = p.Quality()
		}
		u, size, err := netease.SongURL(t.ID, level)
		var s *mp3Stream
		if err == nil {
			dl := startDownload(u, size)
			s, err = openMP3Stream(dl, t.Duration)
			if err != nil {
				dl.Close()
			}
		}
		p.post(func() {
			if gen != p.gen {
				if s != nil {
					s.Close()
				}
				return
			}
			p.loading = false
			if err != nil {
				p.err = err.Error()
				if err == netease.ErrUnavailable {
					p.err = "这首歌需要 VIP 才能播放"
				}
				// Skip what cannot play, unless nothing in the queue can.
				if skipped+1 < len(p.order) {
					next := at + 1
					if next >= len(p.order) {
						next = 0
					}
					p.loadSkipping(next, skipped+1)
				} else {
					p.playing = false
					p.sync()
				}
				return
			}
			p.stream = s
			p.out = audio.NewPlayer(s)
			p.out.SetVolume(p.volume * p.volume)
			p.out.OnEnd(func() {
				p.post(func() {
					if gen == p.gen {
						p.ended()
					}
				})
			})
			if p.pending > 0 {
				p.out.Seek(p.pending)
				p.pending = 0
			}
			if p.playing {
				p.out.Play()
			}
			p.sync()
		})
	}()
}
