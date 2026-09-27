package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/config"
	"rekenraam/backend/internal/lockfile"
)

func TestRunServeListenerFailureClosesApplicationRuntime(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	path := filepath.Join(t.TempDir(), "serve.sqlite")
	cfg := config.Config{AppEnv: "development", HTTPAddr: listener.Addr().String(),
		DatabaseURL: "file:" + path, SessionLifetime: 24 * time.Hour}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	require.Equal(t, 1, runServe(context.Background(), cfg, logger))
	require.NoError(t, lockfile.CheckAvailable(path))
}

func TestRunServeCancellationClosesApplicationRuntime(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	path := filepath.Join(t.TempDir(), "serve.sqlite")
	cfg := config.Config{AppEnv: "development", HTTPAddr: addr,
		DatabaseURL: "file:" + path, SessionLifetime: 24 * time.Hour}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan int, 1)
	go func() { result <- runServe(ctx, cfg, logger) }()
	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.After(5 * time.Second)
	for {
		response, err := client.Get("http://" + addr + "/healthz")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusNoContent {
				break
			}
		}
		select {
		case code := <-result:
			require.FailNowf(t, "server exited before health check", "exit code %d", code)
		case <-deadline:
			cancel()
			require.FailNow(t, "server did not become healthy")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case code := <-result:
		require.Zero(t, code)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "server did not stop after cancellation")
	}
	require.NoError(t, lockfile.CheckAvailable(path))
}
