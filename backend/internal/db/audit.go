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

type AuditTargetParams struct {
	BookID     int64
	TargetType string
	TargetID   int64
	TargetRole string
}

type AuditChangeParams struct {
	BookID            int64
	TableName         string
	RowID             int64
	Operation         string
	BeforeJSON        string
	AfterJSON         string
	ChangedFieldsJSON string
}

type AuditEventFilter struct {
	TargetType  string
	TargetID    int64
	TableName   string
	Operation   string
	ActorUserID int64
	After       string
	Before      string
	Limit       int
	Cursor      int64
}

type AuditEventRecord struct {
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
	Targets       []AuditTargetRecord
	Changes       []AuditChangeRecord
}

type AuditTargetRecord struct {
	AuditEventID int64
	BookID       int64
	TargetType   string
	TargetID     int64
	TargetRole   string
}

type AuditChangeRecord struct {
	ID                int64
	AuditEventID      int64
	BookID            int64
	TableName         string
	RowID             int64
	Operation         string
	BeforeJSON        string
	AfterJSON         string
	ChangedFieldsJSON string
}

type AuditEventListResult struct {
	Events     []AuditEventRecord
	NextCursor int64
}

type AuditRepository struct {
	database *sql.DB
}

func NewAuditRepository(database *sql.DB) *AuditRepository {
	return &AuditRepository{database: database}
}

func (r *AuditRepository) ListAuditEvents(ctx context.Context, filter AuditEventFilter) (AuditEventListResult, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	args := []any{}
	where := []string{"1 = 1"}
	if strings.TrimSpace(filter.TargetType) != "" {
		where = append(where, `EXISTS (
			SELECT 1
			FROM audit_event_targets target
			WHERE target.audit_event_id = audit_events.id
				AND target.target_type = ?
				AND (? <= 0 OR target.target_id = ?)
		)`)
		args = append(args, strings.TrimSpace(filter.TargetType), filter.TargetID, filter.TargetID)
	} else if filter.TargetID > 0 {
		where = append(where, `EXISTS (
			SELECT 1
			FROM audit_event_targets target
			WHERE target.audit_event_id = audit_events.id
				AND target.target_id = ?
		)`)
		args = append(args, filter.TargetID)
	}
	if strings.TrimSpace(filter.TableName) != "" {
		where = append(where, `EXISTS (
			SELECT 1
			FROM audit_event_changes change
			WHERE change.audit_event_id = audit_events.id
				AND change.table_name = ?
		)`)
		args = append(args, strings.TrimSpace(filter.TableName))
	}
	if strings.TrimSpace(filter.Operation) != "" {
		where = append(where, `audit_events.operation = ?`)
		args = append(args, strings.TrimSpace(filter.Operation))
	}
	if filter.ActorUserID > 0 {
		where = append(where, `audit_events.actor_user_id = ?`)
		args = append(args, filter.ActorUserID)
	}
	if strings.TrimSpace(filter.After) != "" {
		where = append(where, `audit_events.occurred_at > ?`)
		args = append(args, strings.TrimSpace(filter.After))
	}
	if strings.TrimSpace(filter.Before) != "" {
		where = append(where, `audit_events.occurred_at < ?`)
		args = append(args, strings.TrimSpace(filter.Before))
	}
	if filter.Cursor > 0 {
		where = append(where, `audit_events.id < ?`)
		args = append(args, filter.Cursor)
	}

	args = append(args, limit+1)
	rows, err := r.database.QueryContext(ctx, `
		SELECT
			id,
			COALESCE(book_id, 0),
			COALESCE(actor_user_id, 0),
			COALESCE(auth_session_id, 0),
			occurred_at,
			COALESCE(request_id, ''),
			origin_type,
			operation,
			reason,
			metadata_json
		FROM audit_events
		WHERE `+strings.Join(where, "\n\t\t\tAND ")+`
		ORDER BY occurred_at DESC, id DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return AuditEventListResult{}, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	events := make([]AuditEventRecord, 0, limit)
	for rows.Next() {
		var event AuditEventRecord
		if err := rows.Scan(
			&event.ID,
			&event.BookID,
			&event.ActorUserID,
			&event.AuthSessionID,
			&event.OccurredAt,
			&event.RequestID,
			&event.OriginType,
			&event.Operation,
			&event.Reason,
			&event.MetadataJSON,
		); err != nil {
			return AuditEventListResult{}, fmt.Errorf("scan audit event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return AuditEventListResult{}, fmt.Errorf("iterate audit events: %w", err)
	}

	nextCursor := int64(0)
	if len(events) > limit {
		nextCursor = events[limit-1].ID
		events = events[:limit]
	}
	for index := range events {
		targets, err := r.auditTargets(ctx, events[index].ID)
		if err != nil {
			return AuditEventListResult{}, err
		}
		changes, err := r.auditChanges(ctx, events[index].ID)
		if err != nil {
			return AuditEventListResult{}, err
		}
		events[index].Targets = targets
		events[index].Changes = changes
	}

	return AuditEventListResult{Events: events, NextCursor: nextCursor}, nil
}

func (r *AuditRepository) auditTargets(ctx context.Context, auditEventID int64) ([]AuditTargetRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT audit_event_id, COALESCE(book_id, 0), target_type, target_id, target_role
		FROM audit_event_targets
		WHERE audit_event_id = ?
		ORDER BY target_type, target_id, target_role
	`, auditEventID)
	if err != nil {
		return nil, fmt.Errorf("list audit targets: %w", err)
	}
	defer rows.Close()

	targets := []AuditTargetRecord{}
	for rows.Next() {
		var target AuditTargetRecord
		if err := rows.Scan(&target.AuditEventID, &target.BookID, &target.TargetType, &target.TargetID, &target.TargetRole); err != nil {
			return nil, fmt.Errorf("scan audit target: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit targets: %w", err)
	}
	return targets, nil
}

func (r *AuditRepository) auditChanges(ctx context.Context, auditEventID int64) ([]AuditChangeRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT
			id,
			audit_event_id,
			COALESCE(book_id, 0),
			table_name,
			row_id,
			operation,
			COALESCE(before_json, ''),
			COALESCE(after_json, ''),
			changed_fields_json
		FROM audit_event_changes
		WHERE audit_event_id = ?
		ORDER BY id
	`, auditEventID)
	if err != nil {
		return nil, fmt.Errorf("list audit changes: %w", err)
	}
	defer rows.Close()

	changes := []AuditChangeRecord{}
	for rows.Next() {
		var change AuditChangeRecord
		if err := rows.Scan(&change.ID, &change.AuditEventID, &change.BookID, &change.TableName, &change.RowID, &change.Operation, &change.BeforeJSON, &change.AfterJSON, &change.ChangedFieldsJSON); err != nil {
			return nil, fmt.Errorf("scan audit change: %w", err)
		}
		changes = append(changes, change)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit changes: %w", err)
	}
	return changes, nil
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

func insertAuditTarget(ctx context.Context, tx *sql.Tx, auditEventID int64, params AuditTargetParams) error {
	targetType := strings.TrimSpace(params.TargetType)
	if targetType == "" {
		return fmt.Errorf("audit target type is required")
	}
	if params.TargetID <= 0 {
		return fmt.Errorf("audit target id is required")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO audit_event_targets (
			audit_event_id,
			book_id,
			target_type,
			target_id,
			target_role
		)
		VALUES (?, ?, ?, ?, ?)
	`, auditEventID, nullablePositiveInt64(params.BookID), targetType, params.TargetID, strings.TrimSpace(params.TargetRole)); err != nil {
		return fmt.Errorf("insert audit target: %w", err)
	}
	return nil
}

