package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
)

// #485 review: a mistaken link is repaired by an audited unlink or
// reassignment that keeps the original link as evidence, and the portable
// bundle carries every link.

// linkedRow imports one savings row and links it to transfer's savings side.
func (f importMatchFixture) linkedRow(t *testing.T, transfer Transaction, row statementRow, postedOn string) (int64, ImportStatementLink) {
	t.Helper()
	batchID, rows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID, row)
	f.resolve(t, batchID, rows[0], f.savingsAccountID, f.eurCommodityID, linkTo(transfer, lineKeyOn(t, transfer, f.savingsAccountID), postedOn))
	require.Equal(t, 1, f.commit(t, batchID).LinkedCount)
	links, err := f.importService.StatementLinks(context.Background(), transfer.ID)
	require.NoError(t, err)
	require.Len(t, links, 1)
	return batchID, links[0]
}

func (f importMatchFixture) correction(link ImportStatementLink, reason string) CorrectStatementLinkInput {
	return CorrectStatementLinkInput{OwnerUserID: f.ownerUserID, IdentityID: link.IdentityID, ExpectedLinkID: link.LinkID, Reason: reason}
}

func (f importMatchFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var count int
	require.NoError(t, f.database.QueryRowContext(context.Background(), query, args...).Scan(&count))
	return count
}

func TestStatementLinkReassignmentRepairsAMistakenMatch(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	ctx := context.Background()
	wrong := f.transfer(t, "2026-06-08", "2026-06-09")
	right := f.transfer(t, "2026-06-09", "2026-06-11")
	statement := statementRow{"2026-06-10", "100.00", "Savings top-up"}
	_, link := f.linkedRow(t, wrong, statement, "2026-06-09")
	assert.True(t, link.Active)
	assert.Equal(t, "2026-06-10", link.RowDate)

	// The reported failure: the wrong transfer is voided and the statement
	// re-imported; the row stays a duplicate and the right transfer unlinked.
	_, err := f.transactionService.VoidTransaction(ctx, VoidTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: wrong.ID, ChangeReason: "entered twice"})
	require.NoError(t, err)
	_, again := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID, statement)
	require.Equal(t, "duplicate", again[0].DedupeStatus)

	// The repair: the row's candidates offer the right transfer, and moving
	// the link keeps the first one as retired evidence.
	current, candidates, err := f.importService.StatementLinkCandidates(ctx, link.IdentityID)
	require.NoError(t, err)
	assert.Equal(t, link.LinkID, current.LinkID)
	require.Len(t, candidates, 1, "the voided transfer is no candidate")
	assert.Equal(t, right.ID, candidates[0].TransactionID)

	before := f.transactionCount(t)
	input := f.correction(link, "linked to the wrong transfer")
	input.TransactionID, input.LineKey, input.PostedOn = right.ID, candidates[0].LineKey, candidates[0].PostedOn
	require.NoError(t, f.importService.ReassignStatementRow(ctx, input))

	moved, err := f.importService.StatementLinks(ctx, right.ID)
	require.NoError(t, err)
	require.Len(t, moved, 1)
	assert.True(t, moved[0].Active)
	assert.Equal(t, link.IdentityID, moved[0].IdentityID, "the same statement row")
	assert.Equal(t, lineKeyOn(t, right, f.savingsAccountID), moved[0].LineKey)
	left, err := f.importService.StatementLinks(ctx, wrong.ID)
	require.NoError(t, err)
	assert.Empty(t, left, "the wrong transfer no longer stands for the row")

	assert.Equal(t, before, f.transactionCount(t), "a correction writes no journal")
	assert.Equal(t, 2, f.linkCount(t), "the first link is kept")
	assert.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM import_posting_link_retirements WHERE link_id = ? AND reason = 'linked to the wrong transfer'`, link.LinkID))
	assert.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM import_posting_links WHERE replaces_link_id = ?`, link.LinkID))
	assert.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM audit_events WHERE operation = 'import.posting.reassign' AND reason = 'linked to the wrong transfer'`))
	assert.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM import_posting_links l JOIN import_posting_link_retirements r ON r.link_id = l.id
		WHERE l.id = ? AND r.retired_audit_event_id = (SELECT created_audit_event_id FROM import_posting_links WHERE replaces_link_id = ?)`, link.LinkID, link.LinkID),
		"retirement and successor share one audit event")

	_, third := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID, statement)
	assert.Equal(t, "duplicate", third[0].DedupeStatus, "re-import stays a no-op")
	requireKnownBasisSelfCheckPasses(t, f.database)
}

