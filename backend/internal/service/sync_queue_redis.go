package service

import (
	"context"

	"moon-eye/backend/internal/queue"
)

// RedisSyncQueue implements SyncQueue by publishing to a Redis Stream via queue.Producer.
type RedisSyncQueue struct {
	producer queue.Producer
}

// NewRedisSyncQueue returns a SyncQueue that enqueues to the given Producer.
func NewRedisSyncQueue(producer queue.Producer) SyncQueue {
	return &RedisSyncQueue{producer: producer}
}

// EnqueueSyncJob sends the job to the Redis stream. Payload should be JSON that
// sync-service understands (e.g. SyncJobPayload: userId, connectionId optional, sinceVersion).
func (q *RedisSyncQueue) EnqueueSyncJob(ctx context.Context, job SyncJob) error {
	if q.producer == nil {
		return nil
	}
	return q.producer.Enqueue(ctx, queue.Message{
		Entity:    job.Entity,
		Operation: job.Operation,
		Payload:   job.Payload,
	})
}
