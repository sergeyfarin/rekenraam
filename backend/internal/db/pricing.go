package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type PricingRepository struct {
	database *sql.DB
	*BackgroundWorkRepository
}

type MarketDataSourceRecord struct {
	ID           int64
	Code         string
	Name         string
	Kind         string
	ProviderKey  sql.NullString
	BaseURL      sql.NullString
	Status       string
	MetadataJSON string
	CreatedAt    string
}

type PricingCurrencyRecord struct {
	ID   int64
	Code string
}

type PricingCurrencyCoverageRecord struct {
	CommodityID int64
	StartDate   string
}

type PriceSeriesRecord struct {
	ID               int64
	BookID           int64
	BaseCommodityID  int64
	QuoteCommodityID int64
	SourceID         sql.NullInt64
	QuoteType        string
	AdjustmentBasis  string
	MarketCode       sql.NullString
	ProviderSeriesID sql.NullString
	Status           string
	MetadataJSON     string
	CreatedAt        string
	CreatedByUserID  int64
}

type PriceSeriesSpec struct {
	BaseCommodityID  int64
	QuoteCommodityID int64
	SourceID         sql.NullInt64
	QuoteType        string
	AdjustmentBasis  string
	MarketCode       string
	ProviderSeriesID string
	MetadataJSON     string
}

type PriceObservationRecord struct {
	ID                      int64
	BookID                  int64
	SeriesID                int64
	BaseCommodityID         int64
	QuoteCommodityID        int64
	QuoteType               string
	AdjustmentBasis         string
	PriceValue              int64
	PriceScale              int
	BaseQuantityValue       int64
	BaseQuantityScale       int
	ValuationDate           string
	ObservedAt              sql.NullString
	SourcePublishedAt       sql.NullString
	SourceID                sql.NullInt64
	ProviderObservationID   sql.NullString
	IsManual                bool
	IsDerived               bool
	SupersedesObservationID sql.NullInt64
	DerivationJSON          string
	MetadataJSON            string
	VoidedAt                sql.NullString
	RecordedAt              string
	CreatedByUserID         int64
}

type PriceObservationSpec struct {
	SeriesID                sql.NullInt64
	BaseCommodityID         int64
	QuoteCommodityID        int64
	QuoteType               string
	AdjustmentBasis         string
	PriceValue              int64
	PriceScale              int
	BaseQuantityValue       int64
	BaseQuantityScale       int
	ValuationDate           string
	ObservedAt              sql.NullString
	SourcePublishedAt       sql.NullString
	SourceID                sql.NullInt64
	MarketCode              string
	ProviderSeriesID        string
	ProviderObservationID   string
	IsManual                bool
	IsDerived               bool
	SupersedesObservationID sql.NullInt64
	DerivationJSON          string
	SeriesMetadataJSON      string
	MetadataJSON            string
}

type CreatePriceObservationParams struct {
	BookID        int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Spec          PriceObservationSpec
	CreatedAt     string
	ChangeReason  string
}

type PricingPolicyRecord struct {
	ID                   int64
	BookID               int64
	BaseCommodityID      sql.NullInt64
	DefaultSourceID      sql.NullInt64
	RefreshEnabled       bool
	RefreshHourUTC       int
	RefreshMinuteUTC     int
	RefreshHourLocal     int
	RefreshMinuteLocal   int
	MaxBackfillDays      int
	StalenessMaxDays     int
	TriangulationMaxHops int
	RoundingMode         string
	PreferOfficialFX     bool
	WeekendPolicy        string
	CreatedAt            string
	CreatedByUserID      int64
	UpdatedAt            string
	UpdatedByUserID      int64
}

type PricingPolicySpec struct {
	BaseCommodityID      sql.NullInt64
	DefaultSourceID      sql.NullInt64
	RefreshEnabled       bool
	RefreshHourUTC       int
	RefreshMinuteUTC     int
	RefreshHourLocal     int
	RefreshMinuteLocal   int
	MaxBackfillDays      int
	StalenessMaxDays     int
	TriangulationMaxHops int
	RoundingMode         string
	PreferOfficialFX     bool
	WeekendPolicy        string
}

type SavePricingPolicyParams struct {
	BookID        int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Spec          PricingPolicySpec
	RecordedAt    string
	ChangeReason  string
}

type PricingSourceAssignmentRecord struct {
	ID               int64
	BookID           int64
	BaseCommodityID  int64
	QuoteCommodityID int64
	SourceID         int64
	Priority         int
	Status           string
	EffectiveFrom    string
	EffectiveTo      sql.NullString
	CreatedAt        string
	CreatedByUserID  int64
}

type PricingSourceAssignmentSpec struct {
	BaseCommodityID  int64
	QuoteCommodityID int64
	SourceID         int64
	Priority         int
	Status           string
	EffectiveFrom    string
	EffectiveTo      sql.NullString
}

type SavePricingSourceAssignmentParams struct {
	BookID        int64
	AssignmentID  int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Spec          PricingSourceAssignmentSpec
	RecordedAt    string
	ChangeReason  string
}

type PricingRefreshRunRecord struct {
	ID             int64
	BookID         int64
	SourceID       int64
	Trigger        string
	Status         string
	StartedAt      string
	FinishedAt     sql.NullString
	ItemsTotal     int
	ItemsSucceeded int
	ItemsFailed    int
	LastError      sql.NullString
}

type PricingRefreshStateRecord struct {
	ID               int64
	BookID           int64
	BaseCommodityID  int64
	QuoteCommodityID int64
	SourceID         int64
	LastSuccessDate  sql.NullString
	LastAttemptAt    sql.NullString
	LastError        sql.NullString
	UpdatedAt        string
}

