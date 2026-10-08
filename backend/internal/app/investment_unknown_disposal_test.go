package app

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
)

// knownTwoLotSale posts a FIFO sale of 12 units across a 10-unit lot (EUR
// 100.00) and a 5-unit lot (EUR 60.00). Writers still refuse unknown inputs,
// so unknown disposal evidence below is appended as the replay writer would
// store it, against that real committed decision.
type knownTwoLotSale struct {
	decisionID, operationID, auditEventID int64
	firstLotID, secondLotID               int64
}

func seedKnownTwoLotSale(t *testing.T, f *investmentsTestFixture) knownTwoLotSale {
	t.Helper()
	first := buyOn(t, f, "2026-06-01", 10, 10000)
	second := buyOn(t, f, "2026-06-02", 5, 6000)
	input := sellInput(f, "2026-06-10", 12)
	input.CostBasisMethod = "fifo"
	_, err := f.investmentService.Sell(context.Background(), input)
	require.NoError(t, err)
	sale := knownTwoLotSale{firstLotID: *first.LotID, secondLotID: *second.LotID}
	require.NoError(t, f.database.QueryRow(`SELECT d.id, d.operation_id, d.created_audit_event_id
		FROM investment_disposal_decisions d`).Scan(&sale.decisionID, &sale.operationID, &sale.auditEventID))
	return sale
}

type disposalRevisionAllocation struct {
	lotID     int64
	quantity  string
	basis     any // nil for unknown
	knowledge string
	proceeds  string
}

// appendDisposalRevision stores a replay revision with the given total
// knowledge/amount and allocation rows, exactly as persisted by replay.
func appendDisposalRevision(t *testing.T, execer interface {
	Exec(string, ...any) (sql.Result, error)
}, sale knownTwoLotSale, knowledge string, total any, allocations []disposalRevisionAllocation) int64 {
	t.Helper()
	var scale any
	if total != nil {
		scale = 2
	}
	result, err := execer.Exec(`INSERT INTO investment_disposal_revisions (book_id, decision_id, revision_seq,
		caused_by_operation_id, supersedes_revision_id, disposed_basis_value, disposed_basis_scale,
		basis_knowledge, created_at, created_audit_event_id)
		VALUES (?, ?, 2, ?, NULL, ?, ?, ?, '2026-10-07T00:00:00Z', ?)`,
		BookID, sale.decisionID, sale.operationID, total, scale, knowledge, sale.auditEventID)
	require.NoError(t, err)
	revisionID, err := result.LastInsertId()
	require.NoError(t, err)
	for index, allocation := range allocations {
		var basisScale any
		if allocation.basis != nil {
			basisScale = 2
		}
		_, err := execer.Exec(`INSERT INTO investment_disposal_revision_allocations (book_id, revision_id,
			allocation_seq, lot_id, quantity_value, quantity_scale, cost_basis_value, cost_basis_scale,
			proceeds_value, proceeds_scale, basis_knowledge) VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, 2, ?)`,
			BookID, revisionID, index+1, allocation.lotID, allocation.quantity, allocation.basis, basisScale,
			allocation.proceeds, allocation.knowledge)
		require.NoError(t, err)
	}
	return revisionID
}

func mixedUnknownRevision(sale knownTwoLotSale) []disposalRevisionAllocation {
	return []disposalRevisionAllocation{
		{lotID: sale.firstLotID, quantity: "10", knowledge: "unknown", proceeds: "8333"},
		{lotID: sale.secondLotID, quantity: "2", basis: "2400", knowledge: "known", proceeds: "1667"},
	}
}

func TestDisposalBasisKnowledgeRequiresCompleteAmountPair(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	sale := seedKnownTwoLotSale(t, f)
	appendDisposalRevision(t, f.database, sale, "known", "12400", []disposalRevisionAllocation{
		{lotID: sale.firstLotID, quantity: "10", basis: "10000", knowledge: "known", proceeds: "8333"},
		{lotID: sale.secondLotID, quantity: "2", basis: "2400", knowledge: "known", proceeds: "1667"},
	})
	for _, table := range []struct{ name, value, scale string }{
		{"investment_disposal_decisions", "disposed_basis_value", "disposed_basis_scale"},
		{"investment_disposal_allocations", "cost_basis_value", "cost_basis_scale"},
		{"investment_disposal_revisions", "disposed_basis_value", "disposed_basis_scale"},
		{"investment_disposal_revision_allocations", "cost_basis_value", "cost_basis_scale"},
	} {
		for _, knowledge := range []string{"known", "unknown"} {
			for mask := 0; mask < 4; mask++ {
				t.Run(table.name+"/"+knowledge+"/"+string(rune('0'+mask)), func(t *testing.T) {
					tx, err := f.database.BeginTx(context.Background(), nil)
					require.NoError(t, err)
					defer tx.Rollback()
					_, err = tx.Exec(`DROP TRIGGER ` + table.name + `_no_update`)
					require.NoError(t, err)
					var value, scale any
					if mask&1 != 0 {
						value = "0" // a known zero, never a stand-in for unknown
					}
					if mask&2 != 0 {
						scale = 0
					}
					_, err = tx.Exec(`UPDATE `+table.name+` SET basis_knowledge=?, `+table.value+`=?, `+table.scale+`=?`,
						knowledge, value, scale)
					if knowledge == "known" && mask == 3 || knowledge == "unknown" && mask == 0 {
						require.NoError(t, err)
					} else {
						require.ErrorContains(t, err, "CHECK constraint failed")
					}
				})
			}
		}
	}
}

