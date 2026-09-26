package db

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"rekenraam/backend/internal/exact"
)

var (
	ErrInvestmentInstrumentExists = errors.New("investment instrument already exists")
	ErrCostBasisProfileExists     = errors.New("cost basis profile already exists")
	ErrDividendDefaultExists      = errors.New("dividend default already exists")
	ErrInsufficientLots           = errors.New("insufficient lots")
	ErrInvestmentBasisRange       = errors.New("investment position basis exceeds supported exact range")
	ErrInvalidDisposalParams      = errors.New("invalid disposal parameters")
	ErrEventSuggestionNotPending  = errors.New("investment event suggestion is not pending")
	// ErrOutOfOrderPositionEvent reports a lot event dated before something the
	// position has already recorded. See requirePositionEventInOrderTx.
	ErrOutOfOrderPositionEvent = errors.New("investment events must be entered in chronological order")
)

type InvestmentRepository struct {
	database *sql.DB
}

type InvestmentInstrumentRecord struct {
	ID                 int64
	BookID             int64
	CommodityID        int64
	CommodityCode      string
	IsBuiltin          bool
	CreatedAt          string
	CreatedByUserID    int64
	VersionID          int64
	VersionSeq         int64
	EffectiveFrom      string
	RecordedAt         string
	ChangedByUserID    int64
	ChangeReason       string
	Status             string
	InstrumentType     string
	DisplayName        string
	Symbol             sql.NullString
	ExchangeCode       sql.NullString
	MIC                sql.NullString
	Issuer             sql.NullString
	CountryCode        sql.NullString
	QuoteCommodityID   sql.NullInt64
	TradingCommodityID sql.NullInt64
	QuantityScale      int
	PriceScale         int
	IdentifiersJSON    string
	MetadataJSON       string
}

type InvestmentInstrumentSpec struct {
	CommodityID        sql.NullInt64
	CommodityCode      string
	InstrumentType     string
	DisplayName        string
	Symbol             string
	ExchangeCode       string
	MIC                string
	Issuer             string
	CountryCode        string
	QuoteCommodityID   sql.NullInt64
	TradingCommodityID sql.NullInt64
	QuantityScale      int
	PriceScale         int
	IdentifiersJSON    string
	MetadataJSON       string
}

type CreateInvestmentInstrumentParams struct {
	BookID          int64
	CreatedByUserID int64
	AuthSessionID   int64
	RequestID       string
	OriginType      string
	Operation       string
	Spec            InvestmentInstrumentSpec
	CreatedAt       string
	EffectiveFrom   string
	ChangeReason    string
}

type UpdateInvestmentInstrumentParams struct {
	BookID          int64
	InstrumentID    int64
	ChangedByUserID int64
	AuthSessionID   int64
	RequestID       string
	OriginType      string
	Operation       string
	Spec            InvestmentInstrumentSpec
	RecordedAt      string
	EffectiveFrom   string
	ChangeReason    string
}

type CostBasisProfileRecord struct {
	ID              int64
	BookID          int64
	Name            string
	Method          string
	IsDefault       bool
	Status          string
	Description     string
	MetadataJSON    string
	CreatedAt       string
	CreatedByUserID int64
	UpdatedAt       string
	UpdatedByUserID int64
	VersionID       int64
	VersionSeq      int64
}

type CostBasisProfileSpec struct {
	Name         string
	Method       string
	IsDefault    bool
	Status       string
	Description  string
	MetadataJSON string
}

type SaveCostBasisProfileParams struct {
	BookID        int64
	ProfileID     int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Spec          CostBasisProfileSpec
	RecordedAt    string
	ChangeReason  string
}

type DividendDefaultRecord struct {
	ID                      int64
	BookID                  int64
	CommodityID             sql.NullInt64
	IncomeAccountID         int64
	WithholdingAccountID    sql.NullInt64
	DefaultWithholdingValue sql.NullInt64
	DefaultWithholdingScale sql.NullInt64
	WithholdingRateBPS      sql.NullInt64
	TaxCountryCode          sql.NullString
	TaxTreatment            sql.NullString
	Status                  string
	EffectiveFrom           string
	EffectiveTo             sql.NullString
	MetadataJSON            string
	CreatedAt               string
	CreatedByUserID         int64
	UpdatedAt               string
	UpdatedByUserID         int64
}

type DividendDefaultSpec struct {
	CommodityID             sql.NullInt64
	IncomeAccountID         int64
	WithholdingAccountID    sql.NullInt64
	DefaultWithholdingValue sql.NullInt64
	DefaultWithholdingScale sql.NullInt64
	WithholdingRateBPS      sql.NullInt64
	TaxCountryCode          string
	TaxTreatment            string
	Status                  string
	EffectiveFrom           string
	EffectiveTo             sql.NullString
	MetadataJSON            string
}

type SaveDividendDefaultParams struct {
	BookID        int64
	DefaultID     int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Spec          DividendDefaultSpec
	RecordedAt    string
	ChangeReason  string
}

type InvestmentLotRecord struct {
	ID                      int64
	BookID                  int64
	AccountID               int64
	CommodityID             int64
	OpenedOn                string
	SourceTransactionID     sql.NullInt64
	Status                  string
	QuantityValue           exact.Coefficient
	QuantityScale           int
	RemainingQuantityValue  exact.Coefficient
	RemainingQuantityScale  int
	CostBasisValue          int64
	CostBasisScale          int
	RemainingCostBasisValue int64
	RemainingCostBasisScale int
	CostCommodityID         int64
	MetadataJSON            string
	CreatedAt               string
	UpdatedAt               string
}

type CreateInvestmentLotParams struct {
	BookID              int64
	AccountID           int64
	CommodityID         int64
	OpenedOn            string
	SourceTransactionID int64
	QuantityValue       exact.Coefficient
	QuantityScale       int
	CostBasisValue      int64
	CostBasisScale      int
	CostCommodityID     int64
	MetadataJSON        string
	CreatedAt           string
	CreatedByUserID     int64
	AuthSessionID       int64
	RequestID           string
	OriginType          string
	Operation           string
	ChangeReason        string
	EventKind           string
}

type LotAllocation struct {
	LotID         int64
	QuantityValue exact.Coefficient
	QuantityScale int
}

type LotDisposalRecord struct {
	EventID         int64
	LotID           int64
	QuantityValue   exact.Coefficient
	QuantityScale   int
	CostBasisValue  int64
	CostBasisScale  int
	CostCommodityID int64
}

type DisposalDecisionSource struct {
	ResolutionTier      string
	AccountVersionID    int64
	ProfileID           int64
	ProfileVersionID    int64
	SourceEffectiveFrom string
	SourceRecordedAt    string
}

type DisposalDecisionRecord struct {
	ID                   int64
	TransactionID        int64
	TransactionVersionID int64
	AccountID            int64
	CommodityID          int64
	CostCommodityID      int64
	EventDate            string
	QuantityValue        exact.Coefficient
	QuantityScale        int
	DisposedBasisValue   exact.Coefficient
	DisposedBasisScale   int
	CostBasisMethod      string
	DisposalDecisionSource
	CreatedAt    string
	AuditEventID int64
	Allocations  []LotDisposalRecord
}

type DisposeLotsParams struct {
	BookID          int64
	AccountID       int64
	CommodityID     int64
	CostCommodityID int64
	TransactionID   int64
	EventDate       string
	QuantityValue   exact.Coefficient
	QuantityScale   int
	Allocations     []LotAllocation
	CostBasisMethod string
	DecisionSource  DisposalDecisionSource
	CreatedAt       string
	ActorUserID     int64
	AuthSessionID   int64
	RequestID       string
	OriginType      string
	Operation       string
	ChangeReason    string
	MetadataJSON    string
}

type InvestmentPositionRecord struct {
	AccountID               int64
	CommodityID             int64
	QuantityValue           exact.Coefficient
	QuantityScale           int
	RemainingCostBasisValue int64
	RemainingCostBasisScale int
	CostCommodityID         int64
	LatestPriceValue        sql.NullInt64
	LatestPriceScale        sql.NullInt64
	LatestPriceDate         sql.NullString
	// base_quantity fields from price_observations: price is (PriceValue/base_quantity) per unit.
	// Defaults: base_quantity_value=1, base_quantity_scale=0 (price is per 1 unit).
	LatestPriceBaseQuantityValue sql.NullInt64
	LatestPriceBaseQuantityScale sql.NullInt64
}

type InvestmentProviderEventRecord struct {
	ID              int64
	BookID          int64
	SourceID        int64
	ProviderEventID sql.NullString
	InstrumentID    sql.NullInt64
	EventFamily     string
	EventDate       string
	Status          string
	NormalizedJSON  string
	RawJSON         string
	CreatedAt       string
}

type EventSuggestionRecord struct {
	ID                      int64
	BookID                  int64
	ProviderEventID         int64
	EventFamily             string
	InstrumentID            sql.NullInt64
	ConfidenceBPS           int
	Status                  string
	ProposedTransactionJSON string
	GeneratedTransactionID  sql.NullInt64
	FailureReason           string
	CreatedAt               string
	UpdatedAt               string
}

type AutomationRuleRecord struct {
	ID                     int64
	BookID                 int64
	SourceID               sql.NullInt64
	InstrumentID           sql.NullInt64
	EventFamily            string
	Mode                   string
	ConfidenceThresholdBPS int
	RequiredAccountsJSON   string
	Status                 string
	EffectiveFrom          string
	EffectiveTo            sql.NullString
	CreatedAt              string
	CreatedByUserID        int64
	UpdatedAt              string
	UpdatedByUserID        int64
}

type AutomationRuleSpec struct {
	SourceID               sql.NullInt64
	InstrumentID           sql.NullInt64
	EventFamily            string
	Mode                   string
	ConfidenceThresholdBPS int
	RequiredAccountsJSON   string
	Status                 string
	EffectiveFrom          string
	EffectiveTo            sql.NullString
}

type SaveAutomationRuleParams struct {
	BookID        int64
	RuleID        int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Spec          AutomationRuleSpec
	RecordedAt    string
	ChangeReason  string
}

func NewInvestmentRepository(database *sql.DB) *InvestmentRepository {
	return &InvestmentRepository{database: database}
}

func (r *InvestmentRepository) ListInstruments(ctx context.Context, bookID int64) ([]InvestmentInstrumentRecord, error) {
	rows, err := r.database.QueryContext(ctx, investmentInstrumentSelect(`
		WHERE ii.book_id = ?
		ORDER BY COALESCE(iiv.symbol, ''), iiv.display_name COLLATE NOCASE, ii.id
	`), bookID)
	if err != nil {
		return nil, fmt.Errorf("list investment instruments: %w", err)
	}
	defer rows.Close()

	return scanInvestmentInstrumentRecords(rows)
}

func (r *InvestmentRepository) SearchInstruments(ctx context.Context, bookID int64, query string, limit int) ([]InvestmentInstrumentRecord, error) {
	like := "%" + strings.TrimSpace(query) + "%"
	rows, err := r.database.QueryContext(ctx, investmentInstrumentSelect(`
		WHERE ii.book_id = ?
			AND (
				iiv.display_name LIKE ?
				OR iiv.symbol LIKE ?
				OR c.code LIKE ?
			)
		ORDER BY COALESCE(iiv.symbol, ''), iiv.display_name COLLATE NOCASE, ii.id
		LIMIT ?
	`), bookID, like, like, like, limit)
	if err != nil {
		return nil, fmt.Errorf("search investment instruments: %w", err)
	}
	defer rows.Close()

	return scanInvestmentInstrumentRecords(rows)
}

func (r *InvestmentRepository) InstrumentByID(ctx context.Context, bookID int64, instrumentID int64) (InvestmentInstrumentRecord, error) {
	var record InvestmentInstrumentRecord
	if err := scanInvestmentInstrumentRecord(r.database.QueryRowContext(ctx, investmentInstrumentSelect(`
		WHERE ii.book_id = ? AND ii.id = ?
	`), bookID, instrumentID), &record); err != nil {
		return InvestmentInstrumentRecord{}, err
	}
	return record, nil
}

func (r *InvestmentRepository) InstrumentByCommodityID(ctx context.Context, bookID int64, commodityID int64) (InvestmentInstrumentRecord, error) {
	var record InvestmentInstrumentRecord
	if err := scanInvestmentInstrumentRecord(r.database.QueryRowContext(ctx, investmentInstrumentSelect(`
		WHERE ii.book_id = ? AND ii.commodity_id = ?
	`), bookID, commodityID), &record); err != nil {
		return InvestmentInstrumentRecord{}, err
	}
	return record, nil
}

// InstrumentByISIN finds an active instrument whose identifiers_json.isin
// matches (case-insensitive), used by online-import instrument resolution
// (B-T212-INVST). ISIN is not a dedicated column (identifiers_json is a
// free-form blob — see docs/plans/import-connection-accounts-plan.md), so this is
// a json_extract scan rather than an indexed lookup; acceptable at the
// personal-finance scale this table holds. Returns ErrNotFound if no active
// instrument has a matching ISIN.
func (r *InvestmentRepository) InstrumentByISIN(ctx context.Context, bookID int64, isin string) (InvestmentInstrumentRecord, error) {
	var record InvestmentInstrumentRecord
	if err := scanInvestmentInstrumentRecord(r.database.QueryRowContext(ctx, investmentInstrumentSelect(`
		WHERE ii.book_id = ? AND iiv.status = 'active'
		  AND upper(json_extract(iiv.identifiers_json, '$.isin')) = upper(?)
	`), bookID, isin), &record); err != nil {
		return InvestmentInstrumentRecord{}, err
	}
	return record, nil
}

// InstrumentBySymbol finds an active instrument by exact symbol match
// (case-insensitive), used as the fallback when ISIN doesn't match — e.g. an
// instrument the user created manually before connecting an online source.
// Returns ErrNotFound if no active instrument has a matching symbol, or if
// more than one does (ambiguous — the caller should treat that the same as
// not found rather than guessing).
func (r *InvestmentRepository) InstrumentBySymbol(ctx context.Context, bookID int64, symbol string) (InvestmentInstrumentRecord, error) {
	rows, err := r.database.QueryContext(ctx, investmentInstrumentSelect(`
		WHERE ii.book_id = ? AND iiv.status = 'active' AND iiv.symbol = ? COLLATE NOCASE
	`), bookID, symbol)
	if err != nil {
		return InvestmentInstrumentRecord{}, fmt.Errorf("find instrument by symbol: %w", err)
	}
	defer rows.Close()

	records, err := scanInvestmentInstrumentRecords(rows)
	if err != nil {
		return InvestmentInstrumentRecord{}, err
	}
	if len(records) != 1 {
		return InvestmentInstrumentRecord{}, ErrNotFound
	}
	return records[0], nil
}

