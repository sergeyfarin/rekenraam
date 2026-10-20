package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode"

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

// transactionStatuses is the persisted transaction lifecycle. An unsaved entry
// (an in-progress UI working copy with no row) is deliberately NOT a status: it
// has no database row and triggers no side effects until it is saved. Manual
// entry and import commit save posted transactions. "draft" means a persisted,
// durable recurring-producer draft excluded from the ledger; future producers
// require their own review/discard surfaces. Background FX coverage
// fires on posted versions only; a promoted draft's foreign-currency dates get
// coverage at promotion. Soft-delete is the separate deleted_at flag, not a
// status value. See docs/plans/transaction-ledger-core-plan.md.
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

func (s *TransactionService) listParams(ctx context.Context, input ListTransactionsInput, filterEntryDate bool) (db.ListTransactionsParams, error) {
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

	cursorDate, cursorDaySequence, cursorID, err := db.DecodeTransactionCursor(input.Cursor)
	if err != nil {
		return db.ListTransactionsParams{}, ValidationError{Message: "cursor is invalid"}
	}

	if input.CategoryID > 0 {
		accountMap, err := s.accountRepository.AccountsByIDs(ctx, BookID, []int64{input.CategoryID})
		if err != nil {
			return db.ListTransactionsParams{}, fmt.Errorf("look up category account: %w", err)
		}
		acct, ok := accountMap[input.CategoryID]
		if !ok {
			return db.ListTransactionsParams{}, ValidationError{Message: "category account not found"}
		}
		if acct.AccountClass != "income" && acct.AccountClass != "expense" {
			return db.ListTransactionsParams{}, ValidationError{Message: "category_id must refer to an income or expense account"}
		}
	}

	categoryType := strings.TrimSpace(input.CategoryType)
	if categoryType != "" && categoryType != "income" && categoryType != "expense" {
		return db.ListTransactionsParams{}, ValidationError{Message: "category type is invalid"}
	}

	dateBasis := strings.TrimSpace(input.DateBasis)
	if dateBasis == "" {
		dateBasis = "transaction"
	}
	if dateBasis != "transaction" && dateBasis != "entry" {
		return db.ListTransactionsParams{}, ValidationError{Message: "date basis is invalid"}
	}
	query := strings.TrimSpace(input.Query)
	if strings.ContainsRune(query, '\x00') {
		return db.ListTransactionsParams{}, ValidationError{Message: "search query must not contain NUL"}
	}

	limit := input.Limit
	if limit <= 0 {
		limit = transactionListLimit
	}
	if limit > transactionMaxLimit {
		limit = transactionMaxLimit
	}

	return db.ListTransactionsParams{
		BookID:            BookID,
		AccountID:         input.AccountID,
		CategoryID:        input.CategoryID,
		CommodityID:       input.CommodityID,
		PayeeID:           input.PayeeID,
		Status:            status,
		ExcludeDraft:      status == "",
		Kind:              kind,
		NeedsReview:       input.NeedsReview,
		Query:             query,
		AfterDate:         afterDate,
		BeforeDate:        beforeDate,
		CursorDate:        cursorDate,
		CursorDaySequence: cursorDaySequence,
		CursorID:          cursorID,
		Limit:             limit,
		FilterEntryDate:   filterEntryDate,
		EntryDateBasis:    dateBasis == "entry",
		CategoryType:      categoryType,
	}, nil
}

