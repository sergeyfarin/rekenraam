package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"

	"rekenraam/backend/internal/db"
)

const fxCoverageWorkKind = "pricing.fx_coverage"

// maxFXCoverageAttempts bounds retries before FX coverage work is given up on
// as terminal (T-39). Backoff caps at 6h, so 8 attempts is roughly a day of
// trying; past that the item is almost certainly failing for a reason a retry
// cannot fix (missing provider credentials, a delisted pair), and leaving it
// pending forever hides the problem behind a queue that looks busy. Failed
// items surface in pricing source health and can be re-enqueued manually.
const maxFXCoverageAttempts = 8

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
	if item.Attempts >= maxFXCoverageAttempts {
		if failErr := s.repository.FailBackgroundWork(ctx, item.ID, workerID, now.Format(time.RFC3339), err.Error()); failErr != nil {
			logger.WarnContext(ctx, "fail exhausted FX coverage work", slog.Int64("work_id", item.ID), slog.Any("err", failErr))
			return
		}
		logger.ErrorContext(ctx, "FX coverage gave up after max attempts", slog.Int64("work_id", item.ID), slog.Int("attempts", item.Attempts), slog.Any("err", err))
		return
	}
	delay := retryDelay(item.Attempts, item.ID)
	if retryErr := s.repository.RetryBackgroundWork(ctx, item.ID, workerID, now.Add(delay).Format(time.RFC3339), now.Format(time.RFC3339), err.Error()); retryErr != nil {
		logger.WarnContext(ctx, "schedule FX coverage retry", slog.Int64("work_id", item.ID), slog.Any("err", retryErr))
		return
	}
	logger.WarnContext(ctx, "FX coverage will retry", slog.Int64("work_id", item.ID), slog.Duration("retry_in", delay), slog.Any("err", err))
}

// PricingBackgroundWorkItem is a queued FX coverage work item as surfaced to
// the operator. Only failed items are listed today (see FailedBackgroundWork):
// healthy work needs no attention and would only add noise.
type PricingBackgroundWorkItem struct {
	ID        int64
	Kind      string
	Reason    string
	Attempts  int
	LastError string
	CreatedAt string
	UpdatedAt string
}

// ErrPricingBackgroundWorkNotFound is returned when a work item to re-enqueue
// does not exist, belongs to another book, or is not in the failed state.
var ErrPricingBackgroundWorkNotFound = errors.New("pricing background work not found")

// ErrPricingBackgroundWorkActive is returned when equivalent work is already
// queued, so re-enqueuing the failed item would duplicate it.
var ErrPricingBackgroundWorkActive = errors.New("equivalent pricing background work is already queued")

// FailedBackgroundWork lists FX coverage work that exhausted its retries.
// These are the items that used to retry forever (T-39) and now need a manual
// re-enqueue once the underlying cause is fixed.
func (s *PricingService) FailedBackgroundWork(ctx context.Context) ([]PricingBackgroundWorkItem, error) {
	records, err := s.repository.ListBackgroundWorkByStatus(ctx, BookID, fxCoverageWorkKind, "failed", 50)
	if err != nil {
		return nil, fmt.Errorf("list failed pricing background work: %w", err)
	}
	items := make([]PricingBackgroundWorkItem, 0, len(records))
	for _, record := range records {
		items = append(items, toPricingBackgroundWorkItem(record))
	}
	return items, nil
}

// RetryBackgroundWork re-enqueues a failed FX coverage item immediately, with
// its attempt counter reset so backoff starts from the bottom again.
func (s *PricingService) RetryBackgroundWork(ctx context.Context, workID int64) (PricingBackgroundWorkItem, error) {
	if workID <= 0 {
		return PricingBackgroundWorkItem{}, ValidationError{Message: "background work id is required"}
	}
	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.RequeueBackgroundWork(ctx, BookID, workID, now, now)
	if errors.Is(err, db.ErrNotFound) {
		return PricingBackgroundWorkItem{}, ErrPricingBackgroundWorkNotFound
	}
	if errors.Is(err, db.ErrBackgroundWorkAlreadyActive) {
		return PricingBackgroundWorkItem{}, ErrPricingBackgroundWorkActive
	}
	if err != nil {
		return PricingBackgroundWorkItem{}, fmt.Errorf("requeue pricing background work: %w", err)
	}
	return toPricingBackgroundWorkItem(record), nil
}

func toPricingBackgroundWorkItem(record db.BackgroundWorkItemRecord) PricingBackgroundWorkItem {
	var payload fxCoverageWorkPayload
	_ = json.Unmarshal([]byte(record.PayloadJSON), &payload)
	return PricingBackgroundWorkItem{
		ID:        record.ID,
		Kind:      record.Kind,
		Reason:    payload.Reason,
		Attempts:  record.Attempts,
		LastError: nullableString(record.LastError),
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
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
