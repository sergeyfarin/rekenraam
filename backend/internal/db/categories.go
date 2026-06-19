package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrCategoriesSetupComplete     = errors.New("categories setup already complete")
	ErrCategoryExists              = errors.New("category already exists")
	ErrCategoryHasChildren         = errors.New("category has children")
	ErrCategoryBuiltIn             = errors.New("built-in category cannot be deleted")
	ErrCategoryUsed                = errors.New("category has financial references")
	ErrCategoryDeleteNotPermitted  = errors.New("category cannot be deleted")
	ErrCategoryParentHasReferences = errors.New("category parent has references")
)

type CategoryRepository struct {
	database *sql.DB
}

type CategorySeedSpec struct {
	Code           string
	CategoryType   string
	BuiltinKey     string
	ParentCode     string
	AllowsPostings bool
	MetadataJSON   string
}

type ListCategoriesParams struct {
	BookID          int64
	Status          string
	CategoryType    string
	IncludeArchived bool
	Query           string
	Limit           int
}

type CreateCategoryParams struct {
	BookID          int64
	CreatedByUserID int64
	AuthSessionID   int64
	RequestID       string
	OriginType      string
	Operation       string
	Spec            AccountSpec
	ChangeReason    string
	CreatedAt       string
	EffectiveFrom   string
}

type UpdateCategoryParams struct {
	BookID          int64
	CategoryID      int64
	ChangedByUserID int64
	AuthSessionID   int64
	RequestID       string
	OriginType      string
	Operation       string
	Spec            AccountSpec
	Status          string
	ChangeReason    string
	RecordedAt      string
	EffectiveFrom   string
}

type CategoryLifecycleParams struct {
	BookID          int64
	CategoryID      int64
	ChangedByUserID int64
	AuthSessionID   int64
	RequestID       string
	OriginType      string
	Operation       string
	ClosedOn        sql.NullString
	Status          string
	Reason          string
	RecordedAt      string
	EffectiveFrom   string
}

type DeleteCategoryParams struct {
	BookID        int64
	CategoryID    int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	OccurredAt    string
}

type EnsureCategoriesParams struct {
	BookID          int64
	ChangedByUserID int64
	AuthSessionID   int64
	RequestID       string
	OriginType      string
	Operation       string
	Specs           []CategorySeedSpec
	CreatedAt       string
	EffectiveFrom   string
}

type EnsureCategoriesRecord struct {
	Categories []AccountRecord
}

func NewCategoryRepository(database *sql.DB) *CategoryRepository {
	return &CategoryRepository{database: database}
}

