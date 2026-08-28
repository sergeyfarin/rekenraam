package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrImportBatchNotFound             = errors.New("import batch not found")
	ErrImportProfileNotFound           = errors.New("import profile not found")
	ErrImportStagedRowAlreadyCommitted = errors.New("import staged row is already committed")
)

type ImportRepository struct {
	database *sql.DB
}

func NewImportRepository(database *sql.DB) *ImportRepository {
	return &ImportRepository{database: database}
}

// --- Record types ---

type ImportBatchRecord struct {
	ID               int64
	BookID           int64
	SourceKind       string
	ProfileID        sql.NullInt64
	ConnectionID     sql.NullInt64
	Status           string
	OriginalFilename string
	SourceMetaJSON   string
	CreatedAt        string
}

type ImportBatchEventRecord struct {
	ID           int64
	BatchID      int64
	EventKind    string
	DetailJSON   string
	AuditEventID sql.NullInt64
	CreatedAt    string
}

type ImportStagedRowRecord struct {
	ID                     int64
	BatchID                int64
	BookID                 int64
	RowIndex               int
	DedupeFingerprint      string
	RawJSON                string
	NormalizedJSON         string
	DedupeStatus           string
	ResolutionJSON         string
	CommitStatus           string
	CommittedTransactionID sql.NullInt64
	CommitError            sql.NullString
}

type ImportCommitIdentityRecord struct {
	ID                     int64
	BookID                 int64
	DedupeFingerprint      string
	CommittedTransactionID int64
	SourceKind             string
	AccountID              int64
	CreatedAt              string
}

type ImportProfileRecord struct {
	ID          int64
	BookID      int64
	Name        string
	AdapterKind string
	ConfigJSON  string
	CreatedAt   string
	UpdatedAt   string
}

type CreateImportProfileParams struct {
	BookID        int64
	Name          string
	AdapterKind   string
	ConfigJSON    string
	CreatedAt     string
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
}

// --- Param types ---

type CreateImportBatchParams struct {
	BookID           int64
	SourceKind       string
	ProfileID        sql.NullInt64
	ConnectionID     sql.NullInt64
	OriginalFilename string
	SourceMetaJSON   string
	CreatedAt        string
	// Audit
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
}

type CreateImportStagedRowParams struct {
	BatchID           int64
	BookID            int64
	RowIndex          int
	DedupeFingerprint string
	RawJSON           string
	NormalizedJSON    string
	DedupeStatus      string
	ResolutionJSON    string
}

type UpdateImportBatchStatusParams struct {
	BatchID    int64
	Status     string
	EventKind  string
	DetailJSON string
	// Audit (optional — some events carry an audit_event_id)
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OccurredAt    string
}

type UpdateImportStagedRowResolutionParams struct {
	RowID          int64
	BatchID        int64
	DedupeStatus   string
	ResolutionJSON string
}

type CommitImportStagedRowParams struct {
	RowID                  int64
	CommitStatus           string
	CommittedTransactionID sql.NullInt64
	CommitError            sql.NullString
}

type CreateImportCommitIdentityParams struct {
	BookID                 int64
	DedupeFingerprint      string
	CommittedTransactionID int64
	SourceKind             string
	AccountID              int64
	CreatedAt              string
}

type CommitImportedTransactionParams struct {
	Identity CreateImportCommitIdentityParams
	Row      CommitImportStagedRowParams
}

type ListImportBatchesParams struct {
	BookID int64
	Limit  int
	// Cursor pagination: (created_at DESC, id DESC)
	CursorCreatedAt string
	CursorID        int64
}

type ListImportStagedRowsParams struct {
	BatchID int64
	Limit   int
	// Cursor: row_index ASC
	CursorRowIndex int
	CursorID       int64
}

// --- Batch operations ---

