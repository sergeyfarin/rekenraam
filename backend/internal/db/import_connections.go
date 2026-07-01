package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrImportConnectionNotFound = errors.New("import connection not found")

// ImportConnectionRecord is the full DB row, including the sealed secret.
// The secret is never surfaced to callers outside the db package directly —
// the app layer opens it in-memory only when needed for a fetch.
type ImportConnectionRecord struct {
	ID                 int64
	BookID             int64
	SourceKind         string
	DisplayName        string
	SecretCiphertext   string
	ConfigJSON         string
	FetchCursor        string
	LastFetchStatus    string
	LastFetchedAt      sql.NullString
	AutoRefreshEnabled bool
	CreatedAt          string
	UpdatedAt          string
}

type ImportConnectionRepository struct {
	database *sql.DB
}

func NewImportConnectionRepository(database *sql.DB) *ImportConnectionRepository {
	return &ImportConnectionRepository{database: database}
}

// --- Params ---

type CreateImportConnectionParams struct {
	BookID           int64
	SourceKind       string
	DisplayName      string
	SecretCiphertext string
	ConfigJSON       string
	CreatedAt        string
}

type UpdateImportConnectionParams struct {
	ID                 int64
	BookID             int64
	DisplayName        string
	SecretCiphertext   string // empty = do not rotate; the caller sets it to the sealed new key or keeps the old
	ConfigJSON         string
	AutoRefreshEnabled bool
	UpdatedAt          string
}

type UpdateImportConnectionCursorParams struct {
	ID              int64
	BookID          int64
	FetchCursor     string
	LastFetchStatus string
	LastFetchedAt   string
	UpdatedAt       string
}

// --- CRUD ---

func (r *ImportConnectionRepository) CreateImportConnection(ctx context.Context, p CreateImportConnectionParams) (ImportConnectionRecord, error) {
	result, err := r.database.ExecContext(ctx, `
		INSERT INTO import_connections (
			book_id, source_kind, display_name, secret_ciphertext,
			config_json, fetch_cursor, last_fetch_status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, '', '', ?, ?)
	`, p.BookID, p.SourceKind, p.DisplayName, p.SecretCiphertext,
		p.ConfigJSON, p.CreatedAt, p.CreatedAt)
	if err != nil {
		return ImportConnectionRecord{}, fmt.Errorf("create import connection: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return ImportConnectionRecord{}, fmt.Errorf("read import connection id: %w", err)
	}
	return r.ImportConnectionByID(ctx, id, p.BookID)
}

// importConnectionSelectColumns is shared by every SELECT against
// import_connections so scanImportConnectionRow/scanImportConnectionRows
// stay in sync with the query column order.
const importConnectionSelectColumns = `
	id, book_id, source_kind, display_name, secret_ciphertext,
	config_json, fetch_cursor, last_fetch_status, last_fetched_at, auto_refresh_enabled, created_at, updated_at
`

func scanImportConnectionRow(scanner rowScanner) (ImportConnectionRecord, error) {
	var rec ImportConnectionRecord
	var autoRefreshEnabled int
	if err := scanner.Scan(
		&rec.ID, &rec.BookID, &rec.SourceKind, &rec.DisplayName, &rec.SecretCiphertext,
		&rec.ConfigJSON, &rec.FetchCursor, &rec.LastFetchStatus, &rec.LastFetchedAt, &autoRefreshEnabled,
		&rec.CreatedAt, &rec.UpdatedAt,
	); err != nil {
		return ImportConnectionRecord{}, err
	}
	rec.AutoRefreshEnabled = autoRefreshEnabled == 1
	return rec, nil
}

func (r *ImportConnectionRepository) ImportConnectionByID(ctx context.Context, id int64, bookID int64) (ImportConnectionRecord, error) {
	rec, err := scanImportConnectionRow(r.database.QueryRowContext(ctx, `
		SELECT `+importConnectionSelectColumns+`
		FROM import_connections WHERE id = ? AND book_id = ?
	`, id, bookID))
	if errors.Is(err, sql.ErrNoRows) {
		return ImportConnectionRecord{}, ErrImportConnectionNotFound
	}
	if err != nil {
		return ImportConnectionRecord{}, fmt.Errorf("read import connection: %w", err)
	}
	return rec, nil
}

func (r *ImportConnectionRepository) ListImportConnections(ctx context.Context, bookID int64) ([]ImportConnectionRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT `+importConnectionSelectColumns+`
		FROM import_connections
		WHERE book_id = ?
		ORDER BY created_at ASC, id ASC
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list import connections: %w", err)
	}
	defer rows.Close()

	var result []ImportConnectionRecord
	for rows.Next() {
		rec, err := scanImportConnectionRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan import connection: %w", err)
		}
		result = append(result, rec)
	}
	return result, rows.Err()
}

