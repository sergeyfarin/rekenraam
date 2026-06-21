package db

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"rekenraam/backend/internal/exact"
)

var (
	ErrTransactionHasPostedVersions = errors.New("transaction has posted or voided versions")
	ErrTransactionReconciled        = errors.New("transaction has reconciled postings")
	ErrTransactionVoided            = errors.New("voided transaction cannot be updated")
	ErrTransactionDeleted           = errors.New("soft-deleted transaction cannot be updated")
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
	DeletedAt                 sql.NullString
	VersionID                 int64
	VersionSeq                int64
	SupersedesVersionID       sql.NullInt64
	Status                    string
	TransactionKind           string
	TransactionDate           string
	TransactionDaySequence    int64
	PayeeID                   sql.NullInt64
	PayeeName                 sql.NullString
	Description               string
	ExternalRefHint           sql.NullString
	NoteMarkdown              string
	MetadataJSON              string
	NeedsReview               bool
	RecordedAt                string
	ChangedByUserID           int64
	ChangeReason              string
	TagIDs                    []int64
	JournalEntries            []JournalEntryRecord
	InvalidatedCheckpointIDs  []int64
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
	AccountDaySequence   int64
	QuantityValue        exact.Coefficient
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
	NeedsReview     bool
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
	QuantityValue        exact.Coefficient
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
	CategoryID      int64
	PayeeID         int64
	Status          string
	ExcludeDraft    bool // true when no Status filter: omit draft rows
	Kind            string
	NeedsReview     bool
	Query           string
	AfterDate            string
	BeforeDate           string
	CursorDate           string
	CursorDaySequence    int64
	CursorID             int64
	Limit                int
	FilterEntryDate      bool
}

type CreateTransactionParams struct {
	BookID                     int64
	CorrectionOfTransactionID  sql.NullInt64
	ActorUserID                int64
	AuthSessionID              int64
	RequestID                  string
	OriginType                 string
	Operation                  string
	Spec                       TransactionSpec
	CreatedAt                  string
	ChangeReason               string
	InvalidateCheckpointRefs   []CheckpointInvalidationRef
	InvalidateCheckpointReason string
}

type UpdateTransactionParams struct {
	BookID                     int64
	TransactionID              int64
	ActorUserID                int64
	AuthSessionID              int64
	RequestID                  string
	OriginType                 string
	Operation                  string
	Spec                       TransactionSpec
	RecordedAt                 string
	ChangeReason               string
	InvalidateCheckpointRefs   []CheckpointInvalidationRef
	InvalidateCheckpointReason string
}

type VoidTransactionParams struct {
	BookID                     int64
	TransactionID              int64
	ActorUserID                int64
	AuthSessionID              int64
	RequestID                  string
	OriginType                 string
	Operation                  string
	RecordedAt                 string
	ChangeReason               string
	InvalidateCheckpointRefs   []CheckpointInvalidationRef
	InvalidateCheckpointReason string
}

type TransactionLifecycleParams = VoidTransactionParams

type SetTransactionDeletedParams struct {
	TransactionLifecycleParams
	Deleted bool
}

type DeleteDraftTransactionParams struct {
	BookID        int64
	TransactionID int64
}

type ApproveTransactionParams struct {
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

type CheckpointInvalidationRef struct {
	AccountID   int64
	CommodityID int64
	EntryDate   string
}

func NewTransactionRepository(database *sql.DB) *TransactionRepository {
	return &TransactionRepository{database: database}
}

func (r *TransactionRepository) ListTransactions(ctx context.Context, params ListTransactionsParams) ([]TransactionRecord, error) {
	where := []string{"t.book_id = ?", "t.deleted_at IS NULL"}
	args := []any{params.BookID}

	if params.Status != "" {
		where = append(where, "tv.status = ?")
		args = append(args, params.Status)
	} else if params.ExcludeDraft {
		where = append(where, "tv.status != 'draft'")
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
	if params.CategoryID > 0 {
		where = append(where, `EXISTS (
			SELECT 1
			FROM journal_entries category_je
			JOIN posting_versions category_pv ON category_pv.journal_entry_id = category_je.id
			WHERE category_je.transaction_version_id = tv.id
				AND category_pv.account_id = ?
		)`)
		args = append(args, params.CategoryID)
	}
	if params.NeedsReview {
		where = append(where, "tv.needs_review = 1")
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
		where = append(where, `(
			tv.transaction_date < ?
			OR (tv.transaction_date = ? AND tv.transaction_day_sequence < ?)
			OR (tv.transaction_date = ? AND tv.transaction_day_sequence = ? AND t.id < ?)
		)`)
		args = append(args,
			params.CursorDate,
			params.CursorDate, params.CursorDaySequence,
			params.CursorDate, params.CursorDaySequence, params.CursorID,
		)
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
		ORDER BY tv.transaction_date DESC, tv.transaction_day_sequence DESC, t.id DESC
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
	where := []string{"t.book_id = ?", "t.deleted_at IS NULL", "pv.account_id = ?"}
	args := []any{params.BookID, params.AccountID}

	if params.Status != "" {
		where = append(where, "tv.status = ?")
		args = append(args, params.Status)
	} else if params.ExcludeDraft {
		where = append(where, "tv.status != 'draft'")
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
		where = append(where, `(
			je.entry_date < ?
			OR (je.entry_date = ? AND pv.account_day_sequence < ?)
			OR (je.entry_date = ? AND pv.account_day_sequence = ? AND pv.id < ?)
		)`)
		args = append(args,
			params.CursorDate,
			params.CursorDate, params.CursorDaySequence,
			params.CursorDate, params.CursorDaySequence, params.CursorID,
		)
	}

	args = append(args, params.Limit)
	rows, err := r.database.QueryContext(ctx, accountRegisterSelect(`
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY je.entry_date DESC, pv.account_day_sequence DESC, pv.id DESC
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
	return r.transactionByID(ctx, bookID, transactionID, false)
}

func (r *TransactionRepository) TransactionByIDIncludingDeleted(ctx context.Context, bookID int64, transactionID int64) (TransactionRecord, error) {
	return r.transactionByID(ctx, bookID, transactionID, true)
}

func (r *TransactionRepository) transactionByID(ctx context.Context, bookID int64, transactionID int64, includeDeleted bool) (TransactionRecord, error) {
	var record TransactionRecord
	deletedCondition := " AND t.deleted_at IS NULL"
	if includeDeleted {
		deletedCondition = ""
	}
	if err := scanTransactionRecord(r.database.QueryRowContext(ctx, transactionSelect(`
		WHERE t.book_id = ? AND t.id = ?`+deletedCondition+`
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
			rollbackTx(ctx, tx)
		}
	}()

	record, err := createTransactionTx(ctx, tx, params)
	if err != nil {
		return TransactionRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, fmt.Errorf("commit create transaction: %w", err)
	}
	committed = true

	return record, nil
}

func createTransactionTx(ctx context.Context, tx *sql.Tx, params CreateTransactionParams) (TransactionRecord, error) {
	record, auditEventID, err := createTransactionWithAuditTx(ctx, tx, params)
	if err != nil {
		return TransactionRecord{}, err
	}
	if len(params.InvalidateCheckpointRefs) > 0 {
		invalidatedIDs, err := invalidateReconciliationCheckpoints(ctx, tx, checkpointInvalidationParams{
			BookID:       params.BookID,
			Refs:         params.InvalidateCheckpointRefs,
			ActorUserID:  params.ActorUserID,
			AuditEventID: auditEventID,
			OccurredAt:   params.CreatedAt,
			Reason:       params.InvalidateCheckpointReason,
		})
		if err != nil {
			return TransactionRecord{}, err
		}
		record.InvalidatedCheckpointIDs = invalidatedIDs
	}
	return record, nil
}

func createTransactionWithAuditTx(ctx context.Context, tx *sql.Tx, params CreateTransactionParams) (TransactionRecord, int64, error) {
	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return TransactionRecord{}, 0, err
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
		return TransactionRecord{}, 0, err
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
		return TransactionRecord{}, 0, fmt.Errorf("insert transaction: %w", err)
	}
	transactionID, err := result.LastInsertId()
	if err != nil {
		return TransactionRecord{}, 0, fmt.Errorf("read transaction id: %w", err)
	}

	repository := &TransactionRepository{}
	record, err := repository.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
		BookID:             params.BookID,
		TransactionID:      transactionID,
		VersionSeq:         1,
		Spec:               params.Spec,
		ReplaceTags:        true,
		RecordedAt:         params.CreatedAt,
		ChangedByUserID:    params.ActorUserID,
		ChangeReason:       params.ChangeReason,
		ChangeAuditEventID: auditEventID,
		RequestID:          params.RequestID,
	})
	if err != nil {
		return TransactionRecord{}, 0, err
	}

	return record, auditEventID, nil
}

