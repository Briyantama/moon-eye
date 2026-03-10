package api

import "github.com/go-chi/chi/v5"

// RegisterFinanceRoutes wires finance-api HTTP routes onto the given router
// using the existing Handler type.
func RegisterFinanceRoutes(r chi.Router, h *Handler) {
	h.RegisterRoutes(r)
}