// ListDueAutoRefreshConnections returns connections with auto-refresh enabled
// whose last fetch attempt (success or terminal failure) was before cutoff —
// or that have never been fetched at all. Scoped to sourceKind because the
// scheduler only drives Trading 212 today; a future provider adds its own
// kind here rather than assuming every online source wants the same cadence.
func (r *ImportConnectionRepository) ListDueAutoRefreshConnections(ctx context.Context, bookID int64, sourceKind string, cutoff string) ([]ImportConnectionRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT `+importConnectionSelectColumns+`
		FROM import_connections
		WHERE book_id = ? AND source_kind = ? AND auto_refresh_enabled = 1
		  AND (last_fetched_at IS NULL OR last_fetched_at <= ?)
		ORDER BY id ASC
	`, bookID, sourceKind, cutoff)
	if err != nil {
		return nil, fmt.Errorf("list due auto-refresh connections: %w", err)
	}
	defer rows.Close()

	var result []ImportConnectionRecord
	for rows.Next() {
		rec, err := scanImportConnectionRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan due auto-refresh connection: %w", err)
		}
		result = append(result, rec)
	}
	return result, rows.Err()
}

// UpdateImportConnection patches display_name, config_json, and optionally
// rotates the secret (when SecretCiphertext is non-empty).
func (r *ImportConnectionRepository) UpdateImportConnection(ctx context.Context, p UpdateImportConnectionParams) (ImportConnectionRecord, error) {
	var result sql.Result
	var err error
	autoRefreshEnabled := boolToInt(p.AutoRefreshEnabled)
	if p.SecretCiphertext != "" {
		result, err = r.database.ExecContext(ctx, `
			UPDATE import_connections
			SET display_name = ?, secret_ciphertext = ?, config_json = ?, auto_refresh_enabled = ?, updated_at = ?
			WHERE id = ? AND book_id = ?
		`, p.DisplayName, p.SecretCiphertext, p.ConfigJSON, autoRefreshEnabled, p.UpdatedAt, p.ID, p.BookID)
	} else {
		result, err = r.database.ExecContext(ctx, `
			UPDATE import_connections
			SET display_name = ?, config_json = ?, auto_refresh_enabled = ?, updated_at = ?
			WHERE id = ? AND book_id = ?
		`, p.DisplayName, p.ConfigJSON, autoRefreshEnabled, p.UpdatedAt, p.ID, p.BookID)
	}
	if err != nil {
		return ImportConnectionRecord{}, fmt.Errorf("update import connection: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return ImportConnectionRecord{}, fmt.Errorf("update import connection rows: %w", err)
	}
	if rows != 1 {
		return ImportConnectionRecord{}, ErrImportConnectionNotFound
	}
	return r.ImportConnectionByID(ctx, p.ID, p.BookID)
}

// UpdateImportConnectionCursor saves the incremental fetch cursor and status
// after a successful fetch. Called by the fetch worker, not the HTTP handler.
func (r *ImportConnectionRepository) UpdateImportConnectionCursor(ctx context.Context, p UpdateImportConnectionCursorParams) error {
	result, err := r.database.ExecContext(ctx, `
		UPDATE import_connections
		SET fetch_cursor = ?, last_fetch_status = ?, last_fetched_at = ?, updated_at = ?
		WHERE id = ? AND book_id = ?
	`, p.FetchCursor, p.LastFetchStatus, p.LastFetchedAt, p.UpdatedAt, p.ID, p.BookID)
	if err != nil {
		return fmt.Errorf("update import connection cursor: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update import connection cursor rows: %w", err)
	}
	if rows != 1 {
		return ErrImportConnectionNotFound
	}
	return nil
}

func (r *ImportConnectionRepository) DeleteImportConnection(ctx context.Context, id int64, bookID int64) error {
	result, err := r.database.ExecContext(ctx, `
		DELETE FROM import_connections WHERE id = ? AND book_id = ?
	`, id, bookID)
	if err != nil {
		return fmt.Errorf("delete import connection: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete import connection rows: %w", err)
	}
	if rows != 1 {
		return ErrImportConnectionNotFound
	}
	return nil
}
