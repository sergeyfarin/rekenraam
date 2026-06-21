package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

const (
	transactionTextMaxBytes = 500
	transactionNoteMaxBytes = 10000
	ledgerJSONMaxBytes      = 10000
	transactionListLimit    = 100
	transactionMaxLimit     = 500
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

// transactionStatuses is the persisted transaction lifecycle. An unsaved entry
// (an in-progress UI working copy with no row) is deliberately NOT a status: it
// has no database row and triggers no side effects until it is saved as a draft
// or posted transaction. "draft" here always means a persisted, durable draft
// (autosave, scheduled generation, committed import awaiting review) excluded
// from the ledger but able to trigger background work such as FX coverage —
// never the unsaved working copy. Soft-delete is the separate deleted_at flag,
// not a status value. See docs/transaction-ledger-core-plan.md.
var transactionStatuses = map[string]bool{
	"draft":  true,
	"posted": true,
	"voided": true,
}

var transactionKinds = map[string]bool{
	"ordinary":        true,
	"transfer":        true,
	"investment":      true,
	"opening_balance": true,
	"adjustment":      true,
}

var entryKinds = map[string]bool{
	"ordinary":        true,
	"transfer_leg":    true,
	"exchange":        true,
	"investment":      true,
	"opening_balance": true,
	"adjustment":      true,
}

var reconciliationStatuses = map[string]bool{
	"uncleared":  true,
	"cleared":    true,
	"reconciled": true,
}

type Transaction struct {
	ID                        int64
	BookID                    int64
	CorrectionOfTransactionID *int64
	Status                    string
	TransactionKind           string
	TransactionDate           string
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

type TransactionService struct {
	repository          *db.TransactionRepository
	payeeRepository     *db.PayeeRepository
	accountRepository   *db.AccountRepository
	commodityRepository *db.CommodityRepository
	now                 func() time.Time
	newLineKey          func() string
}

func NewTransactionService(
	repository *db.TransactionRepository,
	payeeRepository *db.PayeeRepository,
	accountRepository *db.AccountRepository,
	commodityRepository *db.CommodityRepository,
) *TransactionService {
	return &TransactionService{
		repository:          repository,
		payeeRepository:     payeeRepository,
		accountRepository:   accountRepository,
		commodityRepository: commodityRepository,
		now:                 time.Now,
		newLineKey:          uuid.NewString,
	}
}

func (s *TransactionService) ListTransactions(ctx context.Context, input ListTransactionsInput) (ListTransactionsResult, error) {
	if input.CategoryID > 0 {
		accountMap, err := s.accountRepository.AccountsByIDs(ctx, BookID, []int64{input.CategoryID})
		if err != nil {
			return ListTransactionsResult{}, fmt.Errorf("look up category account: %w", err)
		}
		acct, ok := accountMap[input.CategoryID]
		if !ok {
			return ListTransactionsResult{}, ValidationError{Message: "category account not found"}
		}
		if acct.AccountClass != "income" && acct.AccountClass != "expense" {
			return ListTransactionsResult{}, ValidationError{Message: "category_id must refer to an income or expense account"}
		}
	}

	params, err := s.listParams(input, false)
	if err != nil {
		return ListTransactionsResult{}, err
	}

	records, err := s.repository.ListTransactions(ctx, params)
	if err != nil {
		return ListTransactionsResult{}, fmt.Errorf("list transactions: %w", err)
	}

	nextCursor := ""
	if len(records) == params.Limit {
		last := records[len(records)-1]
		nextCursor = db.EncodeTransactionCursor(last.TransactionDate, last.ID)
	}

	txns := toTransactions(records)
	if err := s.enrichPostings(ctx, txns); err != nil {
		return ListTransactionsResult{}, err
	}

	return ListTransactionsResult{
		Transactions: txns,
		NextCursor:   nextCursor,
	}, nil
}

func (s *TransactionService) Register(ctx context.Context, accountID int64, input ListTransactionsInput) (AccountRegisterResult, error) {
	if accountID <= 0 {
		return AccountRegisterResult{}, ValidationError{Message: "account id is required"}
	}
	input.AccountID = accountID
	if strings.TrimSpace(input.Status) == "" {
		input.Status = "posted"
	}

	params, err := s.listParams(input, true)
	if err != nil {
		return AccountRegisterResult{}, err
	}

	records, err := s.repository.AccountRegister(ctx, params)
	if err != nil {
		return AccountRegisterResult{}, fmt.Errorf("list register: %w", err)
	}
	runningBalances, err := s.accountRegisterRunningBalances(ctx, params, records)
	if err != nil {
		return AccountRegisterResult{}, err
	}

	nextCursor := ""
	if len(records) == params.Limit {
		last := records[len(records)-1]
		nextCursor = db.EncodeTransactionCursor(last.JournalEntry.EntryDate, last.Posting.ID)
	}

	entries := toAccountRegisterEntries(records, runningBalances)
	if err := s.enrichRegisterPostings(ctx, entries); err != nil {
		return AccountRegisterResult{}, err
	}

	return AccountRegisterResult{
		Entries:    entries,
		NextCursor: nextCursor,
	}, nil
}

func (s *TransactionService) Transaction(ctx context.Context, transactionID int64) (Transaction, error) {
	if transactionID <= 0 {
		return Transaction{}, ValidationError{Message: "transaction id is required"}
	}

	record, err := s.repository.TransactionByID(ctx, BookID, transactionID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Transaction{}, ErrTransactionNotFound
		}
		return Transaction{}, fmt.Errorf("read transaction: %w", err)
	}

	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) enrichOne(ctx context.Context, txn Transaction) (Transaction, error) {
	slice := []Transaction{txn}
	if err := s.enrichPostings(ctx, slice); err != nil {
		return Transaction{}, err
	}
	return slice[0], nil
}

func (s *TransactionService) CreateTransaction(ctx context.Context, input CreateTransactionInput) (Transaction, error) {
	params, err := s.prepareCreateTransaction(ctx, input)
	if err != nil {
		return Transaction{}, err
	}

	record, err := s.repository.CreateTransaction(ctx, params)
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}

	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) prepareCreateTransaction(ctx context.Context, input CreateTransactionInput) (db.CreateTransactionParams, error) {
	if input.OwnerUserID <= 0 {
		return db.CreateTransactionParams{}, ValidationError{Message: "owner user is required"}
	}

	now := s.now().UTC()
	spec, err := s.cleanTransactionSpec(ctx, input.Spec, cleanTransactionOptions{DefaultStatus: "posted"})
	if err != nil {
		return db.CreateTransactionParams{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "created transaction")
	if err != nil {
		return db.CreateTransactionParams{}, err
	}
	operation := strings.TrimSpace(input.Operation)
	if operation == "" {
		operation = "transaction.create"
	}

	return db.CreateTransactionParams{
		BookID:                    BookID,
		CorrectionOfTransactionID: nullableInt64(input.CorrectionOfTransactionID),
		ActorUserID:               input.OwnerUserID,
		AuthSessionID:             input.AuthSessionID,
		RequestID:                 input.RequestID,
		OriginType:                input.OriginType,
		Operation:                 operation,
		Spec:                      spec,
		CreatedAt:                 now.Format(time.RFC3339),
		ChangeReason:              changeReason,
	}, nil
}

func (s *TransactionService) UpdateTransaction(ctx context.Context, input UpdateTransactionInput) (Transaction, error) {
	if input.OwnerUserID <= 0 {
		return Transaction{}, ValidationError{Message: "owner user is required"}
	}
	if input.TransactionID <= 0 {
		return Transaction{}, ValidationError{Message: "transaction id is required"}
	}

	current, err := s.repository.TransactionByID(ctx, BookID, input.TransactionID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Transaction{}, ErrTransactionNotFound
		}
		return Transaction{}, fmt.Errorf("read transaction: %w", err)
	}
	if current.Status == "voided" {
		return Transaction{}, ErrTransactionVoided
	}

	defaultStatus := "posted"
	if current.Status == "draft" {
		defaultStatus = "draft"
	}
	spec, err := s.cleanTransactionSpec(ctx, input.Spec, cleanTransactionOptions{
		DefaultStatus:    defaultStatus,
		ExistingLineKeys: lineKeySet(current),
		ExistingPostings: existingPostingStateSet(current),
	})
	if err != nil {
		return Transaction{}, err
	}
	if current.Status == "draft" && !input.AllowDraftPromotion {
		spec.Status = "draft"
	} else if current.Status != "draft" {
		spec.Status = "posted"
	}
	invalidationRefs, err := reconciliationInvalidationRefs(current, spec)
	if err != nil {
		return Transaction{}, err
	}
	if len(invalidationRefs) > 0 && !input.ReconciliationOverride {
		return Transaction{}, ErrReconciliationOverrideRequired
	}

	now := s.now().UTC()
	changeReason, err := cleanChangeReason(input.ChangeReason, "updated transaction")
	if err != nil {
		return Transaction{}, err
	}
	operation := strings.TrimSpace(input.Operation)
	if operation == "" {
		operation = "transaction.update"
	}

	record, err := s.repository.UpdateTransaction(ctx, db.UpdateTransactionParams{
		BookID:                     BookID,
		TransactionID:              input.TransactionID,
		ActorUserID:                input.OwnerUserID,
		AuthSessionID:              input.AuthSessionID,
		RequestID:                  input.RequestID,
		OriginType:                 input.OriginType,
		Operation:                  operation,
		Spec:                       spec,
		RecordedAt:                 now.Format(time.RFC3339),
		ChangeReason:               changeReason,
		InvalidateCheckpointRefs:   invalidationRefs,
		InvalidateCheckpointReason: changeReason,
	})
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}

	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) PostTransaction(ctx context.Context, input PostTransactionInput) (Transaction, error) {
	if input.OwnerUserID <= 0 {
		return Transaction{}, ValidationError{Message: "owner user is required"}
	}
	current, err := s.Transaction(ctx, input.TransactionID)
	if err != nil {
		return Transaction{}, err
	}
	if current.Status == "voided" {
		return Transaction{}, ErrTransactionVoided
	}
	if current.Status == "posted" {
		return current, nil
	}

	spec := transactionInputFromTransaction(current)
	spec.Status = "posted"
	return s.UpdateTransaction(ctx, UpdateTransactionInput{
		OwnerUserID:         input.OwnerUserID,
		AuthSessionID:       input.AuthSessionID,
		RequestID:           input.RequestID,
		OriginType:          input.OriginType,
		Operation:           "transaction.post",
		TransactionID:       input.TransactionID,
		Spec:                spec,
		ChangeReason:        input.ChangeReason,
		AllowDraftPromotion: true,
	})
}

