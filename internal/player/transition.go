package player

import "time"

func (p *Player) audioPosition(t time.Duration) time.Duration {
	return time.Duration(float64(p.partStart)*float64(time.Second)/outputRate) + t
}

func (s *sequence) setNormalize(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	gain := func(p *pcmTrack) {
		g := 1.0
		if on {
			g = normGain(p.info)
		}
		p.setGain(g)
	}
	for _, part := range s.parts {
		gain(part.track)
	}
	if s.next != nil {
		gain(s.next)
	}
}

// Queue edits may happen after the decoder read ahead into the next song.
// Flush that decoded lookahead when replacing it; ordinary transitions
// never seek, pause, or recreate the audio output.
func (p *Player) queueAudio(track *pcmTrack) {
	if p.sequence == nil {
		return
	}
	if track != nil {
		g := 1.0
		if p.Normalize != nil && p.Normalize() {
			g = normGain(track.info)
		}
		track.setGain(g)
	}
	p.sequence.queue(track)
	if p.sequence.ahead(p.partStart) {
		p.out.Seek(p.out.Position())
	}
}

// pollAudio updates the visible song only once its first sample is heard.
// The audio has already joined the tracks without waiting for this timer.
func (p *Player) pollAudio(seq *sequence) {
	if p.sequence != seq || p.seeking {
		return
	}
	frame := int64((p.out.Position().Seconds() * outputRate) + 0.5)
	part, pending, ended := seq.state(frame)
	if part.start != p.partStart {
		p.partStart = part.start
		p.gen++
		if p.nextJob != nil && p.nextJob.pcm == part.track {
			p.nextJob.close()
			p.nextJob = nil
		}
		// The prepared successor follows the current order, including repeats.
		if p.repeat != RepeatOne {
			p.at = (p.at + 1) % len(p.order)
		}
		t := part.track.track
		p.track, p.stream, p.info = &t, part.track.raw, part.track.info
		p.gain = normGain(p.info)
		p.lyrics, p.lyricsLoaded = nil, false
		p.err = ""
		seq.discardBefore(part.start)
		p.announce(&t, nil)
		p.loadMetadata(t, p.gen)
		p.refill()
		p.prepareNext()
		p.sync()
	}
	if ended {
		if p.playing {
			p.ended()
		}
		return
	}
	if !pending || !p.playing || p.polling {
		return
	}
	p.polling = true
	time.AfterFunc(10*time.Millisecond, func() {
		p.post(func() {
			if p.sequence != seq {
				return
			}
			p.polling = false
			p.pollAudio(seq)
		})
	})
}
