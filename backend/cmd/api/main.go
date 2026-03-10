//go:build ignore

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"

	"moon-eye/backend/internal/api"
	"moon-eye/backend/internal/service"
	"moon-eye/backend/internal/syncworker"
	db "moon-eye/backend/internal/db"
)

type AppConfig struct {
	DBURL          string
	JWTSecret      string
	GoogleClientID string
	GoogleSecret   string
	RedisURL       string
	Addr           string
}

type App struct {
	Config    AppConfig
	DB        *sqlx.DB
	Redis     *redis.Client
	Router    *chi.Mux
	WorkerCtx context.Context
}

func loadConfig() AppConfig {
	cfg := AppConfig{
		DBURL:          os.Getenv("DB_URL"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		GoogleClientID: os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleSecret:   os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedisURL:       os.Getenv("REDIS_URL"),
		Addr:           os.Getenv("ADDR"),
	}

	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}

	return cfg
}

func mustConnectDB(dsn string) *sqlx.DB {
	if dsn == "" {
		log.Fatal("DB_URL must be set")
	}

	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	return db
}

func mustConnectRedis(rawURL string) *redis.Client {
	if rawURL == "" {
		// Allow running without Redis for local dev; workers will no-op.
		return nil
	}

	opts, err := redis.ParseURL(rawURL)
	if err != nil {
		log.Fatalf("parse redis url: %v", err)
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatalf("ping redis: %v", err)
	}

	return client
}

func newRouter(app *App, handler *api.Handler) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
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

	handler.RegisterRoutes(r)

	return r
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.LUTC | log.Lshortfile)

	cfg := loadConfig()

	db := mustConnectDB(cfg.DBURL)
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("close db: %v", err)
		}
	}()

	redisClient := mustConnectRedis(cfg.RedisURL)
	if redisClient != nil {
		defer func() {
			if err := redisClient.Close(); err != nil {
				log.Printf("close redis: %v", err)
			}
		}()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	txRepo := db.NewPGXTransactionRepository(db)
	txService := service.NewTransactionService(txRepo)
	handler := api.NewHandler(api.Services{
		Transactions: txService,
	})

	app := &App{
		Config:    cfg,
		DB:        db,
		Redis:     redisClient,
		WorkerCtx: ctx,
	}

	app.Router = newRouter(app, handler)

	// Start sync worker pool in background when Redis and DB are available.
	if redisClient != nil {
		go syncworker.StartWorker(ctx, db, redisClient)
	}

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      app.Router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("starting api server on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen and serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil && err != context.DeadlineExceeded {
		log.Printf("server shutdown error: %v", err)
	}
}

