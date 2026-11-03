package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/marketdata"
)

const quoteRefreshWorkKind = "pricing.quote_refresh"

type quoteRefreshPayload struct {
	Reason       string `json:"reason"`
	AssignmentID int64  `json:"assignment_id"`
}

func (s *PricingService) currentQuoteTargets(ctx context.Context, sourceID int64) ([]db.QuoteRefreshTarget, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return nil, referenceErr
	}

	zone, err := s.repository.BookOwnerTimeZone(ctx, BookID)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, err
	}
	return s.repository.QuoteRefreshTargets(ctx, BookID, s.now().In(location).Format(time.DateOnly), sourceID)
}

func (s *PricingService) enqueueQuoteRefresh(ctx context.Context, sourceID int64, reason string) (int, error) {
	targets, err := s.currentQuoteTargets(ctx, sourceID)
	if err != nil {
		return 0, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	for _, target := range targets {
		payload, _ := json.Marshal(quoteRefreshPayload{Reason: reason, AssignmentID: target.AssignmentID})
		if _, err := s.repository.EnqueueBackgroundWork(ctx, BookID, quoteRefreshWorkKind, string(payload), now); err != nil {
			return 0, err
		}
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return len(targets), nil
}

func (s *PricingService) runDueQuoteRefresh(ctx context.Context, logger *slog.Logger, workerID string) {
	for range 4 {
		now := s.now().UTC()
		item, err := s.repository.ClaimBackgroundWork(ctx, quoteRefreshWorkKind, workerID, now.Format(time.RFC3339), now.Add(15*time.Minute).Format(time.RFC3339))
		if errors.Is(err, db.ErrNotFound) || ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.WarnContext(ctx, "claim quote refresh work", slog.Any("err", err))
			return
		}
		s.processQuoteRefresh(ctx, logger, workerID, item)
	}
}

func (s *PricingService) processQuoteRefresh(ctx context.Context, logger *slog.Logger, workerID string, item db.BackgroundWorkItemRecord) {
	var payload quoteRefreshPayload
	var workErr error
	if item.PayloadVersion != 1 || json.Unmarshal([]byte(item.PayloadJSON), &payload) != nil || payload.AssignmentID <= 0 {
		workErr = &marketdata.QuoteError{Status: 400, Terminal: true}
	} else {
		targets, err := s.currentQuoteTargets(ctx, 0)
		workErr = err
		if err == nil {
			for _, target := range targets {
				if target.AssignmentID == payload.AssignmentID {
					workErr = s.refreshQuoteTarget(ctx, target, backgroundRunTrigger(payload.Reason))
					break
				}
			}
		}
	}
	now := s.now().UTC()
	var stateErr error
	if workErr == nil {
		stateErr = s.repository.CompleteBackgroundWork(ctx, item.ID, workerID, now.Format(time.RFC3339))
	} else if terminalQuoteError(workErr) || item.Attempts >= maxFXCoverageAttempts {
		stateErr = s.repository.FailBackgroundWork(ctx, item.ID, workerID, now.Format(time.RFC3339), workErr.Error())
	} else {
		delay := retryDelay(item.Attempts, item.ID)
		var providerErr *marketdata.QuoteError
		if errors.As(workErr, &providerErr) {
			delay = max(delay, providerErr.RetryAfter)
		}
		stateErr = s.repository.RetryBackgroundWork(ctx, item.ID, workerID, now.Add(delay).Format(time.RFC3339), now.Format(time.RFC3339), workErr.Error())
	}
	if stateErr != nil && ctx.Err() == nil {
		logger.WarnContext(ctx, "record quote work outcome", slog.Int64("work_id", item.ID), slog.Any("err", stateErr))
	}
}

func terminalQuoteError(err error) bool {
	var providerErr *marketdata.QuoteError
	return errors.Is(err, marketdata.ErrMissingSecret) || errors.Is(err, marketdata.ErrUnsupportedPair) || errors.As(err, &providerErr) && providerErr.Terminal
}

func (s *PricingService) refreshQuoteTarget(ctx context.Context, target db.QuoteRefreshTarget, trigger string) error {
	now := s.now().UTC()
	run, err := s.repository.RecordRefreshRun(ctx, BookID, target.SourceID, trigger, "running", now.Format(time.RFC3339), "", 0, 0, 0, "")
	if err != nil {
		return err
	}
	observation, skipped, fetchErr := s.fetchAndStoreQuote(ctx, target, now)
	status, itemStatus, lastError := "succeeded", "succeeded", ""
	counters := fxRefreshCounters{Total: 1, Succeeded: 1}
	if skipped {
		itemStatus = "skipped"
	}
	if fetchErr != nil {
		status = "failed"
		itemStatus = "failed"
		lastError = fetchErr.Error()
		counters.Succeeded = 0
		counters.Failed = 1
		counters.LastError = lastError
	}
	pair := fxRefreshTarget{Base: db.PricingCurrencyRecord{ID: target.BaseCommodityID, Code: target.BaseCode}, Quote: db.PricingCurrencyRecord{ID: target.QuoteCommodityID, Code: target.QuoteCode}}
	state := db.PricingRefreshStateSpec{BookID: BookID, BaseCommodityID: target.BaseCommodityID, QuoteCommodityID: target.QuoteCommodityID, SourceID: target.SourceID, LastAttemptAt: now.Format(time.RFC3339), LastError: lastError, UpdatedAt: now.Format(time.RFC3339)}
	if fetchErr == nil {
		state.LastSuccessDate = sql.NullString{String: observation.ValuationDate, Valid: true}
	}
	var persistErr error
	if err := s.recordFXRefreshItem(ctx, run.ID, pair, itemStatus, observation.ID, observation.ProviderObservationID, lastError, now); err != nil {
		persistErr = fmt.Errorf("record quote refresh item for %s/%s: %w", target.BaseCode, target.QuoteCode, err)
	} else if err := s.repository.UpsertRefreshState(ctx, state); err != nil {
		persistErr = fmt.Errorf("record quote refresh state for %s/%s: %w", target.BaseCode, target.QuoteCode, err)
	}
	if persistErr != nil {
		// A stored observation is durable evidence even when its item or
		// health row cannot be written. Finish the run as failed so it never
		// stays running, and keep the fetch error in the result so terminal
		// and Retry-After handling are unchanged (#474).
		_, finishErr := s.repository.FinishRefreshRun(ctx, BookID, run.ID, "failed", s.now().UTC().Format(time.RFC3339), 1, counters.Succeeded, counters.Failed, persistErr.Error())
		return errors.Join(persistErr, finishErr, fetchErr)
	}
	_, err = s.repository.FinishRefreshRun(ctx, BookID, run.ID, status, s.now().UTC().Format(time.RFC3339), 1, counters.Succeeded, counters.Failed, lastError)
	if err != nil {
		return err
	}
	return fetchErr
}

func (s *PricingService) fetchAndStoreQuote(ctx context.Context, target db.QuoteRefreshTarget, now time.Time) (PriceObservation, bool, error) {
	provider, ok := s.providerRegistry.PriceProvider(target.ProviderKey)
	if !ok {
		return PriceObservation{}, false, marketdata.ErrUnsupportedPair
	}
	var identifiers map[string]json.RawMessage
	if json.Unmarshal([]byte(target.IdentifiersJSON), &identifiers) != nil {
		return PriceObservation{}, false, marketdata.ErrUnsupportedPair
	}
	key := map[string]string{"coingecko": "coingecko_id", "twelve_data": "twelve_data_symbol", "yahoo_finance": "yahoo_symbol"}[target.ProviderKey]
	var identity string
	_ = json.Unmarshal(identifiers[key], &identity)
	if identity == "" {
		return PriceObservation{}, false, fmt.Errorf("%w: explicit provider identifier is required", marketdata.ErrUnsupportedPair)
	}
	observations, err := provider.FetchPrices(ctx, marketdata.PriceRequest{InstrumentType: target.InstrumentType, ProviderInstrumentID: identity, QuoteCode: target.QuoteCode, MIC: target.MIC})
	if err != nil {
		return PriceObservation{}, false, err
	}
	if len(observations) != 1 {
		return PriceObservation{}, false, errors.New("quote provider returned no single observation")
	}
	fact := observations[0]
	date, err := time.Parse(time.DateOnly, fact.ValuationDate)
	if err != nil || date.Format(time.DateOnly) > now.Format(time.DateOnly) || fact.ProviderInstrumentID != identity || fact.QuoteCode != target.QuoteCode || fact.AdjustmentBasis != "raw" || fact.ProviderObservationID == "" {
		return PriceObservation{}, false, errors.New("quote provider returned incompatible facts")
	}
	if fact.ObservedAt != "" {
		observed, err := time.Parse(time.RFC3339, fact.ObservedAt)
		if err != nil || observed.After(now.Add(5*time.Minute)) {
			return PriceObservation{}, false, errors.New("quote provider returned a future or invalid observation time")
		}
	}
	// The same provider timestamp can be corrected. Content-address the exact
	// value so retries deduplicate while revised facts append a supersession.
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s:%s:%d:%d", target.BaseCommodityID, target.QuoteCommodityID, fact.ProviderObservationID, fact.ValuationDate, fact.PriceValue, fact.PriceScale)))
	providerID := fmt.Sprintf("quote:%x", digest)

	ownerID, err := s.repository.CurrentBookOwnerID(ctx, BookID)
	if err != nil {
		return PriceObservation{}, false, err
	}
	metadata, _ := json.Marshal(map[string]any{"provider_observation_id": fact.ProviderObservationID, "raw": json.RawMessage(fact.RawJSON)})
	input := PriceObservationInput{OwnerUserID: ownerID, BaseCommodityID: target.BaseCommodityID, QuoteCommodityID: target.QuoteCommodityID, SourceID: &target.SourceID, ProviderObservationID: providerID, ProviderSeriesID: identity, QuoteType: fact.QuoteType, AdjustmentBasis: "raw", PriceValue: fact.PriceValue, PriceScale: fact.PriceScale, BaseQuantityValue: 1, ValuationDate: fact.ValuationDate, ObservedAt: fact.ObservedAt, MetadataJSON: string(metadata), ChangeReason: "refreshed instrument quote"}
	spec, err := cleanPriceObservationSpec(input)
	if err != nil {
		return PriceObservation{}, false, err
	}
	record, skipped, err := s.repository.StoreProviderQuote(ctx, db.CreatePriceObservationParams{BookID: BookID, ActorUserID: ownerID, OriginType: "internal", Operation: "pricing.quote_refresh.store", Spec: spec, CreatedAt: now.Format(time.RFC3339), ChangeReason: "refreshed instrument quote"}, fact.ProviderObservationID)
	return toPriceObservation(record), skipped, err
}

// QueueQuoteRefresh accepts durable work and never downloads synchronously.
func (s *PricingService) QueueQuoteRefresh(ctx context.Context, ownerID int64) (int, error) {
	if ownerID <= 0 {
		return 0, ValidationError{Message: "owner user is required"}
	}
	return s.enqueueQuoteRefresh(ctx, 0, "manual")
}
