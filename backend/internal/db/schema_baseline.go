package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// BaselineIdentity is the unused v0.1 schema generation, independent of Goose's
// forward migration sequence. Old development databases also used version 1.
// Future forward migrations retain this value; it changes only at an explicitly
// declared, incompatible pre-release rewrite, never for an installed release.
const BaselineIdentity = 20261122

// ErrIncompatibleBaseline refuses an incompatible development file or backup.
var ErrIncompatibleBaseline = errors.New("incompatible pre-release baseline: export with the original build, then reset only the disposable development database; see docs/developer-workflow.md")

func requireCompatibleBaseline(ctx context.Context, database *sql.DB) error {
	var hasHistory, hasTables bool
	if err := database.QueryRowContext(ctx, `SELECT
        EXISTS(SELECT 1 FROM sqlite_schema WHERE type='table' AND name='goose_db_version'),
        EXISTS(SELECT 1 FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name != 'goose_db_version')`).Scan(&hasHistory, &hasTables); err != nil {
		return fmt.Errorf("inspect migration history: %w", err)
	}
	if !hasHistory {
		if hasTables {
			return ErrIncompatibleBaseline
		}
		return nil
	}
	var identity int
	if err := database.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&identity); err != nil {
		return fmt.Errorf("read schema baseline identity: %w", err)
	}
	if identity != BaselineIdentity {
		// Goose creates its bookkeeping outside the migration transaction. A
		// failed first migration can leave only its version-zero row behind;
		// retry that empty file without accepting an old application schema.
		var applied, tables int
		if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM goose_db_version applied
            WHERE applied.version_id > 0 AND applied.is_applied=1
              AND NOT EXISTS(SELECT 1 FROM goose_db_version later
                WHERE later.version_id=applied.version_id AND later.id>applied.id)`).Scan(&applied); err != nil {
			return fmt.Errorf("inspect applied migrations: %w", err)
		}
		if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name != 'goose_db_version'`).Scan(&tables); err != nil {
			return fmt.Errorf("inspect initial migration state: %w", err)
		}
		if identity == 0 && applied == 0 && tables == 0 {
			return nil
		}
		return ErrIncompatibleBaseline
	}
	return nil
}
