package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/internal/marketdata"
)

const fxDerivedPriceScale = 12

type fxRefreshTarget struct {
	Base  db.PricingCurrencyRecord
	Quote db.PricingCurrencyRecord
}

type fxRefreshSource struct {
	Record      db.MarketDataSourceRecord
	ProviderKey string
}

// fxPathLeg is one provider-backed leg of a triangulation chain. A chain of N
// legs routes through N-1 intermediate currencies (the policy's "hops").
type fxPathLeg struct {
	Target      fxRefreshTarget
	Observation PriceObservation
	SourceID    int64
}

type fxRefreshOutcome struct {
	Observation PriceObservation
	SourceID    int64
	Skipped     bool
}

type fxRefreshCounters struct {
	Total     int
	Succeeded int
	Failed    int
	LastError string
}

type fxDerivationMetadata struct {
	Kind         string            `json:"kind"`
	Warning      string            `json:"warning"`
	MixedVintage bool              `json:"mixed_vintage"`
	Formula      string            `json:"formula"`
	Legs         []fxDerivationLeg `json:"legs"`
}

type fxDerivationLeg struct {
	ObservationID         int64  `json:"observation_id"`
	SourceID              int64  `json:"source_id"`
	ProviderObservationID string `json:"provider_observation_id"`
	BaseCommodityID       int64  `json:"base_commodity_id"`
	QuoteCommodityID      int64  `json:"quote_commodity_id"`
	ValuationDate         string `json:"valuation_date"`
	ObservedAt            string `json:"observed_at,omitempty"`
	SourcePublishedAt     string `json:"source_published_at,omitempty"`
	PriceValue            int64  `json:"price_value"`
	PriceScale            int    `json:"price_scale"`
	BaseQuantityValue     int64  `json:"base_quantity_value"`
	BaseQuantityScale     int    `json:"base_quantity_scale"`
}

func (s *PricingService) runFXRefresh(ctx context.Context, ownerUserID int64, sourceID int64, trigger string) (PricingRefreshRun, error) {
	startedAt := s.now().UTC()
	policy, err := s.pricingPolicyOrDefault(ctx)
	if err != nil {
		return PricingRefreshRun{}, err
	}
	runSourceID, err := s.refreshRunSourceID(ctx, policy, sourceID)
	if err != nil {
		return PricingRefreshRun{}, err
	}

	runRecord, err := s.repository.RecordRefreshRun(ctx, BookID, runSourceID, trigger, "running", startedAt.Format(time.RFC3339), "", 0, 0, 0, "")
	if err != nil {
		return PricingRefreshRun{}, fmt.Errorf("start pricing refresh run: %w", err)
	}

	counters := fxRefreshCounters{}
	targets, targetErr := s.fxRefreshTargets(ctx, policy)
	if targetErr != nil {
		counters.LastError = targetErr.Error()
		return s.finishFXRefreshRun(ctx, runRecord.ID, counters)
	}

	for _, target := range targets {
		counters.Total++
		outcome, err := s.refreshFXTarget(ctx, ownerUserID, target, policy, sourceID, "", startedAt)
		if err != nil {
			counters.Failed++
			counters.LastError = err.Error()
			_ = s.recordFXRefreshItem(ctx, runRecord.ID, target, "failed", 0, "", err.Error(), startedAt)
			_ = s.repository.UpsertRefreshState(ctx, db.PricingRefreshStateSpec{
				BookID:           BookID,
				BaseCommodityID:  target.Base.ID,
				QuoteCommodityID: target.Quote.ID,
				SourceID:         runSourceID,
				LastAttemptAt:    startedAt.Format(time.RFC3339),
				LastError:        err.Error(),
				UpdatedAt:        startedAt.Format(time.RFC3339),
			})
			continue
		}

		counters.Succeeded++
		itemStatus := "succeeded"
		if outcome.Skipped {
			itemStatus = "skipped"
		}
		_ = s.recordFXRefreshItem(ctx, runRecord.ID, target, itemStatus, outcome.Observation.ID, outcome.Observation.ProviderObservationID, "", startedAt)
		_ = s.repository.UpsertRefreshState(ctx, db.PricingRefreshStateSpec{
			BookID:           BookID,
			BaseCommodityID:  target.Base.ID,
			QuoteCommodityID: target.Quote.ID,
			SourceID:         outcome.SourceID,
			LastSuccessDate:  sql.NullString{String: outcome.Observation.ValuationDate, Valid: outcome.Observation.ValuationDate != ""},
			LastAttemptAt:    startedAt.Format(time.RFC3339),
			UpdatedAt:        startedAt.Format(time.RFC3339),
		})
	}

	return s.finishFXRefreshRun(ctx, runRecord.ID, counters)
}

