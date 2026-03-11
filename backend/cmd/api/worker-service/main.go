package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"moon-eye/backend/internal/service/projection"
	sharedcfg "moon-eye/backend/pkg/shared/config"
	shareddb "moon-eye/backend/pkg/shared/db"
)

func main() {
	stdLogger := log.New(os.Stdout, "worker-service ", log.LstdFlags|log.LUTC|log.Lshortfile)

	zapLogger, err := zap.NewProduction()
	if err != nil {
		stdLogger.Fatalf("init zap logger: %v", err)
	}
	defer zapLogger.Sync() //nolint:errcheck

	cfg, err := sharedcfg.Load("worker-service")
	if err != nil {
		stdLogger.Fatalf("load config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := shareddb.NewPostgresPool(ctx, cfg.Database.URL)
	if err != nil {
		stdLogger.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	source := projection.NewDBChangeEventSource(pool)
	cursor := projection.NewDBCursorStore(pool)
	summaryProjector := projection.NewTransactionSummaryProjector(pool)
	balanceProjector := projection.NewMonthlyBalanceProjector(pool)

	runner := &projection.Runner{
		Name:         "default",
		Source:       source,
		Cursor:       cursor,
		Processors:   []projection.Processor{summaryProjector, balanceProjector},
		Logger:       zapLogger,
		BatchSize:    50,
		PollInterval: 2 * time.Second,
	}

	go func() {
		zapLogger.Info("starting projection runner")
		if err := runner.Start(ctx); err != nil && err != context.Canceled {
			zapLogger.Warn("projection runner exited", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	zapLogger.Info("shutdown signal received")
	cancel()

	// Allow a short time for the runner to exit.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = shutdownCtx

	zapLogger.Info("worker-service exiting")
}