func (s *TransactionService) VoidTransaction(ctx context.Context, input VoidTransactionInput) (Transaction, error) {
	if input.OwnerUserID <= 0 {
		return Transaction{}, ValidationError{Message: "owner user is required"}
	}
	if input.TransactionID <= 0 {
		return Transaction{}, ValidationError{Message: "transaction id is required"}
	}

	now := s.now().UTC()
	changeReason, err := cleanChangeReason(input.ChangeReason, "")
	if err != nil {
		return Transaction{}, err
	}
	if changeReason == "" {
		return Transaction{}, ValidationError{Message: "change reason is required"}
	}
	current, err := s.repository.TransactionByID(ctx, BookID, input.TransactionID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Transaction{}, ErrTransactionNotFound
		}
		return Transaction{}, fmt.Errorf("read transaction: %w", err)
	}
	invalidationRefs := reconciliationRefsFromRecord(current)
	if len(invalidationRefs) > 0 && !input.ReconciliationOverride {
		return Transaction{}, ErrReconciliationOverrideRequired
	}

	record, err := s.repository.VoidTransaction(ctx, db.VoidTransactionParams{
		BookID:                     BookID,
		TransactionID:              input.TransactionID,
		ActorUserID:                input.OwnerUserID,
		AuthSessionID:              input.AuthSessionID,
		RequestID:                  input.RequestID,
		OriginType:                 input.OriginType,
		Operation:                  "transaction.void",
		RecordedAt:                 now.Format(time.RFC3339),
		ChangeReason:               changeReason,
		InvalidateCheckpointRefs:   invalidationRefs,
		InvalidateCheckpointReason: changeReason,
	})
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}

	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) UnvoidTransaction(ctx context.Context, input TransactionLifecycleInput) (Transaction, error) {
	current, changeReason, now, err := s.prepareLifecycleChange(ctx, input, true)
	if err != nil {
		return Transaction{}, err
	}
	if current.DeletedAt != "" {
		return Transaction{}, ErrTransactionDeleted
	}
	if current.Status != "voided" {
		return current, nil
	}
	refs := reconciliationRefsFromTransaction(current)
	if len(refs) > 0 && !input.ReconciliationOverride {
		return Transaction{}, ErrReconciliationOverrideRequired
	}
	record, err := s.repository.UnvoidTransaction(ctx, db.TransactionLifecycleParams{
		BookID: BookID, TransactionID: input.TransactionID, ActorUserID: input.OwnerUserID,
		AuthSessionID: input.AuthSessionID, RequestID: input.RequestID, OriginType: input.OriginType,
		Operation: "transaction.unvoid", RecordedAt: now, ChangeReason: changeReason,
		InvalidateCheckpointRefs: refs, InvalidateCheckpointReason: changeReason,
	})
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}
	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) SoftDeleteTransaction(ctx context.Context, input TransactionLifecycleInput) (Transaction, error) {
	return s.setTransactionDeleted(ctx, input, true)
}

