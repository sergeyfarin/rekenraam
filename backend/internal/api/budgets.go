package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type budgetAmountResponse struct {
	CommodityID   int64             `json:"commodity_id"`
	QuantityValue exact.Coefficient `json:"quantity_value"`
	QuantityScale int               `json:"quantity_scale"`
}
type budgetCategoryAmountResponse struct {
	CommodityID int64                `json:"commodity_id"`
	Target      budgetAmountResponse `json:"target"`
	Actual      budgetAmountResponse `json:"actual"`
	Remaining   budgetAmountResponse `json:"remaining"`
}
type budgetTypeTotalResponse struct {
	CategoryType string                       `json:"category_type"`
	Amount       budgetCategoryAmountResponse `json:"amount"`
}
type budgetCategoryResponse struct {
	ID             int64                          `json:"id"`
	Code           string                         `json:"code,omitempty"`
	Name           string                         `json:"name,omitempty"`
	CategoryType   string                         `json:"category_type"`
	AllowsPostings bool                           `json:"allows_postings"`
	Amounts        []budgetCategoryAmountResponse `json:"amounts"`
}
type budgetAccountResponse struct {
	ID           int64  `json:"id"`
	Code         string `json:"code,omitempty"`
	Name         string `json:"name,omitempty"`
	AccountClass string `json:"account_class"`
	AccountKind  string `json:"account_kind"`
	Treatment    string `json:"treatment"`
}
type budgetCommodityResponse struct {
	ID     int64  `json:"id"`
	Code   string `json:"code"`
	Symbol string `json:"symbol"`
	Scale  int    `json:"scale"`
}
type budgetMonthResponse struct {
	PeriodStart    string                    `json:"period_start"`
	PeriodEnd      string                    `json:"period_end"`
	RolloverPolicy string                    `json:"rollover_policy"`
	Categories     []budgetCategoryResponse  `json:"categories"`
	Totals         []budgetTypeTotalResponse `json:"totals"`
	Accounts       []budgetAccountResponse   `json:"accounts"`
	Commodities    []budgetCommodityResponse `json:"commodities"`
}

func toBudgetAmount(v app.BudgetAmount) budgetAmountResponse {
	return budgetAmountResponse{v.CommodityID, v.QuantityValue, v.QuantityScale}
}
func toBudgetCategoryAmount(v app.BudgetCategoryAmount) budgetCategoryAmountResponse {
	return budgetCategoryAmountResponse{v.CommodityID, toBudgetAmount(v.Target), toBudgetAmount(v.Actual), toBudgetAmount(v.Remaining)}
}
func toBudgetMonth(v app.BudgetMonth) budgetMonthResponse {
	r := budgetMonthResponse{PeriodStart: v.PeriodStart, PeriodEnd: v.PeriodEnd, RolloverPolicy: v.RolloverPolicy, Categories: []budgetCategoryResponse{}, Totals: []budgetTypeTotalResponse{}, Accounts: []budgetAccountResponse{}, Commodities: []budgetCommodityResponse{}}
	for _, x := range v.Categories {
		c := budgetCategoryResponse{ID: x.ID, Code: x.Code, Name: x.Name, CategoryType: x.CategoryType, AllowsPostings: x.AllowsPostings, Amounts: []budgetCategoryAmountResponse{}}
		for _, a := range x.Amounts {
			c.Amounts = append(c.Amounts, toBudgetCategoryAmount(a))
		}
		r.Categories = append(r.Categories, c)
	}
	for _, x := range v.Totals {
		r.Totals = append(r.Totals, budgetTypeTotalResponse{CategoryType: x.CategoryType, Amount: toBudgetCategoryAmount(x.Amount)})
	}
	for _, x := range v.Accounts {
		r.Accounts = append(r.Accounts, budgetAccountResponse{x.ID, x.Code, x.Name, x.AccountClass, x.AccountKind, x.Treatment})
	}
	for _, x := range v.Commodities {
		r.Commodities = append(r.Commodities, budgetCommodityResponse{x.ID, x.Code, x.Symbol, x.Scale})
	}
	return r
}

func budgetMonth(logger *slog.Logger, auth *app.AuthService, service *app.BudgetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		periodStart := r.URL.Query().Get("period_start")
		var v app.BudgetMonth
		var err error
		if periodStart == "" {
			v, err = service.CurrentMonth(r.Context(), owner.ID)
		} else {
			v, err = service.Month(r.Context(), periodStart)
		}
		if err != nil {
			writeBudgetError(w, r, logger, "read budget month", err)
			return
		}
		writeJSON(w, http.StatusOK, toBudgetMonth(v))
	}
}

type setBudgetTargetRequest struct {
	PeriodStart   string             `json:"period_start"`
	QuantityValue *exact.Coefficient `json:"quantity_value"`
	QuantityScale *int               `json:"quantity_scale"`
	ChangeReason  string             `json:"change_reason"`
}

func setBudgetTarget(logger *slog.Logger, auth *app.AuthService, service *app.BudgetService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, auth, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		categoryID, e1 := strconv.ParseInt(r.PathValue("category_id"), 10, 64)
		commodityID, e2 := strconv.ParseInt(r.PathValue("commodity_id"), 10, 64)
		if e1 != nil || e2 != nil || categoryID <= 0 || commodityID <= 0 {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "category or commodity id is invalid")
			return
		}
		var req setBudgetTargetRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeDecodeError(w, err)
			return
		}
		if req.QuantityValue == nil || req.QuantityScale == nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "quantity_value and quantity_scale are required")
			return
		}
		v, err := service.SetTarget(r.Context(), app.SetBudgetTargetInput{BudgetMutationContext: app.BudgetMutationContext{OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()), OriginType: "browser_api", Reason: req.ChangeReason}, CategoryID: categoryID, CommodityID: commodityID, PeriodStart: req.PeriodStart, QuantityValue: *req.QuantityValue, QuantityScale: *req.QuantityScale})
		if err != nil {
			writeBudgetError(w, r, logger, "set budget target", err)
			return
		}
		writeJSON(w, http.StatusOK, toBudgetMonth(v))
	}))
}

type setBudgetTreatmentRequest struct {
	EffectiveFrom string `json:"effective_from"`
	Treatment     string `json:"treatment"`
	ChangeReason  string `json:"change_reason"`
}

func setBudgetTreatment(logger *slog.Logger, auth *app.AuthService, service *app.BudgetService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, auth, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		accountID, err := strconv.ParseInt(r.PathValue("account_id"), 10, 64)
		if err != nil || accountID <= 0 {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "account id is invalid")
			return
		}
		var req setBudgetTreatmentRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeDecodeError(w, err)
			return
		}
		v, err := service.SetTreatment(r.Context(), app.SetBudgetTreatmentInput{BudgetMutationContext: app.BudgetMutationContext{OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()), OriginType: "browser_api", Reason: req.ChangeReason}, AccountID: accountID, EffectiveFrom: req.EffectiveFrom, Treatment: req.Treatment})
		if err != nil {
			writeBudgetError(w, r, logger, "set budget treatment", err)
			return
		}
		writeJSON(w, http.StatusOK, toBudgetMonth(v))
	}))
}

func writeBudgetError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, operation string, err error) {
	if errors.Is(err, app.ErrBudgetSubjectNotFound) {
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "budget subject not found")
		return
	}
	writeLedgerServiceError(w, r, logger, operation, err)
}
