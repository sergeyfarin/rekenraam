package app

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// More to explore (#479).
//
// Every widget not on My Home is shown at real size, so each summary comes
// from the same composed read and starts no work: Cashflow is the Cashflow
// report for the month, Holdings the briefing's own fold valued as net worth
// values it, Returns and Allocation the latest saved returns run (never a
// live calculation; #440 adds those), Dividend income the income coverage
// summary, and Backups the Data screen's backup status. Forecast, Imports and
// Prices need nothing new: the recurring summary, the shared attention read
// and the briefing's valuation inputs already answer them.

const (
	homeHoldingRows    = 3
	homeAllocationRows = 3
	homeIncomeRows     = 3
)

// HomeCashflowSummary is the Cashflow report for this month to date over its
// default liquid-cash scope, as one bucket. Each measure is exact per
// commodity; Converted* are the report's own reporting-currency figures, nil
// when a posting could not be converted, never partial.
type HomeCashflowSummary struct {
	StartDate             string
	EndDate               string
	CashScope             string
	AccountCount          int
	Inflow                []BalanceQuantity
	Outflow               []BalanceQuantity
	TransferNet           []BalanceQuantity
	OperatingNet          []BalanceQuantity
	ConvertedInflow       *BalanceQuantity
	ConvertedOutflow      *BalanceQuantity
	ConvertedTransferNet  *BalanceQuantity
	ConvertedOperatingNet *BalanceQuantity
}

// HomeHoldingsSummary lists the largest positions by value. Each row is one
// holding account (an instrument in one brokerage) from the briefing's fold,
// valued through the same rates as the accounts widget; a position without a
// usable price has no value and ranks last.
type HomeHoldingsSummary struct {
	AsOf          string
	PositionCount int
	UnpricedCount int
	Holdings      []HomeHoldingRow
	// Missing and Stale are the prices and rates valuing every listed
	// position needs, judged and reported as the headline judges its own:
	// each leg of a chained conversion against its own commodity.
	Missing []HomeValuationInput
	Stale   []HomeValuationInput
}

type HomeHoldingRow struct {
	AccountID int64
	// BrokerageID and BrokerageName are the account the holding sits under;
	// zero and empty for a holding at top level.
	BrokerageID   int64
	BrokerageName string
	Balances      []BalanceQuantity
	Value         *BalanceQuantity
}

// HomeReturnsSummary is the latest saved returns run's time-weighted return,
// as frozen when it was saved. RunID is 0 when no run has been saved.
type HomeReturnsSummary struct {
	RunID                  int64
	CapturedAt             string
	From                   string
	To                     string
	AccountScope           string
	AccountCount           int
	TimeWeighted           ReturnFigure
	TimeWeightedAnnualized ReturnFigure
}

// HomeAllocationSummary is the same run's allocation by asset type, largest
// value first. Reason makes it unavailable; Basis known_only means UnknownCount
// balances had no value and are in no weight.
type HomeAllocationSummary struct {
	RunID        int64
	CapturedAt   string
	Date         string
	Reason       string
	Basis        string
	UnknownCount int
	Rows         []HomeAllocationRow
	RowCount     int
}

type HomeAllocationRow struct {
	AssetType string
	Side      string
	Value     *BalanceQuantity
	Weight    *string
}

// HomeIncomeSummary is the income coverage summary: instruments with
// owner-confirmed income, and the book's identifiable unassigned investment
// cash per currency, which is cash and never gross income.
type HomeIncomeSummary struct {
	InstrumentCount int
	UnassignedCount int
	Unassigned      []HomeIncomeUnassigned
}

type HomeIncomeUnassigned struct {
	Cash      BalanceQuantity
	ItemCount int
}

// HomeBackupsSummary is the backup status the Data screen reads: the policy,
// the last successful backup and the latest attempt.
type HomeBackupsSummary struct {
	Enabled       bool
	NextRunAt     string
	LastSuccessAt string
	// LatestStatus is the newest run's status; empty when nothing ever ran.
	LatestStatus    string
	LatestWillRetry bool
}

