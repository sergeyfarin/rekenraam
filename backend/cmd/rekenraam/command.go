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

	"rekenraam/backend/internal/api"
	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/config"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/marketdata"
	"rekenraam/backend/internal/web"
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
	authService := app.NewAuthServiceWithSessionLifetime(authRepository, logger, cfg.SessionLifetime)
	settingsService := app.NewSettingsService(db.NewSettingsRepository(database))
	bookRepository := db.NewBookRepository(database)
	bookService := app.NewBookService(bookRepository, setupService)
	commodityRepository := db.NewCommodityRepository(database)
	currencyService := app.NewCurrencyService(commodityRepository, setupService)
	institutionRepository := db.NewInstitutionRepository(database)
	institutionService := app.NewInstitutionService(institutionRepository)
	accountRepository := db.NewAccountRepository(database)
	accountService := app.NewAccountService(accountRepository, institutionRepository, setupService)
	tagService := app.NewTagService(db.NewTagRepository(database))
	categoryService := app.NewCategoryService(db.NewCategoryRepository(database), setupService)
	payeeRepository := db.NewPayeeRepository(database)
	payeeService := app.NewPayeeService(payeeRepository, accountRepository)
	transactionService := app.NewTransactionService(db.NewTransactionRepository(database), payeeRepository, accountRepository, commodityRepository)
	pricingService := app.NewPricingService(db.NewPricingRepository(database), marketdata.DefaultRegistry(cfg.OpenExchangeRatesAppID))
	investmentService := app.NewInvestmentService(db.NewInvestmentRepository(database), accountService, transactionService, pricingService)
	importConnectionService := app.NewImportConnectionService(db.NewImportConnectionRepository(database), accountService, cfg.SecretKey, app.NewTrading212Prober(nil))
	importService := app.NewImportService(db.NewImportRepository(database), transactionService, accountRepository, importConnectionService, db.NewBackgroundWorkRepository(database), investmentService)
	pricingService.StartScheduler(ctx, logger)
	authService.StartSessionCleanup(ctx, logger)
	pricingService.StartBackgroundWorker(ctx, logger)
	importService.StartBackgroundWorker(ctx, logger)
	importService.StartScheduler(ctx, logger)
	handler := api.NewHandler(logger, web.Handler(), api.Services{
		Setup:            setupService,
		Auth:             authService,
		Settings:         settingsService,
		Book:             bookService,
		Currency:         currencyService,
		Institution:      institutionService,
		Account:          accountService,
		Tag:              tagService,
		Category:         categoryService,
		Payee:            payeeService,
		Transaction:      transactionService,
		Pricing:          pricingService,
		Investment:       investmentService,
		Import:           importService,
		ImportConnection: importConnectionService,
	}, api.HandlerOptions{
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

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