func (s *TransactionService) RestoreTransaction(ctx context.Context, input TransactionLifecycleInput) (Transaction, error) {
	return s.setTransactionDeleted(ctx, input, false)
}

func (s *TransactionService) setTransactionDeleted(ctx context.Context, input TransactionLifecycleInput, deleted bool) (Transaction, error) {
	current, changeReason, now, err := s.prepareLifecycleChange(ctx, input, true)
	if err != nil {
		return Transaction{}, err
	}
	if (current.DeletedAt != "") == deleted {
		return current, nil
	}
	if deleted && current.Status == "draft" {
		return Transaction{}, ErrTransactionPosted
	}
	refs := reconciliationRefsFromTransaction(current)
	if deleted && len(refs) > 0 && !input.ReconciliationOverride {
		return Transaction{}, ErrReconciliationOverrideRequired
	}
	operation := "transaction.restore"
	if deleted {
		operation = "transaction.soft_delete"
	}
	record, err := s.repository.SetTransactionDeleted(ctx, db.SetTransactionDeletedParams{
		TransactionLifecycleParams: db.TransactionLifecycleParams{
			BookID: BookID, TransactionID: input.TransactionID, ActorUserID: input.OwnerUserID,
			AuthSessionID: input.AuthSessionID, RequestID: input.RequestID, OriginType: input.OriginType,
			Operation: operation, RecordedAt: now, ChangeReason: changeReason,
			InvalidateCheckpointRefs: refs, InvalidateCheckpointReason: changeReason,
		}, Deleted: deleted,
	})
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}
	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) prepareLifecycleChange(ctx context.Context, input TransactionLifecycleInput, requireReason bool) (Transaction, string, string, error) {
	if input.OwnerUserID <= 0 {
		return Transaction{}, "", "", ValidationError{Message: "owner user is required"}
	}
	if input.TransactionID <= 0 {
		return Transaction{}, "", "", ValidationError{Message: "transaction id is required"}
	}
	record, err := s.repository.TransactionByIDIncludingDeleted(ctx, BookID, input.TransactionID)
	if err != nil {
		return Transaction{}, "", "", mapTransactionDBError(err)
	}
	reason, err := cleanChangeReason(input.ChangeReason, "")
	if err != nil {
		return Transaction{}, "", "", err
	}
	if requireReason && reason == "" {
		return Transaction{}, "", "", ValidationError{Message: "change reason is required"}
	}
	return toTransaction(record), reason, s.now().UTC().Format(time.RFC3339), nil
}

func (s *TransactionService) DeleteDraftTransaction(ctx context.Context, input DeleteDraftTransactionInput) error {
	if input.TransactionID <= 0 {
		return ValidationError{Message: "transaction id is required"}
	}

	if err := s.repository.DeleteDraftTransaction(ctx, db.DeleteDraftTransactionParams{
		BookID:        BookID,
		TransactionID: input.TransactionID,
	}); err != nil {
		return mapTransactionDBError(err)
	}

	return nil
}

type ApproveTransactionInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	TransactionID int64
	ChangeReason  string
}

func (s *TransactionService) ApproveTransaction(ctx context.Context, input ApproveTransactionInput) (Transaction, error) {
	if input.TransactionID <= 0 {
		return Transaction{}, ValidationError{Message: "transaction id is required"}
	}

	record, err := s.repository.ApproveTransaction(ctx, db.ApproveTransactionParams{
		BookID:        BookID,
		TransactionID: input.TransactionID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    input.OriginType,
		Operation:     "transaction.approve",
		RecordedAt:    s.now().UTC().Format(time.RFC3339Nano),
		ChangeReason:  input.ChangeReason,
	})
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}

	return s.enrichOne(ctx, toTransaction(record))
}

