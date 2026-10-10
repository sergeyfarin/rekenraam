package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
)

// journalCounts is how many transactions and posting versions a book holds:
// a journal-free command must leave both unchanged.
func journalCounts(t *testing.T, f *investmentsTestFixture) [2]int {
	t.Helper()
	var counts [2]int
	require.NoError(t, f.database.QueryRow(`SELECT (SELECT COUNT(*) FROM transactions), (SELECT COUNT(*) FROM posting_versions)`).
		Scan(&counts[0], &counts[1]))
	return counts
}

// operationJournalLinks counts the journal links of the newest operation.
func newestOperation(t *testing.T, f *investmentsTestFixture) (id int64, kind string, links int) {
	t.Helper()
	require.NoError(t, f.database.QueryRow(`SELECT o.id, o.operation_kind,
		(SELECT COUNT(*) FROM investment_operation_journal_links l WHERE l.operation_id = o.id)
		FROM investment_operations o ORDER BY o.id DESC LIMIT 1`).Scan(&id, &kind, &links))
	return id, kind, links
}

// A sourced known zero is recorded without a fabricated zero journal: the
// resolution is a journal-free operation under its own audit event, and the
// sale it reached becomes a known gain against a zero basis.
func TestResolvingUnknownInboundAsKnownZeroIsJournalFree(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", ""))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	sale := latestDisposal(t, f)
	before := journalCounts(t, f)

	input := resolveInput(f, transfer.Transaction.ID, 0)
	impact, err := f.investmentService.ResolveTransferBasisImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.GainImpact.Changes, 1)
	require.Equal(t, db.InvestmentBasisKnown, impact.GainImpact.Changes[0].After.BasisKnowledge)
	requireScaled(t, 0, 2, impact.GainImpact.Changes[0].After.DisposedBasis, "a known zero, not unknown")
	_, err = f.investmentService.ResolveTransferBasis(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	require.Equal(t, before, journalCounts(t, f))
	resolved := resolveAcknowledged(t, f, input)
	require.Nil(t, resolved, "a known zero posts no journal")
	require.Equal(t, before, journalCounts(t, f), "no transaction or posting is fabricated")

	operationID, kind, links := newestOperation(t, f)
	require.Equal(t, "basis_resolution", kind)
	require.Zero(t, links)
	var basis, operation string
	require.NoError(t, f.database.QueryRow(`SELECT r.basis_value, a.operation FROM investment_basis_resolutions r
		JOIN audit_events a ON a.id = r.created_audit_event_id WHERE r.operation_id = ?`, operationID).Scan(&basis, &operation))
	require.Equal(t, []string{"0", "investment.basis_resolution"}, []string{basis, operation})
	var dateRole string
	require.NoError(t, f.database.QueryRow(`SELECT date_role FROM investment_operation_dates WHERE operation_id = ?`, operationID).Scan(&dateRole))
	require.Equal(t, "effective", dateRole)

	resolvedSale := effectiveDisposal(t, f, sale.id)
	require.Equal(t, db.InvestmentBasisKnown, resolvedSale.knowledge)
	requireScaled(t, 0, 2, resolvedSale.basisAmount(t), "the sold unit carries a known zero")
	_, remaining, knowledge := lotStateByID(t, f, *transfer.LotID)
	require.Equal(t, db.InvestmentBasisKnown, knowledge)
	require.Equal(t, "0", remaining.String)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, transfer.Transaction.ID)
	require.NoError(t, err)
	require.True(t, chain.CanCorrectBasisResolution)
	require.Zero(t, chain.EffectiveBasisResolution.TransactionID, "a journal-free resolution has no transaction")
	require.Equal(t, operationID, chain.EffectiveBasisResolution.OperationID)
	requireHealthyUnresolvedBook(t, f)
}

