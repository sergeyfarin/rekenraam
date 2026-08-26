package api

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type reportFiltersResponse struct {
	AccountIDs         []int64 `json:"account_ids"`
	IncludeDescendants bool    `json:"include_descendants"`
	CommodityIDs       []int64 `json:"commodity_ids"`
	ResolvedAccountIDs []int64 `json:"resolved_account_ids"`
}

type spendingQueryResponse struct {
	StartDate   string                `json:"start_date"`
	EndDate     string                `json:"end_date"`
	GroupBy     string                `json:"group_by"`
	Mode        string                `json:"mode"`
	CategoryIDs []int64               `json:"category_ids"`
	PayeeIDs    []int64               `json:"payee_ids"`
	Filters     reportFiltersResponse `json:"filters"`
}

type spendingGroupTotalResponse struct {
	CommodityID      int64             `json:"commodity_id"`
	QuantityValue    exact.Coefficient `json:"quantity_value"`
	QuantityScale    int               `json:"quantity_scale"`
	ShareBasisPoints *int64            `json:"share_basis_points,omitempty"`
}

type spendingGroupResponse struct {
	CategoryID   *int64                       `json:"category_id,omitempty"`
	PayeeID      *int64                       `json:"payee_id,omitempty"`
	Code         string                       `json:"code,omitempty"`
	Name         string                       `json:"name,omitempty"`
	CategoryType string                       `json:"category_type"`
	Totals       []spendingGroupTotalResponse `json:"totals"`
	DrillDown    spendingDrillDownResponse    `json:"drill_down"`
	// Converted is this group in the reporting currency, summed from each
	// posting at its own entry date. Absent when the group holds a posting
	// that could not be converted.
	Converted *balanceQuantityResponse `json:"converted,omitempty"`
}

// spendingDrillDownResponse is the query that reproduces this row's postings.
// It is emitted only when it can honour the same semantics as the report; the
// frontend shows the filter summary rather than a misleading link otherwise.
type spendingDrillDownResponse struct {
	StartDate  string  `json:"start_date"`
	EndDate    string  `json:"end_date"`
	CategoryID *int64  `json:"category_id,omitempty"`
	PayeeID    *int64  `json:"payee_id,omitempty"`
	AccountIDs []int64 `json:"account_ids"`
}

// valuationResponse is what the response says about its own conversions: the
// method that produced them, how stale a rate was allowed to be, which rates
// were used, and which commodities went unconverted. A converted figure
// without this block would be a number with no provenance.
type rateUseResponse struct {
	CommodityID     int64  `json:"commodity_id"`
	ObservationDate string `json:"observation_date"`
	RequestedDate   string `json:"requested_date"`
	Stale           bool   `json:"stale"`
	Derived         bool   `json:"derived"`
}

type rateGapResponse struct {
	CommodityID            int64  `json:"commodity_id"`
	Reason                 string `json:"reason"`
	NearestObservationDate string `json:"nearest_observation_date,omitempty"`
}

type valuationResponse struct {
	CommodityID      int64             `json:"commodity_id"`
	Code             string            `json:"code,omitempty"`
	Scale            int               `json:"scale"`
	Method           string            `json:"method"`
	MaxStalenessDays int               `json:"max_staleness_days"`
	Complete         bool              `json:"complete"`
	RatesUsed        []rateUseResponse `json:"rates_used"`
	Gaps             []rateGapResponse `json:"gaps"`
}

func toValuationResponse(coverage *app.ValuationCoverage) *valuationResponse {
	if coverage == nil {
		return nil
	}
	response := valuationResponse{
		CommodityID:      coverage.CommodityID,
		Code:             coverage.Code,
		Scale:            coverage.Scale,
		Method:           coverage.Method,
		MaxStalenessDays: coverage.MaxStalenessDays,
		Complete:         coverage.Complete,
		RatesUsed:        make([]rateUseResponse, 0, len(coverage.Used)),
		Gaps:             make([]rateGapResponse, 0, len(coverage.Gaps)),
	}
	for _, use := range coverage.Used {
		response.RatesUsed = append(response.RatesUsed, rateUseResponse(use))
	}
	for _, gap := range coverage.Gaps {
		response.Gaps = append(response.Gaps, rateGapResponse(gap))
	}
	return &response
}

