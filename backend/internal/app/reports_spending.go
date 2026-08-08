package app

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// SpendingReport answers "where did money go, and who received it?" over an
// inclusive date range, grouped by exactly one dimension.
//
// Scope and sign, per docs/plans/reports-plan.md:
//
//   - The report operates over income/expense **category postings**, not an
//     inferred bank-statement classification. Transfers between the user's own
//     accounts therefore never appear: a transfer has no category leg.
//   - System accounts are excluded, so the `commodity_trading` clearing leg of
//     an investment trade cannot be mistaken for spending.
//   - Values are returned as positive magnitudes in the direction requested. A
//     refund is a credit against an expense account and so reduces its total,
//     which can legitimately leave a group negative — that is a real net
//     inflow for that category, not an error to be clamped away.
//   - Posted, non-voided, non-soft-deleted records only.
type SpendingReportInput struct {
	StartDate string
	EndDate   string
	// GroupBy is "category" or "payee". R2 supports one dimension per request;
	// a nested category-then-payee pivot is follow-up work.
	GroupBy string
	// Direction is "expense" (default) or "income". Income is a separate,
	// explicitly selected mode and is labelled as income, never as spending.
	Direction string
}

// SpendingGroupTotal is one commodity's exact total for one group, plus that
// group's share of the report within the same commodity.
type SpendingGroupTotal struct {
	BalanceQuantity
	// ShareBasisPoints is the group's share of the report-wide total for this
	// commodity, in hundredths of a percent (10000 = 100%). It is an integer
	// so no float enters the money path, and it is presentational only — the
	// exact figure a user acts on is the quantity, never the share.
	//
	// Zero when the report-wide total for the commodity is zero, since a share
	// of nothing has no meaning.
	ShareBasisPoints int64
}

// SpendingGroup is one row of the ranking: one category or one payee.
type SpendingGroup struct {
	// CategoryAccountID is set when grouping by category. The backend does not
	// resolve a display label for it: built-in categories carry a localized
	// label that only the frontend can render, so the caller joins this ID
	// against the categories read model the way it already does elsewhere.
	CategoryAccountID *int64
	// PayeeID and PayeeLabel are set when grouping by payee. Payee names are
	// canonical user data rather than localized built-ins, so unlike categories
	// the label is resolved here.
	PayeeID    *int64
	PayeeLabel string
	// Unassigned marks the bucket for activity with no payee at all. It is
	// reported rather than dropped, because silently omitting uncategorised
	// money makes the totals stop reconciling.
	Unassigned bool
	Totals     []SpendingGroupTotal
}

// SpendingReportResult carries the effective query alongside the data so an
// exported or printed report can still be understood later.
type SpendingReportResult struct {
	StartDate string
	EndDate   string
	GroupBy   string
	Direction string
	// RankCommodityID names the commodity the group ordering was computed in.
	// Groups are ranked by a single commodity's magnitude because comparing
	// unlike commodities as one number is exactly what the plan forbids; the
	// response says which one so the ordering is explainable rather than
	// arbitrary. Zero when the report is empty.
	RankCommodityID int64
	// CommodityTotals is the report-wide total per commodity — the denominator
	// each group's share is taken against.
	CommodityTotals     []BalanceQuantity
	Groups              []SpendingGroup
	ExcludedSystemRoles []string
}

var spendingGroupBys = map[string]bool{
	"category": true,
	"payee":    true,
}

var spendingDirections = map[string]bool{
	"expense": true,
	"income":  true,
}

