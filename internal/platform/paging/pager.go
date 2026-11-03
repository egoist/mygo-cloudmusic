package paging

import (
	"context"
	"errors"
	"sync"
)

// Provider-neutral paging module.
//
// The pager owns asynchronous page state without knowing a music service.
// Provider adapters supply fetch functions and translate their own endpoints
// at that seam.

type pageState byte

const (
	pageNone pageState = iota
	pageLoading
	pageHave
	pageFailed
)

// Pager keeps a long list sparse and loads the requested page plus a small
// read-ahead window. fetch receives one-based page numbers; all other methods
// use zero-based item/page indexes.
type Pager[T any] struct {
	size, ahead int
	fetch       func(page int) ([]T, int, error)
	post        func(func())
	total       int
	items       []T
	state       []pageState
	err         error
	gen         int
	landed      func(from int, items []T)
	first       func(bool)
	all         func(bool)
	inflight    int
	limit       int
	waitMu      sync.Mutex
	waiters     map[int][]func(bool)
}

// New constructs a pager. post is called by worker completions on the owner
// goroutine; tests may pass a synchronous function.
func New[T any](size, ahead int, post func(func()), fetch func(page int) ([]T, int, error)) *Pager[T] {
	if size <= 0 {
		size = 1
	}
	if ahead < 0 {
		ahead = 0
	}
	if post == nil {
		post = func(fn func()) { fn() }
	}
	return &Pager[T]{size: size, ahead: ahead, post: post, fetch: fetch, total: -1, state: make([]pageState, 1), limit: 6}
}

func (p *Pager[T]) pages() int {
	if p.total < 0 {
		return 1
	}
	if p.total == 0 {
		return 0
	}
	return (p.total + p.size - 1) / p.size
}

// Total returns the known list length, or -1 until its first page lands.
func (p *Pager[T]) Total() int { return p.total }

// Error returns the most recent page error, if one is available.
func (p *Pager[T]) Error() error {
	if p == nil {
		return nil
	}
	return p.err
}

// Len returns the known list length, or zero while it is unknown.
func (p *Pager[T]) Len() int { return max(p.total, 0) }

// At returns item i when its page is loaded, otherwise nil and starts loading.
func (p *Pager[T]) At(i int) *T {
	if i < 0 || p.total >= 0 && i >= p.total {
		return nil
	}
	p.want(i / p.size)
	if p.total < 0 || p.state[i/p.size] != pageHave {
		return nil
	}
	return &p.items[i]
}

func (p *Pager[T]) want(pg int) {
	for k := 0; k <= p.ahead; k++ {
		p.load(pg + k)
	}
}

func (p *Pager[T]) load(pg int) {
	if pg < 0 || pg >= p.pages() || p.state[pg] != pageNone || p.inflight >= p.limit || p.fetch == nil {
		return
	}
	p.state[pg], p.inflight = pageLoading, p.inflight+1
	gen := p.gen
	go func() {
		items, total, err := p.fetch(pg + 1)
		p.post(func() {
			p.inflight--
			if gen != p.gen {
				// A reload may have happened while this request was in flight.
				// Re-issue the corresponding current page once its old slot is
				// released so the first visible rows do not remain skeletons.
				p.want(pg)
				p.progress()
				return
			}
			if err != nil {
				// A platform switch or reconnect invalidates the request. It is
				// not a failed page and must not turn the loading skeleton into
				// a misleading retry button.
				if errors.Is(err, context.Canceled) {
					p.state[pg] = pageNone
					p.wake(pg, false)
					return
				}
				p.fail(pg, err)
				return
			}
			p.Land(pg, items, total)
		})
	}()
}

