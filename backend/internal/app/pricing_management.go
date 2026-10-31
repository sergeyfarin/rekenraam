package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

type PricingCoverage struct {
	Reason           string
	BaseCommodityID  int64
	QuoteCommodityID int64
	Status           string
	AgeDays          *int
	Observation      *PriceObservation
}

type PricingCommodity struct {
	ID     int64
	Code   string
	Name   string
	Kind   string
	Status string
}

type PricingWorkspace struct {
	AsOf             string
	StalenessMaxDays int
	Commodities      []PricingCommodity
	Sources          []MarketDataSource
	Coverage         []PricingCoverage
}

func (s *PricingService) Workspace(ctx context.Context, asOf string) (PricingWorkspace, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return PricingWorkspace{}, referenceErr
	}

	result := PricingWorkspace{Coverage: []PricingCoverage{}}
	if asOf == "" {
		zone, err := s.repository.BookOwnerTimeZone(ctx, BookID)
		if err != nil {
			return result, err
		}
		location, err := time.LoadLocation(zone)
		if err != nil {
			return result, err
		}
		asOf = s.now().In(location).Format(time.DateOnly)
	}
	asOf, err := cleanRequiredDate(asOf, "as of date")
	if err != nil {
		return result, err
	}
	policy, err := s.GetPolicy(ctx)
	if err != nil {
		return result, err
	}
	baseID := int64(0)
	if policy.BaseCommodityID != nil {
		baseID = *policy.BaseCommodityID
	} else {
		base, err := s.repository.DefaultCurrency(ctx, BookID)
		if err != nil {
			return result, err
		}
		baseID = base.ID
	}

	boundary, err := s.repository.CurrentObservationBoundary(ctx, BookID, s.now())
	if err != nil {
		return result, err
	}
	// An explicitly selected future management date is a diagnostic, not current value.
	boundary.AsOf = asOf
	balances := map[int64]map[int64]*exact.ScaledInt{}
	snapshot, err := s.repository.PriceManagementSnapshot(ctx, BookID, asOf, baseID, func(account db.LedgerAccountRecord, posting db.LedgerPostingRecord) {
		if countsNetWorthAccount(account) {
			foldPosting(balances, posting)
		}
	}, boundary)
	if err != nil {
		return result, err
	}
	// Keep obligations per account: equal and opposite balances in separate
	// accounts still need a rate when either account is valued on its own.
	pairs := map[[2]int64]bool{}
	for _, pair := range snapshot.Pairs {
		pairs[[2]int64{pair.BaseCommodityID, pair.QuoteCommodityID}] = true
	}
	if baseID > 0 {
		for _, amounts := range balances {
			for commodityID, amount := range amounts {
				key := [2]int64{commodityID, baseID}
				if commodityID != baseID && amount.Sign() != 0 && !pairs[key] {
					pairs[key] = true
					snapshot.Pairs = append(snapshot.Pairs, db.PricingPairRecord{BaseCommodityID: commodityID, QuoteCommodityID: baseID})
				}
			}
		}
	}
	slices.SortFunc(snapshot.Pairs, func(a, b db.PricingPairRecord) int {
		if a.BaseCommodityID < b.BaseCommodityID || (a.BaseCommodityID == b.BaseCommodityID && a.QuoteCommodityID < b.QuoteCommodityID) {
			return -1
		}
		if a == b {
			return 0
		}
		return 1
	})
	sources, err := s.ListSources(ctx)
	if err != nil {
		return result, err
	}
	result.AsOf = asOf
	result.StalenessMaxDays = policy.StalenessMaxDays
	result.Commodities = make([]PricingCommodity, 0, len(snapshot.Commodities))
	for _, commodity := range snapshot.Commodities {
		result.Commodities = append(result.Commodities, PricingCommodity{
			ID: commodity.ID, Code: commodity.Code, Name: commodity.Name, Kind: commodity.Kind, Status: commodity.Status,
		})
	}
	result.Sources = sources
	candidates := map[[2]int64]PriceObservation{}
	for _, record := range snapshot.Candidates {
		candidates[[2]int64{record.BaseCommodityID, record.QuoteCommodityID}] = toPriceObservation(record)
	}
	requestedDate, _ := time.Parse(time.DateOnly, asOf)
	for _, pair := range snapshot.Pairs {
		if pair.BaseCommodityID == pair.QuoteCommodityID {
			continue
		}
		coverage := PricingCoverage{Reason: snapshot.Reasons[[2]int64{pair.BaseCommodityID, pair.QuoteCommodityID}], BaseCommodityID: pair.BaseCommodityID, QuoteCommodityID: pair.QuoteCommodityID, Status: "missing"}
		if price, ok := candidates[[2]int64{pair.BaseCommodityID, pair.QuoteCommodityID}]; ok {
			date, _ := time.Parse(time.DateOnly, price.ValuationDate)
			age := int((requestedDate.Unix() - date.Unix()) / 86400)
			coverage.AgeDays = &age
			coverage.Observation = &price
			coverage.Status = "usable"
			// A policy longer than the report's window still cannot call
			// usable a price that reports, and so Home, cannot value with.
			if age > policy.StalenessMaxDays || age > DefaultRateStalenessDays {
				coverage.Status = "stale"
			}
		}
		result.Coverage = append(result.Coverage, coverage)
	}
	return result, nil
}

