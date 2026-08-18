package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// ReportFilters is the shared `/api/v1/reports/*` filter contract. Repeated IDs
// within one dimension are an OR-set; different dimensions combine with AND.
// Empty means "no restriction on that dimension" — never "everything except".
type ReportFilters struct {
	AccountIDs         []int64
	IncludeDescendants bool
	CommodityIDs       []int64
}

// ReportFiltersEcho is the effective filter set echoed back on every report
// response, so an exported or printed report can be understood later. Account
// IDs are echoed as requested, not as expanded; ResolvedAccountIDs carries the
// expansion the calculation actually used.
type ReportFiltersEcho struct {
	AccountIDs         []int64
	IncludeDescendants bool
	CommodityIDs       []int64
	ResolvedAccountIDs []int64
}

// spendingModes maps the requested report mode onto the account class whose
// postings form the basis. Income is a distinct, explicitly selected mode — it
// is never folded into a "spending" number.
var spendingModes = map[string]string{
	"spending": "expense",
	"income":   "income",
}

var spendingGroupings = map[string]bool{
	"category": true,
	"payee":    true,
}

type SpendingInput struct {
	StartDate   string
	EndDate     string
	GroupBy     string
	Mode        string
	CategoryIDs []int64
	PayeeIDs    []int64
	Filters     ReportFilters
}

// SpendingGroupTotal is one commodity's exact total for a group, plus that
// group's share of the same commodity's report total.
type SpendingGroupTotal struct {
	CommodityID   int64
	QuantityValue exact.Coefficient
	QuantityScale int
	// ShareBasisPoints is the group's share of this commodity's report total in
	// basis points (10000 = 100%), rounded half-up. Shares are only ever
	// computed within one commodity — unlike commodities are never compared as
	// one number. Absent when the commodity's report total is zero, and signed
	// when a group nets negative (a category dominated by refunds).
	ShareBasisPoints *int64
}

type SpendingGroup struct {
	// CategoryID / PayeeID is set according to the grouping. A nil PayeeID on a
	// payee grouping is the real "no payee recorded" group: those postings are
	// money that moved, so dropping them would stop the ranking reconciling to
	// the commodity totals.
	CategoryID   *int64
	PayeeID      *int64
	Code         string
	Name         string
	CategoryType string
	Totals       []SpendingGroupTotal
}

type SpendingResult struct {
	StartDate           string
	EndDate             string
	GroupBy             string
	Mode                string
	CategoryIDs         []int64
	PayeeIDs            []int64
	Filters             ReportFiltersEcho
	Groups              []SpendingGroup
	CommodityTotals     []BalanceQuantity
	ExcludedSystemRoles []string
	// GroupingPolicy names how rows were formed, so a printed report is not
	// ambiguous about whether parent categories include their children.
	GroupingPolicy string
}

// Spending answers "where did money go, and who received it" over the posted
// ledger. Drafts, voided transactions, and soft-deleted transactions are never
// part of financial truth and never appear here.
//
// Transfers need no explicit exclusion: the basis is income/expense category
// postings only, and an asset-to-asset transfer has none. That is the plan's
// deliberate choice over inferring a bank-statement classification.
func (s *TransactionService) Spending(ctx context.Context, input SpendingInput) (SpendingResult, error) {
	startDate, endDate, err := cleanReportDateRange(input.StartDate, input.EndDate)
	if err != nil {
		return SpendingResult{}, err
	}
	groupBy := strings.TrimSpace(input.GroupBy)
	if !spendingGroupings[groupBy] {
		return SpendingResult{}, ValidationError{Message: "group by must be category or payee"}
	}
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "spending"
	}
	categoryType, ok := spendingModes[mode]
	if !ok {
		return SpendingResult{}, ValidationError{Message: "mode must be spending or income"}
	}

	categoryIDs := dedupeIDs(input.CategoryIDs)
	payeeIDs := dedupeIDs(input.PayeeIDs)
	filters, resolvedAccountIDs, err := s.resolveReportFilters(ctx, input.Filters, endDate)
	if err != nil {
		return SpendingResult{}, err
	}
	if err := s.ensureCategoriesExist(ctx, categoryIDs); err != nil {
		return SpendingResult{}, err
	}
	if err := s.ensurePayeesExist(ctx, payeeIDs); err != nil {
		return SpendingResult{}, err
	}

	postings, err := s.repository.ReportCategoryPostings(ctx, db.ReportCategoryPostingsParams{
		BookID:       BookID,
		StartDate:    startDate,
		EndDate:      endDate,
		CategoryType: categoryType,
		CategoryIDs:  categoryIDs,
		PayeeIDs:     payeeIDs,
		AccountIDs:   resolvedAccountIDs,
		CommodityIDs: filters.CommodityIDs,
	})
	if err != nil {
		return SpendingResult{}, fmt.Errorf("read spending postings: %w", err)
	}

	groups, commodityTotals, err := s.aggregateSpending(ctx, postings, groupBy, categoryType, endDate)
	if err != nil {
		return SpendingResult{}, err
	}

	return SpendingResult{
		StartDate:           startDate,
		EndDate:             endDate,
		GroupBy:             groupBy,
		Mode:                mode,
		CategoryIDs:         categoryIDs,
		PayeeIDs:            payeeIDs,
		Filters:             filters,
		Groups:              groups,
		CommodityTotals:     commodityTotals,
		ExcludedSystemRoles: []string{"commodity_trading", "transfer_clearing", "opening_balances", "fx_gain_loss"},
		GroupingPolicy:      "direct_postings",
	}, nil
}

