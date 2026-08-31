package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

type nullableRecurringField[T any] struct{ app.NullablePatch[T] }

func (field *nullableRecurringField[T]) UnmarshalJSON(data []byte) error {
	field.Set = true
	return json.Unmarshal(data, &field.Value)
}

type recurringPostingRequest struct {
	LineKey       string            `json:"line_key"`
	AccountID     int64             `json:"account_id"`
	QuantityValue exact.Coefficient `json:"quantity_value"`
	QuantityScale int               `json:"quantity_scale"`
	CommodityID   int64             `json:"commodity_id"`
	Memo          string            `json:"memo"`
}

type recurringTemplateRequest struct {
	Name            *string                        `json:"name"`
	Enabled         *bool                          `json:"enabled"`
	TransactionKind *string                        `json:"transaction_kind"`
	PayeeID         nullableRecurringField[int64]  `json:"payee_id"`
	PayeeName       *string                        `json:"payee_name"`
	Description     *string                        `json:"description"`
	NoteMarkdown    *string                        `json:"note_markdown"`
	Frequency       *string                        `json:"frequency"`
	IntervalCount   *int                           `json:"interval_count"`
	ByWeekday       nullableRecurringField[int]    `json:"by_weekday"`
	DayOfMonth      nullableRecurringField[int]    `json:"day_of_month"`
	LastDayOfMonth  *bool                          `json:"last_day_of_month"`
	MonthOfYear     nullableRecurringField[int]    `json:"month_of_year"`
	StartsOn        *string                        `json:"starts_on"`
	EndsOn          nullableRecurringField[string] `json:"ends_on"`
	MaxOccurrences  nullableRecurringField[int]    `json:"max_occurrences"`
	LeadDays        *int                           `json:"lead_days"`
	Postings        *[]recurringPostingRequest     `json:"postings"`
	TagIDs          *[]int64                       `json:"tag_ids"`
}

type recurringTemplateResponse struct {
	ID              int64                     `json:"id"`
	Revision        int64                     `json:"revision"`
	Name            string                    `json:"name"`
	Enabled         bool                      `json:"enabled"`
	TransactionKind string                    `json:"transaction_kind"`
	PayeeID         *int64                    `json:"payee_id"`
	PayeeName       string                    `json:"payee_name"`
	Description     string                    `json:"description"`
	NoteMarkdown    string                    `json:"note_markdown"`
	Frequency       string                    `json:"frequency"`
	IntervalCount   int                       `json:"interval_count"`
	ByWeekday       *int                      `json:"by_weekday"`
	DayOfMonth      *int                      `json:"day_of_month"`
	LastDayOfMonth  bool                      `json:"last_day_of_month"`
	MonthOfYear     *int                      `json:"month_of_year"`
	StartsOn        string                    `json:"starts_on"`
	EndsOn          *string                   `json:"ends_on"`
	MaxOccurrences  *int                      `json:"max_occurrences"`
	LeadDays        int                       `json:"lead_days"`
	GenerateFrom    string                    `json:"generate_from"`
	NextDueOn       *string                   `json:"next_due_on"`
	ArchivedAt      *string                   `json:"archived_at"`
	CreatedAt       string                    `json:"created_at"`
	UpdatedAt       string                    `json:"updated_at"`
	Postings        []recurringPostingRequest `json:"postings"`
	TagIDs          []int64                   `json:"tag_ids"`
}

type recurringTemplatesResponse struct {
	Templates []recurringTemplateResponse `json:"templates"`
}

func (request recurringTemplateRequest) patch() app.RecurringTemplatePatch {
	p := app.RecurringTemplatePatch{Name: request.Name, Enabled: request.Enabled, TransactionKind: request.TransactionKind,
		PayeeID: request.PayeeID.NullablePatch, PayeeName: request.PayeeName, Description: request.Description,
		NoteMarkdown: request.NoteMarkdown, Frequency: request.Frequency, IntervalCount: request.IntervalCount,
		ByWeekday: request.ByWeekday.NullablePatch, DayOfMonth: request.DayOfMonth.NullablePatch,
		LastDayOfMonth: request.LastDayOfMonth, MonthOfYear: request.MonthOfYear.NullablePatch,
		StartsOn: request.StartsOn, EndsOn: request.EndsOn.NullablePatch, MaxOccurrences: request.MaxOccurrences.NullablePatch,
		LeadDays: request.LeadDays, TagIDs: request.TagIDs}
	if request.Postings != nil {
		postings := make([]db.RecurringTemplatePostingSpec, 0, len(*request.Postings))
		for _, posting := range *request.Postings {
			postings = append(postings, db.RecurringTemplatePostingSpec{LineKey: posting.LineKey, AccountID: posting.AccountID,
				QuantityValue: posting.QuantityValue, QuantityScale: posting.QuantityScale, CommodityID: posting.CommodityID, Memo: posting.Memo})
		}
		p.Postings = &postings
	}
	return p
}

