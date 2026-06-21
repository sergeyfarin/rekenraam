package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var ErrCommodityExists = errors.New("commodity already exists")
var ErrCurrenciesSetupComplete = errors.New("currencies setup already complete")

type CommodityRepository struct {
	database *sql.DB
}

type CommodityRecord struct {
	ID               int64
	BookID           int64
	Code             string
	Kind             string
	IsBuiltin        bool
	CreatedAt        string
	CreatedByUserID  int64
	VersionID        int64
	VersionSeq       int64
	EffectiveFrom    string
	RecordedAt       string
	ChangedByUserID  int64
	ChangeReason     string
	Status           string
	Symbol           string
	DisplaySymbol    string
	Name             string
	StandardScale    int
	MaxQuantityScale int
	MetadataJSON     string
}

type CurrencySpec struct {
	Code             string
	Name             string
	Symbol           string
	StandardScale    int
	MaxQuantityScale int
}

type CreateCurrencyParams struct {
	BookID          int64
	CreatedByUserID int64
	AuthSessionID   int64
	RequestID       string
	OriginType      string
	Operation       string
	Spec            CurrencySpec
	CreatedAt       string
	EffectiveFrom   string
}

type CompleteCurrencySetupParams struct {
	BookID          int64
	ChangedByUserID int64
	AuthSessionID   int64
	RequestID       string
	OriginType      string
	Operation       string
	DefaultCode     string
	Specs           []CurrencySpec
	CreatedAt       string
	EffectiveFrom   string
}

type CompleteCurrencySetupRecord struct {
	Currencies       []CommodityRecord
	DefaultCurrency  CommodityRecord
	DefaultUpdatedAt string
}

type SetDefaultCurrencyParams struct {
	BookID          int64
	CommodityID     int64
	UpdatedAt       string
	UpdatedByUserID int64
	AuthSessionID   int64
	RequestID       string
	OriginType      string
	Operation       string
}

func NewCommodityRepository(database *sql.DB) *CommodityRepository {
	return &CommodityRepository{database: database}
}

// PostingCommoditySummary holds the commodity fields needed to enrich posting responses.
type PostingCommoditySummary struct {
	ID            int64
	Code          string
	DisplaySymbol string // prefer display_symbol; falls back to symbol in DB (both stored identically in current data)
	Kind          string
}