func (s *HomeService) cashflowSummary(ctx context.Context, asOf string, reportingID int64) (*HomeCashflowSummary, error) {
	input := CashflowInput{StartDate: homeMonthStart(asOf), EndDate: asOf, Bucket: "month"}
	if reportingID != 0 {
		input.Reporting = &ReportingCurrencyInput{CommodityID: reportingID}
	}
	report, err := s.transactions.Cashflow(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("home cashflow: %w", err)
	}
	summary := &HomeCashflowSummary{StartDate: report.StartDate, EndDate: report.EndDate, CashScope: report.CashScope, AccountCount: len(report.Filters.ResolvedAccountIDs),
		Inflow: []BalanceQuantity{}, Outflow: []BalanceQuantity{}, TransferNet: []BalanceQuantity{}, OperatingNet: []BalanceQuantity{}}
	// One month to date is one bucket; a range inside a month never spans two.
	if len(report.Buckets) == 1 {
		bucket := report.Buckets[0]
		summary.Inflow, summary.Outflow, summary.TransferNet, summary.OperatingNet = bucket.Inflow, bucket.Outflow, bucket.TransferNet, bucket.OperatingNet
		summary.ConvertedInflow, summary.ConvertedOutflow = bucket.ConvertedInflow, bucket.ConvertedOutflow
		summary.ConvertedTransferNet, summary.ConvertedOperatingNet = bucket.ConvertedTransferNet, bucket.ConvertedOperatingNet
	}
	return summary, nil
}

// homeRates converts the briefing's balances to the default currency at
// today, for every account and holding Home values. Nil without a default
// currency or anything to convert.
func (s *HomeService) homeRates(ctx context.Context, briefing HomeBriefing, reportingID int64) (*RateTable, error) {
	if reportingID == 0 || len(briefing.Accounts) == 0 {
		return nil, nil
	}
	seen := []int64{}
	for _, account := range briefing.Accounts {
		for _, balance := range account.SubtreeBalances {
			seen = append(seen, balance.CommodityID)
		}
	}
	rates, err := s.transactions.NewRateTable(ctx, ReportingCurrencyInput{CommodityID: reportingID}, dedupeIDs(seen), briefing.AsOf, briefing.AsOf)
	if err != nil {
		return nil, fmt.Errorf("home values: %w", err)
	}
	return rates, nil
}

// holdingsSummary ranks open holding accounts with a balance by their value,
// through its own coverage of the shared rates so its inputs are its own.
func (s *HomeService) holdingsSummary(ctx context.Context, briefing HomeBriefing, rates *RateTable) (*HomeHoldingsSummary, error) {
	if rates != nil {
		rates = rates.withFreshCoverage()
	}
	names := map[int64]string{}
	for _, account := range briefing.Accounts {
		names[account.AccountID] = account.Name
	}
	summary := &HomeHoldingsSummary{AsOf: briefing.AsOf, Holdings: []HomeHoldingRow{}, Missing: []HomeValuationInput{}, Stale: []HomeValuationInput{}}
	rows := []HomeHoldingRow{}
	for _, account := range briefing.Accounts {
		if !homeHoldingAccountKinds[account.AccountKind] || account.Status != "active" || !homeHasBalance(account.SubtreeBalances) {
			continue
		}
		row := HomeHoldingRow{AccountID: account.AccountID, Balances: account.SubtreeBalances}
		if account.ParentAccountID != nil {
			row.BrokerageID, row.BrokerageName = *account.ParentAccountID, names[*account.ParentAccountID]
		}
		if rates != nil {
			row.Value = homeConverted(rates, row.Balances, briefing.AsOf)
		}
		if row.Value == nil {
			summary.UnpricedCount++
		}
		rows = append(rows, row)
	}
	slices.SortStableFunc(rows, func(a, b HomeHoldingRow) int {
		return cmp.Or(compareHomeValues(a.Value, b.Value), cmp.Compare(a.AccountID, b.AccountID))
	})
	summary.PositionCount = len(rows)
	summary.Holdings = rows[:min(len(rows), homeHoldingRows)]
	if rates != nil && len(rows) > 0 {
		staleness, err := s.transactions.pricingStalenessDays(ctx)
		if err != nil {
			return nil, err
		}
		coverage := HomeHeadline{Missing: []HomeValuationInput{}, Stale: []HomeValuationInput{}}
		if err := s.transactions.classifyHomeCoverage(ctx, &coverage, rates.Coverage(""), briefing.AsOf, staleness); err != nil {
			return nil, err
		}
		summary.Missing, summary.Stale = coverage.Missing, coverage.Stale
	}
	return summary, nil
}

