package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type recurringAmountResponse struct {
	CommodityID   int64             `json:"commodity_id"`
	CommodityCode string            `json:"commodity_code"`
	DebitValue    exact.Coefficient `json:"debit_value"`
	CreditValue   exact.Coefficient `json:"credit_value"`
	QuantityScale int               `json:"quantity_scale"`
}

type recurringDueResponse struct {
	Items      []recurringDueItemResponse `json:"items"`
	NextCursor *string                    `json:"next_cursor"`
}

type recurringDueItemResponse struct {
	ID               int64                     `json:"id"`
	TemplateID       int64                     `json:"template_id"`
	TemplateName     string                    `json:"template_name"`
	TemplateEnabled  bool                      `json:"template_enabled"`
	TemplateArchived bool                      `json:"template_archived"`
	OccurrenceDate   string                    `json:"occurrence_date"`
	Status           string                    `json:"status"`
	TransactionID    *int64                    `json:"transaction_id"`
	Description      string                    `json:"description"`
	PayeeName        string                    `json:"payee_name"`
	ErrorSummary     string                    `json:"error_summary"`
	Amounts          []recurringAmountResponse `json:"amounts"`
}

type recurringOccurrenceResponse struct {
	ID             *int64 `json:"id"`
	OccurrenceDate string `json:"occurrence_date"`
	Status         string `json:"status"`
	TransactionID  *int64 `json:"transaction_id"`
	ErrorSummary   string `json:"error_summary"`
	SkipReason     string `json:"skip_reason"`
}

type recurringOccurrencesResponse struct {
	Occurrences []recurringOccurrenceResponse `json:"occurrences"`
}

type recurringReviewRequest struct {
	OccurrenceDate string `json:"occurrence_date"`
	Reason         string `json:"reason"`
}

type recurringGenerationResponse struct {
	Generated int `json:"generated"`
	Blocked   int `json:"blocked"`
}

func listRecurringDue(logger *slog.Logger, auth *app.AuthService, service *app.RecurringService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		limit := 0
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 200 {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "limit must be between 1 and 200")
				return
			}
			limit = parsed
		}
		page, err := service.Due(r.Context(), owner.ID, r.URL.Query().Get("cursor"), limit)
		if err != nil {
			writeRecurringServiceError(w, r, logger, err)
			return
		}
		response := recurringDueResponse{Items: make([]recurringDueItemResponse, 0, len(page.Items)), NextCursor: recurringOptionalText(page.NextCursor)}
		for _, item := range page.Items {
			row := recurringDueItemResponse{ID: item.ID, TemplateID: item.TemplateID, TemplateName: item.TemplateName,
				TemplateEnabled: item.TemplateEnabled, TemplateArchived: item.TemplateArchived, OccurrenceDate: item.OccurrenceDate, Status: item.Status,
				Description: item.Description, PayeeName: item.PayeeName, ErrorSummary: item.ErrorSummary, Amounts: make([]recurringAmountResponse, 0, len(item.Amounts))}
			if item.TransactionID.Valid {
				id := item.TransactionID.Int64
				row.TransactionID = &id
			}
			for _, a := range item.Amounts {
				row.Amounts = append(row.Amounts, recurringAmountResponse{CommodityID: a.CommodityID, CommodityCode: a.CommodityCode, DebitValue: a.DebitValue, CreditValue: a.CreditValue, QuantityScale: a.QuantityScale})
			}
			response.Items = append(response.Items, row)
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func listRecurringOccurrences(logger *slog.Logger, auth *app.AuthService, service *app.RecurringService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		id, ok := readRecurringTemplateID(w, r)
		if !ok {
			return
		}
		items, err := service.Occurrences(r.Context(), owner.ID, id, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		if err != nil {
			writeRecurringServiceError(w, r, logger, err)
			return
		}
		response := recurringOccurrencesResponse{Occurrences: make([]recurringOccurrenceResponse, 0, len(items))}
		for _, item := range items {
			row := recurringOccurrenceResponse{OccurrenceDate: item.Date, Status: item.Status}
			if item.Record != nil {
				id := item.Record.ID
				row.ID = &id
				row.ErrorSummary = item.Record.ErrorSummary
				row.SkipReason = item.Record.SkipReason
				if item.Record.TransactionID.Valid {
					transactionID := item.Record.TransactionID.Int64
					row.TransactionID = &transactionID
				}
			}
			response.Occurrences = append(response.Occurrences, row)
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func reviewRecurringOccurrence(logger *slog.Logger, auth *app.AuthService, service *app.RecurringService, options HandlerOptions, action string) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, auth, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		id, ok := readRecurringTemplateID(w, r)
		if !ok {
			return
		}
		var request recurringReviewRequest
		if action == "skip" {
			if err := decodeJSONBody(r, &request); err != nil {
				writeDecodeError(w, err)
				return
			}
		} else {
			var retry struct {
				OccurrenceDate string `json:"occurrence_date"`
			}
			if err := decodeJSONBody(r, &retry); err != nil {
				writeDecodeError(w, err)
				return
			}
			request.OccurrenceDate = retry.OccurrenceDate
		}
		input := app.ReviewRecurringInput{OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()), TemplateID: id, OccurrenceDate: request.OccurrenceDate, Reason: request.Reason}
		if action == "skip" {
			if err := service.SkipOccurrence(r.Context(), input); err != nil {
				writeRecurringServiceError(w, r, logger, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		result, err := service.RetryOccurrence(r.Context(), input)
		if err != nil {
			writeRecurringServiceError(w, r, logger, err)
			return
		}
		writeJSON(w, http.StatusOK, recurringGenerationResponse{Generated: result.Generated, Blocked: result.Blocked})
	}))
}