func (s *PricingService) finishFXRefreshRun(ctx context.Context, runID int64, counters fxRefreshCounters) (PricingRefreshRun, error) {
	status := "succeeded"
	if counters.Failed > 0 && counters.Succeeded > 0 {
		status = "partial"
	} else if counters.Failed > 0 {
		status = "failed"
	}
	if counters.Total == 0 && counters.LastError != "" {
		status = "failed"
	}

	record, err := s.repository.FinishRefreshRun(ctx, BookID, runID, status, s.now().UTC().Format(time.RFC3339), counters.Total, counters.Succeeded, counters.Failed, counters.LastError)
	if err != nil {
		return PricingRefreshRun{}, fmt.Errorf("finish pricing refresh run: %w", err)
	}
	return toPricingRefreshRun(record), nil
}

func (s *PricingService) pricingPolicyOrDefault(ctx context.Context) (PricingPolicy, error) {
	record, err := s.repository.GetPricingPolicy(ctx, BookID)
	if err == nil {
		return toPricingPolicy(record), nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return PricingPolicy{}, fmt.Errorf("read pricing policy: %w", err)
	}
	return PricingPolicy{
		BookID:               BookID,
		RefreshEnabled:       true,
		RefreshHourUTC:       4,
		RefreshMinuteUTC:     0,
		RefreshHourLocal:     4,
		RefreshMinuteLocal:   0,
		MaxBackfillDays:      370,
		StalenessMaxDays:     3,
		TriangulationMaxHops: 1,
		RoundingMode:         "half_up",
		PreferOfficialFX:     true,
		WeekendPolicy:        "skip",
	}, nil
}

func (s *PricingService) refreshRunSourceID(ctx context.Context, policy PricingPolicy, overrideSourceID int64) (int64, error) {
	if overrideSourceID > 0 {
		return overrideSourceID, nil
	}
	if policy.DefaultSourceID != nil {
		return *policy.DefaultSourceID, nil
	}
	sourceID, err := s.repository.ManualSourceID(ctx)
	if err != nil {
		return 0, err
	}
	return sourceID, nil
}

func (s *PricingService) fxRefreshTargets(ctx context.Context, policy PricingPolicy) ([]fxRefreshTarget, error) {
	defaultCurrency, err := s.repository.DefaultCurrency(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("default currency is required before FX refresh")
	}
	base := defaultCurrency
	if policy.BaseCommodityID != nil {
		base, err = s.repository.CurrencyByID(ctx, BookID, *policy.BaseCommodityID)
		if err != nil {
			return nil, fmt.Errorf("read FX base currency: %w", err)
		}
	}

	currencies, err := s.repository.ActiveAccountCurrencies(ctx, BookID)
	if err != nil {
		return nil, err
	}
	currencyByID := map[int64]db.PricingCurrencyRecord{
		base.ID:            base,
		defaultCurrency.ID: defaultCurrency,
	}
	for _, currency := range currencies {
		currencyByID[currency.ID] = currency
	}

	targets := make([]fxRefreshTarget, 0, len(currencyByID)*2)
	seen := map[string]bool{}
	for _, currency := range currencyByID {
		if currency.ID == base.ID {
			continue
		}
		for _, pair := range []fxRefreshTarget{{Base: base, Quote: currency}, {Base: currency, Quote: base}} {
			key := fxPairKey(pair.Base.ID, pair.Quote.ID)
			if seen[key] {
				continue
			}
			seen[key] = true
			targets = append(targets, pair)
		}
	}
	return targets, nil
}

