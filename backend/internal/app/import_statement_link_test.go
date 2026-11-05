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
	"time"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/migrations"
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
	active := link.Active
	return CorrectStatementLinkInput{OwnerUserID: f.ownerUserID, IdentityID: link.IdentityID, ExpectedLinkID: link.LinkID, ExpectedLinkActive: &active, Reason: reason}
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
	// The recovery scenario: the wrong transfer is voided, then the link moves.
	_, err := f.transactionService.VoidTransaction(ctx, VoidTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: wrong.ID, ChangeReason: "entered twice"})
	require.NoError(t, err)
	input := f.correction(link, "linked to the wrong transfer")
	input.TransactionID, input.LineKey, input.PostedOn = right.ID, lineKeyOn(t, right, f.savingsAccountID), "2026-06-11"
	require.NoError(t, f.importService.ReassignStatementRow(ctx, input))

	var out bytes.Buffer
	require.NoError(t, NewExportService(db.NewExportRepository(f.database)).WriteBundle(ctx, &out, ExportFilter{}))
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	table := func(name string) [][]string {
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
	rows := table("import-posting-links.csv")
	require.Len(t, rows, 3, "header, the retired link and its successor")
	assert.Equal(t, []string{"link_id", "identity_id", "transaction_id", "posting_line_key", "replaces_link_id", "linked_at", "audit_event_id",
		"retired_at", "retired_audit_event_id", "retire_reason", "target_status", "target_deleted", "linked_transaction_version_id",
		"linked_account_id", "linked_commodity_id", "linked_quantity_value", "linked_quantity_scale", "linked_posted_on", "linked_evidence"}, rows[0])
	identity := strconv.FormatInt(link.IdentityID, 10)
	first, successor := rows[1], rows[2]
	assert.Equal(t, []string{identity, strconv.FormatInt(wrong.ID, 10), lineKeyOn(t, wrong, f.savingsAccountID), ""}, first[1:5])
	assert.Equal(t, "linked to the wrong transfer", first[9])
	assert.NotEmpty(t, first[7])
	assert.Equal(t, []string{identity, strconv.FormatInt(right.ID, 10), lineKeyOn(t, right, f.savingsAccountID), first[0]}, successor[1:5])
	assert.Empty(t, successor[7], "the successor is active")
	assert.Equal(t, first[8], successor[6], "one audit event retires and appends")

	// The voided target is not in ledger.csv; the link says so and keeps its evidence.
	savings, eur := strconv.FormatInt(f.savingsAccountID, 10), strconv.FormatInt(f.eurCommodityID, 10)
	assert.Equal(t, []string{"voided", "0"}, first[10:12])
	assert.Equal(t, strconv.FormatInt(wrong.VersionID, 10), first[12], "the version the owner linked")
	assert.Equal(t, []string{savings, eur, "10000", "2", "2026-06-09", "recorded"}, first[13:19])
	assert.Equal(t, []string{"posted", "0"}, successor[10:12])
	assert.Equal(t, []string{savings, eur, "10000", "2", "2026-06-11", "recorded"}, successor[13:19])

	ledger := table("ledger.csv")
	column := map[string]int{}
	for i, name := range ledger[0] {
		column[name] = i
	}
	inLedger := func(transactionID, lineKey string) bool {
		for _, row := range ledger[1:] {
			if row[column["transaction_id"]] == transactionID && row[column["posting_line_key"]] == lineKey {
				return true
			}
		}
		return false
	}
	assert.False(t, inLedger(first[2], first[3]), "a voided target is outside ledger.csv")
	assert.True(t, inLedger(successor[2], successor[3]), "a posted target joins its ledger.csv row")
}

