package app

import (
	"context"
	"strconv"

	"rekenraam/backend/internal/db"
)

type AuditEvent struct {
	ID            int64
	BookID        int64
	ActorUserID   int64
	AuthSessionID int64
	OccurredAt    string
	RequestID     string
	OriginType    string
	Operation     string
	Reason        string
	MetadataJSON  string
	Targets       []AuditTarget
	Changes       []AuditChange
}

type AuditTarget struct {
	BookID     int64
	TargetType string
	TargetID   int64
	TargetRole string
}

type AuditChange struct {
	ID                int64
	BookID            int64
	TableName         string
	RowID             int64
	Operation         string
	BeforeJSON        string
	AfterJSON         string
	ChangedFieldsJSON string
}

type ListAuditEventsInput struct {
	TargetType  string
	TargetID    int64
	TableName   string
	Operation   string
	ActorUserID int64
	After       string
	Before      string
	Limit       int
	Cursor      string
}

type ListAuditEventsResult struct {
	Events     []AuditEvent
	NextCursor string
}

type AuditService struct {
	repository *db.AuditRepository
}

func NewAuditService(repository *db.AuditRepository) *AuditService {
	return &AuditService{repository: repository}
}

func (s *AuditService) ListAuditEvents(ctx context.Context, input ListAuditEventsInput) (ListAuditEventsResult, error) {
	cursor := int64(0)
	if input.Cursor != "" {
		parsed, err := strconv.ParseInt(input.Cursor, 10, 64)
		if err != nil || parsed <= 0 {
			return ListAuditEventsResult{}, ValidationError{Message: "cursor is invalid"}
		}
		cursor = parsed
	}
	result, err := s.repository.ListAuditEvents(ctx, db.AuditEventFilter{
		TargetType:  input.TargetType,
		TargetID:    input.TargetID,
		TableName:   input.TableName,
		Operation:   input.Operation,
		ActorUserID: input.ActorUserID,
		After:       input.After,
		Before:      input.Before,
		Limit:       input.Limit,
		Cursor:      cursor,
	})
	if err != nil {
		return ListAuditEventsResult{}, err
	}

	events := make([]AuditEvent, 0, len(result.Events))
	for _, event := range result.Events {
		targets := make([]AuditTarget, 0, len(event.Targets))
		for _, target := range event.Targets {
			targets = append(targets, AuditTarget{
				BookID:     target.BookID,
				TargetType: target.TargetType,
				TargetID:   target.TargetID,
				TargetRole: target.TargetRole,
			})
		}
		changes := make([]AuditChange, 0, len(event.Changes))
		for _, change := range event.Changes {
			changes = append(changes, AuditChange{
				ID:                change.ID,
				BookID:            change.BookID,
				TableName:         change.TableName,
				RowID:             change.RowID,
				Operation:         change.Operation,
				BeforeJSON:        change.BeforeJSON,
				AfterJSON:         change.AfterJSON,
				ChangedFieldsJSON: change.ChangedFieldsJSON,
			})
		}
		events = append(events, AuditEvent{
			ID:            event.ID,
			BookID:        event.BookID,
			ActorUserID:   event.ActorUserID,
			AuthSessionID: event.AuthSessionID,
			OccurredAt:    event.OccurredAt,
			RequestID:     event.RequestID,
			OriginType:    event.OriginType,
			Operation:     event.Operation,
			Reason:        event.Reason,
			MetadataJSON:  event.MetadataJSON,
			Targets:       targets,
			Changes:       changes,
		})
	}

	nextCursor := ""
	if result.NextCursor > 0 {
		nextCursor = strconv.FormatInt(result.NextCursor, 10)
	}
	return ListAuditEventsResult{Events: events, NextCursor: nextCursor}, nil
}