func (s *PricingService) refreshFXTarget(ctx context.Context, ownerUserID int64, target fxRefreshTarget, policy PricingPolicy, overrideSourceID int64, valuationDate string, now time.Time) (fxRefreshOutcome, error) {
	if outcome, err := s.fetchAndStoreDirectFX(ctx, ownerUserID, target, policy, overrideSourceID, valuationDate, now); err == nil {
		return outcome, nil
	}

	if policy.TriangulationMaxHops <= 0 {
		return fxRefreshOutcome{}, fmt.Errorf("FX rate %s/%s is unavailable", target.Base.Code, target.Quote.Code)
	}
	return s.fetchAndStoreDerivedFX(ctx, ownerUserID, target, policy, overrideSourceID, valuationDate, now)
}

func (s *PricingService) fetchAndStoreDirectFX(ctx context.Context, ownerUserID int64, target fxRefreshTarget, policy PricingPolicy, overrideSourceID int64, valuationDate string, now time.Time) (fxRefreshOutcome, error) {
	sources, err := s.sourcesForFXPair(ctx, target, policy, overrideSourceID, valuationDate)
	if err != nil {
		return fxRefreshOutcome{}, err
	}

	var lastErr error
	for _, source := range sources {
		provider, ok := s.providerRegistry.FXRateProvider(source.ProviderKey)
		if !ok {
			lastErr = fmt.Errorf("market data provider is unavailable: %s", source.ProviderKey)
			continue
		}
		if valuationDate != "" && !provider.Capabilities().HistoricalFXRates {
			continue
		}
		observations, err := provider.FetchFXRates(ctx, marketdata.FXRateRequest{
			BaseCode:   target.Base.Code,
			QuoteCodes: []string{target.Quote.Code},
			Date:       valuationDate,
		})
		if err != nil {
			if errors.Is(err, marketdata.ErrUnsupportedPair) || errors.Is(err, marketdata.ErrMissingSecret) {
				lastErr = err
				continue
			}
			lastErr = err
			continue
		}
		for _, observation := range observations {
			if strings.EqualFold(observation.BaseCode, target.Base.Code) && strings.EqualFold(observation.QuoteCode, target.Quote.Code) {
				price, skipped, err := s.storeProviderFXObservation(ctx, ownerUserID, source.Record.ID, target, observation, now)
				if err != nil {
					lastErr = err
					continue
				}
				return fxRefreshOutcome{Observation: price, SourceID: source.Record.ID, Skipped: skipped}, nil
			}
		}
		lastErr = fmt.Errorf("provider %s returned no %s/%s rate", source.Record.Code, target.Base.Code, target.Quote.Code)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no active FX provider is configured")
	}
	return fxRefreshOutcome{}, lastErr
}

func (s *PricingService) fetchAndStoreDerivedFX(ctx context.Context, ownerUserID int64, target fxRefreshTarget, policy PricingPolicy, overrideSourceID int64, valuationDate string, now time.Time) (fxRefreshOutcome, error) {
	candidates, err := s.triangulationCandidates(ctx, target)
	if err != nil {
		return fxRefreshOutcome{}, err
	}

	legs, err := s.findFXPath(ctx, ownerUserID, target, candidates, policy, overrideSourceID, valuationDate, now)
	if err != nil {
		return fxRefreshOutcome{}, err
	}

	// The chain is attributed to its first leg's source, as the single-hop
	// derivation always was; every leg's own source is recorded per-leg in the
	// derivation metadata.
	sourceID := legs[0].SourceID
	price, skipped, err := s.storeDerivedFXObservation(ctx, ownerUserID, target, legs, sourceID, now)
	if err != nil {
		return fxRefreshOutcome{}, err
	}
	return fxRefreshOutcome{Observation: price, SourceID: sourceID, Skipped: skipped}, nil
}

