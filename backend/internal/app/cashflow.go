package app

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// defaultCashAccountKinds is the liquid-cash scope a cashflow report uses when
// the caller names no accounts (reports-plan.md, cashflow rule 1). It is a
// named default, not an invisible "all assets" shortcut: the response echoes
// the accounts it resolved to.
var defaultCashAccountKinds = map[string]bool{
	"cash":           true,
	"checking":       true,
	"savings":        true,
	"brokerage_cash": true,
}

type CashflowInput struct {
	StartDate string
	EndDate   string
	Bucket    string
	Filters   ReportFilters
}

// CashflowBucket holds one calendar bucket's exact per-commodity totals.
//
// Every list is per commodity and unlike commodities are never combined. The
// identity net_movement = operating_net + transfer_net holds exactly in every
// commodity of every bucket, because journal entries balance per commodity:
// the counterparts of a cash posting account for precisely the movement it
// represents.
type CashflowBucket struct {
	StartDate string
	EndDate   string
	// Inflow and Outflow are positive magnitudes of money entering and leaving
	// the selected cash scope against income and expense counterparts.
	Inflow  []BalanceQuantity
	Outflow []BalanceQuantity
	// TransferIn and TransferOut are positive magnitudes of movement against
	// asset, liability, and equity counterparts outside the selected scope.
	// This never becomes income or spending.
	TransferIn  []BalanceQuantity
	TransferOut []BalanceQuantity
	// OperatingNet is Inflow - Outflow and excludes transfer/financing movement.
	OperatingNet []BalanceQuantity
	// TransferNet is TransferIn - TransferOut.
	TransferNet []BalanceQuantity
	// NetMovement is the signed sum of every selected-cash posting in the
	// bucket, so it reconciles exactly to the selected cash balance change
	// across the bucket's boundaries.
	NetMovement []BalanceQuantity
}

type CashflowResult struct {
	StartDate           string
	EndDate             string
	Bucket              string
	Filters             ReportFiltersEcho
	CashScope           string
	CashAccountKinds    []string
	Buckets             []CashflowBucket
	ExcludedSystemRoles []string
}

// cashflowTotals accumulates one bucket's exact sums per commodity.
type cashflowTotals struct {
	inflow      map[int64]*exact.ScaledInt
	outflow     map[int64]*exact.ScaledInt
	transferIn  map[int64]*exact.ScaledInt
	transferOut map[int64]*exact.ScaledInt
	netMovement map[int64]*exact.ScaledInt
}

func newCashflowTotals() *cashflowTotals {
	return &cashflowTotals{
		inflow:      map[int64]*exact.ScaledInt{},
		outflow:     map[int64]*exact.ScaledInt{},
		transferIn:  map[int64]*exact.ScaledInt{},
		transferOut: map[int64]*exact.ScaledInt{},
		netMovement: map[int64]*exact.ScaledInt{},
	}
}

