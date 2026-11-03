package qqmusic

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// songShelfPager keeps full pages at the end by overlapping the preceding page.
type songShelfPager struct {
	page, size, total int
	previous, next    ui.Handle
	wheelAt           time.Time
	wheelX            float32
	wheelUsed         bool
}

func (p *songShelfPager) layout(total, size int) {
	size = max(1, size)
	total = max(0, total)
	if p.size == size && p.total == total {
		return
	}
	anchor := p.start()
	p.size, p.total = size, total
	p.page = min(anchor/size, max(0, p.pages()-1))
	p.wheelX, p.wheelUsed = 0, false
}

func (p *songShelfPager) pages() int {
	if p.size == 0 {
		return 0
	}
	return (p.total + p.size - 1) / p.size
}

func (p *songShelfPager) start() int {
	return min(p.page*p.size, max(0, p.total-p.size))
}

func (p *songShelfPager) move(delta int) bool {
	page := min(max(0, p.page+delta), max(0, p.pages()-1))
	if page == p.page {
		return false
	}
	p.page = page
	return true
}

func (p *songShelfPager) scroll(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputScroll || ev.Mods != 0 || p.pages() < 2 || math.Abs(float64(ev.DX)) <= math.Abs(float64(ev.DY)) {
		return false
	}
	// One touchpad gesture changes one page. Ignore its momentum rather than
	// rapidly skipping several pages after the fingers have left the pad.
	if ev.Momentum {
		return true
	}
	if ev.Time.Sub(p.wheelAt) > 180*time.Millisecond || ev.DX*p.wheelX < 0 {
		p.wheelX, p.wheelUsed = 0, false
	}
	p.wheelAt = ev.Time
	p.wheelX += ev.DX
	if !p.wheelUsed && (math.Abs(float64(p.wheelX)) >= 48 || !ev.Precise) {
		direction := 1
		if p.wheelX < 0 {
			direction = -1
		}
		p.move(direction)
		p.wheelUsed = true
	}
	return true
}
