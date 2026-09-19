package app

import (
	"errors"
	"sort"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

var (
	ErrTransactionNotFound  = errors.New("transaction not found")
	ErrTransactionProtected = errors.New("transaction requires corrective workflow")
	ErrTransactionPosted    = errors.New("posted or voided transaction cannot be deleted")
	ErrTransactionVoided    = errors.New("voided transaction cannot be edited")
	// ErrTransactionVersionStale reports an edit prepared against a transaction
	// version another write has already replaced. The caller has to re-read and
	// decide again; retrying the same body would reapply facts the user never
	// saw (T-94).
	ErrTransactionVersionStale = errors.New("transaction changed after this edit was prepared")
	// ErrPostingAccountVersionStale reports a write whose posting checks were
	// decided against an account that has been restructured since (T-100).
	// Like ErrTransactionVersionStale it is a retry-after-reload conflict, not
	// a rejection of what the caller asked for.
	ErrPostingAccountVersionStale     = errors.New("posting account changed after this write was prepared")
	ErrTransactionDeleted             = errors.New("soft-deleted transaction must be restored first")
	ErrTransactionDraftNotVoidable    = errors.New("draft transaction cannot be voided; post or delete it instead")
	ErrInvestmentWorkflowRequired     = errors.New("investment-linked transaction requires an investment workflow")
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
	// CategoryType restricts to transactions touching an income or expense
	// category, matching the spending report's direction.
	CategoryType string
	// DateBasis selects which date the range applies to: "transaction"
	// (default) or "entry", the basis the reports sum on.
	DateBasis string
	Limit     int
	Cursor    string
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
	// ExpectedVersionID is the transaction version the caller composed Spec
	// from. It is how a caller that read the transaction itself — PostTransaction
	// builds the promotion spec out of its own read — binds that spec to the
	// version it came from: this method reads the transaction a second time,
	// and without the binding the write would be checked against a version
	// nobody prepared anything against (T-94). Callers that hand over a spec
	// they did not derive from a prior read may leave it zero; promotion may
	// not.
	ExpectedVersionID int64
}

type PostTransactionInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	OriginType             string
	TransactionID          int64
	ChangeReason           string
	ReconciliationOverride bool
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
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
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
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	OriginType             string
	AccountID              int64
	PostingLineID          int64
	Direction              string // "earlier" | "later"
	ReconciliationOverride bool
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

// DeletedTransaction is a Transaction with soft-delete snapshot fields and
// a pre-computed restore guard flag.
type DeletedTransaction struct {
	Transaction
	DeleteReason                   string
	RestoreBlockedByReconciliation bool
}

type ListDeletedTransactionsInput struct {
	Limit  int
	Cursor string
}

type ListDeletedTransactionsResult struct {
	Transactions []DeletedTransaction
	NextCursor   string
}

type UpdateReconciliationImpactInput struct {
	OwnerUserID   int64
	TransactionID int64
	Spec          TransactionInput
}

type cleanTransactionOptions struct {
	DefaultStatus string
	// ForcedStatus, when non-empty, is the status that will actually be
	// persisted regardless of what the request body's Status field says — used
	// by the update path, where the transaction's current lifecycle state (not
	// the request) decides whether the write stays "draft" or becomes
	// "posted". Balance validation always runs against this final status, not
	// whatever the caller claimed, so a request cannot dodge validateBalanced
	// by naming a different status than the one that will actually land.
	ForcedStatus     string
	ExistingLineKeys map[string]bool
	ExistingPostings map[string]existingPostingState
	// AllowSubledgerManagedPostings lifts the guard that keeps generic
	// transaction writes out of accounts the investment subledger owns (T-96).
	// Only InvestmentService sets it, through
	// prepareInvestmentTransactionForWrite. It lives on this unexported options
	// struct rather than on CreateTransactionInput so that the exemption cannot
	// be reached from the API layer by populating a request field.
	AllowSubledgerManagedPostings bool
	// AccountRuleDependencies, when non-nil, collects the accounts whose
	// versions this spec's posting checks were decided against, so the write
	// can refuse the spec if one of them is restructured before it commits
	// (T-100). Only the paths that go on to write set it; previews and
	// template validation leave it nil.
	AccountRuleDependencies *accountRuleDependencies
}

// accountRuleDependencies accumulates, while a spec is being cleaned, the
// account version each posting check was decided against. One entry per
// account: every check on the same account reads the same version, so a second
// sighting is either identical or a bug in the caller's ordering.
type accountRuleDependencies struct {
	latestVersionByAccount map[int64]int64
}

func newAccountRuleDependencies() *accountRuleDependencies {
	return &accountRuleDependencies{latestVersionByAccount: map[int64]int64{}}
}

// observe records the version a check read, keeping the *first* sighting of
// each account. The earliest read is the one at risk: an investment command
// decides an account's role while planning and only reads the same account
// again while preparing the journal, so recording the later read would hand
// the write a version nothing was decided against and the guard would compare
// the account to itself (T-100). Keeping the first means a version that
// appeared between the two reads fails the write, which is the point.
func (d *accountRuleDependencies) observe(rule db.PostingAccountRule) {
	if d == nil || rule.AccountID <= 0 {
		return
	}
	if _, seen := d.latestVersionByAccount[rule.AccountID]; seen {
		return
	}
	d.latestVersionByAccount[rule.AccountID] = rule.LatestVersionID
}

func (d *accountRuleDependencies) list() []db.AccountRuleDependency {
	if d == nil {
		return nil
	}
	dependencies := make([]db.AccountRuleDependency, 0, len(d.latestVersionByAccount))
	for accountID, latestVersionID := range d.latestVersionByAccount {
		dependencies = append(dependencies, db.AccountRuleDependency{AccountID: accountID, LatestVersionID: latestVersionID})
	}
	sort.Slice(dependencies, func(i, j int) bool { return dependencies[i].AccountID < dependencies[j].AccountID })
	return dependencies
}

type existingPostingState struct {
	ReconciliationStatus string
	ClearedOn            string
}
