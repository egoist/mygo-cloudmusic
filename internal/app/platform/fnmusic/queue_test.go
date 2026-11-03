package fnmusic

import (
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform/paging"
)

// randomPages keeps the already visible physical page first while using every page once.
func TestRandomPagesKeepsBaseAndPermutes(t *testing.T) {
	got := randomPages(250, 42, 0)
	if len(got) != 3 || got[0] != 0 {
		t.Fatalf("pages = %#v", got)
	}
	seen := map[int]bool{}
	for _, pg := range got {
		if seen[pg] {
			t.Fatalf("duplicate page %d", pg)
		}
		seen[pg] = true
	}
}

type queuePlayer struct {
	tracks []netease.Track
	at     int
	feed   func(func([]netease.Track))
	ready  chan struct{}
}

func (p *queuePlayer) Current() *netease.Track        { return nil }
func (p *queuePlayer) Playing() bool                  { return false }
func (p *queuePlayer) Loading() bool                  { return false }
func (p *queuePlayer) Err() string                    { return "" }
func (p *queuePlayer) TogglePlay()                    {}
func (p *queuePlayer) Next()                          {}
func (p *queuePlayer) Queue() []netease.Track         { return nil }
func (p *queuePlayer) PlayOrder() ([]int, int)        { return nil, -1 }
func (p *queuePlayer) PlayQueue([]netease.Track, int) {}
func (p *queuePlayer) AppendQueue([]netease.Track)    {}
func (p *queuePlayer) PlayList(ts []netease.Track, at int, f func(func([]netease.Track))) {
	p.tracks, p.at, p.feed = ts, at, f
	if p.ready != nil {
		close(p.ready)
	}
}
func (p *queuePlayer) PlayRadio(string, []netease.Track, func(func([]netease.Track))) {}
func (p *queuePlayer) EndRadio()                                                      {}
func (p *queuePlayer) Radio() string                                                  { return "" }
func (p *queuePlayer) SetShuffle(bool)                                                {}

// ShuffleList starts with two physical pages, so the first audible batch is large enough without loading the catalog.
func TestShuffleListUsesTwoPages(t *testing.T) {
	tracks := func(page int) ([]netease.Track, int, error) {
		out := make([]netease.Track, 0, 100)
		for i := 0; i < 100 && (page-1)*100+i < 250; i++ {
			out = append(out, netease.Track{ID: int64((page-1)*100 + i + 1), Ref: "x"})
		}
		return out, 250, nil
	}
	pager := paging.New(100, 0, nil, tracks)
	l := &List{Pages: pager}
	p := &queuePlayer{ready: make(chan struct{})}
	ShuffleList(p, l, nil)
	select {
	case <-p.ready:
	case <-time.After(time.Second):
		t.Fatal("shuffle did not start")
	}
	if len(p.tracks) <= 100 || len(p.tracks) > 200 {
		t.Fatalf("first shuffled batch = %d tracks", len(p.tracks))
	}
	if p.at != 0 {
		t.Fatalf("start offset = %d", p.at)
	}
}
