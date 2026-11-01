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
	// ErrReconciliationImpactStale refuses an override reviewed against a
	// checkpoint set other than the one the write would invalidate now (#296).
	ErrReconciliationImpactStale = errors.New("the checkpoints this write would invalidate changed after they were reviewed")
	ErrReconciliationNotFound    = errors.New("reconciliation not found")
	ErrReconciliationClosed      = errors.New("reconciliation is not open")
	ErrReconciliationNotBalanced = errors.New("reconciliation difference must be zero")
	ErrReconciliationPosting     = errors.New("reconciliation posting is invalid")
	ErrReconciliationStartStale  = errors.New("reconciliation starting balance is no longer current")
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
	// SystemLabel is a stable code for a journal the system posted without
	// user text, such as "split_adjustment" (T-136), or "share_exchange" or
	// "spin_off" for that operation's journal (#178, #180); empty otherwise.
	SystemLabel              string
	InvalidatedCheckpointIDs []int64
}

type JournalEntry struct {
	ID           int64
	EntrySeq     int64
	EntryDate    string
	EntryKind    string
	Memo         string
	MetadataJSON string
	// ConversionSide and FeePaidBy are a transfer's recorded choices (#481),
	// "sending", "receiving" or empty; see JournalEntryInput.
	ConversionSide string
	FeePaidBy      string
	Postings       []Posting
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
	SystemLabel               string
	// CorrectionChain is set when the row's transaction belongs to a
	// correction chain of more than one transaction (T-120 #135).
	CorrectionChain *RegisterCorrectionChain
}

type ListTransactionsInput struct {
	AccountID  int64
	CategoryID int64
	// CommodityID includes postings in that commodity and cash-dividend or
	// withholding operation facts naming it as payer, including corrections (#229).
	CommodityID int64
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
	// ReconciliationOverrideCheckpointIDs, when non-nil, is the checkpoint set
	// the owner reviewed for the override; the write refuses a different set
	// (#296). Nil leaves an internal caller's override unbound.
	ReconciliationOverrideCheckpointIDs []int64
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
	// ReconciliationOverrideCheckpointIDs, when non-nil, is the checkpoint set
	// the owner reviewed for the override; the write refuses a different set
	// (#296). Nil leaves an internal caller's override unbound.
	ReconciliationOverrideCheckpointIDs []int64
	AllowDraftPromotion                 bool
	// ExpectedVersionID is the transaction version the caller composed Spec
	// from. It is how a caller that read the transaction itself — PostTransaction
	// builds the promotion spec out of its own read — binds that spec to the
	// version it came from: this method reads the transaction a second time,
	// and without the binding the write would be checked against a version
	// nobody prepared anything against (T-94). Callers that hand over a spec
	// they did not derive from a prior read may leave it zero; promotion may
	// not.
	ExpectedVersionID int64
	// KeepJournal writes the version's stored journal unchanged with Spec's
	// descriptive fields (#481): the edit of a journal no editor can restate
	// exactly. Spec then names no journal entries.
	KeepJournal bool
}

type PostTransactionInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	OriginType             string
	TransactionID          int64
	ChangeReason           string
	ReconciliationOverride bool
	// ReconciliationOverrideCheckpointIDs, when non-nil, is the checkpoint set
	// the owner reviewed for the override; the write refuses a different set
	// (#296). Nil leaves an internal caller's override unbound.
	ReconciliationOverrideCheckpointIDs []int64
	// ExpectedVersionID, when positive, is the version the caller reviewed;
	// any other current version refuses the write as stale (#296).
	ExpectedVersionID int64
}

type VoidTransactionInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	OriginType             string
	TransactionID          int64
	ChangeReason           string
	ReconciliationOverride bool
	// ReconciliationOverrideCheckpointIDs, when non-nil, is the checkpoint set
	// the owner reviewed for the override; the write refuses a different set
	// (#296). Nil leaves an internal caller's override unbound.
	ReconciliationOverrideCheckpointIDs []int64
	// ExpectedVersionID, when positive, is the version the caller reviewed;
	// any other current version refuses the write as stale (#296).
	ExpectedVersionID int64
}

