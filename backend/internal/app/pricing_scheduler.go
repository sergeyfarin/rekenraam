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

	today := now.Format(time.DateOnly)
	scheduledAt := time.Date(now.Year(), now.Month(), now.Day(), policy.RefreshHourUTC, policy.RefreshMinuteUTC, 0, 0, time.UTC)
	if now.Before(scheduledAt) {
		return
	}

	exists, err := s.repository.ScheduledRefreshRunExistsOnDate(ctx, BookID, today)
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
