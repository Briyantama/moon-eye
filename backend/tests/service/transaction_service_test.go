package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"moon-eye/backend/internal/domain"
	"moon-eye/backend/mocks"
	service "moon-eye/backend/internal/service"
)

// NOTE: These lightweight test doubles mirror the behavior of mockery-generated
// mocks without pulling in the full generated code. They satisfy the same
// interfaces as TransactionRepository, ChangeEventWriter, and SyncQueue.

type mockTransactionRepository struct {
	created       *domain.Transaction
	updated       *domain.Transaction
	deleted       *domain.Transaction
	list          []domain.Transaction
	count         int64
	getByID       *domain.Transaction
	getByIDErr    error
	updateErr     error
	softDeleteErr error
}

func (m *mockTransactionRepository) ListByUser(ctx context.Context, userID string, limit, offset int) ([]domain.Transaction, error) {
	return m.list, nil
}

func (m *mockTransactionRepository) CountByUser(ctx context.Context, userID string) (int64, error) {
	return m.count, nil
}

func (m *mockTransactionRepository) Create(ctx context.Context, tx *domain.Transaction) error {
	m.created = tx
	tx.ID = "txn-1"
	return nil
}

func (m *mockTransactionRepository) GetByID(ctx context.Context, userID, id string) (*domain.Transaction, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	return m.getByID, nil
}

func (m *mockTransactionRepository) Update(ctx context.Context, tx *domain.Transaction) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updated = tx
	// Simulate version bump performed in SQL.
	if m.updated != nil {
		m.updated.Version++
	}
	return nil
}

func (m *mockTransactionRepository) SoftDelete(ctx context.Context, userID, id string) (*domain.Transaction, error) {
	if m.softDeleteErr != nil {
		return nil, m.softDeleteErr
	}
	if m.deleted == nil {
		return nil, errors.New("no transaction to delete")
	}
	m.deleted.Deleted = true
	m.deleted.Version++
	return m.deleted, nil
}

var _ service.TransactionRepository = (*mockTransactionRepository)(nil)

type mockChangeEventWriter struct {
	events []service.ChangeEventInput
	err    error
}

func (m *mockChangeEventWriter) Create(ctx context.Context, e service.ChangeEventInput) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, e)
	return nil
}

var _ service.ChangeEventWriter = (*mockChangeEventWriter)(nil)

type mockSyncQueue struct {
	jobs []service.SyncJob
}

func (m *mockSyncQueue) EnqueueSyncJob(ctx context.Context, job service.SyncJob) error {
	m.jobs = append(m.jobs, job)
	return nil
}

var _ service.SyncQueue = (*mockSyncQueue)(nil)

type mockTxManager struct {
	err    error
	called int
}

func (m *mockTxManager) RunInTx(ctx context.Context, fn func(ctx context.Context, uow service.TransactionUnitOfWork) error) error {
	m.called++
	if m.err != nil {
		return m.err
	}
	uowImpl := &mockUnitOfWork{
		txRepo:  &mockTransactionRepository{},
		events:  &mockChangeEventWriter{},
	}
	return fn(ctx, uowImpl)
}

type mockUnitOfWork struct {
	txRepo  *mockTransactionRepository
	events  *mockChangeEventWriter
}

func (u *mockUnitOfWork) Transactions() service.TransactionRepository {
	return u.txRepo
}

func (u *mockUnitOfWork) ChangeEvents() service.ChangeEventWriter {
	return u.events
}

func TestTransactionService_CreateTransaction_PublishesEventAndSync(t *testing.T) {
	repo := &mockTransactionRepository{}
	events := &mockChangeEventWriter{}
	syncQ := &mockSyncQueue{}

	svc := service.NewTransactionService(repo, events, syncQ, nil)

	in := service.CreateTransactionInput{
		UserID:     "user-1",
		AccountID:  "acct-1",
		Amount:     100,
		Currency:   "USD",
		Type:       "expense",
		OccurredAt: time.Now(),
		Metadata:   map[string]any{"note": "test"},
		Source:     "app",
	}

	ctx := context.Background()
	tx, err := svc.CreateTransaction(ctx, in)
	require.NoError(t, err)
	require.NotNil(t, tx)

	require.NotNil(t, repo.created)
	require.Len(t, syncQ.jobs, 1)
}