func TestUnknownDisposalAllocationRequiresUnknownTotalAndMatchingLotEvent(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	sale := seedKnownTwoLotSale(t, f)
	tx, err := f.database.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()

	// A second decision on the same operation is the fixture for an unknown
	// total; each candidate allocation references its own disposal event.
	_, err = tx.Exec(`CREATE TEMP TABLE decision_fixture AS SELECT * FROM investment_disposal_decisions WHERE id = ?`, sale.decisionID)
	require.NoError(t, err)
	_, err = tx.Exec(`UPDATE decision_fixture SET id = NULL, decision_seq = 2, basis_knowledge = 'unknown',
		disposed_basis_value = NULL, disposed_basis_scale = NULL`)
	require.NoError(t, err)
	result, err := tx.Exec(`INSERT INTO investment_disposal_decisions SELECT * FROM decision_fixture`)
	require.NoError(t, err)
	unknownDecisionID, err := result.LastInsertId()
	require.NoError(t, err)
	event := func(knowledge string) int64 {
		var value, scale any = "-1200", 2
		if knowledge == "unknown" {
			value, scale = nil, nil
		}
		result, err := tx.Exec(`INSERT INTO investment_lot_events (book_id, lot_id, event_kind, event_date,
			quantity_value, quantity_scale, cost_basis_value, cost_basis_scale, created_at, created_by_user_id,
			basis_knowledge) VALUES (?, ?, 'disposal', '2026-06-10', '-1', 0, ?, ?, '2026-10-07T00:00:00Z', ?, ?)`,
			BookID, sale.secondLotID, value, scale, f.ownerUserID, knowledge)
		require.NoError(t, err)
		id, err := result.LastInsertId()
		require.NoError(t, err)
		return id
	}
	allocate := func(decisionID, eventID int64, seq int, knowledge string) error {
		var value, scale any = "1200", 2
		if knowledge == "unknown" {
			value, scale = nil, nil
		}
		_, err := tx.Exec(`INSERT INTO investment_disposal_allocations (book_id, decision_id, lot_event_id, lot_id,
			allocation_seq, quantity_value, quantity_scale, cost_basis_value, cost_basis_scale, proceeds_value,
			proceeds_scale, basis_knowledge) VALUES (?, ?, ?, ?, ?, '1', 0, ?, ?, '500', 2, ?)`,
			BookID, decisionID, eventID, sale.secondLotID, seq, value, scale, knowledge)
		return err
	}

	require.NoError(t, allocate(unknownDecisionID, event("unknown"), 1, "unknown"))
	require.NoError(t, allocate(unknownDecisionID, event("known"), 2, "known"),
		"an unknown total keeps independently known allocation amounts")
	require.ErrorContains(t, allocate(sale.decisionID, event("unknown"), 9, "unknown"),
		"disagrees with its decision", "a known total cannot hide an unknown allocation")
	require.ErrorContains(t, allocate(unknownDecisionID, event("unknown"), 3, "known"),
		"disagrees with its decision or lot event", "an allocation cannot invent the basis its event lacks")
	require.ErrorContains(t, allocate(unknownDecisionID, event("known"), 4, "unknown"),
		"disagrees with its decision or lot event")

	revisionID := appendDisposalRevision(t, tx, sale, "known", "1200", nil)
	_, err = tx.Exec(`INSERT INTO investment_disposal_revision_allocations (book_id, revision_id, allocation_seq,
		lot_id, quantity_value, quantity_scale, proceeds_value, proceeds_scale, basis_knowledge)
		VALUES (?, ?, 1, ?, '12', 0, '10000', 2, 'unknown')`, BookID, revisionID, sale.firstLotID)
	require.ErrorContains(t, err, "outside its position or knowledge", "a known revision total cannot hide an unknown allocation")
}

