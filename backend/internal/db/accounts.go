package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var ErrSystemAccountsSetupComplete = errors.New("system accounts setup already complete")

type AccountRepository struct {
	database *sql.DB
}

type AccountRecord struct {
	ID                    int64
	BookID                int64
	IsSystem              bool
	SystemRole            sql.NullString
	CreatedAt             string
	CreatedByUserID       int64
	VersionID             int64
	VersionSeq            int64
	EffectiveFrom         string
	RecordedAt            string
	ChangedByUserID       int64
	ChangeReason          string
	Status                string
	Code                  sql.NullString
	Name                  sql.NullString
	AccountClass          string
	AccountKind           string
	ParentAccountID       sql.NullInt64
	InstitutionID         sql.NullInt64
	CountryCode           sql.NullString
	DefaultCommodityID    sql.NullInt64
	QuantityScaleOverride sql.NullInt64
	AllowsPostings        bool
	NumberLast4           sql.NullString
	ExternalRefHint       sql.NullString
	CommentMarkdown       string
	MetadataJSON          string
}

type AccountSpec struct {
	Code                  string
	Name                  string
	AccountClass          string
	AccountKind           string
	ParentAccountID       sql.NullInt64
	InstitutionID         sql.NullInt64
	CountryCode           string
	DefaultCommodityID    sql.NullInt64
	QuantityScaleOverride sql.NullInt64
	AllowsPostings        bool
	NumberLast4           string
	ExternalRefHint       string
	CommentMarkdown       string
	MetadataJSON          string
}

type ListAccountsParams struct {
	BookID          int64
	Status          string
	AccountClass    string
	IncludeArchived bool
	Query           string
	Limit           int
}

type CreateAccountParams struct {
	BookID          int64
	CreatedByUserID int64
	AuthSessionID   int64
	RequestID       string
	IsSystem        bool
	SystemRole      string
	Spec            AccountSpec
	ChangeReason    string
	CreatedAt       string
	EffectiveFrom   string
}

type UpdateAccountParams struct {
	BookID          int64
	AccountID       int64
	ChangedByUserID int64
	AuthSessionID   int64
	RequestID       string
	Operation       string
	Spec            AccountSpec
	Status          string
	ChangeReason    string
	RecordedAt      string
	EffectiveFrom   string
}

type SystemAccountSpec struct {
	Role         string
	AccountClass string
	AccountKind  string
}

type EnsureSystemAccountsParams struct {
	BookID          int64
	ChangedByUserID int64
	AuthSessionID   int64
	RequestID       string
	Specs           []SystemAccountSpec
	CreatedAt       string
	EffectiveFrom   string
}

type EnsureSystemAccountsRecord struct {
	Accounts []AccountRecord
}

type AccountCommodityRecord struct {
	ID               int64
	BookID           int64
	Kind             string
	Status           string
	MaxQuantityScale int
}

func NewAccountRepository(database *sql.DB) *AccountRepository {
	return &AccountRepository{database: database}
}