type PricingRefreshItemSpec struct {
	RunID          int64
	BookID         int64
	ItemKind       string
	Status         string
	ProviderItemID string
	LocalRefTable  string
	LocalRefID     int64
	Error          string
	RawJSON        string
	CreatedAt      string
}

type PricingRefreshStateSpec struct {
	BookID           int64
	BaseCommodityID  int64
	QuoteCommodityID int64
	SourceID         int64
	LastSuccessDate  sql.NullString
	LastAttemptAt    string
	LastError        string
	UpdatedAt        string
}

func NewPricingRepository(database *sql.DB) *PricingRepository {
	return &PricingRepository{
		database:                 database,
		BackgroundWorkRepository: NewBackgroundWorkRepository(database),
	}
}

func (r *PricingRepository) FXCoverageStartDates(ctx context.Context, bookID int64) ([]PricingCurrencyCoverageRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		WITH required_dates AS (
			SELECT av.default_commodity_id AS commodity_id, av.opened_on AS required_date
			FROM current_account_versions av
			JOIN accounts a ON a.id = av.account_id
			JOIN commodities c ON c.id = av.default_commodity_id
			WHERE a.book_id = ? AND av.status = 'active' AND c.kind = 'currency'
			UNION ALL
			SELECT pv.commodity_id, je.entry_date
			FROM current_transaction_versions tv
			JOIN transactions t ON t.id = tv.transaction_id AND t.deleted_at IS NULL
			JOIN journal_entries je ON je.transaction_version_id = tv.id
			JOIN posting_versions pv ON pv.journal_entry_id = je.id
			JOIN commodities c ON c.id = pv.commodity_id
			WHERE tv.book_id = ? AND tv.status IN ('draft', 'posted') AND c.kind = 'currency'
		)
		SELECT commodity_id, MIN(required_date)
		FROM required_dates
		GROUP BY commodity_id
		ORDER BY commodity_id
	`, bookID, bookID)
	if err != nil {
		return nil, fmt.Errorf("list FX coverage start dates: %w", err)
	}
	defer rows.Close()
	var records []PricingCurrencyCoverageRecord
	for rows.Next() {
		var record PricingCurrencyCoverageRecord
		if err := rows.Scan(&record.CommodityID, &record.StartDate); err != nil {
			return nil, fmt.Errorf("scan FX coverage start date: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate FX coverage start dates: %w", err)
	}
	return records, nil
}

func (r *PricingRepository) PriceObservationExistsOnDate(ctx context.Context, bookID int64, baseCommodityID int64, quoteCommodityID int64, valuationDate string) (bool, error) {
	var exists int
	err := r.database.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM price_observations
			WHERE book_id = ? AND base_commodity_id = ? AND quote_commodity_id = ?
			  AND valuation_date = ? AND voided_at IS NULL
		)
	`, bookID, baseCommodityID, quoteCommodityID, valuationDate).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check FX observation date: %w", err)
	}
	return exists == 1, nil
}

func (r *PricingRepository) ListSources(ctx context.Context) ([]MarketDataSourceRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, code, name, kind, provider_key, base_url, status, metadata_json, created_at
		FROM market_data_sources
		ORDER BY kind, name COLLATE NOCASE
	`)
	if err != nil {
		return nil, fmt.Errorf("list market data sources: %w", err)
	}
	defer rows.Close()

	var records []MarketDataSourceRecord
	for rows.Next() {
		var record MarketDataSourceRecord
		if err := rows.Scan(&record.ID, &record.Code, &record.Name, &record.Kind, &record.ProviderKey, &record.BaseURL, &record.Status, &record.MetadataJSON, &record.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan market data source: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate market data sources: %w", err)
	}
	return records, nil
}

func (r *PricingRepository) SourceByID(ctx context.Context, sourceID int64) (MarketDataSourceRecord, error) {
	var record MarketDataSourceRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT id, code, name, kind, provider_key, base_url, status, metadata_json, created_at
		FROM market_data_sources
		WHERE id = ?
	`, sourceID).Scan(&record.ID, &record.Code, &record.Name, &record.Kind, &record.ProviderKey, &record.BaseURL, &record.Status, &record.MetadataJSON, &record.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MarketDataSourceRecord{}, ErrNotFound
		}
		return MarketDataSourceRecord{}, fmt.Errorf("read market data source: %w", err)
	}
	return record, nil
}

func (r *PricingRepository) CurrentBookOwnerID(ctx context.Context, bookID int64) (int64, error) {
	var ownerID int64
	if err := r.database.QueryRowContext(ctx, `SELECT owner_user_id FROM books WHERE id = ?`, bookID).Scan(&ownerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("read book owner: %w", err)
	}
	return ownerID, nil
}

func (r *PricingRepository) DefaultCurrency(ctx context.Context, bookID int64) (PricingCurrencyRecord, error) {
	var record PricingCurrencyRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT c.id, c.code
		FROM books b
		JOIN commodities c ON c.id = b.default_currency_commodity_id
		JOIN current_commodity_versions cv ON cv.commodity_id = c.id
		WHERE b.id = ?
			AND c.kind = 'currency'
			AND cv.status = 'active'
	`, bookID).Scan(&record.ID, &record.Code); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PricingCurrencyRecord{}, ErrNotFound
		}
		return PricingCurrencyRecord{}, fmt.Errorf("read default pricing currency: %w", err)
	}
	return record, nil
}

func (r *PricingRepository) ActiveAccountCurrencies(ctx context.Context, bookID int64) ([]PricingCurrencyRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT DISTINCT c.id, c.code
		FROM accounts a
		JOIN current_account_versions av ON av.account_id = a.id
		JOIN commodities c ON c.id = av.default_commodity_id
		JOIN current_commodity_versions cv ON cv.commodity_id = c.id
		WHERE a.book_id = ?
			AND av.status = 'active'
			AND av.allows_postings = 1
			AND c.kind = 'currency'
			AND cv.status = 'active'
		ORDER BY c.code
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list active account currencies: %w", err)
	}
	defer rows.Close()

	var records []PricingCurrencyRecord
	for rows.Next() {
		var record PricingCurrencyRecord
		if err := rows.Scan(&record.ID, &record.Code); err != nil {
			return nil, fmt.Errorf("scan active account currency: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active account currencies: %w", err)
	}
	return records, nil
}

func (r *PricingRepository) CurrencyByID(ctx context.Context, bookID int64, commodityID int64) (PricingCurrencyRecord, error) {
	var record PricingCurrencyRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT c.id, c.code
		FROM commodities c
		JOIN current_commodity_versions cv ON cv.commodity_id = c.id
		WHERE c.book_id = ?
			AND c.id = ?
			AND c.kind = 'currency'
			AND cv.status = 'active'
	`, bookID, commodityID).Scan(&record.ID, &record.Code); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PricingCurrencyRecord{}, ErrNotFound
		}
		return PricingCurrencyRecord{}, fmt.Errorf("read pricing currency: %w", err)
	}
	return record, nil
}