// Spending returns category or payee totals for an inclusive date range.
func (s *TransactionService) Spending(ctx context.Context, input SpendingReportInput) (SpendingReportResult, error) {
	startDate, endDate, groupBy, direction, err := cleanSpendingInput(input)
	if err != nil {
		return SpendingReportResult{}, err
	}

	// Account versions are read as of the end date so a category renamed or
	// reparented mid-period still resolves.
	accounts, err := s.repository.LedgerAccountsAsOf(ctx, BookID, endDate)
	if err != nil {
		return SpendingReportResult{}, fmt.Errorf("read spending accounts: %w", err)
	}
	postings, err := s.repository.LedgerCategoryPostings(ctx, BookID, startDate, endDate, "posted", direction)
	if err != nil {
		return SpendingReportResult{}, fmt.Errorf("read spending postings: %w", err)
	}
	accountMap := ledgerAccountMap(accounts)

	grouped := map[spendingGroupKey]map[int64]*exact.ScaledInt{}
	reportTotals := map[int64]*exact.ScaledInt{}
	for _, posting := range postings {
		account, ok := accountMap[posting.AccountID]
		// A posting whose account resolves to a system role or to a class other
		// than the requested direction is not spending in this direction. The
		// SQL already filters on both, so this is a defensive second gate
		// rather than the primary one.
		if !ok || account.SystemRole.Valid || account.AccountClass != direction {
			continue
		}

		key := spendingGroupKeyFor(groupBy, posting)
		amounts := grouped[key]
		if amounts == nil {
			amounts = map[int64]*exact.ScaledInt{}
			grouped[key] = amounts
		}
		addPosting(amounts, posting)
		addPosting(reportTotals, posting)
	}

	// balanceMapToQuantities applies the normal-sign convention for the class,
	// which is what turns debit-positive storage into a positive expense
	// magnitude and a positive income magnitude.
	commodityTotals, err := balanceMapToQuantities(reportTotals, direction)
	if err != nil {
		return SpendingReportResult{}, err
	}
	sortBalanceQuantities(commodityTotals)

	rankCommodityID := rankingCommodity(commodityTotals)
	reportTotalByCommodity := map[int64]BalanceQuantity{}
	for _, total := range commodityTotals {
		reportTotalByCommodity[total.CommodityID] = total
	}

	payeeLabels, err := s.spendingPayeeLabels(ctx, groupBy, grouped)
	if err != nil {
		return SpendingReportResult{}, err
	}

	groups := make([]SpendingGroup, 0, len(grouped))
	for key, amounts := range grouped {
		quantities, err := balanceMapToQuantities(amounts, direction)
		if err != nil {
			return SpendingReportResult{}, err
		}
		sortBalanceQuantities(quantities)

		totals := make([]SpendingGroupTotal, 0, len(quantities))
		for _, quantity := range quantities {
			totals = append(totals, SpendingGroupTotal{
				BalanceQuantity:  quantity,
				ShareBasisPoints: shareBasisPoints(quantity, reportTotalByCommodity[quantity.CommodityID]),
			})
		}

		group := SpendingGroup{Totals: totals}
		switch {
		case key.unassigned:
			group.Unassigned = true
		case groupBy == "category":
			categoryID := key.id
			group.CategoryAccountID = &categoryID
		default:
			payeeID := key.id
			group.PayeeID = &payeeID
			group.PayeeLabel = payeeLabels[key.id]
		}
		groups = append(groups, group)
	}
	sortSpendingGroups(groups, rankCommodityID)

	return SpendingReportResult{
		StartDate:           startDate,
		EndDate:             endDate,
		GroupBy:             groupBy,
		Direction:           direction,
		RankCommodityID:     rankCommodityID,
		CommodityTotals:     commodityTotals,
		Groups:              groups,
		ExcludedSystemRoles: []string{"commodity_trading", "transfer_clearing"},
	}, nil
}

// spendingGroupKey identifies one row of the ranking. `unassigned` is a
// distinct field rather than a sentinel id so a real payee with id 0 could
// never collide with the no-payee bucket.
type spendingGroupKey struct {
	id         int64
	unassigned bool
}

func spendingGroupKeyFor(groupBy string, posting db.LedgerPostingRecord) spendingGroupKey {
	if groupBy == "category" {
		return spendingGroupKey{id: posting.AccountID}
	}
	if posting.PayeeID.Valid {
		return spendingGroupKey{id: posting.PayeeID.Int64}
	}
	// A free-text payee_name without a payee record is not a group of its own:
	// two spellings of the same shop would rank as two payees and invite the
	// user to act on a split that is not real. It falls into the unassigned
	// bucket, which the UI names explicitly.
	return spendingGroupKey{unassigned: true}
}

func (s *TransactionService) spendingPayeeLabels(ctx context.Context, groupBy string, grouped map[spendingGroupKey]map[int64]*exact.ScaledInt) (map[int64]string, error) {
	if groupBy != "payee" {
		return map[int64]string{}, nil
	}
	payeeIDs := make([]int64, 0, len(grouped))
	for key := range grouped {
		if !key.unassigned {
			payeeIDs = append(payeeIDs, key.id)
		}
	}
	labels, err := s.payeeRepository.PayeeNamesByID(ctx, BookID, payeeIDs)
	if err != nil {
		return nil, fmt.Errorf("read spending payee names: %w", err)
	}
	return labels, nil
}

// shareBasisPoints computes group/total in hundredths of a percent using exact
// integer arithmetic, rounding half away from zero.
//
// The two quantities may carry different scales, so both are restated on a
// common scale before dividing — the same alignment rule as exact.ScaledInt,
// applied here because this is a ratio rather than a money value.
func shareBasisPoints(group BalanceQuantity, total BalanceQuantity) int64 {
	groupValue, ok := new(big.Int).SetString(group.NormalQuantityValue.String(), 10)
	if !ok {
		return 0
	}
	totalValue, ok := new(big.Int).SetString(total.NormalQuantityValue.String(), 10)
	if !ok || totalValue.Sign() == 0 {
		return 0
	}

	scale := group.QuantityScale
	if total.QuantityScale > scale {
		scale = total.QuantityScale
	}
	groupValue.Mul(groupValue, exact.Pow10(scale-group.QuantityScale))
	totalValue.Mul(totalValue, exact.Pow10(scale-total.QuantityScale))

	// basis points = round(group * 10000 / total)
	numerator := new(big.Int).Mul(groupValue, big.NewInt(10000))
	quotient, remainder := new(big.Int).QuoRem(numerator, totalValue, new(big.Int))

	// Round half away from zero: compare 2*|remainder| against |total|.
	doubled := new(big.Int).Abs(remainder)
	doubled.Lsh(doubled, 1)
	if doubled.CmpAbs(totalValue) >= 0 {
		if (numerator.Sign() < 0) != (totalValue.Sign() < 0) {
			quotient.Sub(quotient, big.NewInt(1))
		} else {
			quotient.Add(quotient, big.NewInt(1))
		}
	}

	if !quotient.IsInt64() {
		return 0
	}
	return quotient.Int64()
}