func (r *CategoryRepository) ListCategories(ctx context.Context, params ListCategoriesParams) ([]AccountRecord, error) {
	where := []string{"a.book_id = ?", "a.system_role IS NULL", categoryAccountPredicate("av")}
	args := []any{params.BookID}

	if params.Status != "" {
		where = append(where, "av.status = ?")
		args = append(args, params.Status)
	} else if !params.IncludeArchived {
		where = append(where, "av.status != 'archived'")
	}

	if params.CategoryType != "" {
		where = append(where, "av.account_class = ?")
		args = append(args, params.CategoryType)
	}

	query := strings.TrimSpace(params.Query)
	if query != "" {
		where = append(where, "(av.name LIKE ? OR av.code LIKE ? OR json_extract(av.metadata_json, '$.category.builtin_key') LIKE ?)")
		like := "%" + query + "%"
		args = append(args, like, like, like)
	}

	args = append(args, params.Limit)

	rows, err := r.database.QueryContext(ctx, currentAccountSelect("AND "+strings.Join(where, " AND ")+`
		ORDER BY
			CASE av.account_class WHEN 'income' THEN 1 WHEN 'expense' THEN 2 ELSE 9 END,
			COALESCE(av.parent_account_id, 0),
			COALESCE(av.name, av.code, json_extract(av.metadata_json, '$.category.builtin_key'), ''),
			a.id
		LIMIT ?
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	return scanAccountRecords(rows)
}

func (r *CategoryRepository) CategoryByID(ctx context.Context, bookID int64, categoryID int64) (AccountRecord, error) {
	var record AccountRecord
	if err := scanAccountRecord(r.database.QueryRowContext(ctx, currentAccountSelect(`
		AND a.book_id = ?
		AND a.id = ?
		AND a.system_role IS NULL
		AND `+categoryAccountPredicate("av")+`
	`), bookID, categoryID), &record); err != nil {
		return AccountRecord{}, err
	}

	return record, nil
}

func (r *CategoryRepository) WouldCreateCycle(ctx context.Context, bookID int64, categoryID int64, parentCategoryID int64) (bool, error) {
	if categoryID <= 0 || parentCategoryID <= 0 {
		return false, nil
	}

	rows, err := r.database.QueryContext(ctx, `
		WITH RECURSIVE ancestors(id, parent_id) AS (
			SELECT a.id, av.parent_account_id
			FROM accounts a
			JOIN current_account_versions av ON av.account_id = a.id
			WHERE a.book_id = ?
				AND a.id = ?
				AND `+categoryAccountPredicate("av")+`
			UNION ALL
			SELECT parent.id, parent_version.parent_account_id
			FROM accounts parent
			JOIN current_account_versions parent_version ON parent_version.account_id = parent.id
			JOIN ancestors ON ancestors.parent_id = parent.id
			WHERE parent.book_id = ?
				AND `+categoryAccountPredicate("parent_version")+`
		)
		SELECT id
		FROM ancestors
	`, bookID, parentCategoryID, bookID)
	if err != nil {
		return false, fmt.Errorf("read category ancestors: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ancestorID int64
		if err := rows.Scan(&ancestorID); err != nil {
			return false, fmt.Errorf("scan category ancestor: %w", err)
		}
		if ancestorID == categoryID {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate category ancestors: %w", err)
	}

	return false, nil
}

func (r *CategoryRepository) CreateCategory(ctx context.Context, params CreateCategoryParams) (AccountRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("begin create category transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return AccountRecord{}, err
	}
	if params.Spec.Name != "" {
		if err := ensureActiveCategoryNameAvailable(ctx, tx, params.BookID, params.Spec.AccountClass, params.Spec.ParentAccountID, params.Spec.Name, 0); err != nil {
			return AccountRecord{}, err
		}
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
		return AccountRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO accounts (book_id, system_role, created_at, created_by_user_id, created_request_id, created_audit_event_id)
		VALUES (?, NULL, ?, ?, NULLIF(?, ''), ?)
	`, params.BookID, params.CreatedAt, params.CreatedByUserID, params.RequestID, auditEventID)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("insert category account: %w", err)
	}

	categoryID, err := result.LastInsertId()
	if err != nil {
		return AccountRecord{}, fmt.Errorf("read category account id: %w", err)
	}

	record, err := insertAccountVersion(ctx, tx, insertAccountVersionParams{
		BookID:             params.BookID,
		AccountID:          categoryID,
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
		return AccountRecord{}, fmt.Errorf("commit create category transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *CategoryRepository) UpdateCategory(ctx context.Context, params UpdateCategoryParams) (AccountRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("begin update category transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return AccountRecord{}, err
	}

	current, err := currentCategoryByID(ctx, tx, params.BookID, params.CategoryID)
	if err != nil {
		return AccountRecord{}, err
	}
	status := params.Status
	if status == "" {
		status = current.Status
	}
	if status == "active" && params.Spec.Name != "" {
		if err := ensureActiveCategoryNameAvailable(ctx, tx, params.BookID, params.Spec.AccountClass, params.Spec.ParentAccountID, params.Spec.Name, params.CategoryID); err != nil {
			return AccountRecord{}, err
		}
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
		return AccountRecord{}, err
	}

	record, err := insertAccountVersion(ctx, tx, insertAccountVersionParams{
		BookID:             params.BookID,
		AccountID:          params.CategoryID,
		CreatedAt:          current.CreatedAt,
		CreatedByUserID:    current.CreatedByUserID,
		VersionSeq:         current.VersionSeq + 1,
		EffectiveFrom:      params.EffectiveFrom,
		RecordedAt:         params.RecordedAt,
		ChangedByUserID:    params.ChangedByUserID,
		ChangeReason:       params.ChangeReason,
		ChangeAuditEventID: auditEventID,
		Status:             status,
		Spec:               params.Spec,
	})
	if err != nil {
		return AccountRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return AccountRecord{}, fmt.Errorf("commit update category transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *CategoryRepository) ChangeCategoryStatus(ctx context.Context, params CategoryLifecycleParams) (AccountRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("begin change category status transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return AccountRecord{}, err
	}

	current, err := currentCategoryByID(ctx, tx, params.BookID, params.CategoryID)
	if err != nil {
		return AccountRecord{}, err
	}
	if current.Status == params.Status {
		return current, nil
	}
	if params.Status == "archived" {
		hasChildren, err := categoryHasActiveChildren(ctx, tx, params.BookID, params.CategoryID)
		if err != nil {
			return AccountRecord{}, err
		}
		if hasChildren {
			return AccountRecord{}, ErrCategoryHasChildren
		}
	}
	if params.Status == "active" && current.Name.Valid {
		if err := ensureActiveCategoryNameAvailable(ctx, tx, params.BookID, current.AccountClass, current.ParentAccountID, current.Name.String, params.CategoryID); err != nil {
			return AccountRecord{}, err
		}
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ChangedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.RecordedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.Reason,
	})
	if err != nil {
		return AccountRecord{}, err
	}

	spec := accountSpecFromRecord(current)
	spec.ClosedOn = params.ClosedOn
	record, err := insertAccountVersion(ctx, tx, insertAccountVersionParams{
		BookID:             params.BookID,
		AccountID:          params.CategoryID,
		CreatedAt:          current.CreatedAt,
		CreatedByUserID:    current.CreatedByUserID,
		VersionSeq:         current.VersionSeq + 1,
		EffectiveFrom:      params.EffectiveFrom,
		RecordedAt:         params.RecordedAt,
		ChangedByUserID:    params.ChangedByUserID,
		ChangeReason:       params.Reason,
		ChangeAuditEventID: auditEventID,
		Status:             params.Status,
		Spec:               spec,
	})
	if err != nil {
		return AccountRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return AccountRecord{}, fmt.Errorf("commit change category status transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *CategoryRepository) DeleteUnusedCategory(ctx context.Context, params DeleteCategoryParams) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete category transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return err
	}

	current, err := currentCategoryByID(ctx, tx, params.BookID, params.CategoryID)
	if err != nil {
		return err
	}
	if jsonExtractBool(current.MetadataJSON, "$.category.is_builtin") {
		return ErrCategoryBuiltIn
	}
	anyChildren, err := categoryHasAnyChildren(ctx, tx, params.BookID, params.CategoryID)
	if err != nil {
		return err
	}
	if anyChildren {
		return ErrCategoryHasChildren
	}
	used, err := categoryHasFinancialReferences(ctx, tx, params.BookID, params.CategoryID)
	if err != nil {
		return err
	}
	if used {
		return ErrCategoryUsed
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.OccurredAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        "deleted unused category",
	})
	if err != nil {
		return err
	}

	versions, err := tx.ExecContext(ctx, `
		DELETE FROM account_versions
		WHERE account_id = ?
	`, params.CategoryID)
	if err != nil {
		return fmt.Errorf("delete category versions: %w", err)
	}
	versionRows, err := versions.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted category versions: %w", err)
	}
	if versionRows == 0 {
		return ErrNotFound
	}

	result, err := tx.ExecContext(ctx, `
		DELETE FROM accounts
		WHERE id = ?
			AND book_id = ?
			AND system_role IS NULL
	`, params.CategoryID, params.BookID)
	if err != nil {
		return fmt.Errorf("delete category account: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted category account rows: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("%w: deleted %d account rows", ErrCategoryDeleteNotPermitted, rowsAffected)
	}

	_ = auditEventID
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete category transaction: %w", err)
	}
	committed = true

	return nil
}

func (r *CategoryRepository) EnsureCategories(ctx context.Context, params EnsureCategoriesParams) (EnsureCategoriesRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return EnsureCategoriesRecord{}, fmt.Errorf("begin categories setup transaction: %w", err)
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
		WHERE step_key = 'categories'
	`).Scan(&completedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EnsureCategoriesRecord{}, ErrNotFound
		}
		return EnsureCategoriesRecord{}, fmt.Errorf("read categories setup step: %w", err)
	}
	if completedAt.Valid {
		return EnsureCategoriesRecord{}, ErrCategoriesSetupComplete
	}

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return EnsureCategoriesRecord{}, err
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ChangedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        "seeded categories",
	})
	if err != nil {
		return EnsureCategoriesRecord{}, err
	}

	records := make([]AccountRecord, 0, len(params.Specs))
	idsByCode := map[string]int64{}
	for _, spec := range params.Specs {
		parentID := sql.NullInt64{}
		if spec.ParentCode != "" {
			id, ok := idsByCode[spec.ParentCode]
			if !ok {
				return EnsureCategoriesRecord{}, fmt.Errorf("category seed parent %q must be seeded first", spec.ParentCode)
			}
			parentID = sql.NullInt64{Int64: id, Valid: true}
		}

		record, err := ensureBuiltinCategory(ctx, tx, params, spec, parentID, auditEventID)
		if err != nil {
			return EnsureCategoriesRecord{}, err
		}
		idsByCode[spec.Code] = record.ID
		records = append(records, record)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE setup_steps
		SET completed_at = ?, completed_audit_event_id = ?
		WHERE step_key = 'categories' AND completed_at IS NULL
	`, params.CreatedAt, auditEventID)
	if err != nil {
		return EnsureCategoriesRecord{}, fmt.Errorf("mark categories setup complete: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return EnsureCategoriesRecord{}, fmt.Errorf("read categories setup rows affected: %w", err)
	}
	if rowsAffected != 1 {
		return EnsureCategoriesRecord{}, fmt.Errorf("mark categories setup complete: updated %d rows", rowsAffected)
	}

	if err := tx.Commit(); err != nil {
		return EnsureCategoriesRecord{}, fmt.Errorf("commit categories setup transaction: %w", err)
	}
	committed = true

	return EnsureCategoriesRecord{Categories: records}, nil
}

func ensureBuiltinCategory(ctx context.Context, tx *sql.Tx, params EnsureCategoriesParams, spec CategorySeedSpec, parentID sql.NullInt64, auditEventID int64) (AccountRecord, error) {
	record, err := currentCategoryByCode(ctx, tx, params.BookID, spec.Code)
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return AccountRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO accounts (book_id, system_role, created_at, created_by_user_id, created_request_id, created_audit_event_id)
		VALUES (?, NULL, ?, ?, NULLIF(?, ''), ?)
	`, params.BookID, params.CreatedAt, params.ChangedByUserID, params.RequestID, auditEventID)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("insert built-in category account: %w", err)
	}
	categoryID, err := result.LastInsertId()
	if err != nil {
		return AccountRecord{}, fmt.Errorf("read built-in category account id: %w", err)
	}

	return insertAccountVersion(ctx, tx, insertAccountVersionParams{
		BookID:             params.BookID,
		AccountID:          categoryID,
		CreatedAt:          params.CreatedAt,
		CreatedByUserID:    params.ChangedByUserID,
		VersionSeq:         1,
		EffectiveFrom:      params.EffectiveFrom,
		RecordedAt:         params.CreatedAt,
		ChangedByUserID:    params.ChangedByUserID,
		ChangeReason:       "seeded category",
		ChangeAuditEventID: auditEventID,
		Status:             "active",
		Spec: AccountSpec{
			Code:            spec.Code,
			AccountClass:    spec.CategoryType,
			AccountKind:     spec.CategoryType,
			ParentAccountID: parentID,
			AllowsPostings:  spec.AllowsPostings,
			MetadataJSON:    spec.MetadataJSON,
			OpenedOn:        params.EffectiveFrom,
		},
	})
}

func currentCategoryByID(ctx context.Context, tx *sql.Tx, bookID int64, categoryID int64) (AccountRecord, error) {
	var record AccountRecord
	if err := scanAccountRecord(tx.QueryRowContext(ctx, currentAccountSelect(`
		AND a.book_id = ?
		AND a.id = ?
		AND a.system_role IS NULL
		AND `+categoryAccountPredicate("av")+`
	`), bookID, categoryID), &record); err != nil {
		return AccountRecord{}, err
	}

	return record, nil
}

func currentCategoryByCode(ctx context.Context, tx *sql.Tx, bookID int64, code string) (AccountRecord, error) {
	var record AccountRecord
	if err := scanAccountRecord(tx.QueryRowContext(ctx, currentAccountSelect(`
		AND a.book_id = ?
		AND av.code = ?
		AND a.system_role IS NULL
		AND `+categoryAccountPredicate("av")+`
	`), bookID, code), &record); err != nil {
		return AccountRecord{}, err
	}

	return record, nil
}

func ensureActiveCategoryNameAvailable(ctx context.Context, tx *sql.Tx, bookID int64, categoryType string, parentID sql.NullInt64, name string, exceptCategoryID int64) error {
	query := `
		SELECT a.id
		FROM accounts a
		JOIN current_account_versions av ON av.account_id = a.id
		WHERE a.book_id = ?
			AND a.id != ?
			AND a.system_role IS NULL
			AND av.status = 'active'
			AND av.account_class = ?
			AND ` + categoryAccountPredicate("av") + `
			AND lower(av.name) = lower(?)
	`
	args := []any{bookID, exceptCategoryID, categoryType, name}
	if parentID.Valid {
		query += " AND av.parent_account_id = ?"
		args = append(args, parentID.Int64)
	} else {
		query += " AND av.parent_account_id IS NULL"
	}
	query += " LIMIT 1"

	var existingID int64
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&existingID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("check category name availability: %w", err)
	}

	return ErrCategoryExists
}

func categoryHasActiveChildren(ctx context.Context, tx *sql.Tx, bookID int64, categoryID int64) (bool, error) {
	var childID int64
	err := tx.QueryRowContext(ctx, `
		SELECT child.id
		FROM accounts child
		JOIN current_account_versions child_av ON child_av.account_id = child.id
		WHERE child.book_id = ?
			AND child_av.parent_account_id = ?
			AND child.system_role IS NULL
			AND child_av.status = 'active'
			AND `+categoryAccountPredicate("child_av")+`
		LIMIT 1
	`, bookID, categoryID).Scan(&childID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("check category active children: %w", err)
	}
	return true, nil
}

func categoryHasAnyChildren(ctx context.Context, tx *sql.Tx, bookID int64, categoryID int64) (bool, error) {
	var childID int64
	err := tx.QueryRowContext(ctx, `
		SELECT child.id
		FROM accounts child
		JOIN account_versions child_av ON child_av.account_id = child.id
		WHERE child.book_id = ?
			AND child_av.parent_account_id = ?
			AND child.system_role IS NULL
			AND `+categoryAccountPredicate("child_av")+`
		LIMIT 1
	`, bookID, categoryID).Scan(&childID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("check category children: %w", err)
	}
	return true, nil
}

func categoryHasFinancialReferences(ctx context.Context, tx *sql.Tx, bookID int64, categoryID int64) (bool, error) {
	var referenceCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(1) FROM posting_versions WHERE book_id = ? AND account_id = ?)
			+ (SELECT COUNT(1) FROM payee_versions pv JOIN payees p ON p.id = pv.payee_id WHERE p.book_id = ? AND pv.default_category_account_id = ?)
			+ (SELECT COUNT(1) FROM reconciliation_session_postings WHERE book_id = ? AND account_id = ?)
			+ (SELECT COUNT(1) FROM reconciliation_checkpoint_postings WHERE book_id = ? AND account_id = ?)
	`, bookID, categoryID, bookID, categoryID, bookID, categoryID, bookID, categoryID).Scan(&referenceCount); err != nil {
		return false, fmt.Errorf("read category reference count: %w", err)
	}

	return referenceCount > 0, nil
}