func (r *AccountRepository) ListAccounts(ctx context.Context, params ListAccountsParams) ([]AccountRecord, error) {
	where := []string{"a.book_id = ?"}
	args := []any{params.BookID}

	if params.Status != "" {
		where = append(where, "av.status = ?")
		args = append(args, params.Status)
	} else if !params.IncludeArchived {
		where = append(where, "av.status != 'archived'")
	}

	if params.AccountClass != "" {
		where = append(where, "av.account_class = ?")
		args = append(args, params.AccountClass)
	}

	query := strings.TrimSpace(params.Query)
	if query != "" {
		where = append(where, "(av.name LIKE ? OR av.code LIKE ? OR av.account_kind LIKE ? OR a.system_role LIKE ?)")
		like := "%" + query + "%"
		args = append(args, like, like, like, like)
	}

	args = append(args, params.Limit)

	rows, err := r.database.QueryContext(ctx, currentAccountSelect("AND "+strings.Join(where, " AND ")+`
		ORDER BY
			CASE av.account_class
				WHEN 'asset' THEN 1
				WHEN 'liability' THEN 2
				WHEN 'equity' THEN 3
				WHEN 'income' THEN 4
				WHEN 'expense' THEN 5
				ELSE 99
			END,
			COALESCE(av.name, a.system_role, ''),
			a.id
		LIMIT ?
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	return scanAccountRecords(rows)
}

func (r *AccountRepository) CurrentAccountByID(ctx context.Context, bookID int64, accountID int64) (AccountRecord, error) {
	var record AccountRecord
	if err := scanAccountRecord(r.database.QueryRowContext(ctx, currentAccountSelect(`
		AND a.book_id = ? AND a.id = ?
	`), bookID, accountID), &record); err != nil {
		return AccountRecord{}, err
	}

	return record, nil
}

func (r *AccountRepository) ListAccountVersions(ctx context.Context, bookID int64, accountID int64) ([]AccountRecord, error) {
	rows, err := r.database.QueryContext(ctx, accountSelect(`
		WHERE a.book_id = ? AND a.id = ?
		ORDER BY av.version_seq DESC
	`), bookID, accountID)
	if err != nil {
		return nil, fmt.Errorf("list account versions: %w", err)
	}
	defer rows.Close()

	records, err := scanAccountRecords(rows)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, ErrNotFound
	}

	return records, nil
}

func (r *AccountRepository) CreateAccount(ctx context.Context, params CreateAccountParams) (AccountRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("begin create account transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return AccountRecord{}, err
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.CreatedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    "browser_api",
		Operation:     "account.create",
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return AccountRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO accounts (book_id, is_system, system_role, created_at, created_by_user_id, created_request_id, created_audit_event_id)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, NULLIF(?, ''), ?)
	`, params.BookID, boolToInt(params.IsSystem), params.SystemRole, params.CreatedAt, params.CreatedByUserID, params.RequestID, auditEventID)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("insert account: %w", err)
	}

	accountID, err := result.LastInsertId()
	if err != nil {
		return AccountRecord{}, fmt.Errorf("read account id: %w", err)
	}

	record, err := insertAccountVersion(ctx, tx, insertAccountVersionParams{
		BookID:             params.BookID,
		AccountID:          accountID,
		CreatedAt:          params.CreatedAt,
		CreatedByUserID:    params.CreatedByUserID,
		VersionSeq:         1,
		EffectiveFrom:      params.EffectiveFrom,
		RecordedAt:         params.CreatedAt,
		ChangedByUserID:    params.CreatedByUserID,
		ChangeReason:       params.ChangeReason,
		ChangeAuditEventID: auditEventID,
		Status:             "active",
		Spec:               params.Spec,
	})
	if err != nil {
		return AccountRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return AccountRecord{}, fmt.Errorf("commit create account transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *AccountRepository) UpdateAccount(ctx context.Context, params UpdateAccountParams) (AccountRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("begin update account transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	current, err := currentAccountByID(ctx, tx, params.BookID, params.AccountID)
	if err != nil {
		return AccountRecord{}, err
	}

	status := params.Status
	if status == "" {
		status = current.Status
	}

	changeReason := strings.TrimSpace(params.ChangeReason)
	if changeReason == "" {
		changeReason = "updated account"
	}
	operation := strings.TrimSpace(params.Operation)
	if operation == "" {
		operation = "account.update"
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ChangedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.RecordedAt,
		RequestID:     params.RequestID,
		OriginType:    "browser_api",
		Operation:     operation,
		Reason:        changeReason,
	})
	if err != nil {
		return AccountRecord{}, err
	}

	record, err := insertAccountVersion(ctx, tx, insertAccountVersionParams{
		BookID:             params.BookID,
		AccountID:          params.AccountID,
		CreatedAt:          current.CreatedAt,
		CreatedByUserID:    current.CreatedByUserID,
		VersionSeq:         current.VersionSeq + 1,
		EffectiveFrom:      params.EffectiveFrom,
		RecordedAt:         params.RecordedAt,
		ChangedByUserID:    params.ChangedByUserID,
		ChangeReason:       changeReason,
		ChangeAuditEventID: auditEventID,
		Status:             status,
		Spec:               params.Spec,
	})
	if err != nil {
		return AccountRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return AccountRecord{}, fmt.Errorf("commit update account transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *AccountRepository) EnsureSystemAccounts(ctx context.Context, params EnsureSystemAccountsParams) (EnsureSystemAccountsRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return EnsureSystemAccountsRecord{}, fmt.Errorf("begin system accounts setup transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var completedAt sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT completed_at
		FROM setup_steps
		WHERE step_key = 'system_accounts'
	`).Scan(&completedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EnsureSystemAccountsRecord{}, ErrNotFound
		}
		return EnsureSystemAccountsRecord{}, fmt.Errorf("read system accounts setup step: %w", err)
	}
	if completedAt.Valid {
		return EnsureSystemAccountsRecord{}, ErrSystemAccountsSetupComplete
	}

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return EnsureSystemAccountsRecord{}, err
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ChangedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    "setup",
		Operation:     "system_accounts.setup",
		Reason:        "seeded system accounts",
	})
	if err != nil {
		return EnsureSystemAccountsRecord{}, err
	}

	records := make([]AccountRecord, 0, len(params.Specs))
	for _, spec := range params.Specs {
		record, err := r.ensureSystemAccount(ctx, tx, params, spec, auditEventID)
		if err != nil {
			return EnsureSystemAccountsRecord{}, err
		}
		records = append(records, record)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE setup_steps
		SET completed_at = ?
		WHERE step_key = 'system_accounts' AND completed_at IS NULL
	`, params.CreatedAt)
	if err != nil {
		return EnsureSystemAccountsRecord{}, fmt.Errorf("mark system accounts setup complete: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return EnsureSystemAccountsRecord{}, fmt.Errorf("read system accounts setup rows affected: %w", err)
	}
	if rowsAffected != 1 {
		return EnsureSystemAccountsRecord{}, fmt.Errorf("mark system accounts setup complete: updated %d rows", rowsAffected)
	}

	if err := tx.Commit(); err != nil {
		return EnsureSystemAccountsRecord{}, fmt.Errorf("commit system accounts setup transaction: %w", err)
	}
	committed = true

	return EnsureSystemAccountsRecord{Accounts: records}, nil
}

func (r *AccountRepository) WouldCreateCycle(ctx context.Context, bookID int64, accountID int64, parentAccountID int64) (bool, error) {
	if accountID <= 0 || parentAccountID <= 0 {
		return false, nil
	}

	rows, err := r.database.QueryContext(ctx, `
		WITH RECURSIVE ancestors(id, parent_id) AS (
			SELECT a.id, av.parent_account_id
			FROM accounts a
			JOIN account_versions av ON av.account_id = a.id
			WHERE av.version_seq = (
				SELECT MAX(latest.version_seq)
				FROM account_versions latest
				WHERE latest.account_id = a.id
			)
				AND a.book_id = ?
				AND a.id = ?
			UNION ALL
			SELECT parent.id, parent_version.parent_account_id
			FROM accounts parent
			JOIN account_versions parent_version ON parent_version.account_id = parent.id
			JOIN ancestors ON ancestors.parent_id = parent.id
			WHERE parent_version.version_seq = (
				SELECT MAX(latest.version_seq)
				FROM account_versions latest
				WHERE latest.account_id = parent.id
			)
				AND parent.book_id = ?
		)
		SELECT id
		FROM ancestors
	`, bookID, parentAccountID, bookID)
	if err != nil {
		return false, fmt.Errorf("read account ancestors: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ancestorID int64
		if err := rows.Scan(&ancestorID); err != nil {
			return false, fmt.Errorf("scan account ancestor: %w", err)
		}
		if ancestorID == accountID {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate account ancestors: %w", err)
	}

	return false, nil
}

func (r *AccountRepository) CurrentCommodityByID(ctx context.Context, bookID int64, commodityID int64) (AccountCommodityRecord, error) {
	var record AccountCommodityRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT c.id, c.book_id, c.kind, cv.status, cv.max_quantity_scale
		FROM commodities c
		JOIN commodity_versions cv ON cv.commodity_id = c.id
		WHERE cv.version_seq = (
			SELECT MAX(latest.version_seq)
			FROM commodity_versions latest
			WHERE latest.commodity_id = c.id
		)
			AND c.book_id = ?
			AND c.id = ?
	`, bookID, commodityID).Scan(&record.ID, &record.BookID, &record.Kind, &record.Status, &record.MaxQuantityScale); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AccountCommodityRecord{}, ErrNotFound
		}
		return AccountCommodityRecord{}, fmt.Errorf("read account commodity: %w", err)
	}

	return record, nil
}

func (r *AccountRepository) ensureSystemAccount(ctx context.Context, tx *sql.Tx, params EnsureSystemAccountsParams, systemSpec SystemAccountSpec, auditEventID int64) (AccountRecord, error) {
	record, err := currentSystemAccountByRole(ctx, tx, params.BookID, systemSpec.Role)
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return AccountRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO accounts (book_id, is_system, system_role, created_at, created_by_user_id, created_request_id, created_audit_event_id)
		VALUES (?, 1, ?, ?, ?, NULLIF(?, ''), ?)
	`, params.BookID, systemSpec.Role, params.CreatedAt, params.ChangedByUserID, params.RequestID, auditEventID)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("insert system account: %w", err)
	}

	accountID, err := result.LastInsertId()
	if err != nil {
		return AccountRecord{}, fmt.Errorf("read system account id: %w", err)
	}

	return insertAccountVersion(ctx, tx, insertAccountVersionParams{
		BookID:             params.BookID,
		AccountID:          accountID,
		CreatedAt:          params.CreatedAt,
		CreatedByUserID:    params.ChangedByUserID,
		VersionSeq:         1,
		EffectiveFrom:      params.EffectiveFrom,
		RecordedAt:         params.CreatedAt,
		ChangedByUserID:    params.ChangedByUserID,
		ChangeReason:       "seeded system account",
		ChangeAuditEventID: auditEventID,
		Status:             "active",
		Spec: AccountSpec{
			Code: systemSpec.Role,
			// System account display names are derived from system_role at the
			// frontend localization boundary; do not seed English names here.
			AccountClass:   systemSpec.AccountClass,
			AccountKind:    systemSpec.AccountKind,
			AllowsPostings: true,
			MetadataJSON:   `{"source":"system_account_seed"}`,
		},
	})
}

type insertAccountVersionParams struct {
	BookID             int64
	AccountID          int64
	CreatedAt          string
	CreatedByUserID    int64
	VersionSeq         int64
	EffectiveFrom      string
	RecordedAt         string
	ChangedByUserID    int64
	ChangeReason       string
	ChangeAuditEventID int64
	Status             string
	Spec               AccountSpec
}

func insertAccountVersion(ctx context.Context, tx *sql.Tx, params insertAccountVersionParams) (AccountRecord, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO account_versions (
			account_id,
			version_seq,
			effective_from,
			recorded_at,
			changed_by_user_id,
			change_reason,
			status,
			code,
			name,
			account_class,
			account_kind,
			parent_account_id,
			institution_id,
			country_code,
			default_commodity_id,
			quantity_scale_override,
			allows_postings,
			number_last4,
			external_ref_hint,
			comment_markdown,
			metadata_json,
			change_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?)
	`, params.AccountID, params.VersionSeq, params.EffectiveFrom, params.RecordedAt, params.ChangedByUserID, params.ChangeReason, params.Status, params.Spec.Code, params.Spec.Name, params.Spec.AccountClass, params.Spec.AccountKind, nullableInt64Value(params.Spec.ParentAccountID), nullableInt64Value(params.Spec.InstitutionID), params.Spec.CountryCode, nullableInt64Value(params.Spec.DefaultCommodityID), nullableInt64Value(params.Spec.QuantityScaleOverride), boolToInt(params.Spec.AllowsPostings), params.Spec.NumberLast4, params.Spec.ExternalRefHint, params.Spec.CommentMarkdown, params.Spec.MetadataJSON, params.ChangeAuditEventID)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("insert account version: %w", err)
	}

	versionID, err := result.LastInsertId()
	if err != nil {
		return AccountRecord{}, fmt.Errorf("read account version id: %w", err)
	}

	record, err := currentAccountByID(ctx, tx, params.BookID, params.AccountID)
	if err != nil {
		return AccountRecord{}, err
	}
	record.VersionID = versionID

	return record, nil
}

func currentAccountByID(ctx context.Context, tx *sql.Tx, bookID int64, accountID int64) (AccountRecord, error) {
	var record AccountRecord
	if err := scanAccountRecord(tx.QueryRowContext(ctx, currentAccountSelect(`
		AND a.book_id = ? AND a.id = ?
	`), bookID, accountID), &record); err != nil {
		return AccountRecord{}, err
	}

	return record, nil
}

func currentSystemAccountByRole(ctx context.Context, tx *sql.Tx, bookID int64, role string) (AccountRecord, error) {
	var record AccountRecord
	if err := scanAccountRecord(tx.QueryRowContext(ctx, currentAccountSelect(`
		AND a.book_id = ? AND a.system_role = ?
	`), bookID, role), &record); err != nil {
		return AccountRecord{}, err
	}

	return record, nil
}

func currentAccountSelect(extraConditions string) string {
	return accountSelect(`
		WHERE av.version_seq = (
			SELECT MAX(latest.version_seq)
			FROM account_versions latest
			WHERE latest.account_id = a.id
		)
	` + extraConditions)
}

func accountSelect(whereClause string) string {
	return `
		SELECT
			a.id,
			a.book_id,
			a.is_system,
			a.system_role,
			a.created_at,
			a.created_by_user_id,
			av.id,
			av.version_seq,
			av.effective_from,
			av.recorded_at,
			av.changed_by_user_id,
			av.change_reason,
			av.status,
			av.code,
			av.name,
			av.account_class,
			av.account_kind,
			av.parent_account_id,
			av.institution_id,
			av.country_code,
			av.default_commodity_id,
			av.quantity_scale_override,
			av.allows_postings,
			av.number_last4,
			av.external_ref_hint,
			av.comment_markdown,
			av.metadata_json
		FROM accounts a
		JOIN account_versions av ON av.account_id = a.id
	` + whereClause
}

func scanAccountRecord(row rowScanner, record *AccountRecord) error {
	var isSystem int
	var allowsPostings int
	if err := row.Scan(
		&record.ID,
		&record.BookID,
		&isSystem,
		&record.SystemRole,
		&record.CreatedAt,
		&record.CreatedByUserID,
		&record.VersionID,
		&record.VersionSeq,
		&record.EffectiveFrom,
		&record.RecordedAt,
		&record.ChangedByUserID,
		&record.ChangeReason,
		&record.Status,
		&record.Code,
		&record.Name,
		&record.AccountClass,
		&record.AccountKind,
		&record.ParentAccountID,
		&record.InstitutionID,
		&record.CountryCode,
		&record.DefaultCommodityID,
		&record.QuantityScaleOverride,
		&allowsPostings,
		&record.NumberLast4,
		&record.ExternalRefHint,
		&record.CommentMarkdown,
		&record.MetadataJSON,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("scan account: %w", err)
	}

	record.IsSystem = isSystem == 1
	record.AllowsPostings = allowsPostings == 1
	return nil
}

func scanAccountRecords(rows *sql.Rows) ([]AccountRecord, error) {
	var records []AccountRecord
	for rows.Next() {
		var record AccountRecord
		if err := scanAccountRecord(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}

	return records, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableInt64Value(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}
