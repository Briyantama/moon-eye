package main

import (
	"context"
	"encoding/json"
	"net"
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

	"moon-eye/backend/internal/api"
	"moon-eye/backend/internal/db"
	sqlcdb "moon-eye/backend/internal/db/sqlc"
	"moon-eye/backend/internal/queue"
	"moon-eye/backend/internal/service"
	syncworker "moon-eye/backend/internal/sync"
	"moon-eye/backend/internal/wiring"
	"moon-eye/backend/pkg/shared/config"
	shareddb "moon-eye/backend/pkg/shared/db"
	"moon-eye/backend/pkg/shared/logging"
	"moon-eye/backend/pkg/shared/observability"
	"moon-eye/backend/pkg/shared/redisx"
)

const shutdownTimeout = 10 * time.Second

// App holds the HTTP server and runs it with graceful shutdown.
type App struct {
	logger *zap.Logger
	srv    *http.Server
}

// Run starts the server and blocks until a shutdown signal (SIGINT/SIGTERM) or context cancel.
// It then performs graceful shutdown: stops accepting new connections and waits for in-flight requests.
func (a *App) Run(ctx context.Context) error {
	baseCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.srv.BaseContext = func(net.Listener) context.Context { return baseCtx }

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("starting finance-api", zap.String("addr", a.srv.Addr))
		if err := a.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("server exited", zap.Error(err))
			errCh <- err
			cancel()
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		a.logger.Info("shutdown signal received", zap.String("signal", sig.String()))
	case <-baseCtx.Done():
		a.logger.Info("context cancelled")
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := a.srv.Shutdown(shutdownCtx); err != nil && err != context.DeadlineExceeded {
		a.logger.Error("server shutdown error", zap.Error(err))
		return err
	}
	a.logger.Info("server stopped")
	return nil
}

func main() {
	ctx := context.Background()

	logger, err := logging.NewLogger()
	if err != nil {
		panic(err)
	}
	defer logger.Sync() //nolint:errcheck
	ctx = logging.WithLogger(ctx, logger)

	cfg, err := config.Load("FINANCE_API")
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}
	if cfg.Auth.JWTSecret != "" {
		_ = os.Setenv("JWT_SIGNING_KEY", cfg.Auth.JWTSecret)
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

	app, err := newApp(ctx, logger, cfg, dbPool, redisClient)
	if err != nil {
		logger.Fatal("init app", zap.Error(err))
	}

	if err := app.Run(ctx); err != nil {
		logger.Fatal("run", zap.Error(err))
	}
}

// newApp builds services, router, and HTTP server. Caller owns dbPool and redisClient lifecycle.
func newApp(ctx context.Context, logger *zap.Logger, cfg config.Config, dbPool *pgxpool.Pool, redisClient *redis.Client) (*App, error) {
	queries := sqlcdb.New(dbPool)
	repos := db.NewRepositories(dbPool, queries)
	txRunner := db.NewTxRunner(dbPool)

	txManager := wiring.NewTxManagerAdapter(txRunner, repos.Transactions, repos.ChangeEvents)
	txRepo := wiring.NewServiceTransactionAdapter(repos.Transactions, nil)
	changeEventsWriter := service.NewDBChangeEventWriter(repos.ChangeEvents)

	var syncQueue service.SyncQueue
	if redisClient != nil {
		syncQueue = service.NewRedisSyncQueue(queue.NewRedisStreamQueue(redisClient, queue.RedisStreamConfig{Stream: "sync_queue"}))
	}
	transactionService := service.NewTransactionService(txRepo, changeEventsWriter, syncQueue, txManager)
	authService := service.NewAuthService(repos.Users, repos.RefreshTokens, txRunner)

	sheetsConn := db.NewPGShedsConnectionRepository(dbPool)
	sheetsMapping := db.NewPGSheetMappingRepository(dbPool)
	changeEventReader := db.NewPGChangeEventReader(dbPool)
	var sheetsClient syncworker.SheetsClient = &syncworker.NoopSheetsClient{}
	if client, err := syncworker.NewGoogleSheetsClientFromEnv(ctx); err == nil && client != nil {
		sheetsClient = client
		logger.Info("Google Sheets client enabled (credentials from env)")
	} else {
		logger.Info("Google Sheets client not configured; using noop (set GOOGLE_APPLICATION_CREDENTIALS or GOOGLE_SHEETS_CREDENTIALS_JSON for real sync)")
	}
	syncService := syncworker.NewSyncService(sheetsConn, sheetsMapping, changeEventReader, sheetsClient)

	handler := api.NewHandler(api.Services{
		Transactions: transactionService,
		Auth:         authService,
		Sync:         syncService,
		SyncQueue:    syncQueue,
		Connections:  sheetsConn,
	})

	router := newRouter(logger, dbPool, redisClient, handler)
	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return &App{logger: logger, srv: srv}, nil
}

func newRouter(logger *zap.Logger, pool *pgxpool.Pool, redisClient *redis.Client, handler *api.Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, middleware.Timeout(60*time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders: []string{"Link"},
		AllowCredentials: true,
		MaxAge:          300,
	}))

	r.Get("/health", healthHandler(pool, redisClient))
	r.Method(http.MethodGet, "/metrics", observability.Handler())
	handler.RegisterAPIRoutes(r)
	return r
}

func healthHandler(pool *pgxpool.Pool, redisClient *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		status := struct {
			Status   string `json:"status"`
			Database string `json:"database,omitempty"`
			Redis    string `json:"redis,omitempty"`
		}{Status: "ok"}

		if err := pool.Ping(ctx); err != nil {
			status.Status, status.Database = "degraded", "unhealthy"
		} else {
			status.Database = "healthy"
		}
		if redisClient != nil {
			if err := redisClient.Ping(ctx).Err(); err != nil {
				if status.Status == "ok" {
					status.Status = "degraded"
				}
				status.Redis = "unhealthy"
			} else {
				status.Redis = "healthy"
			}
		}

		code := http.StatusOK
		if status.Status == "degraded" {
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(status)
	}
}
