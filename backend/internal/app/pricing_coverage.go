package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"rekenraam/backend/internal/db"
)

// runFXCoverage visits each missing pair/date at most once in a sweep. The
// durable cursor is traversal progress, not proof that a publication exists.
// Independent retry items preserve failed dates without wedging later history.
func (s *PricingService) runFXCoverage(ctx context.Context, ownerUserID int64, payload fxCoverageWorkPayload) (PricingRefreshRun, []db.BackgroundWorkSuccessor, error) {
	startedAt := s.now().UTC()
	ctx, referenceErr := s.settings.referenceContext(ctx, startedAt)
	if referenceErr != nil {
		return PricingRefreshRun{}, nil, referenceErr
	}

	policy, err := s.pricingPolicyOrDefault(ctx)
	if err != nil {
		return PricingRefreshRun{}, nil, err
	}
	runSourceID, err := s.refreshRunSourceID(ctx, policy, 0)
	if err != nil {
		return PricingRefreshRun{}, nil, err
	}
	runRecord, err := s.repository.RecordRefreshRun(ctx, BookID, runSourceID, backgroundRunTrigger(payload.Reason), "running", startedAt.Format(time.RFC3339), "", 0, 0, 0, "")
	if err != nil {
		return PricingRefreshRun{}, nil, fmt.Errorf("start FX coverage run: %w", err)
	}
	fail := func(err error) (PricingRefreshRun, []db.BackgroundWorkSuccessor, error) {
		run, finishErr := s.finishFXRefreshRun(ctx, runRecord.ID, fxRefreshCounters{LastError: err.Error()})
		if finishErr != nil {
			return PricingRefreshRun{}, nil, finishErr
		}
		return run, nil, err
	}
	demand, err := s.fxDemand(ctx)
	if err != nil {
		return fail(err)
	}
	targets, err := s.fxRefreshTargets(ctx, policy, demand)
	if err != nil {
		return fail(err)
	}
	// A persisted cursor must not depend on Go map iteration order.
	slices.SortFunc(targets, func(a, b fxRefreshTarget) int {
		if a.Base.ID < b.Base.ID || (a.Base.ID == b.Base.ID && a.Quote.ID < b.Quote.ID) {
			return -1
		}
		if a.Base.ID == b.Base.ID && a.Quote.ID == b.Quote.ID {
			return 0
		}
		return 1
	})
	defaultCurrency, err := s.repository.DefaultCurrency(ctx, BookID)
	if err != nil {
		return fail(err)
	}
	baseCurrencyID := defaultCurrency.ID
	if policy.BaseCommodityID != nil {
		baseCurrencyID = *policy.BaseCommodityID
	}
	location, err := s.bookLocation(ctx)
	if err != nil {
		return fail(err)
	}
	today := startedAt.In(location).Format(time.DateOnly)
	through := today
	if payload.ThroughDate != "" && payload.ThroughDate < through {
		through = payload.ThroughDate
	}
	remaining := policy.MaxBackfillDays
	if remaining <= 0 {
		remaining = 370
	}
	counters := fxRefreshCounters{}
	// Collect waits before traversal: exhausting another currency's download
	// budget must not discard an independent future obligation.
	successors, err := futureFXCoverageSuccessors(demand, baseCurrencyID, payload, today, location)
	if err != nil {
		return fail(err)
	}
	var retryErr error
scan:
	for _, target := range targets {
		if payload.CurrencyID > 0 && target.Base.ID != payload.CurrencyID && target.Quote.ID != payload.CurrencyID {
			continue
		}
		if payload.Next != nil && (target.Base.ID < payload.Next.BaseID ||
			(target.Base.ID == payload.Next.BaseID && target.Quote.ID < payload.Next.QuoteID)) {
			continue
		}
		start := target.DemandStartDate
		if payload.Retry == nil && (start > today || (payload.Reason == "deferred" && start == "")) {
			continue
		}
		// A deferred date is a wakeup hint, not historical demand. Re-read the
		// current posting/account state so cancellation or edits cannot revive it.
		if payload.Reason != "deferred" && payload.StartDate != "" && (start == "" || payload.StartDate < start) {
			start = payload.StartDate
		}
		if payload.Next != nil && target.Base.ID == payload.Next.BaseID && target.Quote.ID == payload.Next.QuoteID && start < payload.Next.Date {
			start = payload.Next.Date
		}
		end := through
		if payload.Retry != nil {
			if target.Base.ID != payload.Retry.BaseID || target.Quote.ID != payload.Retry.QuoteID {
				continue
			}
			if payload.Retry.Date > today {
				return fail(ValidationError{Message: "FX retry date is in the future"})
			}
			start, end = payload.Retry.Date, payload.Retry.Date
		}
		if start == "" || start > end {
			continue
		}
		for date := start; date <= end; date = nextDate(date) {
			if policy.WeekendPolicy == "skip" && isWeekend(date) {
				continue
			}
			exists, err := s.repository.PriceObservationExistsOnDate(ctx, BookID, target.Base.ID, target.Quote.ID, date)
			if err != nil {
				return fail(err)
			}
			if exists {
				continue
			}
			if remaining == 0 {
				continuation := payload
				continuation.Reason, continuation.ThroughDate = "continuation", through
				continuation.Next = &fxCoverageCursor{BaseID: target.Base.ID, QuoteID: target.Quote.ID, Date: date}
				encoded, err := json.Marshal(continuation)
				if err != nil {
					return fail(err)
				}
				successors = append(successors, db.BackgroundWorkSuccessor{PayloadJSON: string(encoded), AvailableAt: startedAt.Format(time.RFC3339)})
				break scan
			}
			remaining-- // Failures and earlier-date responses also consume the budget.
			counters.Total++
			outcome, refreshErr := s.refreshFXTargetWithDemand(ctx, ownerUserID, target, policy, 0, date, startedAt, demand)
			if ctx.Err() != nil {
				return fail(ctx.Err())
			}
			status, message := "succeeded", ""
			if refreshErr != nil {
				refreshErr = fmt.Errorf("FX coverage %s/%s on %s: %w", target.Base.Code, target.Quote.Code, date, refreshErr)
				counters.Failed++
				counters.LastError, message, status = refreshErr.Error(), refreshErr.Error(), "failed"
				if payload.Retry != nil {
					retryErr = refreshErr
				} else {
					encoded, err := json.Marshal(fxCoverageWorkPayload{Reason: "retry", Retry: &fxCoverageCursor{BaseID: target.Base.ID, QuoteID: target.Quote.ID, Date: date}})
					if err != nil {
						return fail(err)
					}
					// Count this failed download as attempt 1. Claims begin at attempt 2,
					// so the existing eight-attempt cap includes the scan's first failure.
					successors = append(successors, db.BackgroundWorkSuccessor{PayloadJSON: string(encoded), AvailableAt: startedAt.Add(retryDelay(1, runRecord.ID)).Format(time.RFC3339), Attempts: 1})
				}
			} else {
				counters.Succeeded++
				if outcome.Skipped || outcome.Observation.ValuationDate != date {
					status = "skipped"
				}
			}
			raw, err := json.Marshal(map[string]any{"base_commodity_id": target.Base.ID, "quote_commodity_id": target.Quote.ID,
				"requested_date": date, "observed_date": outcome.Observation.ValuationDate})
			if err != nil {
				return fail(err)
			}
			if err := s.repository.RecordRefreshItem(ctx, db.PricingRefreshItemSpec{RunID: runRecord.ID, BookID: BookID, ItemKind: "price", Status: status,
				ProviderItemID: outcome.Observation.ProviderObservationID, LocalRefTable: "price_observations", LocalRefID: outcome.Observation.ID,
				Error: message, RawJSON: string(raw), CreatedAt: startedAt.Format(time.RFC3339)}); err != nil {
				return fail(err)
			}
			sourceID := outcome.SourceID
			state := db.PricingRefreshStateSpec{BookID: BookID, BaseCommodityID: target.Base.ID, QuoteCommodityID: target.Quote.ID,
				LastAttemptAt: startedAt.Format(time.RFC3339), UpdatedAt: startedAt.Format(time.RFC3339), LastError: message}
			if refreshErr == nil {
				state.LastSuccessDate = sql.NullString{String: outcome.Observation.ValuationDate, Valid: true}
			} else {
				sourceID = runSourceID
			}
			state.SourceID = sourceID
			if err := s.repository.UpsertRefreshState(ctx, state); err != nil {
				return fail(err)
			}
		}
	}
	run, err := s.finishFXRefreshRun(ctx, runRecord.ID, counters)
	if err != nil {
		return PricingRefreshRun{}, nil, err
	}
	// A sweep may finish with partial/failed history: its failed downloads now
	// have durable retry successors. A targeted retry retains the same identity
	// and returns its failure to the existing bounded retry handler.
	return run, successors, retryErr
}