// aggregateSpending sums postings per group and commodity in Go — coefficients
// are strings in SQLite and must never be aggregated with SQL SUM — then
// converts to presentation magnitudes and computes within-commodity shares.
func (s *TransactionService) aggregateSpending(
	ctx context.Context,
	postings []db.ReportCategoryPostingRecord,
	groupBy string,
	categoryType string,
	asOf string,
) ([]SpendingGroup, []BalanceQuantity, error) {
	type groupKey struct {
		id       int64
		hasID    bool
		isCatKey bool
	}

	amounts := map[groupKey]map[int64]*exact.ScaledInt{}
	reportTotals := map[int64]*exact.ScaledInt{}
	order := make([]groupKey, 0)

	for _, posting := range postings {
		key := groupKey{isCatKey: groupBy == "category"}
		if groupBy == "category" {
			key.id, key.hasID = posting.AccountID, true
		} else if posting.PayeeID.Valid {
			key.id, key.hasID = posting.PayeeID.Int64, true
		}

		byCommodity := amounts[key]
		if byCommodity == nil {
			byCommodity = map[int64]*exact.ScaledInt{}
			amounts[key] = byCommodity
			order = append(order, key)
		}
		addReportPosting(byCommodity, posting)
		addReportPosting(reportTotals, posting)
	}

	// Income postings are credits under the debit-positive convention; present
	// them as positive income magnitudes. Expense postings are already positive,
	// and refunds stay negative so they reduce the total.
	negate := categoryType == "income"

	commodityTotals, err := reportMagnitudes(reportTotals, negate)
	if err != nil {
		return nil, nil, err
	}
	totalsByCommodity := make(map[int64]BalanceQuantity, len(commodityTotals))
	for _, total := range commodityTotals {
		totalsByCommodity[total.CommodityID] = total
	}

	categoryNames, payeeNames, err := s.spendingGroupLabels(ctx, postings, groupBy, asOf)
	if err != nil {
		return nil, nil, err
	}

	groups := make([]SpendingGroup, 0, len(order))
	for _, key := range order {
		magnitudes, err := reportMagnitudes(amounts[key], negate)
		if err != nil {
			return nil, nil, err
		}
		totals := make([]SpendingGroupTotal, 0, len(magnitudes))
		for _, magnitude := range magnitudes {
			share, err := shareBasisPoints(magnitude, totalsByCommodity[magnitude.CommodityID])
			if err != nil {
				return nil, nil, err
			}
			totals = append(totals, SpendingGroupTotal{
				CommodityID:      magnitude.CommodityID,
				QuantityValue:    magnitude.QuantityValue,
				QuantityScale:    magnitude.QuantityScale,
				ShareBasisPoints: share,
			})
		}

		group := SpendingGroup{CategoryType: categoryType, Totals: totals}
		if key.hasID {
			id := key.id
			if key.isCatKey {
				group.CategoryID = &id
				group.Code = categoryNames[id].code
				group.Name = categoryNames[id].name
			} else {
				group.PayeeID = &id
				group.Name = payeeNames[id]
			}
		}
		groups = append(groups, group)
	}

	sortSpendingGroups(groups)
	return groups, commodityTotals, nil
}

type spendingCategoryLabel struct {
	code string
	name string
}

