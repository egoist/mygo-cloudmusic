package app

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// testPager is a pager of 1000 numbers, 100 to a page, that records which
// pages were asked for and gives their replies when told to.
type testPager struct {
	*pager[int]
	mu      sync.Mutex
	asked   []int
	replies chan func()
	total   int
	fail    map[int]bool
}

func newTestPager() *testPager {
	t := &testPager{replies: make(chan func(), 64), total: 1000, fail: map[int]bool{}}
	t.pager = newPager(100, 2, func(f func()) { t.replies <- f }, func(page int) ([]int, int, error) {
		t.mu.Lock()
		t.asked = append(t.asked, page)
		fail, total := t.fail[page], t.total
		t.mu.Unlock()
		if fail {
			return nil, 0, errors.New("boom")
		}
		items := make([]int, 0, 100)
		for i := (page - 1) * 100; i < min(page*100, total); i++ {
			items = append(items, i)
		}
		return items, total, nil
	})
	return t
}

// drain runs n replies, as the main thread would.
func (t *testPager) drain(n int) {
	for range n {
		select {
		case f := <-t.replies:
			f()
		case <-time.After(5 * time.Second):
			panic("no reply")
		}
	}
}

func (t *testPager) pagesAsked() []int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]int(nil), t.asked...)
}

// A list shows its first page without asking for the rest, and asks for
// the pages ahead of a row only as it is wanted: a list of thousands costs
// one request to open, not all of them.
func TestPagerAsksOnlyForWhatIsWanted(t *testing.T) {
	p := newTestPager()
	if p.at(0) != nil {
		t.Fatal("a row before any page is in")
	}
	p.drain(1)
	if got := p.pagesAsked(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("asked for %v before the length was known", got)
	}
	// Row 0 is in. Wanting it asks for the pages ahead: 2 and 3.
	if v := p.at(0); v == nil || *v != 0 {
		t.Fatal("row 0 not in")
	}
	p.drain(2)
	if got := p.pagesAsked(); len(got) != 3 {
		t.Fatalf("asked for %v, want pages 1 to 3", got)
	}
	if p.len() != 1000 {
		t.Errorf("length %d", p.len())
	}
	// Far down, a page the scroll lands on, and the two after it.
	p.at(750)
	p.drain(3)
	got := map[int]bool{}
	for _, g := range p.pagesAsked() {
		got[g] = true
	}
	for _, want := range []int{8, 9, 10} {
		if !got[want] {
			t.Errorf("page %d not asked for: %v", want, p.pagesAsked())
		}
	}
	if got[5] || got[6] {
		t.Errorf("pages scrolled past were asked for: %v", p.pagesAsked())
	}
	if v := p.at(750); v == nil || *v != 750 {
		t.Error("row 750 not in")
	}
	// A page is asked for once.
	n := len(p.pagesAsked())
	p.at(750)
	p.at(751)
	if len(p.pagesAsked()) != n {
		t.Error("a page was asked for twice")
	}
}

// A page that failed is not asked for again each frame, which would hammer
// a service that is down, but when the listener asks.
func TestPagerRetry(t *testing.T) {
	p := newTestPager()
	p.fail[1] = true
	p.at(0)
	p.drain(1)
	if !p.failed() || p.err == "" {
		t.Fatal("the failure went unnoticed")
	}
	p.at(0)
	p.at(0)
	if n := len(p.pagesAsked()); n != 1 {
		t.Errorf("asked %d times while failed", n)
	}
	p.fail[1] = false
	p.retry()
	p.at(0)
	p.drain(1)
	if p.failed() || p.at(0) == nil {
		t.Error("not loaded after retrying")
	}
}

// Regression guard for a list that changed on the service as it was read:
// pages of the old list arriving after the new one's length was known put
// rows of the old one among the new.
func TestPagerDropsRepliesOfAChangedList(t *testing.T) {
	p := newTestPager()
	p.at(0)
	p.drain(1)
	p.at(0) // asks for pages 2 and 3, still on the way
	p.mu.Lock()
	p.total = 1200
	p.mu.Unlock()
	// The first page again, now of 1200: the list changed.
	p.land(0, make([]int, 100), 1200)
	p.drain(2) // the old replies, of 1000
	if p.len() != 1200 {
		t.Fatalf("length %d", p.len())
	}
	if p.state[1] == pageHave || p.state[2] == pageHave {
		t.Error("pages of the old list were kept")
	}
}

// What a queue starts from is the pages in around the row clicked, and
// where the row is in them.
func TestPagerAround(t *testing.T) {
	p := newTestPager()
	p.at(0)
	p.drain(1)
	p.at(0)
	p.drain(2)
	items, at, end := p.around(150)
	if len(items) != 300 || at != 150 || end != 300 {
		t.Fatalf("%d items, at %d, end %d", len(items), at, end)
	}
	if items[at] != 150 {
		t.Errorf("row 150 is %d", items[at])
	}
	if items, _, _ := p.around(700); items != nil {
		t.Error("a run around a row not in")
	}
}

// Shuffling needs every page: they are all asked for, and then comes the
// answer, or that one failed.
func TestPagerWhole(t *testing.T) {
	p := newTestPager()
	var ok *bool
	p.whole(func(v bool) { ok = &v })
	p.drain(1)
	p.drain(9)
	if ok == nil || !*ok || !p.loaded() {
		t.Fatalf("not whole: %v", ok)
	}
	q := newTestPager()
	q.fail[4] = true
	ok = nil
	q.whole(func(v bool) { ok = &v })
	for i := 0; i < 20 && ok == nil; i++ {
		q.drain(1)
	}
	if ok == nil || *ok {
		t.Error("a failed page was not told")
	}
}

func TestPagerEmpty(t *testing.T) {
	p := newTestPager()
	p.total = 0
	p.at(0)
	p.drain(1)
	if p.len() != 0 || !p.loaded() || p.at(0) != nil {
		t.Errorf("len %d, loaded %v", p.len(), p.loaded())
	}
}