func insertAuditChange(ctx context.Context, tx *sql.Tx, auditEventID int64, params AuditChangeParams) error {
	tableName := strings.TrimSpace(params.TableName)
	if tableName == "" {
		return fmt.Errorf("audit change table name is required")
	}
	if params.RowID <= 0 {
		return fmt.Errorf("audit change row id is required")
	}
	operation := strings.TrimSpace(params.Operation)
	if operation != "insert" && operation != "update" && operation != "delete" {
		return fmt.Errorf("audit change operation is invalid: %s", operation)
	}
	beforeJSON := nullableAuditJSON(params.BeforeJSON)
	afterJSON := nullableAuditJSON(params.AfterJSON)
	changedFieldsJSON := strings.TrimSpace(params.ChangedFieldsJSON)
	if changedFieldsJSON == "" {
		changedFieldsJSON = "[]"
	}
	if !json.Valid([]byte(changedFieldsJSON)) || changedFieldsJSON[0] != '[' {
		return fmt.Errorf("audit changed_fields_json must be a JSON array")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_event_changes (
			audit_event_id,
			book_id,
			table_name,
			row_id,
			operation,
			before_json,
			after_json,
			changed_fields_json
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, auditEventID, nullablePositiveInt64(params.BookID), tableName, params.RowID, operation, beforeJSON, afterJSON, changedFieldsJSON); err != nil {
		return fmt.Errorf("insert audit change: %w", err)
	}
	return nil
}

func nullableAuditJSON(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullablePositiveInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}
