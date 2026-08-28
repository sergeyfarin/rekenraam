// Package testdb provides isolated, fully migrated SQLite databases for tests.
package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"rekenraam/backend/internal/db"
)

var (
	templateOnce sync.Once
	templateData []byte
	templateErr  error
)

// Open copies a process-wide migrated template into a test-owned temporary
// directory, then opens it through the same PRAGMA-validating path as runtime.
// Each test receives an independent database file and pool.
func Open(t testing.TB) (*sql.DB, string) {
	t.Helper()

	data, err := migratedTemplate()
	if err != nil {
		t.Fatalf("build migrated SQLite test template: %v", err)
	}

	databasePath := filepath.Join(t.TempDir(), "rekenraam.sqlite")
	if err := os.WriteFile(databasePath, data, 0o600); err != nil {
		t.Fatalf("copy migrated SQLite test template: %v", err)
	}

	databaseURL := "file:" + databasePath
	database, err := db.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open copied SQLite test database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close copied SQLite test database: %v", err)
		}
	})

	return database, databaseURL
}

func migratedTemplate() ([]byte, error) {
	templateOnce.Do(func() {
		templateData, templateErr = buildTemplate()
	})
	return templateData, templateErr
}

func buildTemplate() ([]byte, error) {
	directory, err := os.MkdirTemp("", "rekenraam-testdb-template-")
	if err != nil {
		return nil, fmt.Errorf("create template directory: %w", err)
	}
	defer os.RemoveAll(directory)

	databasePath := filepath.Join(directory, "template.sqlite")
	database, err := db.Open(context.Background(), "file:"+databasePath)
	if err != nil {
		return nil, err
	}

	if err := db.Migrate(context.Background(), database); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := database.Close(); err != nil {
		return nil, fmt.Errorf("close migrated template: %w", err)
	}

	data, err := os.ReadFile(databasePath)
	if err != nil {
		return nil, fmt.Errorf("read migrated template: %w", err)
	}
	return data, nil
}