func (r *InvestmentRepository) CreateInstrument(ctx context.Context, params CreateInvestmentInstrumentParams) (InvestmentInstrumentRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return InvestmentInstrumentRecord{}, fmt.Errorf("begin create investment instrument: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return InvestmentInstrumentRecord{}, err
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.CreatedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return InvestmentInstrumentRecord{}, err
	}

	commodityID := params.Spec.CommodityID.Int64
	if commodityID == 0 {
		code := strings.TrimSpace(params.Spec.CommodityCode)
		if code == "" {
			code = strings.TrimSpace(params.Spec.Symbol)
		}
		if code == "" {
			code = fmt.Sprintf("SEC-%d", auditEventID)
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO commodities (book_id, code, kind, is_builtin, created_at, created_by_user_id, created_request_id, created_audit_event_id)
			VALUES (?, ?, 'security', 0, ?, ?, NULLIF(?, ''), ?)
		`, params.BookID, code, params.CreatedAt, params.CreatedByUserID, params.RequestID, auditEventID)
		if err != nil {
			return InvestmentInstrumentRecord{}, mapInvestmentConstraintError(fmt.Errorf("insert security commodity: %w", err))
		}
		commodityID, err = result.LastInsertId()
		if err != nil {
			return InvestmentInstrumentRecord{}, fmt.Errorf("read security commodity id: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO commodity_versions (
				commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id,
				change_reason, status, symbol, display_symbol, name, standard_scale,
				max_quantity_scale, metadata_json, change_audit_event_id
			)
			VALUES (?, 1, ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?, ?, ?)
		`, commodityID, CommodityGenesisDate, params.CreatedAt, params.CreatedByUserID, params.ChangeReason, params.Spec.Symbol, params.Spec.Symbol, params.Spec.DisplayName, params.Spec.QuantityScale, params.Spec.QuantityScale, params.Spec.MetadataJSON, auditEventID); err != nil {
			return InvestmentInstrumentRecord{}, fmt.Errorf("insert security commodity version: %w", err)
		}
	}

	var exists int
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM investment_instruments WHERE book_id = ? AND commodity_id = ?)
	`, params.BookID, commodityID).Scan(&exists); err != nil {
		return InvestmentInstrumentRecord{}, fmt.Errorf("check investment instrument exists: %w", err)
	}
	if exists == 1 {
		return InvestmentInstrumentRecord{}, ErrInvestmentInstrumentExists
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO investment_instruments (book_id, commodity_id, created_at, created_by_user_id, created_request_id, created_audit_event_id)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), ?)
	`, params.BookID, commodityID, params.CreatedAt, params.CreatedByUserID, params.RequestID, auditEventID)
	if err != nil {
		return InvestmentInstrumentRecord{}, mapInvestmentConstraintError(fmt.Errorf("insert investment instrument: %w", err))
	}
	instrumentID, err := result.LastInsertId()
	if err != nil {
		return InvestmentInstrumentRecord{}, fmt.Errorf("read investment instrument id: %w", err)
	}

	if _, err := insertInvestmentInstrumentVersion(ctx, tx, instrumentID, 1, params.Spec, commodityID, params.EffectiveFrom, params.CreatedAt, params.CreatedByUserID, params.ChangeReason, auditEventID); err != nil {
		return InvestmentInstrumentRecord{}, err
	}

	record, err := investmentInstrumentByIDTx(ctx, tx, params.BookID, instrumentID)
	if err != nil {
		return InvestmentInstrumentRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return InvestmentInstrumentRecord{}, fmt.Errorf("commit create investment instrument: %w", err)
	}
	committed = true
	return record, nil
}

// DeleteUnusedInstrument removes an instrument and its newly-created security
// commodity only when neither has acquired a durable reference. It exists for
// import compensation: a failed investment route must not leave structural
// records behind. Foreign-key checks deliberately make this fail closed if a
// concurrent or future feature has started using the instrument.
func (r *InvestmentRepository) DeleteUnusedInstrument(ctx context.Context, bookID int64, instrumentID int64) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete unused instrument: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	var commodityID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT commodity_id FROM investment_instruments WHERE id = ? AND book_id = ?
	`, instrumentID, bookID).Scan(&commodityID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("read unused instrument: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM investment_instrument_versions WHERE instrument_id = ?`, instrumentID); err != nil {
		return fmt.Errorf("delete unused instrument versions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM investment_instruments WHERE id = ? AND book_id = ?`, instrumentID, bookID); err != nil {
		return fmt.Errorf("delete unused instrument: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM commodity_versions WHERE commodity_id = ?`, commodityID); err != nil {
		return fmt.Errorf("delete unused security commodity versions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM commodities WHERE id = ? AND book_id = ? AND kind = 'security'`, commodityID, bookID); err != nil {
		return fmt.Errorf("delete unused security commodity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete unused instrument: %w", err)
	}
	committed = true
	return nil
}

func (r *InvestmentRepository) UpdateInstrument(ctx context.Context, params UpdateInvestmentInstrumentParams) (InvestmentInstrumentRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return InvestmentInstrumentRecord{}, fmt.Errorf("begin update investment instrument: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	current, err := investmentInstrumentByIDTx(ctx, tx, params.BookID, params.InstrumentID)
	if err != nil {
		return InvestmentInstrumentRecord{}, err
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ChangedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.RecordedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return InvestmentInstrumentRecord{}, err
	}
	if _, err := insertInvestmentInstrumentVersion(ctx, tx, params.InstrumentID, current.VersionSeq+1, params.Spec, current.CommodityID, params.EffectiveFrom, params.RecordedAt, params.ChangedByUserID, params.ChangeReason, auditEventID); err != nil {
		return InvestmentInstrumentRecord{}, err
	}
	record, err := investmentInstrumentByIDTx(ctx, tx, params.BookID, params.InstrumentID)
	if err != nil {
		return InvestmentInstrumentRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return InvestmentInstrumentRecord{}, fmt.Errorf("commit update investment instrument: %w", err)
	}
	committed = true
	return record, nil
}

func (r *InvestmentRepository) EnsureDefaultCostBasisProfile(ctx context.Context, bookID int64, actorUserID int64, authSessionID int64, requestID string, now string) (CostBasisProfileRecord, error) {
	record, err := r.DefaultCostBasisProfile(ctx, bookID)
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return CostBasisProfileRecord{}, err
	}
	return r.SaveCostBasisProfile(ctx, SaveCostBasisProfileParams{
		BookID:        bookID,
		ActorUserID:   actorUserID,
		AuthSessionID: authSessionID,
		RequestID:     requestID,
		OriginType:    "internal",
		Operation:     "investment.cost_basis_profile.create",
		RecordedAt:    now,
		ChangeReason:  "created default FIFO cost basis profile",
		Spec: CostBasisProfileSpec{
			Name:         "Default FIFO",
			Method:       "fifo",
			IsDefault:    true,
			Status:       "active",
			Description:  "Default first-in first-out cost basis profile",
			MetadataJSON: `{"source":"system_default"}`,
		},
	})
}

func (r *InvestmentRepository) ListCostBasisProfiles(ctx context.Context, bookID int64) ([]CostBasisProfileRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT profile.id, profile.book_id, profile.name, profile.method, profile.is_default,
			profile.status, profile.description, profile.metadata_json, profile.created_at,
			profile.created_by_user_id, profile.updated_at, profile.updated_by_user_id,
			version.id, version.version_seq
		FROM cost_basis_profiles profile
		JOIN cost_basis_profile_versions version ON version.id = profile.current_version_id
		WHERE profile.book_id = ?
		ORDER BY profile.is_default DESC, profile.name COLLATE NOCASE, profile.id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list cost basis profiles: %w", err)
	}
	defer rows.Close()
	return scanCostBasisProfiles(rows)
}

func (r *InvestmentRepository) DefaultCostBasisProfile(ctx context.Context, bookID int64) (CostBasisProfileRecord, error) {
	return scanCostBasisProfileRow(r.database.QueryRowContext(ctx, `
		SELECT profile.id, profile.book_id, profile.name, profile.method, profile.is_default,
			profile.status, profile.description, profile.metadata_json, profile.created_at,
			profile.created_by_user_id, profile.updated_at, profile.updated_by_user_id,
			version.id, version.version_seq
		FROM cost_basis_profiles profile
		JOIN cost_basis_profile_versions version ON version.id = profile.current_version_id
		WHERE profile.book_id = ? AND profile.is_default = 1 AND profile.status = 'active'
	`, bookID))
}

func (r *InvestmentRepository) SaveCostBasisProfile(ctx context.Context, params SaveCostBasisProfileParams) (CostBasisProfileRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return CostBasisProfileRecord{}, fmt.Errorf("begin save cost basis profile: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.RecordedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return CostBasisProfileRecord{}, err
	}
	if params.Spec.IsDefault {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cost_basis_profile_versions (
				book_id, profile_id, version_seq, name, method, is_default, status,
				description, metadata_json, recorded_at, changed_by_user_id, change_reason, audit_event_id
			)
			SELECT profile.book_id, profile.id, current.version_seq + 1, profile.name,
				profile.method, 0, profile.status, profile.description, profile.metadata_json,
				?, ?, ?, ?
			FROM cost_basis_profiles profile
			JOIN cost_basis_profile_versions current ON current.id = profile.current_version_id
			WHERE profile.book_id = ? AND profile.is_default = 1 AND profile.id <> ?
		`, params.RecordedAt, params.ActorUserID, params.ChangeReason, auditEventID,
			params.BookID, params.ProfileID); err != nil {
			return CostBasisProfileRecord{}, fmt.Errorf("version cleared default cost basis profiles: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE cost_basis_profiles
			SET is_default = 0, updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?,
				current_version_id = (
					SELECT version.id FROM cost_basis_profile_versions version
					WHERE version.profile_id = cost_basis_profiles.id
					ORDER BY version.version_seq DESC LIMIT 1
				)
			WHERE book_id = ? AND is_default = 1 AND id <> ?
		`, params.RecordedAt, params.ActorUserID, auditEventID, params.BookID, params.ProfileID); err != nil {
			return CostBasisProfileRecord{}, fmt.Errorf("clear default cost basis profiles: %w", err)
		}
	}
	var profileID int64
	if params.ProfileID > 0 {
		result, err := tx.ExecContext(ctx, `
			UPDATE cost_basis_profiles
			SET name = ?, method = ?, is_default = ?, status = ?, description = ?, metadata_json = ?,
				updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?
			WHERE book_id = ? AND id = ?
		`, params.Spec.Name, params.Spec.Method, boolInt(params.Spec.IsDefault), params.Spec.Status, params.Spec.Description, params.Spec.MetadataJSON, params.RecordedAt, params.ActorUserID, auditEventID, params.BookID, params.ProfileID)
		if err != nil {
			return CostBasisProfileRecord{}, mapInvestmentConstraintError(fmt.Errorf("update cost basis profile: %w", err))
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return CostBasisProfileRecord{}, fmt.Errorf("read cost basis update rows: %w", err)
		}
		if rowsAffected == 0 {
			return CostBasisProfileRecord{}, ErrNotFound
		}
		profileID = params.ProfileID
	} else {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO cost_basis_profiles (
				book_id, name, method, is_default, status, description, metadata_json,
				created_at, created_by_user_id, updated_at, updated_by_user_id,
				created_audit_event_id, updated_audit_event_id
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, params.BookID, params.Spec.Name, params.Spec.Method, boolInt(params.Spec.IsDefault), params.Spec.Status, params.Spec.Description, params.Spec.MetadataJSON, params.RecordedAt, params.ActorUserID, params.RecordedAt, params.ActorUserID, auditEventID, auditEventID)
		if err != nil {
			return CostBasisProfileRecord{}, mapInvestmentConstraintError(fmt.Errorf("insert cost basis profile: %w", err))
		}
		profileID, err = result.LastInsertId()
		if err != nil {
			return CostBasisProfileRecord{}, fmt.Errorf("read cost basis profile id: %w", err)
		}
	}
	var versionSeq int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version_seq), 0) + 1
		FROM cost_basis_profile_versions
		WHERE profile_id = ?
	`, profileID).Scan(&versionSeq); err != nil {
		return CostBasisProfileRecord{}, fmt.Errorf("next cost basis profile version: %w", err)
	}
	versionResult, err := tx.ExecContext(ctx, `
		INSERT INTO cost_basis_profile_versions (
			book_id, profile_id, version_seq, name, method, is_default, status,
			description, metadata_json, recorded_at, changed_by_user_id, change_reason, audit_event_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, params.BookID, profileID, versionSeq, params.Spec.Name, params.Spec.Method,
		boolInt(params.Spec.IsDefault), params.Spec.Status, params.Spec.Description,
		params.Spec.MetadataJSON, params.RecordedAt, params.ActorUserID, params.ChangeReason, auditEventID)
	if err != nil {
		return CostBasisProfileRecord{}, fmt.Errorf("insert cost basis profile version: %w", err)
	}
	versionID, err := versionResult.LastInsertId()
	if err != nil {
		return CostBasisProfileRecord{}, fmt.Errorf("read cost basis profile version id: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE cost_basis_profiles SET current_version_id = ? WHERE book_id = ? AND id = ?
	`, versionID, params.BookID, profileID); err != nil {
		return CostBasisProfileRecord{}, fmt.Errorf("select current cost basis profile version: %w", err)
	}
	record, err := costBasisProfileByIDTx(ctx, tx, params.BookID, profileID)
	if err != nil {
		return CostBasisProfileRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return CostBasisProfileRecord{}, fmt.Errorf("commit save cost basis profile: %w", err)
	}
	committed = true
	return record, nil
}

func (r *InvestmentRepository) ListDividendDefaults(ctx context.Context, bookID int64, commodityID int64) ([]DividendDefaultRecord, error) {
	where := "book_id = ?"
	args := []any{bookID}
	if commodityID > 0 {
		where += " AND commodity_id = ?"
		args = append(args, commodityID)
	}
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, book_id, commodity_id, income_account_id, withholding_account_id, default_withholding_value,
			default_withholding_scale, withholding_rate_bps, tax_country_code, tax_treatment, status,
			effective_from, effective_to, metadata_json, created_at, created_by_user_id, updated_at, updated_by_user_id
		FROM dividend_defaults
		WHERE `+where+`
		ORDER BY commodity_id IS NULL, commodity_id, effective_from DESC, id DESC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list dividend defaults: %w", err)
	}
	defer rows.Close()
	return scanDividendDefaults(rows)
}

func (r *InvestmentRepository) ResolveDividendDefault(ctx context.Context, bookID int64, commodityID int64, date string) (DividendDefaultRecord, error) {
	return scanDividendDefaultRow(r.database.QueryRowContext(ctx, `
		SELECT id, book_id, commodity_id, income_account_id, withholding_account_id, default_withholding_value,
			default_withholding_scale, withholding_rate_bps, tax_country_code, tax_treatment, status,
			effective_from, effective_to, metadata_json, created_at, created_by_user_id, updated_at, updated_by_user_id
		FROM dividend_defaults
		WHERE book_id = ?
			AND status = 'active'
			AND (commodity_id = ? OR commodity_id IS NULL)
			AND effective_from <= ?
			AND (effective_to IS NULL OR effective_to >= ?)
		ORDER BY commodity_id IS NULL, effective_from DESC, id DESC
		LIMIT 1
	`, bookID, commodityID, date, date))
}

func (r *InvestmentRepository) SaveDividendDefault(ctx context.Context, params SaveDividendDefaultParams) (DividendDefaultRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return DividendDefaultRecord{}, fmt.Errorf("begin save dividend default: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.RecordedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return DividendDefaultRecord{}, err
	}
	var defaultID int64
	if params.DefaultID > 0 {
		result, err := tx.ExecContext(ctx, `
			UPDATE dividend_defaults
			SET commodity_id = ?, income_account_id = ?, withholding_account_id = ?,
				default_withholding_value = ?, default_withholding_scale = ?, withholding_rate_bps = ?,
				tax_country_code = NULLIF(?, ''), tax_treatment = NULLIF(?, ''), status = ?,
				effective_from = ?, effective_to = ?, metadata_json = ?,
				updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?
			WHERE book_id = ? AND id = ?
		`, nullableInt64Value(params.Spec.CommodityID), params.Spec.IncomeAccountID, nullableInt64Value(params.Spec.WithholdingAccountID), nullableInt64Value(params.Spec.DefaultWithholdingValue), nullableInt64Value(params.Spec.DefaultWithholdingScale), nullableInt64Value(params.Spec.WithholdingRateBPS), params.Spec.TaxCountryCode, params.Spec.TaxTreatment, params.Spec.Status, params.Spec.EffectiveFrom, nullableStringValue(params.Spec.EffectiveTo), params.Spec.MetadataJSON, params.RecordedAt, params.ActorUserID, auditEventID, params.BookID, params.DefaultID)
		if err != nil {
			return DividendDefaultRecord{}, fmt.Errorf("update dividend default: %w", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return DividendDefaultRecord{}, fmt.Errorf("read dividend default update rows: %w", err)
		}
		if rowsAffected == 0 {
			return DividendDefaultRecord{}, ErrNotFound
		}
		defaultID = params.DefaultID
	} else {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO dividend_defaults (
				book_id, commodity_id, income_account_id, withholding_account_id, default_withholding_value,
				default_withholding_scale, withholding_rate_bps, tax_country_code, tax_treatment, status,
				effective_from, effective_to, metadata_json, created_at, created_by_user_id, updated_at,
				updated_by_user_id, created_audit_event_id, updated_audit_event_id
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, params.BookID, nullableInt64Value(params.Spec.CommodityID), params.Spec.IncomeAccountID, nullableInt64Value(params.Spec.WithholdingAccountID), nullableInt64Value(params.Spec.DefaultWithholdingValue), nullableInt64Value(params.Spec.DefaultWithholdingScale), nullableInt64Value(params.Spec.WithholdingRateBPS), params.Spec.TaxCountryCode, params.Spec.TaxTreatment, params.Spec.Status, params.Spec.EffectiveFrom, nullableStringValue(params.Spec.EffectiveTo), params.Spec.MetadataJSON, params.RecordedAt, params.ActorUserID, params.RecordedAt, params.ActorUserID, auditEventID, auditEventID)
		if err != nil {
			return DividendDefaultRecord{}, fmt.Errorf("insert dividend default: %w", err)
		}
		defaultID, err = result.LastInsertId()
		if err != nil {
			return DividendDefaultRecord{}, fmt.Errorf("read dividend default id: %w", err)
		}
	}
	record, err := dividendDefaultByIDTx(ctx, tx, params.BookID, defaultID)
	if err != nil {
		return DividendDefaultRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return DividendDefaultRecord{}, fmt.Errorf("commit save dividend default: %w", err)
	}
	committed = true
	return record, nil
}

func (r *InvestmentRepository) CreateLot(ctx context.Context, params CreateInvestmentLotParams) (InvestmentLotRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return InvestmentLotRecord{}, fmt.Errorf("begin create investment lot: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	record, err := createLotTx(ctx, tx, params)
	if err != nil {
		return InvestmentLotRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return InvestmentLotRecord{}, fmt.Errorf("commit create investment lot: %w", err)
	}
	committed = true
	return record, nil
}

func createLotTx(ctx context.Context, tx *sql.Tx, params CreateInvestmentLotParams) (InvestmentLotRecord, error) {
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.CreatedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return InvestmentLotRecord{}, err
	}
	return createLotWithAuditTx(ctx, tx, params, auditEventID)
}

// rescaleQuantity re-expresses a coefficient at a higher scale. Only widening
// is supported: narrowing would discard quantity, which is never something this
// package may do silently.
func rescaleQuantity(value exact.Coefficient, from int, to int) (exact.Coefficient, error) {
	if to == from {
		return value, nil
	}
	if to < from {
		return "", fmt.Errorf("%w: cannot narrow a quantity from scale %d to %d", ErrInvalidDisposalParams, from, to)
	}
	widened := new(big.Int).Mul(value.BigInt(), exact.Pow10(to-from))
	return exact.FromBig(widened)
}

func createLotWithAuditTx(ctx context.Context, tx *sql.Tx, params CreateInvestmentLotParams, auditEventID int64) (InvestmentLotRecord, error) {
	// A lot the position should have owned when it was last disposed of cannot
	// be added afterwards: the disposal took its basis from a pool this lot was
	// not in, and nothing recomputes that (T-95).
	if err := requirePositionEventInOrderTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID, params.OpenedOn, "an acquisition"); err != nil {
		return InvestmentLotRecord{}, err
	}
	// A lot's projection starts at the scale its acquisition was recorded at and
	// widens only when a disposal actually needs finer precision (T-97). It is
	// tempting to normalize every lot to the position's full precision up front
	// instead, which would make the scales uniform by construction — but a
	// position's quantity scale propagates into every figure derived from it:
	// PositionsWithGains computes marketScale as quantity scale + price scale -
	// base quantity scale, so padding a quantity by six places pads the reported
	// market value and unrealized gain by six places too. The values stay
	// correct, but the integers grow by a factor of a million, and that function
	// drops market value and gain entirely when the result no longer fits in an
	// int64. Widening on demand keeps the precision where it is needed without
	// making every position pay for it.
	result, err := tx.ExecContext(ctx, `
		INSERT INTO investment_lots (
			book_id, account_id, commodity_id, opened_on, source_transaction_id, status,
			quantity_value, quantity_scale, remaining_quantity_value, remaining_quantity_scale,
			cost_basis_value, cost_basis_scale, remaining_cost_basis_value, remaining_cost_basis_scale,
			cost_commodity_id, metadata_json, created_at, created_by_user_id, created_audit_event_id,
			updated_at, updated_by_user_id, updated_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, 'open', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, params.BookID, params.AccountID, params.CommodityID, params.OpenedOn, nullablePositiveInt64(params.SourceTransactionID), params.QuantityValue, params.QuantityScale, params.QuantityValue, params.QuantityScale, params.CostBasisValue, params.CostBasisScale, params.CostBasisValue, params.CostBasisScale, params.CostCommodityID, params.MetadataJSON, params.CreatedAt, params.CreatedByUserID, auditEventID, params.CreatedAt, params.CreatedByUserID, auditEventID)
	if err != nil {
		return InvestmentLotRecord{}, fmt.Errorf("insert investment lot: %w", err)
	}
	lotID, err := result.LastInsertId()
	if err != nil {
		return InvestmentLotRecord{}, fmt.Errorf("read investment lot id: %w", err)
	}
	eventResult, err := tx.ExecContext(ctx, `
		INSERT INTO investment_lot_events (
			book_id, lot_id, event_kind, transaction_id, event_date, quantity_value, quantity_scale,
			cost_basis_value, cost_basis_scale, metadata_json, created_at, created_by_user_id, created_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, params.BookID, lotID, params.EventKind, nullablePositiveInt64(params.SourceTransactionID), params.OpenedOn, params.QuantityValue, params.QuantityScale, params.CostBasisValue, params.CostBasisScale, params.MetadataJSON, params.CreatedAt, params.CreatedByUserID, auditEventID)
	if err != nil {
		return InvestmentLotRecord{}, fmt.Errorf("insert investment lot event: %w", err)
	}
	if params.SourceTransactionID > 0 {
		operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, params.SourceTransactionID)
		if err != nil {
			return InvestmentLotRecord{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO investment_lot_facts
				(lot_id, book_id, operation_id, account_id, commodity_id, position_side,
				 opened_on, quantity_value, quantity_scale, consideration_value,
				 consideration_scale, cost_commodity_id, created_audit_event_id)
			VALUES (?, ?, ?, ?, ?, 'long', ?, ?, ?, ?, ?, ?, ?)
		`, lotID, params.BookID, operationID, params.AccountID, params.CommodityID,
			params.OpenedOn, params.QuantityValue, params.QuantityScale,
			params.CostBasisValue, params.CostBasisScale, params.CostCommodityID, auditEventID); err != nil {
			return InvestmentLotRecord{}, fmt.Errorf("record investment lot source facts: %w", err)
		}
		eventID, err := eventResult.LastInsertId()
		if err != nil {
			return InvestmentLotRecord{}, fmt.Errorf("read acquisition event id: %w", err)
		}
		if err := linkLotEffectTx(ctx, tx, operationID, eventID); err != nil {
			return InvestmentLotRecord{}, err
		}
	}
	if err := requirePositionBasisRangeTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID, params.CostCommodityID); err != nil {
		return InvestmentLotRecord{}, err
	}
	record, err := investmentLotByIDTx(ctx, tx, params.BookID, lotID)
	if err != nil {
		return InvestmentLotRecord{}, err
	}
	return record, nil
}