func (s *TransactionService) spendingGroupLabels(
	ctx context.Context,
	postings []db.ReportCategoryPostingRecord,
	groupBy string,
	asOf string,
) (map[int64]spendingCategoryLabel, map[int64]string, error) {
	if len(postings) == 0 {
		return map[int64]spendingCategoryLabel{}, map[int64]string{}, nil
	}

	if groupBy == "category" {
		accounts, err := s.repository.LedgerAccountsAsOf(ctx, BookID, asOf)
		if err != nil {
			return nil, nil, fmt.Errorf("read spending category labels: %w", err)
		}
		labels := make(map[int64]spendingCategoryLabel, len(accounts))
		for _, account := range accounts {
			labels[account.ID] = spendingCategoryLabel{
				code: nullableString(account.Code),
				name: nullableString(account.Name),
			}
		}
		return labels, map[int64]string{}, nil
	}

	payeeIDs := make([]int64, 0)
	seen := map[int64]bool{}
	for _, posting := range postings {
		if posting.PayeeID.Valid && !seen[posting.PayeeID.Int64] {
			seen[posting.PayeeID.Int64] = true
			payeeIDs = append(payeeIDs, posting.PayeeID.Int64)
		}
	}
	names, err := s.payeeRepository.ReportPayeeNames(ctx, BookID, payeeIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("read spending payee labels: %w", err)
	}
	return map[int64]spendingCategoryLabel{}, names, nil
}

// sortSpendingGroups ranks by the largest single-commodity magnitude, then by a
// stable identity tiebreak. Magnitudes across unlike commodities are never
// added together to build the ranking key.
func sortSpendingGroups(groups []SpendingGroup) {
	rankOf := func(group SpendingGroup) *big.Int {
		best := big.NewInt(0)
		for i, total := range group.Totals {
			// Compare whole units so a high-scale crypto amount does not outrank
			// a larger fiat amount purely on digit count.
			scaled := exact.ScaledIntFromCoefficient(total.QuantityValue, total.QuantityScale)
			value := scaled.BigInt()
			if scaled.Scale() > 0 {
				value.Quo(value, exact.Pow10(scaled.Scale()))
			}
			if i == 0 || value.Cmp(best) > 0 {
				best = value
			}
		}
		return best
	}

	sort.SliceStable(groups, func(i, j int) bool {
		left, right := rankOf(groups[i]), rankOf(groups[j])
		if cmp := right.Cmp(left); cmp != 0 {
			return cmp < 0
		}
		return spendingGroupIdentity(groups[i]) < spendingGroupIdentity(groups[j])
	})
}

// spendingGroupIdentity is a stable tiebreak. The unattributed group sorts last
// within a tie rather than colliding with a real id 0.
func spendingGroupIdentity(group SpendingGroup) int64 {
	switch {
	case group.CategoryID != nil:
		return *group.CategoryID
	case group.PayeeID != nil:
		return *group.PayeeID
	default:
		return int64(^uint64(0) >> 1)
	}
}

// shareBasisPoints computes value/total in basis points (10000 = 100%), rounded
// half-up on magnitude, using integer arithmetic only — never floating point in
// the money path. A zero or absent denominator yields no share rather than a
// fabricated one.
func shareBasisPoints(value BalanceQuantity, total BalanceQuantity) (*int64, error) {
	if total.QuantityValue == "" {
		return nil, nil
	}
	numerator := exact.ScaledIntFromCoefficient(value.QuantityValue, value.QuantityScale)
	denominator := exact.ScaledIntFromCoefficient(total.QuantityValue, total.QuantityScale)
	if denominator.Sign() == 0 {
		return nil, nil
	}

	// Align both sides to the same scale so the ratio is scale-independent.
	scale := numerator.Scale()
	if denominator.Scale() > scale {
		scale = denominator.Scale()
	}
	num := alignedBigInt(numerator, scale)
	den := alignedBigInt(denominator, scale)

	// Normalize the denominator positive so rounding direction is unambiguous.
	if den.Sign() < 0 {
		num.Neg(num)
		den.Neg(den)
	}

	negative := num.Sign() < 0
	if negative {
		num.Neg(num)
	}
	// half-up on a non-negative magnitude: (2*n*10000 + d) / (2*d).
	num.Mul(num, big.NewInt(20000))
	num.Add(num, den)
	den.Mul(den, big.NewInt(2))
	num.Quo(num, den)
	if negative {
		num.Neg(num)
	}

	if !num.IsInt64() {
		return nil, LedgerOverflowError{CommodityID: value.CommodityID}
	}
	share := num.Int64()
	return &share, nil
}

func alignedBigInt(value *exact.ScaledInt, scale int) *big.Int {
	aligned := value.BigInt()
	if scale > value.Scale() {
		aligned.Mul(aligned, exact.Pow10(scale-value.Scale()))
	}
	return aligned
}