func (r *PricingRepository) ManualSourceID(ctx context.Context) (int64, error) {
	var sourceID int64
	if err := r.database.QueryRowContext(ctx, `SELECT id FROM market_data_sources WHERE code = 'manual'`).Scan(&sourceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("read manual market data source: %w", err)
	}
	return sourceID, nil
}

func (r *PricingRepository) CreatePriceObservation(ctx context.Context, params CreatePriceObservationParams) (PriceObservationRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return PriceObservationRecord{}, fmt.Errorf("begin create price observation: %w", err)
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
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return PriceObservationRecord{}, err
	}
	seriesID := params.Spec.SeriesID.Int64
	if seriesID == 0 {
		series, err := ensurePriceSeriesTx(ctx, tx, params.BookID, PriceSeriesSpec{
			BaseCommodityID:  params.Spec.BaseCommodityID,
			QuoteCommodityID: params.Spec.QuoteCommodityID,
			SourceID:         params.Spec.SourceID,
			QuoteType:        params.Spec.QuoteType,
			AdjustmentBasis:  params.Spec.AdjustmentBasis,
			MarketCode:       params.Spec.MarketCode,
			ProviderSeriesID: params.Spec.ProviderSeriesID,
			MetadataJSON:     params.Spec.SeriesMetadataJSON,
		}, params.ActorUserID, params.CreatedAt, auditEventID)
		if err != nil {
			return PriceObservationRecord{}, err
		}
		seriesID = series.ID
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO price_observations (
			book_id, series_id, base_commodity_id, quote_commodity_id, quote_type, adjustment_basis,
			price_value, price_scale, base_quantity_value, base_quantity_scale, valuation_date,
			observed_at, source_published_at, source_id, provider_observation_id, is_manual, is_derived,
			supersedes_observation_id, derivation_json, metadata_json, recorded_at, created_by_user_id,
			created_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?)
	`, params.BookID, seriesID, params.Spec.BaseCommodityID, params.Spec.QuoteCommodityID, params.Spec.QuoteType, params.Spec.AdjustmentBasis, params.Spec.PriceValue, params.Spec.PriceScale, params.Spec.BaseQuantityValue, params.Spec.BaseQuantityScale, params.Spec.ValuationDate, nullableStringValue(params.Spec.ObservedAt), nullableStringValue(params.Spec.SourcePublishedAt), nullableInt64Value(params.Spec.SourceID), params.Spec.ProviderObservationID, boolInt(params.Spec.IsManual), boolInt(params.Spec.IsDerived), nullableInt64Value(params.Spec.SupersedesObservationID), params.Spec.DerivationJSON, params.Spec.MetadataJSON, params.CreatedAt, params.ActorUserID, auditEventID)
	if err != nil {
		return PriceObservationRecord{}, fmt.Errorf("insert price observation: %w", err)
	}
	observationID, err := result.LastInsertId()
	if err != nil {
		return PriceObservationRecord{}, fmt.Errorf("read price observation id: %w", err)
	}
	record, err := priceObservationByIDTx(ctx, tx, params.BookID, observationID)
	if err != nil {
		return PriceObservationRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return PriceObservationRecord{}, fmt.Errorf("commit create price observation: %w", err)
	}
	committed = true
	return record, nil
}

func (r *PricingRepository) PriceObservationByProviderID(ctx context.Context, sourceID int64, providerObservationID string) (PriceObservationRecord, error) {
	providerObservationID = strings.TrimSpace(providerObservationID)
	if sourceID <= 0 || providerObservationID == "" {
		return PriceObservationRecord{}, ErrNotFound
	}
	return scanPriceObservationRow(r.database.QueryRowContext(ctx, priceObservationSelect(`
		WHERE source_id = ? AND provider_observation_id = ?
	`), sourceID, providerObservationID))
}

func (r *PricingRepository) ListPriceObservations(ctx context.Context, bookID int64, baseCommodityID int64, quoteCommodityID int64, limit int) ([]PriceObservationRecord, error) {
	hasCurrentSchema, err := r.tableHasColumn(ctx, "price_observations", "base_commodity_id")
	if err != nil {
		return nil, err
	}
	if !hasCurrentSchema {
		return r.listLegacyPriceObservations(ctx, bookID, baseCommodityID, quoteCommodityID, limit)
	}

	where := "book_id = ? AND voided_at IS NULL"
	args := []any{bookID}
	if baseCommodityID > 0 {
		where += " AND base_commodity_id = ?"
		args = append(args, baseCommodityID)
	}
	if quoteCommodityID > 0 {
		where += " AND quote_commodity_id = ?"
		args = append(args, quoteCommodityID)
	}
	args = append(args, limit)
	rows, err := r.database.QueryContext(ctx, priceObservationSelect(`
		WHERE `+where+`
		ORDER BY valuation_date DESC, recorded_at DESC, id DESC
		LIMIT ?
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list price observations: %w", err)
	}
	defer rows.Close()
	return scanPriceObservations(rows)
}