// triangulationCandidates is the set of currencies a derived chain may route
// through: every currency an active account uses, plus the book default,
// minus the pair's own endpoints.
func (s *PricingService) triangulationCandidates(ctx context.Context, target fxRefreshTarget) ([]db.PricingCurrencyRecord, error) {
	currencies, err := s.repository.ActiveAccountCurrencies(ctx, BookID)
	if err != nil {
		return nil, err
	}
	if defaultCurrency, err := s.repository.DefaultCurrency(ctx, BookID); err == nil {
		currencies = append(currencies, defaultCurrency)
	}

	seen := map[int64]bool{}
	candidates := make([]db.PricingCurrencyRecord, 0, len(currencies))
	for _, currency := range currencies {
		if seen[currency.ID] || currency.ID == target.Base.ID || currency.ID == target.Quote.ID {
			continue
		}
		seen[currency.ID] = true
		candidates = append(candidates, currency)
	}
	return candidates, nil
}

// findFXPath resolves the shortest chain base -> via... -> quote whose every
// leg a provider can actually serve, routing through at most
// policy.TriangulationMaxHops intermediate currencies (T-40: the knob used to
// be a bare on/off check with the path hard-coded to one hop).
//
// Shortest matters: each additional leg compounds rounding and widens the
// vintage spread across the source observations, so the search deepens one hop
// at a time and returns the first chain it can complete. Re-walking the
// shallower depths costs nothing because every leg fetch is memoized — legs are
// shared across candidate paths and each miss is a provider call.
func (s *PricingService) findFXPath(ctx context.Context, ownerUserID int64, target fxRefreshTarget, candidates []db.PricingCurrencyRecord, policy PricingPolicy, overrideSourceID int64, valuationDate string, now time.Time) ([]fxPathLeg, error) {
	type legResult struct {
		leg fxPathLeg
		err error
	}
	legCache := map[string]legResult{}
	fetchLeg := func(from db.PricingCurrencyRecord, to db.PricingCurrencyRecord) (fxPathLeg, error) {
		key := fxPairKey(from.ID, to.ID)
		if cached, ok := legCache[key]; ok {
			return cached.leg, cached.err
		}
		legTarget := fxRefreshTarget{Base: from, Quote: to}
		outcome, err := s.fetchAndStoreDirectFX(ctx, ownerUserID, legTarget, policy, overrideSourceID, valuationDate, now)
		result := legResult{leg: fxPathLeg{Target: legTarget, Observation: outcome.Observation, SourceID: outcome.SourceID}, err: err}
		legCache[key] = result
		return result.leg, result.err
	}

	var lastErr error
	used := map[int64]bool{}
	for hops := 1; hops <= policy.TriangulationMaxHops; hops++ {
		if legs := searchFXPath(target.Base, target.Quote, candidates, hops, used, fetchLeg, &lastErr); legs != nil {
			return legs, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no triangulation currency is available for %s/%s", target.Base.Code, target.Quote.Code)
	}
	return nil, lastErr
}

// searchFXPath walks candidate chains from `from` to `quote` using at most
// remainingHops intermediate currencies, returning nil when none completes.
// Callers deepen remainingHops one at a time (see findFXPath), which is what
// makes the first chain returned a shortest one.
func searchFXPath(from db.PricingCurrencyRecord, quote db.PricingCurrencyRecord, candidates []db.PricingCurrencyRecord, remainingHops int, used map[int64]bool, fetchLeg func(db.PricingCurrencyRecord, db.PricingCurrencyRecord) (fxPathLeg, error), lastErr *error) []fxPathLeg {
	if remainingHops <= 0 {
		return nil
	}
	for _, via := range candidates {
		if used[via.ID] {
			continue
		}
		first, err := fetchLeg(from, via)
		if err != nil {
			*lastErr = err
			continue
		}
		if last, err := fetchLeg(via, quote); err == nil {
			return []fxPathLeg{first, last}
		} else {
			*lastErr = err
		}

		used[via.ID] = true
		rest := searchFXPath(via, quote, candidates, remainingHops-1, used, fetchLeg, lastErr)
		used[via.ID] = false
		if rest != nil {
			// Fresh slice: siblings in the search must not share a backing array.
			legs := make([]fxPathLeg, 0, len(rest)+1)
			return append(append(legs, first), rest...)
		}
	}
	return nil
}

func (s *PricingService) sourcesForFXPair(ctx context.Context, target fxRefreshTarget, policy PricingPolicy, overrideSourceID int64, valuationDate string) ([]fxRefreshSource, error) {
	sourceIDs := []int64{}
	if overrideSourceID > 0 {
		sourceIDs = append(sourceIDs, overrideSourceID)
	} else {
		date := valuationDate
		if date == "" {
			date = s.now().UTC().Format(time.DateOnly)
		}
		if assignment, err := s.repository.SourceAssignmentForPair(ctx, BookID, target.Base.ID, target.Quote.ID, date); err == nil {
			sourceIDs = append(sourceIDs, assignment.SourceID)
		} else if !errors.Is(err, db.ErrNotFound) {
			return nil, err
		}
		if policy.DefaultSourceID != nil {
			sourceIDs = append(sourceIDs, *policy.DefaultSourceID)
		}
	}

	records, err := s.repository.ListSources(ctx)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if record.Kind == "provider" && record.Status == "active" {
			sourceIDs = append(sourceIDs, record.ID)
		}
	}

	seen := map[int64]bool{}
	sources := make([]fxRefreshSource, 0, len(sourceIDs))
	recordsByID := map[int64]db.MarketDataSourceRecord{}
	for _, record := range records {
		recordsByID[record.ID] = record
	}
	for _, sourceID := range sourceIDs {
		if seen[sourceID] {
			continue
		}
		seen[sourceID] = true
		record, ok := recordsByID[sourceID]
		if !ok {
			record, err = s.repository.SourceByID(ctx, sourceID)
			if err != nil {
				return nil, err
			}
		}
		if record.Kind != "provider" || record.Status != "active" {
			continue
		}
		providerKey := nullableString(record.ProviderKey)
		if providerKey == "" {
			providerKey = record.Code
		}
		sources = append(sources, fxRefreshSource{Record: record, ProviderKey: providerKey})
	}
	return sources, nil
}