// A known zero reaching an unknown outbound makes its links known with a zero
// basis; the complete omitted bridge is zero, so nothing posts.
func TestKnownZeroResolutionMakesOutboundKnownWithoutABridge(t *testing.T) {
	t.Parallel()
	f := newTransferOutFixture(t)
	ctx := context.Background()
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-05-01", ""))
	require.NoError(t, err)
	outbound, err := f.investmentService.ExternalTransferOut(ctx, transferOutOfLot(f, "2026-06-01", *transfer.LotID, 1))
	require.NoError(t, err)
	before := journalCounts(t, f)
	resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 0))
	require.Equal(t, before, journalCounts(t, f))
	require.Equal(t, []string{db.InvestmentBasisKnown}, linkKnowledge(t, f, outbound.Transaction.ID))
	require.Empty(t, outboundBridgeTotal(t, f, outbound.Transaction.ID))
	requireHealthyUnresolvedBook(t, f)
}

// Replacing across zero: a bridged resolution replaced by a known zero posts
// only the inverse of its bridge, and a known zero replaced by a positive
// basis posts only the successor's bridge. Each step revises the sale.
func TestReplacingResolutionBetweenZeroAndPositiveBasis(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", ""))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	sale := latestDisposal(t, f)
	first := resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 8000))

	toZero := replaceResolutionAcknowledged(t, f, correctResolutionInput(f, transfer.Transaction.ID, 0))
	require.NotNil(t, toZero.Inverse)
	require.Nil(t, toZero.Replacement, "the zero successor is journal-free")
	requireScaled(t, -8000, 2, journalByRole(t, f, toZero.Inverse)["commodity_trading"], "the old bridge is inverted")
	requireScaled(t, 0, 2, effectiveDisposal(t, f, sale.id).basisAmount(t), "the sale now carries the known zero")
	zeroID, kind, links := newestOperation(t, f)
	require.Equal(t, "basis_resolution", kind)
	require.Equal(t, 1, links, "its only journal link is the inverse of its predecessor")
	requireHealthyUnresolvedBook(t, f)

	fromZero := replaceResolutionAcknowledged(t, f, correctResolutionInput(f, transfer.Transaction.ID, 6000))
	require.Nil(t, fromZero.Inverse, "a journal-free zero has no bridge to invert")
	require.NotNil(t, fromZero.Replacement)
	requireScaled(t, 6000, 2, journalByRole(t, f, fromZero.Replacement)["commodity_trading"], "the successor posts its complete bridge")
	requireScaled(t, 3000, 2, effectiveDisposal(t, f, sale.id).basisAmount(t), "half the corrected basis left with the sale")
	var correctionOf int64
	require.NoError(t, f.database.QueryRow(`SELECT o.correction_of_operation_id FROM investment_operations o
		JOIN investment_operation_journal_links l ON l.operation_id = o.id AND l.role = 'primary'
		JOIN transaction_versions v ON v.id = l.transaction_version_id WHERE v.transaction_id = ?`,
		fromZero.Replacement.ID).Scan(&correctionOf))
	require.Equal(t, zeroID, correctionOf, "the bridge successor corrects the journal-free zero")

	net := journalByRole(t, f, first)["commodity_trading"]
	net.AddScaled(journalByRole(t, f, toZero.Inverse)["commodity_trading"])
	net.AddScaled(journalByRole(t, f, fromZero.Replacement)["commodity_trading"])
	requireScaled(t, 6000, 2, net, "the resolution journals net to the effective basis")
	requireHealthyUnresolvedBook(t, f)
}

// A known zero is reversed without a journal: the transfer reads unknown
// again, the sale it resolved is unresolved again, and the transfer itself
// becomes correctable.
func TestReversingKnownZeroResolutionIsJournalFree(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", ""))
	require.NoError(t, err)
	_, err = f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 1))
	require.NoError(t, err)
	sale := latestDisposal(t, f)
	resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 0))
	before := journalCounts(t, f)

	reversed := reverseResolutionAcknowledged(t, f, correctResolutionInput(f, transfer.Transaction.ID, 0))
	require.Nil(t, reversed)
	require.Equal(t, before, journalCounts(t, f))
	_, kind, links := newestOperation(t, f)
	require.Equal(t, []any{"reversal", 0}, []any{kind, links})
	require.Equal(t, db.InvestmentBasisUnknown, effectiveDisposal(t, f, sale.id).knowledge)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, transfer.Transaction.ID)
	require.NoError(t, err)
	require.True(t, chain.CanResolveBasis)
	require.True(t, chain.CanReplaceTransfer)
	requireHealthyUnresolvedBook(t, f)
}