func (r *PricingRepository) ListLatestPriceObservationsForPairs(ctx context.Context, bookID int64, pairs [][2]int64) ([]PriceObservationRecord, error) {
	if len(pairs) == 0 {
		return []PriceObservationRecord{}, nil
	}

	hasCurrentSchema, err := r.tableHasColumn(ctx, "price_observations", "base_commodity_id")
	if err != nil {
		return nil, err
	}
	if !hasCurrentSchema {
		records := make([]PriceObservationRecord, 0, len(pairs))
		for _, pair := range pairs {
			pairRecords, err := r.listLegacyPriceObservations(ctx, bookID, pair[0], pair[1], 1)
			if err != nil {
				return nil, err
			}
			if len(pairRecords) > 0 {
				records = append(records, pairRecords[0])
			}
		}
		return records, nil
	}

	clauses := make([]string, 0, len(pairs))
	args := []any{bookID}
	for _, pair := range pairs {
		if pair[0] <= 0 || pair[1] <= 0 {
			continue
		}
		clauses = append(clauses, "(base_commodity_id = ? AND quote_commodity_id = ?)")
		args = append(args, pair[0], pair[1])
	}
	if len(clauses) == 0 {
		return []PriceObservationRecord{}, nil
	}

	rows, err := r.database.QueryContext(ctx, priceObservationSelect(`
		WHERE id IN (
			SELECT id
			FROM (
				SELECT id,
					ROW_NUMBER() OVER (
						PARTITION BY base_commodity_id, quote_commodity_id
						ORDER BY valuation_date DESC, recorded_at DESC, id DESC
					) AS row_number
				FROM price_observations
				WHERE book_id = ? AND voided_at IS NULL AND (`+strings.Join(clauses, " OR ")+`)
			)
			WHERE row_number = 1
		)
		ORDER BY base_commodity_id, quote_commodity_id
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list latest price observations for pairs: %w", err)
	}
	defer rows.Close()
	return scanPriceObservations(rows)
}

func (r *PricingRepository) listLegacyPriceObservations(ctx context.Context, bookID int64, baseCommodityID int64, quoteCommodityID int64, limit int) ([]PriceObservationRecord, error) {
	where := "book_id = ? AND voided_at IS NULL"
	args := []any{bookID}
	if baseCommodityID > 0 {
		where += " AND commodity_id = ?"
		args = append(args, baseCommodityID)
	}
	if quoteCommodityID > 0 {
		where += " AND quote_commodity_id = ?"
		args = append(args, quoteCommodityID)
	}
	args = append(args, limit)
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, book_id, 0 AS series_id, commodity_id AS base_commodity_id, quote_commodity_id,
			observation_kind AS quote_type, 'raw' AS adjustment_basis, price_value, price_scale,
			1 AS base_quantity_value, 0 AS base_quantity_scale, price_date AS valuation_date,
			NULL AS observed_at, NULL AS source_published_at, source_id, provider_observation_id,
			is_manual, is_derived, supersedes_observation_id, '{}' AS derivation_json, metadata_json,
			voided_at, created_at AS recorded_at, created_by_user_id
		FROM price_observations
		WHERE `+where+`
		ORDER BY price_date DESC, created_at DESC, id DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list legacy price observations: %w", err)
	}
	defer rows.Close()
	return scanPriceObservations(rows)
}

func (r *PricingRepository) tableHasColumn(ctx context.Context, tableName string, columnName string) (bool, error) {
	rows, err := r.database.QueryContext(ctx, `PRAGMA table_info(`+tableName+`)`)
	if err != nil {
		return false, fmt.Errorf("read %s schema: %w", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("scan %s schema: %w", tableName, err)
		}
		if name == columnName {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate %s schema: %w", tableName, err)
	}
	return false, nil
}

func (r *PricingRepository) GetPricingPolicy(ctx context.Context, bookID int64) (PricingPolicyRecord, error) {
	return scanPricingPolicyRow(r.database.QueryRowContext(ctx, pricingPolicySelect(`WHERE book_id = ?`), bookID))
}

func (r *PricingRepository) SavePricingPolicy(ctx context.Context, params SavePricingPolicyParams) (PricingPolicyRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return PricingPolicyRecord{}, fmt.Errorf("begin save pricing policy: %w", err)
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
		return PricingPolicyRecord{}, err
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pricing_policies WHERE book_id = ?)`, params.BookID).Scan(&exists); err != nil {
		return PricingPolicyRecord{}, fmt.Errorf("check pricing policy exists: %w", err)
	}
	if exists == 1 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE pricing_policies
			SET base_commodity_id = ?, default_source_id = ?, refresh_enabled = ?, refresh_hour_utc = ?,
				refresh_minute_utc = ?, refresh_hour_local = ?, refresh_minute_local = ?,
				max_backfill_days = ?, staleness_max_days = ?, triangulation_max_hops = ?,
				rounding_mode = ?, prefer_official_fx = ?, weekend_policy = ?, updated_at = ?,
				updated_by_user_id = ?, updated_audit_event_id = ?
			WHERE book_id = ?
		`, nullableInt64Value(params.Spec.BaseCommodityID), nullableInt64Value(params.Spec.DefaultSourceID), boolInt(params.Spec.RefreshEnabled), params.Spec.RefreshHourUTC, params.Spec.RefreshMinuteUTC, params.Spec.RefreshHourLocal, params.Spec.RefreshMinuteLocal, params.Spec.MaxBackfillDays, params.Spec.StalenessMaxDays, params.Spec.TriangulationMaxHops, params.Spec.RoundingMode, boolInt(params.Spec.PreferOfficialFX), params.Spec.WeekendPolicy, params.RecordedAt, params.ActorUserID, auditEventID, params.BookID); err != nil {
			return PricingPolicyRecord{}, fmt.Errorf("update pricing policy: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO pricing_policies (
				book_id, base_commodity_id, default_source_id, refresh_enabled, refresh_hour_utc,
				refresh_minute_utc, refresh_hour_local, refresh_minute_local, max_backfill_days,
				staleness_max_days, triangulation_max_hops,
				rounding_mode, prefer_official_fx, weekend_policy, created_at, created_by_user_id,
				updated_at, updated_by_user_id, created_audit_event_id, updated_audit_event_id
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, params.BookID, nullableInt64Value(params.Spec.BaseCommodityID), nullableInt64Value(params.Spec.DefaultSourceID), boolInt(params.Spec.RefreshEnabled), params.Spec.RefreshHourUTC, params.Spec.RefreshMinuteUTC, params.Spec.RefreshHourLocal, params.Spec.RefreshMinuteLocal, params.Spec.MaxBackfillDays, params.Spec.StalenessMaxDays, params.Spec.TriangulationMaxHops, params.Spec.RoundingMode, boolInt(params.Spec.PreferOfficialFX), params.Spec.WeekendPolicy, params.RecordedAt, params.ActorUserID, params.RecordedAt, params.ActorUserID, auditEventID, auditEventID); err != nil {
			return PricingPolicyRecord{}, fmt.Errorf("insert pricing policy: %w", err)
		}
	}
	record, err := scanPricingPolicyRow(tx.QueryRowContext(ctx, pricingPolicySelect(`WHERE book_id = ?`), params.BookID))
	if err != nil {
		return PricingPolicyRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return PricingPolicyRecord{}, fmt.Errorf("commit save pricing policy: %w", err)
	}
	committed = true
	return record, nil
}

func (r *PricingRepository) ListSourceAssignments(ctx context.Context, bookID int64) ([]PricingSourceAssignmentRecord, error) {
	rows, err := r.database.QueryContext(ctx, pricingSourceAssignmentSelect(`
		WHERE book_id = ?
		ORDER BY status, priority, base_commodity_id, quote_commodity_id, id
	`), bookID)
	if err != nil {
		return nil, fmt.Errorf("list pricing source assignments: %w", err)
	}
	defer rows.Close()
	return scanPricingSourceAssignments(rows)
}

func (r *PricingRepository) SourceAssignmentForPair(ctx context.Context, bookID int64, baseCommodityID int64, quoteCommodityID int64, date string) (PricingSourceAssignmentRecord, error) {
	return scanPricingSourceAssignmentRow(r.database.QueryRowContext(ctx, pricingSourceAssignmentSelect(`
		WHERE book_id = ?
			AND base_commodity_id = ?
			AND quote_commodity_id = ?
			AND status = 'active'
			AND effective_from <= ?
			AND (effective_to IS NULL OR effective_to >= ?)
		ORDER BY priority, effective_from DESC, id
		LIMIT 1
	`), bookID, baseCommodityID, quoteCommodityID, date, date))
}

func (r *PricingRepository) SaveSourceAssignment(ctx context.Context, params SavePricingSourceAssignmentParams) (PricingSourceAssignmentRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return PricingSourceAssignmentRecord{}, fmt.Errorf("begin save pricing source assignment: %w", err)
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
		return PricingSourceAssignmentRecord{}, err
	}
	var assignmentID int64
	if params.AssignmentID > 0 {
		result, err := tx.ExecContext(ctx, `
			UPDATE pricing_source_assignments
			SET base_commodity_id = ?, quote_commodity_id = ?, source_id = ?, priority = ?, status = ?,
				effective_from = ?, effective_to = ?
			WHERE book_id = ? AND id = ?
		`, params.Spec.BaseCommodityID, params.Spec.QuoteCommodityID, params.Spec.SourceID, params.Spec.Priority, params.Spec.Status, params.Spec.EffectiveFrom, nullableStringValue(params.Spec.EffectiveTo), params.BookID, params.AssignmentID)
		if err != nil {
			return PricingSourceAssignmentRecord{}, fmt.Errorf("update pricing source assignment: %w", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return PricingSourceAssignmentRecord{}, fmt.Errorf("read pricing source assignment update rows: %w", err)
		}
		if rowsAffected == 0 {
			return PricingSourceAssignmentRecord{}, ErrNotFound
		}
		assignmentID = params.AssignmentID
	} else {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO pricing_source_assignments (
				book_id, base_commodity_id, quote_commodity_id, source_id, priority, status,
				effective_from, effective_to, created_at, created_by_user_id, created_audit_event_id
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, params.BookID, params.Spec.BaseCommodityID, params.Spec.QuoteCommodityID, params.Spec.SourceID, params.Spec.Priority, params.Spec.Status, params.Spec.EffectiveFrom, nullableStringValue(params.Spec.EffectiveTo), params.RecordedAt, params.ActorUserID, auditEventID)
		if err != nil {
			return PricingSourceAssignmentRecord{}, fmt.Errorf("insert pricing source assignment: %w", err)
		}
		assignmentID, err = result.LastInsertId()
		if err != nil {
			return PricingSourceAssignmentRecord{}, fmt.Errorf("read pricing source assignment id: %w", err)
		}
	}
	record, err := pricingSourceAssignmentByIDTx(ctx, tx, params.BookID, assignmentID)
	if err != nil {
		return PricingSourceAssignmentRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return PricingSourceAssignmentRecord{}, fmt.Errorf("commit save pricing source assignment: %w", err)
	}
	committed = true
	return record, nil
}

func (r *PricingRepository) RecordRefreshRun(ctx context.Context, bookID int64, sourceID int64, trigger string, status string, startedAt string, finishedAt string, itemsTotal int, itemsSucceeded int, itemsFailed int, lastError string) (PricingRefreshRunRecord, error) {
	result, err := r.database.ExecContext(ctx, `
		INSERT INTO market_data_ingest_runs (
			book_id, source_id, trigger, status, started_at, finished_at, items_total, items_succeeded, items_failed, last_error
		)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''))
	`, bookID, sourceID, trigger, status, startedAt, finishedAt, itemsTotal, itemsSucceeded, itemsFailed, lastError)
	if err != nil {
		return PricingRefreshRunRecord{}, fmt.Errorf("insert pricing refresh run: %w", err)
	}
	runID, err := result.LastInsertId()
	if err != nil {
		return PricingRefreshRunRecord{}, fmt.Errorf("read pricing refresh run id: %w", err)
	}
	return r.RefreshRunByID(ctx, bookID, runID)
}

func (r *PricingRepository) FinishRefreshRun(ctx context.Context, bookID int64, runID int64, status string, finishedAt string, itemsTotal int, itemsSucceeded int, itemsFailed int, lastError string) (PricingRefreshRunRecord, error) {
	result, err := r.database.ExecContext(ctx, `
		UPDATE market_data_ingest_runs
		SET status = ?, finished_at = NULLIF(?, ''), items_total = ?, items_succeeded = ?,
			items_failed = ?, last_error = NULLIF(?, '')
		WHERE book_id = ? AND id = ?
	`, status, finishedAt, itemsTotal, itemsSucceeded, itemsFailed, lastError, bookID, runID)
	if err != nil {
		return PricingRefreshRunRecord{}, fmt.Errorf("finish pricing refresh run: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return PricingRefreshRunRecord{}, fmt.Errorf("read pricing refresh run finish rows: %w", err)
	}
	if rowsAffected == 0 {
		return PricingRefreshRunRecord{}, ErrNotFound
	}
	return r.RefreshRunByID(ctx, bookID, runID)
}

func (r *PricingRepository) RecordRefreshItem(ctx context.Context, spec PricingRefreshItemSpec) error {
	if spec.RawJSON == "" {
		spec.RawJSON = "{}"
	}
	_, err := r.database.ExecContext(ctx, `
		INSERT INTO market_data_ingest_items (
			run_id, book_id, item_kind, status, provider_item_id, local_ref_table,
			local_ref_id, error, raw_json, created_at
		)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''), ?, ?)
	`, spec.RunID, spec.BookID, spec.ItemKind, spec.Status, spec.ProviderItemID, spec.LocalRefTable, nullablePositiveInt64(spec.LocalRefID), spec.Error, spec.RawJSON, spec.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert pricing refresh item: %w", err)
	}
	return nil
}

func (r *PricingRepository) UpsertRefreshState(ctx context.Context, spec PricingRefreshStateSpec) error {
	_, err := r.database.ExecContext(ctx, `
		INSERT INTO pricing_refresh_state (
			book_id, base_commodity_id, quote_commodity_id, source_id,
			last_success_date, last_attempt_at, last_error, updated_at
		)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?)
		ON CONFLICT(book_id, base_commodity_id, quote_commodity_id, source_id)
		DO UPDATE SET
			last_success_date = COALESCE(excluded.last_success_date, pricing_refresh_state.last_success_date),
			last_attempt_at = excluded.last_attempt_at,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at
	`, spec.BookID, spec.BaseCommodityID, spec.QuoteCommodityID, spec.SourceID, nullableStringValue(spec.LastSuccessDate), spec.LastAttemptAt, spec.LastError, spec.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert pricing refresh state: %w", err)
	}
	return nil
}

func (r *PricingRepository) ScheduledRefreshRunExistsOnDate(ctx context.Context, bookID int64, date string) (bool, error) {
	var exists int
	if err := r.database.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM market_data_ingest_runs
			WHERE book_id = ?
				AND trigger = 'scheduled'
				AND substr(started_at, 1, 10) = ?
		)
	`, bookID, date).Scan(&exists); err != nil {
		return false, fmt.Errorf("read scheduled pricing refresh existence: %w", err)
	}
	return exists == 1, nil
}