func (s *PricingService) storeProviderFXObservation(ctx context.Context, ownerUserID int64, sourceID int64, target fxRefreshTarget, observation marketdata.FXRateObservation, now time.Time) (PriceObservation, bool, error) {
	providerObservationID := strings.TrimSpace(observation.ProviderObservationID)
	if providerObservationID == "" {
		providerObservationID = fmt.Sprintf("%s:%s:%s:%s", observation.SourceCode, observation.ValuationDate, target.Base.Code, target.Quote.Code)
	}
	if existing, err := s.repository.PriceObservationByProviderID(ctx, sourceID, providerObservationID); err == nil {
		return toPriceObservation(existing), true, nil
	} else if !errors.Is(err, db.ErrNotFound) {
		return PriceObservation{}, false, err
	}

	metadataJSON := strings.TrimSpace(observation.MetadataJSON)
	if metadataJSON == "" {
		metadataJSON = "{}"
	}
	rawJSON := strings.TrimSpace(observation.RawJSON)
	if rawJSON != "" && rawJSON != "{}" {
		metadataJSON = mergeFXMetadata(metadataJSON, rawJSON)
	}

	record, err := s.repository.CreatePriceObservation(ctx, db.CreatePriceObservationParams{
		BookID:       BookID,
		ActorUserID:  ownerUserID,
		OriginType:   "internal",
		Operation:    "pricing.fx_refresh.store",
		CreatedAt:    now.Format(time.RFC3339),
		ChangeReason: "refreshed FX rate",
		Spec: db.PriceObservationSpec{
			BaseCommodityID:       target.Base.ID,
			QuoteCommodityID:      target.Quote.ID,
			QuoteType:             defaultString(observation.QuoteType, "official_fixing"),
			AdjustmentBasis:       defaultString(observation.AdjustmentBasis, "not_applicable"),
			PriceValue:            observation.PriceValue,
			PriceScale:            observation.PriceScale,
			BaseQuantityValue:     observation.BaseQuantityValue,
			BaseQuantityScale:     observation.BaseQuantityScale,
			ValuationDate:         observation.ValuationDate,
			ObservedAt:            nullableSQLString(observation.ObservedAt),
			SourcePublishedAt:     nullableSQLString(observation.SourcePublishedAt),
			SourceID:              sql.NullInt64{Int64: sourceID, Valid: true},
			ProviderSeriesID:      observation.ProviderSeriesID,
			ProviderObservationID: providerObservationID,
			IsManual:              false,
			IsDerived:             false,
			DerivationJSON:        "{}",
			SeriesMetadataJSON:    "{}",
			MetadataJSON:          metadataJSON,
		},
	})
	if err != nil {
		return PriceObservation{}, false, err
	}
	return toPriceObservation(record), false, nil
}

