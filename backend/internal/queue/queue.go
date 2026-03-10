package queue

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Message represents a generic job on a Redis Stream.
// It is the in-memory representation of the on-wire schema:
//
//   entity      -> string
//   operation   -> string
//   payload     -> []byte (typically JSON)
//   retry_count -> int (default 0)
//
// The message ID is the Redis Stream entry ID.
type Message struct {
	ID         string
	Entity     string
	Operation  string
	Payload    []byte
	RetryCount int
}

// Redis field names for the standard queue message schema.
const (
	fieldEntity     = "entity"
	fieldOperation  = "operation"
	fieldPayload    = "payload"
	fieldRetryCount = "retry_count"
)

// Producer defines the behavior to enqueue messages.
type Producer interface {
	Enqueue(ctx context.Context, msg Message) error
}

// Consumer defines the behavior to consume messages.
type Consumer interface {
	Consume(ctx context.Context, handler func(context.Context, Message) error) error
}

// RedisStreamConfig holds configuration for a Redis Stream queue.
type RedisStreamConfig struct {
	Stream       string
	Group        string
	ConsumerName string
	// BlockTimeout controls how long a blocking read waits for messages.
	// If zero, a sane default is used.
	BlockTimeout time.Duration
}

// RedisStreamQueue implements Producer and Consumer using Redis Streams.
// NOTE: This is a minimal implementation and should be hardened for production use.
type RedisStreamQueue struct {
	client *redis.Client
	cfg    RedisStreamConfig
}

func NewRedisStreamQueue(client *redis.Client, cfg RedisStreamConfig) *RedisStreamQueue {
	if cfg.BlockTimeout <= 0 {
		cfg.BlockTimeout = 5 * time.Second
	}
	return &RedisStreamQueue{
		client: client,
		cfg:    cfg,
	}
}

// Enqueue adds a message to the Redis Stream using the standard schema.
func (q *RedisStreamQueue) Enqueue(ctx context.Context, msg Message) error {
	values := map[string]any{
		fieldEntity:    msg.Entity,
		fieldOperation: msg.Operation,
		fieldPayload:   msg.Payload,
	}

	if msg.RetryCount > 0 {
		values[fieldRetryCount] = msg.RetryCount
	}

	_, err := q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: q.cfg.Stream,
		Values: values,
	}).Result()
	return err
}

// Consume starts consuming messages from the Redis Stream using a consumer group.
// The handler is responsible for implementing any higher-level retry semantics.
func (q *RedisStreamQueue) Consume(ctx context.Context, handler func(context.Context, Message) error) error {
	if err := q.ensureGroup(ctx); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		streams, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    q.cfg.Group,
			Consumer: q.cfg.ConsumerName,
			Streams:  []string{q.cfg.Stream, ">"},
			Count:    10,
			Block:    q.cfg.BlockTimeout,
		}).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			return err
		}

		for _, s := range streams {
			for _, m := range s.Messages {
				msg := decodeMessage(m)

				if err := handler(ctx, msg); err != nil {
					// Do not ACK on failure; leave the message pending for retry
					// via Redis consumer group semantics.
					continue
				}

				_, _ = q.client.XAck(ctx, q.cfg.Stream, q.cfg.Group, m.ID).Result()
			}
		}
	}
}

func (q *RedisStreamQueue) ensureGroup(ctx context.Context) error {
	// MKSTREAM ensures the stream exists if it has not been created yet.
	// Ignore BUSYGROUP errors if the group already exists.
	if err := q.client.XGroupCreateMkStream(ctx, q.cfg.Stream, q.cfg.Group, "0").Err(); err != nil {
		if err.Error() == "BUSYGROUP Consumer Group name already exists" {
			return nil
		}
		return err
	}
	return nil
}

func decodeMessage(m redis.XMessage) Message {
	fields := m.Values

	entity, _ := fields[fieldEntity].(string)
	operation, _ := fields[fieldOperation].(string)

	var payload []byte
	switch v := fields[fieldPayload].(type) {
	case string:
		payload = []byte(v)
	case []byte:
		payload = v
	}

	retryCount := 0
	switch v := fields[fieldRetryCount].(type) {
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			retryCount = n
		}
	case int64:
		retryCount = int(v)
	case int:
		retryCount = v
	}

	return Message{
		ID:         m.ID,
		Entity:     entity,
		Operation:  operation,
		Payload:    payload,
		RetryCount: retryCount,
	}
}