func (r *InvestmentRepository) DisposeLots(ctx context.Context, params DisposeLotsParams) ([]LotDisposalRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin dispose lots: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	disposals, err := disposeLotsTx(ctx, tx, params)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit dispose lots: %w", err)
	}
	committed = true
	return disposals, nil
}

func (r *InvestmentRepository) SimulateDisposeLots(ctx context.Context, params DisposeLotsParams) ([]LotDisposalRecord, error) {
	// Use a dedicated connection so toggling FK enforcement is connection-scoped.
	conn, err := r.database.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection for simulate: %w", err)
	}
	// FK must be disabled outside a transaction (SQLite limitation). Always rolled back so safe.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("disable fk for simulate: %w", err)
	}
	// Re-enable FK on the connection before returning it to the pool. Use a fresh context so a
	// cancelled request context cannot prevent the re-enable; if it still fails, discard the
	// connection by closing it so the poisoned connection is never reused.
	defer func() {
		reenableCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(reenableCtx, "PRAGMA foreign_keys = ON"); err != nil {
			conn.Close()
			return
		}
		conn.Close()
	}()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin simulate dispose lots: %w", err)
	}
	defer rollbackTx(ctx, tx)
	if params.CreatedAt == "" {
		params.CreatedAt = "1970-01-01T00:00:00Z"
	}
	if params.OriginType == "" {
		params.OriginType = "internal"
	}
	if params.Operation == "" {
		params.Operation = "investment.lot.simulate"
	}
	return disposeLotsTx(ctx, tx, params)
}

func disposeLotsTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams) ([]LotDisposalRecord, error) {
	if params.MetadataJSON == "" {
		params.MetadataJSON = "{}"
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return nil, err
	}
	return disposeLotsWithAuditTx(ctx, tx, params, auditEventID)
}

// isDisposalCalendarDate reports whether a disposal's event date is a real
// YYYY-MM-DD calendar date, matching the CHECK constraint on the columns it is
// compared against. Lot eligibility is decided by string comparison, which is
// only meaningful for that shape.
func isDisposalCalendarDate(date string) bool {
	_, err := time.Parse(time.DateOnly, date)
	return err == nil
}

var validCostBasisMethods = map[string]bool{
	"fifo":         true,
	"lifo":         true,
	"average_cost": true,
	"specific_lot": true,
}

// acquisitionEventKinds are the lot events that only add to a position. Every
// other kind — disposals, splits, manual adjustments — reads the position's
// current projection and rewrites it, which is what makes ordering matter.
var acquisitionEventKinds = []string{"acquisition", "reinvested_dividend"}

// latestPositionRewriteDateTx returns the most recent date on which something
// rewrote a position's projection. An empty string means nothing has.
func latestPositionRewriteDateTx(ctx context.Context, tx *sql.Tx, bookID int64, accountID int64, commodityID int64) (string, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(acquisitionEventKinds)), ", ")
	args := []any{bookID, accountID, commodityID}
	for _, kind := range acquisitionEventKinds {
		args = append(args, kind)
	}
	var latest string
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(e.event_date), '')
		FROM investment_lot_events e
		JOIN investment_lots l ON l.id = e.lot_id
		WHERE l.book_id = ? AND l.account_id = ? AND l.commodity_id = ?
			AND e.event_kind NOT IN (`+placeholders+`)
	`, args...).Scan(&latest); err != nil {
		return "", fmt.Errorf("read latest position rewrite date: %w", err)
	}
	return latest, nil
}

// requirePositionEventInOrderTx refuses a lot event dated before the last event
// that rewrote the position's projection (T-95).
//
// A lot's cost_basis_value is immutable acquisition evidence, but the
// projection every method reads — remaining quantity and remaining basis — is
// not: a disposal rewrites it from whatever the previous events left behind.
// Average cost redistributes pooled basis across the surviving lots, and
// FIFO/LIFO consume specific ones. So the projection is only meaningful as of
// the last disposal applied to it, and an event dated earlier reads a position
// that has already moved past it. The reported case: buy 10 in January for
// 100, buy 10 in June for 300, sell 5 in July at average cost — after which
// January's surviving 5 shares carry 100 of pooled basis — then enter a sale
// dated March. The March sale passes the opened_on filter, selects the January
// lot, and takes basis that only exists because of a June purchase and a July
// sale.
//
// A backdated event is refused only against *disposals*, not against later
// acquisitions, because acquisitions alone leave the projection intact: the
// opened_on filters already keep a later purchase out of an earlier sale's
// pool, and its own remaining basis is still exactly what was paid. Selling in
// March after entering a June purchase therefore still works, and so does
// entering the June purchase afterwards.
//
// Replaying a position under a corrected history is a real feature, with
// elections and journal entries to preserve, and it is not this. Until it
// exists, the honest answer is to refuse the event rather than to quietly
// compute it against the wrong state. The comparison is inclusive, so same-day
// events stay legal in the order they are entered: buying and selling on one
// day, or two sales on one day, both still work.
func requirePositionEventInOrderTx(ctx context.Context, tx *sql.Tx, bookID int64, accountID int64, commodityID int64, eventDate string, what string) error {
	latest, err := latestPositionRewriteDateTx(ctx, tx, bookID, accountID, commodityID)
	if err != nil {
		return err
	}
	if latest == "" || eventDate >= latest {
		return nil
	}
	return fmt.Errorf("%w: %s dated %s is before this position's disposal on %s", ErrOutOfOrderPositionEvent, what, eventDate, latest)
}

func disposeLotsWithAuditTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams, auditEventID int64) ([]LotDisposalRecord, error) {
	method := params.CostBasisMethod
	if method == "" {
		method = "fifo"
	}
	if !validCostBasisMethods[method] {
		return nil, fmt.Errorf("%w: cost basis method %q is not supported", ErrInvalidDisposalParams, method)
	}
	params.CostBasisMethod = method
	// Every lot-eligibility comparison below is a string comparison against
	// this date, and an empty or malformed one sorts below every stored
	// opened_on — which would read as "no lots are eligible" rather than as the
	// programming error it is. Reject it here instead.
	if !isDisposalCalendarDate(params.EventDate) {
		return nil, fmt.Errorf("%w: disposal event date %q is not a calendar date", ErrInvalidDisposalParams, params.EventDate)
	}
	// A disposal reads the projection a previous disposal left behind, so none
	// may already sit after this date (T-95).
	if err := requirePositionEventInOrderTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID, params.EventDate, "a disposal"); err != nil {
		return nil, err
	}
	costCommodityID, err := resolveDisposalCostCommodityTx(ctx, tx, params)
	if err != nil {
		return nil, err
	}
	params.CostCommodityID = costCommodityID
	if err := enforcePositionMethodFamilyTx(ctx, tx, params, method); err != nil {
		return nil, err
	}

	// One allocation scale for the whole command, resolved before any lot is
	// touched, so every method splits basis at the same precision and that
	// precision does not depend on how the purchases were typed (T-103).
	allocationScale, err := positionBasisAllocationScaleTx(ctx, tx, params)
	if err != nil {
		return nil, err
	}

	var disposals []LotDisposalRecord
	if method == "specific_lot" {
		disposals, err = disposeSpecificLotsTx(ctx, tx, params, auditEventID, allocationScale)
	} else if len(params.Allocations) > 0 {
		return nil, fmt.Errorf("%w: explicit lot allocations are only permitted for specific_lot cost basis method", ErrInvalidDisposalParams)
	} else if method == "average_cost" {
		disposals, err = disposeAverageCostTx(ctx, tx, params, auditEventID, allocationScale)
	} else {
		disposals, err = disposeFIFOOrLIFOTx(ctx, tx, params, auditEventID, method, allocationScale)
	}
	if err != nil {
		return nil, err
	}
	// Include future acquisitions too: widening an eligible lot must not make
	// the current all-lots position unreadable (T-104).
	if err := requirePositionBasisRangeTx(ctx, tx, params.BookID, params.AccountID, params.CommodityID, params.CostCommodityID); err != nil {
		return nil, err
	}
	if err := updatePositionMethodFamilyTx(ctx, tx, params, method, auditEventID); err != nil {
		return nil, err
	}
	return disposals, nil
}

func resolveDisposalCostCommodityTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams) (int64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT cost_commodity_id
		FROM investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND status = 'open'
			AND opened_on <= ?
			AND (? = 0 OR cost_commodity_id = ?)
		ORDER BY cost_commodity_id
	`, params.BookID, params.AccountID, params.CommodityID, params.EventDate, params.CostCommodityID, params.CostCommodityID)
	if err != nil {
		return 0, fmt.Errorf("read disposal cost commodities: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, fmt.Errorf("scan disposal cost commodity: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate disposal cost commodities: %w", err)
	}
	if len(ids) == 0 {
		return 0, ErrInsufficientLots
	}
	if len(ids) > 1 {
		return 0, fmt.Errorf("%w: cost commodity is required when a position has lots in multiple currencies", ErrInvalidDisposalParams)
	}
	return ids[0], nil
}

func methodFamily(method string) string {
	if method == "average_cost" {
		return "average_cost"
	}
	return "individual_lot"
}

func enforcePositionMethodFamilyTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams, method string) error {
	var existing string
	err := tx.QueryRowContext(ctx, `
		SELECT method_family FROM investment_position_basis_state
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND position_side = 'long'
	`, params.BookID, params.AccountID, params.CommodityID, params.CostCommodityID).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read position basis method state: %w", err)
	}
	if existing != methodFamily(method) {
		return fmt.Errorf("%w: cannot switch into or out of average_cost while the position remains open", ErrInvalidDisposalParams)
	}
	return nil
}

func updatePositionMethodFamilyTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams, method string, auditEventID int64) error {
	// Deliberately not date-filtered, unlike the lot-eligibility queries above
	// (T-95). This asks whether the position is closed *now* so the method-family
	// lock can be released; a lot opened after this disposal's date still keeps
	// the position open and the lock in force. Adding `opened_on <= ?` here would
	// release the lock while shares are still held.
	var openCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND status = 'open'
	`, params.BookID, params.AccountID, params.CommodityID, params.CostCommodityID).Scan(&openCount); err != nil {
		return fmt.Errorf("count open lots after disposal: %w", err)
	}
	if openCount == 0 {
		_, err := tx.ExecContext(ctx, `DELETE FROM investment_position_basis_state
			WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND position_side = 'long'`,
			params.BookID, params.AccountID, params.CommodityID, params.CostCommodityID)
		if err != nil {
			return fmt.Errorf("close position basis method state: %w", err)
		}
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO investment_position_basis_state (
			book_id, account_id, commodity_id, cost_commodity_id, position_side, method_family,
			updated_at, updated_by_user_id, updated_audit_event_id
		) VALUES (?, ?, ?, ?, 'long', ?, ?, ?, ?)
		ON CONFLICT (book_id, account_id, commodity_id, cost_commodity_id, position_side) DO UPDATE SET
			method_family = excluded.method_family,
			updated_at = excluded.updated_at,
			updated_by_user_id = excluded.updated_by_user_id,
			updated_audit_event_id = excluded.updated_audit_event_id
	`, params.BookID, params.AccountID, params.CommodityID, params.CostCommodityID,
		methodFamily(method), params.CreatedAt, params.ActorUserID, auditEventID)
	if err != nil {
		return fmt.Errorf("save position basis method state: %w", err)
	}
	return nil
}

func disposeSpecificLotsTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams, auditEventID int64, allocationScale int) ([]LotDisposalRecord, error) {
	if len(params.Allocations) == 0 {
		return nil, fmt.Errorf("%w: specific_lot method requires explicit lot allocations", ErrInvalidDisposalParams)
	}
	// Verify sum(allocations) == params.QuantityValue so lot events stay consistent with the transaction quantity.
	allocationTotal := new(big.Int)
	for _, a := range params.Allocations {
		if a.QuantityScale != params.QuantityScale {
			return nil, fmt.Errorf("%w: specific_lot allocation scale %d does not match sale scale %d", ErrInvalidDisposalParams, a.QuantityScale, params.QuantityScale)
		}
		allocationTotal.Add(allocationTotal, a.QuantityValue.BigInt())
	}
	if allocationTotal.Cmp(params.QuantityValue.BigInt()) != 0 {
		return nil, fmt.Errorf("%w: specific_lot allocations total %s does not equal sale quantity %s", ErrInvalidDisposalParams, allocationTotal, params.QuantityValue)
	}
	disposals := make([]LotDisposalRecord, 0, len(params.Allocations))
	for _, allocation := range params.Allocations {
		disposal, err := disposeLotTx(ctx, tx, params, allocation.LotID, allocation.QuantityValue, allocation.QuantityScale, auditEventID, allocationScale)
		if err != nil {
			return nil, err
		}
		disposals = append(disposals, disposal)
	}
	return disposals, nil
}

func disposeFIFOOrLIFOTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams, auditEventID int64, method string, allocationScale int) ([]LotDisposalRecord, error) {
	orderClause := "ORDER BY opened_on, id"
	if method == "lifo" {
		orderClause = "ORDER BY opened_on DESC, id DESC"
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, remaining_quantity_value, remaining_quantity_scale
		FROM investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND status = 'open'
			AND opened_on <= ?
		`+orderClause, params.BookID, params.AccountID, params.CommodityID, params.CostCommodityID, params.EventDate)
	if err != nil {
		return nil, fmt.Errorf("read %s lots: %w", method, err)
	}
	type lotRef struct {
		id    int64
		value exact.Coefficient
		scale int
	}
	var lots []lotRef
	for rows.Next() {
		var lot lotRef
		if err := rows.Scan(&lot.id, &lot.value, &lot.scale); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan %s lot: %w", method, err)
		}
		lots = append(lots, lot)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close %s lots: %w", method, err)
	}

	// Lots can carry a different quantity_scale than the sale (e.g. imported
	// trades whose raw source strings had differing decimal-place counts for
	// the same magnitude). Track the undisposed sale quantity in big.Int at a
	// scale at least as precise as every candidate lot so comparisons and
	// subtraction below are never done across mismatched scales.
	commonScale := params.QuantityScale
	for _, lot := range lots {
		if lot.scale > commonScale {
			commonScale = lot.scale
		}
	}
	remaining := new(big.Int).Mul(params.QuantityValue.BigInt(), exact.Pow10(commonScale-params.QuantityScale))

	disposals := make([]LotDisposalRecord, 0)
	for _, lot := range lots {
		if remaining.Sign() <= 0 {
			break
		}
		lotAtCommonScale := new(big.Int).Mul(lot.value.BigInt(), exact.Pow10(commonScale-lot.scale))

		takeAtCommonScale := new(big.Int).Set(lotAtCommonScale)
		if takeAtCommonScale.Cmp(remaining) > 0 {
			takeAtCommonScale.Set(remaining)
		}

		// Hand the take to disposeLotTx at the common scale. This used to be
		// converted back to the lot's own scale first, and refused when that
		// conversion had a remainder — which is how selling half a share out of
		// a lot entered as "10" became impossible while the same lot entered as
		// "10.0" allowed it (T-97). disposeLotTx aligns for itself now, so the
		// round trip and its refusal are both gone.
		takeCoeff, err := exact.FromBig(takeAtCommonScale)
		if err != nil {
			return nil, err
		}

		disposal, err := disposeLotTx(ctx, tx, params, lot.id, takeCoeff, commonScale, auditEventID, allocationScale)
		if err != nil {
			return nil, err
		}
		disposals = append(disposals, disposal)
		remaining.Sub(remaining, takeAtCommonScale)
	}
	if remaining.Sign() > 0 {
		return nil, ErrInsufficientLots
	}
	return disposals, nil
}

type avgCostLotRef struct {
	id             int64
	quantityValue  exact.Coefficient
	quantityScale  int
	costBasisValue int64
	costBasisScale int
}

func disposeAverageCostTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams, auditEventID int64, allocationScale int) ([]LotDisposalRecord, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, remaining_quantity_value, remaining_quantity_scale,
		       remaining_cost_basis_value, remaining_cost_basis_scale
		FROM investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND status = 'open'
			AND opened_on <= ?
		ORDER BY opened_on, id
	`, params.BookID, params.AccountID, params.CommodityID, params.CostCommodityID, params.EventDate)
	if err != nil {
		return nil, fmt.Errorf("read average-cost lots: %w", err)
	}
	var lots []avgCostLotRef
	for rows.Next() {
		var lot avgCostLotRef
		if err := rows.Scan(&lot.id, &lot.quantityValue, &lot.quantityScale, &lot.costBasisValue, &lot.costBasisScale); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan average-cost lot: %w", err)
		}
		lots = append(lots, lot)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close average-cost lots: %w", err)
	}
	if len(lots) == 0 {
		return nil, ErrInsufficientLots
	}

	// The pool math below treats every lot's quantity and basis as plain
	// integers at one shared scale each, which is why mixing scales would
	// blend incommensurate magnitudes. This used to be enforced by refusing
	// any position whose lots disagreed — a refusal the user could do nothing
	// about, since the scales came from however each purchase happened to be
	// typed or imported (T-97). Widening every lot to the most precise scale
	// present satisfies the same requirement without turning a presentation
	// detail into a dead end. Nothing narrows, so no quantity or basis is
	// rounded away to make the pool line up.
	commonScale := params.QuantityScale
	for _, lot := range lots {
		if lot.quantityScale > commonScale {
			commonScale = lot.quantityScale
		}
	}
	// The pooled basis is split at the commodity's allocation scale rather
	// than at the deepest scale the purchases happened to be typed at, so the
	// pool rate does not change when the same money is written differently
	// (T-103). It is never shallower than a recorded basis, so this still only
	// ever widens.
	commonCostScale := allocationScale
	for i := range lots {
		quantity, err := rescaleQuantity(lots[i].quantityValue, lots[i].quantityScale, commonScale)
		if err != nil {
			return nil, err
		}
		lots[i].quantityValue, lots[i].quantityScale = quantity, commonScale

		basis, err := exact.ScaledIntFromInt64(lots[i].costBasisValue, lots[i].costBasisScale).TruncatedTo(commonCostScale).Int64()
		if err != nil {
			return nil, fmt.Errorf("align lot %d cost basis to scale %d: %w", lots[i].id, commonCostScale, err)
		}
		lots[i].costBasisValue, lots[i].costBasisScale = basis, commonCostScale
	}

	// Compute pool totals.
	pooledQty := new(big.Int)
	pooledBasis := new(big.Int)
	for _, lot := range lots {
		pooledQty.Add(pooledQty, lot.quantityValue.BigInt())
		pooledBasis.Add(pooledBasis, big.NewInt(lot.costBasisValue))
	}
	sellQtyCoeff, err := rescaleQuantity(params.QuantityValue, params.QuantityScale, commonScale)
	if err != nil {
		return nil, err
	}
	sellQty := sellQtyCoeff.BigInt()
	if sellQty.Cmp(pooledQty) > 0 {
		return nil, ErrInsufficientLots
	}

	// Total disposed basis at pool rate (truncate): pooledBasis * sellQty / pooledQty.
	disposedBasisTotal := new(big.Int).Mul(pooledBasis, sellQty)
	disposedBasisTotal.Quo(disposedBasisTotal, pooledQty)

	// Allocate the sale quantity FIFO only to give the disposal event stable lot
	// links. Basis is pooled: after the sale every surviving projection row is
	// redistributed at the same pool rate, with the final survivor absorbing the
	// integer residual. Original cost_basis_value is never changed.
	takes := make([]*big.Int, len(lots))
	remainingToTake := new(big.Int).Set(sellQty)
	for i, lot := range lots {
		takes[i] = new(big.Int)
		if remainingToTake.Sign() == 0 {
			continue
		}
		takes[i].Set(lot.quantityValue.BigInt())
		if takes[i].Cmp(remainingToTake) > 0 {
			takes[i].Set(remainingToTake)
		}
		remainingToTake.Sub(remainingToTake, takes[i])
	}
	if remainingToTake.Sign() > 0 {
		return nil, ErrInsufficientLots
	}

	disposals := make([]LotDisposalRecord, 0)
	disposedBasisRemaining := new(big.Int).Set(disposedBasisTotal)
	lastTouched := -1
	for i := range takes {
		if takes[i].Sign() > 0 {
			lastTouched = i
		}
	}
	for i, lot := range lots {
		if takes[i].Sign() == 0 {
			continue
		}
		reported := new(big.Int)
		if i == lastTouched {
			reported.Set(disposedBasisRemaining)
		} else {
			reported.Mul(pooledBasis, takes[i])
			reported.Quo(reported, pooledQty)
		}
		reportedValue, err := exact.ScaledIntFromBig(reported, commonCostScale).Int64()
		if err != nil {
			return nil, err
		}
		takeCoeff, err := exact.FromBig(takes[i])
		if err != nil {
			return nil, err
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO investment_lot_events (
				book_id, lot_id, event_kind, transaction_id, event_date, quantity_value, quantity_scale,
				cost_basis_value, cost_basis_scale, cost_basis_method, metadata_json,
				created_at, created_by_user_id, created_audit_event_id
			) VALUES (?, ?, 'disposal', ?, ?, ?, ?, ?, ?, 'average_cost', ?, ?, ?, ?)
		`, params.BookID, lot.id, nullablePositiveInt64(params.TransactionID), params.EventDate,
			takeCoeff.Negated(), lot.quantityScale, -reportedValue, commonCostScale,
			params.MetadataJSON, params.CreatedAt, params.ActorUserID, auditEventID)
		if err != nil {
			return nil, fmt.Errorf("insert average-cost disposal lot event: %w", err)
		}
		eventID, err := result.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("read average-cost disposal lot event id: %w", err)
		}
		disposals = append(disposals, LotDisposalRecord{EventID: eventID, LotID: lot.id, QuantityValue: takeCoeff,
			QuantityScale: lot.quantityScale, CostBasisValue: reportedValue, CostBasisScale: commonCostScale,
			CostCommodityID: params.CostCommodityID})
		disposedBasisRemaining.Sub(disposedBasisRemaining, reported)
	}

	remainingPoolQty := new(big.Int).Sub(pooledQty, sellQty)
	remainingPoolBasis := new(big.Int).Sub(pooledBasis, disposedBasisTotal)
	remainingBasisToAssign := new(big.Int).Set(remainingPoolBasis)
	lastSurvivor := -1
	for i, lot := range lots {
		if new(big.Int).Sub(lot.quantityValue.BigInt(), takes[i]).Sign() > 0 {
			lastSurvivor = i
		}
	}
	for i, lot := range lots {
		nextQtyBig := new(big.Int).Sub(lot.quantityValue.BigInt(), takes[i])
		nextQty, err := exact.FromBig(nextQtyBig)
		if err != nil {
			return nil, err
		}
		nextBasis := new(big.Int)
		status := "closed"
		if nextQtyBig.Sign() > 0 {
			status = "open"
			if i == lastSurvivor {
				nextBasis.Set(remainingBasisToAssign)
			} else {
				nextBasis.Mul(remainingPoolBasis, nextQtyBig)
				nextBasis.Quo(nextBasis, remainingPoolQty)
				remainingBasisToAssign.Sub(remainingBasisToAssign, nextBasis)
			}
		}
		nextBasisValue, err := exact.ScaledIntFromBig(nextBasis, commonCostScale).Int64()
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE investment_lots
			SET remaining_quantity_value = ?, remaining_quantity_scale = ?,
				remaining_cost_basis_value = ?, remaining_cost_basis_scale = ?, status = ?,
				updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?
			WHERE book_id = ? AND id = ?
		`, nextQty, commonScale, nextBasisValue, commonCostScale, status, params.CreatedAt, params.ActorUserID, auditEventID,
			params.BookID, lot.id); err != nil {
			return nil, fmt.Errorf("update average-cost lot projection: %w", err)
		}
	}
	return disposals, nil
}

func (r *InvestmentRepository) CreateTransactionAndLot(ctx context.Context, transactionParams CreateTransactionParams, lotParams CreateInvestmentLotParams) (TransactionRecord, InvestmentLotRecord, error) {
	return r.createTransactionAndLot(ctx, transactionParams, lotParams, nil)
}

// CreateTransactionAndLotWithPostWrite invokes postWrite in the same SQLite
// transaction as the ledger transaction and lot. This keeps import identity
// recording crash-safe: a failure rolls back all three writes together.
func (r *InvestmentRepository) CreateTransactionAndLotWithPostWrite(ctx context.Context, transactionParams CreateTransactionParams, lotParams CreateInvestmentLotParams, postWrite func(*sql.Tx, int64) error) (TransactionRecord, InvestmentLotRecord, error) {
	return r.createTransactionAndLot(ctx, transactionParams, lotParams, postWrite)
}

func (r *InvestmentRepository) createTransactionAndLot(ctx context.Context, transactionParams CreateTransactionParams, lotParams CreateInvestmentLotParams, postWrite func(*sql.Tx, int64) error) (TransactionRecord, InvestmentLotRecord, error) {
	return executeInvestmentWriteTx(ctx, r.database, transactionParams,
		func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (InvestmentLotRecord, error) {
			lotParams.SourceTransactionID = transaction.ID
			return createLotWithAuditTx(ctx, tx, lotParams, auditEventID)
		}, postWrite)
}

func (r *InvestmentRepository) CreateTransactionAndDisposeLots(ctx context.Context, transactionParams CreateTransactionParams, disposalParams DisposeLotsParams) (TransactionRecord, []LotDisposalRecord, error) {
	transaction, disposals, _, err := r.createTransactionAndDisposeLots(ctx, transactionParams, disposalParams, nil)
	return transaction, disposals, err
}

func (r *InvestmentRepository) CreateTransactionAndDisposeLotsWithDecision(ctx context.Context, transactionParams CreateTransactionParams, disposalParams DisposeLotsParams) (TransactionRecord, []LotDisposalRecord, DisposalDecisionRecord, error) {
	return r.createTransactionAndDisposeLots(ctx, transactionParams, disposalParams, nil)
}

