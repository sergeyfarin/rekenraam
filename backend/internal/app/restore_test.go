package app

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/internal/lockfile"
	"rekenraam/backend/internal/secretbox"
)

// scaledTotal folds coefficients in Go, because they are strings in SQLite and
// summing them there would be a float in disguise.
type scaledTotal struct{ value *exact.ScaledInt }

func newScaledTotal() *scaledTotal { return &scaledTotal{value: exact.NewScaledInt()} }

func (t *scaledTotal) add(test *testing.T, coefficient string, scale int) {
	test.Helper()
	parsed, err := exact.Parse(coefficient)
	require.NoError(test, err)
	t.value.AddCoefficient(parsed, scale)
}

func (t *scaledTotal) String() string {
	return exact.DecimalFromBig(t.value.BigInt(), t.value.Scale())
}

func formatKey(accountID int64, commodityID int64) string {
	return strconv.FormatInt(accountID, 10) + "/" + strconv.FormatInt(commodityID, 10)
}

// ledgerFingerprint is what a restored database has to reproduce: the same
// rows, and the same money. Comparing counts alone would pass a database that
// restored the shape of a ledger without its contents.
type ledgerFingerprint struct {
	transactions int64
	postings     int64
	trialBalance map[string]string
}

func fingerprintLedger(t *testing.T, databasePath string) ledgerFingerprint {
	t.Helper()

	database, err := sql.Open("sqlite", "file:"+databasePath+"?mode=ro")
	require.NoError(t, err)
	defer database.Close()

	ctx := context.Background()
	fingerprint := ledgerFingerprint{trialBalance: map[string]string{}}
	require.NoError(t, database.QueryRowContext(ctx, "SELECT COUNT(*) FROM transactions").Scan(&fingerprint.transactions))
	require.NoError(t, database.QueryRowContext(ctx, "SELECT COUNT(*) FROM posting_versions").Scan(&fingerprint.postings))

	// Coefficients are strings, so the balance is folded in Go — summing them
	// in SQL would be a float in disguise.
	rows, err := database.QueryContext(ctx, `
		SELECT account_id, commodity_id, quantity_value, quantity_scale
		FROM posting_versions
		ORDER BY account_id, commodity_id, id
	`)
	require.NoError(t, err)
	defer rows.Close()

	totals := map[string]*scaledTotal{}
	for rows.Next() {
		var accountID, commodityID int64
		var value string
		var scale int
		require.NoError(t, rows.Scan(&accountID, &commodityID, &value, &scale))
		key := formatKey(accountID, commodityID)
		if totals[key] == nil {
			totals[key] = newScaledTotal()
		}
		totals[key].add(t, value, scale)
	}
	require.NoError(t, rows.Err())

	for key, total := range totals {
		fingerprint.trialBalance[key] = total.String()
	}
	return fingerprint
}

// seedRestoreLedger writes a small but real book: two balanced transactions
// through the service layer would need the whole stack, so this writes the rows
// the fingerprint reads, which is what a restore has to bring back.
func seedRestoreLedger(t *testing.T, database *sql.DB) {
	t.Helper()

	ctx := context.Background()
	seedBackupBook(t, database)

	_, err := database.ExecContext(ctx, `
		INSERT INTO commodities (id, book_id, code, kind, is_builtin, created_at, created_by_user_id)
		VALUES (1, 1, 'USD', 'currency', 1, '2026-01-01T00:00:00Z', 1);

		INSERT INTO commodity_versions (
			commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
			status, symbol, display_symbol, name, standard_scale, max_quantity_scale
		) VALUES (1, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'seed', 'active', '$', '$', 'US Dollar', 2, 2);

		INSERT INTO accounts (id, book_id, created_at, created_by_user_id)
		VALUES (1, 1, '2026-01-01T00:00:00Z', 1), (2, 1, '2026-01-01T00:00:00Z', 1);

		INSERT INTO account_versions (
			account_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason,
			status, opened_on, name, account_class, account_kind, allows_postings
		) VALUES
			(1, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'seed', 'active', '2026-01-01', 'Checking', 'asset', 'checking', 1),
			(2, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'seed', 'active', '2026-01-01', 'Salary', 'income', 'income', 1);

		INSERT INTO transactions (id, book_id, created_at, created_by_user_id)
		VALUES (1, 1, '2026-06-01T00:00:00Z', 1);

		INSERT INTO transaction_versions (
			id, book_id, transaction_id, version_seq, status, transaction_kind, transaction_date,
			recorded_at, changed_by_user_id, change_reason
		) VALUES (1, 1, 1, 1, 'posted', 'ordinary', '2026-06-01', '2026-06-01T00:00:00Z', 1, 'seed');

		INSERT INTO journal_entries (id, book_id, transaction_version_id, entry_seq, entry_date, entry_kind)
		VALUES (1, 1, 1, 1, '2026-06-01', 'ordinary');

		INSERT INTO posting_lines (id, book_id, transaction_id, line_key, created_at, created_by_user_id)
		VALUES (1, 1, 1, 'a', '2026-06-01T00:00:00Z', 1), (2, 1, 1, 'b', '2026-06-01T00:00:00Z', 1);

		INSERT INTO posting_versions (
			book_id, transaction_version_id, journal_entry_id, posting_line_id, line_seq,
			account_id, quantity_value, quantity_scale, commodity_id, reconciliation_status
		) VALUES
			(1, 1, 1, 1, 1, 1, '200000', 2, 1, 'uncleared'),
			(1, 1, 1, 2, 2, 2, '-200000', 2, 1, 'uncleared');
	`)
	require.NoError(t, err)
}