func (s *TransactionService) cleanTransactionSpec(ctx context.Context, input TransactionInput, options cleanTransactionOptions) (db.TransactionSpec, error) {
	status := strings.TrimSpace(input.Status)
	if status != "" && (!transactionStatuses[status] || status == "voided") {
		return db.TransactionSpec{}, ValidationError{Message: "transaction status is invalid"}
	}
	if options.ForcedStatus != "" {
		status = options.ForcedStatus
	} else {
		if status == "" {
			status = options.DefaultStatus
		}
		if status == "" {
			status = "posted"
		}
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
		today, err := s.settings.OwnerTodayAt(ctx, options.OwnerUserID, s.now())
		if err != nil {
			return db.TransactionSpec{}, err
		}
		transactionDate = today
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

	// A typed name that already names a payee is linked to it (T-50). Active
	// payee names are unique by normalized name, so the match is unambiguous —
	// this resolves what the caller clearly meant, it does not create anything.
	// Creating a record from an unrecognized name stays a deliberate, confirmed
	// act in the UI; leaving it unlinked here is what lets an import defer the
	// question to review instead of inventing payees in bulk.
	if !payeeID.Valid && cleanedPayeeName != "" {
		resolvedID, resolvedName, err := s.payeeRepository.PayeeByNormalizedName(ctx, BookID, normalizePayeeNameForMatch(cleanedPayeeName))
		switch {
		case err == nil:
			payeeID = nullableInt64(&resolvedID)
			// Adopt the record's capitalisation too, so the stored name and the
			// linked record never disagree about how the payee is spelled.
			cleanedPayeeName = resolvedName
		case !errors.Is(err, db.ErrNotFound):
			return db.TransactionSpec{}, fmt.Errorf("match payee name: %w", err)
		}
	}

	description, err := cleanOptionalText(input.Description, "description", transactionTextMaxBytes)
	if err != nil {
		return db.TransactionSpec{}, err
	}
	externalRefHint, err := cleanOptionalText(input.ExternalRefHint, "external reference", transactionTextMaxBytes)
	if err != nil {
		return db.TransactionSpec{}, err
	}
	noteMarkdown, err := cleanOptionalMultilineText(input.NoteMarkdown, "note", transactionNoteMaxBytes)
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

	journalEntries, err := s.groupPostingsByPostedOn(ctx, input, transactionDate)
	if err != nil {
		return db.TransactionSpec{}, err
	}
	entries := make([]db.JournalEntrySpec, 0, len(journalEntries))
	for entryIndex, entryInput := range journalEntries {
		entry, err := s.cleanJournalEntry(ctx, entryInput, transactionDate, status, options)
		if err != nil {
			return db.TransactionSpec{}, err
		}
		if len(entry.Postings) < 2 {
			return db.TransactionSpec{}, ValidationError{Message: fmt.Sprintf("journal entry %d must have at least two postings", entryIndex+1)}
		}
		entries = append(entries, entry)
	}
	if err := validateLineKeysUnique(entries); err != nil {
		return db.TransactionSpec{}, err
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
		Status:                  status,
		TransactionKind:         kind,
		InvestmentOperationKind: input.InvestmentOperationKind,
		TransactionDate:         transactionDate,
		PayeeID:                 payeeID,
		PayeeName:               nullableSQLString(cleanedPayeeName),
		Description:             description,
		ExternalRefHint:         externalRefHint,
		NoteMarkdown:            noteMarkdown,
		MetadataJSON:            metadataJSON,
		NeedsReview:             input.NeedsReview,
		TagIDs:                  tagIDs,
		JournalEntries:          entries,
	}, nil
}

// validateLineKeysUnique rejects a spec that names one posting line on more
// than one posting (#224). A line is one posting per version: its
// reconciliation state, tags and day order belong to exactly one amount.
func validateLineKeysUnique(entries []db.JournalEntrySpec) error {
	seen := make(map[string]bool)
	for _, entry := range entries {
		for _, posting := range entry.Postings {
			if seen[posting.LineKey] {
				return ValidationError{Message: "posting line key is used more than once"}
			}
			seen[posting.LineKey] = true
		}
	}
	return nil
}

func (s *TransactionService) cleanJournalEntry(ctx context.Context, input JournalEntryInput, transactionDate string, status string, options cleanTransactionOptions) (db.JournalEntrySpec, error) {
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
		posting, err := s.cleanPosting(ctx, postingInput, entryDate, status, options)
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

// isSubledgerManagedAccount reports whether an account's holdings are owned by
// the investment subledger, so that only an investment command may write to it.
// Keyed on the account kind's base_kind family — security_holding covers both
// the security_holding and fund_holding kinds — so a new holding kind added to
// the account_kinds table is guarded without a second edit here.
//
// Deliberately excludes crypto_wallet (base_kind "digital_asset"). The
// investment commands do not offer crypto accounts, so guarding them would
// remove the only way to record a crypto balance without providing a
// replacement. That gap is recorded in docs/backlog.md (T-96).
// subledgerManagedBaseKind is the account_kinds.base_kind family whose holdings
// the investment subledger owns — today the security_holding and fund_holding
// kinds both sit under it. Rules about these accounts compare against this one
// constant rather than spelling out kind codes, so they cannot drift apart or
// miss a kind added to the table later.
const subledgerManagedBaseKind = "security_holding"

func isSubledgerManagedAccount(rule db.PostingAccountRule) bool {
	return rule.BaseKind == subledgerManagedBaseKind
}

// accountRuleMiss is why an as-of PostingAccountRule lookup found nothing.
type accountRuleMiss int

const (
	// accountMissing: there is no such account in the book, so no date the
	// user could pick would make the write work.
	accountMissing accountRuleMiss = iota
	// accountNotYetOpened: the date is before the account opened — entering
	// history for a later-opened account, an ordinary user mistake.
	accountNotYetOpened
	// accountVersionGap: the account was open on the date, but its earliest
	// version takes effect later, so nothing describes the account then.
	accountVersionGap
)

// explainAccountRuleMiss tells apart the cases PostingAccountRule's as-of
// lookup reports alike as ErrNotFound, so the rejection names the actual
// reason instead of claiming the account itself is invalid. The rejection
// stays either way: a posting before the account's first version would make
// the ledger export's account-version fallback load-bearing (ADR 0011, #107).
func (s *TransactionService) explainAccountRuleMiss(ctx context.Context, accountID int64, date string) (accountRuleMiss, db.PostingAccountRule, error) {
	earliest, err := s.repository.EarliestPostingAccountRule(ctx, BookID, accountID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return accountMissing, db.PostingAccountRule{}, nil
		}
		return accountMissing, db.PostingAccountRule{}, fmt.Errorf("read earliest posting account rule: %w", err)
	}
	switch {
	case date < earliest.OpenedOn:
		return accountNotYetOpened, earliest, nil
	case date < earliest.EffectiveFrom:
		return accountVersionGap, earliest, nil
	default:
		// Unreachable while the as-of lookup and this one read the same
		// rows; if they ever disagree, the generic message is the safe one.
		return accountMissing, earliest, nil
	}
}

func (s *TransactionService) cleanPosting(ctx context.Context, input PostingInput, entryDate string, status string, options cleanTransactionOptions) (db.PostingSpec, error) {
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
			miss, earliest, missErr := s.explainAccountRuleMiss(ctx, input.AccountID, entryDate)
			if missErr != nil {
				return db.PostingSpec{}, missErr
			}
			switch miss {
			case accountNotYetOpened:
				return db.PostingSpec{}, ValidationError{Message: "posting date is before account opened date"}
			case accountVersionGap:
				return db.PostingSpec{}, ValidationError{Message: fmt.Sprintf("posting account has no version effective on %s; its earliest version is effective from %s", entryDate, earliest.EffectiveFrom)}
			default:
				return db.PostingSpec{}, ValidationError{Message: "posting account is invalid"}
			}
		}
		return db.PostingSpec{}, fmt.Errorf("read posting account rule: %w", err)
	}
	// Every check below reads this account version, so the write's dependency
	// on it starts here — before the first decision is made, and whether or not
	// that decision is a rejection (T-100).
	options.AccountRuleDependencies.observe(accountRule)
	if !accountRule.AcceptsPostingsOn(entryDate) {
		return db.PostingSpec{}, ValidationError{Message: "posting account is not active for postings"}
	}
	// T-96. A holding account's quantity is not just a number in the register:
	// it is the journal half of a position whose other half is the investment
	// subledger's lots. A generic balanced entry can satisfy every rule above
	// and still leave shares that no lot accounts for, which makes cost basis
	// and realized gains uncomputable for that position — and no current-state
	// diagnostic can repair it, because there is no record of what was
	// acquired or when. So an ordinary write may not touch these accounts at
	// all; the investment commands, which write journal and lots in one
	// database transaction, are the only way in.
	if !options.AllowSubledgerManagedPostings && isSubledgerManagedAccount(accountRule) {
		return db.PostingSpec{}, ValidationError{Message: "posting account is managed by the investment subledger: use a buy, sell, dividend or write-off instead of an ordinary posting"}
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
			// Unlike the account rule above, this cannot mean "not yet
			// effective on that date": a commodity's first version is stamped
			// db.CommodityGenesisDate, so the as-of lookup always resolves it
			// for any entry date (T-42). A miss here means the commodity does
			// not exist.
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
	if lineKey != "" && options.ExistingLineKeys != nil && !options.ExistingLineKeys[lineKey] {
		return db.PostingSpec{}, ValidationError{Message: "posting line key is invalid"}
	}
	if lineKey == "" {
		lineKey = s.newLineKey()
	}

	existingState, hasExistingState := options.ExistingPostings[lineKey]
	if hasExistingState && reconciliationAffectingChange(existingState.EntryDate, existingState.Posting, entryDate, db.PostingSpec{
		AccountID: input.AccountID, CommodityID: input.CommodityID, QuantityValue: input.QuantityValue, QuantityScale: input.QuantityScale,
	}) {
		// A cleared or reconciled status records that a statement verified
		// this line's account, commodity, amount and date. Once one of them
		// changes, no statement verified what the line now shows, so it starts
		// again as uncleared. Its checkpoint links stay with the line as
		// evidence (#297).
		existingState = existingPostingState{}
		hasExistingState = false
	}
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
				value.Mul(value, exact.Pow10(scaleDiff))
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

// cleanOptionalMultilineText is like cleanOptionalText but permits \n and \t,
// for fields that are genuinely multi-line (markdown notes) rather than
// single-line labels/memos.
func cleanOptionalMultilineText(value string, field string, maxBytes int) (string, error) {
	cleaned := strings.TrimSpace(value)
	if len(cleaned) > maxBytes {
		return "", ValidationError{Message: fmt.Sprintf("%s must be at most %d bytes", field, maxBytes)}
	}
	for _, r := range cleaned {
		if r == '\n' || r == '\t' {
			continue
		}
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
				EntryDate:            entry.EntryDate,
				Posting:              posting,
			}
		}
	}
	return states
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
