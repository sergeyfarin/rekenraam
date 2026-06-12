package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
)

var (
	ErrPriceObservationNotFound  = errors.New("price observation not found")
	ErrPricingPolicyNotFound     = errors.New("pricing policy not found")
	ErrPricingAssignmentNotFound = errors.New("pricing source assignment not found")
)

type MarketDataSource struct {
	ID           int64
	Code         string
	Name         string
	Kind         string
	ProviderKey  string
	BaseURL      string
	Status       string
	MetadataJSON string
	CreatedAt    string
}

type PriceObservation struct {
	ID                      int64
	BookID                  int64
	CommodityID             int64
	QuoteCommodityID        int64
	ObservationKind         string
	PriceValue              int64
	PriceScale              int
	PriceDate               string
	SourceID                *int64
	ProviderObservationID   string
	Mode                    string
	IsManual                bool
	IsDerived               bool
	SupersedesObservationID *int64
	MetadataJSON            string
	CreatedAt               string
}

type PriceObservationInput struct {
	OwnerUserID             int64
	AuthSessionID           int64
	RequestID               string
	CommodityID             int64
	QuoteCommodityID        int64
	ObservationKind         string
	PriceValue              int64
	PriceScale              int
	PriceDate               string
	SourceID                *int64
	ProviderObservationID   string
	Mode                    string
	IsManual                bool
	IsDerived               bool
	SupersedesObservationID *int64
	MetadataJSON            string
	ChangeReason            string
}

type CreateTradeImpliedPriceInput struct {
	OwnerUserID      int64
	AuthSessionID    int64
	RequestID        string
	CommodityID      int64
	QuoteCommodityID int64
	PriceDate        string
	QuantityValue    int64
	QuantityScale    int
	CashValue        int64
	CashScale        int
	PriceScale       int
}

type PricingPolicy struct {
	ID                   int64
	BookID               int64
	BaseCommodityID      *int64
	DefaultSourceID      *int64
	RefreshEnabled       bool
	RefreshHourUTC       int
	RefreshMinuteUTC     int
	MaxBackfillDays      int
	StalenessMaxDays     int
	TriangulationMaxHops int
	RoundingMode         string
	PreferOfficialFX     bool
	WeekendPolicy        string
	CreatedAt            string
	UpdatedAt            string
}

type PricingPolicyInput struct {
	OwnerUserID          int64
	AuthSessionID        int64
	RequestID            string
	BaseCommodityID      *int64
	DefaultSourceID      *int64
	RefreshEnabled       bool
	RefreshHourUTC       int
	RefreshMinuteUTC     int
	MaxBackfillDays      int
	StalenessMaxDays     int
	TriangulationMaxHops int
	RoundingMode         string
	PreferOfficialFX     bool
	WeekendPolicy        string
	ChangeReason         string
}

type PricingSourceAssignment struct {
	ID               int64
	BookID           int64
	CommodityID      int64
	QuoteCommodityID int64
	SourceID         int64
	Priority         int
	Status           string
	EffectiveFrom    string
	EffectiveTo      string
	CreatedAt        string
}

type PricingSourceAssignmentInput struct {
	OwnerUserID      int64
	AuthSessionID    int64
	RequestID        string
	AssignmentID     int64
	CommodityID      int64
	QuoteCommodityID int64
	SourceID         int64
	Priority         int
	Status           string
	EffectiveFrom    string
	EffectiveTo      string
	ChangeReason     string
}

type PricingRefreshRun struct {
	ID             int64
	BookID         int64
	SourceID       int64
	Trigger        string
	Status         string
	StartedAt      string
	FinishedAt     string
	ItemsTotal     int
	ItemsSucceeded int
	ItemsFailed    int
	LastError      string
}

type PricingSourceHealth struct {
	ID               int64
	BookID           int64
	CommodityID      int64
	QuoteCommodityID int64
	SourceID         int64
	LastSuccessDate  string
	LastAttemptAt    string
	LastError        string
	UpdatedAt        string
	Status           string
}

type PricingService struct {
	repository *db.PricingRepository
	now        func() time.Time
}

func NewPricingService(repository *db.PricingRepository) *PricingService {
	return &PricingService{repository: repository, now: time.Now}
}

func (s *PricingService) SetNowForTest(now func() time.Time) {
	s.now = now
}

func (s *PricingService) ListSources(ctx context.Context) ([]MarketDataSource, error) {
	records, err := s.repository.ListSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("list market data sources: %w", err)
	}
	return toMarketDataSources(records), nil
}

