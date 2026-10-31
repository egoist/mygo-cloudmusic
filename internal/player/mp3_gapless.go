package player

import (
	"encoding/binary"
	"io"
)

// mp3Gapless reads the Xing/Info frame and its LAME delay/padding fields.
// The metadata frame is not audio. MPEG synthesis adds 529 samples beyond
// the encoder's delay (also accounted for by FFmpeg's MP3 demuxer).
func mp3Gapless(dl *download, start int64) (audioStart, lead, length, samplesPerFrame int64, ok bool) {
	dl.waitFor(start + 4)
	h := dl.peek(start, 4)
	size := frameSize(h)
	if size == 0 {
		return
	}
	dl.waitFor(start + int64(size))
	b := dl.peek(start, size)
	if len(b) != size {
		return
	}
	version := h[1] >> 3 & 3
	mono := h[3]>>6 == 3
	side := 32
	samplesPerFrame = 1152
	if version != 3 {
		side = 17
		samplesPerFrame = 576
	}
	if mono {
		if version == 3 {
			side = 17
		} else {
			side = 9
		}
	}
	at := 4 + side
	if at+12 > len(b) {
		return
	}
	if string(b[at:at+4]) != "Xing" && string(b[at:at+4]) != "Info" {
		return
	}
	flags := binary.BigEndian.Uint32(b[at+4:])
	at += 8
	if flags&1 == 0 {
		return
	}
	count := int64(binary.BigEndian.Uint32(b[at:]))
	at += 4
	if flags&2 != 0 {
		at += 4
	}
	if flags&4 != 0 {
		at += 100
	}
	if flags&8 != 0 {
		at += 4
	}
	if at+24 > len(b) {
		return
	}
	encoder := string(b[at : at+4])
	if encoder != "LAME" && encoder != "Lavc" && encoder != "Lavf" {
		return
	}
	delay := int64(b[at+21])<<4 | int64(b[at+22]>>4)
	padding := int64(b[at+22]&15)<<8 | int64(b[at+23])
	length = count*samplesPerFrame - delay - padding
	if count == 0 || length <= 0 || padding < 529 {
		return 0, 0, 0, 0, false
	}
	return start + int64(size), delay + 529, length, samplesPerFrame, true
}

// taggedPosition locates a compressed frame with enough preroll to rebuild
// the MP3 bit reservoir, then skips exactly to the requested audible sample.
// The lazily built index also works for variable bitrate files.
func (s *mp3Stream) taggedPosition(frame int64) (int64, int, error) {
	target := frame + s.lead
	first := max(target/s.samplesPerFrame-4, 0)
	if len(s.offsets) == 0 {
		s.offsets = []int64{s.start}
	}
	for int64(len(s.offsets)) <= first {
		off := s.offsets[len(s.offsets)-1]
		if s.dl.waitFor(off+4) < off+4 {
			return 0, 0, io.ErrUnexpectedEOF
		}
		size := frameSize(s.dl.peek(off, 4))
		if size == 0 {
			return 0, 0, io.ErrUnexpectedEOF
		}
		s.offsets = append(s.offsets, off+int64(size))
	}
	return s.offsets[first], int(target-first*s.samplesPerFrame) * 4, nil
}