// CreateTransactionAndDisposeLotsWithPostWrite is the sell-side equivalent of
// CreateTransactionAndLotWithPostWrite. The callback runs after lot disposal
// but before the enclosing transaction commits.
func (r *InvestmentRepository) CreateTransactionAndDisposeLotsWithPostWrite(ctx context.Context, transactionParams CreateTransactionParams, disposalParams DisposeLotsParams, postWrite func(*sql.Tx, int64) error) (TransactionRecord, []LotDisposalRecord, error) {
	transaction, disposals, _, err := r.createTransactionAndDisposeLots(ctx, transactionParams, disposalParams, postWrite)
	return transaction, disposals, err
}

func (r *InvestmentRepository) CreateTransactionAndDisposeLotsWithDecisionAndPostWrite(ctx context.Context, transactionParams CreateTransactionParams, disposalParams DisposeLotsParams, postWrite func(*sql.Tx, int64) error) (TransactionRecord, []LotDisposalRecord, DisposalDecisionRecord, error) {
	return r.createTransactionAndDisposeLots(ctx, transactionParams, disposalParams, postWrite)
}

func (r *InvestmentRepository) createTransactionAndDisposeLots(ctx context.Context, transactionParams CreateTransactionParams, disposalParams DisposeLotsParams, postWrite func(*sql.Tx, int64) error) (TransactionRecord, []LotDisposalRecord, DisposalDecisionRecord, error) {
	type result struct {
		disposals []LotDisposalRecord
		decision  DisposalDecisionRecord
	}
	transaction, outcome, err := executeInvestmentWriteTx(ctx, r.database, transactionParams,
		func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (result, error) {
			disposalParams.TransactionID = transaction.ID
			disposals, err := disposeLotsWithAuditTx(ctx, tx, disposalParams, auditEventID)
			if err != nil {
				return result{}, err
			}
			decision, err := createDisposalDecisionTx(ctx, tx, transaction, disposalParams, disposals, auditEventID)
			if err != nil {
				return result{}, err
			}
			return result{disposals: disposals, decision: decision}, nil
		}, postWrite)
	if err != nil {
		return TransactionRecord{}, nil, DisposalDecisionRecord{}, err
	}
	return transaction, outcome.disposals, outcome.decision, nil
}

func createDisposalDecisionTx(ctx context.Context, tx *sql.Tx, transaction TransactionRecord, params DisposeLotsParams, disposals []LotDisposalRecord, auditEventID int64) (DisposalDecisionRecord, error) {
	if len(disposals) == 0 {
		return DisposalDecisionRecord{}, fmt.Errorf("create disposal decision: no allocations")
	}
	operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
	if err != nil {
		return DisposalDecisionRecord{}, err
	}
	costCommodityID := params.CostCommodityID
	if costCommodityID == 0 {
		if err := tx.QueryRowContext(ctx, `SELECT cost_commodity_id FROM investment_lots WHERE book_id = ? AND id = ?`, params.BookID, disposals[0].LotID).Scan(&costCommodityID); err != nil {
			return DisposalDecisionRecord{}, fmt.Errorf("read disposal decision cost commodity: %w", err)
		}
	}
	disposedBasis := exact.NewScaledInt()
	for _, allocation := range disposals {
		disposedBasis.AddInt64(allocation.CostBasisValue, allocation.CostBasisScale)
	}
	disposedBasisValue, err := disposedBasis.Coefficient()
	if err != nil {
		return DisposalDecisionRecord{}, fmt.Errorf("compute disposal decision basis: %w", err)
	}
	source := params.DecisionSource
	if source.ResolutionTier == "" {
		source.ResolutionTier = "fallback"
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO investment_disposal_decisions (
			book_id, transaction_id, transaction_version_id, operation_id, position_side, account_id, commodity_id,
			cost_commodity_id, event_date, quantity_value, quantity_scale,
			disposed_basis_value, disposed_basis_scale, cost_basis_method, resolution_tier,
			account_version_id, profile_id, profile_version_id, source_effective_from,
			source_recorded_at, created_at, created_by_user_id, created_audit_event_id
		) VALUES (?, ?, ?, ?, 'long', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, params.BookID, transaction.ID, transaction.VersionID, operationID, params.AccountID, params.CommodityID,
		costCommodityID, params.EventDate, params.QuantityValue, params.QuantityScale,
		disposedBasisValue, disposedBasis.Scale(), params.CostBasisMethod, source.ResolutionTier,
		nullablePositiveInt64(source.AccountVersionID), nullablePositiveInt64(source.ProfileID),
		nullablePositiveInt64(source.ProfileVersionID), nullableStringValue(sql.NullString{String: source.SourceEffectiveFrom, Valid: source.SourceEffectiveFrom != ""}),
		nullableStringValue(sql.NullString{String: source.SourceRecordedAt, Valid: source.SourceRecordedAt != ""}), params.CreatedAt, params.ActorUserID, auditEventID)
	if err != nil {
		return DisposalDecisionRecord{}, fmt.Errorf("insert disposal decision: %w", err)
	}
	decisionID, err := result.LastInsertId()
	if err != nil {
		return DisposalDecisionRecord{}, fmt.Errorf("read disposal decision id: %w", err)
	}
	for index, allocation := range disposals {
		if err := linkLotEffectTx(ctx, tx, operationID, allocation.EventID); err != nil {
			return DisposalDecisionRecord{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO investment_disposal_allocations (
				book_id, decision_id, lot_event_id, lot_id, allocation_seq,
				quantity_value, quantity_scale, cost_basis_value, cost_basis_scale
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, params.BookID, decisionID, allocation.EventID, allocation.LotID, index+1,
			allocation.QuantityValue, allocation.QuantityScale, allocation.CostBasisValue,
			allocation.CostBasisScale); err != nil {
			return DisposalDecisionRecord{}, fmt.Errorf("insert disposal decision allocation: %w", err)
		}
	}
	return DisposalDecisionRecord{
		ID: decisionID, TransactionID: transaction.ID, TransactionVersionID: transaction.VersionID,
		AccountID: params.AccountID, CommodityID: params.CommodityID, CostCommodityID: costCommodityID,
		EventDate: params.EventDate, QuantityValue: params.QuantityValue, QuantityScale: params.QuantityScale,
		DisposedBasisValue: disposedBasisValue, DisposedBasisScale: disposedBasis.Scale(),
		CostBasisMethod: params.CostBasisMethod, DisposalDecisionSource: source,
		CreatedAt: params.CreatedAt, AuditEventID: auditEventID, Allocations: disposals,
	}, nil
}

func (r *InvestmentRepository) ListLots(ctx context.Context, bookID int64, accountID int64, commodityID int64) ([]InvestmentLotRecord, error) {
	where := []string{"book_id = ?"}
	args := []any{bookID}
	if accountID > 0 {
		where = append(where, "account_id = ?")
		args = append(args, accountID)
	}
	if commodityID > 0 {
		where = append(where, "commodity_id = ?")
		args = append(args, commodityID)
	}
	rows, err := r.database.QueryContext(ctx, investmentLotSelect(`
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY opened_on, id
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list investment lots: %w", err)
	}
	defer rows.Close()
	return scanInvestmentLots(rows)
}

func (r *InvestmentRepository) Positions(ctx context.Context, bookID int64) ([]InvestmentPositionRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT
			lot.account_id,
			lot.commodity_id,
			lot.remaining_quantity_value,
			lot.remaining_quantity_scale,
			lot.remaining_cost_basis_value,
			lot.remaining_cost_basis_scale,
			lot.cost_commodity_id,
			(
				SELECT po.price_value
				FROM price_observations po
				WHERE po.book_id = lot.book_id
					AND po.base_commodity_id = lot.commodity_id
					AND po.quote_commodity_id = lot.cost_commodity_id
					AND po.voided_at IS NULL
				ORDER BY po.valuation_date DESC, po.recorded_at DESC, po.id DESC
				LIMIT 1
			) AS latest_price_value,
			(
				SELECT po.price_scale
				FROM price_observations po
				WHERE po.book_id = lot.book_id
					AND po.base_commodity_id = lot.commodity_id
					AND po.quote_commodity_id = lot.cost_commodity_id
					AND po.voided_at IS NULL
				ORDER BY po.valuation_date DESC, po.recorded_at DESC, po.id DESC
				LIMIT 1
			) AS latest_price_scale,
			(
				SELECT po.valuation_date
				FROM price_observations po
				WHERE po.book_id = lot.book_id
					AND po.base_commodity_id = lot.commodity_id
					AND po.quote_commodity_id = lot.cost_commodity_id
					AND po.voided_at IS NULL
				ORDER BY po.valuation_date DESC, po.recorded_at DESC, po.id DESC
				LIMIT 1
			) AS latest_price_date,
			(
				SELECT po.base_quantity_value
				FROM price_observations po
				WHERE po.book_id = lot.book_id
					AND po.base_commodity_id = lot.commodity_id
					AND po.quote_commodity_id = lot.cost_commodity_id
					AND po.voided_at IS NULL
				ORDER BY po.valuation_date DESC, po.recorded_at DESC, po.id DESC
				LIMIT 1
			) AS latest_price_base_quantity_value,
			(
				SELECT po.base_quantity_scale
				FROM price_observations po
				WHERE po.book_id = lot.book_id
					AND po.base_commodity_id = lot.commodity_id
					AND po.quote_commodity_id = lot.cost_commodity_id
					AND po.voided_at IS NULL
				ORDER BY po.valuation_date DESC, po.recorded_at DESC, po.id DESC
				LIMIT 1
			) AS latest_price_base_quantity_scale
		FROM investment_lots lot
		WHERE lot.book_id = ?
			AND lot.status = 'open'
			AND lot.remaining_quantity_value <> '0'
		ORDER BY lot.account_id, lot.commodity_id, lot.id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read investment positions: %w", err)
	}
	defer rows.Close()
	type positionKey struct {
		accountID, commodityID, costCommodityID int64
	}
	type accumulatedPosition struct {
		record   InvestmentPositionRecord
		quantity *exact.ScaledInt
		cost     *exact.ScaledInt
	}
	positions := map[positionKey]*accumulatedPosition{}
	var order []positionKey
	for rows.Next() {
		var record InvestmentPositionRecord
		var quantity exact.Coefficient
		var quantityScale int
		var costValue int64
		var costScale int
		if err := rows.Scan(&record.AccountID, &record.CommodityID, &quantity, &quantityScale, &costValue, &costScale, &record.CostCommodityID, &record.LatestPriceValue, &record.LatestPriceScale, &record.LatestPriceDate, &record.LatestPriceBaseQuantityValue, &record.LatestPriceBaseQuantityScale); err != nil {
			return nil, fmt.Errorf("scan investment position: %w", err)
		}
		key := positionKey{record.AccountID, record.CommodityID, record.CostCommodityID}
		position := positions[key]
		if position == nil {
			position = &accumulatedPosition{record: record, quantity: exact.NewScaledInt(), cost: exact.NewScaledInt()}
			positions[key] = position
			order = append(order, key)
		}
		position.quantity.Add(quantity.BigInt(), quantityScale)
		position.cost.Add(big.NewInt(costValue), costScale)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate investment positions: %w", err)
	}
	records := make([]InvestmentPositionRecord, 0, len(order))
	for _, key := range order {
		position := positions[key]
		quantity, err := position.quantity.Coefficient()
		if err != nil {
			return nil, err
		}
		cost, err := position.cost.Int64()
		if err != nil {
			return nil, fmt.Errorf("investment position cost basis: %w", err)
		}
		position.record.QuantityValue = quantity
		position.record.QuantityScale = position.quantity.Scale()
		position.record.RemainingCostBasisValue = cost
		position.record.RemainingCostBasisScale = position.cost.Scale()
		records = append(records, position.record)
	}
	return records, nil
}

func (r *InvestmentRepository) CommodityTradingAccountID(ctx context.Context, bookID int64) (int64, error) {
	var accountID int64
	if err := r.database.QueryRowContext(ctx, `
		SELECT id FROM accounts WHERE book_id = ? AND system_role = 'commodity_trading'
	`, bookID).Scan(&accountID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("read commodity trading account: %w", err)
	}
	return accountID, nil
}

func (r *InvestmentRepository) ListProviderEvents(ctx context.Context, bookID int64) ([]InvestmentProviderEventRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, book_id, source_id, provider_event_id, instrument_id, event_family, event_date, status, normalized_json, raw_json, created_at
		FROM investment_provider_events
		WHERE book_id = ?
		ORDER BY event_date DESC, id DESC
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list provider events: %w", err)
	}
	defer rows.Close()
	var records []InvestmentProviderEventRecord
	for rows.Next() {
		var record InvestmentProviderEventRecord
		if err := rows.Scan(&record.ID, &record.BookID, &record.SourceID, &record.ProviderEventID, &record.InstrumentID, &record.EventFamily, &record.EventDate, &record.Status, &record.NormalizedJSON, &record.RawJSON, &record.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan provider event: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider events: %w", err)
	}
	return records, nil
}

func (r *InvestmentRepository) ListEventSuggestions(ctx context.Context, bookID int64) ([]EventSuggestionRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, book_id, provider_event_id, instrument_id, confidence_bps, status, proposed_transaction_json,
			generated_transaction_id, failure_reason, created_at, updated_at
		FROM investment_event_suggestions
		WHERE book_id = ?
		ORDER BY created_at DESC, id DESC
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list event suggestions: %w", err)
	}
	defer rows.Close()
	var records []EventSuggestionRecord
	for rows.Next() {
		var record EventSuggestionRecord
		if err := rows.Scan(&record.ID, &record.BookID, &record.ProviderEventID, &record.InstrumentID, &record.ConfidenceBPS, &record.Status, &record.ProposedTransactionJSON, &record.GeneratedTransactionID, &record.FailureReason, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan event suggestion: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate event suggestions: %w", err)
	}
	return records, nil
}

func (r *InvestmentRepository) SetSuggestionStatus(ctx context.Context, bookID int64, suggestionID int64, status string, actorUserID int64, authSessionID int64, requestID string, now string) (EventSuggestionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("begin set event suggestion status: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        bookID,
		ActorUserID:   actorUserID,
		AuthSessionID: authSessionID,
		OccurredAt:    now,
		RequestID:     requestID,
		OriginType:    "browser_api",
		Operation:     "investment.event_suggestion.update",
		Reason:        "updated investment event suggestion",
	})
	if err != nil {
		return EventSuggestionRecord{}, err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE investment_event_suggestions
		SET status = ?, updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?
		WHERE book_id = ? AND id = ? AND status = 'suggested'
	`, status, now, actorUserID, auditEventID, bookID, suggestionID)
	if err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("update event suggestion: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("read event suggestion update rows: %w", err)
	}
	if rowsAffected == 0 {
		// Distinguish "doesn't exist" (ErrNotFound) from "exists but is no
		// longer pending" (ErrEventSuggestionNotPending) — eventSuggestionByIDTx
		// itself returns ErrNotFound when the row is genuinely missing.
		if _, err := eventSuggestionByIDTx(ctx, tx, bookID, suggestionID); err != nil {
			return EventSuggestionRecord{}, err
		}
		return EventSuggestionRecord{}, ErrEventSuggestionNotPending
	}
	record, err := eventSuggestionByIDTx(ctx, tx, bookID, suggestionID)
	if err != nil {
		return EventSuggestionRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("commit set event suggestion status: %w", err)
	}
	committed = true
	return record, nil
}

// EventSuggestionByID reads one suggestion outside a transaction.
func (r *InvestmentRepository) EventSuggestionByID(ctx context.Context, bookID int64, suggestionID int64) (EventSuggestionRecord, error) {
	var record EventSuggestionRecord
	err := r.database.QueryRowContext(ctx, `
		SELECT s.id, s.book_id, s.provider_event_id, e.event_family, s.instrument_id,
			s.confidence_bps, s.status, s.proposed_transaction_json,
			s.generated_transaction_id, s.failure_reason, s.created_at, s.updated_at
		FROM investment_event_suggestions s
		JOIN investment_provider_events e ON e.id = s.provider_event_id AND e.book_id = s.book_id
		WHERE s.book_id = ? AND s.id = ?
	`, bookID, suggestionID).Scan(&record.ID, &record.BookID, &record.ProviderEventID, &record.EventFamily, &record.InstrumentID, &record.ConfidenceBPS, &record.Status, &record.ProposedTransactionJSON, &record.GeneratedTransactionID, &record.FailureReason, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EventSuggestionRecord{}, ErrNotFound
	}
	if err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("read event suggestion: %w", err)
	}
	return record, nil
}

// MarkSuggestionAcceptedInTx records a suggestion's generated transaction
// within the same transaction that created it — the postWrite callback for
// InvestmentService.dividendWithPostWrite, so a crash between posting the
// transaction and marking the suggestion accepted is impossible (same
// split-transaction hazard this repo guards against elsewhere, e.g.
// recordCommitIdentityAndMarkRowInTx for imports). 0 rows affected means the
// suggestion is no longer 'suggested' (a concurrent accept/ignore won the
// race); the caller's own transaction rolls back in that case.
func (r *InvestmentRepository) MarkSuggestionAcceptedInTx(ctx context.Context, tx *sql.Tx, bookID int64, suggestionID int64, transactionID int64, now string, actorUserID int64, authSessionID int64, requestID string) error {
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        bookID,
		ActorUserID:   actorUserID,
		AuthSessionID: authSessionID,
		OccurredAt:    now,
		RequestID:     requestID,
		OriginType:    "browser_api",
		Operation:     "investment.event_suggestion.accept",
		Reason:        "accepted investment event suggestion",
	})
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE investment_event_suggestions
		SET status = 'accepted', generated_transaction_id = ?, updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?
		WHERE book_id = ? AND id = ? AND status = 'suggested'
	`, transactionID, now, actorUserID, auditEventID, bookID, suggestionID)
	if err != nil {
		return fmt.Errorf("mark event suggestion accepted: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read mark suggestion accepted rows: %w", err)
	}
	if rowsAffected == 0 {
		return ErrEventSuggestionNotPending
	}
	return nil
}

// MarkSuggestionFailed records why a suggestion's proposed transaction could
// not be posted, without creating any ledger transaction. Guarded the same
// way as MarkSuggestionAcceptedInTx: only a 'suggested' suggestion can
// transition.
func (r *InvestmentRepository) MarkSuggestionFailed(ctx context.Context, bookID int64, suggestionID int64, reason string, now string, actorUserID int64, authSessionID int64, requestID string) (EventSuggestionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("begin mark suggestion failed: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        bookID,
		ActorUserID:   actorUserID,
		AuthSessionID: authSessionID,
		OccurredAt:    now,
		RequestID:     requestID,
		OriginType:    "browser_api",
		Operation:     "investment.event_suggestion.fail",
		Reason:        "investment event suggestion failed to post",
	})
	if err != nil {
		return EventSuggestionRecord{}, err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE investment_event_suggestions
		SET status = 'failed', failure_reason = ?, updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?
		WHERE book_id = ? AND id = ? AND status = 'suggested'
	`, reason, now, actorUserID, auditEventID, bookID, suggestionID)
	if err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("mark event suggestion failed: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("read mark suggestion failed rows: %w", err)
	}
	if rowsAffected == 0 {
		return EventSuggestionRecord{}, ErrEventSuggestionNotPending
	}
	record, err := eventSuggestionByIDTx(ctx, tx, bookID, suggestionID)
	if err != nil {
		return EventSuggestionRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("commit mark suggestion failed: %w", err)
	}
	committed = true
	return record, nil
}