// The drill: back a book up, restore it somewhere fresh, and prove the ledger
// came back — not just the row counts, the money.
func TestRestoreDrillMatchesSourceTrialBalance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	sourceURL := "file:" + filepath.Join(root, "source.sqlite")

	source, err := db.Open(ctx, sourceURL)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx, source))
	seedRestoreLedger(t, source)

	readOnly, err := db.OpenReadOnly(ctx, sourceURL)
	require.NoError(t, err)

	backupPath := filepath.Join(root, "backups", "rekenraam-2026-08-24.sqlite")
	_, err = db.OnlineBackupSQLiteDatabase(ctx, readOnly, backupPath, db.OnlineBackupOptions{})
	require.NoError(t, err)
	require.NoError(t, readOnly.Close())

	before := fingerprintLedger(t, filepath.Join(root, "source.sqlite"))
	require.NoError(t, source.Close())

	// A fresh location, as an operator restoring onto a new machine would have.
	restoredURL := "file:" + filepath.Join(root, "restored", "rekenraam.sqlite")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "restored"), 0o700))

	result, err := db.RestoreSQLiteDatabase(ctx, backupPath, restoredURL)
	require.NoError(t, err)
	assert.Positive(t, result.SchemaVersion)

	after := fingerprintLedger(t, filepath.Join(root, "restored", "rekenraam.sqlite"))
	assert.Equal(t, before.transactions, after.transactions)
	assert.Equal(t, before.postings, after.postings)
	assert.Equal(t, before.trialBalance, after.trialBalance, "the restored ledger must hold the same money")

	// And the restored file is usable as a database, not just readable.
	restored, err := db.Open(ctx, restoredURL)
	require.NoError(t, err)
	defer restored.Close()
	require.NoError(t, db.Migrate(ctx, restored))
}

