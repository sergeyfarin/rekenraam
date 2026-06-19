package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrTagExists   = errors.New("tag already exists")
	ErrTagArchived = errors.New("archived tag cannot be updated")
)

type TagRepository struct {
	database *sql.DB
}

type TagRecord struct {
	ID              int64
	BookID          int64
	Name            string
	Kind            string
	Color           sql.NullString
	Icon            sql.NullString
	Status          string
	MetadataJSON    string
	CreatedAt       string
	CreatedByUserID int64
	UpdatedAt       string
	UpdatedByUserID int64
}

type TagSpec struct {
	Name         string
	Kind         string
	Color        string
	Icon         string
	MetadataJSON string
}

type ListTagsParams struct {
	BookID          int64
	Status          string
	Kind            string
	IncludeArchived bool
	Query           string
	Limit           int
}

type CreateTagParams struct {
	BookID        int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Spec          TagSpec
	CreatedAt     string
}

type UpdateTagParams struct {
	BookID        int64
	TagID         int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Spec          TagSpec
	UpdatedAt     string
}

type TagLifecycleParams struct {
	BookID        int64
	TagID         int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Status        string
	Reason        string
	UpdatedAt     string
}

func NewTagRepository(database *sql.DB) *TagRepository {
	return &TagRepository{database: database}
}

