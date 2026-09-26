package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
)

// Slice 1 pins the investment export on both a fresh baseline and a book
// containing lots, a partial disposal, a dividend, and trade-derived prices.
// The 2a baseline rewrite must update this alongside its new lossless files.
func TestInvestmentSlice1FreshAndSeededBundleContract(t *testing.T) {
	for _, seeded := range []bool{false, true} {
		name := "fresh"
		if seeded {
			name = "seeded"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			database, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "candidate.sqlite"))
			require.NoError(t, err)
			defer database.Close()
			require.NoError(t, db.Migrate(ctx, database))

			if seeded {
				seed, err := os.ReadFile(filepath.Join("..", "db", "testdata", "v01_seed.sql"))
				require.NoError(t, err)
				tx, err := database.BeginTx(ctx, nil)
				require.NoError(t, err)
				defer tx.Rollback()
				_, err = tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON")
				require.NoError(t, err)
				_, err = tx.ExecContext(ctx, string(seed))
				require.NoError(t, err)
				require.NoError(t, tx.Commit())
			}

			var out bytes.Buffer
			require.NoError(t, app.NewExportService(db.NewExportRepository(database)).WriteBundle(ctx, &out, app.ExportFilter{}))
			archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
			require.NoError(t, err)
			files := map[string][]byte{}
			for _, entry := range archive.File {
				reader, err := entry.Open()
				require.NoError(t, err)
				content, err := io.ReadAll(reader)
				require.NoError(t, err)
				require.NoError(t, reader.Close())
				files[entry.Name] = content
			}
			var manifest struct {
				SchemaVersion int `json:"schema_version"`
				Files         []struct {
					Name string `json:"name"`
					Rows int64  `json:"rows"`
				} `json:"files"`
			}
			require.NoError(t, json.Unmarshal(files["manifest.json"], &manifest))
			require.Equal(t, 1, manifest.SchemaVersion)
			manifestRows := map[string]int64{}
			for _, file := range manifest.Files {
				manifestRows[file.Name] = file.Rows
			}
			for file, header := range map[string][]string{
				"lots.csv":                  {"lot_id", "account_id", "account_path", "commodity_id", "position_side", "opened_on", "status", "quantity", "remaining_quantity", "cost_basis", "remaining_cost_basis", "cost_commodity_id", "source_transaction_id"},
				"investment-operations.csv": {"operation_id", "transaction_id", "operation_kind", "event_date", "audit_event_id"},
				"disposal-decisions.csv":    {"decision_id", "transaction_id", "transaction_version_id", "account_id", "commodity_id", "cost_commodity_id", "event_date", "quantity", "disposed_basis", "cost_basis_method", "resolution_tier", "account_version_id", "profile_id", "profile_version_id", "source_effective_from", "source_recorded_at", "created_at", "audit_event_id"},
				"disposal-allocations.csv":  {"decision_id", "allocation_seq", "lot_event_id", "lot_id", "quantity", "cost_basis"},
				"prices.csv":                {"base_commodity_id", "quote_commodity_id", "valuation_date", "price", "base_quantity", "quote_type", "adjustment_basis", "is_manual", "is_derived", "source"},
			} {
				rows := readInvestmentContractCSV(t, files, file)
				require.Equal(t, header, rows[0], file)
				require.Equal(t, int64(len(rows)-1), manifestRows[file], file)
				if !seeded {
					require.Len(t, rows, 1, file+" must retain its header without data")
				}
			}
			require.NotEmpty(t, readInvestmentContractCSV(t, files, "ledger.csv"))

			if seeded {
				lots := readInvestmentContractCSV(t, files, "lots.csv")
				require.Len(t, lots, 3)
				require.Equal(t, "1000.00", lots[1][9])
				require.Equal(t, "0.00", lots[1][10])
				require.Equal(t, "600.00", lots[2][9])
				require.Equal(t, "300.00", lots[2][10])

				operations := readInvestmentContractCSV(t, files, "investment-operations.csv")
				require.Len(t, operations, 5)
				require.Equal(t, []string{"buy", "buy", "sell", "dividend"}, []string{operations[1][2], operations[2][2], operations[3][2], operations[4][2]})

				decisions := readInvestmentContractCSV(t, files, "disposal-decisions.csv")
				require.Len(t, decisions, 2)
				require.Equal(t, "12.50", decisions[1][7])
				require.Equal(t, "1300.00", decisions[1][8])
				require.Equal(t, "fifo", decisions[1][9])
				require.Equal(t, "fallback", decisions[1][10])
				require.NotEmpty(t, decisions[1][2], "the export must retain the posted transaction version")

				allocations := readInvestmentContractCSV(t, files, "disposal-allocations.csv")
				require.Len(t, allocations, 3)
				require.Equal(t, decisions[1][0], allocations[1][0])
				require.Equal(t, decisions[1][0], allocations[2][0])
				require.Equal(t, "1000.00", allocations[1][5])
				require.Equal(t, "300.00", allocations[2][5])
				require.Len(t, readInvestmentContractCSV(t, files, "prices.csv"), 4)
			}
		})
	}
}

func readInvestmentContractCSV(t *testing.T, files map[string][]byte, name string) [][]string {
	t.Helper()
	raw, ok := files[name]
	require.True(t, ok, name+" is missing from the bundle")
	require.True(t, bytes.HasPrefix(raw, []byte("\xef\xbb\xbf")), name+" must be UTF-8 with BOM")
	rows, err := csv.NewReader(strings.NewReader(string(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))))).ReadAll()
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	return rows
}
