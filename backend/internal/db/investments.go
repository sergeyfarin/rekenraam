package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"rekenraam/backend/internal/exact"
)

var (
	ErrInvestmentInstrumentExists = errors.New("investment instrument already exists")
	ErrCostBasisProfileExists     = errors.New("cost basis profile already exists")
	ErrDividendDefaultExists      = errors.New("dividend default already exists")
	ErrInsufficientLots           = errors.New("insufficient lots")
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
	LotID          int64
	QuantityValue  exact.Coefficient
	QuantityScale  int
	CostBasisValue int64
	CostBasisScale int
}

type DisposeLotsParams struct {
	BookID        int64
	AccountID     int64
	CommodityID   int64
	TransactionID int64
	EventDate     string
	QuantityValue exact.Coefficient
	QuantityScale int
	Allocations   []LotAllocation
	CreatedAt     string
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	ChangeReason  string
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

func (r *InvestmentRepository) CreateInstrument(ctx context.Context, params CreateInvestmentInstrumentParams) (InvestmentInstrumentRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return InvestmentInstrumentRecord{}, fmt.Errorf("begin create investment instrument: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
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
		`, commodityID, params.EffectiveFrom, params.CreatedAt, params.CreatedByUserID, params.ChangeReason, params.Spec.Symbol, params.Spec.Symbol, params.Spec.DisplayName, params.Spec.QuantityScale, params.Spec.QuantityScale, params.Spec.MetadataJSON, auditEventID); err != nil {
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

func (r *InvestmentRepository) UpdateInstrument(ctx context.Context, params UpdateInvestmentInstrumentParams) (InvestmentInstrumentRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return InvestmentInstrumentRecord{}, fmt.Errorf("begin update investment instrument: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
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
		SELECT id, book_id, name, method, is_default, status, description, metadata_json, created_at, created_by_user_id, updated_at, updated_by_user_id
		FROM cost_basis_profiles
		WHERE book_id = ?
		ORDER BY is_default DESC, name COLLATE NOCASE, id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list cost basis profiles: %w", err)
	}
	defer rows.Close()
	return scanCostBasisProfiles(rows)
}

func (r *InvestmentRepository) DefaultCostBasisProfile(ctx context.Context, bookID int64) (CostBasisProfileRecord, error) {
	return scanCostBasisProfileRow(r.database.QueryRowContext(ctx, `
		SELECT id, book_id, name, method, is_default, status, description, metadata_json, created_at, created_by_user_id, updated_at, updated_by_user_id
		FROM cost_basis_profiles
		WHERE book_id = ? AND is_default = 1 AND status = 'active'
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
			_ = tx.Rollback()
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
			UPDATE cost_basis_profiles
			SET is_default = 0, updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?
			WHERE book_id = ? AND is_default = 1
		`, params.RecordedAt, params.ActorUserID, auditEventID, params.BookID); err != nil {
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
			_ = tx.Rollback()
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
			_ = tx.Rollback()
		}
	}()
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
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO investment_lot_events (
			book_id, lot_id, event_kind, transaction_id, event_date, quantity_value, quantity_scale,
			cost_basis_value, cost_basis_scale, metadata_json, created_at, created_by_user_id, created_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '{}', ?, ?, ?)
	`, params.BookID, lotID, params.EventKind, nullablePositiveInt64(params.SourceTransactionID), params.OpenedOn, params.QuantityValue, params.QuantityScale, params.CostBasisValue, params.CostBasisScale, params.CreatedAt, params.CreatedByUserID, auditEventID); err != nil {
		return InvestmentLotRecord{}, fmt.Errorf("insert investment lot event: %w", err)
	}
	record, err := investmentLotByIDTx(ctx, tx, params.BookID, lotID)
	if err != nil {
		return InvestmentLotRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return InvestmentLotRecord{}, fmt.Errorf("commit create investment lot: %w", err)
	}
	committed = true
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
			_ = tx.Rollback()
		}
	}()
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
	disposals := make([]LotDisposalRecord, 0)
	if len(params.Allocations) > 0 {
		for _, allocation := range params.Allocations {
			disposal, err := disposeLotTx(ctx, tx, params, allocation.LotID, allocation.QuantityValue, allocation.QuantityScale, auditEventID)
			if err != nil {
				return nil, err
			}
			disposals = append(disposals, disposal)
		}
	} else {
		remainingValue := params.QuantityValue
		rows, err := tx.QueryContext(ctx, `
			SELECT id, remaining_quantity_value, remaining_quantity_scale
			FROM investment_lots
			WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND status = 'open'
			ORDER BY opened_on, id
		`, params.BookID, params.AccountID, params.CommodityID)
		if err != nil {
			return nil, fmt.Errorf("read FIFO lots: %w", err)
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
				return nil, fmt.Errorf("scan FIFO lot: %w", err)
			}
			lots = append(lots, lot)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close FIFO lots: %w", err)
		}
		for _, lot := range lots {
			if remainingValue.Sign() <= 0 {
				break
			}
			take := lot.value
			if take.Cmp(remainingValue) > 0 {
				take = remainingValue
			}
			disposal, err := disposeLotTx(ctx, tx, params, lot.id, take, lot.scale, auditEventID)
			if err != nil {
				return nil, err
			}
			disposals = append(disposals, disposal)
			remaining, err := exact.FromBig(new(big.Int).Sub(remainingValue.BigInt(), take.BigInt()))
			if err != nil {
				return nil, err
			}
			remainingValue = remaining
		}
		if remainingValue.Sign() > 0 {
			return nil, ErrInsufficientLots
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit dispose lots: %w", err)
	}
	committed = true
	return disposals, nil
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
			) AS latest_price_date
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
		quantity *scaledInteger
		cost     *scaledInteger
	}
	positions := map[positionKey]*accumulatedPosition{}
	var order []positionKey
	for rows.Next() {
		var record InvestmentPositionRecord
		var quantity exact.Coefficient
		var quantityScale int
		var costValue int64
		var costScale int
		if err := rows.Scan(&record.AccountID, &record.CommodityID, &quantity, &quantityScale, &costValue, &costScale, &record.CostCommodityID, &record.LatestPriceValue, &record.LatestPriceScale, &record.LatestPriceDate); err != nil {
			return nil, fmt.Errorf("scan investment position: %w", err)
		}
		key := positionKey{record.AccountID, record.CommodityID, record.CostCommodityID}
		position := positions[key]
		if position == nil {
			position = &accumulatedPosition{record: record, quantity: newScaledInteger(), cost: newScaledInteger()}
			positions[key] = position
			order = append(order, key)
		}
		position.quantity.add(quantity.BigInt(), quantityScale)
		position.cost.add(big.NewInt(costValue), costScale)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate investment positions: %w", err)
	}
	records := make([]InvestmentPositionRecord, 0, len(order))
	for _, key := range order {
		position := positions[key]
		quantity, err := exact.FromBig(position.quantity.value)
		if err != nil {
			return nil, err
		}
		if !position.cost.value.IsInt64() {
			return nil, fmt.Errorf("investment position cost basis exceeds int64 range")
		}
		position.record.QuantityValue = quantity
		position.record.QuantityScale = position.quantity.scale
		position.record.RemainingCostBasisValue = position.cost.value.Int64()
		position.record.RemainingCostBasisScale = position.cost.scale
		records = append(records, position.record)
	}
	return records, nil
}

