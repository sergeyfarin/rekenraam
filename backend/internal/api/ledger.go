package api

import (
	"errors"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type balanceQuantityResponse struct {
	CommodityID         int64             `json:"commodity_id"`
	QuantityValue       exact.Coefficient `json:"quantity_value"`
	QuantityScale       int               `json:"quantity_scale"`
	NormalQuantityValue exact.Coefficient `json:"normal_quantity_value"`
}

type accountBalanceResponse struct {
	AccountID       int64                     `json:"account_id"`
	BookID          int64                     `json:"book_id"`
	IsSystem        bool                      `json:"is_system"`
	SystemRole      string                    `json:"system_role,omitempty"`
	Status          string                    `json:"status"`
	Code            string                    `json:"code,omitempty"`
	Name            string                    `json:"name,omitempty"`
	AccountClass    string                    `json:"account_class"`
	AccountKind     string                    `json:"account_kind"`
	ParentAccountID *int64                    `json:"parent_account_id,omitempty"`
	AllowsPostings  bool                      `json:"allows_postings"`
	DirectBalances  []balanceQuantityResponse `json:"direct_balances"`
	SubtreeBalances []balanceQuantityResponse `json:"subtree_balances"`
}

type accountBalancesResponse struct {
	AsOf     string                   `json:"as_of"`
	Status   string                   `json:"status"`
	Accounts []accountBalanceResponse `json:"accounts"`
}

type categoryTotalResponse struct {
	CategoryID       int64                     `json:"category_id"`
	BookID           int64                     `json:"book_id"`
	Status           string                    `json:"status"`
	Code             string                    `json:"code,omitempty"`
	Name             string                    `json:"name,omitempty"`
	CategoryType     string                    `json:"category_type"`
	ParentCategoryID *int64                    `json:"parent_category_id,omitempty"`
	AllowsPostings   bool                      `json:"allows_postings"`
	DirectTotals     []balanceQuantityResponse `json:"direct_totals"`
	SubtreeTotals    []balanceQuantityResponse `json:"subtree_totals"`
}

type categoryTotalsResponse struct {
	AfterDate  string                  `json:"after_date,omitempty"`
	BeforeDate string                  `json:"before_date"`
	Status     string                  `json:"status"`
	Categories []categoryTotalResponse `json:"categories"`
}

type netWorthResponse struct {
	AsOf                string                    `json:"as_of"`
	Status              string                    `json:"status"`
	Totals              []balanceQuantityResponse `json:"totals"`
	ExcludedSystemRoles []string                  `json:"excluded_system_roles"`
}

type netWorthSeriesBucketResponse struct {
	StartDate string                    `json:"start_date"`
	EndDate   string                    `json:"end_date"`
	Totals    []balanceQuantityResponse `json:"totals"`
}

type netWorthSeriesQueryResponse struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Bucket    string `json:"bucket"`
}

type netWorthSeriesResponse struct {
	StartDate           string                         `json:"start_date"`
	EndDate             string                         `json:"end_date"`
	Bucket              string                         `json:"bucket"`
	Query               netWorthSeriesQueryResponse    `json:"query"`
	Buckets             []netWorthSeriesBucketResponse `json:"buckets"`
	ExcludedSystemRoles []string                       `json:"excluded_system_roles"`
}

func accountBalances(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		includeSystem, err := parseOptionalBool(r.URL.Query().Get("include_system"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "include_system is invalid")
			return
		}

		result, err := transactionService.AccountBalances(r.Context(), app.AccountBalancesInput{
			AsOf:          r.URL.Query().Get("as_of"),
			Status:        r.URL.Query().Get("status"),
			IncludeSystem: includeSystem,
		})
		if err != nil {
			writeLedgerServiceError(w, r, logger, "read account balances", err)
			return
		}

		writeJSON(w, http.StatusOK, toAccountBalancesResponse(result))
	}
}

func categoryTotals(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		result, err := transactionService.CategoryTotals(r.Context(), app.CategoryTotalsInput{
			AfterDate:    r.URL.Query().Get("after_date"),
			BeforeDate:   r.URL.Query().Get("before_date"),
			Status:       r.URL.Query().Get("status"),
			CategoryType: r.URL.Query().Get("category_type"),
		})
		if err != nil {
			writeLedgerServiceError(w, r, logger, "read category totals", err)
			return
		}

		writeJSON(w, http.StatusOK, toCategoryTotalsResponse(result))
	}
}

