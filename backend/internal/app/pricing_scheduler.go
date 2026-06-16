package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"rekenraam/backend/internal/db"
)

func (s *PricingService) StartScheduler(ctx context.Context, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	go func() {
		s.runScheduledRefreshIfDue(ctx, logger)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runScheduledRefreshIfDue(ctx, logger)
			}
		}
	}()
}

func (s *PricingService) runScheduledRefreshIfDue(ctx context.Context, logger *slog.Logger) {
	now := s.now().UTC()
	policy, err := s.pricingPolicyOrDefault(ctx)
	if err != nil {
		logger.WarnContext(ctx, "read pricing refresh policy", slog.Any("err", err))
		return
	}
	if !policy.RefreshEnabled {
		return
	}

	timeZone, err := s.repository.BookOwnerTimeZone(ctx, BookID)
	if err != nil {
		logger.WarnContext(ctx, "read pricing refresh time zone", slog.Any("err", err))
		return
	}
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		logger.WarnContext(ctx, "load pricing refresh time zone", slog.String("time_zone", timeZone), slog.Any("err", err))
		return
	}
	localNow := now.In(location)
	localScheduledAt := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), policy.RefreshHourLocal, policy.RefreshMinuteLocal, 0, 0, location)
	if localNow.Before(localScheduledAt) {
		return
	}

	localDayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location).UTC()
	localDayEnd := localDayStart.In(location).AddDate(0, 0, 1).UTC()
	exists, err := s.repository.ScheduledRefreshRunExistsBetween(ctx, BookID, localDayStart.Format(time.RFC3339), localDayEnd.Format(time.RFC3339))
	if err != nil {
		logger.WarnContext(ctx, "read scheduled pricing refresh state", slog.Any("err", err))
		return
	}
	if exists {
		return
	}

	ownerID, err := s.repository.CurrentBookOwnerID(ctx, BookID)
	if err != nil {
		if !errors.Is(err, db.ErrNotFound) {
			logger.WarnContext(ctx, "read owner for scheduled pricing refresh", slog.Any("err", err))
		}
		return
	}

	run, err := s.RunRefresh(ctx, ownerID, 0, "scheduled")
	if err != nil {
		logger.WarnContext(ctx, "run scheduled pricing refresh", slog.Any("err", err))
		return
	}
	logger.InfoContext(ctx, "scheduled pricing refresh complete",
		slog.Int64("run_id", run.ID),
		slog.String("status", run.Status),
		slog.Int("items_total", run.ItemsTotal),
		slog.Int("items_failed", run.ItemsFailed),
	)
}
