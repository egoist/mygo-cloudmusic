package player

import (
	"os"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

func TestStreamLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	st, err := netease.SongURL(2652820720, "exhigh")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	dl := startDownload(st.URL, st.Size, nil)
	s, err := openMP3Stream(dl, 278961)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("opened in", time.Since(start), s.rate, s.frames, s.start)
	buf := make([]float32, 4096)
	n, err := s.Read(buf)
	t.Log("read", n, err)
	if err := s.SetPosition(int64(s.rate) * 120); err != nil {
		t.Fatal(err, len(dl.data), dl.done, dl.err)
	}
	total := 0
	for i := 0; i < 200; i++ {
		n, err := s.Read(buf)
		total += n
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Log("after seek read", total, "in", time.Since(start), "progress", dl.Progress())
	b, ok := s.spec.at(int64(s.rate)*120 + 20000)
	t.Log(ok, b)
	s.Close()
}
