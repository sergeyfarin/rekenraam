package app

import (
	"math/big"
	"slices"
	"sort"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// R13 allocation at the closing day (docs/plans/investment-returns-plan.md,
// slice 4). It reads the same image, scope and rates as the closing value,
// so its denominator is that value exactly. Read-only, like the returns.

const (
	AllocationBasisClosingValue = "closing_value"
	AllocationBasisKnownOnly    = "known_only"

	AllocationSideLong  = "long"
	AllocationSideShort = "short"

	// AllocationAssetTypeCash groups currency balances; unclassified is a
	// non-currency commodity with no instrument version effective that day.
	AllocationAssetTypeCash         = "cash"
	AllocationAssetTypeUnclassified = "unclassified"

	ReturnReasonNonPositiveValue = "non_positive_value"
)

// ReturnsAllocation splits the closing portfolio value three ways. Weights
// are shares of Denominator: the closing value when every balance is valued,
// else the known values only (Basis says which). Long and short balances are
// separate rows of a group, never netted inside one label.
type ReturnsAllocation struct {
	Date string
	// Reason makes the whole allocation unavailable (an unclassified short,
	// as for the closing value); nothing else is filled then.
	Reason        string
	Basis         string
	Denominator   *BalanceQuantity
	WeightsReason string
	ByAccount     []ReturnsAllocationRow
	ByAssetType   []ReturnsAllocationRow
	ByCurrency    []ReturnsAllocationRow
	Unknown       []ReturnsAllocationUnknown
}

// ReturnsAllocationRow is one side of one group. Exactly one of AccountID,
// AssetType or CurrencyID names the group, by dimension; a zero CurrencyID
// is a security with no quote currency on the day.
type ReturnsAllocationRow struct {
	AccountID  int64
	AssetType  string
	CurrencyID int64
	Side       string
	Value      *BalanceQuantity
	Weight     *string
}

// ReturnsAllocationUnknown is a balance with no usable price or rate: its
// units are known, its value is not, and it is in no weight.
type ReturnsAllocationUnknown struct {
	AccountID int64
	Quantity  BalanceQuantity
}

type allocationKey struct {
	accountID  int64
	assetType  string
	currencyID int64
	side       string
}

func (r *returnsReader) allocation(closing returnsValuation, classes map[int64]db.ReturnsInstrumentClass) ReturnsAllocation {
	result := ReturnsAllocation{Date: r.to}
	if closing.reason == ReturnReasonUnclassifiedShort {
		result.Reason = closing.reason
		return result
	}
	running := map[int64]map[int64]*exact.ScaledInt{}
	for _, posting := range r.postings {
		if posting.EntryDate <= r.to && r.inScope[posting.AccountID] {
			foldPosting(running, posting)
		}
	}

	groups := [3]map[allocationKey]*big.Rat{{}, {}, {}}
	denominator := new(big.Rat)
	result.Basis = AllocationBasisClosingValue
	for _, accountID := range sortedKeys(running) {
		for _, commodityID := range sortedKeys(running[accountID]) {
			balance := running[accountID][commodityID]
			if balance.Sign() == 0 {
				continue
			}
			coefficient, scale, ok := r.fit(balance.BigInt(), balance.Scale(), commodityID)
			if !ok {
				// The read fails with the overflow; nothing here is reported.
				continue
			}
			value, ok := r.rates.ConvertExact(coefficient, scale, commodityID, r.to)
			if !ok {
				result.Basis = AllocationBasisKnownOnly
				result.Unknown = append(result.Unknown, ReturnsAllocationUnknown{AccountID: accountID, Quantity: BalanceQuantity{
					CommodityID: commodityID, QuantityValue: coefficient, QuantityScale: scale, NormalQuantityValue: coefficient}})
				continue
			}
			denominator.Add(denominator, value)
			side := AllocationSideLong
			if value.Sign() < 0 {
				side = AllocationSideShort
			}
			assetType, currencyID := r.allocationClass(commodityID, classes)
			for i, key := range []allocationKey{
				{accountID: r.topPortfolioAccount(accountID), side: side},
				{assetType: assetType, side: side},
				{currencyID: currencyID, side: side},
			} {
				if groups[i][key] == nil {
					groups[i][key] = new(big.Rat)
				}
				groups[i][key].Add(groups[i][key], value)
			}
		}
	}
	result.Denominator = r.quantity(denominator)
	if denominator.Sign() <= 0 {
		result.WeightsReason = ReturnReasonNonPositiveValue
	}
	result.ByAccount = r.allocationRows(groups[0], denominator, result.WeightsReason == "")
	result.ByAssetType = r.allocationRows(groups[1], denominator, result.WeightsReason == "")
	result.ByCurrency = r.allocationRows(groups[2], denominator, result.WeightsReason == "")
	return result
}

// allocationClass is a commodity's asset type and currency on the day: a
// currency is cash in itself; anything else takes its instrument version's
// type and quote currency, or is unclassified with no currency.
func (r *returnsReader) allocationClass(commodityID int64, classes map[int64]db.ReturnsInstrumentClass) (string, int64) {
	if r.kinds[commodityID] == "currency" {
		return AllocationAssetTypeCash, commodityID
	}
	class, ok := classes[commodityID]
	if !ok {
		return AllocationAssetTypeUnclassified, 0
	}
	return class.InstrumentType, class.QuoteCommodityID.Int64
}

// topPortfolioAccount is the outermost in-scope ancestor: the brokerage a
// holding sits under, or the account itself.
func (r *returnsReader) topPortfolioAccount(accountID int64) int64 {
	for seen := 0; seen < len(r.accounts); seen++ {
		parent := r.accounts[accountID].ParentAccountID
		if !parent.Valid || !r.inScope[parent.Int64] {
			break
		}
		accountID = parent.Int64
	}
	return accountID
}

// allocationRows orders the largest magnitude first, then by side and key,
// so a response is deterministic.
func (r *returnsReader) allocationRows(groups map[allocationKey]*big.Rat, denominator *big.Rat, weighted bool) []ReturnsAllocationRow {
	keys := make([]allocationKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := new(big.Rat).Abs(groups[keys[i]]), new(big.Rat).Abs(groups[keys[j]])
		if c := a.Cmp(b); c != 0 {
			return c > 0
		}
		if keys[i].side != keys[j].side {
			return keys[i].side < keys[j].side
		}
		if keys[i].accountID != keys[j].accountID {
			return keys[i].accountID < keys[j].accountID
		}
		if keys[i].assetType != keys[j].assetType {
			return keys[i].assetType < keys[j].assetType
		}
		return keys[i].currencyID < keys[j].currencyID
	})
	rows := make([]ReturnsAllocationRow, 0, len(keys))
	for _, key := range keys {
		row := ReturnsAllocationRow{AccountID: key.accountID, AssetType: key.assetType, CurrencyID: key.currencyID,
			Side: key.side, Value: r.quantity(groups[key])}
		if weighted {
			weight := formatReturnRat(new(big.Rat).Quo(groups[key], denominator))
			row.Weight = &weight
		}
		rows = append(rows, row)
	}
	return rows
}

func sortedKeys[V any](values map[int64]V) []int64 {
	keys := make([]int64, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