type priceHistoryCursor struct {
	Base   int64 `json:"b"`
	Quote  int64 `json:"q"`
	Voided bool  `json:"v"`
	Before int64 `json:"i"`
}

type PriceHistory struct {
	Prices     []PriceObservation
	NextCursor string
}

func (s *PricingService) History(ctx context.Context, baseID, quoteID int64, includeVoided bool, cursor string, limit int) (PriceHistory, error) {
	if baseID <= 0 || quoteID <= 0 || baseID == quoteID {
		return PriceHistory{}, ValidationError{Message: "distinct price pair is required"}
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		return PriceHistory{}, ValidationError{Message: "price history limit exceeds 100"}
	}
	beforeID := int64(0)
	if cursor != "" {
		payload, err := base64.RawURLEncoding.DecodeString(cursor)
		var decoded priceHistoryCursor
		if err != nil || json.Unmarshal(payload, &decoded) != nil || decoded.Base != baseID || decoded.Quote != quoteID || decoded.Voided != includeVoided || decoded.Before <= 0 {
			return PriceHistory{}, ValidationError{Message: "price history cursor is invalid for these filters"}
		}
		beforeID = decoded.Before
	}
	records, err := s.repository.PriceHistoryPage(ctx, BookID, baseID, quoteID, beforeID, includeVoided, limit+1)
	if err != nil {
		return PriceHistory{}, err
	}
	result := PriceHistory{}
	if len(records) > limit {
		records = records[:limit]
		payload, err := json.Marshal(priceHistoryCursor{Base: baseID, Quote: quoteID, Voided: includeVoided, Before: records[len(records)-1].ID})
		if err != nil {
			return result, fmt.Errorf("encode price cursor: %w", err)
		}
		result.NextCursor = base64.RawURLEncoding.EncodeToString(payload)
	}
	result.Prices = toPriceObservations(records)
	return result, nil
}

// priceInt64 parses the existing bounded pricing model from an exact string;
// coefficients never pass through a browser Number or a Go float.
func priceInt64(coefficient string) (int64, error) {
	value, err := exact.Parse(coefficient)
	if err != nil || value.Sign() <= 0 || value.String() != coefficient {
		return 0, ValidationError{Message: "price coefficients must be canonical positive int64 strings"}
	}
	return exact.ScaledIntFromCoefficient(value, 0).Int64()
}

var ErrPriceImpactStale = errors.New("price dependencies changed; review again")

