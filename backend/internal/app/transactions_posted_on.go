package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// groupPostingsByPostedOn turns one journal entry whose account postings carry
// their own posting day (#187) into one journal entry per date.
//
//   - An account posting (an asset or liability that is not a system account)
//     posts on its PostedOn, or on the entry date when PostedOn is empty.
//   - Every other posting (categories, system legs) follows the account
//     postings when they all share one date — a card purchase posted later is
//     one entry on the posting day — and otherwise stays on the entry date.
//   - Each dated entry is balanced per commodity through transfer_clearing,
//     the money in transit between the dates. The whole transaction must
//     balance first, so the bridge never hides an imbalance.
//
// A request without PostedOn passes through untouched.
func (s *TransactionService) groupPostingsByPostedOn(ctx context.Context, input TransactionInput, transactionDate string) ([]JournalEntryInput, error) {
	usesPostedOn := false
	for _, entry := range input.JournalEntries {
		for _, posting := range entry.Postings {
			if strings.TrimSpace(posting.PostedOn) != "" {
				usesPostedOn = true
			}
		}
	}
	if !usesPostedOn {
		return input.JournalEntries, nil
	}
	if len(input.JournalEntries) != 1 {
		return nil, ValidationError{Message: "posted dates require exactly one journal entry"}
	}
	entry := input.JournalEntries[0]
	entryDate := strings.TrimSpace(entry.EntryDate)
	if entryDate == "" {
		entryDate = transactionDate
	}
	entryDate, err := cleanRequiredDate(entryDate, "entry date")
	if err != nil {
		return nil, err
	}

	if err := validatePostingsBalanced(entry.Postings); err != nil {
		return nil, err
	}

	dates := make([]string, len(entry.Postings))
	accountLeg := make([]bool, len(entry.Postings))
	accountDates := map[string]bool{}
	for index, posting := range entry.Postings {
		postedOn := strings.TrimSpace(posting.PostedOn)
		date := entryDate
		if postedOn != "" {
			date, err = cleanRequiredDate(postedOn, "posted date")
			if err != nil {
				return nil, err
			}
		}
		rule, err := s.repository.PostingAccountRule(ctx, BookID, posting.AccountID, date)
		switch {
		case err == nil:
			accountLeg[index] = isPostedOnAccount(rule)
		case errors.Is(err, db.ErrNotFound):
			// cleanPosting reports a missing or not-yet-opened account with
			// the specific message; an explicit posted date still marks the
			// leg as the account's.
			accountLeg[index] = postedOn != ""
		default:
			return nil, fmt.Errorf("read posting account rule: %w", err)
		}
		if postedOn != "" && !accountLeg[index] {
			return nil, ValidationError{Message: "a posted date applies only to an account posting"}
		}
		dates[index] = date
		if accountLeg[index] {
			accountDates[date] = true
		}
	}

	followDate := entryDate
	if len(accountDates) == 1 {
		for date := range accountDates {
			followDate = date
		}
	}
	grouped := map[string][]PostingInput{}
	for index, posting := range entry.Postings {
		date := followDate
		if accountLeg[index] {
			date = dates[index]
		}
		posting.PostedOn = ""
		grouped[date] = append(grouped[date], posting)
	}
	if len(grouped) == 1 {
		for date, postings := range grouped {
			entry.EntryDate = date
			entry.Postings = postings
		}
		return []JournalEntryInput{entry}, nil
	}

	clearingAccountID, err := s.repository.SystemAccountID(ctx, BookID, "transfer_clearing")
	if err != nil {
		return nil, fmt.Errorf("read transfer clearing account: %w", err)
	}
	orderedDates := make([]string, 0, len(grouped))
	for date := range grouped {
		orderedDates = append(orderedDates, date)
	}
	sort.Strings(orderedDates)

	entries := make([]JournalEntryInput, 0, len(orderedDates))
	for _, date := range orderedDates {
		postings := grouped[date]
		bridges, err := clearingBridges(postings, clearingAccountID)
		if err != nil {
			return nil, err
		}
		entries = append(entries, JournalEntryInput{
			EntryDate:    date,
			EntryKind:    entry.EntryKind,
			Memo:         entry.Memo,
			MetadataJSON: entry.MetadataJSON,
			Postings:     append(postings, bridges...),
		})
	}
	return entries, nil
}