func homeHasBalance(balances []BalanceQuantity) bool {
	for _, balance := range balances {
		if balance.QuantityValue.Sign() != 0 {
			return true
		}
	}
	return false
}

// compareHomeValues orders the largest magnitude first and a missing value
// after every known one. Values from one rate table share a scale.
func compareHomeValues(a, b *BalanceQuantity) int {
	switch {
	case a != nil && b != nil:
		magnitudeA := new(big.Int).Abs(exact.ScaledIntFromCoefficient(a.QuantityValue, a.QuantityScale).BigInt())
		magnitudeB := new(big.Int).Abs(exact.ScaledIntFromCoefficient(b.QuantityValue, b.QuantityScale).BigInt())
		return magnitudeB.Cmp(magnitudeA)
	case a != nil:
		return -1
	case b != nil:
		return 1
	}
	return 0
}

// homeReturnsDocument is the part of a saved returns run's frozen output the
// Returns and Allocation widgets read, by its published field names.
type homeReturnsDocument struct {
	From                   string                `json:"from"`
	To                     string                `json:"to"`
	AccountScope           string                `json:"account_scope"`
	ResolvedAccountIDs     []int64               `json:"resolved_account_ids"`
	TimeWeighted           homeReturnFigure      `json:"time_weighted"`
	TimeWeightedAnnualized homeReturnFigure      `json:"time_weighted_annualized"`
	Allocation             homeReturnsAllocation `json:"allocation"`
}

type homeReturnFigure struct {
	Value  *string `json:"value"`
	Reason *string `json:"reason"`
	Date   *string `json:"date"`
}

func (f homeReturnFigure) figure() ReturnFigure {
	figure := ReturnFigure{Value: f.Value}
	if f.Reason != nil {
		figure.Reason = *f.Reason
	}
	if f.Date != nil {
		figure.Date = *f.Date
	}
	return figure
}

type homeReturnsAllocation struct {
	Date        string  `json:"date"`
	Reason      *string `json:"reason"`
	Basis       *string `json:"basis"`
	ByAssetType []struct {
		AssetType *string `json:"asset_type"`
		Side      string  `json:"side"`
		Value     *struct {
			CommodityID   int64             `json:"commodity_id"`
			QuantityValue exact.Coefficient `json:"quantity_value"`
			QuantityScale int               `json:"quantity_scale"`
		} `json:"value"`
		Weight *string `json:"weight"`
	} `json:"by_asset_type"`
	Unknown []json.RawMessage `json:"unknown"`
}

// returnsSummaries reads the latest saved run once for both widgets. It is a
// stored document checked whole, exactly as a single run read checks it.
func (s *HomeService) returnsSummaries(ctx context.Context) (*HomeReturnsSummary, *HomeAllocationSummary, error) {
	if s.investments == nil {
		return nil, nil, errHomeNotWired
	}
	run, found, err := s.investments.LatestReturnsRun(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("home returns: %w", err)
	}
	returns, allocation := &HomeReturnsSummary{}, &HomeAllocationSummary{Rows: []HomeAllocationRow{}}
	if !found {
		return returns, allocation, nil
	}
	var document homeReturnsDocument
	if err := json.Unmarshal(run.OutputJSON, &document); err != nil {
		return nil, nil, fmt.Errorf("home returns run %d: %w", run.ID, err)
	}
	*returns = HomeReturnsSummary{RunID: run.ID, CapturedAt: run.CapturedAt, From: document.From, To: document.To, AccountScope: document.AccountScope,
		AccountCount: len(document.ResolvedAccountIDs), TimeWeighted: document.TimeWeighted.figure(), TimeWeightedAnnualized: document.TimeWeightedAnnualized.figure()}

	source := document.Allocation
	allocation.RunID, allocation.CapturedAt, allocation.Date, allocation.UnknownCount = run.ID, run.CapturedAt, source.Date, len(source.Unknown)
	if source.Reason != nil {
		allocation.Reason = *source.Reason
	}
	if source.Basis != nil {
		allocation.Basis = *source.Basis
	}
	rows := make([]HomeAllocationRow, 0, len(source.ByAssetType))
	for _, item := range source.ByAssetType {
		row := HomeAllocationRow{Side: item.Side, Weight: item.Weight}
		if item.AssetType != nil {
			row.AssetType = *item.AssetType
		}
		if item.Value != nil {
			row.Value = &BalanceQuantity{CommodityID: item.Value.CommodityID, QuantityValue: item.Value.QuantityValue, QuantityScale: item.Value.QuantityScale, NormalQuantityValue: item.Value.QuantityValue}
		}
		rows = append(rows, row)
	}
	slices.SortStableFunc(rows, func(a, b HomeAllocationRow) int {
		return cmp.Or(compareHomeValues(a.Value, b.Value), strings.Compare(a.AssetType, b.AssetType), strings.Compare(a.Side, b.Side))
	})
	allocation.RowCount = len(rows)
	allocation.Rows = rows[:min(len(rows), homeAllocationRows)]
	return returns, allocation, nil
}

