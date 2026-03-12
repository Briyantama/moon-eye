package syncworker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const (
	// EnvCredentialsFile is the path to a service account or application credentials JSON file.
	EnvCredentialsFile = "GOOGLE_APPLICATION_CREDENTIALS"
	// EnvCredentialsJSON is raw JSON (or base64-encoded JSON) of credentials for Google Sheets API.
	EnvCredentialsJSON = "GOOGLE_SHEETS_CREDENTIALS_JSON"
)

// NewSheetsServiceFromEnv creates a *sheets.Service using credentials from the environment.
// It tries, in order: GOOGLE_SHEETS_CREDENTIALS_JSON (raw or base64), GOOGLE_APPLICATION_CREDENTIALS file,
// then Application Default Credentials. Errors are wrapped; never panics.
// Returns (nil, nil) when no credentials are configured so the caller can fall back to NoopSheetsClient.
func NewSheetsServiceFromEnv(ctx context.Context) (*sheets.Service, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// No credentials configured: caller should use NoopSheetsClient
	if os.Getenv(EnvCredentialsJSON) == "" && os.Getenv(EnvCredentialsFile) == "" {
		return nil, nil
	}

	// 1) Inline JSON from env (preferred)
	if raw := os.Getenv(EnvCredentialsJSON); raw != "" {
		data := []byte(raw)
		if !json.Valid(data) {
			// Optional: env value may be base64-encoded JSON
			if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw)); err == nil && json.Valid(decoded) {
				data = decoded
			}
		}
		creds, err := google.CredentialsFromJSON(ctx, data, sheets.SpreadsheetsScope)
		if err != nil {
			return nil, fmt.Errorf("sheets credentials from JSON: %w", err)
		}
		svc, err := sheets.NewService(ctx, option.WithCredentials(creds))
		if err != nil {
			return nil, fmt.Errorf("sheets new service from credentials: %w", err)
		}
		return svc, nil
	}

	// 2) Credentials file path
	if path := os.Getenv(EnvCredentialsFile); path != "" {
		svc, err := sheets.NewService(ctx, option.WithCredentialsFile(path))
		if err != nil {
			return nil, fmt.Errorf("sheets new service from file %q: %w", path, err)
		}
		return svc, nil
	}

	// 3) No credentials configured: return (nil, nil) so caller uses NoopSheetsClient
	// 4) Otherwise try Application Default Credentials (e.g. on GCE/Cloud Run)
	svc, err := sheets.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("sheets new service (ADC): %w", err)
	}
	return svc, nil
}

// NewGoogleSheetsClientFromEnv builds a real GoogleSheetsClient when credentials are available,
// otherwise returns nil so the caller can use NoopSheetsClient.
func NewGoogleSheetsClientFromEnv(ctx context.Context) (*GoogleSheetsClient, error) {
	svc, err := NewSheetsServiceFromEnv(ctx)
	if err != nil || svc == nil {
		return nil, err
	}
	return NewGoogleSheetsClient(svc), nil
}