func (s *TransactionService) listParams(input ListTransactionsInput, filterEntryDate bool) (db.ListTransactionsParams, error) {
	status := strings.TrimSpace(input.Status)
	if status != "" && !transactionStatuses[status] {
		return db.ListTransactionsParams{}, ValidationError{Message: "transaction status is invalid"}
	}
	kind := strings.TrimSpace(input.Kind)
	if kind != "" && !transactionKinds[kind] {
		return db.ListTransactionsParams{}, ValidationError{Message: "transaction kind is invalid"}
	}
	afterDate, err := cleanOptionalDate(input.AfterDate, "after date")
	if err != nil {
		return db.ListTransactionsParams{}, err
	}
	beforeDate, err := cleanOptionalDate(input.BeforeDate, "before date")
	if err != nil {
		return db.ListTransactionsParams{}, err
	}
	if afterDate != "" && beforeDate != "" && afterDate > beforeDate {
		return db.ListTransactionsParams{}, ValidationError{Message: "after date must be on or before before date"}
	}

	cursorDate, cursorID, err := db.DecodeTransactionCursor(input.Cursor)
	if err != nil {
		return db.ListTransactionsParams{}, ValidationError{Message: "cursor is invalid"}
	}

	limit := input.Limit
	if limit <= 0 {
		limit = transactionListLimit
	}
	if limit > transactionMaxLimit {
		limit = transactionMaxLimit
	}

	return db.ListTransactionsParams{
		BookID:          BookID,
		AccountID:       input.AccountID,
		CategoryID:      input.CategoryID,
		PayeeID:         input.PayeeID,
		Status:          status,
		Kind:            kind,
		NeedsReview:     input.NeedsReview,
		Query:           strings.TrimSpace(input.Query),
		AfterDate:       afterDate,
		BeforeDate:      beforeDate,
		CursorDate:      cursorDate,
		CursorID:        cursorID,
		Limit:           limit,
		FilterEntryDate: filterEntryDate,
	}, nil
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

func (s *TransactionService) cleanTransactionSpec(ctx context.Context, input TransactionInput, options cleanTransactionOptions) (db.TransactionSpec, error) {
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = options.DefaultStatus
	}
	if status == "" {
		status = "posted"
	}
	if !transactionStatuses[status] || status == "voided" {
		return db.TransactionSpec{}, ValidationError{Message: "transaction status is invalid"}
	}

	kind := strings.TrimSpace(input.TransactionKind)
	if kind == "" {
		kind = "ordinary"
	}
	if !transactionKinds[kind] {
		return db.TransactionSpec{}, ValidationError{Message: "transaction kind is invalid"}
	}

	transactionDate := strings.TrimSpace(input.TransactionDate)
	if transactionDate == "" && len(input.JournalEntries) > 0 {
		transactionDate = strings.TrimSpace(input.JournalEntries[0].EntryDate)
	}
	if transactionDate == "" {
		transactionDate = s.now().UTC().Format(time.DateOnly)
	}
	transactionDate, err := cleanRequiredDate(transactionDate, "transaction date")
	if err != nil {
		return db.TransactionSpec{}, err
	}

	payeeID := nullableInt64(input.PayeeID)
	payeeName := strings.TrimSpace(input.PayeeName)
	if input.PayeeID != nil {
		payee, err := s.payeeRepository.PayeeByID(ctx, BookID, *input.PayeeID)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return db.TransactionSpec{}, ValidationError{Message: "payee is invalid"}
			}
			return db.TransactionSpec{}, fmt.Errorf("read payee: %w", err)
		}
		if payee.Status != "active" {
			return db.TransactionSpec{}, ValidationError{Message: "payee is archived"}
		}
		payeeName = payee.Name
	}
	cleanedPayeeName, err := cleanOptionalText(payeeName, "payee name", payeeNameMaxBytes)
	if err != nil {
		return db.TransactionSpec{}, err
	}

	description, err := cleanOptionalText(input.Description, "description", transactionTextMaxBytes)
	if err != nil {
		return db.TransactionSpec{}, err
	}
	externalRefHint, err := cleanOptionalText(input.ExternalRefHint, "external reference", transactionTextMaxBytes)
	if err != nil {
		return db.TransactionSpec{}, err
	}
	noteMarkdown, err := cleanOptionalText(input.NoteMarkdown, "note", transactionNoteMaxBytes)
	if err != nil {
		return db.TransactionSpec{}, err
	}
	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "metadata", ledgerJSONMaxBytes)
	if err != nil {
		return db.TransactionSpec{}, err
	}

	tagIDs, err := s.cleanTagIDs(ctx, input.TagIDs)
	if err != nil {
		return db.TransactionSpec{}, err
	}

	entries := make([]db.JournalEntrySpec, 0, len(input.JournalEntries))
	for entryIndex, entryInput := range input.JournalEntries {
		entry, err := s.cleanJournalEntry(ctx, entryInput, transactionDate, status, options.ExistingLineKeys, options.ExistingPostings)
		if err != nil {
			return db.TransactionSpec{}, err
		}
		if len(entry.Postings) < 2 {
			return db.TransactionSpec{}, ValidationError{Message: fmt.Sprintf("journal entry %d must have at least two postings", entryIndex+1)}
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return db.TransactionSpec{}, ValidationError{Message: "transaction requires at least one journal entry"}
	}
	if status == "posted" {
		if err := validateBalanced(entries); err != nil {
			return db.TransactionSpec{}, err
		}
	}

	return db.TransactionSpec{
		Status:          status,
		TransactionKind: kind,
		TransactionDate: transactionDate,
		PayeeID:         payeeID,
		PayeeName:       nullableSQLString(cleanedPayeeName),
		Description:     description,
		ExternalRefHint: externalRefHint,
		NoteMarkdown:    noteMarkdown,
		MetadataJSON:    metadataJSON,
		NeedsReview:     input.NeedsReview,
		TagIDs:          tagIDs,
		JournalEntries:  entries,
	}, nil
}

func (s *TransactionService) cleanJournalEntry(ctx context.Context, input JournalEntryInput, transactionDate string, status string, existingLineKeys map[string]bool, existingPostings map[string]existingPostingState) (db.JournalEntrySpec, error) {
	entryDate := strings.TrimSpace(input.EntryDate)
	if entryDate == "" {
		entryDate = transactionDate
	}
	entryDate, err := cleanRequiredDate(entryDate, "entry date")
	if err != nil {
		return db.JournalEntrySpec{}, err
	}
	entryKind := strings.TrimSpace(input.EntryKind)
	if entryKind == "" {
		entryKind = "ordinary"
	}
	if !entryKinds[entryKind] {
		return db.JournalEntrySpec{}, ValidationError{Message: "entry kind is invalid"}
	}
	memo, err := cleanOptionalText(input.Memo, "memo", transactionTextMaxBytes)
	if err != nil {
		return db.JournalEntrySpec{}, err
	}
	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "entry metadata", ledgerJSONMaxBytes)
	if err != nil {
		return db.JournalEntrySpec{}, err
	}

	postings := make([]db.PostingSpec, 0, len(input.Postings))
	for _, postingInput := range input.Postings {
		posting, err := s.cleanPosting(ctx, postingInput, entryDate, status, existingLineKeys, existingPostings)
		if err != nil {
			return db.JournalEntrySpec{}, err
		}
		postings = append(postings, posting)
	}

	return db.JournalEntrySpec{
		EntryDate:    entryDate,
		EntryKind:    entryKind,
		Memo:         memo,
		MetadataJSON: metadataJSON,
		Postings:     postings,
	}, nil
}