func (r *InvestmentRepository) ListAutomationRules(ctx context.Context, bookID int64) ([]AutomationRuleRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, book_id, source_id, instrument_id, event_family, mode, confidence_threshold_bps,
			required_accounts_json, status, effective_from, effective_to,
			created_at, created_by_user_id, updated_at, updated_by_user_id
		FROM investment_automation_rules
		WHERE book_id = ?
		ORDER BY created_at DESC, id DESC
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list automation rules: %w", err)
	}
	defer rows.Close()
	var records []AutomationRuleRecord
	for rows.Next() {
		var record AutomationRuleRecord
		if err := rows.Scan(&record.ID, &record.BookID, &record.SourceID, &record.InstrumentID, &record.EventFamily, &record.Mode, &record.ConfidenceThresholdBPS, &record.RequiredAccountsJSON, &record.Status, &record.EffectiveFrom, &record.EffectiveTo, &record.CreatedAt, &record.CreatedByUserID, &record.UpdatedAt, &record.UpdatedByUserID); err != nil {
			return nil, fmt.Errorf("scan automation rule: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate automation rules: %w", err)
	}
	return records, nil
}

func (r *InvestmentRepository) SaveAutomationRule(ctx context.Context, params SaveAutomationRuleParams) (AutomationRuleRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return AutomationRuleRecord{}, fmt.Errorf("begin save automation rule: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.RecordedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return AutomationRuleRecord{}, err
	}

	record, err := upsertAutomationRuleTx(ctx, tx, params.BookID, params.RuleID, params.Spec, params.RecordedAt, params.ActorUserID, auditEventID)
	if err != nil {
		return AutomationRuleRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationRuleRecord{}, fmt.Errorf("commit save automation rule: %w", err)
	}
	committed = true
	return record, nil
}

// upsertAutomationRuleTx creates or updates one automation rule within an
// already-open transaction, attributing the write to auditEventID (shared
// across every row touched by the caller's operation).
func upsertAutomationRuleTx(ctx context.Context, tx *sql.Tx, bookID int64, ruleID int64, spec AutomationRuleSpec, recordedAt string, actorUserID int64, auditEventID int64) (AutomationRuleRecord, error) {
	if ruleID > 0 {
		result, err := tx.ExecContext(ctx, `
			UPDATE investment_automation_rules
			SET source_id = ?, instrument_id = ?, event_family = ?, mode = ?,
				confidence_threshold_bps = ?, required_accounts_json = ?, status = ?,
				effective_from = ?, effective_to = ?, updated_at = ?, updated_by_user_id = ?,
				updated_audit_event_id = ?
			WHERE book_id = ? AND id = ?
		`, nullableInt64Value(spec.SourceID), nullableInt64Value(spec.InstrumentID), spec.EventFamily, spec.Mode, spec.ConfidenceThresholdBPS, spec.RequiredAccountsJSON, spec.Status, spec.EffectiveFrom, nullableStringValue(spec.EffectiveTo), recordedAt, actorUserID, auditEventID, bookID, ruleID)
		if err != nil {
			return AutomationRuleRecord{}, fmt.Errorf("update automation rule: %w", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return AutomationRuleRecord{}, fmt.Errorf("read automation rule update rows: %w", err)
		}
		if rowsAffected == 0 {
			return AutomationRuleRecord{}, ErrNotFound
		}
	} else {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO investment_automation_rules (
				book_id, source_id, instrument_id, event_family, mode, confidence_threshold_bps,
				required_accounts_json, status, effective_from, effective_to,
				created_at, created_by_user_id, updated_at, updated_by_user_id,
				created_audit_event_id, updated_audit_event_id
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, bookID, nullableInt64Value(spec.SourceID), nullableInt64Value(spec.InstrumentID), spec.EventFamily, spec.Mode, spec.ConfidenceThresholdBPS, spec.RequiredAccountsJSON, spec.Status, spec.EffectiveFrom, nullableStringValue(spec.EffectiveTo), recordedAt, actorUserID, recordedAt, actorUserID, auditEventID, auditEventID)
		if err != nil {
			return AutomationRuleRecord{}, fmt.Errorf("insert automation rule: %w", err)
		}
		ruleID, err = result.LastInsertId()
		if err != nil {
			return AutomationRuleRecord{}, fmt.Errorf("read automation rule id: %w", err)
		}
	}

	return automationRuleByIDTx(ctx, tx, bookID, ruleID)
}

// AutomationRuleUpsert is one rule within a ReplaceAutomationRules call.
type AutomationRuleUpsert struct {
	RuleID int64
	Spec   AutomationRuleSpec
}

// ReplaceAutomationRulesParams describes a full-set replace of a book's
// active automation rules.
type ReplaceAutomationRulesParams struct {
	BookID        int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	RecordedAt    string
	ChangeReason  string
	Rules         []AutomationRuleUpsert
}

// ReplaceAutomationRules upserts every rule in params.Rules and archives any
// other active rule for the book that isn't among them, all in one
// transaction sharing one audit event — the "replace" semantics the API
// contract documents (an active auto_post rule silently surviving an
// omission would keep posting real trades unattended).
func (r *InvestmentRepository) ReplaceAutomationRules(ctx context.Context, params ReplaceAutomationRulesParams) ([]AutomationRuleRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin replace automation rules: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.RecordedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return nil, err
	}

	records := make([]AutomationRuleRecord, 0, len(params.Rules))
	touchedIDs := make([]int64, 0, len(params.Rules))
	for _, rule := range params.Rules {
		record, err := upsertAutomationRuleTx(ctx, tx, params.BookID, rule.RuleID, rule.Spec, params.RecordedAt, params.ActorUserID, auditEventID)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
		touchedIDs = append(touchedIDs, record.ID)
	}

	archiveQuery := `
		UPDATE investment_automation_rules
		SET status = 'archived', updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?
		WHERE book_id = ? AND status = 'active'`
	archiveArgs := []any{params.RecordedAt, params.ActorUserID, auditEventID, params.BookID}
	if len(touchedIDs) > 0 {
		placeholders := make([]string, len(touchedIDs))
		for i, id := range touchedIDs {
			placeholders[i] = "?"
			archiveArgs = append(archiveArgs, id)
		}
		archiveQuery += ` AND id NOT IN (` + strings.Join(placeholders, ", ") + `)`
	}
	if _, err := tx.ExecContext(ctx, archiveQuery, archiveArgs...); err != nil {
		return nil, fmt.Errorf("archive superseded automation rules: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit replace automation rules: %w", err)
	}
	committed = true
	return records, nil
}

