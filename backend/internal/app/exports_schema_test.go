package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
)

// ledgerCSVSchemaV1 is the published column list of ledger.csv, written out
// rather than derived, so a reorder or a rename is a failing test instead of a
// silently broken consumer.
//
// ADR 0011 clause 6 makes this list append-only within a major export schema
// version: columns are added at the end, never reordered, renamed, or removed.
// Nothing else in the suite could tell the difference — every other export test
// reads its columns *by name* through the header, which is exactly what makes
// them immune to the change that would break a script parsing by position.
//
// Changing this list legitimately means appending a name here and at the end of
// app.LedgerExportColumns, in the same order. Any other edit needs a new ADR.
var ledgerCSVSchemaV1 = []string{
	"transaction_id",
	"transaction_version_id",
	"transaction_kind",
	"status",
	"transaction_date",
	"correction_of_transaction_id",
	"payee_id",
	"payee",
	"description",
	"external_ref_hint",
	"needs_review",
	"transaction_tags",
	"transaction_complete",
	"journal_entry_id",
	"entry_seq",
	"entry_kind",
	"entry_date",
	"entry_memo",
	"posting_line_key",
	"line_seq",
	"account_id",
	"account_path",
	"account_class",
	"account_kind",
	"quantity",
	"commodity_id",
	"commodity",
	"commodity_kind",
	"posting_memo",
	"reconciliation_status",
	"cleared_on",
	"posting_tags",
}

func TestLedgerExportColumnsAreAppendOnly(t *testing.T) {
	t.Parallel()

	require.GreaterOrEqual(t, len(app.LedgerExportColumns), len(ledgerCSVSchemaV1),
		"a published column may not be removed")
	assert.Equal(t, ledgerCSVSchemaV1, app.LedgerExportColumns[:len(ledgerCSVSchemaV1)],
		"published columns keep their names and their positions; new ones go at the end")

	// A duplicate name would make a header ambiguous to any consumer indexing
	// by name, which is how most of them will read it.
	seen := map[string]bool{}
	for _, column := range app.LedgerExportColumns {
		assert.False(t, seen[column], "duplicate column %q", column)
		seen[column] = true
	}
}
