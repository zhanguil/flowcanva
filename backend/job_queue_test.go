package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestJobQueueHonorsConcurrencyAndCancelsQueuedJob(t *testing.T) {
	queue := NewJobQueue(2)
	release := make(chan struct{})
	var running, peak int32
	var mu sync.Mutex
	results := []<-chan error{}
	for _, id := range []string{"a", "b", "c", "d"} {
		result, err := queue.Enqueue(context.Background(), QueueJob{ID: id, Execute: func(context.Context) error {
			current := atomic.AddInt32(&running, 1)
			mu.Lock()
			if current > peak {
				peak = current
			}
			mu.Unlock()
			<-release
			atomic.AddInt32(&running, -1)
			return nil
		}})
		if err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		results = append(results, result)
	}
	deadline := time.Now().Add(time.Second)
	for atomic.LoadInt32(&running) != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if peak != 2 {
		t.Fatalf("peak concurrency=%d want 2", peak)
	}
	if !queue.Cancel("d") {
		t.Fatal("queued job was not cancelled")
	}
	close(release)
	for index, result := range results {
		err := <-result
		if index == 3 {
			if err == nil {
				t.Fatal("cancelled queued job executed")
			}
		} else if err != nil {
			t.Fatalf("job %d failed: %v", index, err)
		}
	}
	if running, total := queue.Stats(); running != 0 || total != 0 {
		t.Fatalf("queue not drained: %d/%d", running, total)
	}
}