func insertInvestmentInstrumentVersion(ctx context.Context, tx *sql.Tx, instrumentID int64, versionSeq int64, spec InvestmentInstrumentSpec, commodityID int64, effectiveFrom string, recordedAt string, actorUserID int64, changeReason string, auditEventID int64) (int64, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO investment_instrument_versions (
			instrument_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
			status, instrument_type, display_name, symbol, exchange_code, mic, issuer, country_code,
			quote_commodity_id, trading_commodity_id, quantity_scale, price_scale,
			identifiers_json, metadata_json, change_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, ?, 'active', ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
			NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?)
	`, instrumentID, versionSeq, effectiveFrom, recordedAt, actorUserID, changeReason, spec.InstrumentType, spec.DisplayName, spec.Symbol, spec.ExchangeCode, spec.MIC, spec.Issuer, spec.CountryCode, nullableInt64Value(spec.QuoteCommodityID), nullableInt64Value(spec.TradingCommodityID), spec.QuantityScale, spec.PriceScale, spec.IdentifiersJSON, spec.MetadataJSON, auditEventID)
	if err != nil {
		return 0, fmt.Errorf("insert investment instrument version: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read investment instrument version id: %w", err)
	}
	return id, nil
}

func investmentInstrumentByIDTx(ctx context.Context, tx *sql.Tx, bookID int64, instrumentID int64) (InvestmentInstrumentRecord, error) {
	var record InvestmentInstrumentRecord
	if err := scanInvestmentInstrumentRecord(tx.QueryRowContext(ctx, investmentInstrumentSelect(`
		WHERE ii.book_id = ? AND ii.id = ?
	`), bookID, instrumentID), &record); err != nil {
		return InvestmentInstrumentRecord{}, err
	}
	return record, nil
}

func investmentInstrumentSelect(whereClause string) string {
	return `
		SELECT
			ii.id, ii.book_id, ii.commodity_id, c.code, c.is_builtin, ii.created_at, ii.created_by_user_id,
			iiv.id, iiv.version_seq, iiv.effective_from, iiv.recorded_at, iiv.changed_by_user_id,
			iiv.change_reason, iiv.status, iiv.instrument_type, iiv.display_name, iiv.symbol,
			iiv.exchange_code, iiv.mic, iiv.issuer, iiv.country_code, iiv.quote_commodity_id,
			iiv.trading_commodity_id, iiv.quantity_scale, iiv.price_scale, iiv.identifiers_json, iiv.metadata_json
		FROM investment_instruments ii
		JOIN commodities c ON c.id = ii.commodity_id
		JOIN current_investment_instrument_versions iiv ON iiv.instrument_id = ii.id
	` + whereClause
}

func scanInvestmentInstrumentRecord(row rowScanner, record *InvestmentInstrumentRecord) error {
	var isBuiltin int
	if err := row.Scan(
		&record.ID, &record.BookID, &record.CommodityID, &record.CommodityCode, &isBuiltin,
		&record.CreatedAt, &record.CreatedByUserID, &record.VersionID, &record.VersionSeq,
		&record.EffectiveFrom, &record.RecordedAt, &record.ChangedByUserID, &record.ChangeReason,
		&record.Status, &record.InstrumentType, &record.DisplayName, &record.Symbol,
		&record.ExchangeCode, &record.MIC, &record.Issuer, &record.CountryCode,
		&record.QuoteCommodityID, &record.TradingCommodityID, &record.QuantityScale,
		&record.PriceScale, &record.IdentifiersJSON, &record.MetadataJSON,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("scan investment instrument: %w", err)
	}
	record.IsBuiltin = isBuiltin == 1
	return nil
}

func scanInvestmentInstrumentRecords(rows *sql.Rows) ([]InvestmentInstrumentRecord, error) {
	var records []InvestmentInstrumentRecord
	for rows.Next() {
		var record InvestmentInstrumentRecord
		if err := scanInvestmentInstrumentRecord(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate investment instruments: %w", err)
	}
	return records, nil
}

func scanCostBasisProfileRow(row rowScanner) (CostBasisProfileRecord, error) {
	var record CostBasisProfileRecord
	var isDefault int
	if err := row.Scan(&record.ID, &record.BookID, &record.Name, &record.Method, &isDefault, &record.Status, &record.Description, &record.MetadataJSON, &record.CreatedAt, &record.CreatedByUserID, &record.UpdatedAt, &record.UpdatedByUserID, &record.VersionID, &record.VersionSeq); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CostBasisProfileRecord{}, ErrNotFound
		}
		return CostBasisProfileRecord{}, fmt.Errorf("scan cost basis profile: %w", err)
	}
	record.IsDefault = isDefault == 1
	return record, nil
}

func scanCostBasisProfiles(rows *sql.Rows) ([]CostBasisProfileRecord, error) {
	var records []CostBasisProfileRecord
	for rows.Next() {
		record, err := scanCostBasisProfileRow(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cost basis profiles: %w", err)
	}
	return records, nil
}

func costBasisProfileByIDTx(ctx context.Context, tx *sql.Tx, bookID int64, profileID int64) (CostBasisProfileRecord, error) {
	return scanCostBasisProfileRow(tx.QueryRowContext(ctx, `
		SELECT profile.id, profile.book_id, profile.name, profile.method, profile.is_default,
			profile.status, profile.description, profile.metadata_json, profile.created_at,
			profile.created_by_user_id, profile.updated_at, profile.updated_by_user_id,
			version.id, version.version_seq
		FROM cost_basis_profiles profile
		JOIN cost_basis_profile_versions version ON version.id = profile.current_version_id
		WHERE profile.book_id = ? AND profile.id = ?
	`, bookID, profileID))
}

func scanDividendDefaultRow(row rowScanner) (DividendDefaultRecord, error) {
	var record DividendDefaultRecord
	if err := row.Scan(&record.ID, &record.BookID, &record.CommodityID, &record.IncomeAccountID, &record.WithholdingAccountID, &record.DefaultWithholdingValue, &record.DefaultWithholdingScale, &record.WithholdingRateBPS, &record.TaxCountryCode, &record.TaxTreatment, &record.Status, &record.EffectiveFrom, &record.EffectiveTo, &record.MetadataJSON, &record.CreatedAt, &record.CreatedByUserID, &record.UpdatedAt, &record.UpdatedByUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DividendDefaultRecord{}, ErrNotFound
		}
		return DividendDefaultRecord{}, fmt.Errorf("scan dividend default: %w", err)
	}
	return record, nil
}

func scanDividendDefaults(rows *sql.Rows) ([]DividendDefaultRecord, error) {
	var records []DividendDefaultRecord
	for rows.Next() {
		record, err := scanDividendDefaultRow(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dividend defaults: %w", err)
	}
	return records, nil
}

func dividendDefaultByIDTx(ctx context.Context, tx *sql.Tx, bookID int64, defaultID int64) (DividendDefaultRecord, error) {
	return scanDividendDefaultRow(tx.QueryRowContext(ctx, `
		SELECT id, book_id, commodity_id, income_account_id, withholding_account_id, default_withholding_value,
			default_withholding_scale, withholding_rate_bps, tax_country_code, tax_treatment, status,
			effective_from, effective_to, metadata_json, created_at, created_by_user_id, updated_at, updated_by_user_id
		FROM dividend_defaults
		WHERE book_id = ? AND id = ?
	`, bookID, defaultID))
}

func investmentLotSelect(whereClause string) string {
	return `
		SELECT id, book_id, account_id, commodity_id, opened_on, source_transaction_id, status,
			quantity_value, quantity_scale, remaining_quantity_value, remaining_quantity_scale,
			cost_basis_value, cost_basis_scale, remaining_cost_basis_value, remaining_cost_basis_scale,
			cost_commodity_id, metadata_json, created_at, updated_at
		FROM investment_lots
	` + whereClause
}

func investmentLotByIDTx(ctx context.Context, tx *sql.Tx, bookID int64, lotID int64) (InvestmentLotRecord, error) {
	return scanInvestmentLotRow(tx.QueryRowContext(ctx, investmentLotSelect(`
		WHERE book_id = ? AND id = ?
	`), bookID, lotID))
}

func scanInvestmentLotRow(row rowScanner) (InvestmentLotRecord, error) {
	var record InvestmentLotRecord
	if err := row.Scan(&record.ID, &record.BookID, &record.AccountID, &record.CommodityID, &record.OpenedOn, &record.SourceTransactionID, &record.Status, &record.QuantityValue, &record.QuantityScale, &record.RemainingQuantityValue, &record.RemainingQuantityScale, &record.CostBasisValue, &record.CostBasisScale, &record.RemainingCostBasisValue, &record.RemainingCostBasisScale, &record.CostCommodityID, &record.MetadataJSON, &record.CreatedAt, &record.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return InvestmentLotRecord{}, ErrNotFound
		}
		return InvestmentLotRecord{}, fmt.Errorf("scan investment lot: %w", err)
	}
	return record, nil
}

func scanInvestmentLots(rows *sql.Rows) ([]InvestmentLotRecord, error) {
	var records []InvestmentLotRecord
	for rows.Next() {
		record, err := scanInvestmentLotRow(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate investment lots: %w", err)
	}
	return records, nil
}

// Cost-basis allocation precision (T-103).
//
// A partial disposal splits a lot's — or an average-cost pool's — recorded
// basis in the ratio of the quantities. That split used to happen at whichever
// scale the purchase happened to be recorded at, which made the *text* of an
// input into an accounting policy: three shares bought for exactly 10 EUR
// allocated 3 EUR of basis to one share when the amount was entered as "10",
// 3.33 when entered as "10.00", and 3.3333 when entered as "10.0000". All
// three are the same acquisition and must allocate the same basis.
//
// The policy is the cost commodity's own maximum scale: the same ceiling every
// amount in that currency is already held to, so it can only ever deepen a
// recorded basis. It is deliberately not the currency's *standard* scale,
// which would be the natural choice for presentation but cannot be used here —
// a basis legitimately recorded deeper than it (10.0001 EUR in a currency
// displayed to two places) would have to be truncated to reach it, destroying
// recorded basis to make a rounding rule fit. Nothing about an allocation may
// lose money that was actually spent.
//
// Truncating division and residual conservation are unchanged: the remainder
// stays in the lot, so the basis a position holds plus the basis it has
// disposed of still equals what it cost. What changes is only the precision
// the split is taken at, and that it no longer varies with the input.

// recordedBasis is one stored basis figure and the scale it is stored at.
type recordedBasis struct {
	value int64
	scale int
}

// costBasisAllocationScaleTx resolves the allocation scale for a disposal
// against the cost commodity as it stands on the event date.
func costBasisAllocationScaleTx(ctx context.Context, tx *sql.Tx, costCommodityID int64, eventDate string, recorded []recordedBasis) (int, error) {
	var ceiling int
	if err := tx.QueryRowContext(ctx, `
		SELECT cv.max_quantity_scale
		FROM commodity_versions cv
		WHERE cv.commodity_id = ?
			AND cv.id = (
				SELECT asof_cv.id
				FROM commodity_versions asof_cv
				WHERE asof_cv.commodity_id = cv.commodity_id
					AND asof_cv.effective_from <= ?
				ORDER BY asof_cv.effective_from DESC, asof_cv.version_seq DESC
				LIMIT 1
			)
	`, costCommodityID, eventDate).Scan(&ceiling); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("%w: cost commodity %d has no version on %s", ErrInvalidDisposalParams, costCommodityID, eventDate)
		}
		return 0, fmt.Errorf("read cost commodity allocation scale: %w", err)
	}
	scale := costBasisAllocationScale(ceiling, recorded)
	if !basisFitsInt64At(recorded, scale) {
		return 0, ErrInvestmentBasisRange
	}
	return scale, nil
}

// costBasisAllocationScale picks the deepest scale at or below ceiling that
// every recorded basis can be restated to and still fit the int64 the
// projection columns are, and never one shallower than a basis already stored.
//
// The backoff is the explicit answer to coefficient range. It keys on the
// magnitude of the basis rather than on how it was written, so two spellings
// of the same amount still reach the same scale — which is the property this
// whole policy exists to hold.
func costBasisAllocationScale(ceiling int, recorded []recordedBasis) int {
	deepest := 0
	for _, basis := range recorded {
		if basis.scale > deepest {
			deepest = basis.scale
		}
	}
	if ceiling < deepest {
		// A basis was recorded deeper than the commodity now allows. Narrowing
		// it would discard money that is already on the books, so the recorded
		// precision wins.
		return deepest
	}
	scale := ceiling
	for scale > deepest && !basisFitsInt64At(recorded, scale) {
		scale--
	}
	return scale
}

// basisFitsInt64At checks every restated figure *and* their total. The total
// matters because a position's basis is summed — by the projection rows a
// disposal rewrites and by every caller that adds them up — so a scale at
// which each lot fits alone but the position does not is no use.
func basisFitsInt64At(recorded []recordedBasis, scale int) bool {
	total := new(big.Int)
	for _, basis := range recorded {
		restated := big.NewInt(basis.value)
		if basis.scale < scale {
			restated.Mul(restated, exact.Pow10(scale-basis.scale))
		}
		if !restated.IsInt64() {
			return false
		}
		total.Add(total, new(big.Int).Abs(restated))
	}
	return total.IsInt64()
}

// positionBasisAllocationScaleTx resolves the one allocation scale a disposal
// uses, over every open lot of the position that is eligible on its event
// date — not per lot. Two lots of one position must split their basis at the
// same precision: a scale chosen per lot would vary with each lot's magnitude
// and put a single position's projection rows back at mixed scales, which is
// the state T-97 removed.
func positionBasisAllocationScaleTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams) (int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT remaining_cost_basis_value, remaining_cost_basis_scale
		FROM investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ?
			AND status = 'open' AND opened_on <= ?
	`, params.BookID, params.AccountID, params.CommodityID, params.CostCommodityID, params.EventDate)
	if err != nil {
		return 0, fmt.Errorf("read position basis for allocation scale: %w", err)
	}
	defer rows.Close()
	var recorded []recordedBasis
	for rows.Next() {
		var basis recordedBasis
		if err := rows.Scan(&basis.value, &basis.scale); err != nil {
			return 0, fmt.Errorf("scan position basis for allocation scale: %w", err)
		}
		recorded = append(recorded, basis)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate position basis for allocation scale: %w", err)
	}
	return costBasisAllocationScaleTx(ctx, tx, params.CostCommodityID, params.EventDate, recorded)
}

func disposeLotTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams, lotID int64, quantityValue exact.Coefficient, quantityScale int, auditEventID int64, allocationScale int) (LotDisposalRecord, error) {
	lot, err := investmentLotByIDTx(ctx, tx, params.BookID, lotID)
	if err != nil {
		return LotDisposalRecord{}, err
	}
	if lot.AccountID != params.AccountID || lot.CommodityID != params.CommodityID ||
		lot.CostCommodityID != params.CostCommodityID || lot.Status != "open" {
		return LotDisposalRecord{}, ErrNotFound
	}
	// Temporal eligibility (T-95). A disposal may only consume shares that were
	// already held on its own event date; same-day acquisition and disposal is
	// legitimate, so the boundary is inclusive. The selection queries above
	// filter future lots out so FIFO/LIFO/average-cost pick the right eligible
	// lot instead of failing here, but specific_lot names a lot directly and
	// every method — sale, write-off, preview — funnels through this function,
	// so this is the check no disposal path can bypass. Without it a May sale
	// consumes a June acquisition: the position goes negative in history and
	// the disposal's basis comes from shares that did not exist yet.
	if lot.OpenedOn > params.EventDate {
		return LotDisposalRecord{}, fmt.Errorf("%w: lot %d was acquired on %s, after the disposal date %s", ErrInvalidDisposalParams, lotID, lot.OpenedOn, params.EventDate)
	}
	// The disposal and the lot need to be at a common scale for the comparison,
	// the subtraction and the basis proration below — they do not need to be at
	// the *lot's* scale, which is what this used to insist on (T-97). Widen both
	// to whichever carries more precision; neither side ever narrows, so no
	// quantity is rounded away to make the arithmetic line up.
	commonScale := quantityScale
	if lot.RemainingQuantityScale > commonScale {
		commonScale = lot.RemainingQuantityScale
	}
	disposedAtCommon, err := rescaleQuantity(quantityValue, quantityScale, commonScale)
	if err != nil {
		return LotDisposalRecord{}, err
	}
	remainingAtCommon, err := rescaleQuantity(lot.RemainingQuantityValue, lot.RemainingQuantityScale, commonScale)
	if err != nil {
		return LotDisposalRecord{}, err
	}
	if disposedAtCommon.Sign() <= 0 || disposedAtCommon.Cmp(remainingAtCommon) > 0 {
		return LotDisposalRecord{}, ErrInsufficientLots
	}
	// The basis is split at the position's allocation scale, not at whichever
	// scale this purchase was typed at (T-103).
	remainingCost, err := exact.ScaledIntFromInt64(lot.RemainingCostBasisValue, lot.RemainingCostBasisScale).TruncatedTo(allocationScale).Int64()
	if err != nil {
		return LotDisposalRecord{}, fmt.Errorf("restate lot %d cost basis to scale %d: %w", lotID, allocationScale, err)
	}
	// Both quantities are at commonScale, so the ratio is unaffected by which
	// scale that is.
	costBasisValue := proratedCostBasis(remainingCost, disposedAtCommon, remainingAtCommon)
	nextRemainingQuantity, err := exact.FromBig(new(big.Int).Sub(remainingAtCommon.BigInt(), disposedAtCommon.BigInt()))
	if err != nil {
		return LotDisposalRecord{}, err
	}
	nextRemainingCost := remainingCost - costBasisValue
	status := "open"
	if nextRemainingQuantity.Sign() == 0 {
		status = "closed"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE investment_lots
		SET remaining_quantity_value = ?, remaining_quantity_scale = ?,
			remaining_cost_basis_value = ?, remaining_cost_basis_scale = ?, status = ?,
			updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?
		WHERE book_id = ? AND id = ?
	`, nextRemainingQuantity, commonScale, nextRemainingCost, allocationScale, status, params.CreatedAt, params.ActorUserID, auditEventID, params.BookID, lotID); err != nil {
		return LotDisposalRecord{}, fmt.Errorf("update disposed investment lot: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO investment_lot_events (
			book_id, lot_id, event_kind, transaction_id, event_date, quantity_value, quantity_scale,
			cost_basis_value, cost_basis_scale, cost_basis_method, metadata_json,
			created_at, created_by_user_id, created_audit_event_id
		)
		VALUES (?, ?, 'disposal', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, params.BookID, lotID, nullablePositiveInt64(params.TransactionID), params.EventDate,
		quantityValue.Negated(), quantityScale, -costBasisValue, allocationScale,
		params.CostBasisMethod, params.MetadataJSON, params.CreatedAt, params.ActorUserID, auditEventID)
	if err != nil {
		return LotDisposalRecord{}, fmt.Errorf("insert disposal lot event: %w", err)
	}
	eventID, err := result.LastInsertId()
	if err != nil {
		return LotDisposalRecord{}, fmt.Errorf("read disposal lot event id: %w", err)
	}
	return LotDisposalRecord{
		EventID:         eventID,
		LotID:           lotID,
		QuantityValue:   quantityValue,
		QuantityScale:   quantityScale,
		CostBasisValue:  costBasisValue,
		CostBasisScale:  allocationScale,
		CostCommodityID: lot.CostCommodityID,
	}, nil
}

func proratedCostBasis(remainingCost int64, quantity exact.Coefficient, remainingQuantity exact.Coefficient) int64 {
	if remainingQuantity.Sign() <= 0 {
		return 0
	}
	value := new(big.Int).Mul(big.NewInt(remainingCost), quantity.BigInt())
	value.Quo(value, remainingQuantity.BigInt())
	return value.Int64()
}

func eventSuggestionByIDTx(ctx context.Context, tx *sql.Tx, bookID int64, suggestionID int64) (EventSuggestionRecord, error) {
	var record EventSuggestionRecord
	if err := tx.QueryRowContext(ctx, `
		SELECT id, book_id, provider_event_id, instrument_id, confidence_bps, status, proposed_transaction_json,
			generated_transaction_id, failure_reason, created_at, updated_at
		FROM investment_event_suggestions
		WHERE book_id = ? AND id = ?
	`, bookID, suggestionID).Scan(&record.ID, &record.BookID, &record.ProviderEventID, &record.InstrumentID, &record.ConfidenceBPS, &record.Status, &record.ProposedTransactionJSON, &record.GeneratedTransactionID, &record.FailureReason, &record.CreatedAt, &record.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EventSuggestionRecord{}, ErrNotFound
		}
		return EventSuggestionRecord{}, fmt.Errorf("scan event suggestion: %w", err)
	}
	return record, nil
}

func automationRuleByIDTx(ctx context.Context, tx *sql.Tx, bookID int64, ruleID int64) (AutomationRuleRecord, error) {
	var record AutomationRuleRecord
	if err := tx.QueryRowContext(ctx, `
		SELECT id, book_id, source_id, instrument_id, event_family, mode, confidence_threshold_bps,
			required_accounts_json, status, effective_from, effective_to,
			created_at, created_by_user_id, updated_at, updated_by_user_id
		FROM investment_automation_rules
		WHERE book_id = ? AND id = ?
	`, bookID, ruleID).Scan(&record.ID, &record.BookID, &record.SourceID, &record.InstrumentID, &record.EventFamily, &record.Mode, &record.ConfidenceThresholdBPS, &record.RequiredAccountsJSON, &record.Status, &record.EffectiveFrom, &record.EffectiveTo, &record.CreatedAt, &record.CreatedByUserID, &record.UpdatedAt, &record.UpdatedByUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AutomationRuleRecord{}, ErrNotFound
		}
		return AutomationRuleRecord{}, fmt.Errorf("scan automation rule: %w", err)
	}
	return record, nil
}

type RealizedGainsParams struct {
	From string // optional ISO date, inclusive; empty = unbounded
	To   string // optional ISO date, inclusive; empty = unbounded
}

type RealizedGainRecord struct {
	AccountID          int64
	CommodityID        int64
	CostCommodityID    int64
	DisposalDate       string
	TransactionID      sql.NullInt64
	QuantityValue      exact.Coefficient
	QuantityScale      int
	DisposedBasisValue int64
	DisposedBasisScale int
	ProceedsValue      int64
	ProceedsScale      int
	RealizedGainValue  int64
	// RealizedGainScale is the gain's own scale, which is the deeper of the
	// proceeds and disposed-basis scales. It is not always the proceeds scale:
	// 11 EUR of proceeds entered at scale 0 against a 10.99 EUR basis yields
	// 0.01 EUR, a figure scale 0 cannot hold (T-101).
	RealizedGainScale int
}

type realizedGainEventRow struct {
	id              int64
	transactionID   sql.NullInt64
	eventDate       string
	accountID       int64
	commodityID     int64
	costCommodityID int64
	quantityValue   exact.Coefficient
	quantityScale   int
	costBasisValue  int64
	costBasisScale  int
}

// ListRealizedGains returns one row per disposal event group (investment_lot_events
// with event_kind='disposal'), aggregated at the transaction level so a single sell
// transaction that disposes multiple lots produces one realized-gain row (manual
// disposals, which have no transaction_id, each keep their own row). All summation
// is done in Go with big.Int at aligned scales — disposal events and cash postings
// can legitimately carry different quantity/cost scales (e.g. imported trades whose
// raw source amounts had differing decimal-place counts), so SQL SUM/MAX over the
// TEXT coefficient columns would silently blend incommensurate magnitudes.
//
// Cash proceeds are read from the matching sell transaction's postings: any posting
// whose commodity_id matches the lot's cost_commodity_id and whose account does not
// have system_role='commodity_trading' (i.e. a real cash account, not the internal
// clearing account) — summed, since a sale can legitimately have more than one such
// leg (e.g. a fee posted alongside the proceeds). Realized gain = proceeds -
// disposed_basis, aligned to the proceeds scale.
func (r *InvestmentRepository) ListRealizedGains(ctx context.Context, bookID int64, params RealizedGainsParams) ([]RealizedGainRecord, error) {
	dateFilter := ""
	args := []any{bookID}
	if params.From != "" {
		dateFilter += " AND le.event_date >= ?"
		args = append(args, params.From)
	}
	if params.To != "" {
		dateFilter += " AND le.event_date <= ?"
		args = append(args, params.To)
	}

	rows, err := r.database.QueryContext(ctx, `
		SELECT
			le.id,
			le.transaction_id,
			le.event_date,
			lot.account_id,
			lot.commodity_id,
			lot.cost_commodity_id,
			le.quantity_value,
			le.quantity_scale,
			le.cost_basis_value,
			le.cost_basis_scale
		FROM investment_lot_events le
		JOIN investment_lots lot ON lot.id = le.lot_id
		WHERE lot.book_id = ?
			AND le.event_kind = 'disposal'
			`+dateFilter+`
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list realized gain events: %w", err)
	}
	var events []realizedGainEventRow
	for rows.Next() {
		var e realizedGainEventRow
		if err := rows.Scan(&e.id, &e.transactionID, &e.eventDate, &e.accountID, &e.commodityID, &e.costCommodityID, &e.quantityValue, &e.quantityScale, &e.costBasisValue, &e.costBasisScale); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan realized gain event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close realized gain events: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate realized gain events: %w", err)
	}
	if len(events) == 0 {
		return nil, nil
	}

	proceeds, err := r.realizedGainCashProceeds(ctx, bookID)
	if err != nil {
		return nil, err
	}

	// Group key mirrors the prior SQL GROUP BY: (transaction_id or, for manual
	// disposals with no transaction, the lot event's own id), account, commodity,
	// cost commodity, event date.
	type groupKey struct {
		txKey           string
		accountID       int64
		commodityID     int64
		costCommodityID int64
		eventDate       string
	}
	type group struct {
		key           groupKey
		transactionID sql.NullInt64
		minEventID    int64
		quantity      *exact.ScaledInt
		costBasis     *exact.ScaledInt
	}
	groups := map[groupKey]*group{}
	var order []groupKey
	for _, e := range events {
		key := groupKey{accountID: e.accountID, commodityID: e.commodityID, costCommodityID: e.costCommodityID, eventDate: e.eventDate}
		if e.transactionID.Valid {
			key.txKey = fmt.Sprintf("t:%d", e.transactionID.Int64)
		} else {
			key.txKey = fmt.Sprintf("e:%d", e.id)
		}
		g := groups[key]
		if g == nil {
			g = &group{key: key, transactionID: e.transactionID, minEventID: e.id, quantity: exact.NewScaledInt(), costBasis: exact.NewScaledInt()}
			groups[key] = g
			order = append(order, key)
		}
		if e.id < g.minEventID {
			g.minEventID = e.id
		}
		g.quantity.Add(e.quantityValue.BigInt(), e.quantityScale)
		g.costBasis.Add(big.NewInt(e.costBasisValue), e.costBasisScale)
	}

	// Replicate ORDER BY le.event_date DESC, MIN(le.id) DESC.
	slices.SortFunc(order, func(a, b groupKey) int {
		ga, gb := groups[a], groups[b]
		if c := cmp.Compare(gb.key.eventDate, ga.key.eventDate); c != 0 {
			return c
		}
		return cmp.Compare(gb.minEventID, ga.minEventID)
	})

	records := make([]RealizedGainRecord, 0, len(order))
	for _, key := range order {
		g := groups[key]
		// quantity_value in disposal events is stored negated (reduction of
		// holding); negate back to expose the sold quantity as positive.
		quantityValue, err := g.quantity.Negated().Coefficient()
		if err != nil {
			return nil, err
		}
		disposedBasis, err := g.costBasis.Int64()
		if err != nil {
			return nil, fmt.Errorf("realized gain disposed basis: %w", err)
		}
		record := RealizedGainRecord{
			AccountID:          key.accountID,
			CommodityID:        key.commodityID,
			CostCommodityID:    key.costCommodityID,
			DisposalDate:       key.eventDate,
			TransactionID:      g.transactionID,
			QuantityValue:      quantityValue,
			QuantityScale:      g.quantity.Scale(),
			DisposedBasisValue: disposedBasis,
			DisposedBasisScale: g.costBasis.Scale(),
		}

		var matchedProceeds *exact.ScaledInt
		if g.transactionID.Valid {
			matchedProceeds = proceeds[proceedsKey{transactionID: g.transactionID.Int64, costCommodityID: key.costCommodityID}]
		}
		if matchedProceeds != nil {
			proceedsValue, err := matchedProceeds.Int64()
			if err != nil {
				return nil, fmt.Errorf("realized gain proceeds: %w", err)
			}
			record.ProceedsValue = proceedsValue
			record.ProceedsScale = matchedProceeds.Scale()
		} else {
			// No matching transaction or posting found (e.g. manual lot creation).
			record.ProceedsValue = 0
			record.ProceedsScale = record.DisposedBasisScale
		}

		// disposed_basis_value is stored as a negative number (the lot event records
		// the deduction from cost basis). Gain = proceeds − |cost| = proceeds + disposed_basis.
		//
		// The two operands are added at whichever scale is deeper, because the
		// scale a figure was *entered* at says nothing about the precision its
		// difference needs: restating the basis to the proceeds scale first
		// truncated 10.99 EUR of cost to 10 EUR and turned a one-cent gain into
		// a one-euro one, and could as easily hide a loss (T-101). Add
		// deepens the accumulator on its own, so the result carries the exact
		// difference and reports the scale it needs.
		gain := exact.ScaledIntFromInt64(record.ProceedsValue, record.ProceedsScale)
		gain.AddScaled(g.costBasis)
		gainValue, err := gain.Int64()
		if err != nil {
			return nil, fmt.Errorf("realized gain value: %w", err)
		}
		record.RealizedGainValue = gainValue
		record.RealizedGainScale = gain.Scale()
		records = append(records, record)
	}
	return records, nil
}

