package app

import (
	"errors"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

var (
	ErrTransactionNotFound            = errors.New("transaction not found")
	ErrTransactionProtected           = errors.New("transaction requires corrective workflow")
	ErrTransactionPosted              = errors.New("posted or voided transaction cannot be deleted")
	ErrTransactionVoided              = errors.New("voided transaction cannot be edited")
	ErrTransactionDeleted             = errors.New("soft-deleted transaction must be restored first")
	ErrTransactionTag                 = errors.New("transaction tag is invalid")
	ErrReconciliationOverrideRequired = errors.New("reconciliation override is required")
	ErrReconciliationNotFound         = errors.New("reconciliation not found")
	ErrReconciliationClosed           = errors.New("reconciliation is not open")
	ErrReconciliationNotBalanced      = errors.New("reconciliation difference must be zero")
	ErrReconciliationPosting          = errors.New("reconciliation posting is invalid")
)

type Transaction struct {
	ID                        int64
	BookID                    int64
	CorrectionOfTransactionID *int64
	Status                    string
	TransactionKind           string
	TransactionDate           string
	TransactionDaySequence    int64
	PayeeID                   *int64
	PayeeName                 string
	Description               string
	ExternalRefHint           string
	NoteMarkdown              string
	MetadataJSON              string
	NeedsReview               bool
	VersionID                 int64
	VersionSeq                int64
	SupersedesVersionID       *int64
	TagIDs                    []int64
	JournalEntries            []JournalEntry
	CreatedAt                 string
	UpdatedAt                 string
	DeletedAt                 string
	ChangeReason              string
	InvalidatedCheckpointIDs  []int64
}

type JournalEntry struct {
	ID           int64
	EntrySeq     int64
	EntryDate    string
	EntryKind    string
	Memo         string
	MetadataJSON string
	Postings     []Posting
}

type Posting struct {
	ID                   int64
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
	ClearedOn            string
	MetadataJSON         string
	TagIDs               []int64

	// Enriched fields — populated by enrichPostings; nil means system account has no user-visible name.
	AccountName       *string
	AccountCode       *string
	AccountSystemRole *string
	AccountBuiltinKey *string // non-nil for built-in/starter category accounts
	AccountClass      string  // asset | liability | income | expense | equity
	CommodityCode     string  // "USD", "EUR", "AAPL"
	CommoditySymbol   *string // "$", "€"; nil when no symbol is set
}

type AccountRegisterEntry struct {
	TransactionID             int64
	BookID                    int64
	CorrectionOfTransactionID *int64
	Status                    string
	TransactionKind           string
	TransactionDate           string
	PayeeID                   *int64
	PayeeName                 string
	Description               string
	ExternalRefHint           string
	NeedsReview               bool
	VersionID                 int64
	VersionSeq                int64
	SupersedesVersionID       *int64
	TransactionTagIDs         []int64
	JournalEntryID            int64
	EntrySeq                  int64
	EntryDate                 string
	EntryKind                 string
	EntryMemo                 string
	Posting                   Posting
	RunningBalance            BalanceQuantity
	CreatedAt                 string
	UpdatedAt                 string
	ChangeReason              string
}

type ListTransactionsInput struct {
	AccountID   int64
	CategoryID  int64
	PayeeID     int64
	Status      string
	Kind        string
	NeedsReview bool
	Query       string
	AfterDate   string
	BeforeDate  string
	Limit       int
	Cursor      string
}

type ListTransactionsResult struct {
	Transactions []Transaction
	NextCursor   string
}

type AccountRegisterResult struct {
	Entries    []AccountRegisterEntry
	NextCursor string
}

type CreateTransactionInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	OriginType                string
	Operation                 string
	CorrectionOfTransactionID *int64
	Spec                      TransactionInput
	ChangeReason              string
	ReconciliationOverride    bool
}

type UpdateTransactionInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	OriginType             string
	Operation              string
	TransactionID          int64
	Spec                   TransactionInput
	ChangeReason           string
	ReconciliationOverride bool
	AllowDraftPromotion    bool
}

type PostTransactionInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	TransactionID int64
	ChangeReason  string
}

type VoidTransactionInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	OriginType             string
	TransactionID          int64
	ChangeReason           string
	ReconciliationOverride bool
}

type TransactionLifecycleInput = VoidTransactionInput

type DeleteDraftTransactionInput struct {
	TransactionID int64
}

type TransactionInput struct {
	Status          string
	TransactionKind string
	TransactionDate string
	PayeeID         *int64
	PayeeName       string
	Description     string
	ExternalRefHint string
	NoteMarkdown    string
	MetadataJSON    string
	NeedsReview     bool
	TagIDs          []int64
	JournalEntries  []JournalEntryInput
}

type JournalEntryInput struct {
	EntryDate    string
	EntryKind    string
	Memo         string
	MetadataJSON string
	Postings     []PostingInput
}

type PostingInput struct {
	LineKey       string
	AccountID     int64
	QuantityValue exact.Coefficient
	QuantityScale int
	CommodityID   int64
	Memo          string
	MetadataJSON  string
	TagIDs        []int64
}

type ApproveTransactionInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	TransactionID int64
	ChangeReason  string
}

type MoveTransactionInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	TransactionID int64
	Direction     string // "earlier" | "later"
}

type MovePostingInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	AccountID     int64
	PostingLineID int64
	Direction     string // "earlier" | "later"
}

type ReconciliationImpact struct {
	// AffectedCheckpoints holds refs for each checkpoint that would be
	// invalidated by this create or update.
	AffectedCheckpoints []db.CheckpointInvalidationRef
}

type CreateReconciliationImpactInput struct {
	OwnerUserID int64
	Spec        TransactionInput
}

type UpdateReconciliationImpactInput struct {
	OwnerUserID   int64
	TransactionID int64
	Spec          TransactionInput
}

type cleanTransactionOptions struct {
	DefaultStatus    string
	ExistingLineKeys map[string]bool
	ExistingPostings map[string]existingPostingState
}

type existingPostingState struct {
	ReconciliationStatus string
	ClearedOn            string
}
