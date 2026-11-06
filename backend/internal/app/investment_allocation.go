package app

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"slices"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

const AllocationPolicy = "allocation_targets_v1"
const AllocationTargetTotal = 1000000

type SaveAllocationInput struct {
	ExpectedRevision int64                 `json:"expected_revision"`
	AccountIDs       []int64               `json:"account_ids"`
	Targets          []db.AllocationTarget `json:"targets"`
	OwnerUserID      int64                 `json:"-"`
	AuthSessionID    int64                 `json:"-"`
	RequestID        string                `json:"-"`
}

type AllocationAccount struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Code     string `json:"code"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Included bool   `json:"included"`
}
type AllocationMoney struct {
	Value       string `json:"value"`
	Scale       int    `json:"scale"`
	CommodityID int64  `json:"commodity_id"`
}
type AllocationRow struct {
	CommodityID int64            `json:"commodity_id"`
	Side        string           `json:"side"`
	Amount      *AllocationMoney `json:"amount"`
	Weight      *string          `json:"weight"`
	TargetUnits *int             `json:"target_units"`
	Drift       *string          `json:"drift"`
}
type AllocationUnknown struct {
	AccountID   int64  `json:"account_id"`
	CommodityID int64  `json:"commodity_id"`
	Value       string `json:"value"`
	Scale       int    `json:"scale"`
}
type AllocationPoint struct {
	Date         string              `json:"date"`
	Total        *AllocationMoney    `json:"total"`
	Reason       string              `json:"reason"`
	ChartReason  string              `json:"chart_reason"`
	Rows         []AllocationRow     `json:"rows"`
	Unknown      []AllocationUnknown `json:"unknown"`
	NegativeCash *AllocationMoney    `json:"negative_cash"`
}
type AllocationRead struct {
	Policy              string                   `json:"policy"`
	From                string                   `json:"from"`
	To                  string                   `json:"to"`
	ReportingCurrencyID int64                    `json:"reporting_currency_id"`
	KnowledgeCutoff     string                   `json:"knowledge_cutoff"`
	TimeZone            string                   `json:"time_zone"`
	Interval            string                   `json:"interval"`
	Current             db.AllocationRevision    `json:"current"`
	Accounts            []AllocationAccount      `json:"accounts"`
	Commodities         []db.AllocationCommodity `json:"commodities"`
	Revisions           []db.AllocationRevision  `json:"revisions"`
	Points              []AllocationPoint        `json:"points"`
	Valuation           ValuationCoverage        `json:"-"`
	CurrencyAllocation  ReturnsAllocation        `json:"-"`
}

func (s *InvestmentService) SaveAllocation(ctx context.Context, input SaveAllocationInput) (db.AllocationRevision, error) {
	if input.ExpectedRevision < 0 || input.AccountIDs == nil || input.Targets == nil || len(input.AccountIDs) > 10000 || len(input.Targets) > 2000 {
		return db.AllocationRevision{}, ValidationError{Message: "complete allocation selection and targets are required"}
	}
	ids := map[int64]bool{}
	for _, id := range input.AccountIDs {
		if id <= 0 || ids[id] {
			return db.AllocationRevision{}, ValidationError{Message: "account IDs must be positive and unique"}
		}
		ids[id] = true
	}
	total := 0
	seen := map[int64]bool{}
	for _, target := range input.Targets {
		if target.CommodityID < 0 || seen[target.CommodityID] || target.Units < 0 || target.Units > AllocationTargetTotal {
			return db.AllocationRevision{}, ValidationError{Message: "targets must have unique identities and exact nonnegative weights"}
		}
		seen[target.CommodityID] = true
		total += target.Units
	}
	if len(input.Targets) > 0 && total != AllocationTargetTotal {
		return db.AllocationRevision{}, ValidationError{Message: "targets must total exactly 100 percent"}
	}
	input.AccountIDs = slices.Clone(input.AccountIDs)
	input.Targets = slices.Clone(input.Targets)
	slices.Sort(input.AccountIDs)
	slices.SortFunc(input.Targets, func(a, b db.AllocationTarget) int {
		if a.CommodityID < b.CommodityID {
			return -1
		}
		if a.CommodityID > b.CommodityID {
			return 1
		}
		return 0
	})
	clock := s.now()
	ctx, err := s.settings.referenceContext(ctx, clock)
	if err != nil {
		return db.AllocationRevision{}, fmt.Errorf("resolve allocation reference day: %w", err)
	}
	now := clock.UTC().Format(time.RFC3339)
	return s.repository.SaveAllocationRevision(ctx, input.ExpectedRevision, db.AllocationRevision{SavedAt: now, AccountIDs: input.AccountIDs, Targets: input.Targets}, db.AuditEventParams{BookID: BookID, ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, OccurredAt: now, RequestID: input.RequestID, OriginType: "browser_api", Operation: "investment.allocation.save", Reason: "save investment pool and targets"})
}

func (s *InvestmentService) ReadAllocation(ctx context.Context, input ReturnsInput) (AllocationRead, error) {
	snapshot, cleanup, err := s.repository.LiveGainsSnapshot(ctx)
	if err != nil {
		return AllocationRead{}, err
	}
	defer cleanup()
	return readAllocationOn(ctx, snapshot, s.now(), input)
}

func readAllocationOn(ctx context.Context, snapshot *sql.DB, now time.Time, input ReturnsInput) (AllocationRead, error) {
	out := AllocationRead{Policy: AllocationPolicy, Accounts: []AllocationAccount{}, Points: []AllocationPoint{}}
	pricing := db.NewPricingRepository(snapshot)
	boundary, err := pricing.CurrentObservationBoundary(ctx, BookID, now)
	if err != nil {
		return out, err
	}
	ctx = db.WithReferenceDate(ctx, boundary.AsOf)
	out.TimeZone, err = db.NewSettingsRepository(snapshot).OwnerTimeZone(ctx)
	if err != nil {
		return out, err
	}
	if input.To == "" {
		input.To = boundary.AsOf
	}
	if input.From == "" {
		date, parseErr := time.Parse(time.DateOnly, input.To)
		if parseErr != nil {
			return out, ValidationError{Message: "to must be a date (YYYY-MM-DD)"}
		}
		input.From = date.Format("2006") + "-01-01"
	}
	if input.ReportingCommodityID == 0 {
		currency, e := pricing.DefaultCurrency(ctx, BookID)
		if e != nil {
			return out, e
		}
		input.ReportingCommodityID = currency.ID
	}
	input, err = validateReturnsInput(input)
	if err != nil {
		return out, err
	}
	if input.To > boundary.AsOf {
		return out, ValidationError{Message: "to must not be after today"}
	}
	out.From, out.To, out.ReportingCurrencyID, out.KnowledgeCutoff = input.From, input.To, input.ReportingCommodityID, boundary.RecordedAt
	ledger := db.NewTransactionRepository(snapshot)
	accounts, err := ledger.LedgerAccountsAsOf(ctx, BookID, boundary.AsOf)
	if err != nil {
		return out, err
	}
	bases, err := ledger.AccountBaseKinds(ctx)
	if err != nil {
		return out, err
	}
	pool, err := resolveInvestmentPool(ctx, snapshot, accounts, bases)
	if err != nil {
		return out, err
	}
	out.Current, out.Revisions, out.Accounts = pool.Current, pool.Revisions, pool.Accounts
	members := pool.Members
	out.Commodities, err = db.AllocationCommodities(ctx, snapshot, BookID)
	if err != nil {
		return out, err
	}
	reader := returnsReader{ledger: ledger, pricing: pricing, from: input.From, to: input.To, openDate: previousDay(input.From)}
	if err = reader.resolveScope(ctx, nil, &ReturnsResult{}); err != nil {
		return out, err
	}
	reader.inScope = members
	reader.kinds, err = ledger.LedgerCommodityKinds(ctx, BookID)
	if err != nil {
		return out, err
	}
	reader.shorts, err = ledger.ShortLotQuantityEventsThrough(ctx, BookID, input.To)
	if err != nil {
		return out, err
	}
	postings, err := ledger.LedgerPostingsThrough(ctx, BookID, "", "posted")
	if err != nil {
		return out, err
	}
	anchors, err := ledger.ReturnsOperationAnchors(ctx, BookID)
	if err != nil {
		return out, err
	}
	reader.postings = reader.recognise(postings, anchors)
	commodities := []int64{}
	for _, p := range reader.postings {
		if members[p.AccountID] {
			commodities = append(commodities, p.CommodityID)
		}
	}
	reader.rates, err = newRateTableAtBoundary(ctx, pricing, ReportingCurrencyInput{CommodityID: input.ReportingCommodityID, MaxStalenessDay: input.MaxStalenessDays}, commodities, input.From, input.To, boundary)
	if err != nil {
		return out, err
	}
	from, _ := time.Parse(time.DateOnly, input.From)
	to, _ := time.Parse(time.DateOnly, input.To)
	interval, dates := returnsSeriesDates(input.From, input.To, int(to.Sub(from).Hours()/24)+1)
	out.Interval = interval
	dates = slices.Compact(dates)
	valuations := reader.valuations(slices.Clone(dates), reader.rates)
	running := map[int64]map[int64]*exact.ScaledInt{}
	next := 0
	for _, date := range dates {
		for next < len(reader.postings) && reader.postings[next].EntryDate <= date {
			p := reader.postings[next]
			if members[p.AccountID] {
				foldPosting(running, p)
			}
			next++
		}
		out.Points = append(out.Points, reader.allocationPoint(date, running, valuations[date], out.Current.Targets))
	}
	// A selector can inspect an instrument before it was bought or after it
	// was sold, even when it has no explicit target. Supply a known zero only
	// from the ledger; comparison weights still require a complete valuation.
	universe := map[int64]bool{}
	for _, point := range out.Points {
		for _, row := range point.Rows {
			universe[row.CommodityID] = true
		}
	}
	for i := range out.Points {
		point := &out.Points[i]
		present := map[int64]bool{}
		for _, row := range point.Rows {
			present[row.CommodityID] = true
		}
		for _, id := range sortedKeys(universe) {
			if present[id] {
				continue
			}
			row := AllocationRow{CommodityID: id, Side: AllocationSideLong, Amount: allocationMoney(reader.quantity(new(big.Rat)))}
			if len(out.Current.Targets) > 0 {
				units := 0
				row.TargetUnits = &units
			}
			if point.Reason == "" {
				weight := "0.000000"
				row.Weight = &weight
				if row.TargetUnits != nil {
					drift := "0.000000"
					row.Drift = &drift
				}
			}
			point.Rows = append(point.Rows, row)
		}
		slices.SortFunc(point.Rows, func(a, b AllocationRow) int {
			if c := cmp.Compare(a.CommodityID, b.CommodityID); c != 0 {
				return c
			}
			return cmp.Compare(a.Side, b.Side)
		})
	}
	classes, err := ledger.ReturnsInstrumentClassesAsOf(ctx, BookID, input.To)
	if err != nil {
		return out, err
	}
	out.CurrencyAllocation = reader.allocation(valuations[input.To], classes)
	code := ""
	for _, c := range out.Commodities {
		if c.ID == input.ReportingCommodityID {
			code = c.Code
		}
	}
	out.Valuation = reader.rates.Coverage(code)
	return out, reader.overflow
}

func allocationMoney(q *BalanceQuantity) *AllocationMoney {
	if q == nil {
		return nil
	}
	return &AllocationMoney{Value: q.QuantityValue.String(), Scale: q.QuantityScale, CommodityID: q.CommodityID}
}

type instrumentAllocationKey struct {
	commodity int64
	side      string
}

func (r *returnsReader) allocationPoint(date string, running map[int64]map[int64]*exact.ScaledInt, valuation returnsValuation, targets []db.AllocationTarget) AllocationPoint {
	point := AllocationPoint{Date: date, Rows: []AllocationRow{}, Unknown: []AllocationUnknown{}}
	values := map[instrumentAllocationKey]*big.Rat{}
	unknownKeys := map[instrumentAllocationKey]bool{}
	total := new(big.Rat)
	cashTotals := map[int64]*exact.ScaledInt{}
	negativeCash := new(big.Rat)
	negativeCashUnknown := false
	for _, account := range sortedKeys(running) {
		for _, commodity := range sortedKeys(running[account]) {
			balance := running[account][commodity]
			if balance.Sign() == 0 {
				continue
			}
			coefficient, scale, ok := r.fit(balance.BigInt(), balance.Scale(), commodity)
			if !ok {
				continue
			}
			if r.kinds[commodity] == "currency" {
				if cashTotals[commodity] == nil {
					cashTotals[commodity] = exact.NewScaledInt()
				}
				cashTotals[commodity].AddScaled(balance)
				if balance.Sign() < 0 {
					if value, ok := r.rates.ConvertExact(coefficient, scale, commodity, date); ok {
						negativeCash.Add(negativeCash, value)
					} else {
						negativeCashUnknown = true
					}
				}
				continue
			}
			key := instrumentAllocationKey{commodity: commodity, side: AllocationSideLong}
			if balance.Sign() < 0 {
				key.side = AllocationSideShort
			}
			if values[key] == nil {
				values[key] = new(big.Rat)
			}
			value, ok := r.rates.ConvertExact(coefficient, scale, commodity, date)
			if !ok {
				unknownKeys[key] = true
				point.Unknown = append(point.Unknown, AllocationUnknown{AccountID: account, CommodityID: commodity, Value: coefficient.String(), Scale: scale})
				continue
			}
			if value.Sign() < 0 {
				point.ChartReason = "signed_allocation"
			}
			values[key].Add(values[key], value)
			total.Add(total, value)
		}
	}
	// Cash has one net target group. Net each native currency first, then use
	// the shared exact FX table; offsetting native balances need no FX for zero.
	cashKey := instrumentAllocationKey{commodity: 0, side: AllocationSideLong}
	for _, commodity := range sortedKeys(cashTotals) {
		balance := cashTotals[commodity]
		if balance.Sign() == 0 {
			continue
		}
		if values[cashKey] == nil {
			values[cashKey] = new(big.Rat)
		}
		coefficient, scale, ok := r.fit(balance.BigInt(), balance.Scale(), commodity)
		if !ok {
			continue
		}
		value, ok := r.rates.ConvertExact(coefficient, scale, commodity, date)
		if !ok {
			unknownKeys[cashKey] = true
			for _, account := range sortedKeys(running) {
				amount := running[account][commodity]
				if amount == nil || amount.Sign() == 0 {
					continue
				}
				coefficient, scale, ok := r.fit(amount.BigInt(), amount.Scale(), commodity)
				if ok {
					point.Unknown = append(point.Unknown, AllocationUnknown{AccountID: account, CommodityID: commodity, Value: coefficient.String(), Scale: scale})
				}
			}
			continue
		}
		values[cashKey].Add(values[cashKey], value)
		total.Add(total, value)
	}
	if cash := values[cashKey]; cash != nil && cash.Sign() < 0 && !unknownKeys[cashKey] {
		values[instrumentAllocationKey{commodity: 0, side: AllocationSideShort}] = cash
		delete(values, cashKey)
		point.ChartReason = "signed_allocation"
	}
	if !negativeCashUnknown {
		point.NegativeCash = allocationMoney(r.quantity(negativeCash))
	}

	targetMap := map[int64]int{}
	for _, t := range targets {
		targetMap[t.CommodityID] = t.Units
		key := instrumentAllocationKey{commodity: t.CommodityID, side: AllocationSideLong}
		if values[key] == nil {
			values[key] = new(big.Rat)
		}
	}
	point.Reason = valuation.reason
	if len(point.Unknown) > 0 {
		point.Reason = ReturnReasonValuationUnavailable
	}
	if valuation.reason == ReturnReasonUnclassifiedShort {
		point.Reason = valuation.reason
	}
	if point.Reason == "" {
		point.Total = allocationMoney(r.quantity(total))
		if total.Sign() <= 0 {
			point.Reason = ReturnReasonNonPositiveValue
		}
	}
	if point.Reason != "" {
		point.ChartReason = point.Reason
	}
	keys := make([]instrumentAllocationKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b instrumentAllocationKey) int {
		if a.commodity < b.commodity {
			return -1
		}
		if a.commodity > b.commodity {
			return 1
		}
		return cmp.Compare(a.side, b.side)
	})
	for _, key := range keys {
		row := AllocationRow{CommodityID: key.commodity, Side: key.side, Amount: allocationMoney(r.quantity(values[key]))}
		if unknownKeys[key] || (valuation.reason == ReturnReasonUnclassifiedShort && values[key].Sign() != 0) {
			row.Amount = nil
		}
		if len(targets) > 0 {
			units := targetMap[key.commodity]
			if key.side == AllocationSideShort {
				units = 0
			}
			row.TargetUnits = &units
		}
		if point.Reason == "" {
			share := new(big.Rat).Quo(values[key], total)
			weight := formatReturnRat(share)
			row.Weight = &weight
			if row.TargetUnits != nil {
				drift := formatReturnRat(new(big.Rat).Sub(share, big.NewRat(int64(*row.TargetUnits), AllocationTargetTotal)))
				row.Drift = &drift
			}
		}
		point.Rows = append(point.Rows, row)
	}
	return point
}
