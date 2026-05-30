package main

import (
	"context"
	"log/slog"
	"os"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func newLogger(appEnv string) *slog.Logger {
	handlerOptions := &slog.HandlerOptions{Level: slog.LevelInfo}

	if appEnv == "development" {
		return slog.New(slog.NewTextHandler(os.Stdout, handlerOptions))
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, handlerOptions))
}
