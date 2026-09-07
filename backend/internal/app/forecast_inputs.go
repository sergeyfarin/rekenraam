package app

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

type forecastNormalizedInput struct {
	HorizonDays        int
	AccountIDs         []int64
	IncludeDescendants bool
}

type forecastBounds struct {
	AsOf    string
	First   string
	Through string
}

type forecastScope struct {
	AccountIDs []int64
	Accounts   map[int64]db.ForecastAccountVersionRecord
}

func normalizeForecastInput(input ForecastInput) (forecastNormalizedInput, error) {
	horizon := input.HorizonDays
	if horizon == 0 {
		horizon = forecastDefaultHorizonDays
	}
	if horizon < 1 || horizon > forecastMaxHorizonDays {
		return forecastNormalizedInput{}, ValidationError{Message: "forecast horizon must be between 1 and 366 days"}
	}
	seen := map[int64]bool{}
	ids := make([]int64, 0, len(input.AccountIDs))
	for _, id := range input.AccountIDs {
		if id <= 0 {
			return forecastNormalizedInput{}, ValidationError{Message: "forecast account id is invalid"}
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > forecastMaxRootAccounts {
		return forecastNormalizedInput{}, ValidationError{Message: "forecast accepts at most 100 root accounts"}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return forecastNormalizedInput{HorizonDays: horizon, AccountIDs: ids, IncludeDescendants: input.IncludeDescendants}, nil
}

func forecastDateBounds(nowUTC time.Time, timeZone string, horizon int) (forecastBounds, error) {
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		return forecastBounds{}, fmt.Errorf("load forecast time zone: %w", err)
	}
	asOfTime, err := time.Parse(time.DateOnly, nowUTC.In(location).Format(time.DateOnly))
	if err != nil {
		return forecastBounds{}, fmt.Errorf("parse forecast date: %w", err)
	}
	through := asOfTime.AddDate(0, 0, horizon)
	if through.Year() > 9999 {
		return forecastBounds{}, ValidationError{Message: "forecast horizon exceeds the supported calendar"}
	}
	return forecastBounds{AsOf: asOfTime.Format(time.DateOnly), First: asOfTime.AddDate(0, 0, 1).Format(time.DateOnly), Through: through.Format(time.DateOnly)}, nil
}

func resolveForecastScope(versions []db.ForecastAccountVersionRecord, asOf string, input forecastNormalizedInput) (forecastScope, error) {
	current := accountRulesAt(versions, asOf)
	selected := map[int64]bool{}
	if len(input.AccountIDs) == 0 {
		for id, account := range current {
			if defaultForecastAccount(account) {
				selected[id] = true
			}
		}
	} else {
		for _, rootID := range input.AccountIDs {
			root, ok := current[rootID]
			if !ok || root.SystemRole.Valid || (root.AccountClass != "asset" && root.AccountClass != "liability") {
				return forecastScope{}, ValidationError{Message: "forecast account is invalid"}
			}
			if root.AllowsPostings && root.AccountKind == "security_holding" {
				return forecastScope{}, ValidationError{Message: "security holding accounts cannot be forecast"}
			}
			selected[rootID] = true
			if input.IncludeDescendants {
				for id := range current {
					if isForecastDescendant(id, rootID, current) {
						selected[id] = true
					}
				}
			}
		}
	}
	accounts := map[int64]db.ForecastAccountVersionRecord{}
	ids := make([]int64, 0, len(selected))
	for id := range selected {
		account := current[id]
		if account.AllowsPostings && account.AccountKind != "security_holding" && (account.AccountClass == "asset" || account.AccountClass == "liability") {
			accounts[id] = account
			ids = append(ids, id)
		}
	}
	if len(ids) > forecastMaxAccounts {
		return forecastScope{}, ErrForecastTooLarge
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return forecastScope{AccountIDs: ids, Accounts: accounts}, nil
}

func defaultForecastAccount(account db.ForecastAccountVersionRecord) bool {
	if account.SystemRole.Valid || account.Status != "active" || !account.AllowsPostings || (account.AccountClass != "asset" && account.AccountClass != "liability") {
		return false
	}
	switch account.AccountKind {
	case "cash", "checking", "savings", "brokerage_cash":
		return true
	default:
		return false
	}
}

func isForecastDescendant(id, root int64, accounts map[int64]db.ForecastAccountVersionRecord) bool {
	seen := map[int64]bool{}
	for id != root {
		if seen[id] {
			return false
		}
		seen[id] = true
		account, ok := accounts[id]
		if !ok || !account.ParentAccountID.Valid {
			return false
		}
		id = account.ParentAccountID.Int64
	}
	return true
}

func accountRulesAt(versions []db.ForecastAccountVersionRecord, date string) map[int64]db.ForecastAccountVersionRecord {
	result := map[int64]db.ForecastAccountVersionRecord{}
	for _, version := range versions {
		if version.EffectiveFrom > date {
			continue
		}
		current, ok := result[version.AccountID]
		if !ok || version.EffectiveFrom > current.EffectiveFrom || (version.EffectiveFrom == current.EffectiveFrom && version.VersionSeq > current.VersionSeq) {
			result[version.AccountID] = version
		}
	}
	return result
}

func commodityRulesAt(versions []db.ForecastCommodityVersionRecord, date string) map[int64]db.ForecastCommodityVersionRecord {
	result := map[int64]db.ForecastCommodityVersionRecord{}
	for _, version := range versions {
		if version.EffectiveFrom > date {
			continue
		}
		current, ok := result[version.CommodityID]
		if !ok || version.EffectiveFrom > current.EffectiveFrom || (version.EffectiveFrom == current.EffectiveFrom && version.VersionSeq > current.VersionSeq) {
			result[version.CommodityID] = version
		}
	}
	return result
}

type forecastPosting struct {
	AccountID   int64
	CommodityID int64
	Value       exact.Coefficient
	Scale       int
}

type forecastRuleLookup struct {
	accounts    map[int64][]db.ForecastAccountVersionRecord
	commodities map[int64][]db.ForecastCommodityVersionRecord
}

func newForecastRuleLookup(snapshot db.ForecastSnapshot) forecastRuleLookup {
	result := forecastRuleLookup{accounts: map[int64][]db.ForecastAccountVersionRecord{}, commodities: map[int64][]db.ForecastCommodityVersionRecord{}}
	for _, version := range snapshot.AccountVersions {
		result.accounts[version.AccountID] = append(result.accounts[version.AccountID], version)
	}
	for _, version := range snapshot.CommodityVersions {
		result.commodities[version.CommodityID] = append(result.commodities[version.CommodityID], version)
	}
	return result
}

func (r forecastRuleLookup) accountAt(id int64, date string) (db.ForecastAccountVersionRecord, bool) {
	var result db.ForecastAccountVersionRecord
	found := false
	for _, version := range r.accounts[id] {
		if version.EffectiveFrom <= date && (!found || version.EffectiveFrom > result.EffectiveFrom || (version.EffectiveFrom == result.EffectiveFrom && version.VersionSeq > result.VersionSeq)) {
			result, found = version, true
		}
	}
	return result, found
}

func (r forecastRuleLookup) commodityAt(id int64, date string) (db.ForecastCommodityVersionRecord, bool) {
	var result db.ForecastCommodityVersionRecord
	found := false
	for _, version := range r.commodities[id] {
		if version.EffectiveFrom <= date && (!found || version.EffectiveFrom > result.EffectiveFrom || (version.EffectiveFrom == result.EffectiveFrom && version.VersionSeq > result.VersionSeq)) {
			result, found = version, true
		}
	}
	return result, found
}

func validateForecastPostings(postings []forecastPosting, sourceDate, projectionDate string, rules forecastRuleLookup) bool {
	if len(postings) < 2 {
		return false
	}
	totals := map[int64]*exact.ScaledInt{}
	for _, posting := range postings {
		if !forecastPostingAllowed(posting, sourceDate, rules) || (projectionDate != sourceDate && !forecastPostingAllowed(posting, projectionDate, rules)) {
			return false
		}
		if totals[posting.CommodityID] == nil {
			totals[posting.CommodityID] = exact.NewScaledInt()
		}
		totals[posting.CommodityID].AddCoefficient(posting.Value, posting.Scale)
	}
	for _, total := range totals {
		if total.Sign() != 0 {
			return false
		}
	}
	return true
}

func forecastPostingAllowed(posting forecastPosting, date string, rules forecastRuleLookup) bool {
	account, ok := rules.accountAt(posting.AccountID, date)
	if !ok || account.Status != "active" || !account.AllowsPostings || date < account.OpenedOn || (account.ClosedOn.Valid && date > account.ClosedOn.String) {
		return false
	}
	if account.DefaultCommodityID.Valid && account.DefaultCommodityID.Int64 != posting.CommodityID {
		return false
	}
	if account.QuantityScaleOverride.Valid && int64(posting.Scale) > account.QuantityScaleOverride.Int64 {
		return false
	}
	commodity, ok := rules.commodityAt(posting.CommodityID, date)
	if !ok || commodity.Status != "active" || commodity.Kind != "currency" || posting.Scale < 0 || posting.Scale > commodity.MaxQuantityScale || posting.Scale > exact.MaxScaleForCommodityKind(commodity.Kind) {
		return false
	}
	return true
}

func pointerFromNull(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