func futureFXCoverageSuccessors(demand []db.PricingCurrencyDemandRecord, baseCurrencyID int64, payload fxCoverageWorkPayload, today string, location *time.Location) ([]db.BackgroundWorkSuccessor, error) {
	var successors []db.BackgroundWorkSuccessor
	if payload.Retry != nil {
		return successors, nil
	}
	for _, record := range demand {
		if record.CommodityID == baseCurrencyID || record.StartDate <= today ||
			(payload.CurrencyID > 0 && payload.CurrencyID != baseCurrencyID && payload.CurrencyID != record.CommodityID) {
			continue
		}
		date, err := time.Parse(time.DateOnly, record.StartDate)
		if err != nil {
			return nil, fmt.Errorf("parse future FX demand date: %w", err)
		}
		wake := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, location)
		// Some zones advance across midnight. Go can normalize that missing
		// midnight into the previous day; wait for the transition instead of
		// repeatedly claiming an already-due item before its financial date.
		if wake.In(location).Format(time.DateOnly) < record.StartDate {
			_, wake = wake.ZoneBounds()
		}
		// Include the actual date so the posting outbox will not coalesce an
		// earlier, newly current obligation into this later pending wakeup.
		encoded, err := json.Marshal(fxCoverageWorkPayload{Reason: "deferred", StartDate: record.StartDate, CurrencyID: record.CommodityID})
		if err != nil {
			return nil, err
		}
		successors = append(successors, db.BackgroundWorkSuccessor{PayloadJSON: string(encoded), AvailableAt: wake.UTC().Format(time.RFC3339)})
	}
	return successors, nil
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
