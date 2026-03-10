package main

import (
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// TODO: implement auth-service HTTP API, JWT issuance, and OAuth2 login.

func main() {
	logger := log.New(os.Stdout, "auth-service ", log.LstdFlags|log.LUTC|log.Lshortfile)

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// TODO: mount /api/v1/auth routes.

	addr := ":8081"
	logger.Printf("starting auth-service on %s", addr)

	if err := http.ListenAndServe(addr, r); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("listen and serve: %v", err)
	}
}