type proceedsKey struct {
	transactionID   int64
	costCommodityID int64
}

// realizedGainCashProceeds sums, per (transaction, cost commodity), the real cash
// legs of every disposal-bearing sell transaction in the book: postings whose
// commodity matches the lot's cost commodity and whose account is not the internal
// commodity_trading clearing account. Deduplicated by posting id because a
// multi-lot sale joins to the same cash posting once per disposed lot event.
func (r *InvestmentRepository) realizedGainCashProceeds(ctx context.Context, bookID int64) (map[proceedsKey]*exact.ScaledInt, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT DISTINCT
			le2.transaction_id,
			lot2.cost_commodity_id,
			cash_pv.id,
			cash_pv.quantity_value,
			cash_pv.quantity_scale
		FROM investment_lot_events le2
		JOIN investment_lots lot2 ON lot2.id = le2.lot_id
		JOIN current_transaction_versions tv2 ON tv2.transaction_id = le2.transaction_id
		JOIN journal_entries je2 ON je2.transaction_version_id = tv2.id
		JOIN posting_versions cash_pv ON cash_pv.journal_entry_id = je2.id
		WHERE lot2.book_id = ?
			AND le2.event_kind = 'disposal'
			AND le2.transaction_id IS NOT NULL
			AND cash_pv.commodity_id = lot2.cost_commodity_id
			AND NOT EXISTS (
				SELECT 1 FROM accounts a
				WHERE a.id = cash_pv.account_id AND a.system_role = 'commodity_trading'
			)
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read realized gain cash proceeds: %w", err)
	}
	defer rows.Close()

	proceeds := map[proceedsKey]*exact.ScaledInt{}
	for rows.Next() {
		var transactionID int64
		var costCommodityID int64
		var postingID int64
		var quantityValue exact.Coefficient
		var quantityScale int
		if err := rows.Scan(&transactionID, &costCommodityID, &postingID, &quantityValue, &quantityScale); err != nil {
			return nil, fmt.Errorf("scan realized gain cash proceeds: %w", err)
		}
		if quantityValue.Sign() <= 0 {
			continue
		}
		key := proceedsKey{transactionID: transactionID, costCommodityID: costCommodityID}
		total := proceeds[key]
		if total == nil {
			total = exact.NewScaledInt()
			proceeds[key] = total
		}
		total.Add(quantityValue.BigInt(), quantityScale)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate realized gain cash proceeds: %w", err)
	}
	return proceeds, nil
}

type UnrealizedGainRecord struct {
	AccountID               int64
	CommodityID             int64
	CostCommodityID         int64
	QuantityValue           exact.Coefficient
	QuantityScale           int
	RemainingCostBasisValue int64
	RemainingCostBasisScale int
	LatestPriceValue        sql.NullInt64
	LatestPriceScale        sql.NullInt64
	LatestPriceDate         sql.NullString
	MarketValueValue        *int64
	MarketValueScale        *int
	UnrealizedGainValue     *int64
	UnrealizedGainScale     *int
	ValuationUnavailable    string
}

// Valuation unavailability reasons reported by PositionsWithGains when market
// value and unrealized gain are absent. They are distinct outcomes: a position
// nobody has priced is ordinary, while one whose value the read model cannot
// represent is a defect to surface rather than disguise as a missing price.
const (
	ValuationNoPrice         = "no_price"
	ValuationUnrepresentable = "unrepresentable"
)

// int64AtUsableScale restates a computed valuation so it fits the read model's
// int64 columns, reporting whether it could. The scale it was computed at is
// kept whenever the coefficient already fits, so ordinary positions report
// exactly the precision they always have; only a value that would otherwise be
// dropped is restated by removing the trailing zeros it does not need (T-99).
// A product of quantity and price carries the sum of their scales, so widening
// a lot's quantity scale by six places during a fractional disposal multiplies
// the reported market value's coefficient by a million without changing what
// it is worth.
func int64AtUsableScale(value *exact.ScaledInt) (int64, int, bool) {
	if fitted, err := value.Int64(); err == nil {
		return fitted, value.Scale(), true
	}
	normalized := value.Normalized()
	fitted, err := normalized.Int64()
	if err != nil {
		return 0, 0, false
	}
	return fitted, normalized.Scale(), true
}

// PositionsWithGains returns all open positions with unrealized gain computed when a
// price observation is available. Absent gain fields carry a ValuationUnavailable
// reason saying whether no price is known or the value cannot be represented.
func (r *InvestmentRepository) PositionsWithGains(ctx context.Context, bookID int64) ([]UnrealizedGainRecord, error) {
	positions, err := r.Positions(ctx, bookID)
	if err != nil {
		return nil, err
	}
	records := make([]UnrealizedGainRecord, 0, len(positions))
	for _, pos := range positions {
		record := UnrealizedGainRecord{
			AccountID:               pos.AccountID,
			CommodityID:             pos.CommodityID,
			CostCommodityID:         pos.CostCommodityID,
			QuantityValue:           pos.QuantityValue,
			QuantityScale:           pos.QuantityScale,
			RemainingCostBasisValue: pos.RemainingCostBasisValue,
			RemainingCostBasisScale: pos.RemainingCostBasisScale,
			LatestPriceValue:        pos.LatestPriceValue,
			LatestPriceScale:        pos.LatestPriceScale,
			LatestPriceDate:         pos.LatestPriceDate,
		}
		if pos.LatestPriceValue.Valid && pos.LatestPriceScale.Valid {
			// market_value = quantity × price_value ÷ base_quantity_value,
			// through exact.MulDivRound so the whole thing is one big.Int
			// expression with a single rounding step at the end — the same
			// helper the reporting conversions use, rather than a second
			// hand-rolled copy of the arithmetic.
			//
			// price_observations stores price_value at price_scale and
			// base_quantity_value at base_quantity_scale, so a quote can be
			// "per 100 units" as easily as per one. Defaults when those columns
			// are NULL: value=1, scale=0.
			baseQtyValue := int64(1)
			baseQtyScale := 0
			if pos.LatestPriceBaseQuantityValue.Valid {
				baseQtyValue = pos.LatestPriceBaseQuantityValue.Int64
			}
			if pos.LatestPriceBaseQuantityScale.Valid {
				baseQtyScale = int(pos.LatestPriceBaseQuantityScale.Int64)
			}
			if baseQtyValue <= 0 {
				baseQtyValue = 1
			}

			priceScale := int(pos.LatestPriceScale.Int64)
			// The product's natural scale is the sum of the quantity's and the
			// price's, less the base quantity's; a price quoted per fractional
			// base quantity can drive that below zero, which is not a scale.
			// Compute at whichever of that, the cost basis's scale and zero is
			// deepest, so nothing is rounded away before the subtraction.
			gainScale := max(max(pos.QuantityScale+priceScale-baseQtyScale, pos.RemainingCostBasisScale), 0)
			marketBig, err := exact.MulDivRound(
				pos.QuantityValue.BigInt(), pos.QuantityScale,
				big.NewInt(pos.LatestPriceValue.Int64), priceScale,
				big.NewInt(baseQtyValue), baseQtyScale,
				gainScale,
			)
			if err != nil {
				return nil, fmt.Errorf("value position %d/%d: %w", pos.AccountID, pos.CommodityID, err)
			}

			market := exact.ScaledIntFromBig(marketBig, gainScale)
			cost := exact.ScaledIntFromInt64(pos.RemainingCostBasisValue, pos.RemainingCostBasisScale)
			gain := exact.ScaledIntFromBig(marketBig, gainScale)
			gain.SubScaled(cost)

			marketValue, marketValueScale, marketFits := int64AtUsableScale(market)
			gainValue, gainValueScale, gainFits := int64AtUsableScale(gain)
			if marketFits && gainFits {
				record.MarketValueValue = &marketValue
				record.MarketValueScale = &marketValueScale
				record.UnrealizedGainValue = &gainValue
				record.UnrealizedGainScale = &gainValueScale
			} else {
				record.ValuationUnavailable = ValuationUnrepresentable
			}
		} else {
			record.ValuationUnavailable = ValuationNoPrice
		}
		records = append(records, record)
	}
	return records, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func mapInvestmentConstraintError(err error) error {
	message := err.Error()
	switch {
	case strings.Contains(message, "UNIQUE constraint failed: commodities.book_id, commodities.code"):
		return ErrCommodityExists
	case strings.Contains(message, "UNIQUE constraint failed: investment_instruments.book_id, investment_instruments.commodity_id"):
		return ErrInvestmentInstrumentExists
	case strings.Contains(message, "UNIQUE constraint failed: cost_basis_profiles.book_id, cost_basis_profiles.name"):
		return ErrCostBasisProfileExists
	default:
		return err
	}
}

// T-104: admission is checked inside the same transaction as journal, lot and
// audit writes. Never accept a new projection that the position read model
// cannot sum exactly. This also covers reinvestment/import acquisition paths.
func requirePositionBasisRangeTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID int64) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT remaining_cost_basis_value, remaining_cost_basis_scale
		FROM investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ?
			AND status = 'open'
	`, bookID, accountID, commodityID, costCommodityID)
	if err != nil {
		return fmt.Errorf("read position basis range: %w", err)
	}
	defer rows.Close()
	total := exact.NewScaledInt()
	for rows.Next() {
		var value int64
		var scale int
		if err := rows.Scan(&value, &scale); err != nil {
			return fmt.Errorf("scan position basis range: %w", err)
		}
		total.AddInt64(value, scale)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate position basis range: %w", err)
	}
	if _, err := total.Int64(); err != nil {
		return ErrInvestmentBasisRange
	}
	return nil
}