// parseReportingCurrency reads the optional reporting-currency parameters.
// Absent, reports behave exactly as before — the conversion is additive, and so
// is the request.
func parseReportingCurrency(query url.Values) (*app.ReportingCurrencyInput, error) {
	raw := strings.TrimSpace(query.Get("reporting_currency_id"))
	if raw == "" {
		return nil, nil
	}
	commodityID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || commodityID <= 0 {
		return nil, reportFilterError{field: "reporting_currency_id"}
	}

	input := app.ReportingCurrencyInput{CommodityID: commodityID}
	if staleness := strings.TrimSpace(query.Get("max_staleness_days")); staleness != "" {
		days, err := strconv.Atoi(staleness)
		if err != nil || days < 0 || days > 365 {
			return nil, reportFilterError{field: "max_staleness_days"}
		}
		input.MaxStalenessDay = days
	}
	return &input, nil
}

type spendingResponse struct {
	Query           spendingQueryResponse     `json:"query"`
	Groups          []spendingGroupResponse   `json:"groups"`
	CommodityTotals []balanceQuantityResponse `json:"commodity_totals"`
	// RankingCommodityID names the commodity the group order was computed in.
	// The rows are group-major and there is one order, so a reader has to be
	// told which commodity produced it rather than being left to assume the
	// ranking is per commodity. Absent when no group carries a total.
	RankingCommodityID  *int64   `json:"ranking_commodity_id,omitempty"`
	ExcludedSystemRoles []string `json:"excluded_system_roles"`
	GroupingPolicy      string   `json:"grouping_policy"`
	// ConvertedTotal and Valuation appear only when a reporting currency was
	// asked for. The per-commodity totals above are unchanged either way.
	ConvertedTotal *balanceQuantityResponse `json:"converted_total,omitempty"`
	Valuation      *valuationResponse       `json:"valuation,omitempty"`
}

func spendingReport(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		query := r.URL.Query()

		categoryIDs, err := parseRepeatedIDs(query, "category_id")
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "category_id is invalid")
			return
		}
		payeeIDs, err := parseRepeatedIDs(query, "payee_id")
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "payee_id is invalid")
			return
		}
		filters, err := parseReportFilters(query)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		reporting, err := parseReportingCurrency(query)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		result, err := transactionService.Spending(r.Context(), app.SpendingInput{
			StartDate:   query.Get("start_date"),
			EndDate:     query.Get("end_date"),
			GroupBy:     query.Get("group_by"),
			Mode:        query.Get("mode"),
			CategoryIDs: categoryIDs,
			PayeeIDs:    payeeIDs,
			Filters:     filters,
			Reporting:   reporting,
		})
		if err != nil {
			writeLedgerServiceError(w, r, logger, "read spending report", err)
			return
		}

		writeJSON(w, http.StatusOK, toSpendingResponse(result))
	}
}