// normalizeReportFilters drops non-positive and duplicate IDs and sorts what
// remains, so every report path starts from the same selection. It is
// idempotent, which is what lets a caller normalize once and re-expand later
// without the two steps disagreeing.
func normalizeReportFilters(filters ReportFilters) ReportFilters {
	return ReportFilters{
		AccountIDs:         dedupeIDs(filters.AccountIDs),
		IncludeDescendants: filters.IncludeDescendants,
		CommodityIDs:       dedupeIDs(filters.CommodityIDs),
	}
}

// resolveReportFilters validates the shared filter dimensions and expands the
// account selection. Inaccessible IDs are VALIDATION_FAILED, never a silently
// narrower result.
//
// The returned echo carries the normalized selection, so a caller that must
// re-expand per reporting date (net-worth series) feeds the echo back in rather
// than re-deriving the selection from raw input.
func (s *TransactionService) resolveReportFilters(ctx context.Context, filters ReportFilters, asOf string) (ReportFiltersEcho, []int64, error) {
	normalized := normalizeReportFilters(filters)

	for _, accountID := range normalized.AccountIDs {
		if err := s.repository.EnsureAccountInBook(ctx, BookID, accountID); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return ReportFiltersEcho{}, nil, ValidationError{Message: "account filter is invalid"}
			}
			return ReportFiltersEcho{}, nil, fmt.Errorf("check report account filter: %w", err)
		}
	}
	if len(normalized.CommodityIDs) > 0 {
		known, err := s.commodityRepository.CommoditiesByIDs(ctx, BookID, normalized.CommodityIDs)
		if err != nil {
			return ReportFiltersEcho{}, nil, fmt.Errorf("check report commodity filter: %w", err)
		}
		for _, commodityID := range normalized.CommodityIDs {
			if _, ok := known[commodityID]; !ok {
				return ReportFiltersEcho{}, nil, ValidationError{Message: "commodity filter is invalid"}
			}
		}
	}

	resolved, err := s.resolveReportAccountIDs(ctx, normalized, asOf)
	if err != nil {
		return ReportFiltersEcho{}, nil, err
	}

	return ReportFiltersEcho{
		AccountIDs:         normalized.AccountIDs,
		IncludeDescendants: normalized.IncludeDescendants,
		CommodityIDs:       normalized.CommodityIDs,
		ResolvedAccountIDs: resolved,
	}, resolved, nil
}

// echoedReportFilters turns a resolved echo back into the normalized selection
// it came from, for callers that re-expand the same selection at another date.
func echoedReportFilters(echo ReportFiltersEcho) ReportFilters {
	return ReportFilters{
		AccountIDs:         echo.AccountIDs,
		IncludeDescendants: echo.IncludeDescendants,
		CommodityIDs:       echo.CommodityIDs,
	}
}

// resolveReportAccountIDs expands a normalized account selection as of asOf.
// An empty selection stays empty: no account filter, never "match nothing".
func (s *TransactionService) resolveReportAccountIDs(ctx context.Context, normalized ReportFilters, asOf string) ([]int64, error) {
	if len(normalized.AccountIDs) == 0 || !normalized.IncludeDescendants {
		return normalized.AccountIDs, nil
	}
	return s.reportAccountSubtreeIDs(ctx, normalized.AccountIDs, asOf)
}

// reportFilterSet is the in-memory membership test used by reports that filter
// postings in Go rather than in SQL. A nil map means that dimension is
// unrestricted; an empty non-nil map would mean "match nothing", which is never
// what an empty selection means.
type reportFilterSet struct {
	accountIDs   map[int64]bool
	commodityIDs map[int64]bool
}

// reportFilterSetFrom builds the membership test for an already-validated
// selection from the account versions in effect on the reporting date. Parent
// links are versioned, so the expansion is date-dependent and cannot be hoisted
// out of a per-bucket loop; taking the accounts as an argument keeps that loop
// from re-reading them.
func reportFilterSetFrom(accounts []db.LedgerAccountRecord, filters ReportFilters) reportFilterSet {
	normalized := normalizeReportFilters(filters)
	accountIDs := normalized.AccountIDs
	if len(accountIDs) > 0 && normalized.IncludeDescendants {
		accountIDs = accountSubtreeIDs(accounts, accountIDs)
	}
	return reportFilterSet{
		accountIDs:   idSet(accountIDs),
		commodityIDs: idSet(normalized.CommodityIDs),
	}
}

// includes reports whether a posting survives the filter on both dimensions.
func (f reportFilterSet) includes(accountID int64, commodityID int64) bool {
	if f.accountIDs != nil && !f.accountIDs[accountID] {
		return false
	}
	if f.commodityIDs != nil && !f.commodityIDs[commodityID] {
		return false
	}
	return true
}