func (r *TransactionRepository) UpdateTransaction(ctx context.Context, params UpdateTransactionParams) (TransactionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("begin update transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
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
	if current.DeletedAt.Valid {
		return TransactionRecord{}, ErrTransactionDeleted
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

	// Inherit the existing day sequence when the date is unchanged; otherwise
	// the transaction moves to the end of its new date (0 triggers MAX+1).
	inheritedTxDaySeq := int64(0)
	if params.Spec.TransactionDate == current.TransactionDate {
		inheritedTxDaySeq = current.TransactionDaySequence
	}

	record, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
		BookID:                 params.BookID,
		TransactionID:          params.TransactionID,
		VersionSeq:             current.VersionSeq + 1,
		SupersedesVersionID:    sql.NullInt64{Int64: current.VersionID, Valid: true},
		Spec:                   params.Spec,
		ReplaceTags:            true,
		RecordedAt:             params.RecordedAt,
		ChangedByUserID:        params.ActorUserID,
		ChangeReason:           params.ChangeReason,
		ChangeAuditEventID:     auditEventID,
		RequestID:              params.RequestID,
		TransactionDaySequence: inheritedTxDaySeq,
	})
	if err != nil {
		return TransactionRecord{}, mapTransactionConstraintError(err)
	}
	invalidatedCheckpointIDs, err := invalidateReconciliationCheckpoints(ctx, tx, checkpointInvalidationParams{
		BookID:       params.BookID,
		Refs:         params.InvalidateCheckpointRefs,
		ActorUserID:  params.ActorUserID,
		AuditEventID: auditEventID,
		OccurredAt:   params.RecordedAt,
		Reason:       params.InvalidateCheckpointReason,
	})
	if err != nil {
		return TransactionRecord{}, err
	}
	record.InvalidatedCheckpointIDs = invalidatedCheckpointIDs

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
			rollbackTx(ctx, tx)
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
		records := []TransactionRecord{current}
		if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
			return TransactionRecord{}, err
		}
		if err := tx.Commit(); err != nil {
			return TransactionRecord{}, fmt.Errorf("commit void transaction: %w", err)
		}
		committed = true
		return records[0], nil
	}
	if current.DeletedAt.Valid {
		return TransactionRecord{}, ErrTransactionDeleted
	}
	currentRecords := []TransactionRecord{current}
	if err := loadTransactionChildrenTx(ctx, tx, currentRecords); err != nil {
		return TransactionRecord{}, err
	}
	current = currentRecords[0]

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

	spec := transactionSpecFromRecord(current)
	spec.Status = "voided"
	record, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
		BookID:                 params.BookID,
		TransactionID:          params.TransactionID,
		VersionSeq:             current.VersionSeq + 1,
		SupersedesVersionID:    sql.NullInt64{Int64: current.VersionID, Valid: true},
		Spec:                   spec,
		ReplaceTags:            false,
		RecordedAt:             params.RecordedAt,
		ChangedByUserID:        params.ActorUserID,
		ChangeReason:           params.ChangeReason,
		ChangeAuditEventID:     auditEventID,
		RequestID:              params.RequestID,
		TransactionDaySequence: current.TransactionDaySequence,
	})
	if err != nil {
		return TransactionRecord{}, mapTransactionConstraintError(err)
	}
	invalidatedCheckpointIDs, err := invalidateReconciliationCheckpoints(ctx, tx, checkpointInvalidationParams{
		BookID:       params.BookID,
		Refs:         params.InvalidateCheckpointRefs,
		ActorUserID:  params.ActorUserID,
		AuditEventID: auditEventID,
		OccurredAt:   params.RecordedAt,
		Reason:       params.InvalidateCheckpointReason,
	})
	if err != nil {
		return TransactionRecord{}, err
	}
	record.InvalidatedCheckpointIDs = invalidatedCheckpointIDs

	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, fmt.Errorf("commit void transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *TransactionRepository) UnvoidTransaction(ctx context.Context, params TransactionLifecycleParams) (TransactionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("begin unvoid transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return TransactionRecord{}, err
	}
	current, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
	if err != nil {
		return TransactionRecord{}, err
	}
	if current.DeletedAt.Valid {
		return TransactionRecord{}, ErrTransactionDeleted
	}
	if current.Status != "voided" {
		records := []TransactionRecord{current}
		if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
			return TransactionRecord{}, err
		}
		if err := tx.Commit(); err != nil {
			return TransactionRecord{}, fmt.Errorf("commit unvoid transaction: %w", err)
		}
		committed = true
		return records[0], nil
	}

	var prior TransactionRecord
	if err := scanTransactionRecord(tx.QueryRowContext(ctx, transactionVersionSelect("transaction_versions", `
		WHERE tv.id = (
			SELECT prior.id FROM transaction_versions prior
			WHERE prior.transaction_id = ? AND prior.status <> 'voided'
			ORDER BY prior.version_seq DESC, prior.id DESC LIMIT 1
		)
	`), params.TransactionID), &prior); err != nil {
		return TransactionRecord{}, err
	}
	priorRecords := []TransactionRecord{prior}
	if err := loadTransactionChildrenTx(ctx, tx, priorRecords); err != nil {
		return TransactionRecord{}, err
	}
	prior = priorRecords[0]

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID,
		OccurredAt: params.RecordedAt, RequestID: params.RequestID, OriginType: params.OriginType,
		Operation: params.Operation, Reason: params.ChangeReason,
	})
	if err != nil {
		return TransactionRecord{}, err
	}
	record, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
		BookID: params.BookID, TransactionID: params.TransactionID, VersionSeq: current.VersionSeq + 1,
		SupersedesVersionID:    sql.NullInt64{Int64: current.VersionID, Valid: true},
		Spec:                   transactionSpecFromRecord(prior), ReplaceTags: false, RecordedAt: params.RecordedAt,
		ChangedByUserID:        params.ActorUserID, ChangeReason: params.ChangeReason,
		ChangeAuditEventID:     auditEventID, RequestID: params.RequestID,
		TransactionDaySequence: prior.TransactionDaySequence,
	})
	if err != nil {
		return TransactionRecord{}, mapTransactionConstraintError(err)
	}
	invalidated, err := invalidateReconciliationCheckpoints(ctx, tx, checkpointInvalidationParams{
		BookID: params.BookID, Refs: params.InvalidateCheckpointRefs, ActorUserID: params.ActorUserID,
		AuditEventID: auditEventID, OccurredAt: params.RecordedAt, Reason: params.InvalidateCheckpointReason,
	})
	if err != nil {
		return TransactionRecord{}, err
	}
	record.InvalidatedCheckpointIDs = invalidated
	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, fmt.Errorf("commit unvoid transaction: %w", err)
	}
	committed = true
	return record, nil
}