// Cashflow answers what changed the selected liquid cash, without treating a
// transfer as spending.
//
// The classification is exact rather than heuristic. Journal entries balance
// per commodity, so within one entry the selected-cash postings sum to the
// negation of their counterparts. Each counterpart is then classified on its
// own account class and contributes its own signed amount — there is no
// "first counterpart wins", and a transaction with many counterparts needs no
// allocation rule (reports-plan.md, cashflow rule 5).
func (s *TransactionService) Cashflow(ctx context.Context, input CashflowInput) (CashflowResult, error) {
	startDate, endDate, err := cleanReportDateRange(input.StartDate, input.EndDate)
	if err != nil {
		return CashflowResult{}, err
	}
	bucket := strings.TrimSpace(input.Bucket)
	if !reportBuckets[bucket] {
		return CashflowResult{}, ValidationError{Message: "bucket must be day, week, month, quarter, or year"}
	}

	// The cash scope is resolved once, as of the range end, and applied to the
	// whole range. Re-resolving per bucket would let an account join or leave
	// mid-series, and net_movement would stop reconciling to the balance change
	// of any one stable set of accounts.
	filters, resolvedAccountIDs, err := s.resolveReportFilters(ctx, input.Filters, endDate)
	if err != nil {
		return CashflowResult{}, err
	}

	accounts, err := s.repository.LedgerAccountsAsOf(ctx, BookID, endDate)
	if err != nil {
		return CashflowResult{}, fmt.Errorf("read cashflow accounts: %w", err)
	}

	cashScope := "selected_accounts"
	cashAccountIDs := idSet(resolvedAccountIDs)
	if len(cashAccountIDs) == 0 {
		cashScope = "default_liquid_cash"
		cashAccountIDs = defaultCashAccountIDs(accounts)
		filters.ResolvedAccountIDs = sortedIDs(cashAccountIDs)
	} else if err := validateCashflowScope(accounts, resolvedAccountIDs); err != nil {
		return CashflowResult{}, err
	}

	bounds, err := calendarBucketBounds(startDate, endDate, bucket)
	if err != nil {
		return CashflowResult{}, err
	}

	postings, err := s.repository.ReportCashflowPostings(ctx, db.ReportCashflowPostingsParams{
		BookID:       BookID,
		StartDate:    startDate,
		EndDate:      endDate,
		CommodityIDs: filters.CommodityIDs,
	})
	if err != nil {
		return CashflowResult{}, fmt.Errorf("read cashflow postings: %w", err)
	}

	totalsByBucket := make([]*cashflowTotals, len(bounds))
	for index := range totalsByBucket {
		totalsByBucket[index] = newCashflowTotals()
	}

	for _, entry := range groupByJournalEntry(postings) {
		index := bucketIndexFor(bounds, entry.entryDate)
		if index < 0 {
			continue
		}
		classifyCashflowEntry(entry.postings, cashAccountIDs, totalsByBucket[index])
	}

	resultBuckets := make([]CashflowBucket, 0, len(bounds))
	for index, bound := range bounds {
		bucketResult, err := totalsByBucket[index].toBucket(bound.startDate, bound.endDate)
		if err != nil {
			return CashflowResult{}, err
		}
		resultBuckets = append(resultBuckets, bucketResult)
	}

	return CashflowResult{
		StartDate:        startDate,
		EndDate:          endDate,
		Bucket:           bucket,
		Filters:          filters,
		CashScope:        cashScope,
		CashAccountKinds: sortedKinds(defaultCashAccountKinds),
		Buckets:          resultBuckets,
		// System accounts never join the cash scope, but they remain visible as
		// counterparts: transfer clearing is financing movement, not spending.
		ExcludedSystemRoles: []string{"all"},
	}, nil
}

// classifyCashflowEntry folds one journal entry into a bucket's totals.
//
// A cash-to-cash transfer inside the selected scope disappears here without a
// special case: both legs land in netMovement and cancel, and the entry has no
// counterparts left to classify (cashflow rule 2).
func classifyCashflowEntry(postings []db.ReportCashflowPostingRecord, cashAccountIDs map[int64]bool, totals *cashflowTotals) {
	touchesCash := false
	for _, posting := range postings {
		if cashAccountIDs[posting.AccountID] {
			touchesCash = true
			break
		}
	}
	if !touchesCash {
		return
	}

	for _, posting := range postings {
		if cashAccountIDs[posting.AccountID] {
			addCashflowAmount(totals.netMovement, posting, false)
			continue
		}

		switch posting.AccountClass {
		case "income":
			// Income postings are credits; negating gives a positive inflow.
			addCashflowAmount(totals.inflow, posting, true)
		case "expense":
			addCashflowAmount(totals.outflow, posting, false)
		default:
			// Asset, liability, and equity counterparts outside the selected
			// scope are financing movement. The cash contribution is the
			// negation of the counterpart, so a credit counterpart brings money
			// in and a debit counterpart takes it out.
			if posting.QuantityValue.BigInt().Sign() < 0 {
				addCashflowAmount(totals.transferIn, posting, true)
			} else {
				addCashflowAmount(totals.transferOut, posting, false)
			}
		}
	}
}

func addCashflowAmount(amounts map[int64]*exact.ScaledInt, posting db.ReportCashflowPostingRecord, negate bool) {
	amount := amounts[posting.CommodityID]
	if amount == nil {
		amount = exact.NewScaledInt()
		amounts[posting.CommodityID] = amount
	}
	value := posting.QuantityValue
	if negate {
		value = value.Negated()
	}
	amount.AddCoefficient(value, posting.QuantityScale)
}

func (t *cashflowTotals) toBucket(startDate string, endDate string) (CashflowBucket, error) {
	operatingNet := subtractAmounts(t.inflow, t.outflow)
	transferNet := subtractAmounts(t.transferIn, t.transferOut)

	inflow, err := reportMagnitudes(t.inflow, false)
	if err != nil {
		return CashflowBucket{}, err
	}
	outflow, err := reportMagnitudes(t.outflow, false)
	if err != nil {
		return CashflowBucket{}, err
	}
	transferIn, err := reportMagnitudes(t.transferIn, false)
	if err != nil {
		return CashflowBucket{}, err
	}
	transferOut, err := reportMagnitudes(t.transferOut, false)
	if err != nil {
		return CashflowBucket{}, err
	}
	operating, err := reportMagnitudes(operatingNet, false)
	if err != nil {
		return CashflowBucket{}, err
	}
	transfer, err := reportMagnitudes(transferNet, false)
	if err != nil {
		return CashflowBucket{}, err
	}
	netMovement, err := reportMagnitudes(t.netMovement, false)
	if err != nil {
		return CashflowBucket{}, err
	}

	return CashflowBucket{
		StartDate:    startDate,
		EndDate:      endDate,
		Inflow:       inflow,
		Outflow:      outflow,
		TransferIn:   transferIn,
		TransferOut:  transferOut,
		OperatingNet: operating,
		TransferNet:  transfer,
		NetMovement:  netMovement,
	}, nil
}