func (s *TransactionService) cleanPosting(ctx context.Context, input PostingInput, entryDate string, status string, existingLineKeys map[string]bool, existingPostings map[string]existingPostingState) (db.PostingSpec, error) {
	if input.AccountID <= 0 {
		return db.PostingSpec{}, ValidationError{Message: "posting account is required"}
	}
	if input.CommodityID <= 0 {
		return db.PostingSpec{}, ValidationError{Message: "posting commodity is required"}
	}
	if input.QuantityScale < 0 || input.QuantityScale > exact.MaxCryptoScale {
		return db.PostingSpec{}, ValidationError{Message: "posting quantity scale is invalid"}
	}

	accountRule, err := s.repository.PostingAccountRule(ctx, BookID, input.AccountID, entryDate)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.PostingSpec{}, ValidationError{Message: "posting account is invalid"}
		}
		return db.PostingSpec{}, fmt.Errorf("read posting account rule: %w", err)
	}
	if accountRule.Status != "active" || !accountRule.AllowsPostings {
		return db.PostingSpec{}, ValidationError{Message: "posting account is not active for postings"}
	}
	if entryDate < accountRule.OpenedOn {
		return db.PostingSpec{}, ValidationError{Message: "posting date is before account opened date"}
	}
	if accountRule.ClosedOn.Valid && entryDate > accountRule.ClosedOn.String {
		return db.PostingSpec{}, ValidationError{Message: "posting date is after account closed date"}
	}
	if accountRule.DefaultCommodityID.Valid && accountRule.DefaultCommodityID.Int64 != input.CommodityID {
		return db.PostingSpec{}, ValidationError{Message: "posting commodity does not match account default commodity"}
	}
	if accountRule.QuantityScaleOverride.Valid && int64(input.QuantityScale) > accountRule.QuantityScaleOverride.Int64 {
		return db.PostingSpec{}, ValidationError{Message: "posting quantity scale exceeds account precision"}
	}

	commodityRule, err := s.repository.PostingCommodityRule(ctx, BookID, input.CommodityID, entryDate)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.PostingSpec{}, ValidationError{Message: "posting commodity is invalid"}
		}
		return db.PostingSpec{}, fmt.Errorf("read posting commodity rule: %w", err)
	}
	if commodityRule.Status != "active" {
		return db.PostingSpec{}, ValidationError{Message: "posting commodity is archived"}
	}
	if input.QuantityScale > exact.MaxScaleForCommodityKind(commodityRule.CommodityKind) {
		return db.PostingSpec{}, ValidationError{Message: "posting quantity scale exceeds commodity-kind precision"}
	}
	if input.QuantityScale > commodityRule.MaxQuantityScale {
		return db.PostingSpec{}, ValidationError{Message: "posting quantity scale exceeds commodity precision"}
	}

	lineKey := strings.TrimSpace(input.LineKey)
	if lineKey != "" && existingLineKeys != nil && !existingLineKeys[lineKey] {
		return db.PostingSpec{}, ValidationError{Message: "posting line key is invalid"}
	}
	if lineKey == "" {
		lineKey = s.newLineKey()
	}

	existingState, hasExistingState := existingPostings[lineKey]
	reconciliationStatus := "uncleared"
	if hasExistingState {
		reconciliationStatus = existingState.ReconciliationStatus
	}
	if !reconciliationStatuses[reconciliationStatus] {
		return db.PostingSpec{}, ValidationError{Message: "posting reconciliation status is invalid"}
	}
	clearedOn, err := cleanOptionalDate(existingState.ClearedOn, "cleared date")
	if err != nil {
		return db.PostingSpec{}, err
	}
	if clearedOn != "" && reconciliationStatus == "uncleared" {
		return db.PostingSpec{}, ValidationError{Message: "cleared date requires a cleared or reconciled posting"}
	}

	memo, err := cleanOptionalText(input.Memo, "posting memo", transactionTextMaxBytes)
	if err != nil {
		return db.PostingSpec{}, err
	}
	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "posting metadata", ledgerJSONMaxBytes)
	if err != nil {
		return db.PostingSpec{}, err
	}
	tagIDs, err := s.cleanTagIDs(ctx, input.TagIDs)
	if err != nil {
		return db.PostingSpec{}, err
	}

	return db.PostingSpec{
		LineKey:              lineKey,
		AccountID:            input.AccountID,
		QuantityValue:        input.QuantityValue,
		QuantityScale:        input.QuantityScale,
		CommodityID:          input.CommodityID,
		Memo:                 memo,
		ReconciliationStatus: reconciliationStatus,
		ClearedOn:            nullableSQLString(clearedOn),
		MetadataJSON:         metadataJSON,
		TagIDs:               tagIDs,
	}, nil
}

func (s *TransactionService) cleanTagIDs(ctx context.Context, tagIDs []int64) ([]int64, error) {
	if len(tagIDs) == 0 {
		return nil, nil
	}
	seen := make(map[int64]bool, len(tagIDs))
	cleaned := make([]int64, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		if tagID <= 0 {
			return nil, ValidationError{Message: "tag id is invalid"}
		}
		if !seen[tagID] {
			seen[tagID] = true
			cleaned = append(cleaned, tagID)
		}
	}

	active, err := s.repository.ActiveTagIDs(ctx, BookID, cleaned)
	if err != nil {
		return nil, fmt.Errorf("read active tags: %w", err)
	}
	for _, tagID := range cleaned {
		if !active[tagID] {
			return nil, ErrTransactionTag
		}
	}

	return cleaned, nil
}