func (r *TransactionRepository) SetTransactionDeleted(ctx context.Context, params SetTransactionDeletedParams) (TransactionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("begin set transaction deleted: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return TransactionRecord{}, err
	}
	current, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
	if err != nil {
		return TransactionRecord{}, err
	}
	if current.DeletedAt.Valid == params.Deleted {
		records := []TransactionRecord{current}
		if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
			return TransactionRecord{}, err
		}
		if err := tx.Commit(); err != nil {
			return TransactionRecord{}, fmt.Errorf("commit set transaction deleted: %w", err)
		}
		committed = true
		return records[0], nil
	}
	if params.Deleted && current.Status == "draft" {
		return TransactionRecord{}, ErrTransactionHasPostedVersions
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID,
		OccurredAt: params.RecordedAt, RequestID: params.RequestID, OriginType: params.OriginType,
		Operation: params.Operation, Reason: params.ChangeReason,
	})
	if err != nil {
		return TransactionRecord{}, err
	}
	if params.Deleted {
		_, err = tx.ExecContext(ctx, `UPDATE transactions SET deleted_at = ?, deleted_by_user_id = ?, deleted_audit_event_id = ?, delete_reason = ? WHERE book_id = ? AND id = ?`, params.RecordedAt, params.ActorUserID, auditEventID, params.ChangeReason, params.BookID, params.TransactionID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE transactions SET deleted_at = NULL WHERE book_id = ? AND id = ?`, params.BookID, params.TransactionID)
	}
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("set transaction deleted: %w", err)
	}
	action := "restore"
	if params.Deleted {
		action = "soft_delete"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO transaction_deletion_events (book_id, transaction_id, action, occurred_at, actor_user_id, audit_event_id, reason)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, params.BookID, params.TransactionID, action, params.RecordedAt, params.ActorUserID, auditEventID, params.ChangeReason); err != nil {
		return TransactionRecord{}, fmt.Errorf("insert transaction deletion event: %w", err)
	}
	invalidated, err := invalidateReconciliationCheckpoints(ctx, tx, checkpointInvalidationParams{
		BookID: params.BookID, Refs: params.InvalidateCheckpointRefs, ActorUserID: params.ActorUserID,
		AuditEventID: auditEventID, OccurredAt: params.RecordedAt, Reason: params.InvalidateCheckpointReason,
	})
	if err != nil {
		return TransactionRecord{}, err
	}
	record, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
	if err != nil {
		return TransactionRecord{}, err
	}
	records := []TransactionRecord{record}
	if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
		return TransactionRecord{}, err
	}
	record = records[0]
	record.InvalidatedCheckpointIDs = invalidated
	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, fmt.Errorf("commit set transaction deleted: %w", err)
	}
	committed = true
	return record, nil
}

func (r *TransactionRepository) ApproveTransaction(ctx context.Context, params ApproveTransactionParams) (TransactionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("begin approve transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return TransactionRecord{}, err
	}
	current, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
	if err != nil {
		return TransactionRecord{}, err
	}
	if !current.NeedsReview {
		records := []TransactionRecord{current}
		if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
			return TransactionRecord{}, err
		}
		if err := tx.Commit(); err != nil {
			return TransactionRecord{}, fmt.Errorf("commit approve transaction: %w", err)
		}
		committed = true
		return records[0], nil
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
		Status:          current.Status,
		TransactionKind: current.TransactionKind,
		TransactionDate: current.TransactionDate,
		PayeeID:         current.PayeeID,
		PayeeName:       current.PayeeName,
		Description:     current.Description,
		ExternalRefHint: nullStringText(current.ExternalRefHint),
		NoteMarkdown:    current.NoteMarkdown,
		MetadataJSON:    current.MetadataJSON,
		NeedsReview:     false,
	}
	record, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
		BookID:                 params.BookID,
		TransactionID:          params.TransactionID,
		VersionSeq:             current.VersionSeq + 1,
		SupersedesVersionID:    sql.NullInt64{Int64: current.VersionID, Valid: true},
		Spec:                   spec,
		ReplaceTags:            false,
		RecordedAt:             params.RecordedAt,
		ChangedByUserID:        params.ActorUserID,
		ChangeReason:           params.ChangeReason,
		ChangeAuditEventID:     auditEventID,
		RequestID:              params.RequestID,
		TransactionDaySequence: current.TransactionDaySequence,
	})
	if err != nil {
		return TransactionRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, fmt.Errorf("commit approve transaction: %w", err)
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
			rollbackTx(ctx, tx)
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

type MoveTransactionParams struct {
	BookID        int64
	TransactionID int64
	Direction     string // "earlier" | "later"
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	RecordedAt    string
}

// MoveTransaction swaps the transaction_day_sequence of the target transaction
// with the adjacent current transaction on the same date, atomically.
// Returns the updated record of the moved transaction. Returns ErrNotFound when
// there is no adjacent transaction to swap with.
func (r *TransactionRepository) MoveTransaction(ctx context.Context, params MoveTransactionParams) (TransactionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("begin move transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return TransactionRecord{}, err
	}
	current, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
	if err != nil {
		return TransactionRecord{}, err
	}

	// Find the adjacent transaction in the requested direction.
	var adjVersionID int64
	var adjTransactionID int64
	var adjSeq int64
	var adjVersionSeq int64

	var adjQuery string
	if params.Direction == "earlier" {
		// Moving earlier = swapping with the transaction whose sequence is one less (lower number = earlier in list).
		adjQuery = `
			SELECT ctv.id, ctv.transaction_id, ctv.transaction_day_sequence, ctv.version_seq
			FROM current_transaction_versions ctv
			JOIN transactions t ON t.id = ctv.transaction_id
			WHERE ctv.book_id = ?
				AND ctv.transaction_date = ?
				AND ctv.transaction_day_sequence < ?
				AND t.deleted_at IS NULL
			ORDER BY ctv.transaction_day_sequence DESC
			LIMIT 1
		`
	} else {
		// Moving later = swapping with the transaction whose sequence is one more.
		adjQuery = `
			SELECT ctv.id, ctv.transaction_id, ctv.transaction_day_sequence, ctv.version_seq
			FROM current_transaction_versions ctv
			JOIN transactions t ON t.id = ctv.transaction_id
			WHERE ctv.book_id = ?
				AND ctv.transaction_date = ?
				AND ctv.transaction_day_sequence > ?
				AND t.deleted_at IS NULL
			ORDER BY ctv.transaction_day_sequence ASC
			LIMIT 1
		`
	}
	err = tx.QueryRowContext(ctx, adjQuery, params.BookID, current.TransactionDate, current.TransactionDaySequence).
		Scan(&adjVersionID, &adjTransactionID, &adjSeq, &adjVersionSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return TransactionRecord{}, ErrNotFound
	}
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("find adjacent transaction: %w", err)
	}
	_ = adjVersionID

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID,
		OccurredAt: params.RecordedAt, RequestID: params.RequestID, OriginType: params.OriginType,
		Operation: "transaction.move", Reason: "moved " + params.Direction,
	})
	if err != nil {
		return TransactionRecord{}, err
	}

	// Load full current record (with children) for the current transaction.
	currentRecords := []TransactionRecord{current}
	if err := loadTransactionChildrenTx(ctx, tx, currentRecords); err != nil {
		return TransactionRecord{}, err
	}
	current = currentRecords[0]

	// Load adjacent transaction's spec so we can write new versions.
	adj, err := transactionByID(ctx, tx, params.BookID, adjTransactionID)
	if err != nil {
		return TransactionRecord{}, err
	}
	adjRecords := []TransactionRecord{adj}
	if err := loadTransactionChildrenTx(ctx, tx, adjRecords); err != nil {
		return TransactionRecord{}, err
	}
	adj = adjRecords[0]

	// Write new version of the current transaction with the adjacent transaction's sequence.
	movedRecord, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
		BookID: params.BookID, TransactionID: params.TransactionID, VersionSeq: current.VersionSeq + 1,
		SupersedesVersionID: sql.NullInt64{Int64: current.VersionID, Valid: true},
		Spec:                transactionSpecFromRecord(current), ReplaceTags: false,
		RecordedAt: params.RecordedAt, ChangedByUserID: params.ActorUserID,
		ChangeReason: "moved " + params.Direction, ChangeAuditEventID: auditEventID,
		RequestID: params.RequestID, TransactionDaySequence: adjSeq,
	})
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("insert moved transaction version: %w", err)
	}

	// Write new version of the adjacent transaction with the current transaction's sequence.
	_, err = r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
		BookID: params.BookID, TransactionID: adjTransactionID, VersionSeq: adj.VersionSeq + 1,
		SupersedesVersionID: sql.NullInt64{Int64: adj.VersionID, Valid: true},
		Spec:                transactionSpecFromRecord(adj), ReplaceTags: false,
		RecordedAt: params.RecordedAt, ChangedByUserID: params.ActorUserID,
		ChangeReason: "swapped for moved transaction", ChangeAuditEventID: auditEventID,
		RequestID: params.RequestID, TransactionDaySequence: current.TransactionDaySequence,
	})
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("insert adjacent transaction version after move: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, fmt.Errorf("commit move transaction: %w", err)
	}
	committed = true

	return movedRecord, nil
}

type MovePostingParams struct {
	BookID                 int64
	AccountID              int64
	PostingLineID          int64
	Direction              string // "earlier" | "later"
	ActorUserID            int64
	AuthSessionID          int64
	RequestID              string
	OriginType             string
	RecordedAt             string
	ReconciliationOverride bool
}

// MovePosting swaps the account_day_sequence of the target posting with the
// adjacent current posting in the same (account_id, entry_date) scope.
// Returns the parent TransactionRecord of the moved posting. Returns ErrNotFound
// when there is no adjacent posting to swap with.
func (r *TransactionRepository) MovePosting(ctx context.Context, params MovePostingParams) (TransactionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("begin move posting: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return TransactionRecord{}, err
	}

	// Read the current posting version for the given posting line.
	var currentPostingVersionID int64
	var currentTransactionVersionID int64
	var currentTransactionID int64
	var currentEntryDate string
	var currentSeq int64
	if err := tx.QueryRowContext(ctx, `
		SELECT pv.id, pv.transaction_version_id, t.id, je.entry_date, pv.account_day_sequence
		FROM posting_versions pv
		JOIN journal_entries je ON je.id = pv.journal_entry_id
		JOIN current_transaction_versions ctv ON ctv.id = pv.transaction_version_id
		JOIN transactions t ON t.id = ctv.transaction_id
		WHERE pv.book_id = ?
			AND pv.posting_line_id = ?
			AND pv.account_id = ?
			AND t.deleted_at IS NULL
		LIMIT 1
	`, params.BookID, params.PostingLineID, params.AccountID).Scan(
		&currentPostingVersionID, &currentTransactionVersionID, &currentTransactionID, &currentEntryDate, &currentSeq,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TransactionRecord{}, ErrNotFound
		}
		return TransactionRecord{}, fmt.Errorf("read current posting: %w", err)
	}
	_ = currentPostingVersionID

	// Find the adjacent posting in the same (account_id, entry_date) scope.
	var adjPostingLineID int64
	var adjTransactionID int64
	var adjSeq int64

	var adjQuery string
	if params.Direction == "earlier" {
		adjQuery = `
			SELECT pv.posting_line_id, t.id, pv.account_day_sequence
			FROM posting_versions pv
			JOIN journal_entries je ON je.id = pv.journal_entry_id
			JOIN current_transaction_versions ctv ON ctv.id = pv.transaction_version_id
			JOIN transactions t ON t.id = ctv.transaction_id
			WHERE pv.book_id = ?
				AND pv.account_id = ?
				AND je.entry_date = ?
				AND pv.account_day_sequence < ?
				AND t.deleted_at IS NULL
			ORDER BY pv.account_day_sequence DESC
			LIMIT 1
		`
	} else {
		adjQuery = `
			SELECT pv.posting_line_id, t.id, pv.account_day_sequence
			FROM posting_versions pv
			JOIN journal_entries je ON je.id = pv.journal_entry_id
			JOIN current_transaction_versions ctv ON ctv.id = pv.transaction_version_id
			JOIN transactions t ON t.id = ctv.transaction_id
			WHERE pv.book_id = ?
				AND pv.account_id = ?
				AND je.entry_date = ?
				AND pv.account_day_sequence > ?
				AND t.deleted_at IS NULL
			ORDER BY pv.account_day_sequence ASC
			LIMIT 1
		`
	}
	err = tx.QueryRowContext(ctx, adjQuery, params.BookID, params.AccountID, currentEntryDate, currentSeq).
		Scan(&adjPostingLineID, &adjTransactionID, &adjSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return TransactionRecord{}, ErrNotFound
	}
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("find adjacent posting: %w", err)
	}

	// Check whether the swap would move a posting across an active checkpoint boundary.
	// The current posting would take adjSeq; the adjacent posting would take currentSeq.
	// If the new position for either posting crosses the boundary, require override.
	if !params.ReconciliationOverride {
		var stmtDate string
		var stmtAcctSeq int64
		scanErr := tx.QueryRowContext(ctx, `
			SELECT statement_date, statement_account_sequence
			FROM reconciliation_checkpoints
			WHERE book_id = ?
				AND account_id = ?
				AND status = 'active'
			ORDER BY id DESC
			LIMIT 1
		`, params.BookID, params.AccountID).Scan(&stmtDate, &stmtAcctSeq)
		if scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows) {
			return TransactionRecord{}, fmt.Errorf("read checkpoint for move guard: %w", scanErr)
		}
		if scanErr == nil {
			// A posting is inside the period when: entry_date < stmtDate OR (entry_date == stmtDate AND seq <= stmtAcctSeq)
			insideBefore := func(seq int64) bool {
				return currentEntryDate < stmtDate || (currentEntryDate == stmtDate && seq <= stmtAcctSeq)
			}
			// A swap crosses the boundary when one sequence is inside and the other is not.
			if insideBefore(currentSeq) != insideBefore(adjSeq) {
				return TransactionRecord{}, ErrReconciliationOverrideRequired
			}
		}
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID,
		OccurredAt: params.RecordedAt, RequestID: params.RequestID, OriginType: params.OriginType,
		Operation: "posting.move", Reason: "moved " + params.Direction,
	})
	if err != nil {
		return TransactionRecord{}, err
	}

	// Write new transaction version for the moved posting's transaction.
	movedTxRecord, err := transactionByID(ctx, tx, params.BookID, currentTransactionID)
	if err != nil {
		return TransactionRecord{}, err
	}
	movedTxRecords := []TransactionRecord{movedTxRecord}
	if err := loadTransactionChildrenTx(ctx, tx, movedTxRecords); err != nil {
		return TransactionRecord{}, err
	}
	movedTxRecord = movedTxRecords[0]

	// Write a new version of the moved transaction with an explicit seq override
	// for the target posting line.
	movedResult, err := r.insertTransactionVersionWithExplicitPostingSeqs(ctx, tx, insertTransactionVersionParams{
		BookID: params.BookID, TransactionID: currentTransactionID, VersionSeq: movedTxRecord.VersionSeq + 1,
		SupersedesVersionID: sql.NullInt64{Int64: movedTxRecord.VersionID, Valid: true},
		Spec:                transactionSpecFromRecord(movedTxRecord), ReplaceTags: false,
		RecordedAt: params.RecordedAt, ChangedByUserID: params.ActorUserID,
		ChangeReason: "posting moved " + params.Direction, ChangeAuditEventID: auditEventID,
		RequestID: params.RequestID, TransactionDaySequence: movedTxRecord.TransactionDaySequence,
	}, map[int64]int64{params.PostingLineID: adjSeq})
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("insert moved posting version: %w", err)
	}

	// If the adjacent posting is in a different transaction, write a new version
	// of that transaction with the swapped sequence.
	if adjTransactionID != currentTransactionID {
		adjTxRecord, err := transactionByID(ctx, tx, params.BookID, adjTransactionID)
		if err != nil {
			return TransactionRecord{}, err
		}
		adjTxRecords := []TransactionRecord{adjTxRecord}
		if err := loadTransactionChildrenTx(ctx, tx, adjTxRecords); err != nil {
			return TransactionRecord{}, err
		}
		adjTxRecord = adjTxRecords[0]
		_, err = r.insertTransactionVersionWithExplicitPostingSeqs(ctx, tx, insertTransactionVersionParams{
			BookID: params.BookID, TransactionID: adjTransactionID, VersionSeq: adjTxRecord.VersionSeq + 1,
			SupersedesVersionID: sql.NullInt64{Int64: adjTxRecord.VersionID, Valid: true},
			Spec:                transactionSpecFromRecord(adjTxRecord), ReplaceTags: false,
			RecordedAt: params.RecordedAt, ChangedByUserID: params.ActorUserID,
			ChangeReason: "swapped for moved posting", ChangeAuditEventID: auditEventID,
			RequestID: params.RequestID, TransactionDaySequence: adjTxRecord.TransactionDaySequence,
		}, map[int64]int64{adjPostingLineID: currentSeq})
		if err != nil {
			return TransactionRecord{}, fmt.Errorf("insert adjacent posting version after move: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, fmt.Errorf("commit move posting: %w", err)
	}
	committed = true

	return movedResult, nil
}

// insertTransactionVersionWithExplicitPostingSeqs is like insertTransactionVersion
// but accepts a map of postingLineID → account_day_sequence overrides. Any
// posting line ID in the override map uses the specified sequence rather than
// inheriting or allocating.
func (r *TransactionRepository) insertTransactionVersionWithExplicitPostingSeqs(
	ctx context.Context, tx *sql.Tx, params insertTransactionVersionParams,
	seqOverrides map[int64]int64,
) (TransactionRecord, error) {
	txDaySeq := params.TransactionDaySequence
	if txDaySeq <= 0 {
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(transaction_day_sequence), 0) + 1
			FROM transaction_versions
			WHERE book_id = ? AND transaction_date = ?
		`, params.BookID, params.Spec.TransactionDate).Scan(&txDaySeq); err != nil {
			return TransactionRecord{}, fmt.Errorf("compute transaction day sequence: %w", err)
		}
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO transaction_versions (
			book_id, transaction_id, version_seq, supersedes_version_id,
			status, transaction_kind, transaction_date, transaction_day_sequence,
			payee_id, payee_name, description, external_ref_hint, note_markdown,
			metadata_json, needs_review, recorded_at, changed_by_user_id,
			change_reason, change_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?)
	`, params.BookID, params.TransactionID, params.VersionSeq, nullableInt64Value(params.SupersedesVersionID),
		params.Spec.Status, params.Spec.TransactionKind, params.Spec.TransactionDate, txDaySeq,
		nullableInt64Value(params.Spec.PayeeID), nullableStringValue(params.Spec.PayeeName),
		params.Spec.Description, params.Spec.ExternalRefHint, params.Spec.NoteMarkdown,
		params.Spec.MetadataJSON, boolToInt(params.Spec.NeedsReview), params.RecordedAt,
		params.ChangedByUserID, params.ChangeReason, params.ChangeAuditEventID)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("insert transaction version: %w", err)
	}
	transactionVersionID, err := result.LastInsertId()
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("read transaction version id: %w", err)
	}

	postingLineIDs := make(map[int64]bool)
	for entryIndex, entry := range params.Spec.JournalEntries {
		entryResult, err := tx.ExecContext(ctx, `
			INSERT INTO journal_entries (book_id, transaction_version_id, entry_seq, entry_date, entry_kind, memo, metadata_json)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, params.BookID, transactionVersionID, entryIndex+1, entry.EntryDate, entry.EntryKind, entry.Memo, entry.MetadataJSON)
		if err != nil {
			return TransactionRecord{}, fmt.Errorf("insert journal entry: %w", err)
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
			postingLineIDs[postingLineID] = true

			var acctDaySeq int64
			if override, ok := seqOverrides[postingLineID]; ok {
				acctDaySeq = override
			} else {
				acctDaySeq, err = allocateOrInheritAccountDaySeq(ctx, tx, params.BookID, params.SupersedesVersionID.Int64, postingLineID, posting.AccountID, entry.EntryDate)
				if err != nil {
					return TransactionRecord{}, err
				}
			}

			if _, err := tx.ExecContext(ctx, `
				INSERT INTO posting_versions (
					book_id, transaction_version_id, journal_entry_id, posting_line_id,
					line_seq, account_id, account_day_sequence, quantity_value, quantity_scale,
					commodity_id, memo, reconciliation_status, cleared_on, metadata_json
				)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, params.BookID, transactionVersionID, journalEntryID, postingLineID,
				postingIndex+1, posting.AccountID, acctDaySeq, posting.QuantityValue, posting.QuantityScale,
				posting.CommodityID, posting.Memo, posting.ReconciliationStatus,
				nullableStringValue(posting.ClearedOn), posting.MetadataJSON); err != nil {
				return TransactionRecord{}, fmt.Errorf("insert posting version: %w", err)
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
	ReplaceTags         bool
	RecordedAt          string
	ChangedByUserID     int64
	ChangeReason        string
	ChangeAuditEventID  int64
	RequestID           string
	// TransactionDaySequence: if > 0, use this value directly (same-date update that
	// inherits the existing sequence). If 0, allocate MAX+1 for the new date scope.
	TransactionDaySequence int64
}

func (r *TransactionRepository) insertTransactionVersion(ctx context.Context, tx *sql.Tx, params insertTransactionVersionParams) (TransactionRecord, error) {
	txDaySeq := params.TransactionDaySequence
	if txDaySeq <= 0 {
		// Allocate the next sequence position in (book_id, transaction_date).
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(transaction_day_sequence), 0) + 1
			FROM transaction_versions
			WHERE book_id = ? AND transaction_date = ?
		`, params.BookID, params.Spec.TransactionDate).Scan(&txDaySeq); err != nil {
			return TransactionRecord{}, fmt.Errorf("compute transaction day sequence: %w", err)
		}
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO transaction_versions (
			book_id,
			transaction_id,
			version_seq,
			supersedes_version_id,
			status,
			transaction_kind,
			transaction_date,
			transaction_day_sequence,
			payee_id,
			payee_name,
			description,
			external_ref_hint,
			note_markdown,
			metadata_json,
			needs_review,
			recorded_at,
			changed_by_user_id,
			change_reason,
			change_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?)
	`, params.BookID, params.TransactionID, params.VersionSeq, nullableInt64Value(params.SupersedesVersionID), params.Spec.Status, params.Spec.TransactionKind, params.Spec.TransactionDate, txDaySeq, nullableInt64Value(params.Spec.PayeeID), nullableStringValue(params.Spec.PayeeName), params.Spec.Description, params.Spec.ExternalRefHint, params.Spec.NoteMarkdown, params.Spec.MetadataJSON, boolToInt(params.Spec.NeedsReview), params.RecordedAt, params.ChangedByUserID, params.ChangeReason, params.ChangeAuditEventID)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("insert transaction version: %w", err)
	}
	transactionVersionID, err := result.LastInsertId()
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("read transaction version id: %w", err)
	}

	if params.ReplaceTags {
		if err := replaceTransactionTags(ctx, tx, params.BookID, params.TransactionID, params.Spec.TagIDs, params.RecordedAt, params.ChangedByUserID, params.ChangeAuditEventID); err != nil {
			return TransactionRecord{}, err
		}
	}

	postingLineIDs := make(map[int64]bool)
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
			postingLineIDs[postingLineID] = true

			acctDaySeq, err := allocateOrInheritAccountDaySeq(ctx, tx, params.BookID, params.SupersedesVersionID.Int64, postingLineID, posting.AccountID, entry.EntryDate)
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
					account_day_sequence,
					quantity_value,
					quantity_scale,
					commodity_id,
					memo,
					reconciliation_status,
					cleared_on,
					metadata_json
				)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, params.BookID, transactionVersionID, journalEntryID, postingLineID, postingIndex+1, posting.AccountID, acctDaySeq, posting.QuantityValue, posting.QuantityScale, posting.CommodityID, posting.Memo, posting.ReconciliationStatus, nullableStringValue(posting.ClearedOn), posting.MetadataJSON)
			if err != nil {
				return TransactionRecord{}, mapTransactionConstraintError(fmt.Errorf("insert posting version: %w", err))
			}
			if _, err := postingResult.LastInsertId(); err != nil {
				return TransactionRecord{}, fmt.Errorf("read posting version id: %w", err)
			}

			if params.ReplaceTags {
				if err := replacePostingTags(ctx, tx, params.BookID, postingLineID, posting.TagIDs, params.RecordedAt, params.ChangedByUserID, params.ChangeAuditEventID); err != nil {
					return TransactionRecord{}, err
				}
			}
		}
	}
	if params.ReplaceTags {
		if err := clearUnusedPostingLineTags(ctx, tx, params.BookID, params.TransactionID, postingLineIDs); err != nil {
			return TransactionRecord{}, err
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

// allocateOrInheritAccountDaySeq returns the account_day_sequence for the new
// posting version. If the posting line has a version attached to prevVersionID
// with the same account_id and entry_date, we inherit its sequence (the
// posting's register position is unchanged). Otherwise we allocate MAX+1 within
// (book_id, account_id, entry_date).
//
// prevVersionID must be the transaction_version_id that was current BEFORE the
// new version row was inserted. We cannot use current_transaction_versions here
// because the new version row is already inserted (but has no postings yet),
// so the view would return no rows for this posting.
func allocateOrInheritAccountDaySeq(ctx context.Context, tx *sql.Tx, bookID int64, prevVersionID int64, postingLineID int64, accountID int64, entryDate string) (int64, error) {
	if prevVersionID > 0 {
		var inherited int64
		err := tx.QueryRowContext(ctx, `
			SELECT pv.account_day_sequence
			FROM posting_versions pv
			JOIN journal_entries je ON je.id = pv.journal_entry_id
			WHERE pv.book_id = ?
				AND pv.transaction_version_id = ?
				AND pv.posting_line_id = ?
				AND pv.account_id = ?
				AND je.entry_date = ?
			LIMIT 1
		`, bookID, prevVersionID, postingLineID, accountID, entryDate).Scan(&inherited)
		if err == nil {
			return inherited, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("check existing posting day sequence: %w", err)
		}
	}

	// No current version with the same account/date — allocate next position.
	var next int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(pv.account_day_sequence), 0) + 1
		FROM posting_versions pv
		JOIN journal_entries je ON je.id = pv.journal_entry_id
		WHERE pv.book_id = ?
			AND pv.account_id = ?
			AND je.entry_date = ?
	`, bookID, accountID, entryDate).Scan(&next); err != nil {
		return 0, fmt.Errorf("compute account day sequence: %w", err)
	}
	return next, nil
}

