package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"moon-eye/backend/internal/domain"
	"moon-eye/backend/internal/service"
	"moon-eye/backend/pkg/shared/httpx"
)

type Services struct {
	Transactions *service.TransactionService
}

type Handler struct {
	svc Services
}

func NewHandler(svc Services) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/health", h.handleHealth)
	r.Get("/auth/oauth-callback", h.handleOAuthCallback)

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/transactions", func(r chi.Router) {
			r.Get("/", h.handleListTransactions)
			r.Post("/", h.handleCreateTransaction)
			r.Get("/{id}", h.handleGetTransaction)
			r.Put("/{id}", h.handleUpdateTransaction)
			r.Delete("/{id}", h.handleDeleteTransaction)
		})
	})
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (h *Handler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write([]byte("oauth callback not implemented yet"))
}

func (h *Handler) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	// TODO: extract real userID from auth once implemented.
	userID := r.Header.Get("X-Debug-User-ID")
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing user")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	items, page, err := h.svc.Transactions.ListUserTransactionsWithCount(r.Context(), userID, limit, offset)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list transactions")
		return
	}

	resp := listTransactionsResponse{
		Items:      mapTransactions(items),
		Pagination: paginationResponse{Limit: page.Limit, Offset: page.Offset, Total: page.Total},
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

type createTransactionRequest struct {
	AccountID   string             `json:"accountId"`
	Amount      float64            `json:"amount"`
	Currency    string             `json:"currency"`
	Type        string             `json:"type"`
	CategoryID  *string            `json:"categoryId"`
	Description *string            `json:"description"`
	OccurredAt  string             `json:"occurredAt"`
	Metadata    map[string]any     `json:"metadata"`
	Source      string             `json:"source"`
}

func (h *Handler) handleCreateTransaction(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-Debug-User-ID")
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing user")
		return
	}

	var req createTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid body")
		return
	}

	occurredAt, err := time.Parse(time.RFC3339, req.OccurredAt)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid occurredAt")
		return
	}

	in := service.CreateTransactionInput{
		UserID:      userID,
		AccountID:   req.AccountID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Type:        req.Type,
		CategoryID:  req.CategoryID,
		Description: req.Description,
		OccurredAt:  occurredAt,
		Metadata:    req.Metadata,
		Source:      req.Source,
	}

	tx, err := h.svc.Transactions.CreateTransaction(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to create transaction")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, mapTransaction(tx))
}

func (h *Handler) handleGetTransaction(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-Debug-User-ID")
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing user")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "missing id")
		return
	}

	tx, err := h.svc.Transactions.GetTransaction(r.Context(), userID, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to get transaction")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, mapTransaction(tx))
}

func (h *Handler) handleUpdateTransaction(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-Debug-User-ID")
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing user")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "missing id")
		return
	}

	var req createTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid body")
		return
	}

	occurredAt, err := time.Parse(time.RFC3339, req.OccurredAt)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid occurredAt")
		return
	}

	in := service.UpdateTransactionInput{
		AccountID:   req.AccountID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Type:        req.Type,
		CategoryID:  req.CategoryID,
		Description: req.Description,
		OccurredAt:  occurredAt,
		Metadata:    req.Metadata,
		Source:      req.Source,
	}

	tx, err := h.svc.Transactions.UpdateTransaction(r.Context(), userID, id, in)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to update transaction")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, mapTransaction(tx))
}

func (h *Handler) handleDeleteTransaction(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-Debug-User-ID")
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing user")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "missing id")
		return
	}

	if _, err := h.svc.Transactions.SoftDeleteTransaction(r.Context(), userID, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to delete transaction")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// listTransactionsResponse wraps a list of transactions with pagination metadata.
type listTransactionsResponse struct {
	Items      []transactionResponse `json:"items"`
	Pagination paginationResponse    `json:"pagination"`
}

type paginationResponse struct {
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
	Total  int64 `json:"total"`
}

// transactionResponse is the public JSON representation of a transaction.
type transactionResponse struct {
	ID           string             `json:"id"`
	UserID       string             `json:"userId"`
	AccountID    string             `json:"accountId"`
	Amount       float64            `json:"amount"`
	Currency     string             `json:"currency"`
	Type         string             `json:"type"`
	CategoryID   *string            `json:"categoryId,omitempty"`
	Description  *string            `json:"description,omitempty"`
	OccurredAt   time.Time          `json:"occurredAt"`
	Metadata     map[string]any     `json:"metadata,omitempty"`
	Version      int64              `json:"version"`
	LastModified time.Time          `json:"lastModified"`
	Source       string             `json:"source"`
	SheetsRowID  *string            `json:"sheetsRowId,omitempty"`
	Deleted      bool               `json:"deleted"`
}

func mapTransactions(in []domain.Transaction) []transactionResponse {
	out := make([]transactionResponse, 0, len(in))
	for _, t := range in {
		out = append(out, mapTransaction(&t))
	}
	return out
}

func mapTransaction(t *domain.Transaction) transactionResponse {
	if t == nil {
		return transactionResponse{}
	}

	return transactionResponse{
		ID:           t.ID,
		UserID:       t.UserID,
		AccountID:    t.AccountID,
		Amount:       t.Amount,
		Currency:     t.Currency,
		Type:         t.Type,
		CategoryID:   t.CategoryID,
		Description:  t.Description,
		OccurredAt:   t.OccurredAt,
		Metadata:     t.Metadata,
		Version:      t.Version,
		LastModified: t.LastModified,
		Source:       t.Source,
		SheetsRowID:  t.SheetsRowID,
		Deleted:      t.Deleted,
	}
}

