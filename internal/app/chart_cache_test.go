package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestChartCacheCoalescesExpiresAndSeparatesScopes(t *testing.T) {
	var cache pointCache
	var calls atomic.Int32
	start := make(chan struct{})
	release := make(chan struct{})
	load := func() ([]Point, error) {
		if calls.Add(1) == 1 {
			close(start)
		}
		<-release
		v := 1.0
		return []Point{{At: 1, Value: &v}}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := cache.get(context.Background(), "24:false:", load)
			if e != nil || len(v) != 1 {
				t.Error(v, e)
			}
		}()
	}
	<-start
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("duplicate loads: %d", calls.Load())
	}
	cache.mu.Lock()
	cache.entries["24:false:"].expires = time.Now().Add(-time.Second)
	cache.mu.Unlock()
	if _, e := cache.get(context.Background(), "24:false:", load); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 40; i++ {
		if _, e := cache.get(context.Background(), fmt.Sprint(i), load); e != nil {
			t.Fatal(e)
		}
	}
	if calls.Load() != 42 || len(cache.entries) > 32 {
		t.Fatal("scope, expiry or size bound failed")
	}
}
func TestChartCacheCancellationDoesNotPoisonOtherReaders(t *testing.T) {
	var cache pointCache
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		cache.get(ctx, "key", func() ([]Point, error) { close(started); <-ctx.Done(); return nil, ctx.Err() })
	}()
	<-started
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, e := cache.get(cancelled, "key", func() ([]Point, error) { t.Error("cancelled waiter loaded"); return nil, nil }); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	cancel()
	<-done
	want := []Point{{At: 2}}
	got, e := cache.get(context.Background(), "key", func() ([]Point, error) { return want, nil })
	if e != nil || len(got) != 1 || got[0].At != 2 {
		t.Fatal(got, e)
	}
}
