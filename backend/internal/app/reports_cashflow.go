package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// defaultCashAccountKinds is the cash scope a cashflow report uses when the
// caller names no accounts. It is a stated, named default rather than an
// invisible "all assets" shortcut: the response echoes it back so an exported
// report still says what "cash" meant.
var defaultCashAccountKinds = []string{"cash", "checking", "savings", "brokerage_cash"}

type CashflowReportInput struct {
	StartDate string
	EndDate   string
	Bucket    string
}

// CashflowBucketTotal is one commodity's classified movement for one bucket.
//
// All five figures are exact and reconcile by construction:
//
//	net_movement = inflow - outflow + transfer_in - transfer_out
//	operating_net = inflow - outflow
//
// This is not enforced after the fact by a rounding step; it falls out of
// double entry. See cashflowBuckets for why.
type CashflowBucketTotal struct {
	CommodityID   int64
	QuantityScale int
	// Inflow is money arriving from an income counterpart, as a positive
	// magnitude.
	Inflow exact.Coefficient
	// Outflow is money leaving to an expense counterpart, as a positive
	// magnitude.
	Outflow exact.Coefficient
	// OperatingNet is Inflow - Outflow. It deliberately excludes transfer and
	// financing movement, so moving savings into checking never reads as
	// income.
	OperatingNet exact.Coefficient
	// TransferIn and TransferOut are movement between the selected cash scope
	// and an unselected asset, liability, or equity account, as positive
	// magnitudes. Never income, never spending.
	TransferIn  exact.Coefficient
	TransferOut exact.Coefficient
	// NetMovement is the signed change in the selected cash total for the
	// bucket. It reconciles exactly to the cash balance change across the
	// bucket boundaries.
	NetMovement exact.Coefficient
}

type CashflowBucket struct {
	StartDate string
	EndDate   string
	Totals    []CashflowBucketTotal
}

// CashflowScopeAccount names one account in the effective cash scope, so the
// result summary can say what was measured instead of showing an unexplained
// number.
type CashflowScopeAccount struct {
	AccountID   int64
	Name        string
	Code        string
	AccountKind string
}

type CashflowReportResult struct {
	StartDate string
	EndDate   string
	Bucket    string
	// ScopeKinds is the account-kind policy that selected the cash scope.
	ScopeKinds []string
	// ScopeAccounts is the resolved account list that policy produced.
	ScopeAccounts       []CashflowScopeAccount
	Buckets             []CashflowBucket
	ExcludedSystemRoles []string
}

// Cashflow answers "what changed my liquid cash, without treating transfers as
// spending?" over an inclusive date range.
//
// The classification is derived from double entry rather than from a
// per-transaction heuristic. See cashflowBuckets.
func (s *TransactionService) Cashflow(ctx context.Context, input CashflowReportInput) (CashflowReportResult, error) {
	startDate, endDate, bucket, err := cleanCashflowInput(input)
	if err != nil {
		return CashflowReportResult{}, err
	}

	accounts, err := s.repository.LedgerAccountsAsOf(ctx, BookID, endDate)
	if err != nil {
		return CashflowReportResult{}, fmt.Errorf("read cashflow accounts: %w", err)
	}
	accountMap := ledgerAccountMap(accounts)
	scope, scopeAccounts := cashflowScope(accounts)

	postings, err := s.repository.LedgerPostingsInRange(ctx, BookID, startDate, endDate, "posted")
	if err != nil {
		return CashflowReportResult{}, fmt.Errorf("read cashflow postings: %w", err)
	}

	bounds, err := calendarBucketBounds(startDate, endDate, bucket)
	if err != nil {
		return CashflowReportResult{}, err
	}

	buckets, err := cashflowBuckets(bounds, postings, accountMap, scope)
	if err != nil {
		return CashflowReportResult{}, err
	}

	return CashflowReportResult{
		StartDate:           startDate,
		EndDate:             endDate,
		Bucket:              bucket,
		ScopeKinds:          defaultCashAccountKinds,
		ScopeAccounts:       scopeAccounts,
		Buckets:             buckets,
		ExcludedSystemRoles: []string{"commodity_trading", "transfer_clearing"},
	}, nil
}