type TransactionLifecycleInput = VoidTransactionInput

// CheckpointImpactError is a guard refusal naming the checkpoints the write
// would invalidate now; it unwraps to ErrReconciliationOverrideRequired or
// ErrReconciliationImpactStale (#296).
type CheckpointImpactError = db.CheckpointImpactError

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
	// InvestmentOperationKind names the investment-domain command. Generic
	// transaction requests leave it empty; investment services set it.
	InvestmentOperationKind string
	TransactionDate         string
	PayeeID                 *int64
	PayeeName               string
	Description             string
	ExternalRefHint         string
	NoteMarkdown            string
	MetadataJSON            string
	NeedsReview             bool
	TagIDs                  []int64
	JournalEntries          []JournalEntryInput
	// storedJournal marks JournalEntries as a stored journal restated as it
	// is (storedTransactionInput): its exchange legs and clearing
	// bridges are already built, so the writer validates it without building
	// them again. Unexported, so no request can claim it.
	storedJournal bool
}

type JournalEntryInput struct {
	EntryDate    string
	EntryKind    string
	Memo         string
	MetadataJSON string
	// ConversionSide says which side of a currency exchange converted (#481):
	// "sending" (the default, empty) or "receiving". The exchange legs post on
	// that side's day, and the choice is stored, so it survives an edit or a
	// moved day even when both sides post on one day.
	ConversionSide string
	// FeePaidBy says which side of a transfer between two accounts paid a fee
	// charged on top (#481): "sending" or "receiving". Every posting that is
	// neither an account's nor a system one is then that side's fee, in its
	// currency and on its day. Stored like ConversionSide; empty is none.
	FeePaidBy string
	Postings  []PostingInput
}

type PostingInput struct {
	LineKey string
	// PostedOn is the day this account posting appears on its statement when
	// it differs from the entry date (#187). The service turns it into dated
	// journal entries; it is never stored on the posting.
	PostedOn      string
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

type PlaceTransactionInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	OriginType             string
	TransactionID          int64
	AnchorTransactionID    int64
	Position               string // "before" | "after"
	ReconciliationOverride bool
	// ReconciliationOverrideCheckpointIDs, when non-nil, is the checkpoint set
	// the owner reviewed for the override; the write refuses a different set
	// (#296). Nil leaves an internal caller's override unbound.
	ReconciliationOverrideCheckpointIDs []int64
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
	// ReconciliationOverrideCheckpointIDs, when non-nil, is the checkpoint set
	// the owner reviewed for the override; the write refuses a different set
	// (#296). Nil leaves an internal caller's override unbound.
	ReconciliationOverrideCheckpointIDs []int64
}

type ReconciliationImpact struct {
	// AffectedCheckpoints holds refs for each checkpoint that would be
	// invalidated by this create or update.
	AffectedCheckpoints []db.CheckpointInvalidationRef
	// GainImpact lists committed disposals whose operational basis or gain
	// the command would change, with the token that acknowledges exactly that
	// set. Nil means the command has not opted into gain disclosure (T-126).
	GainImpact *db.InvestmentGainImpact
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
	// ExpectedVersionID, when positive, is the version the form was composed
	// against; a preview of a newer transaction is refused as stale (#296).
	ExpectedVersionID int64
	// KeepJournal previews an edit that keeps the stored journal (#481).
	KeepJournal bool
}

type cleanTransactionOptions struct {
	// OwnerUserID resolves a blank transaction date; explicit transaction or
	// journal-entry dates are independent of owner preferences.
	OwnerUserID   int64
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
	// WriterGuardsCheckpoints marks a journal headed for the investment
	// writer, whose combined guard nets every journal of the command at each
	// checkpoint boundary (T-120 #135). Rejecting the journal alone up front
	// would refuse an inverse that its replacement cancels.
	WriterGuardsCheckpoints bool
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

// existingPostingState is what a write carries forward for a posting line it
// keeps. EntryDate and Posting are the facts the stored status was given for:
// a write that changes them resets the status (#297).
type existingPostingState struct {
	ReconciliationStatus string
	ClearedOn            string
	EntryDate            string
	Posting              db.PostingRecord
}
