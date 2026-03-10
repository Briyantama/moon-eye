package syncworker

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// SyncQueueItem mirrors a row from the sync_queue table in Postgres.
type SyncQueueItem struct {
	ID          int64           `db:"id"`
	UserID      string          `db:"user_id"`
	OpType      string          `db:"op_type"`
	Payload     json.RawMessage `db:"payload"`
	Attempts    int             `db:"attempts"`
	AvailableAt time.Time       `db:"available_at"`
	MaxAttempts int             `db:"max_attempts"`
	CreatedAt   time.Time       `db:"created_at"`
}

// StartWorker starts a simple worker loop that periodically pulls items from
// the sync_queue table and processes them. This is intentionally simple;
// production setups may shard and parallelize workers by user or tenant.
func StartWorker(ctx context.Context, db *sql.DB, redisClient *redis.Client) {
	log.Println("[sync-worker] starting")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[sync-worker] context canceled, stopping")
			return
		case <-ticker.C:
			if err := consumeOnce(ctx, db, redisClient); err != nil {
				log.Printf("[sync-worker] error consuming queue: %v", err)
			}
		}
	}
}

func consumeOnce(ctx context.Context, db *sql.DB, redisClient *redis.Client) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var item SyncQueueItem
	query := `
		SELECT id, user_id, op_type, payload, attempts, available_at, max_attempts, created_at
		FROM sync_queue
		WHERE available_at <= now()
		ORDER BY available_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1;
	`
	row := tx.QueryRowContext(ctx, query)
	if err = row.Scan(
		&item.ID,
		&item.UserID,
		&item.OpType,
		&item.Payload,
		&item.Attempts,
		&item.AvailableAt,
		&item.MaxAttempts,
		&item.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			// Nothing to process.
			err = nil
		}
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	if item.ID == 0 {
		return nil
	}

	if err = processQueueItem(ctx, db, redisClient, &item); err != nil {
		log.Printf("[sync-worker] process item %d failed: %v", item.ID, err)
		return markAttemptFailure(ctx, db, &item, err)
	}

	return deleteQueueItem(ctx, db, item.ID)
}

// processQueueItem executes the core two-way sync loop for one logical sync
// operation. The implementation is intentionally high-level and can be
// expanded to match the detailed pseudocode in the design document.
func processQueueItem(ctx context.Context, db *sql.DB, redisClient *redis.Client, item *SyncQueueItem) error {
	// TODO: implement two-way sync using:
	// - change_events table for local changes
	// - Google Sheets API for remote changes
	// - conflict resolution by version and last_modified

	log.Printf("[sync-worker] processing item %d for user %s op=%s", item.ID, item.UserID, item.OpType)
	time.Sleep(100 * time.Millisecond)

	return nil
}

func markAttemptFailure(ctx context.Context, db *sql.DB, item *SyncQueueItem, processErr error) error {
	item.Attempts++
	backoff := time.Duration(1<<item.Attempts) * time.Second
	if item.Attempts >= item.MaxAttempts {
		log.Printf("[sync-worker] item %d reached max attempts; marking as permanent failure", item.ID)
		_, err := db.ExecContext(ctx,
			`UPDATE sync_queue SET attempts = $1, available_at = now() WHERE id = $2`,
			item.Attempts,
			item.ID,
		)
		return err
	}

	nextAvailable := time.Now().Add(backoff)
	_, err := db.ExecContext(ctx,
		`UPDATE sync_queue SET attempts = $1, available_at = $2 WHERE id = $3`,
		item.Attempts,
		nextAvailable,
		item.ID,
	)
	return err
}

func deleteQueueItem(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM sync_queue WHERE id = $1`, id)
	return err
}