type ManualPriceInput struct {
	OwnerUserID       int64
	AuthSessionID     int64
	RequestID         string
	BaseCommodityID   int64
	QuoteCommodityID  int64
	ValuationDate     string
	PriceValue        string
	PriceScale        int
	BaseQuantityValue string
	BaseQuantityScale int
	// AdjustmentBasis is raw (the default) or total_return. A correction
	// keeps the basis of the observation it replaces.
	AdjustmentBasis string
	ObservedAt      string
	Reference       string
	Reason          string
}

// manualEntryBases are the bases an owner can record by hand: raw values
// ledger units; total_return values only an R13 benchmark declared
// total-return (observationSelection never lets it value a balance).
var manualEntryBases = map[string]bool{"raw": true, "total_return": true}

type PriceImpact struct {
	Prices []PriceObservation
	Token  string
}

func priceManagementError(err error) error {
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrPriceObservationNotFound
	case errors.Is(err, db.ErrPriceObservationAlreadyVoided):
		return ErrPriceObservationAlreadyVoided
	case errors.Is(err, db.ErrPriceVoidCascadeTooDeep):
		return ErrPriceVoidCascadeTooDeep
	case errors.Is(err, db.ErrPriceImpactStale):
		return ErrPriceImpactStale
	default:
		return err
	}
}

func (s *PricingService) PreviewPriceImpact(ctx context.Context, observationID int64) (PriceImpact, error) {
	if observationID <= 0 {
		return PriceImpact{}, ValidationError{Message: "price observation id is required"}
	}
	result, err := s.repository.PriceImpact(ctx, BookID, observationID)
	if err != nil {
		return PriceImpact{}, priceManagementError(err)
	}
	return PriceImpact{Prices: toPriceObservations(result.Prices), Token: result.Token}, nil
}

func (s *PricingService) manualPriceSpec(ctx context.Context, input ManualPriceInput) (db.PriceObservationSpec, error) {
	if input.OwnerUserID <= 0 || strings.TrimSpace(input.Reason) == "" {
		return db.PriceObservationSpec{}, ValidationError{Message: "owner and reason are required"}
	}
	if input.BaseCommodityID == input.QuoteCommodityID {
		return db.PriceObservationSpec{}, ValidationError{Message: "price pair must be distinct"}
	}
	baseKind, valid, err := s.repository.PricePairBaseKind(ctx, BookID, input.BaseCommodityID, input.QuoteCommodityID)
	if err != nil {
		return db.PriceObservationSpec{}, err
	}
	if !valid {
		return db.PriceObservationSpec{}, ValidationError{Message: "price commodities are invalid"}
	}
	basis := strings.TrimSpace(input.AdjustmentBasis)
	if basis == "" {
		basis = "raw"
	}
	if basis == "total_return" && baseKind == "currency" {
		return db.PriceObservationSpec{}, ValidationError{Message: "a currency price cannot be total-return"}
	}
	price, err := priceInt64(input.PriceValue)
	if err != nil {
		return db.PriceObservationSpec{}, err
	}
	quantity, err := priceInt64(input.BaseQuantityValue)
	if err != nil {
		return db.PriceObservationSpec{}, err
	}
	reference := strings.TrimSpace(input.Reference)
	if len(reference) > 500 {
		return db.PriceObservationSpec{}, ValidationError{Message: "reference is too long"}
	}
	metadata, err := json.Marshal(map[string]string{"reference": reference})
	if err != nil {
		return db.PriceObservationSpec{}, err
	}
	sourceID, err := s.repository.ManualSourceID(ctx)
	if err != nil {
		return db.PriceObservationSpec{}, err
	}
	return cleanPriceObservationSpec(PriceObservationInput{
		BaseCommodityID: input.BaseCommodityID, QuoteCommodityID: input.QuoteCommodityID, ValuationDate: input.ValuationDate,
		PriceValue: price, PriceScale: input.PriceScale, BaseQuantityValue: quantity, BaseQuantityScale: input.BaseQuantityScale,
		SourceID: &sourceID, IsManual: true, QuoteType: "manual", AdjustmentBasis: basis, ObservedAt: input.ObservedAt,
		MetadataJSON: string(metadata), DerivationJSON: "{}", SeriesMetadataJSON: "{}",
	})
}