func netWorth(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		result, err := transactionService.NetWorth(r.Context(), app.NetWorthInput{
			AsOf:   r.URL.Query().Get("as_of"),
			Status: r.URL.Query().Get("status"),
		})
		if err != nil {
			writeLedgerServiceError(w, r, logger, "read net worth", err)
			return
		}

		writeJSON(w, http.StatusOK, netWorthResponse{
			AsOf:                result.AsOf,
			Status:              result.Status,
			Totals:              toBalanceQuantityResponses(result.Totals),
			ExcludedSystemRoles: result.ExcludedSystemRoles,
		})
	}
}

func netWorthSeries(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		result, err := transactionService.NetWorthSeries(r.Context(), app.NetWorthSeriesInput{
			StartDate: r.URL.Query().Get("start_date"),
			EndDate:   r.URL.Query().Get("end_date"),
			Bucket:    r.URL.Query().Get("bucket"),
		})
		if err != nil {
			writeLedgerServiceError(w, r, logger, "read net worth series", err)
			return
		}

		buckets := make([]netWorthSeriesBucketResponse, 0, len(result.Buckets))
		for _, bucket := range result.Buckets {
			buckets = append(buckets, netWorthSeriesBucketResponse{
				StartDate: bucket.StartDate,
				EndDate:   bucket.EndDate,
				Totals:    toBalanceQuantityResponses(bucket.Totals),
			})
		}
		writeJSON(w, http.StatusOK, netWorthSeriesResponse{
			StartDate: result.StartDate,
			EndDate:   result.EndDate,
			Bucket:    result.Bucket,
			Query: netWorthSeriesQueryResponse{
				StartDate: result.StartDate,
				EndDate:   result.EndDate,
				Bucket:    result.Bucket,
			},
			Buckets:             buckets,
			ExcludedSystemRoles: result.ExcludedSystemRoles,
		})
	}
}

func writeLedgerServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, operation string, err error) {
	var validationErr app.ValidationError
	var overflowErr app.LedgerOverflowError
	switch {
	case errors.As(err, &validationErr):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationErr.Message)
	case errors.As(err, &overflowErr):
		writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", overflowErr.Error())
	default:
		writeServiceInternalError(w, r, logger, operation, err)
	}
}

func toAccountBalancesResponse(result app.AccountBalancesResult) accountBalancesResponse {
	accounts := make([]accountBalanceResponse, 0, len(result.Accounts))
	for _, account := range result.Accounts {
		accounts = append(accounts, accountBalanceResponse{
			AccountID:       account.AccountID,
			BookID:          account.BookID,
			IsSystem:        account.IsSystem,
			SystemRole:      account.SystemRole,
			Status:          account.Status,
			Code:            account.Code,
			Name:            account.Name,
			AccountClass:    account.AccountClass,
			AccountKind:     account.AccountKind,
			ParentAccountID: account.ParentAccountID,
			AllowsPostings:  account.AllowsPostings,
			DirectBalances:  toBalanceQuantityResponses(account.DirectBalances),
			SubtreeBalances: toBalanceQuantityResponses(account.SubtreeBalances),
		})
	}
	return accountBalancesResponse{
		AsOf:     result.AsOf,
		Status:   result.Status,
		Accounts: accounts,
	}
}

func toCategoryTotalsResponse(result app.CategoryTotalsResult) categoryTotalsResponse {
	categories := make([]categoryTotalResponse, 0, len(result.Categories))
	for _, category := range result.Categories {
		categories = append(categories, categoryTotalResponse{
			CategoryID:       category.CategoryID,
			BookID:           category.BookID,
			Status:           category.Status,
			Code:             category.Code,
			Name:             category.Name,
			CategoryType:     category.CategoryType,
			ParentCategoryID: category.ParentCategoryID,
			AllowsPostings:   category.AllowsPostings,
			DirectTotals:     toBalanceQuantityResponses(category.DirectTotals),
			SubtreeTotals:    toBalanceQuantityResponses(category.SubtreeTotals),
		})
	}
	return categoryTotalsResponse{
		AfterDate:  result.AfterDate,
		BeforeDate: result.BeforeDate,
		Status:     result.Status,
		Categories: categories,
	}
}

func toBalanceQuantityResponses(quantities []app.BalanceQuantity) []balanceQuantityResponse {
	responses := make([]balanceQuantityResponse, 0, len(quantities))
	for _, quantity := range quantities {
		responses = append(responses, toBalanceQuantityResponse(quantity))
	}
	return responses
}

func toBalanceQuantityResponse(quantity app.BalanceQuantity) balanceQuantityResponse {
	return balanceQuantityResponse{
		CommodityID:         quantity.CommodityID,
		QuantityValue:       quantity.QuantityValue,
		QuantityScale:       quantity.QuantityScale,
		NormalQuantityValue: quantity.NormalQuantityValue,
	}
}