// Second review: unlinking keeps the link ID, so a reassignment reviewed
// while the link was active must still be refused after another tab
// unlinks it; relinking needs a review of the row as unlinked.
func TestStatementLinkReassignmentRefusesAfterAnInterveningUnlink(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	ctx := context.Background()
	wrong := f.transfer(t, "2026-06-08", "2026-06-09")
	right := f.transfer(t, "2026-06-09", "2026-06-11")
	_, reviewed := f.linkedRow(t, wrong, statementRow{"2026-06-10", "100.00", "Savings top-up"}, "2026-06-09")

	// Tab B unlinks after tab A loaded the active link.
	require.NoError(t, f.importService.UnlinkStatementRow(ctx, f.correction(reviewed, "tab B")))

	staleMove := f.correction(reviewed, "tab A")
	staleMove.TransactionID, staleMove.LineKey, staleMove.PostedOn = right.ID, lineKeyOn(t, right, f.savingsAccountID), "2026-06-11"
	assert.ErrorIs(t, f.importService.ReassignStatementRow(ctx, staleMove), ErrImportStatementLinkStale)
	assert.Equal(t, 1, f.linkCount(t), "no successor appended")
	assert.Equal(t, 0, f.count(t, `SELECT COUNT(*) FROM audit_events WHERE operation = 'import.posting.reassign'`))

	missing := staleMove
	missing.ExpectedLinkActive = nil
	var validation ValidationError
	assert.ErrorAs(t, f.importService.ReassignStatementRow(ctx, missing), &validation, "the reviewed state is required")

	// Reviewed again as unlinked, the row is linked again.
	links, err := f.importService.StatementLinks(ctx, wrong.ID)
	require.NoError(t, err)
	require.Len(t, links, 1)
	require.False(t, links[0].Active)
	relink := f.correction(links[0], "it was the top-up")
	relink.TransactionID, relink.LineKey, relink.PostedOn = staleMove.TransactionID, staleMove.LineKey, staleMove.PostedOn
	require.NoError(t, f.importService.ReassignStatementRow(ctx, relink))
	assert.Equal(t, 2, f.linkCount(t))

	// And an unlink request that reviewed the row as unlinked is refused.
	moved, err := f.importService.StatementLinks(ctx, right.ID)
	require.NoError(t, err)
	unlinkedReview := f.correction(moved[0], "confused")
	inactive := false
	unlinkedReview.ExpectedLinkActive = &inactive
	assert.ErrorIs(t, f.importService.UnlinkStatementRow(ctx, unlinkedReview), ErrImportStatementLinkStale)
}

// Third review: migration 0006 backfills the evidence of links written
// before it. Timestamps tie within a second, so an edit committed in the same
// second as the link would be taken for the version linked. The backfill
// follows audit event order instead (the reviewer's probe).
func TestImportPostingLinkEvidenceBackfillFollowsAuditOrderNotTimestamps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newImportMatchFixture(t)
	second := func() time.Time { return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC) }
	f.transactionService.SetNowForTest(second)
	f.importService.SetNowForTest(second)
	original := f.transfer(t, "2026-06-10", "2026-06-10")
	_, link := f.linkedRow(t, original, statementRow{"2026-06-10", "100.00", "Top up"}, "2026-06-10")

	// Back to the schema before evidence, then an amount edit in the same second.
	provider, err := goose.NewProvider(goose.DialectSQLite3, f.database, migrations.FS)
	require.NoError(t, err)
	_, err = provider.DownTo(ctx, 5)
	require.NoError(t, err)
	posting := func(accountID int64, cents int64) PostingInput {
		return PostingInput{LineKey: lineKeyOn(t, original, accountID), AccountID: accountID, PostedOn: "2026-06-10",
			CommodityID: f.eurCommodityID, QuantityValue: exact.New(cents), QuantityScale: 2}
	}
	edited, err := f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: original.ID, ChangeReason: "amount corrected after link",
		Spec: TransactionInput{TransactionKind: "transfer", TransactionDate: "2026-06-10", Description: "To savings",
			JournalEntries: []JournalEntryInput{{Postings: []PostingInput{posting(f.cashAccountID, -20000), posting(f.savingsAccountID, 20000)}}}},
	})
	require.NoError(t, err)
	require.NotEqual(t, original.VersionID, edited.VersionID)
	require.NoError(t, db.Migrate(ctx, f.database))

	var amount string
	var version int64
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT pv.quantity_value, pv.transaction_version_id
		FROM import_posting_links l JOIN posting_versions pv ON pv.id = l.linked_posting_version_id WHERE l.id = ?`, link.LinkID).Scan(&amount, &version))
	assert.Equal(t, "10000", amount, "the link evidences 100.00, not the later 200.00 edit")
	assert.Equal(t, original.VersionID, version)
}
