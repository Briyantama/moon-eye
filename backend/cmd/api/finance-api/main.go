package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"moon-eye/backend/pkg/shared/config"
	shareddb "moon-eye/backend/pkg/shared/db"
	"moon-eye/backend/pkg/shared/logging"
	"moon-eye/backend/pkg/shared/observability"
	"moon-eye/backend/pkg/shared/redisx"
)

// TODO: import finance-api internal packages (api, service, db, queue, worker) when implemented.

type appDeps struct {
	Logger *zap.Logger
	Config config.Config
	DB     *pgxpool.Pool
	Redis  *redis.Client
}

func main() {
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger, err := logging.NewLogger()
	if err != nil {
		panic(err)
	}
	defer logger.Sync() //nolint:errcheck

	ctx := logging.WithLogger(rootCtx, logger)

	cfg, err := config.Load("FINANCE_API")
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}

	dbPool, err := shareddb.NewPostgresPool(ctx, cfg.Database.URL)
	if err != nil {
		logger.Fatal("connect db", zap.Error(err))
	}
	defer dbPool.Close()

	var redisClient *redis.Client
	if cfg.Redis.URL != "" {
		redisClient, err = redisx.NewClient(ctx, cfg.Redis.URL)
		if err != nil {
			logger.Fatal("connect redis", zap.Error(err))
		}
		defer redisClient.Close()
	}

	deps := appDeps{
		Logger: logger,
		Config: cfg,
		DB:     dbPool,
		Redis:  redisClient,
	}

	router := newRouter(deps)

	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("starting finance-api", zap.String("addr", cfg.Server.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("listen and serve", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil && err != context.DeadlineExceeded {
		logger.Error("server shutdown error", zap.Error(err))
	}
}

func newRouter(deps appDeps) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// TODO: add Zap logging middleware.

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		// TODO: check DB and Redis connections and return detailed status.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Method(http.MethodGet, "/metrics", observability.Handler())

	// TODO: mount /api/v1 routes via internal/api once implemented.

	return r
}