// cashflowScope resolves the default liquid-cash account set: active,
// non-system accounts of the named kinds.
func cashflowScope(accounts []db.LedgerAccountRecord) (map[int64]bool, []CashflowScopeAccount) {
	kinds := map[string]bool{}
	for _, kind := range defaultCashAccountKinds {
		kinds[kind] = true
	}

	scope := map[int64]bool{}
	scopeAccounts := make([]CashflowScopeAccount, 0)
	for _, account := range accounts {
		if account.SystemRole.Valid || account.Status != "active" || !kinds[account.AccountKind] {
			continue
		}
		scope[account.ID] = true
		scopeAccounts = append(scopeAccounts, CashflowScopeAccount{
			AccountID:   account.ID,
			Name:        nullableString(account.Name),
			Code:        nullableString(account.Code),
			AccountKind: account.AccountKind,
		})
	}
	sort.Slice(scopeAccounts, func(i, j int) bool {
		return scopeAccounts[i].AccountID < scopeAccounts[j].AccountID
	})
	return scope, scopeAccounts
}

// cashflowAmounts accumulates one commodity's classified movement while a
// bucket is being built.
type cashflowAmounts struct {
	inflow      *exact.ScaledInt
	outflow     *exact.ScaledInt
	transferIn  *exact.ScaledInt
	transferOut *exact.ScaledInt
	netMovement *exact.ScaledInt
}

func newCashflowAmounts() *cashflowAmounts {
	return &cashflowAmounts{
		inflow:      exact.NewScaledInt(),
		outflow:     exact.NewScaledInt(),
		transferIn:  exact.NewScaledInt(),
		transferOut: exact.NewScaledInt(),
		netMovement: exact.NewScaledInt(),
	}
}

// cashflowBuckets classifies movement in and out of the cash scope.
//
// # Why this reconciles without a fudge factor
//
// Postings are grouped by journal entry, which is the unit `validateBalanced`
// enforces balance over, and an entry carries a single date so it never
// straddles a bucket. Within one entry and one commodity, therefore:
//
//	sum(selected cash postings) + sum(counterpart postings) = 0
//
// So the bucket's net cash movement is exactly the negated sum of the
// counterparts, and splitting the counterparts by account class splits the
// movement:
//
//	inflow       = -sum(income counterparts)     (income is credit-negative)
//	outflow      =  sum(expense counterparts)    (expense is debit-positive)
//	transfer_net = -sum(other counterparts)
//	net_movement = inflow - outflow + transfer_net
//
// Three consequences fall out of this rather than being special-cased:
//
//   - A transaction with many counterparts needs no allocation rule at all.
//     "First counterpart wins" is not merely disallowed here, it is
//     unnecessary — every counterpart contributes its own exact amount.
//   - A transfer between two accounts that are *both* in scope has no
//     counterpart outside the scope, so it contributes zero. It is eliminated
//     because it genuinely changed nothing, not because it was filtered out.
//   - A partial internal transfer (cash out of A, some into B, the rest spent)
//     splits correctly, because only the out-of-scope legs are counted.
func cashflowBuckets(
	bounds []calendarBucketBound,
	postings []db.LedgerPostingRecord,
	accounts map[int64]db.LedgerAccountRecord,
	scope map[int64]bool,
) ([]CashflowBucket, error) {
	// Index postings by journal entry so each entry's counterparts are known
	// when its cash legs are classified.
	entries := map[int64][]db.LedgerPostingRecord{}
	for _, posting := range postings {
		entries[posting.JournalEntryID] = append(entries[posting.JournalEntryID], posting)
	}

	buckets := make([]CashflowBucket, 0, len(bounds))
	for _, bound := range bounds {
		byCommodity := map[int64]*cashflowAmounts{}

		for _, entryPostings := range entries {
			// An entry has one date, so testing any posting places the whole
			// entry.
			if entryPostings[0].EntryDate < bound.startDate || entryPostings[0].EntryDate > bound.endDate {
				continue
			}
			// Only entries that actually touch the cash scope move cash.
			touchesScope := false
			for _, posting := range entryPostings {
				if scope[posting.AccountID] {
					touchesScope = true
					break
				}
			}
			if !touchesScope {
				continue
			}

			for _, posting := range entryPostings {
				if scope[posting.AccountID] {
					// The cash legs give net movement directly.
					amounts := byCommodity[posting.CommodityID]
					if amounts == nil {
						amounts = newCashflowAmounts()
						byCommodity[posting.CommodityID] = amounts
					}
					amounts.netMovement.AddCoefficient(posting.QuantityValue, posting.QuantityScale)
					continue
				}

				account, ok := accounts[posting.AccountID]
				if !ok {
					continue
				}
				amounts := byCommodity[posting.CommodityID]
				if amounts == nil {
					amounts = newCashflowAmounts()
					byCommodity[posting.CommodityID] = amounts
				}

				switch account.AccountClass {
				case "income":
					// Income is stored credit-negative; negating gives a
					// positive inflow magnitude.
					amounts.inflow.AddCoefficient(posting.QuantityValue.Negated(), posting.QuantityScale)
				case "expense":
					amounts.outflow.AddCoefficient(posting.QuantityValue, posting.QuantityScale)
				default:
					// Asset, liability, or equity outside the cash scope:
					// transfer/financing movement. A positive (debit) leg on
					// the counterpart means cash left the scope to fund it.
					if posting.QuantityValue.Sign() >= 0 {
						amounts.transferOut.AddCoefficient(posting.QuantityValue, posting.QuantityScale)
					} else {
						amounts.transferIn.AddCoefficient(posting.QuantityValue.Negated(), posting.QuantityScale)
					}
				}
			}
		}

		totals, err := cashflowTotals(byCommodity)
		if err != nil {
			return nil, err
		}
		buckets = append(buckets, CashflowBucket{
			StartDate: bound.startDate,
			EndDate:   bound.endDate,
			Totals:    totals,
		})
	}

	return buckets, nil
}