func (s *PricingService) ListPrices(ctx context.Context, commodityID int64, quoteCommodityID int64, limit int) ([]PriceObservation, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	records, err := s.repository.ListPriceObservations(ctx, BookID, commodityID, quoteCommodityID, limit)
	if err != nil {
		return nil, fmt.Errorf("list price observations: %w", err)
	}
	return toPriceObservations(records), nil
}

func (s *PricingService) CreatePrice(ctx context.Context, input PriceObservationInput) (PriceObservation, error) {
	if input.OwnerUserID <= 0 {
		return PriceObservation{}, ValidationError{Message: "owner user is required"}
	}
	spec, err := cleanPriceObservationSpec(input)
	if err != nil {
		return PriceObservation{}, err
	}
	now := s.now().UTC()
	changeReason, err := cleanChangeReason(input.ChangeReason, "created price observation")
	if err != nil {
		return PriceObservation{}, err
	}
	record, err := s.repository.CreatePriceObservation(ctx, db.CreatePriceObservationParams{
		BookID:        BookID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    "browser_api",
		Operation:     "pricing.price.create",
		Spec:          spec,
		CreatedAt:     now.Format(time.RFC3339),
		ChangeReason:  changeReason,
	})
	if err != nil {
		return PriceObservation{}, fmt.Errorf("persist price observation: %w", err)
	}
	return toPriceObservation(record), nil
}

func (s *PricingService) CreateTradeImpliedPrice(ctx context.Context, input CreateTradeImpliedPriceInput) error {
	if input.QuantityValue <= 0 || input.CashValue <= 0 {
		return ValidationError{Message: "trade quantity and cash amount are required"}
	}
	priceScale := input.PriceScale
	if priceScale < 0 || priceScale > 12 {
		priceScale = 8
	}
	priceValue, err := scaledDivision(input.CashValue, input.CashScale, input.QuantityValue, input.QuantityScale, priceScale)
	if err != nil {
		return err
	}
	sourceID, err := s.repository.ManualSourceID(ctx)
	if err != nil {
		return err
	}
	_, err = s.repository.CreatePriceObservation(ctx, db.CreatePriceObservationParams{
		BookID:        BookID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    "browser_api",
		Operation:     "pricing.price.trade_implied",
		CreatedAt:     s.now().UTC().Format(time.RFC3339),
		ChangeReason:  "created trade-implied price observation",
		Spec: db.PriceObservationSpec{
			CommodityID:      input.CommodityID,
			QuoteCommodityID: input.QuoteCommodityID,
			ObservationKind:  "trade_implied",
			PriceValue:       priceValue,
			PriceScale:       priceScale,
			PriceDate:        input.PriceDate,
			SourceID:         sql.NullInt64{Int64: sourceID, Valid: true},
			Mode:             "transaction_implied",
			IsManual:         false,
			IsDerived:        true,
			MetadataJSON:     "{}",
		},
	})
	return err
}

func (s *PricingService) GetPolicy(ctx context.Context) (PricingPolicy, error) {
	record, err := s.repository.GetPricingPolicy(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return PricingPolicy{}, ErrPricingPolicyNotFound
		}
		return PricingPolicy{}, fmt.Errorf("read pricing policy: %w", err)
	}
	return toPricingPolicy(record), nil
}

func (s *PricingService) SavePolicy(ctx context.Context, input PricingPolicyInput) (PricingPolicy, error) {
	if input.OwnerUserID <= 0 {
		return PricingPolicy{}, ValidationError{Message: "owner user is required"}
	}
	spec, err := cleanPricingPolicySpec(input)
	if err != nil {
		return PricingPolicy{}, err
	}
	now := s.now().UTC()
	changeReason, err := cleanChangeReason(input.ChangeReason, "saved pricing policy")
	if err != nil {
		return PricingPolicy{}, err
	}
	record, err := s.repository.SavePricingPolicy(ctx, db.SavePricingPolicyParams{
		BookID:        BookID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    "browser_api",
		Operation:     "pricing.policy.save",
		Spec:          spec,
		RecordedAt:    now.Format(time.RFC3339),
		ChangeReason:  changeReason,
	})
	if err != nil {
		return PricingPolicy{}, fmt.Errorf("save pricing policy: %w", err)
	}
	return toPricingPolicy(record), nil
}

func (s *PricingService) ListSourceAssignments(ctx context.Context) ([]PricingSourceAssignment, error) {
	records, err := s.repository.ListSourceAssignments(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list pricing source assignments: %w", err)
	}
	return toPricingSourceAssignments(records), nil
}