func TestUnknownDisposalRevisionKeepsAllocationSetsHealthyAndDamageVisible(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name        string
		allocations func(knownTwoLotSale) []disposalRevisionAllocation
		damaged     bool
	}{
		{name: "mixed unknown is unresolved information", allocations: mixedUnknownRevision},
		{name: "invented unknown over known allocations", damaged: true, allocations: func(sale knownTwoLotSale) []disposalRevisionAllocation {
			return []disposalRevisionAllocation{
				{lotID: sale.firstLotID, quantity: "10", basis: "10000", knowledge: "known", proceeds: "8333"},
				{lotID: sale.secondLotID, quantity: "2", basis: "2400", knowledge: "known", proceeds: "1667"},
			}
		}},
		{name: "quantity damage despite unknown basis", damaged: true, allocations: func(sale knownTwoLotSale) []disposalRevisionAllocation {
			allocations := mixedUnknownRevision(sale)
			allocations[1].quantity = "1"
			return allocations
		}},
		{name: "proceeds damage despite unknown basis", damaged: true, allocations: func(sale knownTwoLotSale) []disposalRevisionAllocation {
			allocations := mixedUnknownRevision(sale)
			allocations[0].proceeds = "8000"
			return allocations
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			sale := seedKnownTwoLotSale(t, f)
			revisionID := appendDisposalRevision(t, f.database, sale, "unknown", nil, scenario.allocations(sale))
			check := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckLotReconciliation)
			if scenario.damaged {
				require.Equal(t, SelfCheckFailed, check.Status)
				require.Contains(t, check.Summary, "disposal allocation sets disagree")
				require.Contains(t, check.Summary, "revision #")
				require.Equal(t, []int64{sale.decisionID}, check.Sample)
			} else {
				require.NotContains(t, check.Summary, "disposal allocation sets disagree",
					"a NULL total over an unknown allocation is not a basis mismatch")
			}
			var total sql.NullString
			require.NoError(t, f.database.QueryRow(`SELECT disposed_basis_value FROM investment_disposal_revisions WHERE id = ?`, revisionID).Scan(&total))
			require.False(t, total.Valid, "unknown total is stored as NULL, never zero")
		})
	}
}

func TestUnknownDisposalRevisionExportsBlankAmountsAndKnowledge(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	sale := seedKnownTwoLotSale(t, f)
	appendDisposalRevision(t, f.database, sale, "unknown", nil, mixedUnknownRevision(sale))
	var out bytes.Buffer
	require.NoError(t, NewExportService(db.NewExportRepository(f.database)).WriteBundle(context.Background(), &out, ExportFilter{}))
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	files := map[string][][]string{}
	for _, file := range archive.File {
		reader, err := file.Open()
		require.NoError(t, err)
		if slices.Contains([]string{"disposal-decisions.csv", "disposal-allocations.csv", "disposal-revisions.csv",
			"disposal-revision-allocations.csv"}, file.Name) {
			files[file.Name], err = csv.NewReader(reader).ReadAll()
			require.NoError(t, err)
		}
		require.NoError(t, reader.Close())
	}
	column := func(file string, row int, name string) string {
		rows := files[file]
		index := slices.Index(rows[0], name)
		require.GreaterOrEqual(t, index, 0, file+" "+name)
		return rows[row][index]
	}

	// The original snapshot stays known evidence.
	require.Equal(t, "known", column("disposal-decisions.csv", 1, "basis_knowledge"))
	require.Equal(t, "124.000000", column("disposal-decisions.csv", 1, "disposed_basis"))
	require.Equal(t, "known", column("disposal-allocations.csv", 1, "basis_knowledge"))

	require.Equal(t, "unknown", column("disposal-revisions.csv", 1, "basis_knowledge"))
	require.Empty(t, column("disposal-revisions.csv", 1, "disposed_basis_value"))
	require.Empty(t, column("disposal-revisions.csv", 1, "disposed_basis_scale"))
	require.Len(t, files["disposal-revision-allocations.csv"], 3)
	require.Equal(t, "unknown", column("disposal-revision-allocations.csv", 1, "basis_knowledge"))
	require.Empty(t, column("disposal-revision-allocations.csv", 1, "cost_basis_value"))
	require.Empty(t, column("disposal-revision-allocations.csv", 1, "cost_basis_scale"))
	require.Equal(t, "10", column("disposal-revision-allocations.csv", 1, "quantity_value"), "quantity survives unknown basis")
	require.Equal(t, "known", column("disposal-revision-allocations.csv", 2, "basis_knowledge"))
	require.Equal(t, "2400", column("disposal-revision-allocations.csv", 2, "cost_basis_value"))
}

func TestRealizedGainsReportUnknownEffectiveRevisionAsUnresolved(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	sale := seedKnownTwoLotSale(t, f)
	appendDisposalRevision(t, f.database, sale, "unknown", nil, mixedUnknownRevision(sale))
	gains, err := f.investmentService.ListRealizedGains(context.Background(), GainsReportParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)
	require.Equal(t, db.InvestmentBasisUnknown, gains[0].BasisKnowledge,
		"one unknown allocation leaves the disposal unresolved, never a gain against its known part")
	require.Equal(t, "12", gains[0].QuantityValue.String())
	require.Equal(t, int64(10000), gains[0].ProceedsValue)
}