func cashflowTotals(byCommodity map[int64]*cashflowAmounts) ([]CashflowBucketTotal, error) {
	totals := make([]CashflowBucketTotal, 0, len(byCommodity))
	for commodityID, amounts := range byCommodity {
		// operating_net is computed from the same ScaledInt values that produced
		// inflow and outflow, so it cannot disagree with them.
		operatingNet := exact.NewScaledInt()
		operatingNet.AddScaled(amounts.inflow)
		operatingNet.SubScaled(amounts.outflow)

		// Every figure in a row is reported at one scale — the widest any of
		// them needed — so a reader is never comparing a 2-scale inflow against
		// a 4-scale outflow in the same line.
		scale := amounts.netMovement.Scale()
		for _, amount := range []*exact.ScaledInt{amounts.inflow, amounts.outflow, operatingNet, amounts.transferIn, amounts.transferOut} {
			if amount.Scale() > scale {
				scale = amount.Scale()
			}
		}

		total := CashflowBucketTotal{CommodityID: commodityID, QuantityScale: scale}
		for _, field := range []struct {
			amount *exact.ScaledInt
			target *exact.Coefficient
		}{
			{amounts.inflow, &total.Inflow},
			{amounts.outflow, &total.Outflow},
			{operatingNet, &total.OperatingNet},
			{amounts.transferIn, &total.TransferIn},
			{amounts.transferOut, &total.TransferOut},
			{amounts.netMovement, &total.NetMovement},
		} {
			// TruncatedTo is lossless when widening, which is the only
			// direction used here: scale is the max of all six.
			value, err := field.amount.TruncatedTo(scale).Coefficient()
			if err != nil {
				return nil, LedgerOverflowError{CommodityID: commodityID}
			}
			*field.target = value
		}
		totals = append(totals, total)
	}

	sort.Slice(totals, func(i, j int) bool {
		return totals[i].CommodityID < totals[j].CommodityID
	})
	return totals, nil
}

func cleanCashflowInput(input CashflowReportInput) (string, string, string, error) {
	startDate, err := cleanRequiredDate(strings.TrimSpace(input.StartDate), "start date")
	if err != nil {
		return "", "", "", err
	}
	endDate, err := cleanRequiredDate(strings.TrimSpace(input.EndDate), "end date")
	if err != nil {
		return "", "", "", err
	}
	if startDate > endDate {
		return "", "", "", ValidationError{Message: "start date must be on or before end date"}
	}
	bucket := strings.TrimSpace(input.Bucket)
	if bucket == "" {
		bucket = "month"
	}
	if !reportBuckets[bucket] {
		return "", "", "", ValidationError{Message: "bucket must be day, week, month, quarter, or year"}
	}
	return startDate, endDate, bucket, nil
}
