package db

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrTransactionHasPostedVersions = errors.New("transaction has posted or voided versions")
	ErrTransactionReconciled        = errors.New("transaction has reconciled postings")
	ErrTransactionVoided            = errors.New("voided transaction cannot be updated")
	ErrArchivedTag                  = errors.New("archived tag cannot be assigned")
)

type TransactionRepository struct {
	database *sql.DB
}

type TransactionRecord struct {
	ID                        int64
	BookID                    int64
	CorrectionOfTransactionID sql.NullInt64
	CreatedAt                 string
	CreatedByUserID           int64
	VersionID                 int64
	VersionSeq                int64
	SupersedesVersionID       sql.NullInt64
	Status                    string
	TransactionKind           string
	TransactionDate           string
	PayeeID                   sql.NullInt64
	PayeeName                 sql.NullString
	Description               string
	ExternalRefHint           sql.NullString
	NoteMarkdown              string
	MetadataJSON              string
	RecordedAt                string
	ChangedByUserID           int64
	ChangeReason              string
	TagIDs                    []int64
	JournalEntries            []JournalEntryRecord
}

type AccountRegisterEntryRecord struct {
	Transaction  TransactionRecord
	JournalEntry JournalEntryRecord
	Posting      PostingRecord
}

type JournalEntryRecord struct {
	ID                   int64
	BookID               int64
	TransactionVersionID int64
	EntrySeq             int64
	EntryDate            string
	EntryKind            string
	Memo                 string
	MetadataJSON         string
	Postings             []PostingRecord
}

type PostingRecord struct {
	ID                   int64
	BookID               int64
	TransactionVersionID int64
	JournalEntryID       int64
	PostingLineID        int64
	LineKey              string
	LineSeq              int64
	AccountID            int64
	QuantityValue        int64
	QuantityScale        int
	CommodityID          int64
	Memo                 string
	ReconciliationStatus string
	ClearedOn            sql.NullString
	MetadataJSON         string
	TagIDs               []int64
}

type TransactionSpec struct {
	Status          string
	TransactionKind string
	TransactionDate string
	PayeeID         sql.NullInt64
	PayeeName       sql.NullString
	Description     string
	ExternalRefHint string
	NoteMarkdown    string
	MetadataJSON    string
	TagIDs          []int64
	JournalEntries  []JournalEntrySpec
}

type JournalEntrySpec struct {
	EntryDate    string
	EntryKind    string
	Memo         string
	MetadataJSON string
	Postings     []PostingSpec
}

type PostingSpec struct {
	LineKey              string
	AccountID            int64
	QuantityValue        int64
	QuantityScale        int
	CommodityID          int64
	Memo                 string
	ReconciliationStatus string
	ClearedOn            sql.NullString
	MetadataJSON         string
	TagIDs               []int64
}

type ListTransactionsParams struct {
	BookID          int64
	AccountID       int64
	PayeeID         int64
	Status          string
	Kind            string
	Query           string
	AfterDate       string
	BeforeDate      string
	CursorDate      string
	CursorID        int64
	Limit           int
	FilterEntryDate bool
}

type CreateTransactionParams struct {
	BookID                    int64
	CorrectionOfTransactionID sql.NullInt64
	ActorUserID               int64
	AuthSessionID             int64
	RequestID                 string
	OriginType                string
	Operation                 string
	Spec                      TransactionSpec
	CreatedAt                 string
	ChangeReason              string
}

type UpdateTransactionParams struct {
	BookID        int64
	TransactionID int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Spec          TransactionSpec
	RecordedAt    string
	ChangeReason  string
}

type VoidTransactionParams struct {
	BookID        int64
	TransactionID int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	RecordedAt    string
	ChangeReason  string
}

type DeleteDraftTransactionParams struct {
	BookID        int64
	TransactionID int64
}

type PostingAccountRule struct {
	AccountID             int64
	AccountClass          string
	Status                string
	OpenedOn              string
	ClosedOn              sql.NullString
	DefaultCommodityID    sql.NullInt64
	QuantityScaleOverride sql.NullInt64
	AllowsPostings        bool
	IsSystem              bool
}

type PostingCommodityRule struct {
	CommodityID      int64
	Status           string
	MaxQuantityScale int
	StandardScale    int
	CommodityBookID  int64
	CommodityKind    string
}

func NewTransactionRepository(database *sql.DB) *TransactionRepository {
	return &TransactionRepository{database: database}
}