// rankingCommodity picks the commodity the group ordering is computed in: the
// one with the largest report-wide magnitude, so the ranking reflects where
// most of the money is. Ties break on the lowest commodity id to keep the
// ordering stable across requests.
func rankingCommodity(totals []BalanceQuantity) int64 {
	var best BalanceQuantity
	var bestMagnitude *big.Int
	for _, total := range totals {
		magnitude, ok := new(big.Int).SetString(total.NormalQuantityValue.String(), 10)
		if !ok {
			continue
		}
		magnitude.Abs(magnitude)
		// Restate on a common scale before comparing, so a 2-scale euro total
		// is not judged smaller than an 18-scale crypto total purely because
		// the latter carries more digits.
		magnitude.Mul(magnitude, exact.Pow10(maxQuantityScale(totals)-total.QuantityScale))
		if bestMagnitude == nil || magnitude.Cmp(bestMagnitude) > 0 {
			bestMagnitude = magnitude
			best = total
		}
	}
	return best.CommodityID
}

func maxQuantityScale(totals []BalanceQuantity) int {
	maxScale := 0
	for _, total := range totals {
		if total.QuantityScale > maxScale {
			maxScale = total.QuantityScale
		}
	}
	return maxScale
}

func sortBalanceQuantities(quantities []BalanceQuantity) {
	sort.Slice(quantities, func(i, j int) bool {
		return quantities[i].CommodityID < quantities[j].CommodityID
	})
}

// sortSpendingGroups ranks by descending magnitude in the ranking commodity.
// Groups without a total in that commodity sort after those that have one, and
// every tie falls back to a stable identity order so repeated requests return
// the same sequence.
func sortSpendingGroups(groups []SpendingGroup, rankCommodityID int64) {
	magnitude := func(group SpendingGroup) (*big.Int, int, bool) {
		for _, total := range group.Totals {
			if total.CommodityID != rankCommodityID {
				continue
			}
			value, ok := new(big.Int).SetString(total.NormalQuantityValue.String(), 10)
			if !ok {
				return nil, 0, false
			}
			return value.Abs(value), total.QuantityScale, true
		}
		return nil, 0, false
	}

	sort.SliceStable(groups, func(i, j int) bool {
		leftValue, leftScale, leftOK := magnitude(groups[i])
		rightValue, rightScale, rightOK := magnitude(groups[j])
		if leftOK != rightOK {
			return leftOK
		}
		if leftOK {
			scale := leftScale
			if rightScale > scale {
				scale = rightScale
			}
			left := new(big.Int).Mul(leftValue, exact.Pow10(scale-leftScale))
			right := new(big.Int).Mul(rightValue, exact.Pow10(scale-rightScale))
			if cmp := left.Cmp(right); cmp != 0 {
				return cmp > 0
			}
		}
		return spendingGroupIdentity(groups[i]) < spendingGroupIdentity(groups[j])
	})
}

// spendingGroupIdentity is a stable tiebreak key. The unassigned bucket sorts
// last among equals so it never displaces a named group.
func spendingGroupIdentity(group SpendingGroup) string {
	switch {
	case group.CategoryAccountID != nil:
		return fmt.Sprintf("category:%020d", *group.CategoryAccountID)
	case group.PayeeID != nil:
		return fmt.Sprintf("payee:%020d", *group.PayeeID)
	default:
		return "~unassigned"
	}
}

func cleanSpendingInput(input SpendingReportInput) (string, string, string, string, error) {
	startDate, err := cleanRequiredDate(strings.TrimSpace(input.StartDate), "start date")
	if err != nil {
		return "", "", "", "", err
	}
	endDate, err := cleanRequiredDate(strings.TrimSpace(input.EndDate), "end date")
	if err != nil {
		return "", "", "", "", err
	}
	if startDate > endDate {
		return "", "", "", "", ValidationError{Message: "start date must be on or before end date"}
	}

	groupBy := strings.TrimSpace(input.GroupBy)
	if groupBy == "" {
		groupBy = "category"
	}
	if !spendingGroupBys[groupBy] {
		return "", "", "", "", ValidationError{Message: "group by must be category or payee"}
	}

	direction := strings.TrimSpace(input.Direction)
	if direction == "" {
		direction = "expense"
	}
	if !spendingDirections[direction] {
		return "", "", "", "", ValidationError{Message: "direction must be expense or income"}
	}

	return startDate, endDate, groupBy, direction, nil
}