// subtractAmounts returns left - right for every commodity present in either,
// keeping the exact scale-aware arithmetic inside ScaledInt.
func subtractAmounts(left map[int64]*exact.ScaledInt, right map[int64]*exact.ScaledInt) map[int64]*exact.ScaledInt {
	result := map[int64]*exact.ScaledInt{}
	for commodityID, amount := range left {
		total := exact.NewScaledInt()
		total.AddScaled(amount)
		result[commodityID] = total
	}
	for commodityID, amount := range right {
		total := result[commodityID]
		if total == nil {
			total = exact.NewScaledInt()
			result[commodityID] = total
		}
		total.SubScaled(amount)
	}
	return result
}

type cashflowEntry struct {
	entryDate string
	postings  []db.ReportCashflowPostingRecord
}

// groupByJournalEntry keeps each entry's postings together, in the order the
// repository returned them.
func groupByJournalEntry(postings []db.ReportCashflowPostingRecord) []cashflowEntry {
	entries := make([]cashflowEntry, 0)
	indexByEntry := map[int64]int{}
	for _, posting := range postings {
		index, ok := indexByEntry[posting.JournalEntryID]
		if !ok {
			entries = append(entries, cashflowEntry{entryDate: posting.EntryDate})
			index = len(entries) - 1
			indexByEntry[posting.JournalEntryID] = index
		}
		entries[index].postings = append(entries[index].postings, posting)
	}
	return entries
}

func bucketIndexFor(bounds []calendarBucketBound, entryDate string) int {
	for index, bound := range bounds {
		if entryDate >= bound.startDate && entryDate <= bound.endDate {
			return index
		}
	}
	return -1
}

// cashflowScopeClasses is what an explicitly selected cash scope may contain.
// The scope is the balance net_movement reconciles to, so every member must be
// an account that holds one: asset or liability. A credit card or a brokerage
// holding is therefore a legal scope — "what moved through this account" is a
// real question — while an income, expense, or equity account has no balance to
// move and would read as cash flowing the wrong way.
var cashflowScopeClasses = map[string]bool{
	"asset":     true,
	"liability": true,
}

// validateCashflowScope rejects a selection this endpoint's own response would
// then contradict. The default scope excludes system accounts (reports-plan.md,
// cashflow rule 1) and every response reports excluded_system_roles: ["all"], so
// an explicit selection that admitted one would claim to exclude an account it
// had just summed. The generic report filter validator cannot carry this rule:
// on net worth and spending a system account is an ordinary basis member.
//
// The resolved selection is checked rather than the named one, because
// include_descendants is what actually becomes the scope.
func validateCashflowScope(accounts []db.LedgerAccountRecord, resolvedAccountIDs []int64) error {
	byID := make(map[int64]db.LedgerAccountRecord, len(accounts))
	for _, account := range accounts {
		byID[account.ID] = account
	}
	for _, accountID := range resolvedAccountIDs {
		account, ok := byID[accountID]
		if !ok {
			// resolveReportFilters already proved the account is in the book. No
			// version as of end_date means it has no postings the range could
			// reach, so it cannot make the scope wrong.
			continue
		}
		if account.SystemRole.Valid {
			return ValidationError{Message: "account filter is invalid: a system account cannot join the cashflow cash scope"}
		}
		if !cashflowScopeClasses[account.AccountClass] {
			return ValidationError{Message: "account filter is invalid: the cashflow cash scope takes asset or liability accounts"}
		}
	}
	return nil
}

// defaultCashAccountIDs is the named liquid-cash default: active, non-system
// accounts of the cash-like kinds.
func defaultCashAccountIDs(accounts []db.LedgerAccountRecord) map[int64]bool {
	ids := map[int64]bool{}
	for _, account := range accounts {
		if account.SystemRole.Valid || account.Status != "active" {
			continue
		}
		if defaultCashAccountKinds[account.AccountKind] {
			ids[account.ID] = true
		}
	}
	return ids
}

func sortedIDs(ids map[int64]bool) []int64 {
	return slices.Sorted(maps.Keys(ids))
}

func sortedKinds(kinds map[string]bool) []string {
	return slices.Sorted(maps.Keys(kinds))
}
