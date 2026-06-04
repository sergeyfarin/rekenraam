package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type AuditEventParams struct {
	BookID        int64
	ActorUserID   int64
	AuthSessionID int64
	OccurredAt    string
	RequestID     string
	OriginType    string
	Operation     string
	Reason        string
	MetadataJSON  string
}

func insertAuditEvent(ctx context.Context, tx *sql.Tx, params AuditEventParams) (int64, error) {
	originType := strings.TrimSpace(params.OriginType)
	if originType == "" {
		originType = "internal"
	}
	metadataJSON := strings.TrimSpace(params.MetadataJSON)
	if metadataJSON == "" {
		metadataJSON = "{}"
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO audit_events (
			book_id,
			actor_user_id,
			auth_session_id,
			occurred_at,
			request_id,
			origin_type,
			operation,
			reason,
			metadata_json
		)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?)
	`, nullablePositiveInt64(params.BookID), nullablePositiveInt64(params.ActorUserID), nullablePositiveInt64(params.AuthSessionID), params.OccurredAt, strings.TrimSpace(params.RequestID), originType, params.Operation, strings.TrimSpace(params.Reason), metadataJSON)
	if err != nil {
		return 0, fmt.Errorf("insert audit event: %w", err)
	}

	auditEventID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read audit event id: %w", err)
	}

	return auditEventID, nil
}

func nullablePositiveInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}
