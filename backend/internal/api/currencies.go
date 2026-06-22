package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

type currencyCatalogResponse struct {
	Currencies []currencyCatalogEntryResponse `json:"currencies"`
}

type currencyCatalogEntryResponse struct {
	Code             string `json:"code"`
	Symbol           string `json:"symbol"`
	EnglishName      string `json:"english_name"`
	StandardScale    int    `json:"standard_scale"`
	MaxQuantityScale int    `json:"max_quantity_scale"`
}

type currencyResponse struct {
	ID               int64  `json:"id"`
	BookID           int64  `json:"book_id"`
	Code             string `json:"code"`
	Name             string `json:"name"`
	Symbol           string `json:"symbol"`
	DisplaySymbol    string `json:"display_symbol"`
	Status           string `json:"status"`
	StandardScale    int    `json:"standard_scale"`
	MaxQuantityScale int    `json:"max_quantity_scale"`
	IsBuiltin        bool   `json:"is_builtin"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type currenciesResponse struct {
	Currencies []currencyResponse `json:"currencies"`
}

type createCurrencyRequest struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type setupCurrencySelectionRequest struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type completeCurrencySetupRequest struct {
	DefaultCurrencyCode string                          `json:"default_currency_code"`
	Currencies          []setupCurrencySelectionRequest `json:"currencies"`
}

type completeCurrencySetupResponse struct {
	Currencies      []currencyResponse  `json:"currencies"`
	DefaultCurrency currencyResponse    `json:"default_currency"`
	Setup           setupStatusResponse `json:"setup"`
}

type setDefaultCurrencyResponse struct {
	DefaultCurrency currencyResponse `json:"default_currency"`
}

func currencyCatalog(logger *slog.Logger, authService *app.AuthService, currencyService *app.CurrencyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		writeJSON(w, http.StatusOK, currencyCatalogResponse{
			Currencies: toCurrencyCatalogEntryResponses(currencyService.Catalog()),
		})
	}
}

func listCurrencies(logger *slog.Logger, authService *app.AuthService, currencyService *app.CurrencyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		currencies, err := currencyService.ListCurrencies(r.Context())
		if err != nil {
			logger.ErrorContext(r.Context(), "list currencies", slog.Any("err", err))
			writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			return
		}

		writeJSON(w, http.StatusOK, currenciesResponse{Currencies: toCurrencyResponses(currencies)})
	}
}

func createCurrency(logger *slog.Logger, authService *app.AuthService, currencyService *app.CurrencyService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var request createCurrencyRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		currency, err := currencyService.CreateCurrency(r.Context(), app.CreateCurrencyInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			Operation:     "currency.create",
			Code:          request.Code,
			Name:          request.Name,
		})
		if err != nil {
			writeCurrencyServiceError(w, r, logger, "create currency", err)
			return
		}

		writeJSON(w, http.StatusCreated, toCurrencyResponse(currency))
	}))
}

func completeCurrencySetup(logger *slog.Logger, authService *app.AuthService, currencyService *app.CurrencyService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var request completeCurrencySetupRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		result, err := currencyService.CompleteCurrencySetup(r.Context(), app.CompleteCurrencySetupInput{
			OwnerUserID:         owner.ID,
			AuthSessionID:       authenticatedSessionID(r),
			RequestID:           RequestIDFromContext(r.Context()),
			OriginType:          "setup",
			Operation:           "currencies.setup",
			DefaultCurrencyCode: request.DefaultCurrencyCode,
			Currencies:          toCurrencySelectionInputs(request.Currencies),
		})
		if err != nil {
			writeCurrencyServiceError(w, r, logger, "complete currencies setup", err)
			return
		}

		writeJSON(w, http.StatusCreated, completeCurrencySetupResponse{
			Currencies:      toCurrencyResponses(result.Currencies),
			DefaultCurrency: toCurrencyResponse(result.DefaultCurrency),
			Setup:           toSetupStatusResponse(result.SetupStatus),
		})
	}))
}

func setDefaultCurrency(logger *slog.Logger, authService *app.AuthService, currencyService *app.CurrencyService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		commodityID, err := strconv.ParseInt(r.PathValue("commodity_id"), 10, 64)
		if err != nil || commodityID <= 0 {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "currency id is invalid")
			return
		}

		result, err := currencyService.SetDefaultCurrency(r.Context(), app.SetDefaultCurrencyInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			Operation:     "book.default_currency.set",
			CommodityID:   commodityID,
		})
		if err != nil {
			writeCurrencyServiceError(w, r, logger, "set default currency", err)
			return
		}

		writeJSON(w, http.StatusOK, setDefaultCurrencyResponse{
			DefaultCurrency: toCurrencyResponse(result.DefaultCurrency),
		})
	}))
}

func writeCurrencyServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	var validationError app.ValidationError
	switch {
	case errors.As(err, &validationError):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
	case errors.Is(err, app.ErrCurrencyNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "currency not found")
	case errors.Is(err, app.ErrCurrencyAlreadyExists):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "currency already exists")
	case errors.Is(err, app.ErrCurrenciesAlreadySetup):
		writeAPIError(w, http.StatusConflict, "SETUP_ALREADY_COMPLETE", "currency setup is already complete")
	default:
		logger.ErrorContext(r.Context(), action, slog.Any("err", err))
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func toCurrencyCatalogEntryResponses(entries []app.CurrencyCatalogEntry) []currencyCatalogEntryResponse {
	responses := make([]currencyCatalogEntryResponse, 0, len(entries))
	for _, entry := range entries {
		responses = append(responses, currencyCatalogEntryResponse{
			Code:             entry.Code,
			Symbol:           entry.Symbol,
			EnglishName:      entry.EnglishName,
			StandardScale:    entry.StandardScale,
			MaxQuantityScale: entry.MaxQuantityScale,
		})
	}

	return responses
}

func toCurrencySelectionInputs(requests []setupCurrencySelectionRequest) []app.CurrencySelectionInput {
	inputs := make([]app.CurrencySelectionInput, 0, len(requests))
	for _, request := range requests {
		inputs = append(inputs, app.CurrencySelectionInput{
			Code: request.Code,
			Name: request.Name,
		})
	}

	return inputs
}

func toCurrencyResponse(currency app.Currency) currencyResponse {
	return currencyResponse{
		ID:               currency.ID,
		BookID:           currency.BookID,
		Code:             currency.Code,
		Name:             currency.Name,
		Symbol:           currency.Symbol,
		DisplaySymbol:    currency.DisplaySymbol,
		Status:           currency.Status,
		StandardScale:    currency.StandardScale,
		MaxQuantityScale: currency.MaxQuantityScale,
		IsBuiltin:        currency.IsBuiltin,
		CreatedAt:        currency.CreatedAt,
		UpdatedAt:        currency.UpdatedAt,
	}
}

func toCurrencyResponses(currencies []app.Currency) []currencyResponse {
	responses := make([]currencyResponse, 0, len(currencies))
	for _, currency := range currencies {
		responses = append(responses, toCurrencyResponse(currency))
	}

	return responses
}
