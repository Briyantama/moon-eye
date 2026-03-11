package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MonthlyBalanceProjector updates monthly_balance from transaction change events.
// Each transaction contributes its amount to the account's month bucket (occurred_at).
const projectorNameMonthlyBalance = "monthly_balance"

// TransactionPayloadBalance is the minimal transaction shape for balance (matches domain.Transaction JSON).
type TransactionPayloadBalance struct {
	UserID     string    `json:"UserID"`
	AccountID  string    `json:"AccountID"`
	Amount     float64   `json:"Amount"`
	OccurredAt time.Time `json:"OccurredAt"`
}

// MonthlyBalanceProjector implements Processor for monthly_balance read model.
type MonthlyBalanceProjector struct {
	pool *pgxpool.Pool
}

// NewMonthlyBalanceProjector returns a Processor that maintains monthly_balance.
func NewMonthlyBalanceProjector(pool *pgxpool.Pool) *MonthlyBalanceProjector {
	return &MonthlyBalanceProjector{pool: pool}
}

func (p *MonthlyBalanceProjector) Process(ctx context.Context, event ChangeEventRow) error {
	if event.EntityType != "transaction" {
		return nil
	}
	var txn TransactionPayloadBalance
	if err := json.Unmarshal(event.Payload, &txn); err != nil {
		return fmt.Errorf("unmarshal transaction payload: %w", err)
	}
	monthKey := txn.OccurredAt.UTC().Format("2006-01")

	switch event.OpType {
	case "create":
		return p.add(ctx, txn.UserID, txn.AccountID, monthKey, txn.Amount)
	case "delete":
		return p.add(ctx, txn.UserID, txn.AccountID, monthKey, -txn.Amount)
	case "update":
		return p.add(ctx, txn.UserID, txn.AccountID, monthKey, txn.Amount)
	default:
		return nil
	}
}

func (p *MonthlyBalanceProjector) add(ctx context.Context, userID, accountID, monthKey string, amount float64) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO monthly_balance (user_id, account_id, month_key, balance, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, now())
		ON CONFLICT (user_id, account_id, month_key) DO UPDATE SET
			balance   = monthly_balance.balance + EXCLUDED.balance,
			updated_at = now()
	`, userID, accountID, monthKey, amount)
	return err
}
