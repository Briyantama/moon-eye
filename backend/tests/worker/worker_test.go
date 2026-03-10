package worker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moon-eye/backend/internal/queue"
	worker "moon-eye/backend/internal/worker"
)

type stubConsumer struct {
	messages []queue.Message
	calls    int
}

func (s *stubConsumer) Consume(ctx context.Context, handler func(context.Context, queue.Message) error) error {
	for _, m := range s.messages {
		s.calls++
		if err := handler(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func TestRunner_Start_InvokesHandler(t *testing.T) {
	consumer := &stubConsumer{
		messages: []queue.Message{
			{ID: "1-0", Entity: "transaction", Operation: "create"},
		},
	}

	var handled int
	r := &worker.Runner{
		Queue:  consumer,
		Logger: nil,
		Name:   "test-runner",
		Handler: func(ctx context.Context, msg queue.Message) error {
			_ = ctx
			handled++
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := r.Start(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, handled)
	require.Equal(t, 1, consumer.calls)
}

func TestRunner_Start_PropagatesError(t *testing.T) {
	consumer := &stubConsumer{
		messages: []queue.Message{
			{ID: "1-0", Entity: "transaction", Operation: "create"},
		},
	}

	r := &worker.Runner{
		Queue:  consumer,
		Logger: nil,
		Name:   "test-runner",
		Handler: func(ctx context.Context, msg queue.Message) error {
			_ = ctx
			_ = msg
			return errors.New("fail")
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := r.Start(ctx)
	require.Error(t, err)
	require.Equal(t, 1, consumer.calls)
}