// CommoditiesByIDs returns a PostingCommoditySummary keyed by commodity ID for the given IDs.
// Missing IDs are absent from the map (not an error). Callers must deduplicate IDs first.
// Does not filter by kind so securities and currencies are both returned.
func (r *CommodityRepository) CommoditiesByIDs(ctx context.Context, bookID int64, ids []int64) (map[int64]PostingCommoditySummary, error) {
	if len(ids) == 0 {
		return map[int64]PostingCommoditySummary{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, bookID)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := `
		SELECT
			c.id,
			c.code,
			COALESCE(NULLIF(cv.display_symbol, ''), NULLIF(cv.symbol, '')),
			c.kind
		FROM commodities c
		JOIN current_commodity_versions cv ON cv.commodity_id = c.id
		WHERE c.book_id = ?
			AND c.id IN (` + strings.Join(placeholders, ", ") + `)`

	rows, err := r.database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("commodities by ids: %w", err)
	}
	defer rows.Close()

	result := make(map[int64]PostingCommoditySummary, len(ids))
	for rows.Next() {
		var s PostingCommoditySummary
		var displaySymbol sql.NullString
		if err := rows.Scan(&s.ID, &s.Code, &displaySymbol, &s.Kind); err != nil {
			return nil, fmt.Errorf("scan posting commodity summary: %w", err)
		}
		if displaySymbol.Valid {
			s.DisplaySymbol = displaySymbol.String
		}
		result[s.ID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate posting commodity summaries: %w", err)
	}

	return result, nil
}

func (r *CommodityRepository) ListCurrencies(ctx context.Context, bookID int64) ([]CommodityRecord, error) {
	rows, err := r.database.QueryContext(ctx, currentCurrencySelect(`
		AND c.book_id = ? AND c.kind = 'currency'
		ORDER BY c.code
	`), bookID)
	if err != nil {
		return nil, fmt.Errorf("list currencies: %w", err)
	}
	defer rows.Close()

	return scanCommodityRecords(rows)
}

func (r *CommodityRepository) CurrentCurrencyByID(ctx context.Context, bookID int64, commodityID int64) (CommodityRecord, error) {
	var record CommodityRecord
	if err := scanCommodityRecord(r.database.QueryRowContext(ctx, currentCurrencySelect(`
		AND c.book_id = ? AND c.kind = 'currency' AND c.id = ?
	`), bookID, commodityID), &record); err != nil {
		return CommodityRecord{}, err
	}

	return record, nil
}

func (r *CommodityRepository) CreateCurrency(ctx context.Context, params CreateCurrencyParams) (CommodityRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return CommodityRecord{}, fmt.Errorf("begin create currency transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return CommodityRecord{}, err
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.CreatedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        "created currency",
	})
	if err != nil {
		return CommodityRecord{}, err
	}

	record, err := createCurrency(ctx, tx, params, auditEventID)
	if err != nil {
		return CommodityRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return CommodityRecord{}, fmt.Errorf("commit create currency transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *CommodityRepository) CompleteCurrencySetup(ctx context.Context, params CompleteCurrencySetupParams) (CompleteCurrencySetupRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return CompleteCurrencySetupRecord{}, fmt.Errorf("begin currencies setup transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	var completedAt sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT completed_at
		FROM setup_steps
		WHERE step_key = 'currencies'
	`).Scan(&completedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CompleteCurrencySetupRecord{}, ErrNotFound
		}
		return CompleteCurrencySetupRecord{}, fmt.Errorf("read currencies setup step: %w", err)
	}
	if completedAt.Valid {
		return CompleteCurrencySetupRecord{}, ErrCurrenciesSetupComplete
	}

	var bookID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM books
		WHERE id = ?
	`, params.BookID).Scan(&bookID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CompleteCurrencySetupRecord{}, ErrNotFound
		}
		return CompleteCurrencySetupRecord{}, fmt.Errorf("read setup book: %w", err)
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ChangedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        "completed currency setup",
	})
	if err != nil {
		return CompleteCurrencySetupRecord{}, err
	}

	currencies := make([]CommodityRecord, 0, len(params.Specs))
	var defaultCurrency CommodityRecord
	for _, spec := range params.Specs {
		record, err := ensureCurrency(ctx, tx, CreateCurrencyParams{
			BookID:          params.BookID,
			CreatedByUserID: params.ChangedByUserID,
			AuthSessionID:   params.AuthSessionID,
			RequestID:       params.RequestID,
			Spec:            spec,
			CreatedAt:       params.CreatedAt,
			EffectiveFrom:   params.EffectiveFrom,
		}, auditEventID)
		if err != nil {
			return CompleteCurrencySetupRecord{}, err
		}

		currencies = append(currencies, record)
		if record.Code == params.DefaultCode {
			defaultCurrency = record
		}
	}
	if defaultCurrency.ID == 0 {
		return CompleteCurrencySetupRecord{}, ErrNotFound
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE books
		SET default_currency_commodity_id = ?, updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?
		WHERE id = ?
	`, defaultCurrency.ID, params.CreatedAt, params.ChangedByUserID, auditEventID, params.BookID); err != nil {
		return CompleteCurrencySetupRecord{}, fmt.Errorf("set default currency: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE setup_steps
		SET completed_at = ?, completed_audit_event_id = ?
		WHERE step_key = 'currencies' AND completed_at IS NULL
	`, params.CreatedAt, auditEventID)
	if err != nil {
		return CompleteCurrencySetupRecord{}, fmt.Errorf("mark currencies setup complete: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return CompleteCurrencySetupRecord{}, fmt.Errorf("read currencies setup rows affected: %w", err)
	}
	if rowsAffected != 1 {
		return CompleteCurrencySetupRecord{}, fmt.Errorf("mark currencies setup complete: updated %d rows", rowsAffected)
	}

	if err := tx.Commit(); err != nil {
		return CompleteCurrencySetupRecord{}, fmt.Errorf("commit currencies setup transaction: %w", err)
	}
	committed = true

	return CompleteCurrencySetupRecord{
		Currencies:       currencies,
		DefaultCurrency:  defaultCurrency,
		DefaultUpdatedAt: params.CreatedAt,
	}, nil
}