func toSpendingResponse(result app.SpendingResult) spendingResponse {
	groups := make([]spendingGroupResponse, 0, len(result.Groups))
	for _, group := range result.Groups {
		totals := make([]spendingGroupTotalResponse, 0, len(group.Totals))
		for _, total := range group.Totals {
			totals = append(totals, spendingGroupTotalResponse{
				CommodityID:      total.CommodityID,
				QuantityValue:    total.QuantityValue,
				QuantityScale:    total.QuantityScale,
				ShareBasisPoints: total.ShareBasisPoints,
			})
		}
		groups = append(groups, spendingGroupResponse{
			CategoryID:   group.CategoryID,
			PayeeID:      group.PayeeID,
			Code:         group.Code,
			Name:         group.Name,
			CategoryType: group.CategoryType,
			Totals:       totals,
			DrillDown: spendingDrillDownResponse{
				StartDate:  result.StartDate,
				EndDate:    result.EndDate,
				CategoryID: group.CategoryID,
				PayeeID:    group.PayeeID,
				AccountIDs: emptyIfNil(result.Filters.ResolvedAccountIDs),
			},
			Converted: toBalanceQuantityPointer(group.Converted),
		})
	}

	return spendingResponse{
		Query: spendingQueryResponse{
			StartDate:   result.StartDate,
			EndDate:     result.EndDate,
			GroupBy:     result.GroupBy,
			Mode:        result.Mode,
			CategoryIDs: emptyIfNil(result.CategoryIDs),
			PayeeIDs:    emptyIfNil(result.PayeeIDs),
			Filters:     toReportFiltersResponse(result.Filters),
		},
		Groups:              groups,
		CommodityTotals:     toBalanceQuantityResponses(result.CommodityTotals),
		RankingCommodityID:  result.RankingCommodityID,
		ExcludedSystemRoles: result.ExcludedSystemRoles,
		GroupingPolicy:      result.GroupingPolicy,
		ConvertedTotal:      toBalanceQuantityPointer(result.ConvertedTotal),
		Valuation:           toValuationResponse(result.Valuation),
	}
}

// toBalanceQuantityPointer keeps an absent converted figure absent. A zero
// would read as "nothing", which is a different claim from "not converted".
func toBalanceQuantityPointer(quantity *app.BalanceQuantity) *balanceQuantityResponse {
	if quantity == nil {
		return nil
	}
	response := balanceQuantityResponse{
		CommodityID:         quantity.CommodityID,
		QuantityValue:       quantity.QuantityValue,
		QuantityScale:       quantity.QuantityScale,
		NormalQuantityValue: quantity.NormalQuantityValue,
	}
	return &response
}

func toReportFiltersResponse(filters app.ReportFiltersEcho) reportFiltersResponse {
	return reportFiltersResponse{
		AccountIDs:         emptyIfNil(filters.AccountIDs),
		IncludeDescendants: filters.IncludeDescendants,
		CommodityIDs:       emptyIfNil(filters.CommodityIDs),
		ResolvedAccountIDs: emptyIfNil(filters.ResolvedAccountIDs),
	}
}

// parseReportFilters reads the shared `/reports/*` filter dimensions. Repeated
// parameters are an OR-set within one dimension.
func parseReportFilters(query url.Values) (app.ReportFilters, error) {
	accountIDs, err := parseRepeatedIDs(query, "account_id")
	if err != nil {
		return app.ReportFilters{}, errInvalidReportFilter("account_id")
	}
	commodityIDs, err := parseRepeatedIDs(query, "commodity_id")
	if err != nil {
		return app.ReportFilters{}, errInvalidReportFilter("commodity_id")
	}
	includeDescendants, err := parseOptionalBool(query.Get("include_descendants"))
	if err != nil {
		return app.ReportFilters{}, errInvalidReportFilter("include_descendants")
	}
	return app.ReportFilters{
		AccountIDs:         accountIDs,
		IncludeDescendants: includeDescendants,
		CommodityIDs:       commodityIDs,
	}, nil
}

type reportFilterError struct {
	field string
}

func (e reportFilterError) Error() string { return e.field + " is invalid" }

func errInvalidReportFilter(field string) error { return reportFilterError{field: field} }

