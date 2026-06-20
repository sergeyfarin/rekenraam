package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
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
		outcome, err := s.refreshFXTarget(ctx, ownerUserID, target, policy, sourceID, startedAt)
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

func (s *PricingService) refreshFXTarget(ctx context.Context, ownerUserID int64, target fxRefreshTarget, policy PricingPolicy, overrideSourceID int64, now time.Time) (fxRefreshOutcome, error) {
	if outcome, err := s.fetchAndStoreDirectFX(ctx, ownerUserID, target, policy, overrideSourceID, now); err == nil {
		return outcome, nil
	}

	if policy.TriangulationMaxHops <= 0 {
		return fxRefreshOutcome{}, fmt.Errorf("FX rate %s/%s is unavailable", target.Base.Code, target.Quote.Code)
	}
	return s.fetchAndStoreDerivedFX(ctx, ownerUserID, target, policy, overrideSourceID, now)
}

func (s *PricingService) fetchAndStoreDirectFX(ctx context.Context, ownerUserID int64, target fxRefreshTarget, policy PricingPolicy, overrideSourceID int64, now time.Time) (fxRefreshOutcome, error) {
	sources, err := s.sourcesForFXPair(ctx, target, policy, overrideSourceID)
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
		observations, err := provider.FetchFXRates(ctx, marketdata.FXRateRequest{
			BaseCode:   target.Base.Code,
			QuoteCodes: []string{target.Quote.Code},
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

func (s *PricingService) fetchAndStoreDerivedFX(ctx context.Context, ownerUserID int64, target fxRefreshTarget, policy PricingPolicy, overrideSourceID int64, now time.Time) (fxRefreshOutcome, error) {
	currencies, err := s.repository.ActiveAccountCurrencies(ctx, BookID)
	if err != nil {
		return fxRefreshOutcome{}, err
	}
	defaultCurrency, err := s.repository.DefaultCurrency(ctx, BookID)
	if err == nil {
		currencies = append(currencies, defaultCurrency)
	}

	var lastErr error
	seen := map[int64]bool{}
	for _, via := range currencies {
		if seen[via.ID] || via.ID == target.Base.ID || via.ID == target.Quote.ID {
			continue
		}
		seen[via.ID] = true

		first, err := s.fetchAndStoreDirectFX(ctx, ownerUserID, fxRefreshTarget{Base: target.Base, Quote: via}, policy, overrideSourceID, now)
		if err != nil {
			lastErr = err
			continue
		}
		second, err := s.fetchAndStoreDirectFX(ctx, ownerUserID, fxRefreshTarget{Base: via, Quote: target.Quote}, policy, overrideSourceID, now)
		if err != nil {
			lastErr = err
			continue
		}

		price, skipped, err := s.storeDerivedFXObservation(ctx, ownerUserID, target, via, first.Observation, second.Observation, first.SourceID, now)
		if err != nil {
			lastErr = err
			continue
		}
		return fxRefreshOutcome{Observation: price, SourceID: first.SourceID, Skipped: skipped}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no triangulation currency is available for %s/%s", target.Base.Code, target.Quote.Code)
	}
	return fxRefreshOutcome{}, lastErr
}

func (s *PricingService) sourcesForFXPair(ctx context.Context, target fxRefreshTarget, policy PricingPolicy, overrideSourceID int64) ([]fxRefreshSource, error) {
	sourceIDs := []int64{}
	if overrideSourceID > 0 {
		sourceIDs = append(sourceIDs, overrideSourceID)
	} else {
		date := s.now().UTC().Format(time.DateOnly)
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

func (s *PricingService) storeDerivedFXObservation(ctx context.Context, ownerUserID int64, target fxRefreshTarget, via db.PricingCurrencyRecord, first PriceObservation, second PriceObservation, sourceID int64, now time.Time) (PriceObservation, bool, error) {
	priceValue, err := scaledFXProduct(first, second, fxDerivedPriceScale)
	if err != nil {
		return PriceObservation{}, false, err
	}
	valuationDate := olderDate(first.ValuationDate, second.ValuationDate)
	providerObservationID := fmt.Sprintf("DERIVED:%s:%d:%d:via:%d:%s:%s", valuationDate, target.Base.ID, target.Quote.ID, via.ID, first.ProviderObservationID, second.ProviderObservationID)
	if existing, err := s.repository.PriceObservationByProviderID(ctx, sourceID, providerObservationID); err == nil {
		return toPriceObservation(existing), true, nil
	} else if !errors.Is(err, db.ErrNotFound) {
		return PriceObservation{}, false, err
	}

	derivationJSON, err := fxDerivationJSON(first, second, sourceID, target, via)
	if err != nil {
		return PriceObservation{}, false, err
	}
	metadataJSON := fmt.Sprintf(`{"source":"fx_refresh","derived":true,"via_currency_code":%q,"mixed_vintage":%t}`, via.Code, fxMixedVintage(first, second))
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

func scaledFXProduct(first PriceObservation, second PriceObservation, resultScale int) (int64, error) {
	numerator := big.NewInt(first.PriceValue)
	numerator.Mul(numerator, big.NewInt(second.PriceValue))
	numerator.Mul(numerator, pow10(first.BaseQuantityScale+second.BaseQuantityScale+resultScale))

	denominator := big.NewInt(first.BaseQuantityValue)
	denominator.Mul(denominator, big.NewInt(second.BaseQuantityValue))
	denominator.Mul(denominator, pow10(first.PriceScale+second.PriceScale))
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

func fxDerivationJSON(first PriceObservation, second PriceObservation, sourceID int64, target fxRefreshTarget, via db.PricingCurrencyRecord) (string, error) {
	payload := fxDerivationMetadata{
		Kind:         "triangulated_fx",
		Warning:      "Derived from multiple FX observations; source legs may have different valuation, observation, or publication times.",
		MixedVintage: fxMixedVintage(first, second),
		Formula:      fmt.Sprintf("%s/%s = (%s/%s) * (%s/%s)", target.Base.Code, target.Quote.Code, target.Base.Code, via.Code, via.Code, target.Quote.Code),
		Legs: []fxDerivationLeg{
			fxObservationLeg(first, sourceID),
			fxObservationLeg(second, sourceID),
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode FX derivation metadata: %w", err)
	}
	return string(encoded), nil
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

func fxMixedVintage(first PriceObservation, second PriceObservation) bool {
	return first.ValuationDate != second.ValuationDate ||
		first.ObservedAt != second.ObservedAt ||
		first.SourcePublishedAt != second.SourcePublishedAt
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