func (r *PricingRepository) ScheduledRefreshRunExistsBetween(ctx context.Context, bookID int64, startUTC string, endUTC string) (bool, error) {
	var exists int
	if err := r.database.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM market_data_ingest_runs
			WHERE book_id = ?
				AND trigger = 'scheduled'
				AND started_at >= ?
				AND started_at < ?
		)
	`, bookID, startUTC, endUTC).Scan(&exists); err != nil {
		return false, fmt.Errorf("read scheduled pricing refresh existence: %w", err)
	}
	return exists == 1, nil
}

func (r *PricingRepository) UserTimeZone(ctx context.Context, userID int64) (string, error) {
	var timeZone string
	if err := r.database.QueryRowContext(ctx, `
		SELECT time_zone
		FROM user_preferences
		WHERE user_id = ?
	`, userID).Scan(&timeZone); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "UTC", nil
		}
		return "", fmt.Errorf("read user time zone: %w", err)
	}
	return timeZone, nil
}

func (r *PricingRepository) BookOwnerTimeZone(ctx context.Context, bookID int64) (string, error) {
	var timeZone string
	if err := r.database.QueryRowContext(ctx, `
		SELECT COALESCE(up.time_zone, 'UTC')
		FROM books b
		LEFT JOIN user_preferences up ON up.user_id = b.owner_user_id
		WHERE b.id = ?
	`, bookID).Scan(&timeZone); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "UTC", nil
		}
		return "", fmt.Errorf("read book owner time zone: %w", err)
	}
	return timeZone, nil
}

func (r *PricingRepository) ListRefreshRuns(ctx context.Context, bookID int64, limit int) ([]PricingRefreshRunRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, book_id, source_id, trigger, status, started_at, finished_at, items_total, items_succeeded, items_failed, last_error
		FROM market_data_ingest_runs
		WHERE book_id = ?
		ORDER BY started_at DESC, id DESC
		LIMIT ?
	`, bookID, limit)
	if err != nil {
		return nil, fmt.Errorf("list pricing refresh runs: %w", err)
	}
	defer rows.Close()
	return scanRefreshRuns(rows)
}