// Self-check ties a resolution's journals to its basis: a known zero with a
// bridge, or a positive basis without one, is damage. The exemption covers
// only journal-free known zeros and their reversals.
func TestSelfCheckDetectsKnownZeroJournalMismatch(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, from, to, summary string
		basis                   int64
	}{
		{"bridged resolution relabelled zero", "8000", "0", "known-zero basis resolution or its reversal posts a journal", 8000},
		{"journal-free zero relabelled positive", "0", "8000", "basis resolution disagrees with its bridge or pinned transfer", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			seedExternalTransferEquity(t, f.database)
			transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", ""))
			require.NoError(t, err)
			resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, tc.basis))
			requireHealthyUnresolvedBook(t, f)

			_, err = f.database.Exec(`DROP TRIGGER investment_basis_resolutions_no_update`)
			require.NoError(t, err)
			_, err = f.database.Exec(`UPDATE investment_basis_resolutions SET basis_value = ? WHERE basis_value = ?`, tc.to, tc.from)
			require.NoError(t, err)
			foundation := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation)
			require.Equal(t, SelfCheckFailed, foundation.Status)
			require.Contains(t, foundation.Summary, tc.summary)
		})
	}
}

// The bundle exports a journal-free resolution with a blank transaction and a
// zero basis, and a restored database keeps it intact and healthy.
func TestKnownZeroResolutionExportsAndSurvivesRestore(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	transfer, err := f.investmentService.ExternalTransferIn(ctx, unknownTransferInInput(f, "2026-06-01", ""))
	require.NoError(t, err)
	resolveAcknowledged(t, f, resolveInput(f, transfer.Transaction.ID, 0))
	zeroID, _, _ := newestOperation(t, f)

	var out bytes.Buffer
	require.NoError(t, NewExportService(db.NewExportRepository(f.database)).WriteBundle(ctx, &out, ExportFilter{}))
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	csvRows := func(name string) [][]string {
		for _, entry := range archive.File {
			if entry.Name != name {
				continue
			}
			reader, err := entry.Open()
			require.NoError(t, err)
			raw, err := io.ReadAll(reader)
			require.NoError(t, err)
			rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), "\xef\xbb\xbf"))).ReadAll()
			require.NoError(t, err)
			return rows
		}
		t.Fatalf("%s is missing from the bundle", name)
		return nil
	}
	resolutions := csvRows("investment-basis-resolutions.csv")
	require.Len(t, resolutions, 2)
	require.Equal(t, "0", resolutions[1][7], "the known zero is numeric, never blank")
	var exported bool
	for _, row := range csvRows("investment-operations.csv")[1:] {
		if row[0] == strconv.FormatInt(zeroID, 10) {
			exported = true
			require.Equal(t, "basis_resolution", row[2])
			require.Empty(t, row[1], "a journal-free operation exports no transaction")
		}
	}
	require.True(t, exported)

	backupPath := filepath.Join(t.TempDir(), "backup.sqlite")
	_, err = db.OnlineBackupSQLiteDatabase(ctx, f.database, backupPath, db.OnlineBackupOptions{})
	require.NoError(t, err)
	restoredURL := "file:" + filepath.Join(t.TempDir(), "restored.sqlite")
	_, err = db.RestoreSQLiteDatabase(ctx, backupPath, restoredURL)
	require.NoError(t, err)
	restored, err := db.Open(ctx, restoredURL)
	require.NoError(t, err)
	defer restored.Close()
	var basis string
	var links int
	require.NoError(t, restored.QueryRow(`SELECT r.basis_value, (SELECT COUNT(*) FROM investment_operation_journal_links l
		WHERE l.operation_id = r.operation_id) FROM investment_basis_resolutions r WHERE r.operation_id = ?`, zeroID).Scan(&basis, &links))
	require.Equal(t, "0", basis)
	require.Zero(t, links)
	run, err := selfCheckOver(t, restored).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, resultFor(t, run, CheckInvestmentFoundation).Status, resultFor(t, run, CheckInvestmentFoundation).Summary)
	require.Equal(t, SelfCheckPassed, resultFor(t, run, CheckInvestmentReplay).Status, resultFor(t, run, CheckInvestmentReplay).Summary)
}
