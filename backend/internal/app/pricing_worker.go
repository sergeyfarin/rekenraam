package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"

	"rekenraam/backend/internal/db"
)

const fxCoverageWorkKind = "pricing.fx_coverage"

type fxCoverageWorkPayload struct {
	Reason     string `json:"reason"`
	StartDate  string `json:"start_date"`
	CurrencyID int64  `json:"currency_id,omitempty"`
}

func (s *PricingService) StartBackgroundWorker(ctx context.Context, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	workerID := uuid.NewString()
	go func() {
		s.runDueFXCoverage(ctx, logger, workerID)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runDueFXCoverage(ctx, logger, workerID)
			}
		}
	}()
}

func (s *PricingService) runDueFXCoverage(ctx context.Context, logger *slog.Logger, workerID string) {
	for processed := 0; processed < 4; processed++ {
		now := s.now().UTC()
		item, err := s.repository.ClaimBackgroundWork(ctx, fxCoverageWorkKind, workerID, now.Format(time.RFC3339), now.Add(15*time.Minute).Format(time.RFC3339))
		if errors.Is(err, db.ErrNotFound) || ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.WarnContext(ctx, "claim FX coverage work", slog.Any("err", err))
			return
		}
		s.processFXCoverageWork(ctx, logger, workerID, item)
	}
}

func (s *PricingService) processFXCoverageWork(ctx context.Context, logger *slog.Logger, workerID string, item db.BackgroundWorkItemRecord) {
	var payload fxCoverageWorkPayload
	if item.PayloadVersion != 1 || json.Unmarshal([]byte(item.PayloadJSON), &payload) != nil {
		_ = s.repository.FailBackgroundWork(ctx, item.ID, workerID, s.now().UTC().Format(time.RFC3339), "invalid FX coverage work payload")
		return
	}
	if payload.StartDate != "" {
		if _, err := time.Parse(time.DateOnly, payload.StartDate); err != nil {
			_ = s.repository.FailBackgroundWork(ctx, item.ID, workerID, s.now().UTC().Format(time.RFC3339), "invalid FX coverage start date")
			return
		}
	}
	var err error
	ownerID, err := s.repository.CurrentBookOwnerID(ctx, item.BookID)
	if err == nil {
		var hasMore bool
		_, hasMore, err = s.runFXCoverage(ctx, ownerID, payload.CurrencyID, payload.StartDate, backgroundRunTrigger(payload.Reason))
		if err == nil {
			now := s.now().UTC().Format(time.RFC3339)
			if hasMore {
				continuation, _ := json.Marshal(fxCoverageWorkPayload{Reason: "continuation", StartDate: payload.StartDate, CurrencyID: payload.CurrencyID})
				_, err = s.repository.EnqueueBackgroundWork(ctx, item.BookID, fxCoverageWorkKind, string(continuation), now)
			}
			if err == nil {
				err = s.repository.CompleteBackgroundWork(ctx, item.ID, workerID, now)
			}
		}
	}
	if err == nil || ctx.Err() != nil {
		return
	}
	now := s.now().UTC()
	delay := retryDelay(item.Attempts, item.ID)
	if retryErr := s.repository.RetryBackgroundWork(ctx, item.ID, workerID, now.Add(delay).Format(time.RFC3339), now.Format(time.RFC3339), err.Error()); retryErr != nil {
		logger.WarnContext(ctx, "schedule FX coverage retry", slog.Int64("work_id", item.ID), slog.Any("err", retryErr))
		return
	}
	logger.WarnContext(ctx, "FX coverage will retry", slog.Int64("work_id", item.ID), slog.Duration("retry_in", delay), slog.Any("err", err))
}

func backgroundRunTrigger(reason string) string {
	if reason == "manual" {
		return "manual"
	}
	if reason == "scheduled" {
		return "scheduled"
	}
	if reason == "continuation" {
		return "recovery"
	}
	return "domain"
}

func retryDelay(attempt int, workID int64) time.Duration {
	exponent := math.Min(float64(max(attempt-1, 0)), 8)
	delay := time.Minute * time.Duration(math.Pow(2, exponent))
	// Stable per-item jitter avoids a restart causing all due work to retry at once.
	delay = delay * time.Duration(80+(workID%41)) / 100
	if delay > 6*time.Hour {
		return 6 * time.Hour
	}
	return delay
}