func (r *TransactionRepository) ListTransactions(ctx context.Context, params ListTransactionsParams) ([]TransactionRecord, error) {
	where := []string{"t.book_id = ?"}
	args := []any{params.BookID}

	if params.Status != "" {
		where = append(where, "tv.status = ?")
		args = append(args, params.Status)
	}
	if params.Kind != "" {
		where = append(where, "tv.transaction_kind = ?")
		args = append(args, params.Kind)
	}
	if params.PayeeID > 0 {
		where = append(where, "tv.payee_id = ?")
		args = append(args, params.PayeeID)
	}
	if params.AccountID > 0 {
		where = append(where, `EXISTS (
			SELECT 1
			FROM journal_entries account_je
			JOIN posting_versions account_pv ON account_pv.journal_entry_id = account_je.id
			WHERE account_je.transaction_version_id = tv.id
				AND account_pv.account_id = ?
		)`)
		args = append(args, params.AccountID)
	}
	dateColumn := "tv.transaction_date"
	if params.FilterEntryDate {
		dateColumn = `(SELECT MAX(date_je.entry_date) FROM journal_entries date_je WHERE date_je.transaction_version_id = tv.id)`
	}
	if params.AfterDate != "" {
		where = append(where, dateColumn+" >= ?")
		args = append(args, params.AfterDate)
	}
	if params.BeforeDate != "" {
		where = append(where, dateColumn+" <= ?")
		args = append(args, params.BeforeDate)
	}
	if params.CursorDate != "" && params.CursorID > 0 {
		where = append(where, "(tv.transaction_date < ? OR (tv.transaction_date = ? AND t.id < ?))")
		args = append(args, params.CursorDate, params.CursorDate, params.CursorID)
	}
	if strings.TrimSpace(params.Query) != "" {
		where = append(where, `tv.id IN (
			SELECT CAST(transaction_version_id AS INTEGER)
			FROM transaction_search
			WHERE transaction_search MATCH ?
		)`)
		args = append(args, ftsPhrase(params.Query))
	}

	args = append(args, params.Limit)
	rows, err := r.database.QueryContext(ctx, transactionSelect(`
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY tv.transaction_date DESC, t.id DESC
		LIMIT ?
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}
	defer rows.Close()

	records, err := scanTransactionRecords(rows)
	if err != nil {
		return nil, err
	}
	if err := r.loadTransactionChildren(ctx, records); err != nil {
		return nil, err
	}

	return records, nil
}

func (r *TransactionRepository) AccountRegister(ctx context.Context, params ListTransactionsParams) ([]AccountRegisterEntryRecord, error) {
	where := []string{"t.book_id = ?", "pv.account_id = ?"}
	args := []any{params.BookID, params.AccountID}

	if params.Status != "" {
		where = append(where, "tv.status = ?")
		args = append(args, params.Status)
	}
	if params.AfterDate != "" {
		where = append(where, "je.entry_date >= ?")
		args = append(args, params.AfterDate)
	}
	if params.BeforeDate != "" {
		where = append(where, "je.entry_date <= ?")
		args = append(args, params.BeforeDate)
	}
	if params.CursorDate != "" && params.CursorID > 0 {
		where = append(where, "(je.entry_date < ? OR (je.entry_date = ? AND pv.id < ?))")
		args = append(args, params.CursorDate, params.CursorDate, params.CursorID)
	}

	args = append(args, params.Limit)
	rows, err := r.database.QueryContext(ctx, accountRegisterSelect(`
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY je.entry_date DESC, pv.id DESC
		LIMIT ?
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list account register: %w", err)
	}
	defer rows.Close()

	entries, err := scanAccountRegisterEntries(rows)
	if err != nil {
		return nil, err
	}
	if err := r.loadAccountRegisterTags(ctx, entries); err != nil {
		return nil, err
	}

	return entries, nil
}

func (r *TransactionRepository) TransactionByID(ctx context.Context, bookID int64, transactionID int64) (TransactionRecord, error) {
	var record TransactionRecord
	if err := scanTransactionRecord(r.database.QueryRowContext(ctx, transactionSelect(`
		WHERE t.book_id = ? AND t.id = ?
	`), bookID, transactionID), &record); err != nil {
		return TransactionRecord{}, err
	}

	records := []TransactionRecord{record}
	if err := r.loadTransactionChildren(ctx, records); err != nil {
		return TransactionRecord{}, err
	}

	return records[0], nil
}

func (r *TransactionRepository) CreateTransaction(ctx context.Context, params CreateTransactionParams) (TransactionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("begin create transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return TransactionRecord{}, err
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
		return TransactionRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO transactions (
			book_id,
			correction_of_transaction_id,
			created_at,
			created_by_user_id,
			created_request_id,
			created_audit_event_id
		)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), ?)
	`, params.BookID, nullableInt64Value(params.CorrectionOfTransactionID), params.CreatedAt, params.ActorUserID, params.RequestID, auditEventID)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("insert transaction: %w", err)
	}
	transactionID, err := result.LastInsertId()
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("read transaction id: %w", err)
	}

	record, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
		BookID:             params.BookID,
		TransactionID:      transactionID,
		VersionSeq:         1,
		Spec:               params.Spec,
		RecordedAt:         params.CreatedAt,
		ChangedByUserID:    params.ActorUserID,
		ChangeReason:       params.ChangeReason,
		ChangeAuditEventID: auditEventID,
		RequestID:          params.RequestID,
	})
	if err != nil {
		return TransactionRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, fmt.Errorf("commit create transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *TransactionRepository) UpdateTransaction(ctx context.Context, params UpdateTransactionParams) (TransactionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("begin update transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return TransactionRecord{}, err
	}

	current, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
	if err != nil {
		return TransactionRecord{}, err
	}
	if current.Status == "voided" {
		return TransactionRecord{}, ErrTransactionVoided
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
		return TransactionRecord{}, err
	}

	record, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
		BookID:              params.BookID,
		TransactionID:       params.TransactionID,
		VersionSeq:          current.VersionSeq + 1,
		SupersedesVersionID: sql.NullInt64{Int64: current.VersionID, Valid: true},
		Spec:                params.Spec,
		RecordedAt:          params.RecordedAt,
		ChangedByUserID:     params.ActorUserID,
		ChangeReason:        params.ChangeReason,
		ChangeAuditEventID:  auditEventID,
		RequestID:           params.RequestID,
	})
	if err != nil {
		return TransactionRecord{}, mapTransactionConstraintError(err)
	}

	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, fmt.Errorf("commit update transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *TransactionRepository) VoidTransaction(ctx context.Context, params VoidTransactionParams) (TransactionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("begin void transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return TransactionRecord{}, err
	}
	current, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
	if err != nil {
		return TransactionRecord{}, err
	}
	if current.Status == "voided" {
		return current, nil
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
		return TransactionRecord{}, err
	}

	spec := TransactionSpec{
		Status:          "voided",
		TransactionKind: current.TransactionKind,
		TransactionDate: current.TransactionDate,
		PayeeID:         current.PayeeID,
		PayeeName:       current.PayeeName,
		Description:     current.Description,
		ExternalRefHint: nullStringText(current.ExternalRefHint),
		NoteMarkdown:    current.NoteMarkdown,
		MetadataJSON:    current.MetadataJSON,
	}
	record, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
		BookID:              params.BookID,
		TransactionID:       params.TransactionID,
		VersionSeq:          current.VersionSeq + 1,
		SupersedesVersionID: sql.NullInt64{Int64: current.VersionID, Valid: true},
		Spec:                spec,
		RecordedAt:          params.RecordedAt,
		ChangedByUserID:     params.ActorUserID,
		ChangeReason:        params.ChangeReason,
		ChangeAuditEventID:  auditEventID,
		RequestID:           params.RequestID,
	})
	if err != nil {
		return TransactionRecord{}, mapTransactionConstraintError(err)
	}

	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, fmt.Errorf("commit void transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *TransactionRepository) DeleteDraftTransaction(ctx context.Context, params DeleteDraftTransactionParams) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete draft transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return err
	}
	if _, err := transactionByID(ctx, tx, params.BookID, params.TransactionID); err != nil {
		return err
	}

	var durableCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM transaction_versions
		WHERE transaction_id = ?
			AND status IN ('posted', 'voided')
	`, params.TransactionID).Scan(&durableCount); err != nil {
		return fmt.Errorf("count durable transaction versions: %w", err)
	}
	if durableCount > 0 {
		return ErrTransactionHasPostedVersions
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM posting_tags
		WHERE posting_line_id IN (
			SELECT id FROM posting_lines WHERE transaction_id = ?
		)
	`, params.TransactionID); err != nil {
		return fmt.Errorf("delete posting tags: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM transaction_tags WHERE transaction_id = ?", params.TransactionID); err != nil {
		return fmt.Errorf("delete transaction tags: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM posting_versions
		WHERE transaction_version_id IN (
			SELECT id FROM transaction_versions WHERE transaction_id = ?
		)
	`, params.TransactionID); err != nil {
		return fmt.Errorf("delete posting versions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM journal_entries
		WHERE transaction_version_id IN (
			SELECT id FROM transaction_versions WHERE transaction_id = ?
		)
	`, params.TransactionID); err != nil {
		return fmt.Errorf("delete journal entries: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM transaction_search WHERE transaction_id = ?", params.TransactionID); err != nil {
		return fmt.Errorf("delete transaction search rows: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM transaction_versions WHERE transaction_id = ?", params.TransactionID); err != nil {
		return fmt.Errorf("delete transaction versions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM posting_lines WHERE transaction_id = ?", params.TransactionID); err != nil {
		return fmt.Errorf("delete posting lines: %w", err)
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM transactions WHERE book_id = ? AND id = ?", params.BookID, params.TransactionID)
	if err != nil {
		return fmt.Errorf("delete transaction: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read delete transaction rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete draft transaction: %w", err)
	}
	committed = true

	return nil
}

func (r *TransactionRepository) PostingAccountRule(ctx context.Context, bookID int64, accountID int64, entryDate string) (PostingAccountRule, error) {
	var rule PostingAccountRule
	var systemRole sql.NullString
	var allowsPostings int
	if err := r.database.QueryRowContext(ctx, `
		SELECT
			a.id,
			av.account_class,
			av.status,
			av.opened_on,
			av.closed_on,
			av.default_commodity_id,
			av.quantity_scale_override,
			av.allows_postings,
			a.system_role
		FROM accounts a
		JOIN account_versions av ON av.account_id = a.id
		WHERE a.book_id = ?
			AND a.id = ?
			AND av.id = (
				SELECT asof_av.id
				FROM account_versions asof_av
				WHERE asof_av.account_id = a.id
					AND asof_av.effective_from <= ?
				ORDER BY asof_av.effective_from DESC, asof_av.version_seq DESC
				LIMIT 1
			)
	`, bookID, accountID, entryDate).Scan(
		&rule.AccountID,
		&rule.AccountClass,
		&rule.Status,
		&rule.OpenedOn,
		&rule.ClosedOn,
		&rule.DefaultCommodityID,
		&rule.QuantityScaleOverride,
		&allowsPostings,
		&systemRole,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PostingAccountRule{}, ErrNotFound
		}
		return PostingAccountRule{}, fmt.Errorf("read posting account rule: %w", err)
	}
	rule.AllowsPostings = allowsPostings == 1
	rule.IsSystem = systemRole.Valid

	return rule, nil
}

func (r *TransactionRepository) PostingCommodityRule(ctx context.Context, bookID int64, commodityID int64, entryDate string) (PostingCommodityRule, error) {
	var rule PostingCommodityRule
	if err := r.database.QueryRowContext(ctx, `
		SELECT
			c.id,
			c.book_id,
			c.kind,
			cv.status,
			cv.standard_scale,
			cv.max_quantity_scale
		FROM commodities c
		JOIN commodity_versions cv ON cv.commodity_id = c.id
		WHERE c.book_id = ?
			AND c.id = ?
			AND cv.id = (
				SELECT asof_cv.id
				FROM commodity_versions asof_cv
				WHERE asof_cv.commodity_id = c.id
					AND asof_cv.effective_from <= ?
				ORDER BY asof_cv.effective_from DESC, asof_cv.version_seq DESC
				LIMIT 1
			)
	`, bookID, commodityID, entryDate).Scan(&rule.CommodityID, &rule.CommodityBookID, &rule.CommodityKind, &rule.Status, &rule.StandardScale, &rule.MaxQuantityScale); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PostingCommodityRule{}, ErrNotFound
		}
		return PostingCommodityRule{}, fmt.Errorf("read posting commodity rule: %w", err)
	}

	return rule, nil
}

func (r *TransactionRepository) ActiveTagIDs(ctx context.Context, bookID int64, tagIDs []int64) (map[int64]bool, error) {
	if len(tagIDs) == 0 {
		return map[int64]bool{}, nil
	}

	placeholders := make([]string, 0, len(tagIDs))
	args := []any{bookID}
	for _, tagID := range tagIDs {
		placeholders = append(placeholders, "?")
		args = append(args, tagID)
	}

	rows, err := r.database.QueryContext(ctx, `
		SELECT id
		FROM tags
		WHERE book_id = ?
			AND status = 'active'
			AND id IN (`+strings.Join(placeholders, ",")+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("read active tags: %w", err)
	}
	defer rows.Close()

	active := make(map[int64]bool, len(tagIDs))
	for rows.Next() {
		var tagID int64
		if err := rows.Scan(&tagID); err != nil {
			return nil, fmt.Errorf("scan active tag: %w", err)
		}
		active[tagID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active tags: %w", err)
	}

	return active, nil
}

type insertTransactionVersionParams struct {
	BookID              int64
	TransactionID       int64
	VersionSeq          int64
	SupersedesVersionID sql.NullInt64
	Spec                TransactionSpec
	RecordedAt          string
	ChangedByUserID     int64
	ChangeReason        string
	ChangeAuditEventID  int64
	RequestID           string
}

func (r *TransactionRepository) insertTransactionVersion(ctx context.Context, tx *sql.Tx, params insertTransactionVersionParams) (TransactionRecord, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO transaction_versions (
			book_id,
			transaction_id,
			version_seq,
			supersedes_version_id,
			status,
			transaction_kind,
			transaction_date,
			payee_id,
			payee_name,
			description,
			external_ref_hint,
			note_markdown,
			metadata_json,
			recorded_at,
			changed_by_user_id,
			change_reason,
			change_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?)
	`, params.BookID, params.TransactionID, params.VersionSeq, nullableInt64Value(params.SupersedesVersionID), params.Spec.Status, params.Spec.TransactionKind, params.Spec.TransactionDate, nullableInt64Value(params.Spec.PayeeID), nullableStringValue(params.Spec.PayeeName), params.Spec.Description, params.Spec.ExternalRefHint, params.Spec.NoteMarkdown, params.Spec.MetadataJSON, params.RecordedAt, params.ChangedByUserID, params.ChangeReason, params.ChangeAuditEventID)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("insert transaction version: %w", err)
	}
	transactionVersionID, err := result.LastInsertId()
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("read transaction version id: %w", err)
	}

	for _, tagID := range params.Spec.TagIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO transaction_tags (
				book_id,
				transaction_id,
				tag_id,
				created_at,
				created_by_user_id,
				created_audit_event_id
			)
			VALUES (?, ?, ?, ?, ?, ?)
		`, params.BookID, params.TransactionID, tagID, params.RecordedAt, params.ChangedByUserID, params.ChangeAuditEventID); err != nil {
			return TransactionRecord{}, mapTransactionConstraintError(fmt.Errorf("insert transaction tag: %w", err))
		}
	}

	for entryIndex, entry := range params.Spec.JournalEntries {
		entryResult, err := tx.ExecContext(ctx, `
			INSERT INTO journal_entries (
				book_id,
				transaction_version_id,
				entry_seq,
				entry_date,
				entry_kind,
				memo,
				metadata_json
			)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, params.BookID, transactionVersionID, entryIndex+1, entry.EntryDate, entry.EntryKind, entry.Memo, entry.MetadataJSON)
		if err != nil {
			return TransactionRecord{}, mapTransactionConstraintError(fmt.Errorf("insert journal entry: %w", err))
		}
		journalEntryID, err := entryResult.LastInsertId()
		if err != nil {
			return TransactionRecord{}, fmt.Errorf("read journal entry id: %w", err)
		}

		for postingIndex, posting := range entry.Postings {
			postingLineID, err := ensurePostingLine(ctx, tx, params.BookID, params.TransactionID, posting.LineKey, params.RecordedAt, params.ChangedByUserID, params.RequestID, params.ChangeAuditEventID)
			if err != nil {
				return TransactionRecord{}, err
			}

			postingResult, err := tx.ExecContext(ctx, `
				INSERT INTO posting_versions (
					book_id,
					transaction_version_id,
					journal_entry_id,
					posting_line_id,
					line_seq,
					account_id,
					quantity_value,
					quantity_scale,
					commodity_id,
					memo,
					reconciliation_status,
					cleared_on,
					metadata_json
				)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, params.BookID, transactionVersionID, journalEntryID, postingLineID, postingIndex+1, posting.AccountID, posting.QuantityValue, posting.QuantityScale, posting.CommodityID, posting.Memo, posting.ReconciliationStatus, nullableStringValue(posting.ClearedOn), posting.MetadataJSON)
			if err != nil {
				return TransactionRecord{}, mapTransactionConstraintError(fmt.Errorf("insert posting version: %w", err))
			}
			if _, err := postingResult.LastInsertId(); err != nil {
				return TransactionRecord{}, fmt.Errorf("read posting version id: %w", err)
			}

			for _, tagID := range posting.TagIDs {
				if _, err := tx.ExecContext(ctx, `
					INSERT OR IGNORE INTO posting_tags (
						book_id,
						posting_line_id,
						tag_id,
						created_at,
						created_by_user_id,
						created_audit_event_id
					)
					VALUES (?, ?, ?, ?, ?, ?)
				`, params.BookID, postingLineID, tagID, params.RecordedAt, params.ChangedByUserID, params.ChangeAuditEventID); err != nil {
					return TransactionRecord{}, mapTransactionConstraintError(fmt.Errorf("insert posting tag: %w", err))
				}
			}
		}
	}

	record, err := transactionVersionByID(ctx, tx, transactionVersionID)
	if err != nil {
		return TransactionRecord{}, err
	}
	records := []TransactionRecord{record}
	if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
		return TransactionRecord{}, err
	}

	return records[0], nil
}

func ensurePostingLine(ctx context.Context, tx *sql.Tx, bookID int64, transactionID int64, lineKey string, createdAt string, actorUserID int64, requestID string, auditEventID int64) (int64, error) {
	var postingLineID int64
	err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM posting_lines
		WHERE book_id = ?
			AND transaction_id = ?
			AND line_key = ?
	`, bookID, transactionID, lineKey).Scan(&postingLineID)
	if err == nil {
		return postingLineID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("read posting line: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO posting_lines (
			book_id,
			transaction_id,
			line_key,
			created_at,
			created_by_user_id,
			created_request_id,
			created_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), ?)
	`, bookID, transactionID, lineKey, createdAt, actorUserID, requestID, auditEventID)
	if err != nil {
		return 0, fmt.Errorf("insert posting line: %w", err)
	}

	postingLineID, err = result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read posting line id: %w", err)
	}

	return postingLineID, nil
}

func (r *TransactionRepository) loadTransactionChildren(ctx context.Context, records []TransactionRecord) error {
	return loadTransactionChildrenQuery(ctx, r.database, records)
}

func (r *TransactionRepository) loadAccountRegisterTags(ctx context.Context, entries []AccountRegisterEntryRecord) error {
	for index := range entries {
		tagIDs, err := transactionTagIDs(ctx, r.database, entries[index].Transaction.ID)
		if err != nil {
			return err
		}
		entries[index].Transaction.TagIDs = tagIDs

		postingTagIDs, err := postingTagIDs(ctx, r.database, entries[index].Posting.PostingLineID)
		if err != nil {
			return err
		}
		entries[index].Posting.TagIDs = postingTagIDs
	}

	return nil
}

func loadTransactionChildrenTx(ctx context.Context, tx *sql.Tx, records []TransactionRecord) error {
	return loadTransactionChildrenQuery(ctx, tx, records)
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func loadTransactionChildrenQuery(ctx context.Context, queryer queryer, records []TransactionRecord) error {
	for index := range records {
		tagIDs, err := transactionTagIDs(ctx, queryer, records[index].ID)
		if err != nil {
			return err
		}
		records[index].TagIDs = tagIDs

		entries, err := journalEntriesForVersion(ctx, queryer, records[index].VersionID)
		if err != nil {
			return err
		}
		records[index].JournalEntries = entries
	}

	return nil
}

func transactionTagIDs(ctx context.Context, queryer queryer, transactionID int64) ([]int64, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT tag_id
		FROM transaction_tags
		WHERE transaction_id = ?
		ORDER BY tag_id
	`, transactionID)
	if err != nil {
		return nil, fmt.Errorf("read transaction tags: %w", err)
	}
	defer rows.Close()

	var tagIDs []int64
	for rows.Next() {
		var tagID int64
		if err := rows.Scan(&tagID); err != nil {
			return nil, fmt.Errorf("scan transaction tag: %w", err)
		}
		tagIDs = append(tagIDs, tagID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transaction tags: %w", err)
	}

	return tagIDs, nil
}