func (s *PricingService) SaveSourceAssignment(ctx context.Context, input PricingSourceAssignmentInput) (PricingSourceAssignment, error) {
	if input.OwnerUserID <= 0 {
		return PricingSourceAssignment{}, ValidationError{Message: "owner user is required"}
	}
	spec, err := cleanPricingSourceAssignmentSpec(input, s.now().UTC())
	if err != nil {
		return PricingSourceAssignment{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "saved pricing source assignment")
	if err != nil {
		return PricingSourceAssignment{}, err
	}
	record, err := s.repository.SaveSourceAssignment(ctx, db.SavePricingSourceAssignmentParams{
		BookID:        BookID,
		AssignmentID:  input.AssignmentID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    "browser_api",
		Operation:     sourceAssignmentOperation(input.AssignmentID),
		Spec:          spec,
		RecordedAt:    s.now().UTC().Format(time.RFC3339),
		ChangeReason:  changeReason,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return PricingSourceAssignment{}, ErrPricingAssignmentNotFound
		}
		return PricingSourceAssignment{}, fmt.Errorf("save pricing source assignment: %w", err)
	}
	return toPricingSourceAssignment(record), nil
}

func (s *PricingService) RunRefresh(ctx context.Context, ownerUserID int64, sourceID int64, trigger string) (PricingRefreshRun, error) {
	if ownerUserID <= 0 {
		return PricingRefreshRun{}, ValidationError{Message: "owner user is required"}
	}
	if sourceID <= 0 {
		var err error
		sourceID, err = s.repository.ManualSourceID(ctx)
		if err != nil {
			return PricingRefreshRun{}, err
		}
	}
	trigger = strings.TrimSpace(trigger)
	if trigger == "" {
		trigger = "manual"
	}
	if trigger != "manual" && trigger != "scheduled" && trigger != "import" {
		return PricingRefreshRun{}, ValidationError{Message: "refresh trigger is invalid"}
	}
	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.RecordRefreshRun(ctx, BookID, sourceID, trigger, "succeeded", now, now, 0, 0, 0, "")
	if err != nil {
		return PricingRefreshRun{}, fmt.Errorf("record pricing refresh run: %w", err)
	}
	return toPricingRefreshRun(record), nil
}

func (s *PricingService) ListRefreshRuns(ctx context.Context, limit int) ([]PricingRefreshRun, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	records, err := s.repository.ListRefreshRuns(ctx, BookID, limit)
	if err != nil {
		return nil, fmt.Errorf("list pricing refresh runs: %w", err)
	}
	return toPricingRefreshRuns(records), nil
}

func (s *PricingService) SourceHealth(ctx context.Context) ([]PricingSourceHealth, error) {
	records, err := s.repository.ListRefreshState(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list pricing source health: %w", err)
	}
	health := make([]PricingSourceHealth, 0, len(records))
	for _, record := range records {
		status := "ok"
		if record.LastError.Valid && strings.TrimSpace(record.LastError.String) != "" {
			status = "error"
		} else if !record.LastSuccessDate.Valid {
			status = "never_run"
		}
		health = append(health, PricingSourceHealth{
			ID:               record.ID,
			BookID:           record.BookID,
			CommodityID:      record.CommodityID,
			QuoteCommodityID: record.QuoteCommodityID,
			SourceID:         record.SourceID,
			LastSuccessDate:  nullableString(record.LastSuccessDate),
			LastAttemptAt:    nullableString(record.LastAttemptAt),
			LastError:        nullableString(record.LastError),
			UpdatedAt:        record.UpdatedAt,
			Status:           status,
		})
	}
	return health, nil
}

func cleanPriceObservationSpec(input PriceObservationInput) (db.PriceObservationSpec, error) {
	if input.CommodityID <= 0 || input.QuoteCommodityID <= 0 {
		return db.PriceObservationSpec{}, ValidationError{Message: "price commodities are required"}
	}
	if input.PriceValue <= 0 {
		return db.PriceObservationSpec{}, ValidationError{Message: "price value is required"}
	}
	if input.PriceScale < 0 || input.PriceScale > 12 {
		return db.PriceObservationSpec{}, ValidationError{Message: "price scale is invalid"}
	}
	priceDate, err := cleanRequiredDate(input.PriceDate, "price date")
	if err != nil {
		return db.PriceObservationSpec{}, err
	}
	kind := strings.TrimSpace(input.ObservationKind)
	if kind == "" {
		kind = "market_price"
	}
	if !map[string]bool{"market_price": true, "fx_daily": true, "fx_official": true, "trade_implied": true, "valuation_override": true}[kind] {
		return db.PriceObservationSpec{}, ValidationError{Message: "price observation kind is invalid"}
	}
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "manual"
	}
	if !map[string]bool{"market": true, "daily": true, "official": true, "transaction_implied": true, "manual": true}[mode] {
		return db.PriceObservationSpec{}, ValidationError{Message: "price mode is invalid"}
	}
	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "metadata", investmentJSONMaxBytes)
	if err != nil {
		return db.PriceObservationSpec{}, err
	}
	return db.PriceObservationSpec{
		CommodityID:             input.CommodityID,
		QuoteCommodityID:        input.QuoteCommodityID,
		ObservationKind:         kind,
		PriceValue:              input.PriceValue,
		PriceScale:              input.PriceScale,
		PriceDate:               priceDate,
		SourceID:                nullableInt64(input.SourceID),
		ProviderObservationID:   strings.TrimSpace(input.ProviderObservationID),
		Mode:                    mode,
		IsManual:                input.IsManual,
		IsDerived:               input.IsDerived,
		SupersedesObservationID: nullableInt64(input.SupersedesObservationID),
		MetadataJSON:            metadataJSON,
	}, nil
}