func validateBalanced(entries []db.JournalEntrySpec) error {
	for entryIndex, entry := range entries {
		sums := map[int64]*big.Int{}
		maxScales := map[int64]int{}
		for _, posting := range entry.Postings {
			if posting.QuantityScale > maxScales[posting.CommodityID] {
				maxScales[posting.CommodityID] = posting.QuantityScale
			}
		}
		for _, posting := range entry.Postings {
			sum := sums[posting.CommodityID]
			if sum == nil {
				sum = big.NewInt(0)
				sums[posting.CommodityID] = sum
			}
			value := posting.QuantityValue.BigInt()
			scaleDiff := maxScales[posting.CommodityID] - posting.QuantityScale
			if scaleDiff > 0 {
				value.Mul(value, pow10(scaleDiff))
			}
			sum.Add(sum, value)
		}
		for _, sum := range sums {
			if sum.Sign() != 0 {
				return ValidationError{Message: fmt.Sprintf("journal entry %d is not balanced by commodity", entryIndex+1)}
			}
		}
	}

	return nil
}

// maxPow10Scale caps the exponent accepted by pow10. No legitimate financial
// quantity or alignment delta should exceed this; a larger value indicates a
// logic bug in the caller, not a data problem.
const maxPow10Scale = 60

func pow10(scale int) *big.Int {
	if scale < 0 || scale > maxPow10Scale {
		panic(fmt.Sprintf("pow10: scale %d out of range [0, %d]", scale, maxPow10Scale))
	}
	result := big.NewInt(1)
	ten := big.NewInt(10)
	for i := 0; i < scale; i++ {
		result.Mul(result, ten)
	}
	return result
}

func cleanRequiredDate(value string, field string) (string, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return "", ValidationError{Message: field + " is required"}
	}
	if _, err := time.Parse(time.DateOnly, cleaned); err != nil {
		return "", ValidationError{Message: field + " must use YYYY-MM-DD"}
	}
	return cleaned, nil
}

func cleanOptionalDate(value string, field string) (string, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return "", nil
	}
	return cleanRequiredDate(cleaned, field)
}

func cleanOptionalText(value string, field string, maxBytes int) (string, error) {
	cleaned := strings.TrimSpace(value)
	if len(cleaned) > maxBytes {
		return "", ValidationError{Message: fmt.Sprintf("%s must be at most %d bytes", field, maxBytes)}
	}
	for _, r := range cleaned {
		if unicode.IsControl(r) {
			return "", ValidationError{Message: field + " must not contain control characters"}
		}
	}
	return cleaned, nil
}

func lineKeySet(record db.TransactionRecord) map[string]bool {
	keys := map[string]bool{}
	for _, entry := range record.JournalEntries {
		for _, posting := range entry.Postings {
			keys[posting.LineKey] = true
		}
	}
	return keys
}

func existingPostingStateSet(record db.TransactionRecord) map[string]existingPostingState {
	states := map[string]existingPostingState{}
	for _, entry := range record.JournalEntries {
		for _, posting := range entry.Postings {
			states[posting.LineKey] = existingPostingState{
				ReconciliationStatus: posting.ReconciliationStatus,
				ClearedOn:            nullableString(posting.ClearedOn),
			}
		}
	}
	return states
}

func reconciliationInvalidationRefs(current db.TransactionRecord, spec db.TransactionSpec) ([]db.CheckpointInvalidationRef, error) {
	currentByLineKey := map[string]struct {
		entryDate string
		posting   db.PostingRecord
	}{}
	for _, entry := range current.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.ReconciliationStatus == "reconciled" {
				currentByLineKey[posting.LineKey] = struct {
					entryDate string
					posting   db.PostingRecord
				}{entryDate: entry.EntryDate, posting: posting}
			}
		}
	}
	if len(currentByLineKey) == 0 {
		return nil, nil
	}

	nextByLineKey := map[string]struct {
		entryDate string
		posting   db.PostingSpec
	}{}
	for _, entry := range spec.JournalEntries {
		for _, posting := range entry.Postings {
			nextByLineKey[posting.LineKey] = struct {
				entryDate string
				posting   db.PostingSpec
			}{entryDate: entry.EntryDate, posting: posting}
		}
	}

	refs := make([]db.CheckpointInvalidationRef, 0)
	for lineKey, currentPosting := range currentByLineKey {
		nextPosting, ok := nextByLineKey[lineKey]
		if !ok || reconciliationAffectingPostingChange(currentPosting.entryDate, currentPosting.posting, nextPosting.entryDate, nextPosting.posting) {
			refs = append(refs, db.CheckpointInvalidationRef{
				AccountID:   currentPosting.posting.AccountID,
				CommodityID: currentPosting.posting.CommodityID,
				EntryDate:   currentPosting.entryDate,
			})
		}
	}
	return refs, nil
}

func reconciliationAffectingPostingChange(currentEntryDate string, current db.PostingRecord, nextEntryDate string, next db.PostingSpec) bool {
	return currentEntryDate != nextEntryDate ||
		current.AccountID != next.AccountID ||
		current.CommodityID != next.CommodityID ||
		current.QuantityValue != next.QuantityValue ||
		current.QuantityScale != next.QuantityScale
}

func reconciliationRefsFromRecord(record db.TransactionRecord) []db.CheckpointInvalidationRef {
	refs := make([]db.CheckpointInvalidationRef, 0)
	for _, entry := range record.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.ReconciliationStatus == "reconciled" {
				refs = append(refs, db.CheckpointInvalidationRef{
					AccountID:   posting.AccountID,
					CommodityID: posting.CommodityID,
					EntryDate:   entry.EntryDate,
				})
			}
		}
	}
	return refs
}

func reconciliationRefsFromTransaction(transaction Transaction) []db.CheckpointInvalidationRef {
	refs := make([]db.CheckpointInvalidationRef, 0)
	for _, entry := range transaction.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.ReconciliationStatus == "reconciled" {
				refs = append(refs, db.CheckpointInvalidationRef{
					AccountID: posting.AccountID, CommodityID: posting.CommodityID, EntryDate: entry.EntryDate,
				})
			}
		}
	}
	return refs
}