func journalEntriesForVersion(ctx context.Context, queryer queryer, transactionVersionID int64) ([]JournalEntryRecord, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT id, book_id, transaction_version_id, entry_seq, entry_date, entry_kind, memo, metadata_json
		FROM journal_entries
		WHERE transaction_version_id = ?
		ORDER BY entry_seq
	`, transactionVersionID)
	if err != nil {
		return nil, fmt.Errorf("read journal entries: %w", err)
	}
	defer rows.Close()

	var entries []JournalEntryRecord
	for rows.Next() {
		var entry JournalEntryRecord
		if err := rows.Scan(&entry.ID, &entry.BookID, &entry.TransactionVersionID, &entry.EntrySeq, &entry.EntryDate, &entry.EntryKind, &entry.Memo, &entry.MetadataJSON); err != nil {
			return nil, fmt.Errorf("scan journal entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate journal entries: %w", err)
	}

	for index := range entries {
		postings, err := postingsForJournalEntry(ctx, queryer, entries[index].ID)
		if err != nil {
			return nil, err
		}
		entries[index].Postings = postings
	}

	return entries, nil
}

func postingsForJournalEntry(ctx context.Context, queryer queryer, journalEntryID int64) ([]PostingRecord, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT
			pv.id,
			pv.book_id,
			pv.transaction_version_id,
			pv.journal_entry_id,
			pv.posting_line_id,
			pl.line_key,
			pv.line_seq,
			pv.account_id,
			pv.quantity_value,
			pv.quantity_scale,
			pv.commodity_id,
			pv.memo,
			pv.reconciliation_status,
			pv.cleared_on,
			pv.metadata_json
		FROM posting_versions pv
		JOIN posting_lines pl ON pl.id = pv.posting_line_id
		WHERE pv.journal_entry_id = ?
		ORDER BY pv.line_seq
	`, journalEntryID)
	if err != nil {
		return nil, fmt.Errorf("read posting versions: %w", err)
	}
	defer rows.Close()

	var postings []PostingRecord
	for rows.Next() {
		var posting PostingRecord
		if err := rows.Scan(&posting.ID, &posting.BookID, &posting.TransactionVersionID, &posting.JournalEntryID, &posting.PostingLineID, &posting.LineKey, &posting.LineSeq, &posting.AccountID, &posting.QuantityValue, &posting.QuantityScale, &posting.CommodityID, &posting.Memo, &posting.ReconciliationStatus, &posting.ClearedOn, &posting.MetadataJSON); err != nil {
			return nil, fmt.Errorf("scan posting version: %w", err)
		}
		postings = append(postings, posting)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate posting versions: %w", err)
	}

	for index := range postings {
		tagIDs, err := postingTagIDs(ctx, queryer, postings[index].PostingLineID)
		if err != nil {
			return nil, err
		}
		postings[index].TagIDs = tagIDs
	}

	return postings, nil
}

