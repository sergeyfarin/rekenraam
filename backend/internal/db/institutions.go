package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type InstitutionRepository struct {
	database *sql.DB
}

type InstitutionRecord struct {
	ID              int64
	BookID          int64
	CreatedAt       string
	CreatedByUserID int64
	VersionID       int64
	VersionSeq      int64
	EffectiveFrom   string
	RecordedAt      string
	ChangedByUserID int64
	ChangeReason    string
	Status          string
	Name            string
	Kind            string
	CountryCode     sql.NullString
	Website         sql.NullString
	LogoURL         sql.NullString
	LogoSmallURL    sql.NullString
	BackdropURL     sql.NullString
	AddressJSON     string
	CommentMarkdown string
	MetadataJSON    string
}

type InstitutionSpec struct {
	Name            string
	Kind            string
	CountryCode     string
	Website         string
	LogoURL         string
	LogoSmallURL    string
	BackdropURL     string
	AddressJSON     string
	CommentMarkdown string
	MetadataJSON    string
}

type ListInstitutionsParams struct {
	BookID          int64
	Status          string
	IncludeArchived bool
	Query           string
	Limit           int
}

type CreateInstitutionParams struct {
	BookID          int64
	CreatedByUserID int64
	Spec            InstitutionSpec
	ChangeReason    string
	CreatedAt       string
	EffectiveFrom   string
}

type UpdateInstitutionParams struct {
	BookID          int64
	InstitutionID   int64
	ChangedByUserID int64
	Spec            InstitutionSpec
	Status          string
	ChangeReason    string
	RecordedAt      string
	EffectiveFrom   string
}

func NewInstitutionRepository(database *sql.DB) *InstitutionRepository {
	return &InstitutionRepository{database: database}
}