func parseRepeatedIDs(query url.Values, name string) ([]int64, error) {
	raw := query[name]
	if len(raw) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(raw))
	for _, value := range raw {
		if value == "" {
			continue
		}
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return nil, reportFilterError{field: name}
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// emptyIfNil keeps JSON arrays as `[]` rather than `null`, so generated
// frontend types never have to special-case an absent list.
func emptyIfNil(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

type cashflowBucketResponse struct {
	StartDate    string                    `json:"start_date"`
	EndDate      string                    `json:"end_date"`
	Inflow       []balanceQuantityResponse `json:"inflow"`
	Outflow      []balanceQuantityResponse `json:"outflow"`
	TransferIn   []balanceQuantityResponse `json:"transfer_in"`
	TransferOut  []balanceQuantityResponse `json:"transfer_out"`
	OperatingNet []balanceQuantityResponse `json:"operating_net"`
	TransferNet  []balanceQuantityResponse `json:"transfer_net"`
	NetMovement  []balanceQuantityResponse `json:"net_movement"`
	// The same measures in the reporting currency, absent unless one was asked
	// for. Converted net movement is derived from the converted parts, so
	// net_movement = operating_net + transfer_net holds here exactly as it does
	// in the exact figures.
	ConvertedInflow       *balanceQuantityResponse `json:"converted_inflow,omitempty"`
	ConvertedOutflow      *balanceQuantityResponse `json:"converted_outflow,omitempty"`
	ConvertedOperatingNet *balanceQuantityResponse `json:"converted_operating_net,omitempty"`
	ConvertedTransferNet  *balanceQuantityResponse `json:"converted_transfer_net,omitempty"`
	ConvertedNetMovement  *balanceQuantityResponse `json:"converted_net_movement,omitempty"`
}

type cashflowQueryResponse struct {
	StartDate string                `json:"start_date"`
	EndDate   string                `json:"end_date"`
	Bucket    string                `json:"bucket"`
	Filters   reportFiltersResponse `json:"filters"`
	CashScope string                `json:"cash_scope"`
	CashKinds []string              `json:"default_cash_account_kinds"`
}

type cashflowResponse struct {
	Query               cashflowQueryResponse    `json:"query"`
	Buckets             []cashflowBucketResponse `json:"buckets"`
	ExcludedSystemRoles []string                 `json:"excluded_system_roles"`
	Valuation           *valuationResponse       `json:"valuation,omitempty"`
}

func cashflowReport(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		query := r.URL.Query()
		filters, err := parseReportFilters(query)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		reporting, err := parseReportingCurrency(query)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		result, err := transactionService.Cashflow(r.Context(), app.CashflowInput{
			StartDate: query.Get("start_date"),
			EndDate:   query.Get("end_date"),
			Bucket:    query.Get("bucket"),
			Filters:   filters,
			Reporting: reporting,
		})
		if err != nil {
			writeLedgerServiceError(w, r, logger, "read cashflow", err)
			return
		}

		buckets := make([]cashflowBucketResponse, 0, len(result.Buckets))
		for _, bucket := range result.Buckets {
			buckets = append(buckets, cashflowBucketResponse{
				StartDate:    bucket.StartDate,
				EndDate:      bucket.EndDate,
				Inflow:       toBalanceQuantityResponses(bucket.Inflow),
				Outflow:      toBalanceQuantityResponses(bucket.Outflow),
				TransferIn:   toBalanceQuantityResponses(bucket.TransferIn),
				TransferOut:  toBalanceQuantityResponses(bucket.TransferOut),
				OperatingNet: toBalanceQuantityResponses(bucket.OperatingNet),
				TransferNet:  toBalanceQuantityResponses(bucket.TransferNet),
				NetMovement:  toBalanceQuantityResponses(bucket.NetMovement),

				ConvertedInflow:       toBalanceQuantityPointer(bucket.ConvertedInflow),
				ConvertedOutflow:      toBalanceQuantityPointer(bucket.ConvertedOutflow),
				ConvertedOperatingNet: toBalanceQuantityPointer(bucket.ConvertedOperatingNet),
				ConvertedTransferNet:  toBalanceQuantityPointer(bucket.ConvertedTransferNet),
				ConvertedNetMovement:  toBalanceQuantityPointer(bucket.ConvertedNetMovement),
			})
		}

		writeJSON(w, http.StatusOK, cashflowResponse{
			Query: cashflowQueryResponse{
				StartDate: result.StartDate,
				EndDate:   result.EndDate,
				Bucket:    result.Bucket,
				Filters:   toReportFiltersResponse(result.Filters),
				CashScope: result.CashScope,
				CashKinds: result.CashAccountKinds,
			},
			Buckets:             buckets,
			ExcludedSystemRoles: result.ExcludedSystemRoles,
			Valuation:           toValuationResponse(result.Valuation),
		})
	}
}