func (r *TagRepository) ListTags(ctx context.Context, params ListTagsParams) ([]TagRecord, error) {
	where := []string{"book_id = ?"}
	args := []any{params.BookID}

	if params.Status != "" {
		where = append(where, "status = ?")
		args = append(args, params.Status)
	} else if !params.IncludeArchived {
		where = append(where, "status != 'archived'")
	}

	if params.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, params.Kind)
	}

	query := strings.TrimSpace(params.Query)
	if query != "" {
		where = append(where, "(name LIKE ? OR kind LIKE ?)")
		like := "%" + query + "%"
		args = append(args, like, like)
	}

	args = append(args, params.Limit)

	rows, err := r.database.QueryContext(ctx, tagSelect(`
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY
			CASE kind
				WHEN 'project' THEN 1
				WHEN 'person' THEN 2
				WHEN 'flag' THEN 3
				WHEN 'place' THEN 4
				ELSE 5
			END,
			name COLLATE NOCASE,
			id
		LIMIT ?
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	return scanTagRecords(rows)
}

func (r *TagRepository) TagByID(ctx context.Context, bookID int64, tagID int64) (TagRecord, error) {
	var record TagRecord
	if err := scanTagRecord(r.database.QueryRowContext(ctx, tagSelect(`
		WHERE book_id = ? AND id = ?
	`), bookID, tagID), &record); err != nil {
		return TagRecord{}, err
	}

	return record, nil
}

func (r *TagRepository) CreateTag(ctx context.Context, params CreateTagParams) (TagRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TagRecord{}, fmt.Errorf("begin create tag transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return TagRecord{}, err
	}
	if err := ensureActiveTagNameAvailable(ctx, tx, params.BookID, params.Spec.Kind, params.Spec.Name, 0); err != nil {
		return TagRecord{}, err
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        "created tag",
	})
	if err != nil {
		return TagRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO tags (
			book_id,
			name,
			kind,
			color,
			icon,
			status,
			metadata_json,
			created_at,
			created_by_user_id,
			updated_at,
			updated_by_user_id,
			created_request_id,
			created_audit_event_id,
			updated_audit_event_id
		)
		VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), 'active', ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?)
	`, params.BookID, params.Spec.Name, params.Spec.Kind, params.Spec.Color, params.Spec.Icon, params.Spec.MetadataJSON, params.CreatedAt, params.ActorUserID, params.CreatedAt, params.ActorUserID, params.RequestID, auditEventID, auditEventID)
	if err != nil {
		return TagRecord{}, fmt.Errorf("insert tag: %w", err)
	}

	tagID, err := result.LastInsertId()
	if err != nil {
		return TagRecord{}, fmt.Errorf("read tag id: %w", err)
	}

	record, err := tagByID(ctx, tx, params.BookID, tagID)
	if err != nil {
		return TagRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return TagRecord{}, fmt.Errorf("commit create tag transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *TagRepository) UpdateTag(ctx context.Context, params UpdateTagParams) (TagRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TagRecord{}, fmt.Errorf("begin update tag transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return TagRecord{}, err
	}

	current, err := tagByID(ctx, tx, params.BookID, params.TagID)
	if err != nil {
		return TagRecord{}, err
	}
	if current.Status == "archived" {
		return TagRecord{}, ErrTagArchived
	}
	if err := ensureActiveTagNameAvailable(ctx, tx, params.BookID, params.Spec.Kind, params.Spec.Name, params.TagID); err != nil {
		return TagRecord{}, err
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.UpdatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        "updated tag",
	})
	if err != nil {
		return TagRecord{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE tags
		SET name = ?,
			kind = ?,
			color = NULLIF(?, ''),
			icon = NULLIF(?, ''),
			metadata_json = ?,
			updated_at = ?,
			updated_by_user_id = ?,
			updated_audit_event_id = ?
		WHERE book_id = ? AND id = ?
	`, params.Spec.Name, params.Spec.Kind, params.Spec.Color, params.Spec.Icon, params.Spec.MetadataJSON, params.UpdatedAt, params.ActorUserID, auditEventID, params.BookID, params.TagID); err != nil {
		return TagRecord{}, fmt.Errorf("update tag: %w", err)
	}

	record, err := tagByID(ctx, tx, params.BookID, params.TagID)
	if err != nil {
		return TagRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return TagRecord{}, fmt.Errorf("commit update tag transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *TagRepository) ChangeTagStatus(ctx context.Context, params TagLifecycleParams) (TagRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TagRecord{}, fmt.Errorf("begin tag lifecycle transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return TagRecord{}, err
	}

	current, err := tagByID(ctx, tx, params.BookID, params.TagID)
	if err != nil {
		return TagRecord{}, err
	}
	if current.Status == params.Status {
		return current, nil
	}
	if params.Status == "active" {
		if err := ensureActiveTagNameAvailable(ctx, tx, params.BookID, current.Kind, current.Name, params.TagID); err != nil {
			return TagRecord{}, err
		}
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.UpdatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.Reason,
	})
	if err != nil {
		return TagRecord{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE tags
		SET status = ?,
			updated_at = ?,
			updated_by_user_id = ?,
			updated_audit_event_id = ?
		WHERE book_id = ? AND id = ?
	`, params.Status, params.UpdatedAt, params.ActorUserID, auditEventID, params.BookID, params.TagID); err != nil {
		return TagRecord{}, fmt.Errorf("update tag status: %w", err)
	}

	record, err := tagByID(ctx, tx, params.BookID, params.TagID)
	if err != nil {
		return TagRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return TagRecord{}, fmt.Errorf("commit tag lifecycle transaction: %w", err)
	}
	committed = true

	return record, nil
}

func tagByID(ctx context.Context, tx *sql.Tx, bookID int64, tagID int64) (TagRecord, error) {
	var record TagRecord
	if err := scanTagRecord(tx.QueryRowContext(ctx, tagSelect(`
		WHERE book_id = ? AND id = ?
	`), bookID, tagID), &record); err != nil {
		return TagRecord{}, err
	}

	return record, nil
}

func ensureActiveTagNameAvailable(ctx context.Context, tx *sql.Tx, bookID int64, kind string, name string, exceptTagID int64) error {
	var existingID int64
	err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM tags
		WHERE book_id = ?
			AND kind = ?
			AND name = ? COLLATE NOCASE
			AND status = 'active'
			AND id != ?
		LIMIT 1
	`, bookID, kind, name, exceptTagID).Scan(&existingID)
	if err == nil {
		return ErrTagExists
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return fmt.Errorf("check tag uniqueness: %w", err)
}

func tagSelect(whereClause string) string {
	return `
		SELECT
			id,
			book_id,
			name,
			kind,
			color,
			icon,
			status,
			metadata_json,
			created_at,
			created_by_user_id,
			updated_at,
			updated_by_user_id
		FROM tags
	` + whereClause
}

func scanTagRecord(row rowScanner, record *TagRecord) error {
	if err := row.Scan(
		&record.ID,
		&record.BookID,
		&record.Name,
		&record.Kind,
		&record.Color,
		&record.Icon,
		&record.Status,
		&record.MetadataJSON,
		&record.CreatedAt,
		&record.CreatedByUserID,
		&record.UpdatedAt,
		&record.UpdatedByUserID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("scan tag: %w", err)
	}

	return nil
}

func scanTagRecords(rows *sql.Rows) ([]TagRecord, error) {
	var records []TagRecord
	for rows.Next() {
		var record TagRecord
		if err := scanTagRecord(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tags: %w", err)
	}

	return records, nil
}