func (r *ImportRepository) ListImportProfiles(ctx context.Context, bookID int64) ([]ImportProfileRecord, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, book_id, name, adapter_kind, config_json, created_at, updated_at FROM import_profiles WHERE book_id = ? ORDER BY name, id`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list import profiles: %w", err)
	}
	defer rows.Close()
	var records []ImportProfileRecord
	for rows.Next() {
		var record ImportProfileRecord
		if err := rows.Scan(&record.ID, &record.BookID, &record.Name, &record.AdapterKind, &record.ConfigJSON, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan import profile: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (r *ImportRepository) ImportProfileByID(ctx context.Context, bookID, profileID int64) (ImportProfileRecord, error) {
	var record ImportProfileRecord
	err := r.database.QueryRowContext(ctx, `SELECT id, book_id, name, adapter_kind, config_json, created_at, updated_at FROM import_profiles WHERE book_id = ? AND id = ?`, bookID, profileID).Scan(&record.ID, &record.BookID, &record.Name, &record.AdapterKind, &record.ConfigJSON, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ImportProfileRecord{}, ErrImportProfileNotFound
	}
	if err != nil {
		return ImportProfileRecord{}, fmt.Errorf("read import profile: %w", err)
	}
	return record, nil
}

func (r *ImportRepository) CreateImportProfile(ctx context.Context, params CreateImportProfileParams) (ImportProfileRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return ImportProfileRecord{}, fmt.Errorf("begin create import profile: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return ImportProfileRecord{}, err
	}
	if _, err := insertAuditEvent(ctx, tx, AuditEventParams{BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID, OccurredAt: params.CreatedAt, RequestID: params.RequestID, OriginType: "browser_api", Operation: "import.profile.create", Reason: "import profile created"}); err != nil {
		return ImportProfileRecord{}, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO import_profiles (book_id, name, adapter_kind, config_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, params.BookID, params.Name, params.AdapterKind, params.ConfigJSON, params.CreatedAt, params.CreatedAt)
	if err != nil {
		return ImportProfileRecord{}, fmt.Errorf("insert import profile: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return ImportProfileRecord{}, fmt.Errorf("read import profile id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ImportProfileRecord{}, fmt.Errorf("commit create import profile: %w", err)
	}
	committed = true
	return ImportProfileRecord{ID: id, BookID: params.BookID, Name: params.Name, AdapterKind: params.AdapterKind, ConfigJSON: params.ConfigJSON, CreatedAt: params.CreatedAt, UpdatedAt: params.CreatedAt}, nil
}

func (r *ImportRepository) CreateImportBatch(ctx context.Context, params CreateImportBatchParams) (ImportBatchRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return ImportBatchRecord{}, fmt.Errorf("begin create import batch: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return ImportBatchRecord{}, err
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    "import",
		Operation:     "import.batch.create",
		Reason:        "import batch created",
	})
	if err != nil {
		return ImportBatchRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO import_batches (book_id, source_kind, profile_id, connection_id, status, original_filename, source_meta_json, created_at)
		VALUES (?, ?, ?, ?, 'previewing', ?, ?, ?)
	`, params.BookID, params.SourceKind, params.ProfileID, params.ConnectionID, params.OriginalFilename, params.SourceMetaJSON, params.CreatedAt)
	if err != nil {
		return ImportBatchRecord{}, fmt.Errorf("insert import batch: %w", err)
	}

	batchID, err := result.LastInsertId()
	if err != nil {
		return ImportBatchRecord{}, fmt.Errorf("read import batch id: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO import_batch_events (batch_id, event_kind, detail_json, audit_event_id, created_at)
		VALUES (?, 'created', '{}', ?, ?)
	`, batchID, auditEventID, params.CreatedAt); err != nil {
		return ImportBatchRecord{}, fmt.Errorf("insert import batch event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ImportBatchRecord{}, fmt.Errorf("commit create import batch: %w", err)
	}
	committed = true

	return ImportBatchRecord{
		ID:               batchID,
		BookID:           params.BookID,
		SourceKind:       params.SourceKind,
		ProfileID:        params.ProfileID,
		ConnectionID:     params.ConnectionID,
		Status:           "previewing",
		OriginalFilename: params.OriginalFilename,
		SourceMetaJSON:   params.SourceMetaJSON,
		CreatedAt:        params.CreatedAt,
	}, nil
}

func (r *ImportRepository) ImportBatchByID(ctx context.Context, bookID, batchID int64) (ImportBatchRecord, error) {
	var rec ImportBatchRecord
	err := r.database.QueryRowContext(ctx, `
		SELECT id, book_id, source_kind, profile_id, connection_id, status, original_filename, source_meta_json, created_at
		FROM import_batches
		WHERE id = ? AND book_id = ?
	`, batchID, bookID).Scan(
		&rec.ID, &rec.BookID, &rec.SourceKind, &rec.ProfileID, &rec.ConnectionID, &rec.Status,
		&rec.OriginalFilename, &rec.SourceMetaJSON, &rec.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ImportBatchRecord{}, ErrImportBatchNotFound
	}
	if err != nil {
		return ImportBatchRecord{}, fmt.Errorf("read import batch: %w", err)
	}
	return rec, nil
}

func (r *ImportRepository) ListImportBatches(ctx context.Context, params ListImportBatchesParams) ([]ImportBatchRecord, error) {
	limit := params.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var rows *sql.Rows
	var err error

	if params.CursorCreatedAt != "" && params.CursorID > 0 {
		rows, err = r.database.QueryContext(ctx, `
			SELECT id, book_id, source_kind, profile_id, connection_id, status, original_filename, source_meta_json, created_at
			FROM import_batches
			WHERE book_id = ? AND (created_at < ? OR (created_at = ? AND id < ?))
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		`, params.BookID, params.CursorCreatedAt, params.CursorCreatedAt, params.CursorID, limit)
	} else {
		rows, err = r.database.QueryContext(ctx, `
			SELECT id, book_id, source_kind, profile_id, connection_id, status, original_filename, source_meta_json, created_at
			FROM import_batches
			WHERE book_id = ?
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		`, params.BookID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list import batches: %w", err)
	}
	defer rows.Close()

	var records []ImportBatchRecord
	for rows.Next() {
		var rec ImportBatchRecord
		if err := rows.Scan(
			&rec.ID, &rec.BookID, &rec.SourceKind, &rec.ProfileID, &rec.ConnectionID, &rec.Status,
			&rec.OriginalFilename, &rec.SourceMetaJSON, &rec.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan import batch: %w", err)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (r *ImportRepository) UpdateImportBatchStatus(ctx context.Context, params UpdateImportBatchStatusParams) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update import batch status: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	result, err := tx.ExecContext(ctx, `
		UPDATE import_batches SET status = ? WHERE id = ?
	`, params.Status, params.BatchID)
	if err != nil {
		return fmt.Errorf("update import batch status: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrImportBatchNotFound
	}

	occurredAt := params.OccurredAt
	var auditEventID sql.NullInt64
	if params.ActorUserID > 0 && occurredAt != "" {
		eid, err := insertAuditEvent(ctx, tx, AuditEventParams{
			BookID:        0,
			ActorUserID:   params.ActorUserID,
			AuthSessionID: params.AuthSessionID,
			OccurredAt:    occurredAt,
			RequestID:     params.RequestID,
			OriginType:    "import",
			Operation:     "import.batch." + params.EventKind,
			Reason:        params.EventKind,
		})
		if err != nil {
			return err
		}
		auditEventID = sql.NullInt64{Int64: eid, Valid: true}
	}

	detailJSON := params.DetailJSON
	if detailJSON == "" {
		detailJSON = "{}"
	}
	eventCreatedAt := occurredAt
	if eventCreatedAt == "" {
		eventCreatedAt = params.OccurredAt
	}
	// Use a deterministic timestamp fallback by querying batch created_at
	if eventCreatedAt == "" {
		_ = r.database.QueryRowContext(ctx, `SELECT created_at FROM import_batches WHERE id = ?`, params.BatchID).Scan(&eventCreatedAt)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO import_batch_events (batch_id, event_kind, detail_json, audit_event_id, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, params.BatchID, params.EventKind, detailJSON, auditEventID, eventCreatedAt); err != nil {
		return fmt.Errorf("insert import batch event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update import batch status: %w", err)
	}
	committed = true
	return nil
}

// UpdateImportBatchSourceMeta overwrites source_meta_json. Used by the
// Trading 212 fetch worker to flip fetch_status (fetching → ready/failed) and
// record the parsed date/currency hints once a fetch completes — a transient
// metadata update, not a status transition, so it does not write an
// import_batch_events row.
func (r *ImportRepository) UpdateImportBatchSourceMeta(ctx context.Context, batchID int64, sourceMetaJSON string) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE import_batches SET source_meta_json = ? WHERE id = ?
	`, sourceMetaJSON, batchID)
	if err != nil {
		return fmt.Errorf("update import batch source meta: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrImportBatchNotFound
	}
	return nil
}

// HasInFlightFetch reports whether a connection already has a previewing
// batch whose source_meta.fetch_status is "fetching" — the double-fetch
// guard StartOnlineImportBatch relies on, since the background queue itself
// does not coalesce different connection/cursor payloads.
//
// queryRowContexter is satisfied by both *sql.DB and *sql.Tx, so the same
// query backs the tx-scoped check inside StartOnlineImportBatch (where it
// matters for correctness) without duplicating the SQL.
type queryRowContexter interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func hasInFlightFetch(ctx context.Context, q queryRowContexter, bookID, connectionID int64) (bool, error) {
	var exists int
	err := q.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM import_batches
			WHERE book_id = ? AND connection_id = ? AND status = 'previewing'
			  AND json_extract(source_meta_json, '$.fetch_status') = 'fetching'
		)
	`, bookID, connectionID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check in-flight fetch: %w", err)
	}
	return exists == 1, nil
}

// ErrImportFetchAlreadyInProgress is StartOnlineImportBatch's guard-tripped
// sentinel; the app layer maps it to ErrImportFetchInProgress.
var ErrImportFetchAlreadyInProgress = errors.New("import fetch already in progress for this connection")

// StartOnlineImportBatchParams groups the fields StartOnlineImportBatch needs
// to create a previewing online batch and enqueue its fetch work item.
type StartOnlineImportBatchParams struct {
	BookID         int64
	ConnectionID   int64
	SourceKind     string
	SourceMetaJSON string
	CreatedAt      string
	ActorUserID    int64
	AuthSessionID  int64
	RequestID      string
	WorkKind       string
}

// StartOnlineImportBatch atomically checks the in-flight guard, inserts the
// previewing batch (+ its creation audit trail), and enqueues the durable
// fetch work item — all in one transaction.
//
// Doing these as three separate calls (as the first cut of this method did)
// left two real gaps: if the enqueue failed after the batch insert
// committed, the batch was stranded permanently in fetch_status=fetching
// with nothing left to ever process it, and every later start/refresh for
// that connection would then hit the in-flight guard forever, with no
// automatic recovery. And because the guard check and the batch insert were
// separate statements, two concurrent callers could each see "no in-flight
// fetch" before either had inserted its batch, both passing the guard.
//
// One transaction fixes both: the database is opened with
// SetMaxOpenConns(1) (see sqlite.go), so a transaction here is a full mutual
// exclusion lock — a second caller's BeginTx blocks until this one commits
// or rolls back — and a failure at any step rolls back everything, so a
// half-finished attempt never leaves a batch behind.
//
// buildWorkPayload receives the newly assigned batch ID (only known after
// the INSERT) and returns the JSON payload to enqueue; this keeps the
// payload's shape an app-layer concern rather than something this package
// needs to know about.
func (r *ImportRepository) StartOnlineImportBatch(ctx context.Context, params StartOnlineImportBatchParams, buildWorkPayload func(batchID int64) (string, error)) (ImportBatchRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return ImportBatchRecord{}, fmt.Errorf("begin start online import batch: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	inFlight, err := hasInFlightFetch(ctx, tx, params.BookID, params.ConnectionID)
	if err != nil {
		return ImportBatchRecord{}, err
	}
	if inFlight {
		return ImportBatchRecord{}, ErrImportFetchAlreadyInProgress
	}

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return ImportBatchRecord{}, err
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    "import",
		Operation:     "import.batch.create",
		Reason:        "import batch created",
	})
	if err != nil {
		return ImportBatchRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO import_batches (book_id, source_kind, connection_id, status, original_filename, source_meta_json, created_at)
		VALUES (?, ?, ?, 'previewing', '', ?, ?)
	`, params.BookID, params.SourceKind, params.ConnectionID, params.SourceMetaJSON, params.CreatedAt)
	if err != nil {
		return ImportBatchRecord{}, fmt.Errorf("insert online import batch: %w", err)
	}
	batchID, err := result.LastInsertId()
	if err != nil {
		return ImportBatchRecord{}, fmt.Errorf("read online import batch id: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO import_batch_events (batch_id, event_kind, detail_json, audit_event_id, created_at)
		VALUES (?, 'created', '{}', ?, ?)
	`, batchID, auditEventID, params.CreatedAt); err != nil {
		return ImportBatchRecord{}, fmt.Errorf("insert online import batch event: %w", err)
	}

	payloadJSON, err := buildWorkPayload(batchID)
	if err != nil {
		return ImportBatchRecord{}, fmt.Errorf("build fetch work payload: %w", err)
	}

	// A plain INSERT, not EnqueueBackgroundWork's coalescing INSERT ... ON
	// CONFLICT: the payload always embeds this call's freshly minted
	// batchID, so it can never collide with any existing work item's
	// (book_id, kind, payload_json).
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO background_work_items (book_id, kind, payload_json, available_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, params.BookID, params.WorkKind, payloadJSON, params.CreatedAt, params.CreatedAt, params.CreatedAt); err != nil {
		return ImportBatchRecord{}, fmt.Errorf("enqueue online import fetch work: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ImportBatchRecord{}, fmt.Errorf("commit start online import batch: %w", err)
	}
	committed = true

	return ImportBatchRecord{
		ID:             batchID,
		BookID:         params.BookID,
		SourceKind:     params.SourceKind,
		ConnectionID:   sql.NullInt64{Int64: params.ConnectionID, Valid: true},
		Status:         "previewing",
		SourceMetaJSON: params.SourceMetaJSON,
		CreatedAt:      params.CreatedAt,
	}, nil
}

// --- Staged row operations ---

func (r *ImportRepository) InsertImportStagedRows(ctx context.Context, rows []CreateImportStagedRowParams) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin insert staged rows: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO import_staged_rows
			(batch_id, book_id, row_index, dedupe_fingerprint, raw_json, normalized_json, dedupe_status, resolution_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert staged row: %w", err)
	}
	defer stmt.Close()

	for _, row := range rows {
		dedupeStatus := row.DedupeStatus
		if dedupeStatus == "" {
			dedupeStatus = "new"
		}
		resolutionJSON := row.ResolutionJSON
		if resolutionJSON == "" {
			resolutionJSON = "{}"
		}
		if _, err := stmt.ExecContext(ctx,
			row.BatchID, row.BookID, row.RowIndex, row.DedupeFingerprint,
			row.RawJSON, row.NormalizedJSON, dedupeStatus, resolutionJSON,
		); err != nil {
			return fmt.Errorf("insert staged row %d: %w", row.RowIndex, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit insert staged rows: %w", err)
	}
	committed = true
	return nil
}

// ListImportStagedRows is the paginated API path; limit is clamped to 500 / default 200.
func (r *ImportRepository) ListImportStagedRows(ctx context.Context, params ListImportStagedRowsParams) ([]ImportStagedRowRecord, error) {
	limit := params.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	var dbRows *sql.Rows
	var err error

	if params.CursorRowIndex > 0 || params.CursorID > 0 {
		dbRows, err = r.database.QueryContext(ctx, `
			SELECT id, batch_id, book_id, row_index, dedupe_fingerprint,
			       raw_json, normalized_json, dedupe_status, resolution_json,
			       commit_status, committed_transaction_id, commit_error
			FROM import_staged_rows
			WHERE batch_id = ? AND (row_index > ? OR (row_index = ? AND id > ?))
			ORDER BY row_index ASC, id ASC
			LIMIT ?
		`, params.BatchID, params.CursorRowIndex, params.CursorRowIndex, params.CursorID, limit)
	} else {
		dbRows, err = r.database.QueryContext(ctx, `
			SELECT id, batch_id, book_id, row_index, dedupe_fingerprint,
			       raw_json, normalized_json, dedupe_status, resolution_json,
			       commit_status, committed_transaction_id, commit_error
			FROM import_staged_rows
			WHERE batch_id = ?
			ORDER BY row_index ASC, id ASC
			LIMIT ?
		`, params.BatchID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list staged rows: %w", err)
	}
	defer dbRows.Close()

	return scanImportStagedRows(dbRows)
}

// ListAllImportStagedRows returns every row for a batch with no limit.
// Only for internal service operations (commit, preview) where partial reads would corrupt state.
func (r *ImportRepository) ListAllImportStagedRows(ctx context.Context, batchID int64) ([]ImportStagedRowRecord, error) {
	dbRows, err := r.database.QueryContext(ctx, `
		SELECT id, batch_id, book_id, row_index, dedupe_fingerprint,
		       raw_json, normalized_json, dedupe_status, resolution_json,
		       commit_status, committed_transaction_id, commit_error
		FROM import_staged_rows
		WHERE batch_id = ?
		ORDER BY row_index ASC, id ASC
	`, batchID)
	if err != nil {
		return nil, fmt.Errorf("list all staged rows: %w", err)
	}
	defer dbRows.Close()

	return scanImportStagedRows(dbRows)
}

func scanImportStagedRows(dbRows *sql.Rows) ([]ImportStagedRowRecord, error) {
	var records []ImportStagedRowRecord
	for dbRows.Next() {
		rec, err := scanImportStagedRow(dbRows)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, dbRows.Err()
}

type importStagedRowScanner interface {
	Scan(dest ...any) error
}

func scanImportStagedRow(row importStagedRowScanner) (ImportStagedRowRecord, error) {
	var rec ImportStagedRowRecord
	if err := row.Scan(
		&rec.ID, &rec.BatchID, &rec.BookID, &rec.RowIndex, &rec.DedupeFingerprint,
		&rec.RawJSON, &rec.NormalizedJSON, &rec.DedupeStatus, &rec.ResolutionJSON,
		&rec.CommitStatus, &rec.CommittedTransactionID, &rec.CommitError,
	); err != nil {
		return ImportStagedRowRecord{}, fmt.Errorf("scan staged row: %w", err)
	}
	return rec, nil
}

// ImportStagedRowByID reloads one staged row for a stale-writer transition
// check. It deliberately distinguishes a missing row from a committed row.
func (r *ImportRepository) ImportStagedRowByID(ctx context.Context, rowID int64) (ImportStagedRowRecord, error) {
	rec, err := scanImportStagedRow(r.database.QueryRowContext(ctx, `
		SELECT id, batch_id, book_id, row_index, dedupe_fingerprint,
		       raw_json, normalized_json, dedupe_status, resolution_json,
		       commit_status, committed_transaction_id, commit_error
		FROM import_staged_rows
		WHERE id = ?
	`, rowID))
	if errors.Is(err, sql.ErrNoRows) {
		return ImportStagedRowRecord{}, ErrNotFound
	}
	if err != nil {
		return ImportStagedRowRecord{}, fmt.Errorf("read staged row: %w", err)
	}
	return rec, nil
}

func (r *ImportRepository) UpdateImportStagedRowResolution(ctx context.Context, params UpdateImportStagedRowResolutionParams) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE import_staged_rows
		SET dedupe_status = ?, resolution_json = ?
		WHERE id = ? AND batch_id = ?
	`, params.DedupeStatus, params.ResolutionJSON, params.RowID, params.BatchID)
	if err != nil {
		return fmt.Errorf("update staged row resolution: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *ImportRepository) CommitImportStagedRow(ctx context.Context, params CommitImportStagedRowParams) error {
	return commitImportStagedRowExec(ctx, r.database, params)
}

func (r *ImportRepository) CommitImportStagedRowInTx(ctx context.Context, tx *sql.Tx, params CommitImportStagedRowParams) error {
	return commitImportStagedRowExec(ctx, tx, params)
}

// MarkImportStagedRowCommittedIfPending records a successful idempotent
// outcome without overwriting a terminal result written by a concurrent
// commit. The identity and winning row marker are written atomically, so a
// no-op here means the other caller has already marked the row committed.
func (r *ImportRepository) MarkImportStagedRowCommittedIfPending(ctx context.Context, rowID, transactionID int64) (bool, error) {
	result, err := r.database.ExecContext(ctx, `
		UPDATE import_staged_rows
		SET commit_status = 'committed', committed_transaction_id = ?, commit_error = NULL
		WHERE id = ? AND commit_status = 'pending'
	`, transactionID, rowID)
	if err != nil {
		return false, fmt.Errorf("mark staged row committed if pending: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("committed-if-pending rows affected: %w", err)
	}
	return n == 1, nil
}

func (r *ImportRepository) CommitImportedTransaction(ctx context.Context, params CommitImportedTransactionParams, createTransaction func(*sql.Tx) (int64, error)) (int64, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin commit imported transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	transactionID, err := createTransaction(tx)
	if err != nil {
		return 0, err
	}

	identity := params.Identity
	identity.CommittedTransactionID = transactionID
	if err := r.CreateCommitIdentity(ctx, tx, identity); err != nil {
		return 0, fmt.Errorf("create commit identity: %w", err)
	}

	row := params.Row
	row.CommitStatus = "committed"
	row.CommittedTransactionID = sql.NullInt64{Int64: transactionID, Valid: true}
	if err := r.CommitImportStagedRowInTx(ctx, tx, row); err != nil {
		return 0, fmt.Errorf("mark row committed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit imported transaction: %w", err)
	}
	committed = true
	return transactionID, nil
}

type execContexter interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func commitImportStagedRowExec(ctx context.Context, db execContexter, params CommitImportStagedRowParams) error {
	result, err := db.ExecContext(ctx, `
		UPDATE import_staged_rows
		SET commit_status = ?, committed_transaction_id = ?, commit_error = ?
		WHERE id = ? AND commit_status <> 'committed'
	`, params.CommitStatus, params.CommittedTransactionID, params.CommitError, params.RowID)
	if err != nil {
		return fmt.Errorf("commit staged row: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		var commitStatus string
		err := db.QueryRowContext(ctx, `SELECT commit_status FROM import_staged_rows WHERE id = ?`, params.RowID).Scan(&commitStatus)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read staged row after terminal transition: %w", err)
		}
		if commitStatus == "committed" {
			return ErrImportStagedRowAlreadyCommitted
		}
		return fmt.Errorf("staged row %d terminal transition was not applied from status %q", params.RowID, commitStatus)
	}
	return nil
}

// --- Commit identity operations ---

func (r *ImportRepository) FindCommitIdentity(ctx context.Context, bookID int64, dedupeFingerprint string) (ImportCommitIdentityRecord, bool, error) {
	var rec ImportCommitIdentityRecord
	err := r.database.QueryRowContext(ctx, `
		SELECT id, book_id, dedupe_fingerprint, committed_transaction_id, source_kind, account_id, created_at
		FROM import_commit_identities
		WHERE book_id = ? AND dedupe_fingerprint = ?
	`, bookID, dedupeFingerprint).Scan(
		&rec.ID, &rec.BookID, &rec.DedupeFingerprint, &rec.CommittedTransactionID,
		&rec.SourceKind, &rec.AccountID, &rec.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ImportCommitIdentityRecord{}, false, nil
	}
	if err != nil {
		return ImportCommitIdentityRecord{}, false, fmt.Errorf("find commit identity: %w", err)
	}
	return rec, true, nil
}

var ErrCommitIdentityConflict = errors.New("commit identity already exists for a different transaction")

func (r *ImportRepository) CreateCommitIdentity(ctx context.Context, tx *sql.Tx, params CreateImportCommitIdentityParams) error {
	result, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO import_commit_identities
			(book_id, dedupe_fingerprint, committed_transaction_id, source_kind, account_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, params.BookID, params.DedupeFingerprint, params.CommittedTransactionID,
		params.SourceKind, params.AccountID, params.CreatedAt)
	if err != nil {
		return fmt.Errorf("create commit identity: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("create commit identity rows affected: %w", err)
	}
	if n == 0 {
		// Row already existed (race or retry). Verify it points to the same transaction.
		var existingTxnID int64
		if err := tx.QueryRowContext(ctx, `
			SELECT committed_transaction_id FROM import_commit_identities
			WHERE book_id = ? AND dedupe_fingerprint = ?
		`, params.BookID, params.DedupeFingerprint).Scan(&existingTxnID); err != nil {
			return fmt.Errorf("verify commit identity: %w", err)
		}
		if existingTxnID != params.CommittedTransactionID {
			return ErrCommitIdentityConflict
		}
	}
	return nil
}

// CurrentBookOwnerID reads the owning user for a book, used by the scheduled
// auto-refresh worker to attribute a system-triggered fetch to a real user
// (mirrors PricingRepository.CurrentBookOwnerID).
func (r *ImportRepository) CurrentBookOwnerID(ctx context.Context, bookID int64) (int64, error) {
	var ownerID int64
	if err := r.database.QueryRowContext(ctx, `SELECT owner_user_id FROM books WHERE id = ?`, bookID).Scan(&ownerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("read book owner: %w", err)
	}
	return ownerID, nil
}

// CountStagedRowsByCommitStatus returns counts by commit_status for a batch.
func (r *ImportRepository) CountStagedRowsByCommitStatus(ctx context.Context, batchID int64) (map[string]int, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT commit_status, COUNT(*) FROM import_staged_rows WHERE batch_id = ? GROUP BY commit_status
	`, batchID)
	if err != nil {
		return nil, fmt.Errorf("count staged rows: %w", err)
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan count: %w", err)
		}
		counts[status] = count
	}
	return counts, rows.Err()
}
