// Package appruntime owns the web application's databases, services, workers,
// and HTTP handler. The server command owns only the listener and OS signals.
package appruntime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"rekenraam/backend/internal/api"
	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/config"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/lockfile"
	"rekenraam/backend/internal/marketdata"
	"rekenraam/backend/internal/web"
)

// Runtime owns every resource needed to serve the app, except the TCP listener.
// Close is safe to call more than once, including after a canceled Close call.
type Runtime struct {
	handler  http.Handler
	lock     *lockfile.Lock
	database *sql.DB
	readOnly *sql.DB
	cancel   context.CancelFunc
	workers  []<-chan struct{}

	closeOnce sync.Once
	closed    chan struct{}
	closeErr  error
}

type openDependencies struct {
	openDatabase   func(context.Context, string) (*sql.DB, error)
	migrate        func(context.Context, *sql.DB) error
	secureDatabase func(string) error
	openReadOnly   func(context.Context, string) (*sql.DB, error)
}

// Open locks, migrates, constructs, and starts one application runtime. Failed
// startup releases every resource acquired before the failure.
func Open(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Runtime, error) {
	return openWith(ctx, cfg, logger, openDependencies{
		openDatabase: db.Open, migrate: db.Migrate,
		secureDatabase: db.EnforceSQLiteFilePermissions,
		openReadOnly:   db.OpenReadOnly,
	})
}

func openWith(ctx context.Context, cfg config.Config, logger *slog.Logger, deps openDependencies) (_ *Runtime, err error) {
	if logger == nil {
		logger = slog.Default()
	}
	// config.Load already refuses this; a hand-built Config must not route a
	// real Trading 212 API key to a stub host either (T-128).
	if cfg.Trading212BaseURL != "" && cfg.AppEnv != "development" {
		return nil, errors.New("a Trading 212 base URL override is only allowed in development")
	}
	r := &Runtime{closed: make(chan struct{})}
	defer func() {
		if err != nil {
			err = errors.Join(err, r.closeResources())
		}
	}()
	databasePath, err := db.ResolveSQLiteFilePath(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	r.lock, err = lockfile.Acquire(databasePath)
	if err != nil {
		return nil, fmt.Errorf("acquire database lock: %w", err)
	}
	r.database, err = deps.openDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := deps.migrate(ctx, r.database); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	if err := deps.secureDatabase(cfg.DatabaseURL); err != nil {
		return nil, fmt.Errorf("secure sqlite files: %w", err)
	}
	r.readOnly, err = deps.openReadOnly(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open read-only database: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("finish application startup: %w", err)
	}

	database := r.database
	readOnlyDatabase := r.readOnly
	setupRepository := db.NewSetupRepository(database)
	setupService := app.NewSetupService(setupRepository)
	authRepository := db.NewAuthRepository(database)
	authService := app.NewAuthServiceWithSessionLifetime(authRepository, logger, cfg.SessionLifetime)
	// The same key seals connection credentials and MFA shared secrets.
	authService.SetSecretKey(cfg.SecretKey)
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
	transactionService.SetPricingRepository(db.NewPricingRepository(database))
	investmentService := app.NewInvestmentService(db.NewInvestmentRepository(database), accountService, transactionService, pricingService)
	importConnectionService := app.NewImportConnectionService(db.NewImportConnectionRepository(database), accountService, cfg.SecretKey, app.NewTrading212Prober(nil, cfg.Trading212BaseURL))
	importService := app.NewImportService(db.NewImportRepository(database), transactionService, accountRepository, importConnectionService, db.NewBackgroundWorkRepository(database), investmentService)
	importService.SetTrading212BaseURL(cfg.Trading212BaseURL)
	exportService := app.NewExportService(db.NewExportRepository(readOnlyDatabase))
	forecastService := app.NewForecastService(db.NewForecastRepository(readOnlyDatabase))
	budgetService := app.NewBudgetService(db.NewBudgetRepository(database), settingsService)
	backupService := app.NewBackupService(db.NewBackupRepository(database), db.NewBackgroundWorkRepository(database), readOnlyDatabase, cfg.DatabaseURL, cfg.BackupDir)
	selfCheckService := app.NewSelfCheckService(db.NewSelfCheckRepository(database, readOnlyDatabase))
	// A running self-check from a previous process cannot still be active.
	if recovered, recoverErr := selfCheckService.RecoverInterruptedRuns(ctx); recoverErr != nil {
		logger.Error("recover interrupted self-check runs", slog.Any("err", recoverErr))
	} else if recovered > 0 {
		logger.Warn("recovered self-check run interrupted by a previous crash", slog.Int64("count", recovered))
	}
	backupService.SetSelfCheck(selfCheckService)
	recurringService := app.NewRecurringService(db.NewRecurringRepository(database), transactionService, settingsService)
	r.handler = api.NewHandler(logger, web.Handler(), api.Services{
		Setup: setupService, Auth: authService, Settings: settingsService,
		Book: bookService, Currency: currencyService, Institution: institutionService,
		Account: accountService, Tag: tagService, Category: categoryService,
		Payee: payeeService, Transaction: transactionService, Recurring: recurringService,
		Pricing: pricingService, Investment: investmentService, Import: importService,
		ImportConnection: importConnectionService, Export: exportService,
		Backup: backupService, SelfCheck: selfCheckService, Forecast: forecastService,
		Budget: budgetService,
	}, api.HandlerOptions{
		TrustProxyHeaders: cfg.TrustProxyHeaders,
		TrustedProxyCIDRs: cfg.TrustedProxyCIDRs,
		SetupToken:        cfg.SetupToken,
	})

	workerCtx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.workers = []<-chan struct{}{
		pricingService.StartScheduler(workerCtx, logger),
		authService.StartSessionCleanup(workerCtx, logger),
		pricingService.StartBackgroundWorker(workerCtx, logger),
		importService.StartBackgroundWorker(workerCtx, logger),
		importService.StartScheduler(workerCtx, logger),
		backupService.StartBackgroundWorker(workerCtx, logger),
		backupService.StartScheduler(workerCtx, logger),
		recurringService.StartScheduler(workerCtx, logger),
	}
	return r, nil
}

// Handler is the same API and static-file handler served by the Go binary.
func (r *Runtime) Handler() http.Handler { return r.handler }

// Close cancels and joins workers before releasing the databases and process
// lock. If ctx expires, cleanup continues; a later Close call can await it.
func (r *Runtime) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		if r.cancel != nil {
			r.cancel()
		}
		go func() {
			for _, worker := range r.workers {
				<-worker
			}
			r.closeErr = r.closeResources()
			close(r.closed)
		}()
	})
	select {
	case <-r.closed:
		return r.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runtime) closeResources() error {
	var errs []error
	if r.readOnly != nil {
		errs = append(errs, r.readOnly.Close())
		r.readOnly = nil
	}
	if r.database != nil {
		errs = append(errs, r.database.Close())
		r.database = nil
	}
	if r.lock != nil {
		errs = append(errs, r.lock.Close())
		r.lock = nil
	}
	return errors.Join(errs...)
}