func cleanPricingPolicySpec(input PricingPolicyInput) (db.PricingPolicySpec, error) {
	if input.RefreshHourUTC < 0 || input.RefreshHourUTC > 23 {
		return db.PricingPolicySpec{}, ValidationError{Message: "refresh hour is invalid"}
	}
	if input.RefreshMinuteUTC < 0 || input.RefreshMinuteUTC > 59 {
		return db.PricingPolicySpec{}, ValidationError{Message: "refresh minute is invalid"}
	}
	if input.MaxBackfillDays <= 0 {
		input.MaxBackfillDays = 370
	}
	if input.StalenessMaxDays <= 0 {
		input.StalenessMaxDays = 3
	}
	if input.TriangulationMaxHops < 0 || input.TriangulationMaxHops > 4 {
		return db.PricingPolicySpec{}, ValidationError{Message: "triangulation max hops is invalid"}
	}
	roundingMode := strings.TrimSpace(input.RoundingMode)
	if roundingMode == "" {
		roundingMode = "half_up"
	}
	if !map[string]bool{"half_up": true, "half_even": true, "down": true, "up": true}[roundingMode] {
		return db.PricingPolicySpec{}, ValidationError{Message: "rounding mode is invalid"}
	}
	weekendPolicy := strings.TrimSpace(input.WeekendPolicy)
	if weekendPolicy == "" {
		weekendPolicy = "skip"
	}
	if !map[string]bool{"skip": true, "fill_previous": true, "download": true}[weekendPolicy] {
		return db.PricingPolicySpec{}, ValidationError{Message: "weekend policy is invalid"}
	}
	return db.PricingPolicySpec{
		BaseCommodityID:      nullableInt64(input.BaseCommodityID),
		DefaultSourceID:      nullableInt64(input.DefaultSourceID),
		RefreshEnabled:       input.RefreshEnabled,
		RefreshHourUTC:       input.RefreshHourUTC,
		RefreshMinuteUTC:     input.RefreshMinuteUTC,
		MaxBackfillDays:      input.MaxBackfillDays,
		StalenessMaxDays:     input.StalenessMaxDays,
		TriangulationMaxHops: input.TriangulationMaxHops,
		RoundingMode:         roundingMode,
		PreferOfficialFX:     input.PreferOfficialFX,
		WeekendPolicy:        weekendPolicy,
	}, nil
}

func cleanPricingSourceAssignmentSpec(input PricingSourceAssignmentInput, now time.Time) (db.PricingSourceAssignmentSpec, error) {
	if input.CommodityID <= 0 || input.QuoteCommodityID <= 0 || input.SourceID <= 0 {
		return db.PricingSourceAssignmentSpec{}, ValidationError{Message: "source assignment references are required"}
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "archived" {
		return db.PricingSourceAssignmentSpec{}, ValidationError{Message: "source assignment status is invalid"}
	}
	priority := input.Priority
	if priority < 0 {
		return db.PricingSourceAssignmentSpec{}, ValidationError{Message: "priority is invalid"}
	}
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return db.PricingSourceAssignmentSpec{}, err
	}
	effectiveTo, err := cleanOptionalDate(input.EffectiveTo, "effective to")
	if err != nil {
		return db.PricingSourceAssignmentSpec{}, err
	}
	if effectiveTo != "" && effectiveTo < effectiveFrom {
		return db.PricingSourceAssignmentSpec{}, ValidationError{Message: "effective to must be on or after effective from"}
	}
	return db.PricingSourceAssignmentSpec{
		CommodityID:      input.CommodityID,
		QuoteCommodityID: input.QuoteCommodityID,
		SourceID:         input.SourceID,
		Priority:         priority,
		Status:           status,
		EffectiveFrom:    effectiveFrom,
		EffectiveTo:      nullableSQLString(effectiveTo),
	}, nil
}

