package main

import (
	"context"
	"errors"
	"sync"
)

var errJobAlreadyQueued = errors.New("job already queued")

type QueueJob struct {
	ID      string
	Execute func(context.Context) error
}

type JobQueue struct {
	jobs    chan queuedJob
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	running int
}

type queuedJob struct {
	ctx    context.Context
	job    QueueJob
	result chan error
}

func NewJobQueue(maxConcurrent int) *JobQueue {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	q := &JobQueue{jobs: make(chan queuedJob, 1024), cancels: map[string]context.CancelFunc{}}
	for range maxConcurrent {
		go q.worker()
	}
	return q
}

func (q *JobQueue) Enqueue(parent context.Context, job QueueJob) (<-chan error, error) {
	if job.ID == "" || job.Execute == nil {
		return nil, errors.New("invalid queue job")
	}
	ctx, cancel := context.WithCancel(parent)
	q.mu.Lock()
	if _, exists := q.cancels[job.ID]; exists {
		q.mu.Unlock()
		cancel()
		return nil, errJobAlreadyQueued
	}
	q.cancels[job.ID] = cancel
	q.mu.Unlock()
	result := make(chan error, 1)
	q.jobs <- queuedJob{ctx: ctx, job: job, result: result}
	return result, nil
}

func (q *JobQueue) worker() {
	for item := range q.jobs {
		err := item.ctx.Err()
		if err == nil {
			q.mu.Lock()
			q.running++
			q.mu.Unlock()
			err = item.job.Execute(item.ctx)
			q.mu.Lock()
			q.running--
			q.mu.Unlock()
		}
		item.result <- err
		q.mu.Lock()
		delete(q.cancels, item.job.ID)
		q.mu.Unlock()
		close(item.result)
	}
}

func (q *JobQueue) Cancel(jobID string) bool {
	q.mu.Lock()
	cancel, exists := q.cancels[jobID]
	q.mu.Unlock()
	if exists {
		cancel()
	}
	return exists
}

func (q *JobQueue) Stats() (running, total int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.running, len(q.cancels)
}
