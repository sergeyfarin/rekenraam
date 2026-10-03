package appruntime

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/config"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/lockfile"
)

func runtimeTestConfig(t *testing.T) (config.Config, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	return config.Config{
		AppEnv: "development", DatabaseURL: "file:" + path,
		SessionLifetime: 24 * time.Hour,
	}, path
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRuntimeConstructsSameAPIAndClosesResources(t *testing.T) {
	cfg, path := runtimeTestConfig(t)
	runtime, err := Open(context.Background(), cfg, quietLogger())
	require.NoError(t, err)
	connection := runtime.database
	for route, wantStatus := range map[string]int{
		"/healthz":       http.StatusNoContent,
		"/api/v1/health": http.StatusOK,
	} {
		response := httptest.NewRecorder()
		runtime.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
		require.Equal(t, wantStatus, response.Code, route)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, runtime.Close(ctx))
	require.NoError(t, runtime.Close(ctx), "close is idempotent")
	require.Error(t, connection.PingContext(context.Background()), "main pool must be closed")
	require.NoError(t, lockfile.CheckAvailable(path), "restore lock must be released")
}

func TestRuntimeStartupFailureReleasesLockAndOpenedDatabase(t *testing.T) {
	failed := errors.New("injected startup failure")
	for _, step := range []string{"database", "migration", "permissions", "read_only"} {
		t.Run(step, func(t *testing.T) {
			cfg, path := runtimeTestConfig(t)
			var opened *sql.DB
			deps := openDependencies{
				openDatabase: func(ctx context.Context, url string) (*sql.DB, error) {
					if step == "database" {
						return nil, failed
					}
					var err error
					opened, err = db.Open(ctx, url)
					return opened, err
				},
				migrate: func(ctx context.Context, database *sql.DB) error {
					if step == "migration" {
						return failed
					}
					return db.Migrate(ctx, database)
				},
				secureDatabase: func(url string) error {
					if step == "permissions" {
						return failed
					}
					return db.EnforceSQLiteFilePermissions(url)
				},
				openReadOnly: func(ctx context.Context, url string) (*sql.DB, error) {
					if step == "read_only" {
						return nil, failed
					}
					return db.OpenReadOnly(ctx, url)
				},
			}
			runtime, err := openWith(context.Background(), cfg, quietLogger(), deps)
			require.Nil(t, runtime)
			require.ErrorIs(t, err, failed)
			require.NoError(t, lockfile.CheckAvailable(path), "failed startup must release restore lock")
			if opened != nil {
				require.Error(t, opened.PingContext(context.Background()), "failed startup must close main pool")
			}
		})
	}
}

func TestRuntimeCloseDeadlineCanBeRetried(t *testing.T) {
	worker := make(chan struct{})
	runtime := &Runtime{closed: make(chan struct{}), workers: []<-chan struct{}{worker}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, runtime.Close(ctx), context.Canceled)
	close(worker)
	require.NoError(t, runtime.Close(context.Background()))
	require.NoError(t, runtime.Close(context.Background()))
}

func TestRuntimeRefusesTrading212BaseURLOutsideDevelopment(t *testing.T) {
	cfg, path := runtimeTestConfig(t)
	cfg.AppEnv = "production"
	cfg.Trading212BaseURL = "http://127.0.0.1:16890/api/v0"
	_, err := Open(context.Background(), cfg, quietLogger())
	require.ErrorContains(t, err, "only allowed in development")
	require.NoError(t, lockfile.CheckAvailable(path), "refused before taking the lock")
}