func (s *PricingService) storeDerivedFXObservation(ctx context.Context, ownerUserID int64, target fxRefreshTarget, legs []fxPathLeg, sourceID int64, now time.Time) (PriceObservation, bool, error) {
	observations := fxPathObservations(legs)
	priceValue, err := scaledFXProduct(observations, fxDerivedPriceScale)
	if err != nil {
		return PriceObservation{}, false, err
	}
	valuationDate := oldestFXDate(observations)
	providerObservationID := fxDerivedProviderObservationID(target, legs, valuationDate)
	if existing, err := s.repository.PriceObservationByProviderID(ctx, sourceID, providerObservationID); err == nil {
		return toPriceObservation(existing), true, nil
	} else if !errors.Is(err, db.ErrNotFound) {
		return PriceObservation{}, false, err
	}

	derivationJSON, err := fxDerivationJSON(legs, sourceID, target)
	if err != nil {
		return PriceObservation{}, false, err
	}
	metadataJSON, err := fxDerivedMetadataJSON(legs)
	if err != nil {
		return PriceObservation{}, false, err
	}
	record, err := s.repository.CreatePriceObservation(ctx, db.CreatePriceObservationParams{
		BookID:       BookID,
		ActorUserID:  ownerUserID,
		OriginType:   "internal",
		Operation:    "pricing.fx_refresh.derive",
		CreatedAt:    now.Format(time.RFC3339),
		ChangeReason: "derived FX rate from provider observations",
		Spec: db.PriceObservationSpec{
			BaseCommodityID:       target.Base.ID,
			QuoteCommodityID:      target.Quote.ID,
			QuoteType:             "official_fixing",
			AdjustmentBasis:       "not_applicable",
			PriceValue:            priceValue,
			PriceScale:            fxDerivedPriceScale,
			BaseQuantityValue:     1,
			BaseQuantityScale:     0,
			ValuationDate:         valuationDate,
			SourceID:              sql.NullInt64{Int64: sourceID, Valid: true},
			ProviderSeriesID:      fmt.Sprintf("DERIVED:%d:%d", target.Base.ID, target.Quote.ID),
			ProviderObservationID: providerObservationID,
			IsManual:              false,
			IsDerived:             true,
			DerivationJSON:        derivationJSON,
			SeriesMetadataJSON:    `{"source":"fx_refresh","derived":true}`,
			MetadataJSON:          metadataJSON,
		},
	})
	if err != nil {
		return PriceObservation{}, false, err
	}
	return toPriceObservation(record), false, nil
}

