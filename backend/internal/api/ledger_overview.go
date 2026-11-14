package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

type ledgerPositionResponse struct {
	HoldingAccountID int64                    `json:"holding_account_id"`
	CommodityID      int64                    `json:"commodity_id"`
	Quantity         balanceQuantityResponse  `json:"quantity"`
	Value            *balanceQuantityResponse `json:"value"`
}
type ledgerImportSourceResponse struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}
type ledgerAccountResponse struct {
	CashAccounts      []accountResponse            `json:"cash_accounts"`
	OtherAccounts     []accountResponse            `json:"other_accounts"`
	HoldingAccounts   []accountResponse            `json:"holding_accounts"`
	Sources           []ledgerImportSourceResponse `json:"sources"`
	Account           accountResponse              `json:"account"`
	Balances          []balanceQuantityResponse    `json:"balances"`
	Cash              []balanceQuantityResponse    `json:"cash"`
	Unsettled         []balanceQuantityResponse    `json:"unsettled"`
	Positions         []ledgerPositionResponse     `json:"positions"`
	Value             *balanceQuantityResponse     `json:"value"`
	Valuation         *valuationResponse           `json:"valuation"`
	UnclassifiedShort bool                         `json:"unclassified_short"`
}
type ledgerOverviewResponse struct {
	AsOf         string                          `json:"as_of"`
	Accounts     []ledgerAccountResponse         `json:"accounts"`
	Institutions []institutionResponse           `json:"institutions"`
	Commodities  []homeReportingCurrencyResponse `json:"commodities"`
	NextCursor   *string                         `json:"next_cursor"`
}

func ledgerOverview(logger *slog.Logger, auth *app.AuthService, service *app.LedgerService, detail bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		input := app.LedgerOverviewInput{OwnerUserID: owner.ID, AsOf: r.URL.Query().Get("as_of"), Cursor: r.URL.Query().Get("cursor")}
		if detail {
			var err error
			input.AccountID, err = strconv.ParseInt(r.PathValue("account_id"), 10, 64)
			if err != nil || input.AccountID <= 0 {
				writeAPIError(w, 400, "VALIDATION_FAILED", "account_id is invalid")
				return
			}
		}
		result, err := service.Overview(r.Context(), input)
		if err != nil {
			if errors.Is(err, app.ErrAccountNotFound) {
				writeAccountServiceError(w, r, logger, "read ledger overview", err)
			} else {
				writeLedgerServiceError(w, r, logger, "read ledger overview", err)
			}
			return
		}
		response := ledgerOverviewResponse{AsOf: result.AsOf, Accounts: []ledgerAccountResponse{}, Institutions: toInstitutionResponses(result.Institutions), Commodities: []homeReportingCurrencyResponse{}}
		if result.NextCursor != "" {
			response.NextCursor = &result.NextCursor
		}
		for _, c := range result.Commodities {
			response.Commodities = append(response.Commodities, homeReportingCurrencyResponse{CommodityID: c.ID, Code: c.Code, Kind: c.Kind, Symbol: c.DisplaySymbol, Scale: c.StandardScale})
		}
		for _, row := range result.Accounts {
			item := ledgerAccountResponse{Account: toAccountResponse(row.Account), CashAccounts: toAccountResponses(row.CashAccounts), OtherAccounts: toAccountResponses(row.OtherAccounts), HoldingAccounts: toAccountResponses(row.HoldingAccounts), Balances: toBalanceQuantityResponses(row.Balances), Cash: toBalanceQuantityResponses(row.Cash), Unsettled: toBalanceQuantityResponses(row.Unsettled), Positions: []ledgerPositionResponse{}, Value: optionalBalanceQuantity(row.Value), Valuation: toValuationResponse(row.Valuation), UnclassifiedShort: row.UnclassifiedShort}
			for _, p := range row.Positions {
				item.Positions = append(item.Positions, ledgerPositionResponse{HoldingAccountID: p.HoldingAccountID, CommodityID: p.CommodityID, Quantity: toBalanceQuantityResponse(p.Quantity), Value: optionalBalanceQuantity(p.Value)})
			}
			item.Sources = []ledgerImportSourceResponse{}
			for _, source := range row.Sources {
				item.Sources = append(item.Sources, ledgerImportSourceResponse{Kind: source.Kind, Name: source.Name})
			}
			response.Accounts = append(response.Accounts, item)
		}
		writeJSON(w, 200, response)
	}
}
