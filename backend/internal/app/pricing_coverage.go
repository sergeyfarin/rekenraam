package app

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"rekenraam/backend/internal/db"
)

func (s *PricingService) runFXCoverage(ctx context.Context, ownerUserID int64, requestedCurrencyID int64, requestedStart string, trigger string) (PricingRefreshRun, bool, error) {
	startedAt := s.now().UTC()
	policy, err := s.pricingPolicyOrDefault(ctx)
	if err != nil {
		return PricingRefreshRun{}, false, err
	}
	runSourceID, err := s.refreshRunSourceID(ctx, policy, 0)
	if err != nil {
		return PricingRefreshRun{}, false, err
	}
	runRecord, err := s.repository.RecordRefreshRun(ctx, BookID, runSourceID, trigger, "running", startedAt.Format(time.RFC3339), "", 0, 0, 0, "")
	if err != nil {
		return PricingRefreshRun{}, false, fmt.Errorf("start FX coverage run: %w", err)
	}

	coverage, err := s.repository.FXCoverageStartDates(ctx, BookID)
	if err != nil {
		return s.failFXCoverageRun(ctx, runRecord.ID, err)
	}
	starts := make(map[int64]string, len(coverage))
	for _, record := range coverage {
		starts[record.CommodityID] = record.StartDate
	}
	targets, err := s.fxRefreshTargets(ctx, policy)
	if err != nil {
		return s.failFXCoverageRun(ctx, runRecord.ID, err)
	}
	defaultCurrency, err := s.repository.DefaultCurrency(ctx, BookID)
	if err != nil {
		return s.failFXCoverageRun(ctx, runRecord.ID, err)
	}
	baseCurrencyID := defaultCurrency.ID
	if policy.BaseCommodityID != nil {
		baseCurrencyID = *policy.BaseCommodityID
	}
	location, err := s.bookLocation(ctx)
	if err != nil {
		return s.failFXCoverageRun(ctx, runRecord.ID, err)
	}
	today := startedAt.In(location).Format(time.DateOnly)
	remaining := policy.MaxBackfillDays
	if remaining <= 0 {
		remaining = 370
	}
	counters := fxRefreshCounters{}
	hasMore := false

	for _, target := range targets {
		if requestedCurrencyID > 0 && target.Base.ID != requestedCurrencyID && target.Quote.ID != requestedCurrencyID {
			continue
		}
		coverageCurrencyID := target.Base.ID
		if coverageCurrencyID == baseCurrencyID {
			coverageCurrencyID = target.Quote.ID
		}
		start := starts[coverageCurrencyID]
		if requestedStart != "" && (start == "" || requestedStart < start) {
			start = requestedStart
		}
		if start == "" || start > today {
			continue
		}
		consecutiveFailures := 0
		for date := start; date <= today; date = nextDate(date) {
			if policy.WeekendPolicy == "skip" && isWeekend(date) {
				continue
			}
			exists, err := s.repository.PriceObservationExistsOnDate(ctx, BookID, target.Base.ID, target.Quote.ID, date)
			if err != nil {
				counters.LastError = err.Error()
				counters.Failed++
				break
			}
			if exists {
				continue
			}
			if remaining == 0 {
				hasMore = true
				break
			}
			counters.Total++
			outcome, refreshErr := s.refreshFXTarget(ctx, ownerUserID, target, policy, 0, date, startedAt)
			if refreshErr != nil {
				consecutiveFailures++
				counters.Failed++
				counters.LastError = refreshErr.Error()
				_ = s.recordFXRefreshItem(ctx, runRecord.ID, target, "failed", 0, "", refreshErr.Error(), startedAt)
				if consecutiveFailures >= 3 {
					hasMore = true
					break
				}
				continue
			}
			consecutiveFailures = 0
			remaining--
			counters.Succeeded++
			itemStatus := "succeeded"
			if outcome.Skipped || outcome.Observation.ValuationDate != date {
				itemStatus = "skipped"
			}
			_ = s.recordFXRefreshItem(ctx, runRecord.ID, target, itemStatus, outcome.Observation.ID, outcome.Observation.ProviderObservationID, "", startedAt)
			_ = s.repository.UpsertRefreshState(ctx, db.PricingRefreshStateSpec{
				BookID: BookID, BaseCommodityID: target.Base.ID, QuoteCommodityID: target.Quote.ID,
				SourceID: outcome.SourceID, LastSuccessDate: sql.NullString{String: outcome.Observation.ValuationDate, Valid: outcome.Observation.ValuationDate != ""},
				LastAttemptAt: startedAt.Format(time.RFC3339), UpdatedAt: startedAt.Format(time.RFC3339),
			})
		}
	}

	run, err := s.finishFXRefreshRun(ctx, runRecord.ID, counters)
	if err != nil {
		return PricingRefreshRun{}, hasMore, err
	}
	if run.Status == "failed" || run.Status == "partial" {
		return run, hasMore, fmt.Errorf("FX coverage run %d %s: %s", run.ID, run.Status, run.LastError)
	}
	return run, hasMore, nil
}

func (s *PricingService) failFXCoverageRun(ctx context.Context, runID int64, cause error) (PricingRefreshRun, bool, error) {
	run, finishErr := s.finishFXRefreshRun(ctx, runID, fxRefreshCounters{LastError: cause.Error()})
	if finishErr != nil {
		return PricingRefreshRun{}, false, finishErr
	}
	return run, false, cause
}

func (s *PricingService) bookLocation(ctx context.Context) (*time.Location, error) {
	name, err := s.repository.BookOwnerTimeZone(ctx, BookID)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("load book time zone: %w", err)
	}
	return location, nil
}

func nextDate(value string) string {
	date, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return "9999-12-31"
	}
	return date.AddDate(0, 0, 1).Format(time.DateOnly)
}

func isWeekend(value string) bool {
	date, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return false
	}
	return date.Weekday() == time.Saturday || date.Weekday() == time.Sunday
}