func replaceTransactionTags(ctx context.Context, tx *sql.Tx, bookID int64, transactionID int64, tagIDs []int64, recordedAt string, actorUserID int64, auditEventID int64) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM transaction_tags WHERE book_id = ? AND transaction_id = ?", bookID, transactionID); err != nil {
		return fmt.Errorf("delete transaction tags: %w", err)
	}
	for _, tagID := range tagIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transaction_tags (
				book_id,
				transaction_id,
				tag_id,
				created_at,
				created_by_user_id,
				created_audit_event_id
			)
			VALUES (?, ?, ?, ?, ?, ?)
		`, bookID, transactionID, tagID, recordedAt, actorUserID, auditEventID); err != nil {
			return mapTransactionConstraintError(fmt.Errorf("insert transaction tag: %w", err))
		}
	}
	return nil
}

func replacePostingTags(ctx context.Context, tx *sql.Tx, bookID int64, postingLineID int64, tagIDs []int64, recordedAt string, actorUserID int64, auditEventID int64) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM posting_tags WHERE book_id = ? AND posting_line_id = ?", bookID, postingLineID); err != nil {
		return fmt.Errorf("delete posting tags: %w", err)
	}
	for _, tagID := range tagIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO posting_tags (
				book_id,
				posting_line_id,
				tag_id,
				created_at,
				created_by_user_id,
				created_audit_event_id
			)
			VALUES (?, ?, ?, ?, ?, ?)
		`, bookID, postingLineID, tagID, recordedAt, actorUserID, auditEventID); err != nil {
			return mapTransactionConstraintError(fmt.Errorf("insert posting tag: %w", err))
		}
	}
	return nil
}

