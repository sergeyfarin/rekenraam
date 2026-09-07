package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

const (
	forecastDefaultHorizonDays = 90
	forecastMaxHorizonDays     = 366
	forecastMaxRootAccounts    = 100
	forecastMaxAccounts        = 200
	forecastMaxCandidates      = 20000
	forecastMaxOutputPoints    = 200000
	forecastMaxDiagnostics     = 50
)

var ErrForecastTooLarge = errors.New("forecast exceeds a configured limit")

type ForecastInput struct {
	OwnerUserID        int64
	HorizonDays        int
	AccountIDs         []int64
	IncludeDescendants bool
}

type ForecastQuantity struct {
	Value exact.Coefficient
	Scale int
}

type ForecastAccount struct {
	ID                 int64
	Name               string
	AccountClass       string
	AccountKind        string
	Status             string
	ParentAccountID    *int64
	DefaultCommodityID *int64
	AllowsPostings     bool
}

type ForecastCommodity struct {
	ID            int64
	Code          string
	StandardScale int
}

type ForecastComponents struct {
	Posted   ForecastQuantity
	Draft    ForecastQuantity
	Template ForecastQuantity
}

type ForecastPoint struct {
	Date             string
	Components       ForecastComponents
	RecordedBalance  ForecastQuantity
	ProjectedBalance ForecastQuantity
}

type ForecastSeries struct {
	AccountID         int64
	CommodityID       int64
	Opening           ForecastQuantity
	Points            []ForecastPoint
	Minimum           ForecastQuantity
	MinimumDate       string
	FirstNegativeDate string
}

type ForecastEventAmount struct {
	AccountID   int64
	CommodityID int64
	Quantity    ForecastQuantity
}

type ForecastEvent struct {
	Key            string
	Source         string
	SourceID       int64
	EntryID        int64
	TemplateID     int64
	OccurrenceID   int64
	OccurrenceDate string
	SourceDate     string
	ProjectedDate  string
	CarriedForward bool
	PayeeName      string
	Amounts        []ForecastEventAmount
}

type ForecastDiagnostic struct {
	Code           string
	TemplateID     int64
	OccurrenceID   int64
	OccurrenceDate string
	SourceDate     string
}

type ForecastAssumptions struct {
	Complete       bool
	CarriedForward int
	Excluded       int
	Total          int
	Hidden         int
	Diagnostics    []ForecastDiagnostic
}

type ForecastSourceCounts struct{ Posted, Draft, Template int }

type ForecastResult struct {
	AsOfDate     string
	FirstDate    string
	ThroughDate  string
	ComputedAt   string
	BasisToken   string
	AccountIDs   []int64
	Accounts     []ForecastAccount
	Commodities  []ForecastCommodity
	Series       []ForecastSeries
	Aggregates   []ForecastSeries
	Events       []ForecastEvent
	SourceCounts ForecastSourceCounts
	Assumptions  ForecastAssumptions
}

type ForecastService struct {
	repository *db.ForecastRepository
	now        func() time.Time
}

func NewForecastService(repository *db.ForecastRepository) *ForecastService {
	return &ForecastService{repository: repository, now: time.Now}
}

func (s *ForecastService) SetNowForTest(now func() time.Time) { s.now = now }

func (s *ForecastService) Balances(ctx context.Context, input ForecastInput) (ForecastResult, error) {
	if input.OwnerUserID <= 0 {
		return ForecastResult{}, ValidationError{Message: "owner user is required"}
	}
	normalized, err := normalizeForecastInput(input)
	if err != nil {
		return ForecastResult{}, err
	}
	nowUTC := s.now().UTC()
	var scope forecastScope
	var bounds forecastBounds
	snapshot, err := s.repository.LoadResolvedSnapshot(ctx, db.ForecastSnapshotRequest{
		BookID: BookID, OwnerUserID: input.OwnerUserID,
		Limits: db.DefaultForecastSnapshotLimits(),
	}, func(base db.ForecastSnapshot) (db.ForecastSnapshotResolution, error) {
		bounds, err = forecastDateBounds(nowUTC, base.TimeZone, normalized.HorizonDays)
		if err != nil {
			return db.ForecastSnapshotResolution{}, err
		}
		scope, err = resolveForecastScope(base.AccountVersions, bounds.AsOf, normalized)
		return db.ForecastSnapshotResolution{AccountIDs: scope.AccountIDs, ThroughDate: bounds.Through}, err
	})
	if err != nil {
		if errors.Is(err, db.ErrForecastInputTooLarge) {
			return ForecastResult{}, ErrForecastTooLarge
		}
		return ForecastResult{}, err
	}
	result, err := buildForecast(ctx, normalized, bounds, scope, snapshot, nowUTC.Format(time.RFC3339))
	if err != nil {
		return ForecastResult{}, err
	}
	digestInput := struct {
		Input    forecastNormalizedInput
		Bounds   forecastBounds
		Scope    []int64
		Snapshot db.ForecastSnapshot
	}{normalized, bounds, scope.AccountIDs, snapshot}
	encoded, err := json.Marshal(digestInput)
	if err != nil {
		return ForecastResult{}, fmt.Errorf("encode forecast basis: %w", err)
	}
	hash := sha256.Sum256(encoded)
	result.BasisToken = hex.EncodeToString(hash[:])
	return result, nil
}
