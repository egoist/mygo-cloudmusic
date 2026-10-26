package app

import (
	"encoding/json"
	"math/rand/v2"
	"sync"

	"netease-music-mygo/internal/netease"
)

// A queue of the Feiniu platform is read from a list of songs as it is
// played, in the order of the list or at random: never the whole of it.
//
// At random, the order is a shuffled list of the positions in the list,
// from a seed, and what is asked of the service is the song at each
// position, a few at a time. Nothing is shuffled that is not there, and
// every song comes once. The seed and how far the queue has got are kept in
// the session, so that after a restart the same order goes on.

// fnBatch is how many songs a queue asks for at a time, at random: one
// request for each, a few at once.
const fnBatch = 10

type fnQueue struct {
	list    *fnList
	shuffle bool
	seed    int64
	perm    []int // the positions at random, made once the list's length is known
	next    int   // in order, the position asked for next; at random, how far into perm
	// pos is the position in the list of each song handed to the player,
	// to read on in order from the song playing when shuffle is switched off.
	pos map[int64]int
}

func newFnQueue(l *fnList, shuffle bool) *fnQueue {
	q := &fnQueue{list: l, shuffle: shuffle, pos: map[int64]int{}}
	if shuffle {
		q.seed = rand.Int64()
	}
	return q
}

// fnOrigin is what the session keeps of a queue: which list, in which order,
// and how far. It is what origin returns.
type fnOrigin struct {
	T       string `json:"t"`
	Ref     string `json:"ref,omitempty"`
	Q       string `json:"q,omitempty"`
	Sort    string `json:"sort,omitempty"`
	Shuffle bool   `json:"shuffle,omitempty"`
	Seed    int64  `json:"seed,omitempty"`
	Next    int    `json:"next"`
}

func (q *fnQueue) origin() string {
	s := q.list.spec
	b, _ := json.Marshal(fnOrigin{T: s.typ, Ref: s.ref, Q: s.q, Sort: s.sort, Shuffle: q.shuffle, Seed: q.seed, Next: q.next})
	return string(b)
}

// fnFeed returns what gives the queue its songs, in order or at random.
func (a *app) fnFeed(q *fnQueue) func(got func([]netease.Track)) {
	if q.shuffle {
		return a.fnRandomFeed(q)
	}
	return a.fnOrderFeed(q)
}

// fnOrderFeed gives the songs of the list from position q.next on, a page at
// a time, as the queue asks: the page may be in already, read ahead.
func (a *app) fnOrderFeed(q *fnQueue) func(got func([]netease.Track)) {
	return func(got func([]netease.Track)) {
		p := q.list.pg
		if p.total >= 0 && q.next >= p.total {
			got(nil)
			return
		}
		page := q.next / p.size
		p.when(page, func(ok bool) {
			if !ok {
				a.toast = "加载更多歌曲失败"
				got(nil)
				return
			}
			end := min((page+1)*p.size, p.total)
			if q.next >= end {
				got(nil)
				return
			}
			items := append([]netease.Track(nil), p.items[q.next:end]...)
			for i, t := range items {
				q.pos[t.ID] = q.next + i
			}
			q.next = end
			got(items)
		})
	}
}

// randomPageSize is how large a page to ask for a song at a position of a
// list of total songs by. The service answers a page past its 2000th with
// its first, without a word: asking for position 3000 as page 3001 of one
// song each gets the first song, as the nearly every of a long list would.
// So the pages are as large as it takes to keep their numbers under a
// thousand, and the song is the one at its place in the page.
func randomPageSize(total int) int { return total/1000 + 1 }

// fnRandomFeed gives the songs at the next positions of the random order,
// fnBatch of them, each asked for alone.
func (a *app) fnRandomFeed(q *fnQueue) func(got func([]netease.Track)) {
	return func(got func([]netease.Track)) {
		p := q.list.pg
		p.when(0, func(ok bool) {
			if !ok {
				a.toast = "加载更多歌曲失败"
				got(nil)
				return
			}
			if len(q.perm) != p.total {
				q.perm = rand.New(rand.NewPCG(uint64(q.seed), 0x9e3779b97f4a7c15)).Perm(p.total)
			}
			if q.next >= len(q.perm) {
				got(nil)
				return
			}
			end := min(q.next+fnBatch, len(q.perm))
			at := append([]int(nil), q.perm[q.next:end]...)
			fetch, size := q.list.fetch, randomPageSize(p.total)
			go func() {
				songs := make([]*netease.Track, len(at))
				var wg sync.WaitGroup
				sem := make(chan struct{}, 6)
				for k, i := range at {
					wg.Add(1)
					sem <- struct{}{}
					go func() {
						defer func() { <-sem; wg.Done() }()
						if tr, _, err := fetch(i/size+1, size); err == nil && i%size < len(tr) {
							songs[k] = &tr[i%size]
						}
					}()
				}
				wg.Wait()
				a.fnPost(func() {
					var out []netease.Track
					for k, s := range songs {
						if s != nil {
							out = append(out, *s)
							q.pos[s.ID] = at[k]
						}
					}
					q.next = end
					a.noteHearts(out)
					if len(out) == 0 {
						a.toast = "加载更多歌曲失败"
					}
					got(out)
				})
			}()
		})
	}
}