func clearUnusedPostingLineTags(ctx context.Context, tx *sql.Tx, bookID int64, transactionID int64, usedPostingLineIDs map[int64]bool) error {
	if len(usedPostingLineIDs) == 0 {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM posting_tags
			WHERE book_id = ?
				AND posting_line_id IN (
					SELECT id FROM posting_lines WHERE transaction_id = ?
				)
		`, bookID, transactionID); err != nil {
			return fmt.Errorf("delete all posting tags for transaction: %w", err)
		}
		return nil
	}

	placeholders := make([]string, 0, len(usedPostingLineIDs))
	args := []any{bookID, transactionID}
	for postingLineID := range usedPostingLineIDs {
		placeholders = append(placeholders, "?")
		args = append(args, postingLineID)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM posting_tags
		WHERE book_id = ?
			AND posting_line_id IN (
				SELECT id FROM posting_lines WHERE transaction_id = ?
			)
			AND posting_line_id NOT IN (`+strings.Join(placeholders, ",")+`)
	`, args...); err != nil {
		return fmt.Errorf("delete unused posting line tags: %w", err)
	}
	return nil
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
			pv.account_day_sequence,
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
		if err := rows.Scan(&posting.ID, &posting.BookID, &posting.TransactionVersionID, &posting.JournalEntryID, &posting.PostingLineID, &posting.LineKey, &posting.LineSeq, &posting.AccountID, &posting.AccountDaySequence, &posting.QuantityValue, &posting.QuantityScale, &posting.CommodityID, &posting.Memo, &posting.ReconciliationStatus, &posting.ClearedOn, &posting.MetadataJSON); err != nil {
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
	if err := scanTransactionRecord(tx.QueryRowContext(ctx, transactionVersionSelect("transaction_versions", `
		WHERE tv.id = ?
	`), transactionVersionID), &record); err != nil {
		return TransactionRecord{}, err
	}

	return record, nil
}

func transactionSelect(extraConditions string) string {
	return transactionVersionSelect("current_transaction_versions", extraConditions)
}

func transactionVersionSelect(source string, extraConditions string) string {
	return `
		SELECT
			t.id,
			t.book_id,
			t.correction_of_transaction_id,
			t.created_at,
			t.created_by_user_id,
			t.deleted_at,
			tv.id,
			tv.version_seq,
			tv.supersedes_version_id,
			tv.status,
			tv.transaction_kind,
			tv.transaction_date,
			tv.transaction_day_sequence,
			tv.payee_id,
			tv.payee_name,
			tv.description,
			tv.external_ref_hint,
			tv.note_markdown,
			tv.metadata_json,
			tv.needs_review,
			tv.recorded_at,
			tv.changed_by_user_id,
			tv.change_reason
		FROM transactions t
		JOIN ` + source + ` tv ON tv.transaction_id = t.id
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
			t.deleted_at,
			tv.id,
			tv.version_seq,
			tv.supersedes_version_id,
			tv.status,
			tv.transaction_kind,
			tv.transaction_date,
			tv.transaction_day_sequence,
			tv.payee_id,
			tv.payee_name,
			tv.description,
			tv.external_ref_hint,
			tv.note_markdown,
			tv.metadata_json,
			tv.needs_review,
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
			pv.account_day_sequence,
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
		&record.DeletedAt,
		&record.VersionID,
		&record.VersionSeq,
		&record.SupersedesVersionID,
		&record.Status,
		&record.TransactionKind,
		&record.TransactionDate,
		&record.TransactionDaySequence,
		&record.PayeeID,
		&record.PayeeName,
		&record.Description,
		&record.ExternalRefHint,
		&record.NoteMarkdown,
		&record.MetadataJSON,
		&record.NeedsReview,
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
		&entry.Transaction.DeletedAt,
		&entry.Transaction.VersionID,
		&entry.Transaction.VersionSeq,
		&entry.Transaction.SupersedesVersionID,
		&entry.Transaction.Status,
		&entry.Transaction.TransactionKind,
		&entry.Transaction.TransactionDate,
		&entry.Transaction.TransactionDaySequence,
		&entry.Transaction.PayeeID,
		&entry.Transaction.PayeeName,
		&entry.Transaction.Description,
		&entry.Transaction.ExternalRefHint,
		&entry.Transaction.NoteMarkdown,
		&entry.Transaction.MetadataJSON,
		&entry.Transaction.NeedsReview,
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
		&entry.Posting.AccountDaySequence,
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

// EncodeTransactionCursor encodes the 3-tuple (date, day_sequence, id) used
// for stable same-day ordering of the global transaction list.
func EncodeTransactionCursor(transactionDate string, daySequence int64, transactionID int64) string {
	if transactionDate == "" || transactionID <= 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s|%d|%d", transactionDate, daySequence, transactionID)))
}

// DecodeTransactionCursor decodes a 3-tuple cursor produced by EncodeTransactionCursor.
func DecodeTransactionCursor(cursor string) (date string, daySequence int64, id int64, err error) {
	cleaned := strings.TrimSpace(cursor)
	if cleaned == "" {
		return "", 0, 0, nil
	}
	decoded, decErr := base64.RawURLEncoding.DecodeString(cleaned)
	if decErr != nil {
		return "", 0, 0, fmt.Errorf("cursor is invalid")
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 {
		return "", 0, 0, fmt.Errorf("cursor is invalid")
	}
	var seq, txID int64
	if _, scanErr := fmt.Sscanf(parts[1], "%d", &seq); scanErr != nil || seq < 0 {
		return "", 0, 0, fmt.Errorf("cursor is invalid")
	}
	if _, scanErr := fmt.Sscanf(parts[2], "%d", &txID); scanErr != nil || txID <= 0 {
		return "", 0, 0, fmt.Errorf("cursor is invalid")
	}
	return parts[0], seq, txID, nil
}

// EncodeRegisterCursor encodes the 3-tuple (entry_date, account_day_sequence, posting_version_id)
// for the account register pagination.
func EncodeRegisterCursor(entryDate string, daySequence int64, postingVersionID int64) string {
	if entryDate == "" || postingVersionID <= 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s|%d|%d", entryDate, daySequence, postingVersionID)))
}

// DecodeRegisterCursor decodes a 3-tuple cursor produced by EncodeRegisterCursor.
func DecodeRegisterCursor(cursor string) (date string, daySequence int64, id int64, err error) {
	// Register and transaction cursors share the same format; delegate.
	return DecodeTransactionCursor(cursor)
}