func (s *HomeService) incomeSummary(ctx context.Context) (*HomeIncomeSummary, error) {
	if s.investments == nil {
		return nil, errHomeNotWired
	}
	coverage, err := s.investments.DividendCoverage(ctx, DividendCoverageInput{SummaryOnly: true})
	if err != nil {
		return nil, fmt.Errorf("home income coverage: %w", err)
	}
	summary := &HomeIncomeSummary{Unassigned: []HomeIncomeUnassigned{}}
	instruments := map[int64]bool{}
	for _, instrument := range coverage.Instruments {
		if instrument.DividendCount > 0 {
			instruments[instrument.CommodityID] = true
		}
	}
	summary.InstrumentCount = len(instruments)
	totals := slices.Clone(coverage.UnassignedTotals)
	slices.SortStableFunc(totals, func(a, b DividendCoverageTotal) int {
		return cmp.Or(cmp.Compare(b.ItemCount, a.ItemCount), strings.Compare(a.CurrencyCode, b.CurrencyCode))
	})
	for _, total := range totals {
		summary.UnassignedCount += total.ItemCount
		if len(summary.Unassigned) < homeIncomeRows {
			summary.Unassigned = append(summary.Unassigned, HomeIncomeUnassigned{ItemCount: total.ItemCount,
				Cash: BalanceQuantity{CommodityID: total.CashCommodityID, QuantityValue: total.Cash.Value, QuantityScale: total.Cash.Scale, NormalQuantityValue: total.Cash.Value}})
		}
	}
	return summary, nil
}

func (s *HomeService) backupsSummary(ctx context.Context) (*HomeBackupsSummary, error) {
	if s.backups == nil {
		return nil, errHomeNotWired
	}
	status, err := s.backups.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("home backups: %w", err)
	}
	summary := &HomeBackupsSummary{Enabled: status.Policy.Enabled}
	if !status.NextRunAt.IsZero() {
		summary.NextRunAt = status.NextRunAt.UTC().Format(time.RFC3339)
	}
	if status.LastSuccess != nil {
		summary.LastSuccessAt = cmp.Or(status.LastSuccess.FinishedAt.String, status.LastSuccess.CreatedAt)
	}
	if len(status.Runs) > 0 {
		latest := status.Runs[0]
		summary.LatestStatus = latest.Status
		// As the Data screen says it: a failed run with a retry queued is not
		// the same as one that spent its attempts.
		summary.LatestWillRetry = latest.Status == "failed" && latest.WorkStatus.String == "pending"
	}
	return summary, nil
}

// errHomeNotWired is a summary whose service this server runs without; the
// widget fails in its card.
var errHomeNotWired = errors.New("home summary service not configured")

// LatestReturnsRun is the newest saved returns run with its frozen output,
// checked whole as a single run read checks it. found is false when no run
// has been saved.
func (s *InvestmentService) LatestReturnsRun(ctx context.Context) (db.ReturnsRunRecord, bool, error) {
	runs, err := s.repository.ListReturnsRuns(ctx, BookID, 0, 1)
	if err != nil || len(runs) == 0 {
		return db.ReturnsRunRecord{}, false, err
	}
	if !db.ValidReturnsRunDocument(runs[0]) {
		return db.ReturnsRunRecord{}, false, ErrReturnsRunIntegrity
	}
	return runs[0], true, nil
}