func TestTransactionService_UpdateTransaction_EmitsUpdateEventAndSync(t *testing.T) {
	now := time.Now().UTC()
	existing := &domain.Transaction{
		ID:           "txn-1",
		UserID:       "user-1",
		AccountID:    "acct-1",
		Amount:       50,
		Currency:     "USD",
		Type:         "expense",
		OccurredAt:   now,
		Version:      1,
		LastModified: now,
	}

	repo := &mockTransactionRepository{
		getByID: existing,
	}
	events := &mockChangeEventWriter{}
	syncQ := &mockSyncQueue{}

	svc := service.NewTransactionService(repo, events, syncQ, nil)

	in := service.UpdateTransactionInput{
		AccountID:  "acct-2",
		Amount:     75,
		Currency:   "USD",
		Type:       "expense",
		OccurredAt: now.Add(time.Hour),
		Metadata:   map[string]any{"note": "updated"},
		Source:     "app",
	}

	ctx := context.Background()
	tx, err := svc.UpdateTransaction(ctx, "user-1", "txn-1", in)
	require.NoError(t, err)
	require.NotNil(t, tx)

	require.Equal(t, "acct-2", tx.AccountID)
	require.Equal(t, 75.0, tx.Amount)
	require.Len(t, events.events, 1)
	require.Equal(t, "update", events.events[0].Operation)
	require.Len(t, syncQ.jobs, 1)
	require.Equal(t, "update", syncQ.jobs[0].Operation)
}

func TestTransactionService_SoftDeleteTransaction_EmitsDeleteEventAndSync(t *testing.T) {
	now := time.Now().UTC()
	existing := &domain.Transaction{
		ID:           "txn-1",
		UserID:       "user-1",
		AccountID:    "acct-1",
		Amount:       50,
		Currency:     "USD",
		Type:         "expense",
		OccurredAt:   now,
		Version:      1,
		LastModified: now,
	}

	repo := &mockTransactionRepository{
		deleted: existing,
	}
	events := &mockChangeEventWriter{}
	syncQ := &mockSyncQueue{}

	svc := service.NewTransactionService(repo, events, syncQ, nil)

	ctx := context.Background()
	tx, err := svc.SoftDeleteTransaction(ctx, "user-1", "txn-1")
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.True(t, tx.Deleted)

	require.Len(t, events.events, 1)
	require.Equal(t, "delete", events.events[0].Operation)
	require.Len(t, syncQ.jobs, 1)
	require.Equal(t, "delete", syncQ.jobs[0].Operation)
}

// --- Transactional tests using generated mocks for TxManager and SyncQueue ---

func TestTransactionService_CreateTransaction_Tx_Success(t *testing.T) {
	repo := &mockTransactionRepository{}
	events := &mockChangeEventWriter{}
	syncQ := mocks.NewMockSyncQueue(t)
	txMgr := mocks.NewMockTxManager(t)

	syncQ.EXPECT().
		EnqueueSyncJob(mock.Anything, mock.Anything).
		Return(nil)

	txMgr.EXPECT().
		RunInTx(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context, service.TransactionUnitOfWork) error) error {
			uow := &mockUnitOfWork{
				txRepo: repo,
				events: events,
			}
			return fn(ctx, uow)
		})

	svc := service.NewTransactionService(repo, events, syncQ, txMgr)

	in := service.CreateTransactionInput{
		UserID:     "user-1",
		AccountID:  "acct-1",
		Amount:     100,
		Currency:   "USD",
		Type:       "expense",
		OccurredAt: time.Now(),
		Metadata:   map[string]any{"note": "tx-success"},
		Source:     "app",
	}

	ctx := context.Background()
	tx, err := svc.CreateTransaction(ctx, in)
	require.NoError(t, err)
	require.NotNil(t, tx)

	// Event and sync job should have been emitted.
	require.Len(t, events.events, 1)
	require.Len(t, syncQ.Calls, 1)
}

func TestTransactionService_CreateTransaction_Tx_RollbackOnChangeEventFailure(t *testing.T) {
	repo := &mockTransactionRepository{}
	events := &mockChangeEventWriter{
		err: errors.New("write failure"),
	}
	syncQ := mocks.NewMockSyncQueue(t)
	txMgr := mocks.NewMockTxManager(t)

	txMgr.EXPECT().
		RunInTx(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context, service.TransactionUnitOfWork) error) error {
			uow := &mockUnitOfWork{
				txRepo: repo,
				events: events,
			}
			return fn(ctx, uow)
		})

	svc := service.NewTransactionService(repo, events, syncQ, txMgr)

	in := service.CreateTransactionInput{
		UserID:     "user-1",
		AccountID:  "acct-1",
		Amount:     100,
		Currency:   "USD",
		Type:       "expense",
		OccurredAt: time.Now(),
		Metadata:   map[string]any{"note": "tx-rollback"},
		Source:     "app",
	}

	ctx := context.Background()
	tx, err := svc.CreateTransaction(ctx, in)
	require.Error(t, err)
	require.Nil(t, tx)

	// No successful change events recorded due to failure.
	require.Len(t, events.events, 0)

	// Sync queue must not be called when the transaction fails.
	syncQ.AssertNotCalled(t, "EnqueueSyncJob", mock.Anything, mock.Anything)
}

