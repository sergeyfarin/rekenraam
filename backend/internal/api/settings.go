package api

import (
	"errors"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type userPreferencesResponse struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	TimeZone  string `json:"time_zone"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type userPreferencesRequest struct {
	TimeZone     string `json:"time_zone"`
	ChangeReason string `json:"change_reason"`
}

type currencySettingsPageResponse struct {
	Book         bookResponse                      `json:"book"`
	Preferences  userPreferencesResponse           `json:"preferences"`
	Currencies   []currencyResponse                `json:"currencies"`
	Sources      []marketDataSourceResponse        `json:"sources"`
	Policy       pricingPolicyResponse             `json:"policy"`
	Assignments  []pricingSourceAssignmentResponse `json:"assignments"`
	RefreshRuns  []pricingRefreshRunResponse       `json:"refresh_runs"`
	SourceHealth []pricingSourceHealthResponse     `json:"source_health"`
	LatestRates  []priceObservationResponse        `json:"latest_rates"`
}

func userPreferences(logger *slog.Logger, authService *app.AuthService, settingsService *app.SettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}

		preferences, err := settingsService.Preferences(r.Context(), owner.ID)
		if err != nil {
			logger.ErrorContext(r.Context(), "read user preferences", slog.Any("err", err))
			writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			return
		}

		writeJSON(w, http.StatusOK, toUserPreferencesResponse(preferences))
	}
}

func saveUserPreferences(logger *slog.Logger, authService *app.AuthService, settingsService *app.SettingsService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var request userPreferencesRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		preferences, err := settingsService.SavePreferences(r.Context(), app.SaveUserPreferencesInput{
			UserID:        owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			TimeZone:      request.TimeZone,
			ChangeReason:  request.ChangeReason,
		})
		if err != nil {
			var validationError app.ValidationError
			switch {
			case errors.As(err, &validationError):
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
			default:
				logger.ErrorContext(r.Context(), "save user preferences", slog.Any("err", err))
				writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusOK, toUserPreferencesResponse(preferences))
	}))
}

func currencySettingsPageData(logger *slog.Logger, authService *app.AuthService, settingsService *app.SettingsService, bookService *app.BookService, currencyService *app.CurrencyService, pricingService *app.PricingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}

		book, err := bookService.CurrentBook(r.Context())
		if err != nil {
			writeCurrencySettingsPageError(w, r, logger, "read currency settings book", err)
			return
		}

		preferences, err := settingsService.Preferences(r.Context(), owner.ID)
		if err != nil {
			writeCurrencySettingsPageError(w, r, logger, "read currency settings preferences", err)
			return
		}

		currencies, err := currencyService.ListCurrencies(r.Context())
		if err != nil {
			writeCurrencySettingsPageError(w, r, logger, "read currency settings currencies", err)
			return
		}

		sources, err := pricingService.ListSources(r.Context())
		if err != nil {
			writeCurrencySettingsPageError(w, r, logger, "read currency settings sources", err)
			return
		}

		policy, err := pricingService.GetPolicy(r.Context())
		if err != nil {
			writeCurrencySettingsPageError(w, r, logger, "read currency settings policy", err)
			return
		}

		assignments, err := pricingService.ListSourceAssignments(r.Context())
		if err != nil {
			writeCurrencySettingsPageError(w, r, logger, "read currency settings assignments", err)
			return
		}

		refreshRuns, err := pricingService.ListRefreshRuns(r.Context(), 50)
		if err != nil {
			writeCurrencySettingsPageError(w, r, logger, "read currency settings refresh runs", err)
			return
		}

		sourceHealth, err := pricingService.SourceHealth(r.Context())
		if err != nil {
			writeCurrencySettingsPageError(w, r, logger, "read currency settings source health", err)
			return
		}

		latestRates, err := pricingService.LatestPricesForPairs(r.Context(), currencySettingsRatePairs(currencies, book.DefaultCurrencyCommodityID))
		if err != nil {
			writeCurrencySettingsPageError(w, r, logger, "read currency settings latest rates", err)
			return
		}

		writeJSON(w, http.StatusOK, currencySettingsPageResponse{
			Book:         toBookResponse(book),
			Preferences:  toUserPreferencesResponse(preferences),
			Currencies:   toCurrencyResponses(currencies),
			Sources:      toMarketDataSourceResponses(sources),
			Policy:       toPricingPolicyResponse(policy),
			Assignments:  toPricingSourceAssignmentResponses(assignments),
			RefreshRuns:  toPricingRefreshRunResponses(refreshRuns),
			SourceHealth: toPricingSourceHealthResponses(sourceHealth),
			LatestRates:  toPriceObservationResponses(latestRates),
		})
	}
}

func currencySettingsRatePairs(currencies []app.Currency, defaultCurrencyID *int64) [][2]int64 {
	if defaultCurrencyID == nil {
		return [][2]int64{}
	}

	pairs := make([][2]int64, 0, len(currencies)*2)
	for _, currency := range currencies {
		if currency.Status != "active" || currency.ID == *defaultCurrencyID {
			continue
		}
		pairs = append(pairs, [2]int64{currency.ID, *defaultCurrencyID}, [2]int64{*defaultCurrencyID, currency.ID})
	}
	return pairs
}

func writeCurrencySettingsPageError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	switch {
	case errors.Is(err, app.ErrBookNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "book not found")
	default:
		writePricingServiceError(w, r, logger, action, err)
	}
}

func toUserPreferencesResponse(preferences app.UserPreferences) userPreferencesResponse {
	return userPreferencesResponse{
		ID:        preferences.ID,
		UserID:    preferences.UserID,
		TimeZone:  preferences.TimeZone,
		CreatedAt: preferences.CreatedAt,
		UpdatedAt: preferences.UpdatedAt,
	}
}
