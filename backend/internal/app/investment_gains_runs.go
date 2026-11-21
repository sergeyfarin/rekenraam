package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"rekenraam/backend/internal/db"
)

var ErrGainsRunNotFound = errors.New("investment gains run not found")
var ErrGainsRunIntegrity = errors.New("investment gains run integrity check failed")

type GainsRunRequest struct {
	ReportType    string `json:"report_type"`
	From          string `json:"from,omitempty"`
	To            string `json:"to,omitempty"`
	ValuationAsOf string `json:"valuation_as_of,omitempty"`
}

type GainsRunInput struct {
	Request                    GainsRunRequest
	OwnerUserID, AuthSessionID int64
	RequestID                  string
}

// GainsRunProjection is server-computed. The formatter only supplies the
// versioned wire representation; a caller cannot submit its own report output.
type GainsRunProjection struct {
	Profile        GainsProfile
	Currencies     []Currency
	Realized       []RealizedGainEntry
	Unrealized     []UnrealizedGainEntry
	CapitalReturns []CapitalReturnRead
	Statement      *GainsStatement
}

type GainsRunFormatter func(GainsRunProjection) (json.RawMessage, error)

func (s *InvestmentService) CreateGainsRun(ctx context.Context, input GainsRunInput, format GainsRunFormatter) (db.GainsRunRecord, error) {
	request := input.Request
	if request.ReportType != "gains" && request.ReportType != "statement" {
		return db.GainsRunRecord{}, ValidationError{Message: "report type must be gains or statement"}
	}
	for _, date := range []*string{&request.From, &request.To, &request.ValuationAsOf} {
		if *date != "" {
			cleaned, err := cleanRequiredDate(*date, "report date")
			if err != nil {
				return db.GainsRunRecord{}, err
			}
			*date = cleaned
		}
	}
	if request.From != "" && request.To != "" && request.From > request.To {
		return db.GainsRunRecord{}, ValidationError{Message: "period start cannot be after period end"}
	}
	if request.ReportType == "statement" && (request.From == "" || request.To == "" || request.ValuationAsOf != "") {
		return db.GainsRunRecord{}, ValidationError{Message: "statement requires a period and uses its closing valuation"}
	}
	snapshot, fingerprint, cleanup, err := s.repository.GainsSnapshot(ctx)
	if err != nil {
		return db.GainsRunRecord{}, fmt.Errorf("capture gains snapshot: %w", err)
	}
	defer cleanup()
	captured := s.now().UTC()
	ctx, err = NewSettingsService(db.NewSettingsRepository(snapshot)).referenceContext(ctx, captured)
	if err != nil {
		return db.GainsRunRecord{}, fmt.Errorf("resolve saved gains reference day: %w", err)
	}
	reader := NewInvestmentService(db.NewInvestmentRepository(snapshot), nil, nil, nil)
	reader.consistentGainsRead = true
	reader.now = func() time.Time { return captured }
	projection := GainsRunProjection{}
	projection.Currencies, err = NewCurrencyService(db.NewCommodityRepository(snapshot), nil).ListCurrencies(ctx)
	if err != nil {
		return db.GainsRunRecord{}, err
	}
	if request.ReportType == "statement" {
		statement, err := reader.GainsStatement(ctx, request.From, request.To)
		if err != nil {
			return db.GainsRunRecord{}, err
		}
		projection.Statement, projection.Profile = &statement, statement.Profile
	} else {
		projection.Profile, projection.Unrealized, err = reader.UnrealizedGainsProjection(ctx, request.ValuationAsOf)
		if err != nil {
			return db.GainsRunRecord{}, err
		}
		request.ValuationAsOf = projection.Profile.ValuationAsOf
		period := GainsReportParams{From: request.From, To: request.To}
		projection.Realized, err = reader.ListRealizedGains(ctx, period)
		if err != nil {
			return db.GainsRunRecord{}, err
		}
		projection.CapitalReturns, err = reader.ListCapitalReturns(ctx, period)
		if err != nil {
			return db.GainsRunRecord{}, err
		}
	}
	// Pin disposal bounds in the saved descriptor, including unbounded omissions.
	projection.Profile.From, projection.Profile.To = request.From, request.To
	if projection.Statement != nil {
		projection.Statement.Profile = projection.Profile
	}
	output, err := format(projection)
	if err != nil {
		return db.GainsRunRecord{}, err
	}
	// Compact once before hashing so HTTP's RawMessage encoding and portable
	// exports retain the same document bytes.
	var compact json.RawMessage
	if !json.Valid(output) {
		return db.GainsRunRecord{}, ErrGainsRunIntegrity
	}
	compact, err = json.Marshal(output)
	if err != nil {
		return db.GainsRunRecord{}, err
	}
	inputs, err := json.Marshal(struct {
		GainsRunRequest
		SnapshotSHA256 string       `json:"snapshot_sha256"`
		Profile        GainsProfile `json:"profile"`
	}{request, fingerprint, projection.Profile})
	if err != nil {
		return db.GainsRunRecord{}, err
	}
	// Statement schema 2 added capital-return disclosure (#238); schema 3
	// adds unclassified imported dividend cash beside income (#246).
	schemaVersion := 1
	if request.ReportType == "statement" {
		schemaVersion = 3
	}
	run := db.GainsRunRecord{BookID: BookID, SchemaVersion: schemaVersion, ReportType: request.ReportType,
		CapturedAt: captured.Format(time.RFC3339Nano), CreatedAt: s.now().UTC().Format(time.RFC3339),
		CreatedByUserID: input.OwnerUserID, SnapshotSHA256: fingerprint,
		InputsJSON: inputs, OutputJSON: compact, InputsSHA256: db.GainsRunDigest(inputs), OutputSHA256: db.GainsRunDigest(compact)}
	if !db.ValidGainsRunDocument(run) {
		return db.GainsRunRecord{}, ErrGainsRunIntegrity
	}
	return s.repository.CreateGainsRun(ctx, run, db.AuditEventParams{BookID: BookID, ActorUserID: input.OwnerUserID,
		AuthSessionID: input.AuthSessionID, RequestID: input.RequestID, OriginType: "browser_api", OccurredAt: run.CreatedAt})
}

func (s *InvestmentService) GainsRun(ctx context.Context, id int64) (db.GainsRunRecord, error) {
	run, err := s.repository.GainsRun(ctx, BookID, id)
	if errors.Is(err, db.ErrNotFound) {
		return run, ErrGainsRunNotFound
	}
	if err == nil && !db.ValidGainsRunDocument(run) {
		return db.GainsRunRecord{}, ErrGainsRunIntegrity
	}
	return run, err
}

func (s *InvestmentService) ListGainsRuns(ctx context.Context, before int64) ([]db.GainsRunRecord, int64, error) {
	if before < 0 {
		return nil, 0, ValidationError{Message: "invalid gains run cursor"}
	}
	runs, err := s.repository.ListGainsRuns(ctx, BookID, before, 21)
	if err != nil {
		return nil, 0, err
	}
	for _, run := range runs {
		if db.GainsRunDigest(run.InputsJSON) != run.InputsSHA256 {
			return nil, 0, ErrGainsRunIntegrity
		}
	}
	var next int64
	if len(runs) > 20 {
		runs = runs[:20]
		next = runs[19].ID
	}
	return runs, next, nil
}