// idSet returns nil for an empty selection, so "no filter" and "matches
// nothing" stay distinguishable.
func idSet(ids []int64) map[int64]bool {
	if len(ids) == 0 {
		return nil
	}
	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// reportAccountSubtreeIDs expands each selected account to itself plus every
// descendant, using the account versions in effect on asOf. Parent links are
// versioned, so the expansion is date-dependent and must happen here rather
// than in the frontend.
func (s *TransactionService) reportAccountSubtreeIDs(ctx context.Context, accountIDs []int64, asOf string) ([]int64, error) {
	accounts, err := s.repository.LedgerAccountsAsOf(ctx, BookID, asOf)
	if err != nil {
		return nil, fmt.Errorf("read accounts for report subtree: %w", err)
	}
	return accountSubtreeIDs(accounts, accountIDs), nil
}

// accountSubtreeIDs is the expansion itself, over account versions the caller
// already holds.
func accountSubtreeIDs(accounts []db.LedgerAccountRecord, accountIDs []int64) []int64 {
	childrenByParent := map[int64][]int64{}
	for _, account := range accounts {
		if account.ParentAccountID.Valid {
			childrenByParent[account.ParentAccountID.Int64] = append(childrenByParent[account.ParentAccountID.Int64], account.ID)
		}
	}

	resolved := make([]int64, 0, len(accountIDs))
	seen := map[int64]bool{}
	pending := append([]int64(nil), accountIDs...)
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[current] {
			continue
		}
		seen[current] = true
		resolved = append(resolved, current)
		pending = append(pending, childrenByParent[current]...)
	}
	sort.Slice(resolved, func(i, j int) bool { return resolved[i] < resolved[j] })
	return resolved
}

func (s *TransactionService) ensureCategoriesExist(ctx context.Context, categoryIDs []int64) error {
	for _, categoryID := range categoryIDs {
		if err := s.repository.EnsureAccountInBook(ctx, BookID, categoryID); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return ValidationError{Message: "category filter is invalid"}
			}
			return fmt.Errorf("check report category filter: %w", err)
		}
	}
	return nil
}

func (s *TransactionService) ensurePayeesExist(ctx context.Context, payeeIDs []int64) error {
	for _, payeeID := range payeeIDs {
		if _, err := s.payeeRepository.PayeeByID(ctx, BookID, payeeID); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return ValidationError{Message: "payee filter is invalid"}
			}
			return fmt.Errorf("check report payee filter: %w", err)
		}
	}
	return nil
}

func cleanReportDateRange(startInput string, endInput string) (string, string, error) {
	startDate, err := cleanRequiredDate(strings.TrimSpace(startInput), "start date")
	if err != nil {
		return "", "", err
	}
	endDate, err := cleanRequiredDate(strings.TrimSpace(endInput), "end date")
	if err != nil {
		return "", "", err
	}
	if startDate > endDate {
		return "", "", ValidationError{Message: "start date must be on or before end date"}
	}
	return startDate, endDate, nil
}

func addReportPosting(amounts map[int64]*exact.ScaledInt, posting db.ReportCategoryPostingRecord) {
	amount := amounts[posting.CommodityID]
	if amount == nil {
		amount = exact.NewScaledInt()
		amounts[posting.CommodityID] = amount
	}
	amount.AddCoefficient(posting.QuantityValue, posting.QuantityScale)
}

// reportMagnitudes converts accumulated postings into presentation values,
// negating when the basis class is credit-normal (income).
func reportMagnitudes(amounts map[int64]*exact.ScaledInt, negate bool) ([]BalanceQuantity, error) {
	quantities := make([]BalanceQuantity, 0, len(amounts))
	for commodityID, amount := range amounts {
		value, err := amount.Coefficient()
		if err != nil {
			return nil, LedgerOverflowError{CommodityID: commodityID}
		}
		if negate {
			value = value.Negated()
		}
		quantities = append(quantities, BalanceQuantity{
			CommodityID:         commodityID,
			QuantityValue:       value,
			QuantityScale:       amount.Scale(),
			NormalQuantityValue: value,
		})
	}
	sort.Slice(quantities, func(i, j int) bool { return quantities[i].CommodityID < quantities[j].CommodityID })
	return quantities, nil
}

func dedupeIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := map[int64]bool{}
	deduped := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		deduped = append(deduped, id)
	}
	if len(deduped) == 0 {
		return nil
	}
	sort.Slice(deduped, func(i, j int) bool { return deduped[i] < deduped[j] })
	return deduped
}
