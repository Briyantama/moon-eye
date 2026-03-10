package main

import (
	"context"
	"log"
	"os"

	"go.uber.org/zap"
)

// TODO: implement worker-service runners for projections and sync queues.

func main() {
	stdLogger := log.New(os.Stdout, "worker-service ", log.LstdFlags|log.LUTC|log.Lshortfile)

	zapLogger, err := zap.NewProduction()
	if err != nil {
		stdLogger.Fatalf("init zap logger: %v", err)
	}
	defer zapLogger.Sync() //nolint:errcheck

	ctx := context.Background()

	zapLogger.Info("starting worker-service")
	// TODO: initialize config, DB, Redis, and start worker.Runner instances for event and sync queues.

	<-ctx.Done()
	zapLogger.Info("worker-service exiting")
}