// The case that makes the WAL handling matter: transactions committed but not
// checkpointed at backup time must be in the backup and in the restore.
func TestRestorePreservesUncheckpointedWALContent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	sourceURL := "file:" + filepath.Join(root, "source.sqlite")
	sourcePath := filepath.Join(root, "source.sqlite")

	source, err := db.Open(ctx, sourceURL)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx, source))
	seedRestoreLedger(t, source)

	// Write more, and deliberately do not checkpoint: this content lives only
	// in the WAL.
	_, err = source.ExecContext(ctx, `
		INSERT INTO audit_events (occurred_at, origin_type, operation)
		VALUES ('2026-08-24T04:00:00Z', 'internal', 'uncheckpointed.write')
	`)
	require.NoError(t, err)
	walInfo, err := os.Stat(sourcePath + "-wal")
	require.NoError(t, err)
	require.Positive(t, walInfo.Size(), "the fixture needs uncheckpointed content to be meaningful")

	readOnly, err := db.OpenReadOnly(ctx, sourceURL)
	require.NoError(t, err)
	backupPath := filepath.Join(root, "backups", "rekenraam-2026-08-24.sqlite")
	_, err = db.OnlineBackupSQLiteDatabase(ctx, readOnly, backupPath, db.OnlineBackupOptions{})
	require.NoError(t, err)
	require.NoError(t, readOnly.Close())
	require.NoError(t, source.Close())

	// The backup carries what was only in the WAL.
	backup, err := sql.Open("sqlite", "file:"+backupPath+"?mode=ro")
	require.NoError(t, err)
	var events int64
	require.NoError(t, backup.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE operation = 'uncheckpointed.write'`).Scan(&events))
	require.NoError(t, backup.Close())
	assert.Equal(t, int64(1), events, "the online backup API copies committed WAL content")

	// And restoring over a database whose own WAL holds unflushed work
	// preserves that work rather than discarding it.
	//
	// The earlier version of this test closed the database first, which was the
	// one thing it could not do (T-65): closing a SQLite database checkpoints
	// and deletes its WAL, so the restore found nothing to fold and the
	// scenario in the name never happened. What a crash actually leaves is a
	// file *set* with an uncheckpointed WAL and no process holding it, so this
	// builds exactly that — copying an open database's files, which is wrong as
	// a backup and right as a simulation of a machine losing power.
	target, err := db.Open(ctx, sourceURL)
	require.NoError(t, err)
	_, err = target.ExecContext(ctx, `
		INSERT INTO audit_events (occurred_at, origin_type, operation)
		VALUES ('2026-08-24T05:00:00Z', 'internal', 'about.to.be.replaced')
	`)
	require.NoError(t, err)

	crashed := filepath.Join(root, "crashed")
	require.NoError(t, os.MkdirAll(crashed, 0o700))
	for _, suffix := range []string{"", "-wal", "-shm"} {
		source, err := os.ReadFile(sourcePath + suffix)
		if err != nil {
			continue
		}
		require.NoError(t, os.WriteFile(filepath.Join(crashed, "rekenraam.sqlite"+suffix), source, 0o600))
	}
	require.NoError(t, target.Close())

	crashedWAL, err := os.Stat(filepath.Join(crashed, "rekenraam.sqlite-wal"))
	require.NoError(t, err, "the simulation needs a WAL to exist at restore time")
	require.Positive(t, crashedWAL.Size(), "and it needs content in it")

	crashedURL := "file:" + filepath.Join(crashed, "rekenraam.sqlite")
	result, err := db.RestoreSQLiteDatabase(ctx, backupPath, crashedURL)
	require.NoError(t, err)
	require.NotEmpty(t, result.PreservedDir)

	// The preserved *main file alone* must carry what lived only in the WAL.
	// Reading the preserved set would prove nothing — the WAL travels with it —
	// so the main file is copied away from its sidecars first. This is what
	// fails if the checkpoint before preservation is ever dropped.
	foldedPath := filepath.Join(root, "folded.sqlite")
	foldedBytes, err := os.ReadFile(filepath.Join(result.PreservedDir, "rekenraam.sqlite"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(foldedPath, foldedBytes, 0o600))

	folded, err := sql.Open("sqlite", "file:"+foldedPath+"?mode=ro")
	require.NoError(t, err)
	defer folded.Close()
	var replaced int64
	require.NoError(t, folded.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE operation = 'about.to.be.replaced'`).Scan(&replaced))
	assert.Equal(t, int64(1), replaced,
		"the checkpoint before preservation must fold the WAL into the file it preserves")

	// And the restore itself installed the backup, which predates that write.
	restored, err := sql.Open("sqlite", "file:"+filepath.Join(crashed, "rekenraam.sqlite")+"?mode=ro")
	require.NoError(t, err)
	defer restored.Close()
	var afterRestore int64
	require.NoError(t, restored.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE operation = 'about.to.be.replaced'`).Scan(&afterRestore))
	assert.Equal(t, int64(0), afterRestore, "the restored database is the backup, not the replaced one")
}

func TestRestoreRefusesWhileServeHoldsTheLock(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	databasePath := filepath.Join(root, "rekenraam.sqlite")
	require.NoError(t, os.WriteFile(databasePath, []byte("x"), 0o600))

	held, err := lockfile.Acquire(databasePath)
	require.NoError(t, err)
	defer held.Close()

	err = lockfile.CheckAvailable(databasePath)
	require.Error(t, err, "a running server must be detectable")
	var locked lockfile.ErrLocked
	require.ErrorAs(t, err, &locked)
	assert.Equal(t, os.Getpid(), locked.PID, "and the holder is named so a human can look for it")

	// Released, the same check passes — the lock says "in use now", not
	// "was in use once".
	require.NoError(t, held.Close())
	assert.NoError(t, lockfile.CheckAvailable(databasePath))
}

func TestRestoreRefusesSourceEqualsDestination(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	databaseURL := "file:" + filepath.Join(root, "rekenraam.sqlite")

	database, err := db.Open(ctx, databaseURL)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx, database))
	require.NoError(t, database.Close())

	_, err = db.RestoreSQLiteDatabase(ctx, filepath.Join(root, "rekenraam.sqlite"), databaseURL)
	require.ErrorIs(t, err, db.ErrRestoreSourceIsDestination,
		"restoring a file over itself would move it aside and then look for it")

	// A symlink to the same file is the same file.
	linkPath := filepath.Join(root, "same-database.sqlite")
	require.NoError(t, os.Symlink(filepath.Join(root, "rekenraam.sqlite"), linkPath))
	_, err = db.RestoreSQLiteDatabase(ctx, linkPath, databaseURL)
	require.ErrorIs(t, err, db.ErrRestoreSourceIsDestination)
}

func TestRestoreRefusesNewerSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	backupPath := filepath.Join(root, "future.sqlite")

	// A backup written by a build with migrations this one does not have.
	future, err := sql.Open("sqlite", "file:"+backupPath)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx, future))
	_, err = future.ExecContext(ctx, `
		INSERT INTO goose_db_version (version_id, is_applied, tstamp)
		VALUES (99999, 1, '2027-01-01 00:00:00')
	`)
	require.NoError(t, err)
	require.NoError(t, future.Close())

	_, err = db.RestoreSQLiteDatabase(ctx, backupPath, "file:"+filepath.Join(root, "target.sqlite"))
	require.ErrorIs(t, err, db.ErrRestoreSchemaNewer,
		"a newer schema must be refused rather than half-migrated by an older build")
}

// The key is not in the backup. What a restore must guarantee is that sealed
// data is still readable when the operator kept it — and unreadable, loudly,
// when they did not.
func TestSealedDataDecryptsAfterRestoreWithRetainedKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	sourceURL := "file:" + filepath.Join(root, "source.sqlite")

	source, err := db.Open(ctx, sourceURL)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx, source))
	seedRestoreLedger(t, source)

	key := []byte("0123456789abcdef0123456789abcdef")
	box, err := secretbox.New(key)
	require.NoError(t, err)
	sealed, err := box.Seal([]byte("JBSWY3DPEHPK3PXP"))
	require.NoError(t, err)

	_, err = source.ExecContext(ctx, `
		INSERT INTO user_mfa_totp (user_id, secret_ciphertext, status, created_at)
		VALUES (1, ?, 'active', '2026-01-01T00:00:00Z')
	`, sealed)
	require.NoError(t, err)

	readOnly, err := db.OpenReadOnly(ctx, sourceURL)
	require.NoError(t, err)
	backupPath := filepath.Join(root, "backups", "rekenraam-2026-08-24.sqlite")
	_, err = db.OnlineBackupSQLiteDatabase(ctx, readOnly, backupPath, db.OnlineBackupOptions{})
	require.NoError(t, err)
	require.NoError(t, readOnly.Close())
	require.NoError(t, source.Close())

	inspection, err := db.InspectBackup(ctx, backupPath)
	require.NoError(t, err)
	assert.Equal(t, int64(1), inspection.SealedRowCounts["user_mfa_totp"],
		"verify-backup must be able to say how much sealed data is at stake")

	samples, err := db.SealedSamples(ctx, backupPath)
	require.NoError(t, err)
	require.Contains(t, samples, "user_mfa_totp")

	// With the retained key: readable.
	plaintext, err := box.Open(samples["user_mfa_totp"])
	require.NoError(t, err)
	assert.Equal(t, "JBSWY3DPEHPK3PXP", string(plaintext))

	// With a different key: it fails, and it fails as an error rather than as
	// silently wrong data.
	otherBox, err := secretbox.New([]byte("fedcba9876543210fedcba9876543210"))
	require.NoError(t, err)
	_, err = otherBox.Open(samples["user_mfa_totp"])
	require.Error(t, err, "a lost key must be loud, not silently empty")
}