func (s *PricingService) CreateManualPrice(ctx context.Context, input ManualPriceInput) (PriceObservation, error) {
	if basis := strings.TrimSpace(input.AdjustmentBasis); basis != "" && !manualEntryBases[basis] {
		return PriceObservation{}, ValidationError{Message: "manual prices are raw or total-return"}
	}
	spec, err := s.manualPriceSpec(ctx, input)
	if err != nil {
		return PriceObservation{}, err
	}
	reason, err := cleanChangeReason(input.Reason, "")
	if err != nil {
		return PriceObservation{}, err
	}
	record, err := s.repository.CreatePriceObservation(ctx, db.CreatePriceObservationParams{
		BookID: BookID, ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		OriginType: "browser_api", Operation: "pricing.price.create", Spec: spec, CreatedAt: s.now().UTC().Format(time.RFC3339), ChangeReason: reason,
	})
	if err != nil {
		return PriceObservation{}, fmt.Errorf("create manual price: %w", err)
	}
	return toPriceObservation(record), nil
}

type CorrectPriceInput struct {
	ManualPriceInput
	ObservationID  int64
	ExpectedImpact string
}
type CorrectPriceResult struct {
	Replacement PriceObservation
	Retired     []PriceObservation
}

func (s *PricingService) CorrectPrice(ctx context.Context, input CorrectPriceInput) (CorrectPriceResult, error) {
	if strings.TrimSpace(input.ExpectedImpact) == "" {
		return CorrectPriceResult{}, ValidationError{Message: "reviewed price impact is required"}
	}
	impact, err := s.PreviewPriceImpact(ctx, input.ObservationID)
	if err != nil {
		return CorrectPriceResult{}, err
	}
	root := impact.Prices[0]
	if (input.BaseCommodityID != 0 && input.BaseCommodityID != root.BaseCommodityID) || (input.QuoteCommodityID != 0 && input.QuoteCommodityID != root.QuoteCommodityID) || (input.ValuationDate != "" && input.ValuationDate != root.ValuationDate) {
		return CorrectPriceResult{}, ValidationError{Message: "price correction keeps the same pair and valuation date"}
	}
	// Relabelling a basis would move the price between selections (a
	// total-return benchmark quote would start valuing ledger units), so a
	// correction is a new value on the same basis, never a new basis.
	if input.AdjustmentBasis != "" && strings.TrimSpace(input.AdjustmentBasis) != root.AdjustmentBasis {
		return CorrectPriceResult{}, ValidationError{Message: "price correction keeps the same adjustment basis"}
	}
	input.AdjustmentBasis = root.AdjustmentBasis
	input.BaseCommodityID = root.BaseCommodityID
	input.QuoteCommodityID = root.QuoteCommodityID
	input.ValuationDate = root.ValuationDate
	spec, err := s.manualPriceSpec(ctx, input.ManualPriceInput)
	if err != nil {
		return CorrectPriceResult{}, err
	}
	reason, err := cleanChangeReason(input.Reason, "")
	if err != nil {
		return CorrectPriceResult{}, err
	}
	result, err := s.repository.CorrectPrice(ctx, db.CorrectPriceParams{
		ObservationID: input.ObservationID, ExpectedImpact: input.ExpectedImpact,
		Create: db.CreatePriceObservationParams{BookID: BookID, ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
			RequestID: input.RequestID, OriginType: "browser_api", Operation: "pricing.price.correct", Spec: spec,
			CreatedAt: s.now().UTC().Format(time.RFC3339), ChangeReason: reason},
	})
	if err != nil {
		return CorrectPriceResult{}, priceManagementError(err)
	}
	return CorrectPriceResult{Replacement: toPriceObservation(result.Replacement), Retired: toPriceObservations(result.Retired)}, nil
}
