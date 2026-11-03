package paging

import (
	"errors"
	"testing"
)

// A landed page exposes its items, reports its total, and returns the loaded run around an item.
func TestPagerLandAndAround(t *testing.T) {
	p := New[int](2, 0, nil, nil)
	p.Land(0, []int{1, 2}, 4)
	p.Land(1, []int{3, 4}, 4)
	if p.Total() != 4 || p.Len() != 4 || p.At(2) == nil || *p.At(2) != 3 {
		t.Fatalf("pager contents: total=%d len=%d item=%v", p.Total(), p.Len(), p.At(2))
	}
	items, at, end := p.Around(2)
	if len(items) != 4 || at != 2 || end != 4 {
		t.Fatalf("around = %v, %d, %d", items, at, end)
	}
}

// A failed page notifies waiters and Retry allows the same page to be loaded again.
func TestPagerRetry(t *testing.T) {
	tries := 0
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	p := New(2, 0, func(fn func()) { fn() }, func(page int) ([]int, int, error) {
		tries++
		started <- struct{}{}
		<-release
		if tries == 1 {
			return nil, 0, errors.New("temporary")
		}
		return []int{7}, 1, nil
	})
	var ok []bool
	answered := make(chan bool, 2)
	p.When(0, func(got bool) { ok = append(ok, got); answered <- got })
	<-started
	close(release)
	if got := <-answered; got {
		t.Fatal("first page unexpectedly succeeded")
	}
	if len(ok) != 1 || ok[0] || !p.FailedAt(0) {
		t.Fatalf("failed page: ok=%v failed=%v", ok, p.FailedAt(0))
	}
	p.Retry()
	p.When(0, func(got bool) { ok = append(ok, got); answered <- got })
	if got := <-answered; !got {
		t.Fatal("retry did not succeed")
	}
	if len(ok) != 2 || !ok[1] || p.At(0) == nil || *p.At(0) != 7 {
		t.Fatalf("retried page: ok=%v item=%v", ok, p.At(0))
	}
}

// Removing an item shortens the list and invalidates pages that shifted after it.
func TestPagerRemoveAt(t *testing.T) {
	p := New[int](2, 0, nil, nil)
	p.Land(0, []int{1, 2}, 4)
	p.Land(1, []int{3, 4}, 4)
	p.RemoveAt(1)
	if p.Len() != 3 || p.IndexOf(func(v *int) bool { return *v == 2 }) != -1 {
		t.Fatalf("remove left stale data: len=%d index=%d", p.Len(), p.IndexOf(func(v *int) bool { return *v == 2 }))
	}
}

// PageSize and Peek expose layout information without starting a request, as virtual views need.
func TestPagerPageLayoutAndPeek(t *testing.T) {
	p := New[int](3, 0, nil, func(int) ([]int, int, error) { t.Fatal("Peek started a fetch"); return nil, 0, nil })
	p.Land(0, []int{1, 2, 3}, 4)
	if p.PageSize() != 3 || p.PageCount() != 2 {
		t.Fatalf("page layout = size %d count %d", p.PageSize(), p.PageCount())
	}
	if got := p.Peek(1); got == nil || *got != 2 {
		t.Fatalf("Peek(1) = %v", got)
	}
	if p.Peek(3) != nil {
		t.Fatal("Peek returned an item from an unloaded page")
	}
}

// Reload keeps the known page geometry while invalidating loaded rows for a new sort order.
func TestPagerReloadKeepsLayout(t *testing.T) {
	p := New[int](2, 0, nil, nil)
	p.Land(0, []int{1, 2}, 5)
	p.Land(1, []int{3, 4}, 5)
	p.Reload()
	if p.Total() != 5 || p.PageCount() != 3 || p.Len() != 5 {
		t.Fatalf("reload changed layout: total=%d pages=%d len=%d", p.Total(), p.PageCount(), p.Len())
	}
	if p.Peek(0) != nil || p.Peek(3) != nil {
		t.Fatal("reload retained stale page items")
	}
}