// startQueue plays songs, which are the first of the queue q reads from.
func (a *app) startQueue(q *fnQueue, songs []netease.Track, at int) {
	a.fn.q = q
	a.pl.PlayList(songs, at, a.fnFeed(q), q.origin)
	a.pl.SetShuffle(q.shuffle)
}

// playFn plays the list from song i: the pages in around it and then the
// pages after, as the queue nears the end of what is in; or, with shuffle
// on, from that song at random.
func (a *app) playFn(l *fnList, i int) {
	if a.pl.Shuffle() {
		tr := l.pg.at(i)
		if tr == nil {
			return
		}
		q := newFnQueue(l, true)
		q.pos[tr.ID] = i
		a.startQueue(q, []netease.Track{*tr}, 0)
		return
	}
	items, at, end := l.pg.around(i)
	if items == nil {
		return
	}
	q := newFnQueue(l, false)
	q.next = end
	for k, t := range items {
		q.pos[t.ID] = end - len(items) + k
	}
	a.startQueue(q, items, at)
}

// shuffleFn plays the list in a random order. It asks for the length of the
// list and a few songs, not for the whole of it.
func (a *app) shuffleFn(l *fnList) {
	if l.shuffle {
		return
	}
	l.shuffle = true
	q := newFnQueue(l, true)
	a.fnFeed(q)(func(first []netease.Track) {
		l.shuffle = false
		if len(first) > 0 {
			a.startQueue(q, first, 0)
		}
		a.redraw()
	})
}

// fnShuffleToggled is what the player asks when shuffle is switched on or
// off over a queue read from a list: the queue is made over from the song
// playing, at random or in order from it, and the player has nothing of its
// own to shuffle. It returns false for a queue that is not of the service's.
func (a *app) fnShuffleToggled(on bool) bool {
	old, cur := a.fn.q, a.pl.Current()
	if old == nil || cur == nil || a.fn.client == nil {
		return false
	}
	q := newFnQueue(old.list, on)
	q.pos = old.pos
	if !on {
		// In order from the song playing, if its place in the list is
		// known; if not, from the start of it.
		if i, ok := old.pos[cur.ID]; ok {
			q.next = i + 1
		}
	}
	a.fn.q = q
	a.pl.ListFrom(a.fnFeed(q), q.origin)
	return true
}

// restoreFnOrigin makes the queue of a restored session go on reading the
// list it was read from, where it had got to: the songs it held are
// what was loaded, and the rest of the list is read as it plays on.
func (a *app) restoreFnOrigin(raw string) {
	var o fnOrigin
	if json.Unmarshal([]byte(raw), &o) != nil || o.T == "" || a.fn.client == nil || a.pl.Current() == nil {
		return
	}
	l := a.newFnList(fnSpec{typ: o.T, ref: o.Ref, q: o.Q, sort: o.Sort})
	if l == nil {
		return
	}
	q := newFnQueue(l, o.Shuffle)
	q.seed, q.next = o.Seed, o.Next
	a.fn.q = q
	a.pl.AttachList(a.fnFeed(q), q.origin)
}

// Roaming is the service's own random play, which picks every song and is
// asked for the next with the roam ID of the one playing. It is a radio,
// as NetEase's private FM is.
const fnRoamRadio = "fn:roam"

// startRoam starts the roam, and plays its first song.
func (a *app) startRoam() {
	c := a.fn.client
	if c == nil {
		a.toast = "还没有连接飞牛音乐"
		return
	}
	if a.radioStarting == fnRoamRadio {
		return
	}
	a.radioStarting = fnRoamRadio
	go func() {
		tr, id, err := c.RoamStart()
		a.fnPost(func() {
			if a.radioStarting != fnRoamRadio {
				return
			}
			a.radioStarting = ""
			if err != nil {
				a.toast = "漫游暂时无法开启，请稍后再试"
				return
			}
			a.fn.roamID = id
			a.pl.PlayRadio(fnRoamRadio, []netease.Track{tr}, a.radioFeed(fnRoamRadio))
		})
	}()
}

// roamFeed gives the roam's next song, which the service picks after the one
// whose roam ID it is told. Restored from a session, it has no roam ID, and
// the service starts the roam over.
func (a *app) roamFeed(got func([]netease.Track)) {
	c, after := a.fn.client, a.fn.roamID
	if c == nil {
		got(nil)
		return
	}
	// What is queued: the roam may pick a song again, which the queue does
	// not take twice and would stop for, so the chain is asked on from it.
	queued := map[string]bool{}
	for _, t := range a.pl.Queue() {
		queued[t.Ref] = true
	}
	go func() {
		var tr netease.Track
		var id string
		var err error
		for range roamTries {
			if after == "" {
				tr, id, err = c.RoamStart()
			} else if tr, id, err = c.RoamNext(after); err != nil {
				// A chain the service no longer has (it ends in time): a new
				// one is started.
				tr, id, err = c.RoamStart()
			}
			if err != nil || !queued[tr.Ref] {
				break
			}
			after = id
		}
		a.fnPost(func() {
			if err != nil {
				got(nil)
				return
			}
			a.fn.roamID = id
			got([]netease.Track{tr})
		})
	}()
}

// roamTries is how many songs of a roam are asked for to find one that is not
// queued already, before the queue is given the last.
const roamTries = 6