func transactionInputFromTransaction(transaction Transaction) TransactionInput {
	entries := make([]JournalEntryInput, 0, len(transaction.JournalEntries))
	for _, entry := range transaction.JournalEntries {
		postings := make([]PostingInput, 0, len(entry.Postings))
		for _, posting := range entry.Postings {
			postings = append(postings, PostingInput{
				LineKey:       posting.LineKey,
				AccountID:     posting.AccountID,
				QuantityValue: posting.QuantityValue,
				QuantityScale: posting.QuantityScale,
				CommodityID:   posting.CommodityID,
				Memo:          posting.Memo,
				MetadataJSON:  posting.MetadataJSON,
				TagIDs:        posting.TagIDs,
			})
		}
		entries = append(entries, JournalEntryInput{
			EntryDate:    entry.EntryDate,
			EntryKind:    entry.EntryKind,
			Memo:         entry.Memo,
			MetadataJSON: entry.MetadataJSON,
			Postings:     postings,
		})
	}

	return TransactionInput{
		Status:          transaction.Status,
		TransactionKind: transaction.TransactionKind,
		TransactionDate: transaction.TransactionDate,
		PayeeID:         transaction.PayeeID,
		PayeeName:       transaction.PayeeName,
		Description:     transaction.Description,
		ExternalRefHint: transaction.ExternalRefHint,
		NoteMarkdown:    transaction.NoteMarkdown,
		MetadataJSON:    transaction.MetadataJSON,
		TagIDs:          transaction.TagIDs,
		JournalEntries:  entries,
	}
}

func mapTransactionDBError(err error) error {
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrTransactionNotFound
	case errors.Is(err, db.ErrTransactionHasPostedVersions):
		return ErrTransactionPosted
	case errors.Is(err, db.ErrTransactionVoided):
		return ErrTransactionVoided
	case errors.Is(err, db.ErrTransactionDeleted):
		return ErrTransactionDeleted
	case errors.Is(err, db.ErrTransactionReconciled):
		return ErrTransactionProtected
	case errors.Is(err, db.ErrArchivedTag):
		return ErrTransactionTag
	case errors.Is(err, db.ErrReconciliationNotFound):
		return ErrReconciliationNotFound
	case errors.Is(err, db.ErrReconciliationClosed):
		return ErrReconciliationClosed
	case errors.Is(err, db.ErrReconciliationNotBalanced):
		return ErrReconciliationNotBalanced
	case errors.Is(err, db.ErrReconciliationPosting), errors.Is(err, db.ErrReconciliationCheckpoint):
		return ErrReconciliationPosting
	default:
		return fmt.Errorf("transaction repository: %w", err)
	}
}

// enrichPostings performs one bulk account lookup and one bulk commodity lookup
// across all postings in the given transactions, then fills the enriched fields
// on each Posting in memory. It must be called before any response path returns
// transaction data to callers. A missing account or commodity is an internal error.
func (s *TransactionService) enrichPostings(ctx context.Context, transactions []Transaction) error {
	accountIDs := make(map[int64]struct{})
	commodityIDs := make(map[int64]struct{})
	for i := range transactions {
		for j := range transactions[i].JournalEntries {
			for k := range transactions[i].JournalEntries[j].Postings {
				accountIDs[transactions[i].JournalEntries[j].Postings[k].AccountID] = struct{}{}
				commodityIDs[transactions[i].JournalEntries[j].Postings[k].CommodityID] = struct{}{}
			}
		}
	}

	flatAccountIDs := make([]int64, 0, len(accountIDs))
	for id := range accountIDs {
		flatAccountIDs = append(flatAccountIDs, id)
	}
	flatCommodityIDs := make([]int64, 0, len(commodityIDs))
	for id := range commodityIDs {
		flatCommodityIDs = append(flatCommodityIDs, id)
	}

	accountMap, err := s.accountRepository.AccountsByIDs(ctx, BookID, flatAccountIDs)
	if err != nil {
		return fmt.Errorf("enrich postings: accounts lookup: %w", err)
	}
	commodityMap, err := s.commodityRepository.CommoditiesByIDs(ctx, BookID, flatCommodityIDs)
	if err != nil {
		return fmt.Errorf("enrich postings: commodities lookup: %w", err)
	}

	for i := range transactions {
		for j := range transactions[i].JournalEntries {
			for k := range transactions[i].JournalEntries[j].Postings {
				p := &transactions[i].JournalEntries[j].Postings[k]

				acct, ok := accountMap[p.AccountID]
				if !ok {
					return fmt.Errorf("enrich postings: account %d not found", p.AccountID)
				}
				if acct.Name.Valid {
					name := acct.Name.String
					p.AccountName = &name
				}
				if acct.Code.Valid {
					code := acct.Code.String
					p.AccountCode = &code
				}
				if acct.SystemRole.Valid {
					role := acct.SystemRole.String
					p.AccountSystemRole = &role
				}
				if acct.BuiltinKey.Valid {
					key := acct.BuiltinKey.String
					p.AccountBuiltinKey = &key
				}
				p.AccountClass = acct.AccountClass

				comm, ok := commodityMap[p.CommodityID]
				if !ok {
					return fmt.Errorf("enrich postings: commodity %d not found", p.CommodityID)
				}
				p.CommodityCode = comm.Code
				if comm.DisplaySymbol != "" {
					sym := comm.DisplaySymbol
					p.CommoditySymbol = &sym
				}
			}
		}
	}

	return nil
}

// enrichRegisterPostings is the AccountRegisterEntry equivalent of enrichPostings.
func (s *TransactionService) enrichRegisterPostings(ctx context.Context, entries []AccountRegisterEntry) error {
	accountIDs := make(map[int64]struct{})
	commodityIDs := make(map[int64]struct{})
	for i := range entries {
		accountIDs[entries[i].Posting.AccountID] = struct{}{}
		commodityIDs[entries[i].Posting.CommodityID] = struct{}{}
	}

	flatAccountIDs := make([]int64, 0, len(accountIDs))
	for id := range accountIDs {
		flatAccountIDs = append(flatAccountIDs, id)
	}
	flatCommodityIDs := make([]int64, 0, len(commodityIDs))
	for id := range commodityIDs {
		flatCommodityIDs = append(flatCommodityIDs, id)
	}

	accountMap, err := s.accountRepository.AccountsByIDs(ctx, BookID, flatAccountIDs)
	if err != nil {
		return fmt.Errorf("enrich register postings: accounts lookup: %w", err)
	}
	commodityMap, err := s.commodityRepository.CommoditiesByIDs(ctx, BookID, flatCommodityIDs)
	if err != nil {
		return fmt.Errorf("enrich register postings: commodities lookup: %w", err)
	}

	for i := range entries {
		p := &entries[i].Posting

		acct, ok := accountMap[p.AccountID]
		if !ok {
			return fmt.Errorf("enrich register postings: account %d not found", p.AccountID)
		}
		if acct.Name.Valid {
			name := acct.Name.String
			p.AccountName = &name
		}
		if acct.Code.Valid {
			code := acct.Code.String
			p.AccountCode = &code
		}
		if acct.SystemRole.Valid {
			role := acct.SystemRole.String
			p.AccountSystemRole = &role
		}
		if acct.BuiltinKey.Valid {
			key := acct.BuiltinKey.String
			p.AccountBuiltinKey = &key
		}
		p.AccountClass = acct.AccountClass

		comm, ok := commodityMap[p.CommodityID]
		if !ok {
			return fmt.Errorf("enrich register postings: commodity %d not found", p.CommodityID)
		}
		p.CommodityCode = comm.Code
		if comm.DisplaySymbol != "" {
			sym := comm.DisplaySymbol
			p.CommoditySymbol = &sym
		}
	}

	return nil
}

