package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type investmentEntryAccountResponse struct {
	accountResponse
	InvestmentHolding    bool `json:"investment_holding"`
	InvestmentSettlement bool `json:"investment_settlement"`
}

func investmentEntryAccounts(logger *slog.Logger, auth *app.AuthService, service *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, auth); !ok {
			return
		}
		accounts, err := service.EntryAccounts(r.Context(), r.URL.Query().Get("as_of"))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "read entry accounts", err)
			return
		}
		result := struct {
			Accounts []investmentEntryAccountResponse `json:"accounts"`
		}{Accounts: make([]investmentEntryAccountResponse, 0, len(accounts))}
		for _, account := range accounts {
			result.Accounts = append(result.Accounts, investmentEntryAccountResponse{
				accountResponse: toAccountResponse(account.Account), InvestmentHolding: account.InvestmentHolding,
				InvestmentSettlement: account.InvestmentSettlement})
		}
		writeJSON(w, http.StatusOK, result)
	}
}
