package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"moon-eye/backend/internal/db"
	"moon-eye/backend/internal/queue"
	syncworker "moon-eye/backend/internal/sync"
	"moon-eye/backend/internal/worker"
	sharedcfg "moon-eye/backend/pkg/shared/config"
	shareddb "moon-eye/backend/pkg/shared/db"
	"moon-eye/backend/pkg/shared/redisx"
)

func main() {
	logger := log.New(os.Stdout, "sync-service ", log.LstdFlags|log.LUTC|log.Lshortfile)

	cfg, err := sharedcfg.Load("sync-service")
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	zapLogger, _ := zap.NewProduction()
	defer func() { _ = zapLogger.Sync() }()

	pool, err := shareddb.NewPostgresPool(ctx, cfg.Database.URL)
	if err != nil {
		logger.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	redisClient, err := redisx.NewClient(ctx, cfg.Redis.URL)
	if err != nil {
		logger.Fatalf("connect redis: %v", err)
	}
	defer func() { _ = redisClient.Close() }()

	// Wire repositories and sync service.
	connRepo := db.NewPGShedsConnectionRepository(pool)
	mappingRepo := db.NewPGSheetMappingRepository(pool)
	changeReader := db.NewPGChangeEventReader(pool)
	sheetsClient := &syncworker.NoopSheetsClient{}
	syncService := syncworker.NewSyncService(connRepo, mappingRepo, changeReader, sheetsClient)

	// Redis Streams queue for sync jobs.
	streamCfg := queue.RedisStreamConfig{
		Stream:       "sync_queue",
		Group:        "sync_workers",
		ConsumerName: "sync-service",
	}
	syncQueue := queue.NewRedisStreamQueue(redisClient, streamCfg)

	// Worker runner for sync jobs.
	runner := &worker.Runner{
		Queue:  syncQueue,
		Logger: zapLogger,
		Name:   "sync-worker",
		Handler: func(ctx context.Context, msg queue.Message) error {
			return syncService.HandleSyncJob(ctx, msg)
		},
	}

	go func() {
		if err := runner.Start(ctx); err != nil && err != context.Canceled {
			logger.Printf("sync worker error: %v", err)
		}
	}()

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		// Basic health: DB and Redis connectivity checks.
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"degraded","db":"unhealthy"}`))
			return
		}
		if err := redisClient.Ping(r.Context()).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"degraded","redis":"unhealthy"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// TODO: mount /api/v1/sheets and /api/v1/sync routes.

	addr := cfg.Server.Addr
	logger.Printf("starting sync-service on %s", addr)

	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("listen and serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Printf("server shutdown error: %v", err)
	}
}

