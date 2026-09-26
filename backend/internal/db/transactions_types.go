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
	// ErrTransactionVersionStale reports a write prepared against a transaction
	// version that is no longer current. See requireExpectedVersionTx.
	ErrTransactionVersionStale = errors.New("transaction changed after this edit was prepared")
	ErrTransactionDeleted      = errors.New("soft-deleted transaction cannot be updated")
	ErrArchivedTag             = errors.New("archived tag cannot be assigned")
	// ErrPostingAccountVersionStale reports a write whose posting checks were
	// decided against an account version that is no longer the account's
	// latest. See requireAccountRuleDependenciesTx (T-100).
	ErrPostingAccountVersionStale = errors.New("posting account changed after this write was prepared")
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
	Status                  string
	TransactionKind         string
	InvestmentOperationKind string
	TransactionDate         string
	PayeeID                 sql.NullInt64
	PayeeName               sql.NullString
	Description             string
	ExternalRefHint         string
	NoteMarkdown            string
	MetadataJSON            string
	NeedsReview             bool
	TagIDs                  []int64
	JournalEntries          []JournalEntrySpec
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
	// EntryDateBasis narrows by journal-entry date the way the reports do:
	// a transaction matches when one of its entries falls inside the range,
	// rather than by the transaction's own date. FilterEntryDate is the
	// register's coarser MAX(entry_date) variant and stays separate.
	EntryDateBasis bool
	CategoryType   string
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
	// CheckpointCandidates are the positions this write touches; the write
	// transaction resolves them into the checkpoints that must be invalidated
	// and applies the override guard there (T-94). Callers pass candidates, not
	// resolved refs, because a ref list resolved before BeginTx is a statement
	// about checkpoint state at a moment that has already passed.
	CheckpointCandidates       []PeriodScopedCheckpointRef
	ReconciliationOverride     bool
	InvalidateCheckpointReason string
	// AccountRuleDependencies are the accounts whose versions the posting
	// checks in Spec were decided against. The write transaction refuses the
	// spec if any of them has been restructured since (T-100).
	AccountRuleDependencies []AccountRuleDependency
	InvestmentComponents    []InvestmentComponentSpec
	TradeImpliedPrice       *TradeImpliedPriceSpec
}

// InvestmentComponentSpec is an exact source fact, signed from the owner's
// perspective. Legacy trades supply net settlement with unknown gross.
type InvestmentComponentSpec struct {
	Kind         string
	CommodityID  int64
	AmountValue  string
	AmountScale  int
	AmountDate   string
	GrossUnknown bool
}

type TradeImpliedPriceSpec struct {
	BaseCommodityID  int64
	QuoteCommodityID int64
	PriceValue       int64
	PriceScale       int
	ValuationDate    string
	Approximate      bool
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
	// CheckpointCandidates are the positions this write touches; the write
	// transaction resolves them into the checkpoints that must be invalidated
	// and applies the override guard there (T-94). Callers pass candidates, not
	// resolved refs, because a ref list resolved before BeginTx is a statement
	// about checkpoint state at a moment that has already passed.
	CheckpointCandidates       []PeriodScopedCheckpointRef
	ReconciliationOverride     bool
	InvalidateCheckpointReason string
	// ExpectedVersionID is the transaction version this write was prepared
	// against. The write transaction refuses to apply the spec if the
	// transaction has moved on since (T-94); it is required, because a spec
	// prepared against no particular version cannot be checked at all.
	ExpectedVersionID int64
	// AccountRuleDependencies are the accounts whose versions the posting
	// checks in Spec were decided against. The write transaction refuses the
	// spec if any of them has been restructured since (T-100).
	AccountRuleDependencies []AccountRuleDependency
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
	// CheckpointCandidates are the positions this write touches; the write
	// transaction resolves them into the checkpoints that must be invalidated
	// and applies the override guard there (T-94). Callers pass candidates, not
	// resolved refs, because a ref list resolved before BeginTx is a statement
	// about checkpoint state at a moment that has already passed.
	CheckpointCandidates       []PeriodScopedCheckpointRef
	ReconciliationOverride     bool
	InvalidateCheckpointReason string
	// ExpectedVersionID is the transaction version whose positions the
	// candidates above were derived from. See UpdateTransactionParams (T-94).
	ExpectedVersionID int64
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

// AccountRuleDependency names an account a prepared write's posting checks
// depend on, together with the account's newest version at the moment those
// checks were made. The financial write re-reads the same number inside its own
// database transaction and refuses the write if it has moved (T-100).
//
// The dependency is account-wide rather than as-of-the-entry-date on purpose.
// The rule it has to interlock with — "an account's structure may not change
// once it has postings" — is itself account-wide, so a per-date dependency
// would leave a gap: an edit effective in February could be admitted while an
// unrelated January posting was in flight, and neither side would see the
// other. Comparing the account's newest version makes the two rules exclude
// each other in both commit orders.
type AccountRuleDependency struct {
	AccountID int64
	// LatestVersionID is the account's highest account_versions.id at
	// preparation time, used as an opaque change token for the account's whole
	// version history.
	LatestVersionID int64
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
	AccountKind           string
	// VersionID is the account version this rule was resolved from — the one
	// effective on the entry date.
	VersionID int64
	// LatestVersionID is the account's newest version, which may be later than
	// VersionID when the account has edits effective after the entry date. It
	// is the change token carried in AccountRuleDependency (T-100).
	LatestVersionID int64
	// BaseKind is the account kind's family from the account_kinds table
	// (e.g. both security_holding and fund_holding have base_kind
	// "security_holding"). Rules that apply to a family of kinds key off this
	// rather than enumerating codes, so a kind added to the table later is
	// covered without a second edit somewhere else.
	BaseKind string
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