func (s *PricingService) recordFXRefreshItem(ctx context.Context, runID int64, target fxRefreshTarget, status string, observationID int64, providerObservationID string, itemError string, now time.Time) error {
	return s.repository.RecordRefreshItem(ctx, db.PricingRefreshItemSpec{
		RunID:          runID,
		BookID:         BookID,
		ItemKind:       "price",
		Status:         status,
		ProviderItemID: providerObservationID,
		LocalRefTable:  "price_observations",
		LocalRefID:     observationID,
		Error:          itemError,
		RawJSON:        fmt.Sprintf(`{"base_commodity_id":%d,"quote_commodity_id":%d,"base_code":%q,"quote_code":%q}`, target.Base.ID, target.Quote.ID, target.Base.Code, target.Quote.Code),
		CreatedAt:      now.Format(time.RFC3339),
	})
}

// scaledFXProduct multiplies every leg's rate (priceValue/10^priceScale per
// baseQuantityValue/10^baseQuantityScale) into one rate at resultScale. The
// whole chain is one exact big.Int quotient, so no intermediate leg product is
// ever rounded — only the final result is.
func scaledFXProduct(observations []PriceObservation, resultScale int) (int64, error) {
	if len(observations) < 2 {
		return 0, fmt.Errorf("derived FX needs at least two legs")
	}
	numerator := big.NewInt(1)
	denominator := big.NewInt(1)
	baseQuantityScaleSum := resultScale
	priceScaleSum := 0
	for _, observation := range observations {
		numerator.Mul(numerator, big.NewInt(observation.PriceValue))
		denominator.Mul(denominator, big.NewInt(observation.BaseQuantityValue))
		baseQuantityScaleSum += observation.BaseQuantityScale
		priceScaleSum += observation.PriceScale
	}
	numerator.Mul(numerator, exact.Pow10(baseQuantityScaleSum))
	denominator.Mul(denominator, exact.Pow10(priceScaleSum))
	if denominator.Sign() == 0 {
		return 0, fmt.Errorf("derived FX denominator is zero")
	}

	value, remainder := new(big.Int), new(big.Int)
	value.QuoRem(numerator, denominator, remainder)
	// Price inputs are positive. Round half up at the explicit result scale so
	// derived rates are not systematically biased downward by truncation.
	if new(big.Int).Lsh(remainder, 1).Cmp(denominator) >= 0 {
		value.Add(value, big.NewInt(1))
	}
	if value.Sign() <= 0 {
		return 0, fmt.Errorf("derived FX value is zero")
	}
	if !value.IsInt64() {
		return 0, fmt.Errorf("derived FX value overflows int64")
	}
	return value.Int64(), nil
}

func fxDerivationJSON(legs []fxPathLeg, sourceID int64, target fxRefreshTarget) (string, error) {
	derivationLegs := make([]fxDerivationLeg, 0, len(legs))
	factors := make([]string, 0, len(legs))
	for _, leg := range legs {
		derivationLegs = append(derivationLegs, fxObservationLeg(leg.Observation, sourceID))
		factors = append(factors, fmt.Sprintf("(%s/%s)", leg.Target.Base.Code, leg.Target.Quote.Code))
	}
	payload := fxDerivationMetadata{
		Kind:         "triangulated_fx",
		Warning:      "Derived from multiple FX observations; source legs may have different valuation, observation, or publication times.",
		MixedVintage: fxMixedVintage(fxPathObservations(legs)),
		Formula:      fmt.Sprintf("%s/%s = %s", target.Base.Code, target.Quote.Code, strings.Join(factors, " * ")),
		Legs:         derivationLegs,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode FX derivation metadata: %w", err)
	}
	return string(encoded), nil
}

type fxDerivedMetadata struct {
	Source           string   `json:"source"`
	Derived          bool     `json:"derived"`
	ViaCurrencyCodes []string `json:"via_currency_codes"`
	MixedVintage     bool     `json:"mixed_vintage"`
}