func TestStatementLinkUnlinkKeepsEvidenceAndTheRowCanBeLinkedAgain(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	ctx := context.Background()
	transfer := f.transfer(t, "2026-06-08", "2026-06-10")
	_, link := f.linkedRow(t, transfer, statementRow{"2026-06-10", "100.00", "Transfer in"}, "2026-06-10")

	require.NoError(t, f.importService.UnlinkStatementRow(ctx, f.correction(link, "not this transfer")))

	shown, err := f.importService.StatementLinks(ctx, transfer.ID)
	require.NoError(t, err)
	require.Len(t, shown, 1, "the unlinked row stays visible where it was linked")
	assert.False(t, shown[0].Active)
	assert.Equal(t, "not this transfer", shown[0].RetireReason)
	assert.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM audit_events WHERE operation = 'import.posting.unlink'`))

	// The posting is free again: a new row may match it.
	batchID, rows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID, statementRow{"2026-06-11", "100.00", "Another credit"})
	assert.NotNil(t, reviewFor(f.preview(t, batchID), rows[0].ID))

	// The unlinked row is linked again from its retired link.
	_, candidates, err := f.importService.StatementLinkCandidates(ctx, link.IdentityID)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	input := f.correction(shown[0], "it was this transfer after all")
	input.TransactionID, input.LineKey, input.PostedOn = transfer.ID, candidates[0].LineKey, candidates[0].PostedOn
	require.NoError(t, f.importService.ReassignStatementRow(ctx, input))
	relinked, err := f.importService.StatementLinks(ctx, transfer.ID)
	require.NoError(t, err)
	require.Len(t, relinked, 1)
	assert.True(t, relinked[0].Active)
	assert.Equal(t, 2, f.linkCount(t))
	assert.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM import_posting_link_retirements`), "relinking an unlinked row retires nothing more")
}