func toRecurringTemplateResponse(template app.RecurringTemplate) recurringTemplateResponse {
	spec := template.Spec
	result := recurringTemplateResponse{ID: template.ID, Revision: template.Revision, Name: spec.Name, Enabled: spec.Enabled,
		TransactionKind: spec.TransactionKind, PayeeID: spec.PayeeID, PayeeName: spec.PayeeName, Description: spec.Description,
		NoteMarkdown: spec.NoteMarkdown, Frequency: spec.Frequency, IntervalCount: spec.IntervalCount, ByWeekday: spec.ByWeekday,
		DayOfMonth: spec.DayOfMonth, LastDayOfMonth: spec.LastDayOfMonth, MonthOfYear: spec.MonthOfYear, StartsOn: spec.StartsOn,
		EndsOn: recurringOptionalText(spec.EndsOn), MaxOccurrences: spec.MaxOccurrences, LeadDays: spec.LeadDays,
		GenerateFrom: spec.GenerateFrom, NextDueOn: recurringOptionalText(template.NextDueOn),
		ArchivedAt: recurringOptionalText(template.ArchivedAt), CreatedAt: template.CreatedAt, UpdatedAt: template.UpdatedAt,
		TagIDs: append([]int64{}, spec.TagIDs...), Postings: make([]recurringPostingRequest, 0, len(spec.Postings))}
	for _, posting := range spec.Postings {
		result.Postings = append(result.Postings, recurringPostingRequest{LineKey: posting.LineKey, AccountID: posting.AccountID,
			QuantityValue: posting.QuantityValue, QuantityScale: posting.QuantityScale, CommodityID: posting.CommodityID, Memo: posting.Memo})
	}
	return result
}

func recurringOptionalText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func listRecurringTemplates(logger *slog.Logger, auth *app.AuthService, service *app.RecurringService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		includeArchived, err := parseOptionalBool(r.URL.Query().Get("include_archived"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "include_archived is invalid")
			return
		}
		templates, err := service.ListTemplates(r.Context(), owner.ID, includeArchived)
		if err != nil {
			writeRecurringServiceError(w, r, logger, err)
			return
		}
		response := recurringTemplatesResponse{Templates: make([]recurringTemplateResponse, 0, len(templates))}
		for _, template := range templates {
			response.Templates = append(response.Templates, toRecurringTemplateResponse(template))
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func readRecurringTemplateID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("template_id"), 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "template id is invalid")
		return 0, false
	}
	return id, true
}

func readRecurringTemplate(logger *slog.Logger, auth *app.AuthService, service *app.RecurringService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		id, ok := readRecurringTemplateID(w, r)
		if !ok {
			return
		}
		template, err := service.Template(r.Context(), owner.ID, id)
		if err != nil {
			writeRecurringServiceError(w, r, logger, err)
			return
		}
		writeJSON(w, http.StatusOK, toRecurringTemplateResponse(template))
	}
}

func mutateRecurringTemplate(logger *slog.Logger, auth *app.AuthService, service *app.RecurringService, options HandlerOptions, action string) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, auth, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		input := app.WriteRecurringTemplateInput{OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context())}
		if action != "create" {
			id, ok := readRecurringTemplateID(w, r)
			if !ok {
				return
			}
			input.TemplateID = id
		}
		var request recurringTemplateRequest
		if action == "archive" {
			var empty struct{}
			if err := decodeJSONBody(r, &empty); err != nil {
				writeDecodeError(w, err)
				return
			}
		} else {
			if err := decodeJSONBody(r, &request); err != nil {
				writeDecodeError(w, err)
				return
			}
			input.Patch = request.patch()
		}
		var template app.RecurringTemplate
		var err error
		status := http.StatusOK
		switch action {
		case "create":
			template, err = service.CreateTemplate(r.Context(), input)
			status = http.StatusCreated
		case "update":
			template, err = service.UpdateTemplate(r.Context(), input)
		case "archive":
			template, err = service.ArchiveTemplate(r.Context(), input)
		}
		if err != nil {
			writeRecurringServiceError(w, r, logger, err)
			return
		}
		writeJSON(w, status, toRecurringTemplateResponse(template))
	}))
}

func writeRecurringServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, app.ErrRecurringOccurrenceMaterialized):
		writeAPIError(w, http.StatusConflict, "RECURRING_OCCURRENCE_ALREADY_MATERIALIZED", "occurrence already materialized or changed; reload before reviewing")
	case errors.Is(err, app.ErrRecurringTemplateUnbalanced):
		writeAPIError(w, http.StatusBadRequest, "RECURRING_TEMPLATE_UNBALANCED", "template must balance by commodity")
	case errors.Is(err, app.ErrRecurringScheduleInvalid):
		writeAPIError(w, http.StatusBadRequest, "RECURRING_SCHEDULE_INVALID", "recurring schedule is invalid")
	case errors.Is(err, app.ErrRecurringTemplateArchived):
		writeAPIError(w, http.StatusConflict, "RECURRING_TEMPLATE_ARCHIVED", "recurring template is archived")
	case errors.Is(err, app.ErrRecurringTemplateConflict):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "recurring template changed; reload and retry")
	case errors.Is(err, app.ErrRecurringTemplateNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "recurring template not found")
	default:
		writeTransactionServiceError(w, r, logger, "recurring template", err)
	}
}
