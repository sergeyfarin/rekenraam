package api

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

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

type spendingResponse struct {
	Query               spendingQueryResponse     `json:"query"`
	Groups              []spendingGroupResponse   `json:"groups"`
	CommodityTotals     []balanceQuantityResponse `json:"commodity_totals"`
	ExcludedSystemRoles []string                  `json:"excluded_system_roles"`
	GroupingPolicy      string                    `json:"grouping_policy"`
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

		result, err := transactionService.Spending(r.Context(), app.SpendingInput{
			StartDate:   query.Get("start_date"),
			EndDate:     query.Get("end_date"),
			GroupBy:     query.Get("group_by"),
			Mode:        query.Get("mode"),
			CategoryIDs: categoryIDs,
			PayeeIDs:    payeeIDs,
			Filters:     filters,
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
		ExcludedSystemRoles: result.ExcludedSystemRoles,
		GroupingPolicy:      result.GroupingPolicy,
	}
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
