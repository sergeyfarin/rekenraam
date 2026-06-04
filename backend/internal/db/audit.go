package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var auditOperationPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

var auditOriginTypes = map[string]bool{
	"browser_api":  true,
	"setup":        true,
	"cli_recovery": true,
	"import":       true,
	"system_seed":  true,
	"scheduled":    true,
	"internal":     true,
}

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
	if !auditOriginTypes[originType] {
		return 0, fmt.Errorf("audit origin type is invalid: %s", originType)
	}

	operation := strings.TrimSpace(params.Operation)
	if !auditOperationPattern.MatchString(operation) {
		return 0, fmt.Errorf("audit operation is invalid: %s", operation)
	}

	occurredAt := strings.TrimSpace(params.OccurredAt)
	parsedOccurredAt, err := time.Parse(time.RFC3339, occurredAt)
	if err != nil || parsedOccurredAt.Location() != time.UTC || parsedOccurredAt.Format(time.RFC3339) != occurredAt {
		return 0, fmt.Errorf("audit occurred_at must be UTC RFC3339 without fractional seconds")
	}

	metadataJSON := strings.TrimSpace(params.MetadataJSON)
	if metadataJSON == "" {
		metadataJSON = "{}"
	}
	if !json.Valid([]byte(metadataJSON)) || metadataJSON[0] != '{' {
		return 0, fmt.Errorf("audit metadata_json must be a JSON object")
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
	`, nullablePositiveInt64(params.BookID), nullablePositiveInt64(params.ActorUserID), nullablePositiveInt64(params.AuthSessionID), occurredAt, strings.TrimSpace(params.RequestID), originType, operation, strings.TrimSpace(params.Reason), metadataJSON)
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