func (r *PricingRepository) RefreshRunByID(ctx context.Context, bookID int64, runID int64) (PricingRefreshRunRecord, error) {
	var record PricingRefreshRunRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT id, book_id, source_id, trigger, status, started_at, finished_at, items_total, items_succeeded, items_failed, last_error
		FROM market_data_ingest_runs
		WHERE book_id = ? AND id = ?
	`, bookID, runID).Scan(&record.ID, &record.BookID, &record.SourceID, &record.Trigger, &record.Status, &record.StartedAt, &record.FinishedAt, &record.ItemsTotal, &record.ItemsSucceeded, &record.ItemsFailed, &record.LastError); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PricingRefreshRunRecord{}, ErrNotFound
		}
		return PricingRefreshRunRecord{}, fmt.Errorf("scan pricing refresh run: %w", err)
	}
	return record, nil
}

func (r *PricingRepository) ListRefreshState(ctx context.Context, bookID int64) ([]PricingRefreshStateRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, book_id, base_commodity_id, quote_commodity_id, source_id, last_success_date, last_attempt_at, last_error, updated_at
		FROM pricing_refresh_state
		WHERE book_id = ?
		ORDER BY updated_at DESC, id DESC
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list pricing refresh state: %w", err)
	}
	defer rows.Close()
	var records []PricingRefreshStateRecord
	for rows.Next() {
		var record PricingRefreshStateRecord
		if err := rows.Scan(&record.ID, &record.BookID, &record.BaseCommodityID, &record.QuoteCommodityID, &record.SourceID, &record.LastSuccessDate, &record.LastAttemptAt, &record.LastError, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan pricing refresh state: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pricing refresh state: %w", err)
	}
	return records, nil
}

func ensurePriceSeriesTx(ctx context.Context, tx *sql.Tx, bookID int64, spec PriceSeriesSpec, actorUserID int64, createdAt string, auditEventID int64) (PriceSeriesRecord, error) {
	sourceIDValue := int64(0)
	if spec.SourceID.Valid {
		sourceIDValue = spec.SourceID.Int64
	}
	var seriesID int64
	err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM price_series
		WHERE book_id = ?
			AND base_commodity_id = ?
			AND quote_commodity_id = ?
			AND IFNULL(source_id, 0) = ?
			AND quote_type = ?
			AND adjustment_basis = ?
			AND IFNULL(market_code, '') = ?
			AND IFNULL(provider_series_id, '') = ?
			AND status = 'active'
		ORDER BY id
		LIMIT 1
	`, bookID, spec.BaseCommodityID, spec.QuoteCommodityID, sourceIDValue, spec.QuoteType, spec.AdjustmentBasis, strings.TrimSpace(spec.MarketCode), strings.TrimSpace(spec.ProviderSeriesID)).Scan(&seriesID)
	if err == nil {
		return priceSeriesByIDTx(ctx, tx, bookID, seriesID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return PriceSeriesRecord{}, fmt.Errorf("find price series: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO price_series (
			book_id, base_commodity_id, quote_commodity_id, source_id, quote_type,
			adjustment_basis, market_code, provider_series_id, status, metadata_json,
			created_at, created_by_user_id, created_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), 'active', ?, ?, ?, ?)
	`, bookID, spec.BaseCommodityID, spec.QuoteCommodityID, nullableInt64Value(spec.SourceID), spec.QuoteType, spec.AdjustmentBasis, strings.TrimSpace(spec.MarketCode), strings.TrimSpace(spec.ProviderSeriesID), spec.MetadataJSON, createdAt, actorUserID, auditEventID)
	if err != nil {
		return PriceSeriesRecord{}, fmt.Errorf("insert price series: %w", err)
	}
	seriesID, err = result.LastInsertId()
	if err != nil {
		return PriceSeriesRecord{}, fmt.Errorf("read price series id: %w", err)
	}
	return priceSeriesByIDTx(ctx, tx, bookID, seriesID)
}

func priceSeriesByIDTx(ctx context.Context, tx *sql.Tx, bookID int64, seriesID int64) (PriceSeriesRecord, error) {
	var record PriceSeriesRecord
	if err := tx.QueryRowContext(ctx, `
		SELECT id, book_id, base_commodity_id, quote_commodity_id, source_id, quote_type,
			adjustment_basis, market_code, provider_series_id, status, metadata_json,
			created_at, created_by_user_id
		FROM price_series
		WHERE book_id = ? AND id = ?
	`, bookID, seriesID).Scan(&record.ID, &record.BookID, &record.BaseCommodityID, &record.QuoteCommodityID, &record.SourceID, &record.QuoteType, &record.AdjustmentBasis, &record.MarketCode, &record.ProviderSeriesID, &record.Status, &record.MetadataJSON, &record.CreatedAt, &record.CreatedByUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PriceSeriesRecord{}, ErrNotFound
		}
		return PriceSeriesRecord{}, fmt.Errorf("scan price series: %w", err)
	}
	return record, nil
}

func priceObservationByIDTx(ctx context.Context, tx *sql.Tx, bookID int64, observationID int64) (PriceObservationRecord, error) {
	return scanPriceObservationRow(tx.QueryRowContext(ctx, priceObservationSelect(`
		WHERE book_id = ? AND id = ?
	`), bookID, observationID))
}

func priceObservationSelect(whereClause string) string {
	return `
		SELECT id, book_id, series_id, base_commodity_id, quote_commodity_id, quote_type,
			adjustment_basis, price_value, price_scale, base_quantity_value, base_quantity_scale,
			valuation_date, observed_at, source_published_at, source_id, provider_observation_id,
			is_manual, is_derived, supersedes_observation_id, derivation_json, metadata_json,
			voided_at, recorded_at, created_by_user_id
		FROM price_observations
	` + whereClause
}

func scanPriceObservationRow(row rowScanner) (PriceObservationRecord, error) {
	var record PriceObservationRecord
	var isManual int
	var isDerived int
	if err := row.Scan(&record.ID, &record.BookID, &record.SeriesID, &record.BaseCommodityID, &record.QuoteCommodityID, &record.QuoteType, &record.AdjustmentBasis, &record.PriceValue, &record.PriceScale, &record.BaseQuantityValue, &record.BaseQuantityScale, &record.ValuationDate, &record.ObservedAt, &record.SourcePublishedAt, &record.SourceID, &record.ProviderObservationID, &isManual, &isDerived, &record.SupersedesObservationID, &record.DerivationJSON, &record.MetadataJSON, &record.VoidedAt, &record.RecordedAt, &record.CreatedByUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PriceObservationRecord{}, ErrNotFound
		}
		return PriceObservationRecord{}, fmt.Errorf("scan price observation: %w", err)
	}
	record.IsManual = isManual == 1
	record.IsDerived = isDerived == 1
	return record, nil
}

func scanPriceObservations(rows *sql.Rows) ([]PriceObservationRecord, error) {
	var records []PriceObservationRecord
	for rows.Next() {
		record, err := scanPriceObservationRow(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate price observations: %w", err)
	}
	return records, nil
}

func pricingPolicySelect(whereClause string) string {
	return `
		SELECT id, book_id, base_commodity_id, default_source_id, refresh_enabled, refresh_hour_utc,
			refresh_minute_utc, refresh_hour_local, refresh_minute_local, max_backfill_days, staleness_max_days, triangulation_max_hops,
			rounding_mode, prefer_official_fx, weekend_policy, created_at, created_by_user_id,
			updated_at, updated_by_user_id
		FROM pricing_policies
	` + whereClause
}

func scanPricingPolicyRow(row rowScanner) (PricingPolicyRecord, error) {
	var record PricingPolicyRecord
	var refreshEnabled int
	var preferOfficialFX int
	if err := row.Scan(&record.ID, &record.BookID, &record.BaseCommodityID, &record.DefaultSourceID, &refreshEnabled, &record.RefreshHourUTC, &record.RefreshMinuteUTC, &record.RefreshHourLocal, &record.RefreshMinuteLocal, &record.MaxBackfillDays, &record.StalenessMaxDays, &record.TriangulationMaxHops, &record.RoundingMode, &preferOfficialFX, &record.WeekendPolicy, &record.CreatedAt, &record.CreatedByUserID, &record.UpdatedAt, &record.UpdatedByUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PricingPolicyRecord{}, ErrNotFound
		}
		return PricingPolicyRecord{}, fmt.Errorf("scan pricing policy: %w", err)
	}
	record.RefreshEnabled = refreshEnabled == 1
	record.PreferOfficialFX = preferOfficialFX == 1
	return record, nil
}

func pricingSourceAssignmentSelect(whereClause string) string {
	return `
		SELECT id, book_id, base_commodity_id, quote_commodity_id, source_id, priority, status,
			effective_from, effective_to, created_at, created_by_user_id
		FROM pricing_source_assignments
	` + whereClause
}

func pricingSourceAssignmentByIDTx(ctx context.Context, tx *sql.Tx, bookID int64, assignmentID int64) (PricingSourceAssignmentRecord, error) {
	return scanPricingSourceAssignmentRow(tx.QueryRowContext(ctx, pricingSourceAssignmentSelect(`
		WHERE book_id = ? AND id = ?
	`), bookID, assignmentID))
}

func scanPricingSourceAssignmentRow(row rowScanner) (PricingSourceAssignmentRecord, error) {
	var record PricingSourceAssignmentRecord
	if err := row.Scan(&record.ID, &record.BookID, &record.BaseCommodityID, &record.QuoteCommodityID, &record.SourceID, &record.Priority, &record.Status, &record.EffectiveFrom, &record.EffectiveTo, &record.CreatedAt, &record.CreatedByUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PricingSourceAssignmentRecord{}, ErrNotFound
		}
		return PricingSourceAssignmentRecord{}, fmt.Errorf("scan pricing source assignment: %w", err)
	}
	return record, nil
}

func scanPricingSourceAssignments(rows *sql.Rows) ([]PricingSourceAssignmentRecord, error) {
	var records []PricingSourceAssignmentRecord
	for rows.Next() {
		record, err := scanPricingSourceAssignmentRow(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pricing source assignments: %w", err)
	}
	return records, nil
}

func scanRefreshRuns(rows *sql.Rows) ([]PricingRefreshRunRecord, error) {
	var records []PricingRefreshRunRecord
	for rows.Next() {
		var record PricingRefreshRunRecord
		if err := rows.Scan(&record.ID, &record.BookID, &record.SourceID, &record.Trigger, &record.Status, &record.StartedAt, &record.FinishedAt, &record.ItemsTotal, &record.ItemsSucceeded, &record.ItemsFailed, &record.LastError); err != nil {
			return nil, fmt.Errorf("scan pricing refresh run: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pricing refresh runs: %w", err)
	}
	return records, nil
}
