package player

// track remembers a file-backed download until its last reader/writer is gone.
// The caller holds c.mu; downloads are not exposed to readers before this call.
func (c *Cache) track(d *download) {
	if c.active == nil {
		c.active = make(map[*download]struct{})
	}
	c.active[d] = struct{}{}
	d.onRelease = func() {
		c.mu.Lock()
		delete(c.active, d)
		c.mu.Unlock()
	}
}

// Quiesce blocks new disk operations and closes all cached readers and writers.
// Call it off the main thread, after suspending playback. Call the returned
// function only when reusing this cache after failure; otherwise leave it
// blocked so stale loads cannot recreate files in the old directory.
func (c *Cache) Quiesce() func() {
	if c == nil {
		return func() {}
	}
	c.mu.Lock()
	c.blocked = true
	active := make([]*download, 0, len(c.active))
	for d := range c.active {
		active = append(active, d)
	}
	c.mu.Unlock()
	for _, d := range active {
		d.halt()
	}
	return func() {
		c.mu.Lock()
		c.blocked = false
		c.mu.Unlock()
	}
}

// SuspendCache releases the current decoder and defers further loads until the
// returned main-thread function runs. Queue, seek and play/pause changes made
// during migration take effect when playback resumes.
func (p *Player) SuspendCache() func() {
	position := p.Position()
	p.cacheChanging = true
	p.gen++
	if p.stream != nil {
		dl, _ := p.stream.source()
		dl.Close()
	}
	if p.out != nil {
		p.out.Close()
	}
	p.out, p.stream, p.info = nil, nil, nil
	p.fade.attach(nil, 1)
	p.seeking = false
	p.pending = position
	p.loading = p.track != nil
	p.sync()
	return func() {
		p.cacheChanging = false
		if !p.awaiting && p.track != nil && p.at >= 0 && p.at < len(p.order) {
			p.loadSkipping(p.at, 0, p.playing, p.pending)
		} else {
			p.loading = false
			p.sync()
		}
	}
}
