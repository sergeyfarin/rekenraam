package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"rekenraam/backend/internal/db"
)

// trading212AutoRefreshInterval is how long a connection can go
// unattempted before the scheduler considers it due again. Unlike
// PricingService's scheduler (a fixed local wall-clock time), this is a
// rolling window since the last attempt — simpler (no book-owner-timezone
// plumbing needed) and better suited to a rate-limited third-party API: no
// thundering-herd at a fixed hour, and self-correcting if the server was
// down at the usual time. A var, not a const, so tests can shrink it.
var trading212AutoRefreshInterval = 24 * time.Hour

// SetAutoRefreshIntervalForTest overrides trading212AutoRefreshInterval for
// the duration of a test and returns a restore function.
func SetAutoRefreshIntervalForTest(d time.Duration) (restore func()) {
	old := trading212AutoRefreshInterval
	trading212AutoRefreshInterval = d
	return func() { trading212AutoRefreshInterval = old }
}

// StartScheduler starts the Trading 212 auto-refresh scheduler loop,
// mirroring PricingService.StartScheduler (pricing_scheduler.go): a
// once-a-minute ticker that checks which connections are due and refreshes
// them via the existing RefreshImportConnection path (Slice 3) — no new
// fetch logic, just deciding when to call it. No-ops if the service was
// constructed without online-import dependencies.
func (s *ImportService) StartScheduler(ctx context.Context, logger *slog.Logger) {
	if s.backgroundWork == nil || s.connectionService == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	go func() {
		s.runDueTrading212AutoRefreshes(ctx, logger)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runDueTrading212AutoRefreshes(ctx, logger)
			}
		}
	}()
}

func (s *ImportService) runDueTrading212AutoRefreshes(ctx context.Context, logger *slog.Logger) {
	cutoff := s.now().UTC().Add(-trading212AutoRefreshInterval)
	ids, err := s.connectionService.ListDueAutoRefreshConnectionIDs(ctx, "trading212", cutoff)
	if err != nil {
		logger.WarnContext(ctx, "list due trading212 auto-refresh connections", slog.Any("err", err))
		return
	}
	if len(ids) == 0 {
		return
	}

	ownerID, err := s.repository.CurrentBookOwnerID(ctx, BookID)
	if err != nil {
		if !errors.Is(err, db.ErrNotFound) {
			logger.WarnContext(ctx, "read owner for scheduled trading212 auto-refresh", slog.Any("err", err))
		}
		return
	}

	for _, connectionID := range ids {
		_, err := s.RefreshImportConnection(ctx, RefreshImportConnectionInput{
			OwnerUserID:  ownerID,
			ConnectionID: connectionID,
		})
		if err == nil {
			continue
		}
		if errors.Is(err, ErrImportFetchInProgress) {
			// Expected, not a failure: a manual refresh or a still-running
			// previous auto-refresh is already in flight for this
			// connection (e.g. a deep multi-continuation fetch that hasn't
			// caught up to now yet). The next tick tries again.
			continue
		}
		logger.WarnContext(ctx, "scheduled trading212 auto-refresh", slog.Int64("connection_id", connectionID), slog.Any("err", err))
	}
}