type scaledInteger struct {
	value *big.Int
	scale int
}

func newScaledInteger() *scaledInteger { return &scaledInteger{value: big.NewInt(0)} }

func (s *scaledInteger) add(value *big.Int, scale int) {
	if scale > s.scale {
		s.value.Mul(s.value, pow10DB(scale-s.scale))
		s.scale = scale
	}
	addend := new(big.Int).Set(value)
	if scale < s.scale {
		addend.Mul(addend, pow10DB(s.scale-scale))
	}
	s.value.Add(s.value, addend)
}

func pow10DB(scale int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
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
			_ = tx.Rollback()
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
		WHERE book_id = ? AND id = ?
	`, status, now, actorUserID, auditEventID, bookID, suggestionID)
	if err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("update event suggestion: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return EventSuggestionRecord{}, fmt.Errorf("read event suggestion update rows: %w", err)
	}
	if rowsAffected == 0 {
		return EventSuggestionRecord{}, ErrNotFound
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

func (r *InvestmentRepository) SaveAutomationRule(ctx context.Context, params SaveAutomationRuleParams) (AutomationRuleRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return AutomationRuleRecord{}, fmt.Errorf("begin save automation rule: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
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

	ruleID := params.RuleID
	if ruleID > 0 {
		result, err := tx.ExecContext(ctx, `
			UPDATE investment_automation_rules
			SET source_id = ?, instrument_id = ?, event_family = ?, mode = ?,
				confidence_threshold_bps = ?, required_accounts_json = ?, status = ?,
				effective_from = ?, effective_to = ?, updated_at = ?, updated_by_user_id = ?,
				updated_audit_event_id = ?
			WHERE book_id = ? AND id = ?
		`, nullableInt64Value(params.Spec.SourceID), nullableInt64Value(params.Spec.InstrumentID), params.Spec.EventFamily, params.Spec.Mode, params.Spec.ConfidenceThresholdBPS, params.Spec.RequiredAccountsJSON, params.Spec.Status, params.Spec.EffectiveFrom, nullableStringValue(params.Spec.EffectiveTo), params.RecordedAt, params.ActorUserID, auditEventID, params.BookID, ruleID)
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
		`, params.BookID, nullableInt64Value(params.Spec.SourceID), nullableInt64Value(params.Spec.InstrumentID), params.Spec.EventFamily, params.Spec.Mode, params.Spec.ConfidenceThresholdBPS, params.Spec.RequiredAccountsJSON, params.Spec.Status, params.Spec.EffectiveFrom, nullableStringValue(params.Spec.EffectiveTo), params.RecordedAt, params.ActorUserID, params.RecordedAt, params.ActorUserID, auditEventID, auditEventID)
		if err != nil {
			return AutomationRuleRecord{}, fmt.Errorf("insert automation rule: %w", err)
		}
		ruleID, err = result.LastInsertId()
		if err != nil {
			return AutomationRuleRecord{}, fmt.Errorf("read automation rule id: %w", err)
		}
	}

	record, err := automationRuleByIDTx(ctx, tx, params.BookID, ruleID)
	if err != nil {
		return AutomationRuleRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationRuleRecord{}, fmt.Errorf("commit save automation rule: %w", err)
	}
	committed = true
	return record, nil
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
	if err := row.Scan(&record.ID, &record.BookID, &record.Name, &record.Method, &isDefault, &record.Status, &record.Description, &record.MetadataJSON, &record.CreatedAt, &record.CreatedByUserID, &record.UpdatedAt, &record.UpdatedByUserID); err != nil {
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
		SELECT id, book_id, name, method, is_default, status, description, metadata_json, created_at, created_by_user_id, updated_at, updated_by_user_id
		FROM cost_basis_profiles
		WHERE book_id = ? AND id = ?
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

func disposeLotTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams, lotID int64, quantityValue exact.Coefficient, quantityScale int, auditEventID int64) (LotDisposalRecord, error) {
	lot, err := investmentLotByIDTx(ctx, tx, params.BookID, lotID)
	if err != nil {
		return LotDisposalRecord{}, err
	}
	if lot.AccountID != params.AccountID || lot.CommodityID != params.CommodityID || lot.Status != "open" {
		return LotDisposalRecord{}, ErrNotFound
	}
	if quantityScale != lot.RemainingQuantityScale {
		return LotDisposalRecord{}, fmt.Errorf("lot allocation scale does not match lot scale")
	}
	if quantityValue.Sign() <= 0 || quantityValue.Cmp(lot.RemainingQuantityValue) > 0 {
		return LotDisposalRecord{}, ErrInsufficientLots
	}
	costBasisValue := proratedCostBasis(lot.RemainingCostBasisValue, quantityValue, lot.RemainingQuantityValue)
	nextRemainingQuantity, err := exact.FromBig(new(big.Int).Sub(lot.RemainingQuantityValue.BigInt(), quantityValue.BigInt()))
	if err != nil {
		return LotDisposalRecord{}, err
	}
	nextRemainingCost := lot.RemainingCostBasisValue - costBasisValue
	status := "open"
	if nextRemainingQuantity.Sign() == 0 {
		status = "closed"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE investment_lots
		SET remaining_quantity_value = ?, remaining_cost_basis_value = ?, status = ?,
			updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?
		WHERE book_id = ? AND id = ?
	`, nextRemainingQuantity, nextRemainingCost, status, params.CreatedAt, params.ActorUserID, auditEventID, params.BookID, lotID); err != nil {
		return LotDisposalRecord{}, fmt.Errorf("update disposed investment lot: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO investment_lot_events (
			book_id, lot_id, event_kind, transaction_id, event_date, quantity_value, quantity_scale,
			cost_basis_value, cost_basis_scale, metadata_json, created_at, created_by_user_id, created_audit_event_id
		)
		VALUES (?, ?, 'disposal', ?, ?, ?, ?, ?, ?, '{}', ?, ?, ?)
	`, params.BookID, lotID, nullablePositiveInt64(params.TransactionID), params.EventDate, quantityValue.Negated(), quantityScale, -costBasisValue, lot.RemainingCostBasisScale, params.CreatedAt, params.ActorUserID, auditEventID); err != nil {
		return LotDisposalRecord{}, fmt.Errorf("insert disposal lot event: %w", err)
	}
	return LotDisposalRecord{
		LotID:          lotID,
		QuantityValue:  quantityValue,
		QuantityScale:  quantityScale,
		CostBasisValue: costBasisValue,
		CostBasisScale: lot.RemainingCostBasisScale,
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
