package app

// pager is a long list of the Feiniu service's, loaded a page at a time as
// the listener scrolls to it: the page of a row that is wanted, and the
// next few ahead of it, so that a scroll at a reading pace never meets a
// row not loaded. A list of thousands costs one request to show, not
// all of them.
//
// A pager is read and changed on the main thread only: the requests run on
// goroutines, and their replies come back through post.
type pager[T any] struct {
	size, ahead int
	fetch       func(page int) ([]T, int, error) // page from 1: its items, and the list's total
	post        func(func())

	total int // -1 until the first page is in
	items []T // total long once known, as loaded as have says
	state []pageState
	err   string // why the last page that failed did, "" if none
	gen   int    // changes when the list starts over, to drop late replies
	// landed is called on the main thread as a page comes in, with the
	// index of its first item and the items.
	landed func(from int, items []T)
	// first is called once, as the first page lands or fails.
	first func(ok bool)
	all   func(ok bool) // waits for every page, see whole
	// inflight is how many pages are asked for, of at most limit: a list
	// asked for whole does not put a service to a hundred requests at once.
	inflight, limit int
	waiters         map[int][]func(ok bool)
}

type pageState byte

const (
	pageNone pageState = iota
	pageLoading
	pageHave
	pageFailed
)

func newPager[T any](size, ahead int, post func(func()), fetch func(page int) ([]T, int, error)) *pager[T] {
	return &pager[T]{size: size, ahead: ahead, post: post, fetch: fetch, total: -1, state: make([]pageState, 1), limit: 6}
}

// pages returns how many pages the list has, 1 until the total is known,
// and for an empty list, whose one page is empty.
func (p *pager[T]) pages() int {
	if p.total < 0 {
		return 1
	}
	return max(1, (p.total+p.size-1)/p.size)
}

// len is how many items the list has, 0 until it is known.
func (p *pager[T]) len() int { return max(p.total, 0) }

// at returns item i, nil if its page is not in. It asks for that page, and
// those ahead of it, if they are not.
func (p *pager[T]) at(i int) *T {
	if i < 0 || p.total >= 0 && i >= p.total {
		return nil
	}
	p.want(i / p.size)
	if p.total < 0 || p.state[i/p.size] != pageHave {
		return nil
	}
	return &p.items[i]
}

// want asks for page pg (from 0) and the pages ahead of it.
func (p *pager[T]) want(pg int) {
	for k := 0; k <= p.ahead; k++ {
		p.load(pg + k)
	}
}

// load asks for page pg (from 0) unless it is in, asked for, or failed: a
// page that failed is asked for again by retry. Before the list's length
// is known only the first page can be asked for.
func (p *pager[T]) load(pg int) {
	if pg < 0 || pg >= p.pages() || p.state[pg] != pageNone || p.inflight >= p.limit {
		return
	}
	p.state[pg] = pageLoading
	p.inflight++
	gen := p.gen
	go func() {
		items, total, err := p.fetch(pg + 1)
		p.post(func() {
			p.inflight--
			if gen != p.gen {
				p.progress()
				return
			}
			if err != nil {
				p.fail(pg, err)
				return
			}
			p.land(pg, items, total)
		})
	}()
}

// land puts page pg (from 0) in. The first to land says how long the list
// is; one that says another length is of a list that changed on the
// service as it was read, so what came before is dropped, and its replies
// still on the way with it.
func (p *pager[T]) land(pg int, items []T, total int) {
	if p.total != total {
		if p.total >= 0 {
			p.gen++
		}
		p.total = total
		p.items = make([]T, total)
		p.state = make([]pageState, p.pages())
	}
	if pg >= len(p.state) {
		return
	}
	from := min(pg*p.size, total)
	n := copy(p.items[from:], items)
	p.state[pg] = pageHave
	p.err = ""
	if p.landed != nil {
		p.landed(from, p.items[from:from+n])
	}
	if pg == 0 && p.first != nil {
		f := p.first
		p.first = nil
		f(true)
	}
	p.wake(pg, true)
	p.progress()
}