func postingTagIDs(ctx context.Context, queryer queryer, postingLineID int64) ([]int64, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT tag_id
		FROM posting_tags
		WHERE posting_line_id = ?
		ORDER BY tag_id
	`, postingLineID)
	if err != nil {
		return nil, fmt.Errorf("read posting tags: %w", err)
	}
	defer rows.Close()

	var tagIDs []int64
	for rows.Next() {
		var tagID int64
		if err := rows.Scan(&tagID); err != nil {
			return nil, fmt.Errorf("scan posting tag: %w", err)
		}
		tagIDs = append(tagIDs, tagID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate posting tags: %w", err)
	}

	return tagIDs, nil
}

func transactionByID(ctx context.Context, tx *sql.Tx, bookID int64, transactionID int64) (TransactionRecord, error) {
	var record TransactionRecord
	if err := scanTransactionRecord(tx.QueryRowContext(ctx, transactionSelect(`
		WHERE t.book_id = ? AND t.id = ?
	`), bookID, transactionID), &record); err != nil {
		return TransactionRecord{}, err
	}

	return record, nil
}

func transactionVersionByID(ctx context.Context, tx *sql.Tx, transactionVersionID int64) (TransactionRecord, error) {
	var record TransactionRecord
	if err := scanTransactionRecord(tx.QueryRowContext(ctx, transactionSelect(`
		WHERE tv.id = ?
	`), transactionVersionID), &record); err != nil {
		return TransactionRecord{}, err
	}

	return record, nil
}

func transactionSelect(extraConditions string) string {
	return `
		SELECT
			t.id,
			t.book_id,
			t.correction_of_transaction_id,
			t.created_at,
			t.created_by_user_id,
			tv.id,
			tv.version_seq,
			tv.supersedes_version_id,
			tv.status,
			tv.transaction_kind,
			tv.transaction_date,
			tv.payee_id,
			tv.payee_name,
			tv.description,
			tv.external_ref_hint,
			tv.note_markdown,
			tv.metadata_json,
			tv.recorded_at,
			tv.changed_by_user_id,
			tv.change_reason
		FROM transactions t
		JOIN current_transaction_versions tv ON tv.transaction_id = t.id
	` + extraConditions
}

func accountRegisterSelect(extraConditions string) string {
	return `
		SELECT
			t.id,
			t.book_id,
			t.correction_of_transaction_id,
			t.created_at,
			t.created_by_user_id,
			tv.id,
			tv.version_seq,
			tv.supersedes_version_id,
			tv.status,
			tv.transaction_kind,
			tv.transaction_date,
			tv.payee_id,
			tv.payee_name,
			tv.description,
			tv.external_ref_hint,
			tv.note_markdown,
			tv.metadata_json,
			tv.recorded_at,
			tv.changed_by_user_id,
			tv.change_reason,
			je.id,
			je.book_id,
			je.transaction_version_id,
			je.entry_seq,
			je.entry_date,
			je.entry_kind,
			je.memo,
			je.metadata_json,
			pv.id,
			pv.book_id,
			pv.transaction_version_id,
			pv.journal_entry_id,
			pv.posting_line_id,
			pl.line_key,
			pv.line_seq,
			pv.account_id,
			pv.quantity_value,
			pv.quantity_scale,
			pv.commodity_id,
			pv.memo,
			pv.reconciliation_status,
			pv.cleared_on,
			pv.metadata_json
		FROM transactions t
		JOIN current_transaction_versions tv ON tv.transaction_id = t.id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		JOIN posting_lines pl ON pl.id = pv.posting_line_id
	` + extraConditions
}

func scanTransactionRecords(rows *sql.Rows) ([]TransactionRecord, error) {
	var records []TransactionRecord
	for rows.Next() {
		var record TransactionRecord
		if err := scanTransactionRecord(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}

	return records, nil
}

func scanAccountRegisterEntries(rows *sql.Rows) ([]AccountRegisterEntryRecord, error) {
	var entries []AccountRegisterEntryRecord
	for rows.Next() {
		var entry AccountRegisterEntryRecord
		if err := scanAccountRegisterEntry(rows, &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account register: %w", err)
	}

	return entries, nil
}

func scanTransactionRecord(scanner interface{ Scan(dest ...any) error }, record *TransactionRecord) error {
	if err := scanner.Scan(
		&record.ID,
		&record.BookID,
		&record.CorrectionOfTransactionID,
		&record.CreatedAt,
		&record.CreatedByUserID,
		&record.VersionID,
		&record.VersionSeq,
		&record.SupersedesVersionID,
		&record.Status,
		&record.TransactionKind,
		&record.TransactionDate,
		&record.PayeeID,
		&record.PayeeName,
		&record.Description,
		&record.ExternalRefHint,
		&record.NoteMarkdown,
		&record.MetadataJSON,
		&record.RecordedAt,
		&record.ChangedByUserID,
		&record.ChangeReason,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("scan transaction: %w", err)
	}

	return nil
}

func scanAccountRegisterEntry(scanner interface{ Scan(dest ...any) error }, entry *AccountRegisterEntryRecord) error {
	if err := scanner.Scan(
		&entry.Transaction.ID,
		&entry.Transaction.BookID,
		&entry.Transaction.CorrectionOfTransactionID,
		&entry.Transaction.CreatedAt,
		&entry.Transaction.CreatedByUserID,
		&entry.Transaction.VersionID,
		&entry.Transaction.VersionSeq,
		&entry.Transaction.SupersedesVersionID,
		&entry.Transaction.Status,
		&entry.Transaction.TransactionKind,
		&entry.Transaction.TransactionDate,
		&entry.Transaction.PayeeID,
		&entry.Transaction.PayeeName,
		&entry.Transaction.Description,
		&entry.Transaction.ExternalRefHint,
		&entry.Transaction.NoteMarkdown,
		&entry.Transaction.MetadataJSON,
		&entry.Transaction.RecordedAt,
		&entry.Transaction.ChangedByUserID,
		&entry.Transaction.ChangeReason,
		&entry.JournalEntry.ID,
		&entry.JournalEntry.BookID,
		&entry.JournalEntry.TransactionVersionID,
		&entry.JournalEntry.EntrySeq,
		&entry.JournalEntry.EntryDate,
		&entry.JournalEntry.EntryKind,
		&entry.JournalEntry.Memo,
		&entry.JournalEntry.MetadataJSON,
		&entry.Posting.ID,
		&entry.Posting.BookID,
		&entry.Posting.TransactionVersionID,
		&entry.Posting.JournalEntryID,
		&entry.Posting.PostingLineID,
		&entry.Posting.LineKey,
		&entry.Posting.LineSeq,
		&entry.Posting.AccountID,
		&entry.Posting.QuantityValue,
		&entry.Posting.QuantityScale,
		&entry.Posting.CommodityID,
		&entry.Posting.Memo,
		&entry.Posting.ReconciliationStatus,
		&entry.Posting.ClearedOn,
		&entry.Posting.MetadataJSON,
	); err != nil {
		return fmt.Errorf("scan account register entry: %w", err)
	}

	return nil
}

func mapTransactionConstraintError(err error) error {
	message := err.Error()
	switch {
	case strings.Contains(message, "reconciled postings require a corrective transaction"):
		return ErrTransactionReconciled
	case strings.Contains(message, "active tag"):
		return ErrArchivedTag
	default:
		return err
	}
}

func ftsPhrase(value string) string {
	cleaned := strings.TrimSpace(value)
	cleaned = strings.ReplaceAll(cleaned, `"`, `""`)
	return `"` + cleaned + `"`
}

func nullStringText(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func EncodeTransactionCursor(transactionDate string, transactionID int64) string {
	if transactionDate == "" || transactionID <= 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s|%d", transactionDate, transactionID)))
}

func DecodeTransactionCursor(cursor string) (string, int64, error) {
	cleaned := strings.TrimSpace(cursor)
	if cleaned == "" {
		return "", 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cleaned)
	if err != nil {
		return "", 0, fmt.Errorf("cursor is invalid")
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("cursor is invalid")
	}
	var id int64
	if _, err := fmt.Sscanf(parts[1], "%d", &id); err != nil || id <= 0 {
		return "", 0, fmt.Errorf("cursor is invalid")
	}
	return parts[0], id, nil
}
