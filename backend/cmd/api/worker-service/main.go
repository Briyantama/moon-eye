package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"moon-eye/backend/internal/db"
	sqlcdb "moon-eye/backend/internal/db/sqlc"
	"moon-eye/backend/internal/queue"
	"moon-eye/backend/internal/service/projection"
	"moon-eye/backend/internal/worker"
	syncworker "moon-eye/backend/internal/sync"
	"moon-eye/backend/pkg/shared/config"
	shareddb "moon-eye/backend/pkg/shared/db"
	"moon-eye/backend/pkg/shared/logging"
	"moon-eye/backend/pkg/shared/redisx"
)

const (
	shutdownTimeout = 10 * time.Second
	projectionName  = "default"
	batchSize       = 50
	pollInterval    = 2 * time.Second
	syncStream      = "sync_queue"
	syncGroup       = "sync_workers"
	syncConsumer    = "worker-service"
)

func main() {
	ctx := context.Background()

	logger, err := logging.NewLogger()
	if err != nil {
		panic(err)
	}
	defer logger.Sync() //nolint:errcheck
	ctx = logging.WithLogger(ctx, logger)

	cfg, err := config.Load("WORKER_SERVICE")
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}

	pool, err := shareddb.NewPostgresPool(ctx, cfg.Database.URL)
	if err != nil {
		logger.Fatal("connect db", zap.Error(err))
	}
	defer pool.Close()

	queries := sqlcdb.New(pool)
	_ = db.NewRepositories(pool, queries)
	_ = db.NewTxRunner(pool)

	source := projection.NewDBChangeEventSource(pool)
	cursor := projection.NewDBCursorStore(pool)
	summaryProjector := projection.NewTransactionSummaryProjector(pool)
	balanceProjector := projection.NewMonthlyBalanceProjector(pool)

	projRunner := &projection.Runner{
		Name:         projectionName,
		Source:       source,
		Cursor:       cursor,
		Processors:   []projection.Processor{summaryProjector, balanceProjector},
		Logger:       logger,
		BatchSize:    batchSize,
		PollInterval: pollInterval,
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		logger.Info("starting projection runner")
		if err := projRunner.Start(runCtx); err != nil && err != context.Canceled {
			logger.Warn("projection runner exited", zap.Error(err))
		}
	}()

	var syncRunner *worker.Runner
	if cfg.Redis.URL != "" {
		redisClient, err := redisx.NewClient(ctx, cfg.Redis.URL)
		if err != nil {
			logger.Fatal("connect redis", zap.Error(err))
		}
		defer redisClient.Close()

		syncQueue := queue.NewRedisStreamQueue(redisClient, queue.RedisStreamConfig{
			Stream:       syncStream,
			Group:        syncGroup,
			ConsumerName: syncConsumer,
			BlockTimeout: 5 * time.Second,
		})

		sheetsConn := db.NewPGShedsConnectionRepository(pool)
		sheetsMapping := db.NewPGSheetMappingRepository(pool)
		changeEventReader := db.NewPGChangeEventReader(pool)
		syncService := syncworker.NewSyncService(sheetsConn, sheetsMapping, changeEventReader, &syncworker.NoopSheetsClient{})

		syncRunner = &worker.Runner{
			Queue:   syncQueue,
			Logger:  logger,
			Name:    "sync",
			Handler: syncService.HandleSyncJob,
		}
		go func() {
			logger.Info("starting sync worker")
			if err := syncRunner.Start(runCtx); err != nil && err != context.Canceled {
				logger.Warn("sync worker exited", zap.Error(err))
			}
		}()
	} else {
		logger.Info("redis not configured; sync worker disabled")
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutdown signal received")
	cancel()

	time.Sleep(500 * time.Millisecond)
	logger.Info("worker-service exiting")
}