func categoryAccountPredicate(alias string) string {
	return alias + `.account_class IN ('income', 'expense')
		AND ` + alias + `.account_kind = ` + alias + `.account_class
		AND json_extract(` + alias + `.metadata_json, '$.category.type') = ` + alias + `.account_class`
}

func accountSpecFromRecord(record AccountRecord) AccountSpec {
	return AccountSpec{
		Code:                  dbNullableString(record.Code),
		Name:                  dbNullableString(record.Name),
		AccountClass:          record.AccountClass,
		AccountKind:           record.AccountKind,
		ParentAccountID:       record.ParentAccountID,
		InstitutionID:         record.InstitutionID,
		CountryCode:           dbNullableString(record.CountryCode),
		DefaultCommodityID:    record.DefaultCommodityID,
		QuantityScaleOverride: record.QuantityScaleOverride,
		AllowsPostings:        record.AllowsPostings,
		NumberLast4:           dbNullableString(record.NumberLast4),
		ExternalRefHint:       dbNullableString(record.ExternalRefHint),
		CommentMarkdown:       record.CommentMarkdown,
		MetadataJSON:          record.MetadataJSON,
		OpenedOn:              record.OpenedOn,
		ClosedOn:              record.ClosedOn,
	}
}

func jsonExtractBool(jsonText string, path string) bool {
	if path != "$.category.is_builtin" {
		return false
	}
	var envelope struct {
		Category struct {
			IsBuiltin bool `json:"is_builtin"`
		} `json:"category"`
	}
	if err := json.Unmarshal([]byte(jsonText), &envelope); err != nil {
		return false
	}
	return envelope.Category.IsBuiltin
}

func dbNullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
