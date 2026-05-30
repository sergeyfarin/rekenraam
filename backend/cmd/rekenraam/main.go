package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"rekenraam/backend/internal/api"
	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/config"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/web"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	logger := newLogger(cfg.AppEnv)

	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("open database", slog.Any("err", err))
		os.Exit(1)
	}
	defer database.Close()

	if err := db.Migrate(ctx, database); err != nil {
		logger.Error("run migrations", slog.Any("err", err))
		os.Exit(1)
	}

	setupRepository := db.NewSetupRepository(database)
	setupService := app.NewSetupService(setupRepository)
	handler := api.NewHandler(logger, web.Handler(), setupService)
	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: handler,
	}

	logger.Info("server starting",
		slog.String("addr", cfg.HTTPAddr),
		slog.String("app_env", cfg.AppEnv),
	)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("serve http", slog.Any("err", err))
		os.Exit(1)
	}
}

func newLogger(appEnv string) *slog.Logger {
	handlerOptions := &slog.HandlerOptions{Level: slog.LevelInfo}

	if appEnv == "development" {
		return slog.New(slog.NewTextHandler(os.Stdout, handlerOptions))
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, handlerOptions))
}
