package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type homeReportingCurrencyResponse struct {
	CommodityID int64  `json:"commodity_id"`
	Code        string `json:"code"`
	Symbol      string `json:"symbol"`
	Scale       int    `json:"scale"`
}

type homeValuationInputResponse struct {
	CommodityID int64  `json:"commodity_id"`
	Code        string `json:"code"`
	Kind        string `json:"kind"`
}

type homeHeadlineResponse struct {
	ID           string                       `json:"id"`
	AccountCount int                          `json:"account_count"`
	Value        *balanceQuantityResponse     `json:"value"`
	StartValue   *balanceQuantityResponse     `json:"start_value"`
	Change       *balanceQuantityResponse     `json:"change"`
	Unavailable  *string                      `json:"unavailable"`
	Missing      []homeValuationInputResponse `json:"missing"`
	Stale        []homeValuationInputResponse `json:"stale"`
}

type homeResponse struct {
	AsOf              string                         `json:"as_of"`
	StartBoundary     string                         `json:"start_boundary"`
	ReportingCurrency *homeReportingCurrencyResponse `json:"reporting_currency"`
	EmptyBook         bool                           `json:"empty_book"`
	Headlines         []homeHeadlineResponse         `json:"headlines"`
}

func homeRead(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		result, err := transactionService.HomeBriefing(r.Context(), owner.ID)
		if err != nil {
			writeLedgerServiceError(w, r, logger, "read home", err)
			return
		}
		response := homeResponse{AsOf: result.AsOf, StartBoundary: result.StartBoundary, EmptyBook: result.EmptyBook, Headlines: []homeHeadlineResponse{}}
		if currency := result.ReportingCurrency; currency != nil {
			response.ReportingCurrency = &homeReportingCurrencyResponse{CommodityID: currency.ID, Code: currency.Code, Symbol: currency.DisplaySymbol, Scale: currency.StandardScale}
		}
		for _, headline := range result.Headlines {
			item := homeHeadlineResponse{
				ID:           headline.ID,
				AccountCount: headline.AccountCount,
				Value:        toBalanceQuantityPointer(headline.Value),
				StartValue:   toBalanceQuantityPointer(headline.StartValue),
				Change:       toBalanceQuantityPointer(headline.Change),
				Missing:      toHomeValuationInputs(headline.Missing),
				Stale:        toHomeValuationInputs(headline.Stale),
			}
			if headline.Unavailable != "" {
				item.Unavailable = &headline.Unavailable
			}
			response.Headlines = append(response.Headlines, item)
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, response)
	}
}

func toHomeValuationInputs(inputs []app.HomeValuationInput) []homeValuationInputResponse {
	response := make([]homeValuationInputResponse, 0, len(inputs))
	for _, input := range inputs {
		response = append(response, homeValuationInputResponse{CommodityID: input.CommodityID, Code: input.Code, Kind: input.Kind})
	}
	return response
}
