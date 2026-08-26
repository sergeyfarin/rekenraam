package app

import (
	"context"
	"fmt"
	"maps"
	"math/big"
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
	// Reporting is optional; the exact per-commodity measures are unchanged
	// whether or not it is set.
	Reporting *ReportingCurrencyInput
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
	// Converted holds the same seven measures in the reporting currency, each
	// summed from postings converted at their own entry date. Absent when a
	// posting in the bucket could not be converted. The three net measures are
	// derived from the converted parts so their identities hold exactly — see
	// cashflowTotals.
	ConvertedInflow       *BalanceQuantity
	ConvertedOutflow      *BalanceQuantity
	ConvertedTransferIn   *BalanceQuantity
	ConvertedTransferOut  *BalanceQuantity
	ConvertedTransferNet  *BalanceQuantity
	ConvertedOperatingNet *BalanceQuantity
	ConvertedNetMovement  *BalanceQuantity
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
	Valuation           *ValuationCoverage
}

// cashflowTotals accumulates one bucket's exact sums per commodity, and — when
// a reporting currency is asked for — the same measures converted.
//
// The converted net movement is **derived** from the converted parts rather
// than accumulated from the cash postings directly. Both are defensible; only
// one keeps R2's identity. Each posting is rounded to the reporting currency as
// it is converted, and rounding is not additive: converting a -100.00 cash
// posting can differ by a minor unit from converting its 60.00 and 40.00
// counterparts separately. Deriving makes net_movement = operating_net +
// transfer_net true by construction in the converted view, exactly as it is in
// the exact one, and the response names the method that produced it.
type cashflowTotals struct {
	inflow      map[int64]*exact.ScaledInt
	outflow     map[int64]*exact.ScaledInt
	transferIn  map[int64]*exact.ScaledInt
	transferOut map[int64]*exact.ScaledInt
	netMovement map[int64]*exact.ScaledInt

	rates                *RateTable
	convertedInflow      *ConvertedTotal
	convertedOutflow     *ConvertedTotal
	convertedTransferIn  *ConvertedTotal
	convertedTransferOut *ConvertedTotal
}

func newCashflowTotals(rates *RateTable) *cashflowTotals {
	totals := &cashflowTotals{
		inflow:      map[int64]*exact.ScaledInt{},
		outflow:     map[int64]*exact.ScaledInt{},
		transferIn:  map[int64]*exact.ScaledInt{},
		transferOut: map[int64]*exact.ScaledInt{},
		netMovement: map[int64]*exact.ScaledInt{},
	}
	if rates != nil {
		totals.rates = rates
		totals.convertedInflow = NewConvertedTotal(rates.Scale())
		totals.convertedOutflow = NewConvertedTotal(rates.Scale())
		totals.convertedTransferIn = NewConvertedTotal(rates.Scale())
		totals.convertedTransferOut = NewConvertedTotal(rates.Scale())
	}
	return totals
}