// when calls then once page pg (from 0) is in, asking for it if it is not,
// with false if it cannot be had.
func (p *pager[T]) when(pg int, then func(ok bool)) {
	if pg >= 0 && pg < len(p.state) && p.state[pg] == pageHave {
		then(true)
		return
	}
	// A page far into a list not yet read: how long the list is, which the
	// first page tells, comes first, as no page can be asked for before.
	if p.total < 0 && pg > 0 {
		p.when(0, func(ok bool) {
			if !ok {
				then(false)
				return
			}
			p.when(pg, then)
		})
		return
	}
	if p.waiters == nil {
		p.waiters = map[int][]func(bool){}
	}
	p.waiters[pg] = append(p.waiters[pg], then)
	if pg < len(p.state) && p.state[pg] == pageFailed {
		p.state[pg] = pageNone
	}
	p.load(pg)
}

// wake tells those waiting for page pg how it came out.
func (p *pager[T]) wake(pg int, ok bool) {
	ws := p.waiters[pg]
	delete(p.waiters, pg)
	for _, w := range ws {
		w(ok)
	}
}

// fail records that page pg (from 0) could not be had.
func (p *pager[T]) fail(pg int, err error) {
	p.err = err.Error()
	if pg < len(p.state) {
		p.state[pg] = pageFailed
	}
	if pg == 0 && p.first != nil {
		f := p.first
		p.first = nil
		f(false)
	}
	p.wake(pg, false)
	p.progress()
}

// failedAt reports whether the page of row i failed, not yet asked for
// again.
func (p *pager[T]) failedAt(i int) bool {
	return i >= 0 && i/p.size < len(p.state) && p.state[i/p.size] == pageFailed
}

// failed reports whether a page failed, not yet asked for again.
func (p *pager[T]) failed() bool {
	for _, s := range p.state {
		if s == pageFailed {
			return true
		}
	}
	return false
}

// retry asks again for the pages that failed.
func (p *pager[T]) retry() {
	for i, s := range p.state {
		if s == pageFailed {
			p.state[i] = pageNone
		}
	}
	p.err = ""
}

// indexOf returns the position of the first item among those in that match,
// or -1, without asking for any page.
func (p *pager[T]) indexOf(match func(*T) bool) int {
	for pg, s := range p.state {
		if s != pageHave {
			continue
		}
		for i := pg * p.size; i < min((pg+1)*p.size, p.total); i++ {
			if match(&p.items[i]) {
				return i
			}
		}
	}
	return -1
}

// removeAt takes item i out of the list, as the list is shorter by it on the
// service: the pages from its own on are asked for again, as what is in them
// has moved up by one, and what was on its way is let go of.
func (p *pager[T]) removeAt(i int) {
	if i < 0 || i >= p.total {
		return
	}
	copy(p.items[i:], p.items[i+1:])
	p.total--
	p.items = p.items[:p.total]
	p.gen++
	p.inflight = 0
	if n := p.pages(); n < len(p.state) {
		p.state = p.state[:n]
	}
	for k := i / p.size; k < len(p.state); k++ {
		p.state[k] = pageNone
	}
	p.waiters = nil
}

// loaded reports whether every page is in.
func (p *pager[T]) loaded() bool {
	if p.total < 0 {
		return false
	}
	for _, s := range p.state {
		if s != pageHave {
			return false
		}
	}
	return true
}

// around returns the items that are in, in a run with item i: from the
// first of the pages in before it that follow one another, to the end of
// those after; and where i is in them, and the index the run ends at. A
// queue starts from it, to go on with the pages after as they come.
func (p *pager[T]) around(i int) (items []T, at, end int) {
	if p.total < 0 || i < 0 || i >= p.total || p.state[i/p.size] != pageHave {
		return nil, 0, i
	}
	pg := i / p.size
	first := pg
	for first > 0 && p.state[first-1] == pageHave {
		first--
	}
	last := pg
	for last+1 < p.pages() && p.state[last+1] == pageHave {
		last++
	}
	start, end := first*p.size, min((last+1)*p.size, p.total)
	return p.items[start:end], i - start, end
}

// whole calls then once every page is in, asking for those not, all at
// once: for what needs the whole list, as shuffling it. then gets false if
// a page failed. One waits at a time.
func (p *pager[T]) whole(then func(ok bool)) {
	p.all = then
	p.progress()
}

// progress goes on towards the whole list for whoever waits for it.
func (p *pager[T]) progress() {
	if p.all == nil {
		return
	}
	switch {
	case p.loaded():
		f := p.all
		p.all = nil
		f(true)
	case p.failed():
		f := p.all
		p.all = nil
		f(false)
	default:
		for pg := range p.pages() {
			p.load(pg)
		}
	}
}
