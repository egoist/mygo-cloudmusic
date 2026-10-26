package player

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"netease-music-mygo/internal/fnmusic"
	"netease-music-mygo/internal/netease"
)

// mp4Box wraps payload as a box of MP4.
func mp4Box(typ string, payload []byte) []byte {
	b := make([]byte, 8, 8+len(payload))
	binary.BigEndian.PutUint32(b, uint32(8+len(payload)))
	copy(b[4:], typ)
	return append(b, payload...)
}

// transcodingService is a Feiniu service that transcodes any track to the
// MP3 of testdata/seek.mp3, in pieces of 76 frames (just under two seconds)
// as fragments of MP4, and counts which pieces were asked for.
type transcodingService struct {
	*httptest.Server
	pieces int
	mu     sync.Mutex
	hits   map[int]int
}

func newTranscodingService(t *testing.T) (*transcodingService, []byte) {
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	// Cut on the frames' boundaries, after the ID3 tag.
	var frames [][]byte
	for p := int(id3Size(downloaded(b))); p+4 <= len(b); {
		n := frameSize(b[p:])
		if n == 0 || p+n > len(b) {
			break
		}
		frames = append(frames, b[p:p+n])
		p += n
	}
	const per = 76
	var pieces [][]byte
	var whole []byte
	for i := 0; i < len(frames); i += per {
		var piece []byte
		for _, f := range frames[i:min(i+per, len(frames))] {
			piece = append(piece, f...)
		}
		pieces = append(pieces, piece)
		whole = append(whole, piece...)
	}
	s := &transcodingService{pieces: len(pieces), hits: map[int]int{}}
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, data any) {
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	}
	mux.HandleFunc("/music/api/v1/user/password-login", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]string{"userToken": "t"})
	})
	mux.HandleFunc("/music/api/v1/track/transcode", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]string{"url": "/music/api/v1/track/hls/x/preset.m3u8"})
	})
	mux.HandleFunc("/music/api/v1/track/transcode/quit", func(w http.ResponseWriter, r *http.Request) { reply(w, nil) })
	mux.HandleFunc("/music/api/v1/track/hls/x/preset.m3u8", func(w http.ResponseWriter, r *http.Request) {
		var sb strings.Builder
		sb.WriteString("#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n")
		for i, p := range pieces {
			var n int
			for q := 0; q < len(p); q += frameSize(p[q:]) {
				n++
			}
			fmt.Fprintf(&sb, "#EXTINF:%.6f,\n%05d.m4s\n", float64(n*1152)/44100, i)
		}
		sb.WriteString("#EXT-X-ENDLIST\n")
		w.Write([]byte(sb.String()))
	})
	for i, p := range pieces {
		mux.HandleFunc(fmt.Sprintf("/music/api/v1/track/hls/x/%05d.m4s", i), func(w http.ResponseWriter, r *http.Request) {
			s.mu.Lock()
			s.hits[i]++
			s.mu.Unlock()
			w.Write(append(mp4Box("styp", []byte("msdh")), append(mp4Box("moof", []byte("....")), mp4Box("mdat", p)...)...))
		})
	}
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s, whole
}

func (s *transcodingService) fetched() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []int
	for i := range s.pieces {
		if s.hits[i] > 0 {
			out = append(out, i)
		}
	}
	return out
}

func openTranscodedTestStream(t *testing.T, svc *transcodingService) *hlsStream {
	c, err := fnmusic.New(fnmusic.Config{Address: svc.URL, Username: "me", PassHash: fnmusic.HashPassword("x")})
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.TranscodeHLS(context.Background(), &netease.Track{Ref: "g", Format: "m4a", Source: fnmusic.Source})
	if err != nil {
		t.Fatal(err)
	}
	s, err := openHLSStream(h)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// closestDiff is maxDiff of a against b shifted by a few samples either way,
// the least: a piece's start is where its length says, to within a sample.
func closestDiff(a, b []float32) float64 {
	best := 1e9
	for shift := -4; shift <= 4; shift++ {
		x, y := a, b
		switch {
		case shift > 0:
			y = b[min(shift, len(b)):]
		case shift < 0:
			x = a[min(-shift, len(a)):]
		}
		best = min(best, maxDiff(x, y))
	}
	return best
}

// A song the service transcodes to pieces decodes from the first to the last
// as the MP3 they make up does, and is as long as the pieces say.
func TestHLSStreamDecodesWhole(t *testing.T) {
	svc, whole := newTranscodingService(t)
	ref := decodeAll(t, whole)
	s := openTranscodedTestStream(t, svc)
	if d := s.frames - int64(len(ref)/2); d < -2304 || d > 2304 {
		t.Errorf("%d frames long, the audio has %d", s.frames, len(ref)/2)
	}
	var got []float32
	buf := make([]float32, 4096)
	for {
		n, err := s.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil || n == 0 {
			break
		}
	}
	if len(got) != len(ref) || maxDiff(got, ref) > 1e-6 {
		t.Errorf("decoded %d samples, off by %.5f of %d", len(got), maxDiff(got, ref), len(ref))
	}
	if p, _ := s.prog.Progress(), 0; p != 1 {
		t.Errorf("progress %v after reading it all", p)
	}
}

// A seek asks for the piece it lands in and the one before it, which the
// decoder needs for the bits an MP3 frame borrows, not for all that come
// before: it lands where it should, and is quick for it.
func TestHLSStreamSeeksToAPiece(t *testing.T) {
	svc, whole := newTranscodingService(t)
	ref := decodeAll(t, whole)
	s := openTranscodedTestStream(t, svc)
	buf := make([]float32, 4096)
	s.Read(buf)
	svc.mu.Lock()
	svc.hits = map[int]int{}
	svc.mu.Unlock()

	frame := s.frames * 3 / 4
	if err := s.SetPosition(frame); err != nil {
		t.Fatal(err)
	}
	n, err := s.Read(buf)
	if err != nil || n == 0 {
		t.Fatal(n, err)
	}
	if d := closestDiff(buf[:n], ref[frame*2:]); d > 1e-4 {
		t.Errorf("landed off: differs by %.4f", d)
	}
	for _, i := range svc.fetched() {
		if i < svc.pieces*3/4-2 {
			t.Errorf("piece %d of %d was asked for by a seek to three quarters", i, svc.pieces)
		}
	}
	// Seeks near the start and the very end, and back.
	for _, f := range []int64{0, 5000, s.frames - 3000, frame / 2} {
		if err := s.SetPosition(f); err != nil {
			t.Fatal(err)
		}
		n, _ := s.Read(buf)
		if n == 0 {
			continue
		}
		if d := closestDiff(buf[:n], ref[f*2:]); d > 1e-4 {
			t.Errorf("seek to frame %d: differs by %.4f", f, d)
		}
	}
}
