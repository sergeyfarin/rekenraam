package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

type auditEventsResponse struct {
	Events     []auditEventResponse `json:"events"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

type auditEventResponse struct {
	ID            int64                 `json:"id"`
	BookID        int64                 `json:"book_id,omitempty"`
	ActorUserID   int64                 `json:"actor_user_id,omitempty"`
	AuthSessionID int64                 `json:"auth_session_id,omitempty"`
	OccurredAt    string                `json:"occurred_at"`
	RequestID     string                `json:"request_id,omitempty"`
	OriginType    string                `json:"origin_type"`
	Operation     string                `json:"operation"`
	Reason        string                `json:"reason"`
	Metadata      json.RawMessage       `json:"metadata"`
	Targets       []auditTargetResponse `json:"targets"`
	Changes       []auditChangeResponse `json:"changes"`
}

type auditTargetResponse struct {
	BookID     int64  `json:"book_id,omitempty"`
	TargetType string `json:"target_type"`
	TargetID   int64  `json:"target_id"`
	TargetRole string `json:"target_role"`
}

type auditChangeResponse struct {
	ID            int64           `json:"id"`
	BookID        int64           `json:"book_id,omitempty"`
	TableName     string          `json:"table_name"`
	RowID         int64           `json:"row_id"`
	Operation     string          `json:"operation"`
	Before        json.RawMessage `json:"before,omitempty"`
	After         json.RawMessage `json:"after,omitempty"`
	ChangedFields json.RawMessage `json:"changed_fields"`
}

func listAuditEvents(logger *slog.Logger, authService *app.AuthService, auditService *app.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		input, ok := readAuditEventsInput(w, r)
		if !ok {
			return
		}
		result, err := auditService.ListAuditEvents(r.Context(), input)
		if err != nil {
			writeAuditServiceError(w, r, logger, err)
			return
		}
		writeJSON(w, http.StatusOK, auditEventsResponse{
			Events:     toAuditEventResponses(result.Events),
			NextCursor: result.NextCursor,
		})
	}
}

func readAuditEventsInput(w http.ResponseWriter, r *http.Request) (app.ListAuditEventsInput, bool) {
	query := r.URL.Query()
	targetID, ok := parseOptionalPositiveInt64(w, query.Get("target_id"), "target id")
	if !ok {
		return app.ListAuditEventsInput{}, false
	}
	actorUserID, ok := parseOptionalPositiveInt64(w, query.Get("actor_user_id"), "actor user id")
	if !ok {
		return app.ListAuditEventsInput{}, false
	}
	limit := 0
	if query.Get("limit") != "" {
		parsed, err := strconv.Atoi(query.Get("limit"))
		if err != nil || parsed <= 0 {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "limit is invalid")
			return app.ListAuditEventsInput{}, false
		}
		limit = parsed
	}
	return app.ListAuditEventsInput{
		TargetType:  query.Get("target_type"),
		TargetID:    targetID,
		TableName:   query.Get("table_name"),
		Operation:   query.Get("operation"),
		ActorUserID: actorUserID,
		After:       query.Get("after"),
		Before:      query.Get("before"),
		Limit:       limit,
		Cursor:      query.Get("cursor"),
	}, true
}

func writeAuditServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var validationError app.ValidationError
	if errors.As(err, &validationError) {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
		return
	}
	logger.ErrorContext(r.Context(), "list audit events", slog.Any("err", err))
	writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func toAuditEventResponses(events []app.AuditEvent) []auditEventResponse {
	responses := make([]auditEventResponse, 0, len(events))
	for _, event := range events {
		targets := make([]auditTargetResponse, 0, len(event.Targets))
		for _, target := range event.Targets {
			targets = append(targets, auditTargetResponse{
				BookID:     target.BookID,
				TargetType: target.TargetType,
				TargetID:   target.TargetID,
				TargetRole: target.TargetRole,
			})
		}
		changes := make([]auditChangeResponse, 0, len(event.Changes))
		for _, change := range event.Changes {
			changes = append(changes, auditChangeResponse{
				ID:            change.ID,
				BookID:        change.BookID,
				TableName:     change.TableName,
				RowID:         change.RowID,
				Operation:     change.Operation,
				Before:        rawAuditJSON(change.BeforeJSON),
				After:         rawAuditJSON(change.AfterJSON),
				ChangedFields: rawAuditJSON(change.ChangedFieldsJSON),
			})
		}
		responses = append(responses, auditEventResponse{
			ID:            event.ID,
			BookID:        event.BookID,
			ActorUserID:   event.ActorUserID,
			AuthSessionID: event.AuthSessionID,
			OccurredAt:    event.OccurredAt,
			RequestID:     event.RequestID,
			OriginType:    event.OriginType,
			Operation:     event.Operation,
			Reason:        event.Reason,
			Metadata:      rawAuditJSON(event.MetadataJSON),
			Targets:       targets,
			Changes:       changes,
		})
	}
	return responses
}

func rawAuditJSON(value string) json.RawMessage {
	if value == "" {
		return nil
	}
	return json.RawMessage(value)
}
