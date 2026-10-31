package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

type pricingCommodityResponse struct {
	ID     int64  `json:"id"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

type pricingCoverageResponse struct {
	Reason           string                    `json:"reason,omitempty"`
	BaseCommodityID  int64                     `json:"base_commodity_id"`
	QuoteCommodityID int64                     `json:"quote_commodity_id"`
	Status           string                    `json:"status"`
	AgeDays          *int                      `json:"age_days,omitempty"`
	Observation      *priceObservationResponse `json:"observation,omitempty"`
}

type pricingWorkspaceResponse struct {
	AsOf                string                     `json:"as_of"`
	StalenessMaxDays    int                        `json:"staleness_max_days"`
	ValuationWindowDays int                        `json:"valuation_window_days"`
	Commodities         []pricingCommodityResponse `json:"commodities"`
	Sources             []marketDataSourceResponse `json:"sources"`
	Coverage            []pricingCoverageResponse  `json:"coverage"`
}

func pricingWorkspace(logger *slog.Logger, auth *app.AuthService, service *app.PricingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, auth); !ok {
			return
		}
		result, err := service.Workspace(r.Context(), r.URL.Query().Get("as_of"))
		if err != nil {
			writePricingServiceError(w, r, logger, "read price workspace", err)
			return
		}
		response := pricingWorkspaceResponse{AsOf: result.AsOf, StalenessMaxDays: result.StalenessMaxDays, ValuationWindowDays: result.ValuationWindowDays, Commodities: []pricingCommodityResponse{}, Sources: toMarketDataSourceResponses(result.Sources), Coverage: []pricingCoverageResponse{}}
		for _, commodity := range result.Commodities {
			response.Commodities = append(response.Commodities, pricingCommodityResponse{
				ID: commodity.ID, Code: commodity.Code, Name: commodity.Name, Kind: commodity.Kind, Status: commodity.Status,
			})
		}

		for _, row := range result.Coverage {
			item := pricingCoverageResponse{Reason: row.Reason, BaseCommodityID: row.BaseCommodityID, QuoteCommodityID: row.QuoteCommodityID, Status: row.Status, AgeDays: row.AgeDays}
			if row.Observation != nil {
				price := toPriceObservationResponse(*row.Observation)
				item.Observation = &price
			}
			response.Coverage = append(response.Coverage, item)
		}
		writeJSON(w, http.StatusOK, response)
	}
}

type priceHistoryResponse struct {
	Prices     []priceObservationResponse `json:"prices"`
	NextCursor string                     `json:"next_cursor,omitempty"`
}

func priceHistory(logger *slog.Logger, auth *app.AuthService, service *app.PricingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, auth); !ok {
			return
		}
		base, ok := parseOptionalPositiveInt64(w, r.URL.Query().Get("base_commodity_id"), "base commodity id")
		if !ok {
			return
		}
		quote, ok := parseOptionalPositiveInt64(w, r.URL.Query().Get("quote_commodity_id"), "quote commodity id")
		if !ok {
			return
		}
		limit := 50
		if value := r.URL.Query().Get("limit"); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed <= 0 {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid history limit")
				return
			}
			limit = parsed
		}
		include := r.URL.Query().Get("include_voided")
		if include != "" && include != "true" && include != "false" {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid include voided filter")
			return
		}
		result, err := service.History(r.Context(), base, quote, include == "true", r.URL.Query().Get("cursor"), limit)
		if err != nil {
			writePricingServiceError(w, r, logger, "read price history", err)
			return
		}
		writeJSON(w, http.StatusOK, priceHistoryResponse{Prices: toPriceObservationResponses(result.Prices), NextCursor: result.NextCursor})
	}
}

type manualPriceRequest struct {
	BaseCommodityID   int64  `json:"base_commodity_id"`
	QuoteCommodityID  int64  `json:"quote_commodity_id"`
	ValuationDate     string `json:"valuation_date"`
	PriceValue        string `json:"price_value"`
	PriceScale        int    `json:"price_scale"`
	BaseQuantityValue string `json:"base_quantity_value"`
	BaseQuantityScale int    `json:"base_quantity_scale"`
	AdjustmentBasis   string `json:"adjustment_basis"`
	ObservedAt        string `json:"observed_at"`
	Reference         string `json:"reference"`
	Reason            string `json:"reason"`
	ExpectedImpact    string `json:"expected_impact"`
}

type priceImpactResponse struct {
	Prices []priceObservationResponse `json:"prices"`
	Token  string                     `json:"token"`
}
type correctPriceResponse struct {
	Replacement priceObservationResponse   `json:"replacement"`
	Retired     []priceObservationResponse `json:"retired"`
}

func previewPriceImpact(logger *slog.Logger, auth *app.AuthService, service *app.PricingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, auth); !ok {
			return
		}
		id, ok := readPathInt64(w, r, "price_id", "price observation id")
		if !ok {
			return
		}
		result, err := service.PreviewPriceImpact(r.Context(), id)
		if err != nil {
			writePricingServiceError(w, r, logger, "preview price impact", err)
			return
		}
		writeJSON(w, http.StatusOK, priceImpactResponse{Prices: toPriceObservationResponses(result.Prices), Token: result.Token})
	}
}

func writeManualPrice(logger *slog.Logger, auth *app.AuthService, service *app.PricingService, options HandlerOptions, correct bool) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, auth, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request manualPriceRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		input := app.ManualPriceInput{OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			BaseCommodityID: request.BaseCommodityID, QuoteCommodityID: request.QuoteCommodityID, ValuationDate: request.ValuationDate,
			PriceValue: request.PriceValue, PriceScale: request.PriceScale, BaseQuantityValue: request.BaseQuantityValue, BaseQuantityScale: request.BaseQuantityScale,
			AdjustmentBasis: request.AdjustmentBasis, ObservedAt: request.ObservedAt, Reference: request.Reference, Reason: request.Reason}
		if correct {
			id, ok := readPathInt64(w, r, "price_id", "price observation id")
			if !ok {
				return
			}
			result, err := service.CorrectPrice(r.Context(), app.CorrectPriceInput{ManualPriceInput: input, ObservationID: id, ExpectedImpact: request.ExpectedImpact})
			if err != nil {
				writePricingServiceError(w, r, logger, "correct price", err)
				return
			}
			writeJSON(w, http.StatusCreated, correctPriceResponse{Replacement: toPriceObservationResponse(result.Replacement), Retired: toPriceObservationResponses(result.Retired)})
		} else {
			if request.ExpectedImpact != "" {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "manual entry does not accept an impact token")
				return
			}
			result, err := service.CreateManualPrice(r.Context(), input)
			if err != nil {
				writePricingServiceError(w, r, logger, "create manual price", err)
				return
			}
			writeJSON(w, http.StatusCreated, toPriceObservationResponse(result))
		}
	}))
}