func fxDerivedMetadataJSON(legs []fxPathLeg) (string, error) {
	// One entry per intermediate currency: every leg's quote except the last,
	// which is the target's own quote.
	viaCodes := make([]string, 0, len(legs)-1)
	for _, leg := range legs[:len(legs)-1] {
		viaCodes = append(viaCodes, leg.Target.Quote.Code)
	}
	encoded, err := json.Marshal(fxDerivedMetadata{
		Source:           "fx_refresh",
		Derived:          true,
		ViaCurrencyCodes: viaCodes,
		MixedVintage:     fxMixedVintage(fxPathObservations(legs)),
	})
	if err != nil {
		return "", fmt.Errorf("encode derived FX metadata: %w", err)
	}
	return string(encoded), nil
}

// fxDerivedProviderObservationID is the chain's dedupe key. The single-hop
// spelling is unchanged, so pre-T-40 derived rows still match and are not
// re-derived.
func fxDerivedProviderObservationID(target fxRefreshTarget, legs []fxPathLeg, valuationDate string) string {
	viaIDs := make([]string, 0, len(legs)-1)
	for _, leg := range legs[:len(legs)-1] {
		viaIDs = append(viaIDs, strconv.FormatInt(leg.Target.Quote.ID, 10))
	}
	observationIDs := make([]string, 0, len(legs))
	for _, leg := range legs {
		observationIDs = append(observationIDs, leg.Observation.ProviderObservationID)
	}
	return fmt.Sprintf("DERIVED:%s:%d:%d:via:%s:%s", valuationDate, target.Base.ID, target.Quote.ID,
		strings.Join(viaIDs, "-"), strings.Join(observationIDs, ":"))
}

func fxPathObservations(legs []fxPathLeg) []PriceObservation {
	observations := make([]PriceObservation, 0, len(legs))
	for _, leg := range legs {
		observations = append(observations, leg.Observation)
	}
	return observations
}

func fxObservationLeg(observation PriceObservation, fallbackSourceID int64) fxDerivationLeg {
	sourceID := fallbackSourceID
	if observation.SourceID != nil {
		sourceID = *observation.SourceID
	}
	return fxDerivationLeg{
		ObservationID:         observation.ID,
		SourceID:              sourceID,
		ProviderObservationID: observation.ProviderObservationID,
		BaseCommodityID:       observation.BaseCommodityID,
		QuoteCommodityID:      observation.QuoteCommodityID,
		ValuationDate:         observation.ValuationDate,
		ObservedAt:            observation.ObservedAt,
		SourcePublishedAt:     observation.SourcePublishedAt,
		PriceValue:            observation.PriceValue,
		PriceScale:            observation.PriceScale,
		BaseQuantityValue:     observation.BaseQuantityValue,
		BaseQuantityScale:     observation.BaseQuantityScale,
	}
}

// fxMixedVintage reports whether the chain's legs disagree on when they were
// valued, observed, or published — the caveat a reader of a derived rate needs.
func fxMixedVintage(observations []PriceObservation) bool {
	for _, observation := range observations[1:] {
		if observation.ValuationDate != observations[0].ValuationDate ||
			observation.ObservedAt != observations[0].ObservedAt ||
			observation.SourcePublishedAt != observations[0].SourcePublishedAt {
			return true
		}
	}
	return false
}

// oldestFXDate dates the chain by its stalest leg: a derived rate is only as
// current as its least current input.
func oldestFXDate(observations []PriceObservation) string {
	oldest := ""
	for _, observation := range observations {
		oldest = olderDate(oldest, observation.ValuationDate)
	}
	return oldest
}

func olderDate(left string, right string) string {
	if left == "" {
		return right
	}
	if right == "" || left <= right {
		return left
	}
	return right
}

func fxPairKey(baseCommodityID int64, quoteCommodityID int64) string {
	return fmt.Sprintf("%d:%d", baseCommodityID, quoteCommodityID)
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func mergeFXMetadata(metadataJSON string, rawJSON string) string {
	if !json.Valid([]byte(metadataJSON)) || !json.Valid([]byte(rawJSON)) {
		return metadataJSON
	}
	return fmt.Sprintf(`{"provider_metadata":%s,"raw":%s}`, metadataJSON, rawJSON)
}
