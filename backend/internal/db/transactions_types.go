package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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

func (r *TransactionRepository) withTx(ctx context.Context, op string, fn func(*sql.Tx) error) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin %s: %w", op, err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s: %w", op, err)
	}
	committed = true
	return nil
}

func withTransactionRecordTx(r *TransactionRepository, ctx context.Context, op string, fn func(*sql.Tx) (TransactionRecord, error)) (TransactionRecord, error) {
	var record TransactionRecord
	err := r.withTx(ctx, op, func(tx *sql.Tx) error {
		var err error
		record, err = fn(tx)
		return err
	})
	if err != nil {
		return TransactionRecord{}, err
	}
	return record, err
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
	BookID            int64
	AccountID         int64
	CategoryID        int64
	PayeeID           int64
	Status            string
	ExcludeDraft      bool // true when no Status filter: omit draft rows
	Kind              string
	NeedsReview       bool
	Query             string
	AfterDate         string
	BeforeDate        string
	CursorDate        string
	CursorDaySequence int64
	CursorID          int64
	Limit             int
	FilterEntryDate   bool
}

type ListDeletedTransactionsParams struct {
	BookID          int64
	CursorDeletedAt string
	CursorID        int64
	Limit           int
}

// DeletedTransactionRecord is a TransactionRecord that also carries the snapshot
// columns written at soft-delete time.
type DeletedTransactionRecord struct {
	TransactionRecord
	DeleteReason string
	DeletedAt    string
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
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	ChangeReason  string
	OccurredAt    string
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
	CheckpointID             int64
	AccountID                int64
	CommodityID              int64
	EntryDate                string
	StatementDate            string
	StatementAccountSequence int64
	// Enriched fields — set by the app layer after the DB lookup.
	AccountLabel  string
	CommodityCode string
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
	// PostingSeqOverrides: posting line IDs present in this map use the specified
	// account day sequence. Nil means inherit or allocate for every posting.
	PostingSeqOverrides map[int64]int64
	// TransactionDaySequence: if > 0, use this value directly (same-date update that
	// inherits the existing sequence). If 0, allocate MAX+1 for the new date scope.
	TransactionDaySequence int64
}

func NewTransactionRepository(database *sql.DB) *TransactionRepository {
	return &TransactionRepository{database: database}
}