// Land applies a page result. It is exported for deterministic adapters and
// tests that already have a response.
func (p *Pager[T]) Land(pg int, items []T, total int) {
	if pg < 0 || total < 0 {
		return
	}
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
	p.state[pg], p.err = pageHave, nil
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

// When calls fn once page pg is available, loading it if necessary.
func (p *Pager[T]) When(pg int, fn func(bool)) {
	if pg >= 0 && pg < len(p.state) && p.state[pg] == pageHave {
		fn(true)
		return
	}
	if p.total < 0 && pg > 0 {
		p.When(0, func(ok bool) {
			if !ok {
				fn(false)
				return
			}
			p.When(pg, fn)
		})
		return
	}
	p.waitMu.Lock()
	if p.waiters == nil {
		p.waiters = map[int][]func(bool){}
	}
	p.waiters[pg] = append(p.waiters[pg], fn)
	p.waitMu.Unlock()
	if pg < len(p.state) && p.state[pg] == pageFailed {
		p.state[pg] = pageNone
	}
	p.load(pg)
}

func (p *Pager[T]) wake(pg int, ok bool) {
	p.waitMu.Lock()
	fns := append([]func(bool){}, p.waiters[pg]...)
	delete(p.waiters, pg)
	p.waitMu.Unlock()
	for _, fn := range fns {
		fn(ok)
	}
}

func (p *Pager[T]) fail(pg int, err error) {
	p.err = err
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

// FailedAt reports whether the page containing item i failed.
func (p *Pager[T]) FailedAt(i int) bool {
	return i >= 0 && i/p.size < len(p.state) && p.state[i/p.size] == pageFailed
}

func (p *Pager[T]) failed() bool {
	for _, s := range p.state {
		if s == pageFailed {
			return true
		}
	}
	return false
}

// Retry makes failed pages eligible for loading again.
func (p *Pager[T]) Retry() {
	for i, s := range p.state {
		if s == pageFailed {
			p.state[i] = pageNone
		}
	}
	p.err = nil
}

// Reload invalidates every page while keeping the known total and list
// geometry. This lets a sort or filter change show stable rows and skeletons
// instead of collapsing the whole page back to an unknown loading state.
func (p *Pager[T]) Reload() {
	if p == nil {
		return
	}
	p.gen++
	p.err = nil
	for i := range p.state {
		p.state[i] = pageNone
	}
	p.waitMu.Lock()
	p.waiters = nil
	p.waitMu.Unlock()
}

// IndexOf finds the first loaded item matching fn without starting requests.
func (p *Pager[T]) IndexOf(fn func(*T) bool) int {
	for pg, s := range p.state {
		if s != pageHave {
			continue
		}
		for i := pg * p.size; i < min((pg+1)*p.size, p.total); i++ {
			if fn(&p.items[i]) {
				return i
			}
		}
	}
	return -1
}

// RemoveAt removes item i and invalidates pages that shifted after it.
func (p *Pager[T]) RemoveAt(i int) {
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
	p.waitMu.Lock()
	p.waiters = nil
	p.waitMu.Unlock()
}

// Around returns the contiguous loaded run containing i, the item's offset,
// and the end index of the run.
func (p *Pager[T]) Around(i int) (items []T, at, end int) {
	if p.total < 0 || i < 0 || i >= p.total || p.state[i/p.size] != pageHave {
		return nil, 0, i
	}
	pg := i / p.size
	first, last := pg, pg
	for first > 0 && p.state[first-1] == pageHave {
		first--
	}
	for last+1 < p.pages() && p.state[last+1] == pageHave {
		last++
	}
	start, end := first*p.size, min((last+1)*p.size, p.total)
	return p.items[start:end], i - start, end
}

// Whole invokes fn once all pages are loaded, or false if any page fails.
func (p *Pager[T]) Whole(fn func(bool)) {
	p.all = fn
	p.progress()
}

func (p *Pager[T]) loaded() bool {
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

func (p *Pager[T]) progress() {
	if p.all == nil {
		return
	}
	switch {
	case p.loaded():
		fn := p.all
		p.all = nil
		fn(true)
	case p.failed():
		fn := p.all
		p.all = nil
		fn(false)
	default:
		for pg := 0; pg < p.pages(); pg++ {
			p.load(pg)
		}
	}
}

// SetLanded observes pages as they land on the owner goroutine.
func (p *Pager[T]) SetLanded(fn func(from int, items []T)) { p.landed = fn }

// SetFirst observes the first page landing or failing.
func (p *Pager[T]) SetFirst(fn func(bool)) { p.first = fn }

// PageSize returns the number of items requested per physical page.
func (p *Pager[T]) PageSize() int {
	if p == nil || p.size <= 0 {
		return 0
	}
	return p.size
}

// PageCount returns the number of physical pages once the total is known.
func (p *Pager[T]) PageCount() int {
	if p == nil || p.total < 0 {
		return 0
	}
	return p.pages()
}

// Peek returns a loaded item without starting a network request.
func (p *Pager[T]) Peek(i int) *T {
	if p == nil || i < 0 || p.total < 0 || i >= p.total || i/p.size >= len(p.state) || p.state[i/p.size] != pageHave {
		return nil
	}
	return &p.items[i]
}
