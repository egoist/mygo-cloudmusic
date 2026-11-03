package qqmusic

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

type groupTestResult struct {
	value int
	err   error
}

func waitGroupWaiters[T any](t *testing.T, group *requestGroup[T], key string, want int) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		group.mu.Lock()
		call := group.calls[key]
		joined := call != nil && call.waiters == want
		group.mu.Unlock()
		if joined {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("request did not acquire %d waiting callers", want)
		default:
			runtime.Gosched()
		}
	}
}

func receiveGroupResult(t *testing.T, results <-chan groupTestResult) groupTestResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("request did not complete")
		return groupTestResult{}
	}
}

// Concurrent identical reads share one load, and a completed result is not retained by the group.
func TestRequestGroupSharesInFlight(t *testing.T) {
	var group requestGroup[int]
	var loads atomic.Int32
	started, finish := make(chan struct{}), make(chan struct{})
	load := func(context.Context) (int, error) {
		loads.Add(1)
		close(started)
		<-finish
		return 42, nil
	}
	results := make(chan groupTestResult, 2)
	go func() { v, err := group.do(context.Background(), "query", load); results <- groupTestResult{v, err} }()
	<-started
	go func() { v, err := group.do(context.Background(), "query", load); results <- groupTestResult{v, err} }()
	waitGroupWaiters(t, &group, "query", 2)
	close(finish)
	for range 2 {
		if got := receiveGroupResult(t, results); got.value != 42 || got.err != nil {
			t.Fatalf("shared result: %+v", got)
		}
	}
	value, err := group.do(context.Background(), "query", func(context.Context) (int, error) { loads.Add(1); return 43, nil })
	if value != 43 || err != nil || loads.Load() != 2 {
		t.Fatalf("next read: value=%d err=%v loads=%d", value, err, loads.Load())
	}
}

// Canceling one caller must not abort a shared read that another caller still needs.
func TestRequestGroupCancellationKeepsOtherWaiter(t *testing.T) {
	var group requestGroup[int]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan context.Context, 1)
	finish := make(chan struct{})
	load := func(shared context.Context) (int, error) { started <- shared; <-finish; return 42, nil }
	first, second := make(chan groupTestResult, 1), make(chan groupTestResult, 1)
	go func() { v, err := group.do(ctx, "query", load); first <- groupTestResult{v, err} }()
	shared := <-started
	go func() { v, err := group.do(context.Background(), "query", load); second <- groupTestResult{v, err} }()
	waitGroupWaiters(t, &group, "query", 2)
	cancel()
	if got := receiveGroupResult(t, first); !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled caller: %+v", got)
	}
	select {
	case <-shared.Done():
		t.Fatal("one caller canceled the other caller's read")
	default:
	}
	close(finish)
	if got := receiveGroupResult(t, second); got.value != 42 || got.err != nil {
		t.Fatalf("remaining caller: %+v", got)
	}
}

// The last caller cancels its load, and that old load cannot remove a newer read for the same query.
func TestRequestGroupLastCancelAllowsFreshRead(t *testing.T) {
	var group requestGroup[int]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oldStarted, oldFinish := make(chan context.Context, 1), make(chan struct{})
	oldResult := make(chan groupTestResult, 1)
	go func() {
		v, err := group.do(ctx, "query", func(shared context.Context) (int, error) {
			oldStarted <- shared
			<-shared.Done()
			<-oldFinish
			return 0, shared.Err()
		})
		oldResult <- groupTestResult{v, err}
	}()
	oldContext := <-oldStarted
	group.mu.Lock()
	oldCall := group.calls["query"]
	group.mu.Unlock()
	cancel()
	if got := receiveGroupResult(t, oldResult); !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled last caller: %+v", got)
	}
	select {
	case <-oldContext.Done():
	default:
		t.Fatal("last caller left its load running")
	}
	started, finish := make(chan struct{}), make(chan struct{})
	var freshLoads atomic.Int32
	load := func(context.Context) (int, error) { freshLoads.Add(1); close(started); <-finish; return 99, nil }
	fresh := make(chan groupTestResult, 2)
	go func() { v, err := group.do(context.Background(), "query", load); fresh <- groupTestResult{v, err} }()
	<-started
	close(oldFinish)
	<-oldCall.done
	go func() { v, err := group.do(context.Background(), "query", load); fresh <- groupTestResult{v, err} }()
	waitGroupWaiters(t, &group, "query", 2)
	close(finish)
	for range 2 {
		if got := receiveGroupResult(t, fresh); got.value != 99 || got.err != nil {
			t.Fatalf("fresh result: %+v", got)
		}
	}
	if freshLoads.Load() != 1 {
		t.Fatalf("fresh read loaded %d times", freshLoads.Load())
	}
}

// Already canceled callers do not start work, and failed reads remain retryable.
func TestRequestGroupDoesNotKeepErrors(t *testing.T) {
	var group requestGroup[int]
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if _, err := group.do(ctx, "query", func(context.Context) (int, error) { called = true; return 0, nil }); !errors.Is(err, context.Canceled) || called {
		t.Fatalf("already canceled read: called=%t err=%v", called, err)
	}
	want := errors.New("temporary")
	if _, err := group.do(context.Background(), "query", func(context.Context) (int, error) { return 0, want }); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if value, err := group.do(context.Background(), "query", func(context.Context) (int, error) { return 7, nil }); value != 7 || err != nil {
		t.Fatalf("retry: value=%d err=%v", value, err)
	}
}
