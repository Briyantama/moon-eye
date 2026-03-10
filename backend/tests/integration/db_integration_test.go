package db_test

import (
	"context"
	"testing"

	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// NOTE: This is a skeleton for containerized integration tests using testcontainers-go.

func TestPostgresContainer(t *testing.T) {
	t.Skip("TODO: implement integration test that runs migrations and verifies repositories")

	ctx := context.Background()

	req := tc.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "postgres",
			"POSTGRES_DB":       "finance",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp"),
	}

	postgres, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	defer postgres.Terminate(ctx) //nolint:errcheck
}

