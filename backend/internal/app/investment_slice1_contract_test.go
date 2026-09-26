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

// The slice 1 economics fixture now pins the slice 2a investment export on
// both a fresh baseline and a book with lots, a partial disposal, a dividend,
// and trade-derived prices.
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
			require.Equal(t, 2, manifest.SchemaVersion)
			manifestRows := map[string]int64{}
			for _, file := range manifest.Files {
				manifestRows[file.Name] = file.Rows
			}
			for file, header := range map[string][]string{
				"lots.csv":                               {"lot_id", "account_id", "account_path", "commodity_id", "position_side", "opened_on", "status", "quantity", "remaining_quantity", "cost_basis", "remaining_cost_basis", "cost_commodity_id", "source_transaction_id"},
				"investment-operations.csv":              {"operation_id", "transaction_id", "operation_kind", "event_date", "audit_event_id"},
				"investment-operation-journal-links.csv": {"operation_id", "link_seq", "transaction_version_id", "role"},
				"investment-operation-dates.csv":         {"operation_id", "date_role", "event_date"},
				"investment-operation-components.csv":    {"component_id", "operation_id", "component_seq", "component_kind", "commodity_id", "amount_value", "amount_scale", "amount_date", "gross_unknown", "charge_treatment", "charge_account_id", "resolution_tier", "fee_policy_version_id", "source_evidence_json", "audit_event_id"},
				"investment-lot-facts.csv":               {"lot_id", "operation_id", "account_id", "commodity_id", "position_side", "opened_on", "quantity_value", "quantity_scale", "consideration_value", "consideration_scale", "cost_commodity_id", "audit_event_id"},
				"investment-lot-events.csv":              {"lot_event_id", "lot_id", "event_kind", "transaction_id", "event_date", "quantity_value", "quantity_scale", "cost_basis_value", "cost_basis_scale", "cost_basis_method", "metadata_json", "audit_event_id"},
				"investment-lot-effects.csv":             {"operation_id", "effect_seq", "lot_event_id"},
				"investment-fee-policies.csv":            {"policy_id", "account_id", "charge_kind", "created_at", "audit_event_id"},
				"investment-fee-policy-versions.csv":     {"version_id", "policy_id", "version_seq", "effective_from", "treatment", "charge_account_id", "recorded_at", "audit_event_id"},
				"disposal-decisions.csv":                 {"decision_id", "transaction_id", "transaction_version_id", "account_id", "commodity_id", "cost_commodity_id", "event_date", "quantity", "disposed_basis", "cost_basis_method", "resolution_tier", "account_version_id", "profile_id", "profile_version_id", "source_effective_from", "source_recorded_at", "created_at", "audit_event_id", "operation_id", "position_side"},
				"disposal-allocations.csv":               {"decision_id", "allocation_seq", "lot_event_id", "lot_id", "quantity", "cost_basis"},
				"prices.csv":                             {"base_commodity_id", "quote_commodity_id", "valuation_date", "price", "base_quantity", "quote_type", "adjustment_basis", "is_manual", "is_derived", "source", "is_approximate", "source_transaction_version_id", "audit_event_id"},
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
				require.Equal(t, operations[3][0], decisions[1][18], "disposal decision links its operation")
				require.Equal(t, "long", decisions[1][19])
				require.NotEmpty(t, decisions[1][2], "the export must retain the posted transaction version")

				allocations := readInvestmentContractCSV(t, files, "disposal-allocations.csv")
				require.Len(t, allocations, 3)
				require.Equal(t, decisions[1][0], allocations[1][0])
				require.Equal(t, decisions[1][0], allocations[2][0])
				require.Equal(t, "1000.00", allocations[1][5])
				require.Equal(t, "300.00", allocations[2][5])
				require.Len(t, readInvestmentContractCSV(t, files, "prices.csv"), 4)
				prices := readInvestmentContractCSV(t, files, "prices.csv")
				require.Equal(t, "true", prices[1][10], "net-derived trade price remains usable but approximate")
				require.Equal(t, "11", prices[1][11], "price points to its posted transaction version")
				require.Equal(t, operations[1][4], prices[1][12], "trade and price share one audit event")
				require.Contains(t, string(files["accounts.csv"]), "external_investment_transfer_equity")
				require.Len(t, readInvestmentContractCSV(t, files, "investment-operation-journal-links.csv"), 5)
				require.Len(t, readInvestmentContractCSV(t, files, "investment-operation-components.csv"), 5)
				require.Len(t, readInvestmentContractCSV(t, files, "investment-lot-facts.csv"), 3)
				require.Len(t, readInvestmentContractCSV(t, files, "investment-lot-effects.csv"), 5)
				require.Len(t, readInvestmentContractCSV(t, files, "investment-fee-policy-versions.csv"), 2)
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