// isPostedOnAccount reports whether a posting's account has its own posting
// day: a user's asset or liability, which a statement lists.
func isPostedOnAccount(rule db.PostingAccountRule) bool {
	return !rule.IsSystem && (rule.AccountClass == "asset" || rule.AccountClass == "liability")
}

// clearingBridges returns the transfer_clearing postings that balance one
// dated group per commodity, in commodity order.
func clearingBridges(postings []PostingInput, clearingAccountID int64) ([]PostingInput, error) {
	sums, commodities := sumPostingsByCommodity(postings)
	bridges := make([]PostingInput, 0, len(commodities))
	for _, commodityID := range commodities {
		sum := sums[commodityID]
		if sum.Sign() == 0 {
			continue
		}
		value, err := sum.Negated().Coefficient()
		if err != nil {
			return nil, ValidationError{Message: "posting amounts exceed the coefficient range"}
		}
		bridges = append(bridges, PostingInput{
			AccountID:     clearingAccountID,
			CommodityID:   commodityID,
			QuantityValue: value,
			QuantityScale: sum.Scale(),
		})
	}
	return bridges, nil
}

// validatePostingsBalanced rejects postings that do not balance per
// commodity before they are split across dates.
func validatePostingsBalanced(postings []PostingInput) error {
	sums, _ := sumPostingsByCommodity(postings)
	for _, sum := range sums {
		if sum.Sign() != 0 {
			return ValidationError{Message: "journal entry 1 is not balanced by commodity"}
		}
	}
	return nil
}

func sumPostingsByCommodity(postings []PostingInput) (map[int64]*exact.ScaledInt, []int64) {
	sums := map[int64]*exact.ScaledInt{}
	commodities := []int64{}
	for _, posting := range postings {
		sum := sums[posting.CommodityID]
		if sum == nil {
			sum = exact.NewScaledInt()
			sums[posting.CommodityID] = sum
			commodities = append(commodities, posting.CommodityID)
		}
		sum.AddCoefficient(posting.QuantityValue, posting.QuantityScale)
	}
	sort.Slice(commodities, func(i, j int) bool { return commodities[i] < commodities[j] })
	return sums, commodities
}

// MovePostingToDayInput moves one account posting to the adjacent calendar
// day (#187): the register's reorder mode offers it at a day's edge.
type MovePostingToDayInput struct {
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

// MovePostingToDay re-dates one account posting by a calendar day and keeps
// every other posting where it is. It is an ordinary edit of the transaction:
// the posted-date rule rebuilds the dated journal, the reconciliation guard
// applies to every account whose period the change crosses, and the posting
// lands at the end of its new day, as any posting newly on a day does.
//
// A journal the rule would not rebuild exactly as stored (an import with its
// own entry layout, say) is refused rather than reshaped; the editor shows it
// as it is.
func (s *TransactionService) MovePostingToDay(ctx context.Context, input MovePostingToDayInput) (Transaction, error) {
	if input.AccountID <= 0 || input.PostingLineID <= 0 {
		return Transaction{}, ValidationError{Message: "account and posting line are required"}
	}
	if input.Direction != "earlier" && input.Direction != "later" {
		return Transaction{}, ValidationError{Message: "direction must be 'earlier' or 'later'"}
	}
	transactionID, err := s.repository.PostingLineTransactionID(ctx, BookID, input.PostingLineID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Transaction{}, ErrTransactionNotFound
		}
		return Transaction{}, fmt.Errorf("read posting line: %w", err)
	}
	current, err := s.Transaction(ctx, transactionID)
	if err != nil {
		return Transaction{}, err
	}
	if current.Status != "posted" {
		return Transaction{}, ValidationError{Message: "only a posted transaction moves to another day"}
	}

	spec, ok := postedOnSpec(current)
	if !ok {
		return Transaction{}, ValidationError{Message: "this transaction's journal cannot move by day; edit it instead"}
	}
	withExchange, err := s.buildExchangeLegs(ctx, spec, spec.TransactionDate)
	if err != nil {
		return Transaction{}, err
	}
	rebuilt, err := s.groupPostingsByPostedOn(ctx, withExchange, spec.TransactionDate)
	if err != nil {
		return Transaction{}, err
	}
	if !sameJournalDays(current.JournalEntries, rebuilt) {
		return Transaction{}, ValidationError{Message: "this transaction's journal cannot move by day; edit it instead"}
	}

	moved := false
	for index, posting := range spec.JournalEntries[0].Postings {
		original := postingByLineKey(current, posting.LineKey)
		if original == nil || original.PostingLineID != input.PostingLineID {
			continue
		}
		if original.AccountID != input.AccountID {
			return Transaction{}, ErrTransactionNotFound
		}
		if posting.PostedOn == "" {
			// postedOnSpec dates only account postings.
			return Transaction{}, ValidationError{Message: "only an account posting moves to another day"}
		}
		day, err := time.Parse(time.DateOnly, posting.PostedOn)
		if err != nil {
			return Transaction{}, fmt.Errorf("parse posting day: %w", err)
		}
		step := 1
		if input.Direction == "earlier" {
			step = -1
		}
		spec.JournalEntries[0].Postings[index].PostedOn = day.AddDate(0, 0, step).Format(time.DateOnly)
		moved = true
	}
	if !moved {
		return Transaction{}, ValidationError{Message: "only an account posting moves to another day"}
	}

	return s.UpdateTransaction(ctx, UpdateTransactionInput{
		ExpectedVersionID:                   current.VersionID,
		OwnerUserID:                         input.OwnerUserID,
		AuthSessionID:                       input.AuthSessionID,
		RequestID:                           input.RequestID,
		OriginType:                          input.OriginType,
		Operation:                           "posting.move_day",
		TransactionID:                       current.ID,
		Spec:                                spec,
		ChangeReason:                        "moved posting " + input.Direction + " by a day",
		ReconciliationOverride:              input.ReconciliationOverride,
		ReconciliationOverrideCheckpointIDs: input.ReconciliationOverrideCheckpointIDs,
	})
}

