package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var ErrPayeeExists = errors.New("payee already exists")

type PayeeRepository struct {
	database *sql.DB
}

type PayeeRecord struct {
	ID                       int64
	BookID                   int64
	CreatedAt                string
	CreatedByUserID          int64
	VersionID                int64
	VersionSeq               int64
	EffectiveFrom            string
	RecordedAt               string
	ChangedByUserID          int64
	ChangeReason             string
	Status                   string
	Name                     string
	NormalizedName           string
	DefaultAccountID         sql.NullInt64
	DefaultCategoryAccountID sql.NullInt64
	MetadataJSON             string
}

type PayeeSpec struct {
	Name                     string
	NormalizedName           string
	DefaultAccountID         sql.NullInt64
	DefaultCategoryAccountID sql.NullInt64
	MetadataJSON             string
}

type ListPayeesParams struct {
	BookID          int64
	Status          string
	IncludeArchived bool
	Query           string
	Limit           int
}

type CreatePayeeParams struct {
	BookID        int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Spec          PayeeSpec
	CreatedAt     string
	EffectiveFrom string
	ChangeReason  string
}

type UpdatePayeeParams struct {
	BookID        int64
	PayeeID       int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Spec          PayeeSpec
	Status        string
	RecordedAt    string
	EffectiveFrom string
	ChangeReason  string
}

func NewPayeeRepository(database *sql.DB) *PayeeRepository {
	return &PayeeRepository{database: database}
}