func TestStatementLinkCorrectionRefusesWhatTheOwnerDidNotReview(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	ctx := context.Background()
	first := f.transfer(t, "2026-06-08", "2026-06-10")
	second := f.transfer(t, "2026-06-08", "2026-06-11")
	_, link := f.linkedRow(t, first, statementRow{"2026-06-10", "100.00", "Transfer in"}, "2026-06-10")
	unchanged := func() {
		t.Helper()
		assert.Equal(t, 1, f.linkCount(t))
		assert.Equal(t, 0, f.count(t, `SELECT COUNT(*) FROM import_posting_link_retirements`))
		assert.Equal(t, 0, f.count(t, `SELECT COUNT(*) FROM audit_events WHERE operation IN ('import.posting.unlink', 'import.posting.reassign')`))
	}
	moveTo := func(transaction Transaction, postedOn string) CorrectStatementLinkInput {
		input := f.correction(link, "wrong transfer")
		input.TransactionID, input.LineKey, input.PostedOn = transaction.ID, lineKeyOn(t, transaction, f.savingsAccountID), postedOn
		return input
	}

	stale := f.correction(link, "wrong transfer")
	stale.ExpectedLinkID = link.LinkID + 100
	assert.ErrorIs(t, f.importService.UnlinkStatementRow(ctx, stale), ErrImportStatementLinkStale, "another link than the one reviewed")

	assert.ErrorIs(t, f.importService.ReassignStatementRow(ctx, moveTo(second, "2026-06-12")), ErrImportStatementLinkStale, "not the posting day reviewed")
	assert.ErrorIs(t, f.importService.ReassignStatementRow(ctx, moveTo(first, "2026-06-10")), ErrImportStatementLinkStale, "the line it already links")

	var validation ValidationError
	assert.ErrorAs(t, f.importService.UnlinkStatementRow(ctx, f.correction(link, "  ")), &validation, "a correction states why")
	unchanged()

	// A posting another row links is not offered and cannot be taken.
	_, other := f.linkedRow(t, second, statementRow{"2026-06-11", "100.00", "Second transfer in"}, "2026-06-11")
	_, candidates, err := f.importService.StatementLinkCandidates(ctx, link.IdentityID)
	require.NoError(t, err)
	assert.Empty(t, candidates)
	assert.ErrorIs(t, f.importService.ReassignStatementRow(ctx, moveTo(second, "2026-06-11")), ErrImportStatementLinkStale)
	assert.Equal(t, 2, f.linkCount(t))
	assert.Equal(t, 0, f.count(t, `SELECT COUNT(*) FROM import_posting_link_retirements`))
	assert.True(t, other.Active)

	// A reviewed link that was since unlinked elsewhere refuses a second unlink.
	require.NoError(t, f.importService.UnlinkStatementRow(ctx, f.correction(link, "first unlink")))
	assert.ErrorIs(t, f.importService.UnlinkStatementRow(ctx, f.correction(link, "second unlink")), ErrImportStatementLinkStale)
}

func TestBundleExportsImportPostingLinksWithTheirHistory(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	ctx := context.Background()
	wrong := f.transfer(t, "2026-06-08", "2026-06-09")
	right := f.transfer(t, "2026-06-09", "2026-06-11")
	_, link := f.linkedRow(t, wrong, statementRow{"2026-06-10", "100.00", "Savings top-up"}, "2026-06-09")
	input := f.correction(link, "linked to the wrong transfer")
	input.TransactionID, input.LineKey, input.PostedOn = right.ID, lineKeyOn(t, right, f.savingsAccountID), "2026-06-11"
	require.NoError(t, f.importService.ReassignStatementRow(ctx, input))

	var out bytes.Buffer
	require.NoError(t, NewExportService(db.NewExportRepository(f.database)).WriteBundle(ctx, &out, ExportFilter{}))
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	var rows [][]string
	for _, entry := range archive.File {
		if entry.Name != "import-posting-links.csv" {
			continue
		}
		reader, err := entry.Open()
		require.NoError(t, err)
		raw, err := io.ReadAll(reader)
		require.NoError(t, err)
		rows, err = csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), "\xef\xbb\xbf"))).ReadAll()
		require.NoError(t, err)
	}
	require.Len(t, rows, 3, "header, the retired link and its successor")
	assert.Equal(t, []string{"link_id", "identity_id", "transaction_id", "posting_line_key", "replaces_link_id", "linked_at", "audit_event_id", "retired_at", "retired_audit_event_id", "retire_reason"}, rows[0])
	identity := strconv.FormatInt(link.IdentityID, 10)
	first, successor := rows[1], rows[2]
	assert.Equal(t, []string{identity, strconv.FormatInt(wrong.ID, 10), lineKeyOn(t, wrong, f.savingsAccountID), ""}, first[1:5])
	assert.Equal(t, "linked to the wrong transfer", first[9])
	assert.NotEmpty(t, first[7])
	assert.Equal(t, []string{identity, strconv.FormatInt(right.ID, 10), lineKeyOn(t, right, f.savingsAccountID), first[0]}, successor[1:5])
	assert.Empty(t, successor[7], "the successor is active")
	assert.Equal(t, first[8], successor[6], "one audit event retires and appends")
}
