package qqmusic

import (
	"context"
	"sync"
)

// requestGroup shares an in-flight read until all of its callers have left.
// Completed reads are removed; callers decide whether their results are cacheable.
type requestGroup[T any] struct {
	mu    sync.Mutex
	calls map[string]*groupRequest[T]
}

type groupRequest[T any] struct {
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	waiters   int
	completed bool
	value     T
	err       error
}

func (g *requestGroup[T]) do(ctx context.Context, key string, load func(context.Context) (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	g.mu.Lock()
	if g.calls == nil {
		g.calls = make(map[string]*groupRequest[T])
	}
	call := g.calls[key]
	if call == nil {
		shared, cancel := context.WithCancel(context.Background())
		call = &groupRequest[T]{ctx: shared, cancel: cancel, done: make(chan struct{}), waiters: 1}
		g.calls[key] = call
		go g.run(key, call, load)
	} else {
		call.waiters++
	}
	g.mu.Unlock()

	select {
	case <-call.done:
		return call.value, call.err
	case <-ctx.Done():
		g.mu.Lock()
		if !call.completed {
			call.waiters--
			if call.waiters == 0 {
				if g.calls[key] == call {
					delete(g.calls, key)
				}
				call.cancel()
			}
		}
		g.mu.Unlock()
		return zero, ctx.Err()
	}
}

func (g *requestGroup[T]) run(key string, call *groupRequest[T], load func(context.Context) (T, error)) {
	value, err := load(call.ctx)
	g.mu.Lock()
	call.value, call.err, call.completed = value, err, true
	if g.calls[key] == call {
		delete(g.calls, key)
	}
	close(call.done)
	g.mu.Unlock()
	call.cancel()
}