func (r *CommodityRepository) SetDefaultCurrency(ctx context.Context, params SetDefaultCurrencyParams) (CommodityRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return CommodityRecord{}, fmt.Errorf("begin set default currency transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	record, err := r.currentCurrencyByID(ctx, tx, params.BookID, params.CommodityID)
	if err != nil {
		return CommodityRecord{}, err
	}
	if record.Status != "active" {
		return CommodityRecord{}, ErrNotFound
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.UpdatedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.UpdatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        "set default currency",
	})
	if err != nil {
		return CommodityRecord{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE books
		SET default_currency_commodity_id = ?, updated_at = ?, updated_by_user_id = ?, updated_audit_event_id = ?
		WHERE id = ?
	`, params.CommodityID, params.UpdatedAt, params.UpdatedByUserID, auditEventID, params.BookID); err != nil {
		return CommodityRecord{}, fmt.Errorf("set default currency: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return CommodityRecord{}, fmt.Errorf("commit set default currency transaction: %w", err)
	}
	committed = true

	return record, nil
}

func ensureCurrency(ctx context.Context, tx *sql.Tx, params CreateCurrencyParams, auditEventID int64) (CommodityRecord, error) {
	record, err := currentCurrencyByCode(ctx, tx, params.BookID, params.Spec.Code)
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return CommodityRecord{}, err
	}

	return createCurrency(ctx, tx, params, auditEventID)
}

func createCurrency(ctx context.Context, tx *sql.Tx, params CreateCurrencyParams, auditEventID int64) (CommodityRecord, error) {
	if _, err := currentCurrencyByCode(ctx, tx, params.BookID, params.Spec.Code); err == nil {
		return CommodityRecord{}, ErrCommodityExists
	} else if !errors.Is(err, ErrNotFound) {
		return CommodityRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO commodities (book_id, code, kind, is_builtin, created_at, created_by_user_id, created_request_id, created_audit_event_id)
		VALUES (?, ?, 'currency', 1, ?, ?, NULLIF(?, ''), ?)
	`, params.BookID, params.Spec.Code, params.CreatedAt, params.CreatedByUserID, params.RequestID, auditEventID)
	if err != nil {
		return CommodityRecord{}, fmt.Errorf("insert currency commodity: %w", err)
	}

	commodityID, err := result.LastInsertId()
	if err != nil {
		return CommodityRecord{}, fmt.Errorf("read currency commodity id: %w", err)
	}

	result, err = tx.ExecContext(ctx, `
		INSERT INTO commodity_versions (
			commodity_id,
			version_seq,
			effective_from,
			recorded_at,
			changed_by_user_id,
			change_reason,
			status,
			symbol,
			display_symbol,
			name,
			standard_scale,
			max_quantity_scale,
			metadata_json,
			change_audit_event_id
		)
		VALUES (?, 1, ?, ?, ?, 'created from embedded currency catalog', 'active', ?, ?, ?, ?, ?, '{"source":"embedded_currency_catalog"}', ?)
	`, commodityID, params.EffectiveFrom, params.CreatedAt, params.CreatedByUserID, params.Spec.Symbol, params.Spec.Symbol, params.Spec.Name, params.Spec.StandardScale, params.Spec.MaxQuantityScale, auditEventID)
	if err != nil {
		return CommodityRecord{}, fmt.Errorf("insert currency version: %w", err)
	}

	versionID, err := result.LastInsertId()
	if err != nil {
		return CommodityRecord{}, fmt.Errorf("read currency version id: %w", err)
	}

	return CommodityRecord{
		ID:               commodityID,
		BookID:           params.BookID,
		Code:             params.Spec.Code,
		Kind:             "currency",
		IsBuiltin:        true,
		CreatedAt:        params.CreatedAt,
		CreatedByUserID:  params.CreatedByUserID,
		VersionID:        versionID,
		VersionSeq:       1,
		EffectiveFrom:    params.EffectiveFrom,
		RecordedAt:       params.CreatedAt,
		ChangedByUserID:  params.CreatedByUserID,
		ChangeReason:     "created from embedded currency catalog",
		Status:           "active",
		Symbol:           params.Spec.Symbol,
		DisplaySymbol:    params.Spec.Symbol,
		Name:             params.Spec.Name,
		StandardScale:    params.Spec.StandardScale,
		MaxQuantityScale: params.Spec.MaxQuantityScale,
		MetadataJSON:     `{"source":"embedded_currency_catalog"}`,
	}, nil
}

func (r *CommodityRepository) currentCurrencyByID(ctx context.Context, tx *sql.Tx, bookID int64, commodityID int64) (CommodityRecord, error) {
	var record CommodityRecord
	if err := scanCommodityRecord(tx.QueryRowContext(ctx, currentCurrencySelect(`
		AND c.book_id = ? AND c.kind = 'currency' AND c.id = ?
	`), bookID, commodityID), &record); err != nil {
		return CommodityRecord{}, err
	}

	return record, nil
}

func currentCurrencyByCode(ctx context.Context, tx *sql.Tx, bookID int64, code string) (CommodityRecord, error) {
	var record CommodityRecord
	if err := scanCommodityRecord(tx.QueryRowContext(ctx, currentCurrencySelect(`
		AND c.book_id = ? AND c.kind = 'currency' AND c.code = ?
	`), bookID, code), &record); err != nil {
		return CommodityRecord{}, err
	}

	return record, nil
}

func currentCurrencySelect(whereClause string) string {
	return `
		SELECT
			c.id,
			c.book_id,
			c.code,
			c.kind,
			c.is_builtin,
			c.created_at,
			c.created_by_user_id,
			cv.id,
			cv.version_seq,
			cv.effective_from,
			cv.recorded_at,
			cv.changed_by_user_id,
			cv.change_reason,
			cv.status,
			cv.symbol,
			cv.display_symbol,
			cv.name,
			cv.standard_scale,
			cv.max_quantity_scale,
			cv.metadata_json
		FROM commodities c
		JOIN current_commodity_versions cv ON cv.commodity_id = c.id
		WHERE 1 = 1
	` + whereClause
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCommodityRecord(row rowScanner, record *CommodityRecord) error {
	var isBuiltin int
	if err := row.Scan(
		&record.ID,
		&record.BookID,
		&record.Code,
		&record.Kind,
		&isBuiltin,
		&record.CreatedAt,
		&record.CreatedByUserID,
		&record.VersionID,
		&record.VersionSeq,
		&record.EffectiveFrom,
		&record.RecordedAt,
		&record.ChangedByUserID,
		&record.ChangeReason,
		&record.Status,
		&record.Symbol,
		&record.DisplaySymbol,
		&record.Name,
		&record.StandardScale,
		&record.MaxQuantityScale,
		&record.MetadataJSON,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("scan commodity: %w", err)
	}
	record.IsBuiltin = isBuiltin == 1

	return nil
}

func scanCommodityRecords(rows *sql.Rows) ([]CommodityRecord, error) {
	var records []CommodityRecord
	for rows.Next() {
		var record CommodityRecord
		if err := scanCommodityRecord(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commodities: %w", err)
	}

	return records, nil
}
