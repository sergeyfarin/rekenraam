package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/appruntime"
	"rekenraam/backend/internal/config"
	"rekenraam/backend/internal/db"
)

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 2
	}
	logger := newLogger(cfg.AppEnv)

	if len(args) == 0 {
		return runServe(ctx, cfg, logger)
	}

	switch args[0] {
	case "serve":
		return runServe(ctx, cfg, logger)
	case "recover-owner":
		return runRecoverOwner(ctx, cfg, args[1:], stdin, stdout, stderr)
	case "verify-backup":
		return runVerifyBackup(ctx, cfg, args[1:], stdout, stderr)
	case "restore":
		return runRestore(ctx, cfg, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		return 2
	}
}

func runServe(ctx context.Context, cfg config.Config, logger *slog.Logger) (exitCode int) {
	if cfg.GeneratedSetupToken {
		logger.Warn("generated one-time setup token; set SETUP_TOKEN before the next restart if setup is not completed", slog.String("setup_token", cfg.SetupToken))
	}

	runtime, err := appruntime.Open(ctx, cfg, logger)
	if err != nil {
		logger.Error("start application runtime", slog.Any("err", err))
		return 1
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := runtime.Close(shutdownCtx); err != nil {
			logger.Error("close application runtime", slog.Any("err", err))
			exitCode = 1
		}
	}()

	server := newHTTPServer(cfg.HTTPAddr, runtime.Handler())
	logger.Info("server starting", slog.String("addr", cfg.HTTPAddr), slog.String("app_env", cfg.AppEnv))
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("serve http", slog.Any("err", err))
			return 1
		}
	case <-ctx.Done():
		logger.Info("server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown http server", slog.Any("err", err))
			return 1
		}
		if err := <-errCh; err != nil && err != http.ErrServerClosed {
			logger.Error("serve http", slog.Any("err", err))
			return 1
		}
	}
	return 0
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
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

	recoveryService := app.NewRecoveryService(database, cfg.DatabaseURL, db.NewRecoveryRepository(database))
	resolvedBackupPath, err := recoveryService.PrepareBackup(ctx, app.RecoverOwnerInput{
		BackupPath:    backupPath,
		AllowNoBackup: allowNoBackup,
	})
	if err != nil {
		fmt.Fprintf(stderr, "recover owner: %v\n", err)
		return 1
	}

	if err := db.Migrate(ctx, database); err != nil {
		fmt.Fprintf(stderr, "run migrations: %v\n", err)
		return 1
	}
	if err := db.EnforceSQLiteFilePermissions(cfg.DatabaseURL); err != nil {
		fmt.Fprintf(stderr, "secure sqlite files: %v\n", err)
		return 1
	}

	if err := recoveryService.ResetOwnerAccess(ctx, password); err != nil {
		fmt.Fprintf(stderr, "recover owner: %v\n", err)
		return 1
	}

	if resolvedBackupPath != "" {
		fmt.Fprintf(stdout, "owner recovery completed; verified backup written to %s\n", resolvedBackupPath)
		return 0
	}

	fmt.Fprintln(stdout, "owner recovery completed without backup")
	return 0
}

func readPasswordFromStdin(stdin io.Reader) (string, error) {
	var passwordBytes []byte
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(os.Stderr, "New owner password: ")
		var err error
		passwordBytes, err = term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
	} else {
		var err error
		passwordBytes, err = io.ReadAll(io.LimitReader(stdin, 1<<20))
		if err != nil {
			return "", err
		}
	}

	password := strings.TrimRight(string(passwordBytes), "\r\n")
	if password == "" {
		return "", fmt.Errorf("password is required")
	}

	return password, nil
}
