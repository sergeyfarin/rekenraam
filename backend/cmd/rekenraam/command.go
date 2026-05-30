package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"rekenraam/backend/internal/api"
	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/config"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/web"
)

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	cfg := config.Load()
	logger := newLogger(cfg.AppEnv)

	if len(args) == 0 {
		return runServe(ctx, cfg, logger)
	}

	switch args[0] {
	case "serve":
		return runServe(ctx, cfg, logger)
	case "recover-owner":
		return runRecoverOwner(ctx, cfg, args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		return 2
	}
}

func runServe(ctx context.Context, cfg config.Config, logger *slog.Logger) int {
	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("open database", slog.Any("err", err))
		return 1
	}
	defer database.Close()

	if err := db.Migrate(ctx, database); err != nil {
		logger.Error("run migrations", slog.Any("err", err))
		return 1
	}

	setupRepository := db.NewSetupRepository(database)
	setupService := app.NewSetupService(setupRepository)
	authRepository := db.NewAuthRepository(database)
	authService := app.NewAuthService(authRepository)
	handler := api.NewHandler(logger, web.Handler(), setupService, authService, api.HandlerOptions{
		TrustProxyHeaders: cfg.TrustProxyHeaders,
		TrustedProxyCIDRs: cfg.TrustedProxyCIDRs,
	})
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
		return 1
	}

	return 0
}

func runRecoverOwner(ctx context.Context, cfg config.Config, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	flagSet := flag.NewFlagSet("recover-owner", flag.ContinueOnError)
	flagSet.SetOutput(stderr)

	var backupPath string
	var allowNoBackup bool
	var passwordStdin bool

	flagSet.StringVar(&backupPath, "backup-path", "", "path for the verified SQLite backup")
	flagSet.BoolVar(&allowNoBackup, "allow-no-backup", false, "permit recovery without creating a verified backup")
	flagSet.BoolVar(&passwordStdin, "password-stdin", false, "read the new owner password from stdin")

	if err := flagSet.Parse(args); err != nil {
		return 2
	}
	if flagSet.NArg() != 0 {
		fmt.Fprintln(stderr, "recover-owner does not accept positional arguments")
		return 2
	}
	if !passwordStdin {
		fmt.Fprintln(stderr, "recover-owner requires --password-stdin")
		return 2
	}

	password, err := readPasswordFromStdin(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "read password from stdin: %v\n", err)
		return 2
	}

	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(stderr, "open database: %v\n", err)
		return 1
	}
	defer database.Close()

	if err := db.Migrate(ctx, database); err != nil {
		fmt.Fprintf(stderr, "run migrations: %v\n", err)
		return 1
	}

	recoveryService := app.NewRecoveryService(database, cfg.DatabaseURL, db.NewRecoveryRepository(database))
	result, err := recoveryService.RecoverOwnerAccess(ctx, app.RecoverOwnerInput{
		Password:      password,
		BackupPath:    backupPath,
		AllowNoBackup: allowNoBackup,
	})
	if err != nil {
		fmt.Fprintf(stderr, "recover owner: %v\n", err)
		return 1
	}

	if result.BackupPath != "" {
		fmt.Fprintf(stdout, "owner recovery completed; verified backup written to %s\n", result.BackupPath)
		return 0
	}

	fmt.Fprintln(stdout, "owner recovery completed without backup")
	return 0
}

func readPasswordFromStdin(stdin io.Reader) (string, error) {
	passwordBytes, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
	if err != nil {
		return "", err
	}

	password := strings.TrimRight(string(passwordBytes), "\r\n")
	if password == "" {
		return "", fmt.Errorf("password is required")
	}

	return password, nil
}