// postedOnSpec restates a stored transaction as one journal entry whose
// account postings carry their posting days, leaving out the transfer_clearing
// bridge the posted-date rule rebuilds and an exchange's commodity_trading
// legs, which the exchange rule rebuilds (#481). It reports false when the entries
// differ in anything but their date, which one entry cannot carry.
func postedOnSpec(transaction Transaction) (TransactionInput, bool) {
	spec := transactionInputFromTransaction(transaction)
	spec.NeedsReview = transaction.NeedsReview
	if len(spec.JournalEntries) == 0 {
		return spec, false
	}
	first := spec.JournalEntries[0]
	bridged := len(transaction.JournalEntries) > 1
	merged := JournalEntryInput{EntryDate: transaction.TransactionDate, EntryKind: first.EntryKind, Memo: first.Memo, MetadataJSON: first.MetadataJSON}
	for entryIndex, entry := range transaction.JournalEntries {
		input := spec.JournalEntries[entryIndex]
		if input.EntryKind != first.EntryKind || input.Memo != first.Memo || input.MetadataJSON != first.MetadataJSON {
			return spec, false
		}
		for postingIndex, posting := range entry.Postings {
			if bridged && posting.AccountSystemRole != nil && *posting.AccountSystemRole == "transfer_clearing" {
				continue
			}
			if isExchangeTradingLeg(entry, posting) {
				continue
			}
			postingInput := input.Postings[postingIndex]
			if posting.AccountSystemRole == nil && (posting.AccountClass == "asset" || posting.AccountClass == "liability") {
				postingInput.PostedOn = entry.EntryDate
			}
			merged.Postings = append(merged.Postings, postingInput)
		}
	}
	spec.JournalEntries = []JournalEntryInput{merged}
	return spec, true
}

// sameJournalDays reports whether a rebuilt journal posts every stored
// posting line on the day it is stored on.
func sameJournalDays(stored []JournalEntry, rebuilt []JournalEntryInput) bool {
	days := map[string]string{}
	for _, entry := range rebuilt {
		for _, posting := range entry.Postings {
			if posting.LineKey != "" {
				days[posting.LineKey] = entry.EntryDate
			}
		}
	}
	for _, entry := range stored {
		for _, posting := range entry.Postings {
			day, kept := days[posting.LineKey]
			if kept && day != entry.EntryDate {
				return false
			}
		}
	}
	return true
}

func postingByLineKey(transaction Transaction, lineKey string) *Posting {
	for entryIndex := range transaction.JournalEntries {
		for postingIndex := range transaction.JournalEntries[entryIndex].Postings {
			posting := &transaction.JournalEntries[entryIndex].Postings[postingIndex]
			if posting.LineKey == lineKey {
				return posting
			}
		}
	}
	return nil
}