func toMarketDataSource(record db.MarketDataSourceRecord) MarketDataSource {
	return MarketDataSource{ID: record.ID, Code: record.Code, Name: record.Name, Kind: record.Kind, ProviderKey: nullableString(record.ProviderKey), BaseURL: nullableString(record.BaseURL), Status: record.Status, MetadataJSON: record.MetadataJSON, CreatedAt: record.CreatedAt}
}

func toMarketDataSources(records []db.MarketDataSourceRecord) []MarketDataSource {
	sources := make([]MarketDataSource, 0, len(records))
	for _, record := range records {
		sources = append(sources, toMarketDataSource(record))
	}
	return sources
}

func toPriceObservation(record db.PriceObservationRecord) PriceObservation {
	return PriceObservation{
		ID:                      record.ID,
		BookID:                  record.BookID,
		CommodityID:             record.CommodityID,
		QuoteCommodityID:        record.QuoteCommodityID,
		ObservationKind:         record.ObservationKind,
		PriceValue:              record.PriceValue,
		PriceScale:              record.PriceScale,
		PriceDate:               record.PriceDate,
		SourceID:                nullableSQLInt64Ptr(record.SourceID),
		ProviderObservationID:   nullableString(record.ProviderObservationID),
		Mode:                    record.Mode,
		IsManual:                record.IsManual,
		IsDerived:               record.IsDerived,
		SupersedesObservationID: nullableSQLInt64Ptr(record.SupersedesObservationID),
		MetadataJSON:            record.MetadataJSON,
		CreatedAt:               record.CreatedAt,
	}
}

func toPriceObservations(records []db.PriceObservationRecord) []PriceObservation {
	observations := make([]PriceObservation, 0, len(records))
	for _, record := range records {
		observations = append(observations, toPriceObservation(record))
	}
	return observations
}

func toPricingPolicy(record db.PricingPolicyRecord) PricingPolicy {
	return PricingPolicy{ID: record.ID, BookID: record.BookID, BaseCommodityID: nullableSQLInt64Ptr(record.BaseCommodityID), DefaultSourceID: nullableSQLInt64Ptr(record.DefaultSourceID), RefreshEnabled: record.RefreshEnabled, RefreshHourUTC: record.RefreshHourUTC, RefreshMinuteUTC: record.RefreshMinuteUTC, MaxBackfillDays: record.MaxBackfillDays, StalenessMaxDays: record.StalenessMaxDays, TriangulationMaxHops: record.TriangulationMaxHops, RoundingMode: record.RoundingMode, PreferOfficialFX: record.PreferOfficialFX, WeekendPolicy: record.WeekendPolicy, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

func toPricingSourceAssignment(record db.PricingSourceAssignmentRecord) PricingSourceAssignment {
	return PricingSourceAssignment{ID: record.ID, BookID: record.BookID, CommodityID: record.CommodityID, QuoteCommodityID: record.QuoteCommodityID, SourceID: record.SourceID, Priority: record.Priority, Status: record.Status, EffectiveFrom: record.EffectiveFrom, EffectiveTo: nullableString(record.EffectiveTo), CreatedAt: record.CreatedAt}
}

func toPricingSourceAssignments(records []db.PricingSourceAssignmentRecord) []PricingSourceAssignment {
	assignments := make([]PricingSourceAssignment, 0, len(records))
	for _, record := range records {
		assignments = append(assignments, toPricingSourceAssignment(record))
	}
	return assignments
}

func toPricingRefreshRun(record db.PricingRefreshRunRecord) PricingRefreshRun {
	return PricingRefreshRun{ID: record.ID, BookID: record.BookID, SourceID: record.SourceID, Trigger: record.Trigger, Status: record.Status, StartedAt: record.StartedAt, FinishedAt: nullableString(record.FinishedAt), ItemsTotal: record.ItemsTotal, ItemsSucceeded: record.ItemsSucceeded, ItemsFailed: record.ItemsFailed, LastError: nullableString(record.LastError)}
}

func toPricingRefreshRuns(records []db.PricingRefreshRunRecord) []PricingRefreshRun {
	runs := make([]PricingRefreshRun, 0, len(records))
	for _, record := range records {
		runs = append(runs, toPricingRefreshRun(record))
	}
	return runs
}

func sourceAssignmentOperation(assignmentID int64) string {
	if assignmentID > 0 {
		return "pricing.source_assignment.update"
	}
	return "pricing.source_assignment.create"
}