func toTransactions(records []db.TransactionRecord) []Transaction {
	transactions := make([]Transaction, 0, len(records))
	for _, record := range records {
		transactions = append(transactions, toTransaction(record))
	}
	return transactions
}

func toAccountRegisterEntries(records []db.AccountRegisterEntryRecord, runningBalances map[int64]BalanceQuantity) []AccountRegisterEntry {
	entries := make([]AccountRegisterEntry, 0, len(records))
	for _, record := range records {
		transaction := toTransaction(record.Transaction)
		posting := Posting{
			ID:                   record.Posting.ID,
			PostingLineID:        record.Posting.PostingLineID,
			LineKey:              record.Posting.LineKey,
			LineSeq:              record.Posting.LineSeq,
			AccountID:            record.Posting.AccountID,
			QuantityValue:        record.Posting.QuantityValue,
			QuantityScale:        record.Posting.QuantityScale,
			CommodityID:          record.Posting.CommodityID,
			Memo:                 record.Posting.Memo,
			ReconciliationStatus: record.Posting.ReconciliationStatus,
			ClearedOn:            nullableString(record.Posting.ClearedOn),
			MetadataJSON:         record.Posting.MetadataJSON,
			TagIDs:               record.Posting.TagIDs,
		}
		entries = append(entries, AccountRegisterEntry{
			TransactionID:             transaction.ID,
			BookID:                    transaction.BookID,
			CorrectionOfTransactionID: transaction.CorrectionOfTransactionID,
			Status:                    transaction.Status,
			TransactionKind:           transaction.TransactionKind,
			TransactionDate:           transaction.TransactionDate,
			PayeeID:                   transaction.PayeeID,
			PayeeName:                 transaction.PayeeName,
			Description:               transaction.Description,
			ExternalRefHint:           transaction.ExternalRefHint,
			NeedsReview:               transaction.NeedsReview,
			VersionID:                 transaction.VersionID,
			VersionSeq:                transaction.VersionSeq,
			SupersedesVersionID:       transaction.SupersedesVersionID,
			TransactionTagIDs:         transaction.TagIDs,
			JournalEntryID:            record.JournalEntry.ID,
			EntrySeq:                  record.JournalEntry.EntrySeq,
			EntryDate:                 record.JournalEntry.EntryDate,
			EntryKind:                 record.JournalEntry.EntryKind,
			EntryMemo:                 record.JournalEntry.Memo,
			Posting:                   posting,
			RunningBalance:            runningBalances[record.Posting.ID],
			CreatedAt:                 transaction.CreatedAt,
			UpdatedAt:                 transaction.UpdatedAt,
			ChangeReason:              transaction.ChangeReason,
		})
	}
	return entries
}

func toTransaction(record db.TransactionRecord) Transaction {
	entries := make([]JournalEntry, 0, len(record.JournalEntries))
	for _, entry := range record.JournalEntries {
		postings := make([]Posting, 0, len(entry.Postings))
		for _, posting := range entry.Postings {
			postings = append(postings, Posting{
				ID:                   posting.ID,
				PostingLineID:        posting.PostingLineID,
				LineKey:              posting.LineKey,
				LineSeq:              posting.LineSeq,
				AccountID:            posting.AccountID,
				QuantityValue:        posting.QuantityValue,
				QuantityScale:        posting.QuantityScale,
				CommodityID:          posting.CommodityID,
				Memo:                 posting.Memo,
				ReconciliationStatus: posting.ReconciliationStatus,
				ClearedOn:            nullableString(posting.ClearedOn),
				MetadataJSON:         posting.MetadataJSON,
				TagIDs:               posting.TagIDs,
			})
		}
		entries = append(entries, JournalEntry{
			ID:           entry.ID,
			EntrySeq:     entry.EntrySeq,
			EntryDate:    entry.EntryDate,
			EntryKind:    entry.EntryKind,
			Memo:         entry.Memo,
			MetadataJSON: entry.MetadataJSON,
			Postings:     postings,
		})
	}

	return Transaction{
		ID:                        record.ID,
		BookID:                    record.BookID,
		CorrectionOfTransactionID: nullableInt64FromSQL(record.CorrectionOfTransactionID),
		Status:                    record.Status,
		TransactionKind:           record.TransactionKind,
		TransactionDate:           record.TransactionDate,
		PayeeID:                   nullableInt64FromSQL(record.PayeeID),
		PayeeName:                 nullableString(record.PayeeName),
		Description:               record.Description,
		ExternalRefHint:           nullableString(record.ExternalRefHint),
		NoteMarkdown:              record.NoteMarkdown,
		MetadataJSON:              record.MetadataJSON,
		NeedsReview:               record.NeedsReview,
		VersionID:                 record.VersionID,
		VersionSeq:                record.VersionSeq,
		SupersedesVersionID:       nullableInt64FromSQL(record.SupersedesVersionID),
		TagIDs:                    record.TagIDs,
		JournalEntries:            entries,
		CreatedAt:                 record.CreatedAt,
		UpdatedAt:                 record.RecordedAt,
		DeletedAt:                 nullableString(record.DeletedAt),
		ChangeReason:              record.ChangeReason,
		InvalidatedCheckpointIDs:  record.InvalidatedCheckpointIDs,
	}
}