// addConverted values one counterpart posting at its own entry date — a flow,
// converted when it happened.
func (t *cashflowTotals) addConverted(target *ConvertedTotal, posting db.ReportCashflowPostingRecord, negate bool) {
	if t.rates == nil {
		return
	}
	value := posting.QuantityValue
	if negate {
		value = value.Negated()
	}
	converted, ok := t.rates.Convert(value, posting.QuantityScale, posting.CommodityID, posting.EntryDate)
	target.Add(converted, ok)
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

	var rates *RateTable
	if input.Reporting != nil {
		commodityIDs := make([]int64, 0, len(postings))
		for _, posting := range postings {
			commodityIDs = append(commodityIDs, posting.CommodityID)
		}
		rates, err = s.NewRateTable(ctx, *input.Reporting, commodityIDs, startDate, endDate)
		if err != nil {
			return CashflowResult{}, err
		}
	}

	totalsByBucket := make([]*cashflowTotals, len(bounds))
	for index := range totalsByBucket {
		totalsByBucket[index] = newCashflowTotals(rates)
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
		Valuation:           s.valuationCoverage(ctx, rates),
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
			totals.addConverted(totals.convertedInflow, posting, true)
		case "expense":
			addCashflowAmount(totals.outflow, posting, false)
			totals.addConverted(totals.convertedOutflow, posting, false)
		default:
			// Asset, liability, and equity counterparts outside the selected
			// scope are financing movement. The cash contribution is the
			// negation of the counterpart, so a credit counterpart brings money
			// in and a debit counterpart takes it out.
			if posting.QuantityValue.BigInt().Sign() < 0 {
				addCashflowAmount(totals.transferIn, posting, true)
				totals.addConverted(totals.convertedTransferIn, posting, true)
			} else {
				addCashflowAmount(totals.transferOut, posting, false)
				totals.addConverted(totals.convertedTransferOut, posting, false)
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

	// Each measure keeps the scale it accumulated at, and each is paired with
	// that scale in the response. Restating them all at the deepest scale any of
	// them reached would read more tidily, but it costs a digit per decimal
	// place against a 38-digit ceiling — so a measure that is representable as
	// recorded could be pushed past the limit by the precision of an unrelated
	// one, turning a valid report into LEDGER_OVERFLOW. Pairing is the client's
	// job and cannot be taken away from it by rewriting the numbers.
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

	bucket := CashflowBucket{
		StartDate:    startDate,
		EndDate:      endDate,
		Inflow:       inflow,
		Outflow:      outflow,
		TransferIn:   transferIn,
		TransferOut:  transferOut,
		OperatingNet: operating,
		TransferNet:  transfer,
		NetMovement:  netMovement,
	}
	t.attachConverted(&bucket)
	return bucket, nil
}

// attachConverted fills in the reporting-currency measures.
//
// Operating net, transfer net and net movement are computed from the converted
// inflow/outflow/transfer figures rather than accumulated separately, so
// net_movement = operating_net + transfer_net is an identity here exactly as it
// is in the exact figures. Any measure that could not be fully converted stays
// absent, and takes the derived measures that depend on it with it.
func (t *cashflowTotals) attachConverted(bucket *CashflowBucket) {
	if t.rates == nil {
		return
	}

	quantity := func(total *ConvertedTotal) *BalanceQuantity {
		value, scale, ok := total.Value()
		if !ok {
			return nil
		}
		return &BalanceQuantity{
			CommodityID:         t.rates.QuoteCommodityID(),
			QuantityValue:       value,
			QuantityScale:       scale,
			NormalQuantityValue: value,
		}
	}

	inflow := quantity(t.convertedInflow)
	outflow := quantity(t.convertedOutflow)
	transferIn := quantity(t.convertedTransferIn)
	transferOut := quantity(t.convertedTransferOut)

	bucket.ConvertedInflow = inflow
	bucket.ConvertedOutflow = outflow
	bucket.ConvertedTransferIn = transferIn
	bucket.ConvertedTransferOut = transferOut

	operating := differenceOf(inflow, outflow, t.rates)
	transferNet := differenceOf(transferIn, transferOut, t.rates)
	bucket.ConvertedOperatingNet = operating
	bucket.ConvertedTransferNet = transferNet
	bucket.ConvertedNetMovement = sumOf(operating, transferNet, t.rates)
}

// differenceOf and sumOf combine two converted figures at the reporting
// currency's own scale. Both are already rounded there, so this is plain
// integer arithmetic with nothing left to align — and nothing left to round,
// which is what keeps the identity exact.
func differenceOf(left *BalanceQuantity, right *BalanceQuantity, rates *RateTable) *BalanceQuantity {
	if left == nil || right == nil {
		return nil
	}
	value := new(big.Int).Sub(left.QuantityValue.BigInt(), right.QuantityValue.BigInt())
	return convertedQuantity(value, rates)
}

func sumOf(left *BalanceQuantity, right *BalanceQuantity, rates *RateTable) *BalanceQuantity {
	if left == nil || right == nil {
		return nil
	}
	value := new(big.Int).Add(left.QuantityValue.BigInt(), right.QuantityValue.BigInt())
	return convertedQuantity(value, rates)
}

func convertedQuantity(value *big.Int, rates *RateTable) *BalanceQuantity {
	coefficient, err := exact.FromBig(value)
	if err != nil {
		return nil
	}
	return &BalanceQuantity{
		CommodityID:         rates.QuoteCommodityID(),
		QuantityValue:       coefficient,
		QuantityScale:       rates.Scale(),
		NormalQuantityValue: coefficient,
	}
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

// validateCashflowScope adds the one rule this endpoint has beyond the shared
// account-filter contract.
//
// The asset/liability requirement is already enforced for every report by
// validateReportAccountClasses — the cash scope is the balance net_movement
// reconciles to, so a credit card is a legal scope and an income or equity
// account is not. What is specific here is the system-account exclusion: the
// default scope skips them (reports-plan.md, cashflow rule 1) and every
// response reports excluded_system_roles: ["all"], so an explicit selection
// that admitted one would claim to exclude an account it had just summed. That
// rule cannot move into the shared validator, because on net worth
// transfer_clearing is an ordinary basis member.
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
