package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type cashflowBucketTotalResponse struct {
	CommodityID   int64             `json:"commodity_id"`
	QuantityScale int               `json:"quantity_scale"`
	Inflow        exact.Coefficient `json:"inflow"`
	Outflow       exact.Coefficient `json:"outflow"`
	OperatingNet  exact.Coefficient `json:"operating_net"`
	TransferIn    exact.Coefficient `json:"transfer_in"`
	TransferOut   exact.Coefficient `json:"transfer_out"`
	NetMovement   exact.Coefficient `json:"net_movement"`
}

type cashflowBucketResponse struct {
	StartDate string                        `json:"start_date"`
	EndDate   string                        `json:"end_date"`
	Totals    []cashflowBucketTotalResponse `json:"totals"`
}

type cashflowScopeAccountResponse struct {
	AccountID   int64  `json:"account_id"`
	Name        string `json:"name,omitempty"`
	Code        string `json:"code,omitempty"`
	AccountKind string `json:"account_kind"`
}

type cashflowQueryResponse struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Bucket    string `json:"bucket"`
}

type cashflowResponse struct {
	StartDate           string                         `json:"start_date"`
	EndDate             string                         `json:"end_date"`
	Bucket              string                         `json:"bucket"`
	Query               cashflowQueryResponse          `json:"query"`
	ScopeKinds          []string                       `json:"scope_kinds"`
	ScopeAccounts       []cashflowScopeAccountResponse `json:"scope_accounts"`
	Buckets             []cashflowBucketResponse       `json:"buckets"`
	ExcludedSystemRoles []string                       `json:"excluded_system_roles"`
}

func cashflowReport(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		result, err := transactionService.Cashflow(r.Context(), app.CashflowReportInput{
			StartDate: r.URL.Query().Get("start_date"),
			EndDate:   r.URL.Query().Get("end_date"),
			Bucket:    r.URL.Query().Get("bucket"),
		})
		if err != nil {
			writeLedgerServiceError(w, r, logger, "read cashflow report", err)
			return
		}

		buckets := make([]cashflowBucketResponse, 0, len(result.Buckets))
		for _, bucket := range result.Buckets {
			totals := make([]cashflowBucketTotalResponse, 0, len(bucket.Totals))
			for _, total := range bucket.Totals {
				totals = append(totals, cashflowBucketTotalResponse{
					CommodityID:   total.CommodityID,
					QuantityScale: total.QuantityScale,
					Inflow:        total.Inflow,
					Outflow:       total.Outflow,
					OperatingNet:  total.OperatingNet,
					TransferIn:    total.TransferIn,
					TransferOut:   total.TransferOut,
					NetMovement:   total.NetMovement,
				})
			}
			buckets = append(buckets, cashflowBucketResponse{
				StartDate: bucket.StartDate,
				EndDate:   bucket.EndDate,
				Totals:    totals,
			})
		}

		scopeAccounts := make([]cashflowScopeAccountResponse, 0, len(result.ScopeAccounts))
		for _, account := range result.ScopeAccounts {
			scopeAccounts = append(scopeAccounts, cashflowScopeAccountResponse{
				AccountID:   account.AccountID,
				Name:        account.Name,
				Code:        account.Code,
				AccountKind: account.AccountKind,
			})
		}

		writeJSON(w, http.StatusOK, cashflowResponse{
			StartDate: result.StartDate,
			EndDate:   result.EndDate,
			Bucket:    result.Bucket,
			Query: cashflowQueryResponse{
				StartDate: result.StartDate,
				EndDate:   result.EndDate,
				Bucket:    result.Bucket,
			},
			ScopeKinds:          result.ScopeKinds,
			ScopeAccounts:       scopeAccounts,
			Buckets:             buckets,
			ExcludedSystemRoles: result.ExcludedSystemRoles,
		})
	}
}