func (r *PayeeRepository) ListPayees(ctx context.Context, params ListPayeesParams) ([]PayeeRecord, error) {
	where := []string{"p.book_id = ?"}
	args := []any{params.BookID}

	if params.Status != "" {
		where = append(where, "pv.status = ?")
		args = append(args, params.Status)
	} else if !params.IncludeArchived {
		where = append(where, "pv.status != 'archived'")
	}

	query := strings.TrimSpace(params.Query)
	if query != "" {
		where = append(where, "(pv.name LIKE ? OR pv.normalized_name LIKE ?)")
		like := "%" + query + "%"
		args = append(args, like, like)
	}

	args = append(args, params.Limit)

	rows, err := r.database.QueryContext(ctx, currentPayeeSelect("AND "+strings.Join(where, " AND ")+`
		ORDER BY pv.name COLLATE NOCASE, p.id
		LIMIT ?
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list payees: %w", err)
	}
	defer rows.Close()

	return scanPayeeRecords(rows)
}

func (r *PayeeRepository) PayeeByID(ctx context.Context, bookID int64, payeeID int64) (PayeeRecord, error) {
	var record PayeeRecord
	if err := scanPayeeRecord(r.database.QueryRowContext(ctx, currentPayeeSelect(`
		AND p.book_id = ? AND p.id = ?
	`), bookID, payeeID), &record); err != nil {
		return PayeeRecord{}, err
	}

	return record, nil
}

func (r *PayeeRepository) CreatePayee(ctx context.Context, params CreatePayeeParams) (PayeeRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return PayeeRecord{}, fmt.Errorf("begin create payee transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return PayeeRecord{}, err
	}
	if err := ensurePayeeNameAvailable(ctx, tx, params.BookID, params.Spec.NormalizedName, 0); err != nil {
		return PayeeRecord{}, err
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
		return PayeeRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO payees (book_id, created_at, created_by_user_id, created_request_id, created_audit_event_id)
		VALUES (?, ?, ?, NULLIF(?, ''), ?)
	`, params.BookID, params.CreatedAt, params.ActorUserID, params.RequestID, auditEventID)
	if err != nil {
		return PayeeRecord{}, fmt.Errorf("insert payee: %w", err)
	}

	payeeID, err := result.LastInsertId()
	if err != nil {
		return PayeeRecord{}, fmt.Errorf("read payee id: %w", err)
	}

	record, err := insertPayeeVersion(ctx, tx, insertPayeeVersionParams{
		PayeeID:            payeeID,
		VersionSeq:         1,
		EffectiveFrom:      params.EffectiveFrom,
		RecordedAt:         params.CreatedAt,
		ChangedByUserID:    params.ActorUserID,
		ChangeReason:       params.ChangeReason,
		ChangeAuditEventID: auditEventID,
		Status:             "active",
		Spec:               params.Spec,
	})
	if err != nil {
		return PayeeRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return PayeeRecord{}, fmt.Errorf("commit create payee transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *PayeeRepository) UpdatePayee(ctx context.Context, params UpdatePayeeParams) (PayeeRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return PayeeRecord{}, fmt.Errorf("begin update payee transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return PayeeRecord{}, err
	}

	current, err := payeeByID(ctx, tx, params.BookID, params.PayeeID)
	if err != nil {
		return PayeeRecord{}, err
	}

	status := params.Status
	if status == "" {
		status = current.Status
	}
	if status == "active" {
		if err := ensurePayeeNameAvailable(ctx, tx, params.BookID, params.Spec.NormalizedName, params.PayeeID); err != nil {
			return PayeeRecord{}, err
		}
	}

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
		return PayeeRecord{}, err
	}

	record, err := insertPayeeVersion(ctx, tx, insertPayeeVersionParams{
		PayeeID:            params.PayeeID,
		VersionSeq:         current.VersionSeq + 1,
		EffectiveFrom:      params.EffectiveFrom,
		RecordedAt:         params.RecordedAt,
		ChangedByUserID:    params.ActorUserID,
		ChangeReason:       params.ChangeReason,
		ChangeAuditEventID: auditEventID,
		Status:             status,
		Spec:               params.Spec,
	})
	if err != nil {
		return PayeeRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return PayeeRecord{}, fmt.Errorf("commit update payee transaction: %w", err)
	}
	committed = true

	return record, nil
}

type insertPayeeVersionParams struct {
	PayeeID            int64
	VersionSeq         int64
	EffectiveFrom      string
	RecordedAt         string
	ChangedByUserID    int64
	ChangeReason       string
	ChangeAuditEventID int64
	Status             string
	Spec               PayeeSpec
}

func insertPayeeVersion(ctx context.Context, tx *sql.Tx, params insertPayeeVersionParams) (PayeeRecord, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO payee_versions (
			payee_id,
			version_seq,
			effective_from,
			recorded_at,
			changed_by_user_id,
			change_reason,
			status,
			name,
			normalized_name,
			default_account_id,
			default_category_account_id,
			metadata_json,
			change_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, params.PayeeID, params.VersionSeq, params.EffectiveFrom, params.RecordedAt, params.ChangedByUserID, params.ChangeReason, params.Status, params.Spec.Name, params.Spec.NormalizedName, nullableInt64Value(params.Spec.DefaultAccountID), nullableInt64Value(params.Spec.DefaultCategoryAccountID), params.Spec.MetadataJSON, params.ChangeAuditEventID)
	if err != nil {
		return PayeeRecord{}, fmt.Errorf("insert payee version: %w", err)
	}

	versionID, err := result.LastInsertId()
	if err != nil {
		return PayeeRecord{}, fmt.Errorf("read payee version id: %w", err)
	}

	var record PayeeRecord
	if err := scanPayeeRecord(tx.QueryRowContext(ctx, payeeSelect("payee_versions", `
		WHERE pv.id = ?
	`), versionID), &record); err != nil {
		return PayeeRecord{}, err
	}

	return record, nil
}

func ensurePayeeNameAvailable(ctx context.Context, tx *sql.Tx, bookID int64, normalizedName string, excludePayeeID int64) error {
	var existingID int64
	err := tx.QueryRowContext(ctx, `
		SELECT p.id
		FROM payees p
		JOIN current_payee_versions pv ON pv.payee_id = p.id
		WHERE p.book_id = ?
			AND pv.status = 'active'
			AND pv.normalized_name = ?
			AND p.id <> ?
		LIMIT 1
	`, bookID, normalizedName, excludePayeeID).Scan(&existingID)
	if err == nil {
		return ErrPayeeExists
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}

	return fmt.Errorf("check payee name availability: %w", err)
}

func payeeByID(ctx context.Context, tx *sql.Tx, bookID int64, payeeID int64) (PayeeRecord, error) {
	var record PayeeRecord
	if err := scanPayeeRecord(tx.QueryRowContext(ctx, currentPayeeSelect(`
		AND p.book_id = ? AND p.id = ?
	`), bookID, payeeID), &record); err != nil {
		return PayeeRecord{}, err
	}

	return record, nil
}

func currentPayeeSelect(extraConditions string) string {
	return payeeSelect("current_payee_versions", `
		WHERE 1 = 1
	`+extraConditions)
}

func payeeSelect(versionTable string, extraConditions string) string {
	return `
		SELECT
			p.id,
			p.book_id,
			p.created_at,
			p.created_by_user_id,
			pv.id,
			pv.version_seq,
			pv.effective_from,
			pv.recorded_at,
			pv.changed_by_user_id,
			pv.change_reason,
			pv.status,
			pv.name,
			pv.normalized_name,
			pv.default_account_id,
			pv.default_category_account_id,
			pv.metadata_json
		FROM payees p
		JOIN ` + versionTable + ` pv ON pv.payee_id = p.id
	` + extraConditions
}

func scanPayeeRecords(rows *sql.Rows) ([]PayeeRecord, error) {
	var records []PayeeRecord
	for rows.Next() {
		var record PayeeRecord
		if err := scanPayeeRecord(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate payees: %w", err)
	}

	return records, nil
}

func scanPayeeRecord(scanner interface{ Scan(dest ...any) error }, record *PayeeRecord) error {
	if err := scanner.Scan(
		&record.ID,
		&record.BookID,
		&record.CreatedAt,
		&record.CreatedByUserID,
		&record.VersionID,
		&record.VersionSeq,
		&record.EffectiveFrom,
		&record.RecordedAt,
		&record.ChangedByUserID,
		&record.ChangeReason,
		&record.Status,
		&record.Name,
		&record.NormalizedName,
		&record.DefaultAccountID,
		&record.DefaultCategoryAccountID,
		&record.MetadataJSON,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("scan payee: %w", err)
	}

	return nil
}