func (r *InstitutionRepository) ListInstitutions(ctx context.Context, params ListInstitutionsParams) ([]InstitutionRecord, error) {
	where := []string{"i.book_id = ?"}
	args := []any{params.BookID}

	if params.Status != "" {
		where = append(where, "iv.status = ?")
		args = append(args, params.Status)
	} else if !params.IncludeArchived {
		where = append(where, "iv.status != 'archived'")
	}

	query := strings.TrimSpace(params.Query)
	if query != "" {
		where = append(where, "(iv.name LIKE ? OR iv.kind LIKE ? OR iv.country_code LIKE ?)")
		like := "%" + query + "%"
		args = append(args, like, like, like)
	}

	args = append(args, params.Limit)

	rows, err := r.database.QueryContext(ctx, currentInstitutionSelect("AND "+strings.Join(where, " AND ")+`
		ORDER BY iv.status, iv.name COLLATE NOCASE, i.id
		LIMIT ?
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list institutions: %w", err)
	}
	defer rows.Close()

	return scanInstitutionRecords(rows)
}

func (r *InstitutionRepository) CurrentInstitutionByID(ctx context.Context, bookID int64, institutionID int64) (InstitutionRecord, error) {
	var record InstitutionRecord
	if err := scanInstitutionRecord(r.database.QueryRowContext(ctx, currentInstitutionSelect(`
		AND i.book_id = ? AND i.id = ?
	`), bookID, institutionID), &record); err != nil {
		return InstitutionRecord{}, err
	}

	return record, nil
}

func (r *InstitutionRepository) ListInstitutionVersions(ctx context.Context, bookID int64, institutionID int64) ([]InstitutionRecord, error) {
	rows, err := r.database.QueryContext(ctx, institutionSelect(`
		WHERE i.book_id = ? AND i.id = ?
		ORDER BY iv.version_seq DESC
	`), bookID, institutionID)
	if err != nil {
		return nil, fmt.Errorf("list institution versions: %w", err)
	}
	defer rows.Close()

	records, err := scanInstitutionRecords(rows)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, ErrNotFound
	}

	return records, nil
}

func (r *InstitutionRepository) CreateInstitution(ctx context.Context, params CreateInstitutionParams) (InstitutionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return InstitutionRecord{}, fmt.Errorf("begin create institution transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return InstitutionRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO institutions (book_id, created_at, created_by_user_id, created_request_id)
		VALUES (?, ?, ?, NULL)
	`, params.BookID, params.CreatedAt, params.CreatedByUserID)
	if err != nil {
		return InstitutionRecord{}, fmt.Errorf("insert institution: %w", err)
	}

	institutionID, err := result.LastInsertId()
	if err != nil {
		return InstitutionRecord{}, fmt.Errorf("read institution id: %w", err)
	}

	record, err := insertInstitutionVersion(ctx, tx, insertInstitutionVersionParams{
		BookID:          params.BookID,
		InstitutionID:   institutionID,
		CreatedAt:       params.CreatedAt,
		CreatedByUserID: params.CreatedByUserID,
		VersionSeq:      1,
		EffectiveFrom:   params.EffectiveFrom,
		RecordedAt:      params.CreatedAt,
		ChangedByUserID: params.CreatedByUserID,
		ChangeReason:    params.ChangeReason,
		Status:          "active",
		Spec:            params.Spec,
	})
	if err != nil {
		return InstitutionRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return InstitutionRecord{}, fmt.Errorf("commit create institution transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *InstitutionRepository) UpdateInstitution(ctx context.Context, params UpdateInstitutionParams) (InstitutionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return InstitutionRecord{}, fmt.Errorf("begin update institution transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	current, err := currentInstitutionByID(ctx, tx, params.BookID, params.InstitutionID)
	if err != nil {
		return InstitutionRecord{}, err
	}

	status := params.Status
	if status == "" {
		status = current.Status
	}

	changeReason := strings.TrimSpace(params.ChangeReason)
	if changeReason == "" {
		changeReason = "updated institution"
	}

	record, err := insertInstitutionVersion(ctx, tx, insertInstitutionVersionParams{
		BookID:          params.BookID,
		InstitutionID:   params.InstitutionID,
		CreatedAt:       current.CreatedAt,
		CreatedByUserID: current.CreatedByUserID,
		VersionSeq:      current.VersionSeq + 1,
		EffectiveFrom:   params.EffectiveFrom,
		RecordedAt:      params.RecordedAt,
		ChangedByUserID: params.ChangedByUserID,
		ChangeReason:    changeReason,
		Status:          status,
		Spec:            params.Spec,
	})
	if err != nil {
		return InstitutionRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return InstitutionRecord{}, fmt.Errorf("commit update institution transaction: %w", err)
	}
	committed = true

	return record, nil
}

type insertInstitutionVersionParams struct {
	BookID          int64
	InstitutionID   int64
	CreatedAt       string
	CreatedByUserID int64
	VersionSeq      int64
	EffectiveFrom   string
	RecordedAt      string
	ChangedByUserID int64
	ChangeReason    string
	Status          string
	Spec            InstitutionSpec
}

func insertInstitutionVersion(ctx context.Context, tx *sql.Tx, params insertInstitutionVersionParams) (InstitutionRecord, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO institution_versions (
			institution_id,
			version_seq,
			effective_from,
			recorded_at,
			changed_by_user_id,
			change_reason,
			status,
			name,
			kind,
			country_code,
			website,
			logo_url,
			logo_small_url,
			backdrop_url,
			address_json,
			comment_markdown,
			metadata_json
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?)
	`, params.InstitutionID, params.VersionSeq, params.EffectiveFrom, params.RecordedAt, params.ChangedByUserID, params.ChangeReason, params.Status, params.Spec.Name, params.Spec.Kind, params.Spec.CountryCode, params.Spec.Website, params.Spec.LogoURL, params.Spec.LogoSmallURL, params.Spec.BackdropURL, params.Spec.AddressJSON, params.Spec.CommentMarkdown, params.Spec.MetadataJSON)
	if err != nil {
		return InstitutionRecord{}, fmt.Errorf("insert institution version: %w", err)
	}

	versionID, err := result.LastInsertId()
	if err != nil {
		return InstitutionRecord{}, fmt.Errorf("read institution version id: %w", err)
	}

	record, err := currentInstitutionByID(ctx, tx, params.BookID, params.InstitutionID)
	if err != nil {
		return InstitutionRecord{}, err
	}
	record.VersionID = versionID

	return record, nil
}

func readBookForUpdate(ctx context.Context, tx *sql.Tx, bookID int64) (int64, error) {
	var id int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM books
		WHERE id = ?
	`, bookID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("read book: %w", err)
	}

	return id, nil
}

func currentInstitutionByID(ctx context.Context, tx *sql.Tx, bookID int64, institutionID int64) (InstitutionRecord, error) {
	var record InstitutionRecord
	if err := scanInstitutionRecord(tx.QueryRowContext(ctx, currentInstitutionSelect(`
		AND i.book_id = ? AND i.id = ?
	`), bookID, institutionID), &record); err != nil {
		return InstitutionRecord{}, err
	}

	return record, nil
}

func currentInstitutionSelect(whereClause string) string {
	return institutionSelect(`
		WHERE iv.version_seq = (
			SELECT MAX(latest.version_seq)
			FROM institution_versions latest
			WHERE latest.institution_id = i.id
		)
	` + whereClause)
}

func institutionSelect(whereClause string) string {
	return `
		SELECT
			i.id,
			i.book_id,
			i.created_at,
			i.created_by_user_id,
			iv.id,
			iv.version_seq,
			iv.effective_from,
			iv.recorded_at,
			iv.changed_by_user_id,
			iv.change_reason,
			iv.status,
			iv.name,
			iv.kind,
			iv.country_code,
			iv.website,
			iv.logo_url,
			iv.logo_small_url,
			iv.backdrop_url,
			iv.address_json,
			iv.comment_markdown,
			iv.metadata_json
		FROM institutions i
		JOIN institution_versions iv ON iv.institution_id = i.id
	` + whereClause
}

func scanInstitutionRecord(row rowScanner, record *InstitutionRecord) error {
	if err := row.Scan(
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
		&record.Kind,
		&record.CountryCode,
		&record.Website,
		&record.LogoURL,
		&record.LogoSmallURL,
		&record.BackdropURL,
		&record.AddressJSON,
		&record.CommentMarkdown,
		&record.MetadataJSON,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("scan institution: %w", err)
	}

	return nil
}

func scanInstitutionRecords(rows *sql.Rows) ([]InstitutionRecord, error) {
	var records []InstitutionRecord
	for rows.Next() {
		var record InstitutionRecord
		if err := scanInstitutionRecord(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate institutions: %w", err)
	}

	return records, nil
}
