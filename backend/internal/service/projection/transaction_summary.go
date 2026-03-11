package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TransactionSummaryProjector updates transaction_summary from transaction change events.
// Idempotency is ensured by the runner processing each event once (cursor advance after success).
const projectorNameTransactionSummary = "transaction_summary"

// TransactionPayload is the minimal transaction shape in change_events payload (matches domain.Transaction JSON).
type TransactionPayload struct {
	UserID     string    `json:"UserID"`
	Amount     float64   `json:"Amount"`
	Currency   string    `json:"Currency"`
	Type       string    `json:"Type"`
	OccurredAt time.Time `json:"OccurredAt"`
	Deleted    bool      `json:"Deleted"`
}

// TransactionSummaryProjector implements Processor for transaction_summary read model.
type TransactionSummaryProjector struct {
	pool *pgxpool.Pool
}

// NewTransactionSummaryProjector returns a Processor that maintains transaction_summary.
func NewTransactionSummaryProjector(pool *pgxpool.Pool) *TransactionSummaryProjector {
	return &TransactionSummaryProjector{pool: pool}
}

func (p *TransactionSummaryProjector) Process(ctx context.Context, event ChangeEventRow) error {
	if event.EntityType != "transaction" {
		return nil
	}
	var txn TransactionPayload
	if err := json.Unmarshal(event.Payload, &txn); err != nil {
		return fmt.Errorf("unmarshal transaction payload: %w", err)
	}
	periodKey := txn.OccurredAt.UTC().Format("2006-01")
	if txn.Currency == "" {
		txn.Currency = "IDR"
	}
	if txn.Type == "" {
		txn.Type = "expense"
	}

	switch event.OpType {
	case "create":
		return p.add(ctx, txn.UserID, periodKey, txn.Currency, txn.Type, txn.Amount, 1)
	case "delete":
		return p.add(ctx, txn.UserID, periodKey, txn.Currency, txn.Type, -txn.Amount, -1)
	case "update":
		return p.add(ctx, txn.UserID, periodKey, txn.Currency, txn.Type, txn.Amount, 1)
	default:
		return nil
	}
}

func (p *TransactionSummaryProjector) add(ctx context.Context, userID, periodKey, currency, txnType string, amount float64, countDelta int64) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO transaction_summary (user_id, period_key, currency, type, total_amount, count, updated_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, now())
		ON CONFLICT (user_id, period_key, currency, type) DO UPDATE SET
			total_amount = transaction_summary.total_amount + EXCLUDED.total_amount,
			count        = transaction_summary.count + EXCLUDED.count,
			updated_at  = now()
	`, userID, periodKey, currency, txnType, amount, countDelta)
	return err
}
