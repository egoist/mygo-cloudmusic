package player

import (
	"netease-music-mygo/internal/netease"
)

// A trackLoad owns a song until playback takes it. Its fields, including
// the completion callback, belong to the main thread.
type trackLoad struct {
	id        int64
	level     string
	info      *netease.Stream
	stream    stream
	pcm       *pcmTrack
	released  bool
	err       error
	ready     bool
	cancelled bool
	done      func()
}

var openTrack = (*Player).open

func (p *Player) quality() string {
	if p.Quality != nil {
		return p.Quality()
	}
	return "exhigh"
}

func (p *Player) startLoad(t netease.Track, level string) *trackLoad {
	j := &trackLoad{id: t.ID, level: level}
	open := openTrack
	go func() {
		info, s, err := open(p, t, level)
		var pcm *pcmTrack
		if err == nil {
			pcm, err = preparePCM(s, t, info)
		}
		p.post(func() {
			j.info, j.stream, j.pcm, j.err, j.ready = info, s, pcm, err, true
			if j.cancelled {
				j.close()
			} else if j.done != nil {
				j.done()
			}
		})
	}()
	return j
}

func (j *trackLoad) use(done func()) {
	j.done = done
	if j.ready {
		done()
	}
}

func (j *trackLoad) close() {
	j.cancelled = true
	j.done = nil
	if j.pcm != nil && !j.released {
		j.released = true
		j.pcm.release()
	}
}

// takeNext transfers the prepared song into the foreground. A skip while
// it is opening waits for that same request instead of starting another.
func (p *Player) takeNext(t netease.Track, level string) *trackLoad {
	j := p.nextJob
	p.nextJob = nil
	if j != nil {
		if j.id == t.ID && j.level == level {
			return j
		}
		j.close()
	}
	return p.startLoad(t, level)
}

// prepareNext keeps just the next song ready, following the actual play
// order. Queue and repeat changes replace a preparation that no longer fits.
func (p *Player) prepareNext() {
	if p.out == nil || p.stream == nil {
		return
	}
	at := p.at + 1
	if at >= len(p.order) && p.repeat == RepeatAll && p.feed == nil {
		at = 0
	}
	if p.repeat == RepeatOne {
		at = p.at
	}
	if at >= len(p.order) {
		p.queueAudio(nil)
		if p.nextJob != nil {
			p.nextJob.close()
			p.nextJob = nil
		}
		return
	}
	t, level := p.queue[p.order[at]], p.quality()
	if j := p.nextJob; j != nil {
		if j.id == t.ID && j.level == level {
			return
		}
		p.queueAudio(nil)
		j.close()
	}
	p.nextJob = p.startLoad(t, level)
	job := p.nextJob
	job.use(func() {
		if job.err == nil {
			p.queueAudio(job.pcm)
		}
	})
}
