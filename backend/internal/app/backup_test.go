package app

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
)

type backupHarness struct {
	service     *BackupService
	repository  *db.BackupRepository
	writer      *sql.DB
	readOnly    *sql.DB
	directory   string
	databaseURL string
	now         time.Time
}

func (h *backupHarness) advance(d time.Duration) { h.now = h.now.Add(d) }

func newBackupHarness(t *testing.T) *backupHarness {
	t.Helper()

	ctx := context.Background()
	root := t.TempDir()
	databaseURL := "file:" + filepath.Join(root, "rekenraam.sqlite")

	writer, err := db.Open(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, writer.Close()) })
	require.NoError(t, db.Migrate(ctx, writer))

	seedBackupBook(t, writer)

	readOnly, err := db.OpenReadOnly(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readOnly.Close()) })

	directory := filepath.Join(root, "backups")
	repository := db.NewBackupRepository(writer)
	service := NewBackupService(repository, db.NewBackgroundWorkRepository(writer), readOnly, databaseURL, directory)

	harness := &backupHarness{
		service:     service,
		repository:  repository,
		writer:      writer,
		readOnly:    readOnly,
		directory:   directory,
		databaseURL: databaseURL,
		now:         time.Date(2026, 8, 24, 4, 0, 0, 0, time.UTC),
	}
	service.SetNowForTest(func() time.Time { return harness.now })

	return harness
}

// seedBackupBook creates the minimum a backup needs to exist: a book with an
// owner, since a run is attributed to one.
func seedBackupBook(t *testing.T, database *sql.DB) {
	t.Helper()

	ctx := context.Background()
	_, err := database.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'x', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`)
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, `
		INSERT INTO books (id, code, name, owner_user_id, created_at, updated_at, updated_by_user_id)
		VALUES (1, 'personal', 'Personal', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 1)
	`)
	require.NoError(t, err)
}

func (h *backupHarness) runManualBackup(t *testing.T) db.BackupRunRecord {
	t.Helper()

	run, err := h.service.RequestBackup(context.Background(), 1)
	require.NoError(t, err)
	require.NoError(t, h.service.RunBackup(context.Background(), run.ID))

	completed, err := h.repository.BackupRunByID(context.Background(), run.ID)
	require.NoError(t, err)
	return completed
}

// "Backed up nightly and provably balanced" is one sentence, so the check runs
// where the backup does rather than waiting for someone to press a button.
func TestSuccessfulBackupChainsTheSelfCheck(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	ctx := context.Background()

	readOnly, err := db.OpenReadOnly(ctx, harness.databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readOnly.Close()) })

	selfCheck := NewSelfCheckService(db.NewSelfCheckRepository(harness.writer, readOnly))
	selfCheck.SetNowForTest(func() time.Time { return harness.now })
	harness.service.SetSelfCheck(selfCheck)

	_, hasRun, err := selfCheck.LatestSelfCheck(ctx)
	require.NoError(t, err)
	require.False(t, hasRun)

	harness.runManualBackup(t)

	run, hasRun, err := selfCheck.LatestSelfCheck(ctx)
	require.NoError(t, err)
	require.True(t, hasRun, "a successful backup must leave a check behind it")
	assert.Equal(t, "scheduled", run.Trigger)
	assert.Equal(t, SelfCheckPassed, run.Status)
}

// The copy uses SQLite's online backup API, which ADR 0004 names as the
// preferred in-app mechanism — and it produces a database that opens, verifies,
// and holds the same data.
func TestBackupUsesOnlineBackupAPI(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	run := harness.runManualBackup(t)

	assert.Equal(t, "completed", run.Status)
	assert.True(t, run.Verified)
	require.True(t, run.ByteSize.Valid)
	assert.Positive(t, run.ByteSize.Int64)
	require.True(t, run.PageCount.Valid)
	assert.Positive(t, run.PageCount.Int64, "the online backup API reports the pages it copied")

	info, err := os.Stat(run.TargetPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "a full copy of the ledger is not world-readable")

	// The copy is a real database carrying the source's rows.
	copied, err := sql.Open("sqlite", "file:"+run.TargetPath+"?mode=ro")
	require.NoError(t, err)
	defer copied.Close()
	var books int
	require.NoError(t, copied.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM books").Scan(&books))
	assert.Equal(t, 1, books)
}

// The read pool is the source, so a backup cannot stall a save. Written as a
// behaviour rather than an assertion about wiring: the writer must stay usable
// while a backup runs.
func TestBackupSourcesFromReadOnlyPoolAndDoesNotBlockWrites(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)

	done := make(chan error, 1)
	go func() {
		run, err := harness.service.RequestBackup(context.Background(), 1)
		if err != nil {
			done <- err
			return
		}
		done <- harness.service.RunBackup(context.Background(), run.ID)
	}()

	// A write during the copy must succeed rather than wait for it.
	writeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := harness.writer.ExecContext(writeCtx, `
		INSERT INTO audit_events (occurred_at, origin_type, operation)
		VALUES ('2026-08-24T04:00:00Z', 'internal', 'test.during.backup')
	`)
	require.NoError(t, err, "a backup must not hold the write connection")

	require.NoError(t, <-done)
}

// A .part file is always a failed attempt, so the final name appears only after
// verification passes.
func TestBackupWorkerVerifiesBeforePublishingFinalName(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	run := harness.runManualBackup(t)

	assert.NoError(t, db.VerifySQLiteBackup(context.Background(), run.TargetPath))

	entries, err := os.ReadDir(harness.directory)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.NotContains(t, entry.Name(), ".part", "a partial file must never survive a successful run")
	}
}

// Crash points: the run is re-executed after failing at each of the four
// moments that matter, and none of them may leave a duplicate or an untracked
// file.
func TestBackupRetryAfterCrashLeavesNoDuplicateOrUntrackedFile(t *testing.T) {
	t.Parallel()

	for name, crash := range map[string]func(t *testing.T, h *backupHarness, run db.BackupRunRecord){
		"after creation, before verification": func(t *testing.T, h *backupHarness, run db.BackupRunRecord) {
			// A copy that died mid-write leaves a .part behind.
			require.NoError(t, os.MkdirAll(h.directory, 0o700))
			require.NoError(t, os.WriteFile(run.TargetPath+".part", []byte("half a database"), 0o600))
		},
		"after verification, before rename": func(t *testing.T, h *backupHarness, run db.BackupRunRecord) {
			require.NoError(t, os.MkdirAll(h.directory, 0o700))
			require.NoError(t, os.WriteFile(run.TargetPath+".part", []byte("verified but unpublished"), 0o600))
		},
		"after rename, before the run was recorded": func(t *testing.T, h *backupHarness, run db.BackupRunRecord) {
			// The real file is already in place; a re-run must adopt it rather
			// than write a second one.
			require.NoError(t, h.service.RunBackup(context.Background(), run.ID))
			require.NoError(t, h.repository.UpdateBackupRun(context.Background(), db.UpdateBackupRunParams{
				ID:     run.ID,
				Status: "running",
				Now:    h.now.Format(time.RFC3339),
			}))
		},
		"after the run was recorded": func(t *testing.T, h *backupHarness, run db.BackupRunRecord) {
			require.NoError(t, h.service.RunBackup(context.Background(), run.ID))
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			harness := newBackupHarness(t)
			run, err := harness.service.RequestBackup(context.Background(), 1)
			require.NoError(t, err)

			crash(t, harness, run)

			// The re-run after the crash must succeed and settle the run.
			require.NoError(t, harness.service.RunBackup(context.Background(), run.ID))

			final, err := harness.repository.BackupRunByID(context.Background(), run.ID)
			require.NoError(t, err)
			assert.Equal(t, "completed", final.Status)
			assert.NoError(t, db.VerifySQLiteBackup(context.Background(), final.TargetPath))
			// A run recorded as completed and verified must describe the file
			// it settled on. The adoption path used to report a page count of
			// zero — a verified backup claiming to hold no pages — because it
			// took the figure from a copy that never ran instead of from the
			// file it adopted.
			require.True(t, final.PageCount.Valid, "a settled run must record a page count")
			assert.Positive(t, final.PageCount.Int64, "a verified backup holds pages")
			require.True(t, final.ByteSize.Valid)
			info, err := os.Stat(final.TargetPath)
			require.NoError(t, err)
			assert.Equal(t, info.Size(), final.ByteSize.Int64, "the recorded size is the file's")

			entries, err := os.ReadDir(harness.directory)
			require.NoError(t, err)
			var backups []string
			for _, entry := range entries {
				assert.NotContains(t, entry.Name(), ".part", "no partial file may survive")
				backups = append(backups, entry.Name())
			}
			assert.Len(t, backups, 1, "a retry writes the same path, it does not scatter duplicates: %v", backups)
		})
	}
}

// Asking for the same scheduled night twice is one backup, because the work
// queue's own uniqueness stops covering an occurrence once it completes.
func TestScheduledOccurrenceIsCreatedOnlyOnce(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	ctx := context.Background()

	first, err := harness.service.createRun(ctx, "scheduled", "scheduled:2026-08-24", "2026-08-24", 1)
	require.NoError(t, err)
	require.NoError(t, harness.service.RunBackup(ctx, first.ID))

	_, err = harness.service.createRun(ctx, "scheduled", "scheduled:2026-08-24", "2026-08-24", 1)
	require.ErrorIs(t, err, db.ErrBackupOccurrenceExists,
		"a completed occurrence must not be enqueued again — the queue's uniqueness covers only pending and running items")

	exists, err := harness.repository.ScheduledRunExistsForLocalDate(ctx, BookID, "2026-08-24")
	require.NoError(t, err)
	assert.True(t, exists)
}

// Retention deletes the app's own backups, oldest first, and nothing else.
func TestBackupWorkerPrunesOnlyItsOwnFiles(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	ctx := context.Background()

	_, err := harness.service.SavePolicy(ctx, SaveBackupPolicyInput{
		UserID: 1, Enabled: true, HourLocal: 3, MinuteLocal: 15, RetentionCount: 2,
	})
	require.NoError(t, err)

	// A file the app did not create, sitting in the same directory.
	require.NoError(t, os.MkdirAll(harness.directory, 0o700))
	stranger := filepath.Join(harness.directory, "someone-elses-notes.txt")
	require.NoError(t, os.WriteFile(stranger, []byte("not ours"), 0o600))

	var paths []string
	for range 3 {
		run := harness.runManualBackup(t)
		paths = append(paths, run.TargetPath)
		harness.advance(time.Hour)
	}

	assert.NoFileExists(t, paths[0], "the oldest backup beyond the retention count is pruned")
	assert.FileExists(t, paths[1])
	assert.FileExists(t, paths[2])
	assert.FileExists(t, stranger, "a file the app never recorded is not its to delete")
}

// A name and a database row are not authority to unlink a path.
func TestPruneRefusesSymlinkAndPathOutsideBackupDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	directory := filepath.Join(root, "backups")
	require.NoError(t, os.MkdirAll(directory, 0o700))

	// Something precious, elsewhere.
	outside := filepath.Join(root, "precious.sqlite")
	require.NoError(t, os.WriteFile(outside, []byte("do not delete"), 0o600))

	// A symlink planted in the backup directory, named exactly as the app names
	// its own files.
	planted := filepath.Join(directory, "rekenraam-2026-01-01.sqlite")
	require.NoError(t, os.Symlink(outside, planted))

	err := removeBackupFile(directory, planted)
	require.ErrorIs(t, err, ErrBackupDirectoryUnsafe)
	assert.FileExists(t, outside, "the symlink target must survive")
	assert.FileExists(t, planted, "and the link itself is left for a human to look at")

	// A path outside the directory is refused even with the right name.
	elsewhere := filepath.Join(root, "rekenraam-2026-01-02.sqlite")
	require.NoError(t, os.WriteFile(elsewhere, []byte("also not ours"), 0o600))
	require.ErrorIs(t, removeBackupFile(directory, elsewhere), ErrBackupDirectoryUnsafe)
	assert.FileExists(t, elsewhere)

	// A file that does not match the app's naming is refused too.
	foreign := filepath.Join(directory, "notes.txt")
	require.NoError(t, os.WriteFile(foreign, []byte("keep"), 0o600))
	require.ErrorIs(t, removeBackupFile(directory, foreign), ErrBackupDirectoryUnsafe)
	assert.FileExists(t, foreign)
}

// A full disk is not an invalid request: the same work succeeds once space
// returns, so it must not be terminal.
func TestDiskShortageIsRetryableAndRecovers(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	ctx := context.Background()

	// A directory that cannot be written to stands in for a full disk: the
	// preflight has no way to fabricate free space, but the copy's failure
	// takes the same path.
	require.NoError(t, os.MkdirAll(harness.directory, 0o500))
	t.Cleanup(func() { _ = os.Chmod(harness.directory, 0o700) })

	run, err := harness.service.RequestBackup(ctx, 1)
	require.NoError(t, err)
	err = harness.service.RunBackup(ctx, run.ID)
	require.Error(t, err, "an unwritable destination must fail")

	failed, err := harness.repository.BackupRunByID(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", failed.Status)
	assert.NotEmpty(t, failed.ErrorSummary)
	assert.Equal(t, 1, failed.Attempts)

	// Space returns; the identical work now succeeds without being re-created.
	require.NoError(t, os.Chmod(harness.directory, 0o700))
	require.NoError(t, harness.service.RunBackup(ctx, run.ID))

	recovered, err := harness.repository.BackupRunByID(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", recovered.Status)
	assert.True(t, recovered.Verified)
	assert.Equal(t, 2, recovered.Attempts, "the retry is the same run, not a new one")
}

// A copy that cannot complete never publishes a final-named file.
//
// A true ENOSPC cannot be injected portably, so the blocked destination stands
// in for it: what matters is the property, which is that the only path to the
// final name runs through a completed, verified copy.
func TestBlockedCopyNeverPublishesAFinalNamedBackup(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	ctx := context.Background()

	run, err := harness.service.RequestBackup(ctx, 1)
	require.NoError(t, err)

	// A non-empty directory where the partial file belongs: it cannot be
	// removed and cannot be written through.
	blocked := run.TargetPath + ".part"
	require.NoError(t, os.MkdirAll(blocked, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(blocked, "occupied"), []byte("x"), 0o600))

	require.Error(t, harness.service.RunBackup(ctx, run.ID))

	assert.NoFileExists(t, run.TargetPath, "no final-named file may appear without a verified copy behind it")

	failed, err := harness.repository.BackupRunByID(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", failed.Status)
	assert.NotEmpty(t, failed.ErrorSummary, "and the failure is recorded where the screen can show it")
}

// The policy is a promise made to a person in their own time zone.
func TestNextBackupRunFollowsTheOwnersLocalTime(t *testing.T) {
	t.Parallel()

	policy := BackupPolicy{Enabled: true, HourLocal: 3, MinuteLocal: 15, TimeZone: "Europe/Amsterdam"}

	// 02:00 Amsterdam on a summer day is 00:00 UTC; the next run is later the
	// same local day.
	next := nextBackupRun(time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), policy)
	assert.Equal(t, "2026-08-24T03:15:00+02:00", next.Format(time.RFC3339))

	// Past today's time, the next one is tomorrow.
	next = nextBackupRun(time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC), policy)
	assert.Equal(t, "2026-08-25T03:15:00+02:00", next.Format(time.RFC3339))
}

func TestBackupPolicyValidatesItsBounds(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	ctx := context.Background()

	for name, input := range map[string]SaveBackupPolicyInput{
		"hour out of range":   {UserID: 1, HourLocal: 24, MinuteLocal: 0, RetentionCount: 1},
		"minute out of range": {UserID: 1, HourLocal: 3, MinuteLocal: 60, RetentionCount: 1},
		"retention below one": {UserID: 1, HourLocal: 3, MinuteLocal: 15, RetentionCount: 0},
	} {
		_, err := harness.service.SavePolicy(ctx, input)
		require.Errorf(t, err, "%s must be rejected", name)
	}

	saved, err := harness.service.SavePolicy(ctx, SaveBackupPolicyInput{
		UserID: 1, Enabled: false, HourLocal: 22, MinuteLocal: 30, RetentionCount: 7,
	})
	require.NoError(t, err)
	assert.False(t, saved.Enabled)
	assert.Equal(t, 22, saved.HourLocal)
	assert.Equal(t, 7, saved.RetentionCount)

	// A book that never configured one still has a policy the scheduler runs on.
	fresh := newBackupHarness(t)
	defaults, err := fresh.service.Policy(ctx)
	require.NoError(t, err)
	assert.True(t, defaults.Enabled)
	assert.Equal(t, 3, defaults.HourLocal)
	assert.Equal(t, 14, defaults.RetentionCount)
}

// --- T-66: the schedule and the queue path around a backup ---
//
// RunBackup was tested directly, which left the code that decides *when* a
// backup happens, and the queue transitions around it, entirely unexercised —
// the part that makes "backed up nightly" true rather than aspirational.

// The scheduler waits for the policy's local time, then arranges exactly one
// backup for that day.
func TestSchedulerArrangesOneBackupPerLocalDay(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := harness.service.SavePolicy(ctx, SaveBackupPolicyInput{
		UserID: 1, Enabled: true, HourLocal: 3, MinuteLocal: 15, RetentionCount: 14,
	})
	require.NoError(t, err)

	// 02:00 UTC, before the 03:15 the policy asks for.
	harness.now = time.Date(2026, 8, 24, 2, 0, 0, 0, time.UTC)
	harness.service.scheduleBackupIfDue(ctx, logger)
	runs, err := harness.repository.ListBackupRuns(ctx, BookID, 10)
	require.NoError(t, err)
	assert.Empty(t, runs, "nothing is due before the policy's time")

	// 03:20 — due.
	harness.now = time.Date(2026, 8, 24, 3, 20, 0, 0, time.UTC)
	harness.service.scheduleBackupIfDue(ctx, logger)
	runs, err = harness.repository.ListBackupRuns(ctx, BookID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, "scheduled", runs[0].Trigger)
	assert.Equal(t, "2026-08-24", runs[0].ScheduledForLocalDate.String)

	// Every later tick that day is a no-op: the scheduler ticks once a minute,
	// and a second run for one night would be a second backup nobody asked for.
	for _, minute := range []int{21, 30, 59} {
		harness.now = time.Date(2026, 8, 24, 3, minute, 0, 0, time.UTC)
		harness.service.scheduleBackupIfDue(ctx, logger)
	}
	runs, err = harness.repository.ListBackupRuns(ctx, BookID, 10)
	require.NoError(t, err)
	assert.Len(t, runs, 1, "one occurrence per local day, however often the ticker fires")

	// The next day is a new occurrence.
	harness.now = time.Date(2026, 8, 25, 3, 20, 0, 0, time.UTC)
	harness.service.scheduleBackupIfDue(ctx, logger)
	runs, err = harness.repository.ListBackupRuns(ctx, BookID, 10)
	require.NoError(t, err)
	assert.Len(t, runs, 2)
}

// A disabled policy schedules nothing, however late it gets.
func TestSchedulerRespectsADisabledPolicy(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := harness.service.SavePolicy(ctx, SaveBackupPolicyInput{
		UserID: 1, Enabled: false, HourLocal: 3, MinuteLocal: 15, RetentionCount: 14,
	})
	require.NoError(t, err)

	harness.now = time.Date(2026, 8, 24, 23, 59, 0, 0, time.UTC)
	harness.service.scheduleBackupIfDue(ctx, logger)

	runs, err := harness.repository.ListBackupRuns(ctx, BookID, 10)
	require.NoError(t, err)
	assert.Empty(t, runs)
}

// "Nightly at 03:15" is a promise made to a person, so it follows their clock
// through a DST change rather than drifting an hour twice a year.
func TestSchedulerFollowsTheOwnersClockAcrossDST(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := harness.writer.ExecContext(ctx, `
		INSERT INTO user_preferences (user_id, time_zone, created_at, created_by_user_id, updated_at, updated_by_user_id)
		VALUES (1, 'Europe/Amsterdam', '2026-01-01T00:00:00Z', 1, '2026-01-01T00:00:00Z', 1)
	`)
	require.NoError(t, err)
	_, err = harness.service.SavePolicy(ctx, SaveBackupPolicyInput{
		UserID: 1, Enabled: true, HourLocal: 3, MinuteLocal: 15, RetentionCount: 14,
	})
	require.NoError(t, err)

	// Amsterdam is UTC+2 in summer: 02:00 UTC is 04:00 local, which is past
	// 03:15 local — due, even though it is not yet 03:15 UTC.
	harness.now = time.Date(2026, 8, 24, 2, 0, 0, 0, time.UTC)
	harness.service.scheduleBackupIfDue(ctx, logger)

	runs, err := harness.repository.ListBackupRuns(ctx, BookID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1, "the policy's time is local, not UTC")
	assert.Equal(t, "2026-08-24", runs[0].ScheduledForLocalDate.String)

	// In winter Amsterdam is UTC+1, so the same local time is a different UTC
	// instant — and 02:00 UTC on that day is 03:00 local, which is *not* due.
	harness.now = time.Date(2026, 12, 24, 2, 0, 0, 0, time.UTC)
	harness.service.scheduleBackupIfDue(ctx, logger)
	runs, err = harness.repository.ListBackupRuns(ctx, BookID, 10)
	require.NoError(t, err)
	assert.Len(t, runs, 1, "03:00 local is before 03:15 local, whatever the offset is")

	harness.now = time.Date(2026, 12, 24, 2, 30, 0, 0, time.UTC)
	harness.service.scheduleBackupIfDue(ctx, logger)
	runs, err = harness.repository.ListBackupRuns(ctx, BookID, 10)
	require.NoError(t, err)
	assert.Len(t, runs, 2, "03:30 local is past it")
}

// The queue path: a claimed item runs the backup and is completed, so the same
// occurrence is not handed out again.
func TestWorkerCompletesAClaimedBackup(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	work := db.NewBackgroundWorkRepository(harness.writer)

	run, err := harness.service.RequestBackup(ctx, 1)
	require.NoError(t, err)

	harness.service.runDueBackups(ctx, logger, "worker-1")

	completed, err := harness.repository.BackupRunByID(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", completed.Status)
	assert.True(t, completed.Verified)
	assert.FileExists(t, completed.TargetPath)

	item, err := work.BackgroundWorkByID(ctx, completed.WorkItemID.Int64)
	require.NoError(t, err)
	assert.Equal(t, "completed", item.Status, "a finished backup must not be claimable again")

	// A second sweep finds nothing to do.
	harness.service.runDueBackups(ctx, logger, "worker-1")
	runs, err := harness.repository.ListBackupRuns(ctx, BookID, 10)
	require.NoError(t, err)
	assert.Len(t, runs, 1)
}

// A failing backup is retried with backoff until the cap, and then given up on
// — the T-39 shape, which is only safe because RetryBackupRun exists as the way
// back.
func TestWorkerRetriesToTheCapAndThenStops(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	work := db.NewBackgroundWorkRepository(harness.writer)

	// An unwritable backup directory: every attempt fails the same way, which
	// is what a full disk looks like to this worker.
	require.NoError(t, os.MkdirAll(harness.directory, 0o500))
	t.Cleanup(func() { _ = os.Chmod(harness.directory, 0o700) })

	run, err := harness.service.RequestBackup(ctx, 1)
	require.NoError(t, err)

	for attempt := 1; attempt <= maxBackupAttempts+1; attempt++ {
		// Move past the backoff each round, so the item is due again.
		harness.advance(2 * time.Hour)
		harness.service.runDueBackups(ctx, logger, "worker-1")
	}

	item, err := work.BackgroundWorkByID(ctx, run.WorkItemID.Int64)
	require.NoError(t, err)
	assert.Equal(t, "failed", item.Status, "a bounded retry must stop, not loop forever")
	assert.Equal(t, maxBackupAttempts, item.Attempts)

	failed, err := harness.repository.BackupRunByID(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", failed.Status)
	assert.NotEmpty(t, failed.ErrorSummary, "and it must say why")

	// The way back: space returns, the owner retries, and the same run
	// succeeds without being recreated.
	require.NoError(t, os.Chmod(harness.directory, 0o700))
	_, err = harness.service.RetryBackupRun(ctx, run.ID)
	require.NoError(t, err)

	harness.advance(time.Minute)
	harness.service.runDueBackups(ctx, logger, "worker-1")

	recovered, err := harness.repository.BackupRunByID(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", recovered.Status)
	assert.FileExists(t, recovered.TargetPath)
}

// A payload naming no run can never succeed, so it fails immediately rather
// than occupying five attempts to reach the same conclusion.
func TestWorkerFailsAnUnusablePayloadWithoutRetrying(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	work := db.NewBackgroundWorkRepository(harness.writer)

	item, err := work.EnqueueBackgroundWork(ctx, BookID, BackupWorkKind, `{"run_id":0}`,
		harness.now.Format(time.RFC3339))
	require.NoError(t, err)

	harness.service.runDueBackups(ctx, logger, "worker-1")

	failed, err := work.BackgroundWorkByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", failed.Status)
	assert.Equal(t, 1, failed.Attempts, "an unusable payload is terminal on the first attempt")
	assert.Contains(t, failed.LastError.String, "invalid backup work payload")
}

// The retry guards: a cap is only safe with a way back, and a way back is only
// safe if it refuses the cases that would duplicate work.
func TestRetryBackupRunRefusesWhatCannotOrNeedNotBeRetried(t *testing.T) {
	t.Parallel()

	harness := newBackupHarness(t)
	ctx := context.Background()

	completed := harness.runManualBackup(t)
	_, err := harness.service.RetryBackupRun(ctx, completed.ID)
	require.Error(t, err, "a completed backup has nothing to retry")
	assert.Contains(t, err.Error(), "already completed")

	// A run that is still queued: retrying would be a second copy of work the
	// queue is already going to do.
	queued, err := harness.service.RequestBackup(ctx, 1)
	require.NoError(t, err)
	_, err = harness.service.RetryBackupRun(ctx, queued.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already queued")

	_, err = harness.service.RetryBackupRun(ctx, 987654)
	require.ErrorIs(t, err, db.ErrNotFound)
}

// Pruning and the self-check are independent consequences of a verified copy.
// Chaining them meant a backup directory that could not be tidied silently
// skipped the ledger integrity check, while the API went on reporting a plain
// success — "backed up nightly and provably balanced" quietly losing its second
// half, with nothing anywhere saying so.
func TestPruningFailureStillLeavesASelfCheckBehind(t *testing.T) {
	harness := newBackupHarness(t)
	ctx := context.Background()

	readOnly, err := db.OpenReadOnly(ctx, harness.databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readOnly.Close()) })

	selfCheck := NewSelfCheckService(db.NewSelfCheckRepository(harness.writer, readOnly))
	selfCheck.SetNowForTest(func() time.Time { return harness.now })

	// One completed backup, then a retention of 1, so the *next* run has
	// something to prune.
	first := harness.runManualBackup(t)
	require.Equal(t, "completed", first.Status)
	_, err = harness.service.SavePolicy(ctx, SaveBackupPolicyInput{
		UserID: 1, Enabled: true, HourLocal: 3, MinuteLocal: 15, RetentionCount: 1,
	})
	require.NoError(t, err)

	// The second run adopts a file already in place, so it needs no write of
	// its own — which lets the backup directory be read-only for the whole
	// call. Publishing succeeds; unlinking the older backup cannot.
	harness.service.SetSelfCheck(selfCheck)
	harness.now = harness.now.Add(24 * time.Hour)
	second, err := harness.service.RequestBackup(ctx, 1)
	require.NoError(t, err)
	_, err = db.OnlineBackupSQLiteDatabase(ctx, harness.readOnly, second.TargetPath, db.OnlineBackupOptions{})
	require.NoError(t, err)

	require.NoError(t, os.Chmod(harness.directory, 0o500))
	t.Cleanup(func() { _ = os.Chmod(harness.directory, 0o700) })

	require.NoError(t, harness.service.RunBackup(ctx, second.ID))

	settled, err := harness.repository.BackupRunByID(ctx, second.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", settled.Status, "a copy that exists and verifies is still a backup")

	// The point of the test: the prune failed, and the check ran anyway.
	assert.FileExists(t, first.TargetPath, "the prune must really have been unable to unlink")
	run, hasRun, err := selfCheck.LatestSelfCheck(ctx)
	require.NoError(t, err)
	require.True(t, hasRun, "an untidy backup directory is no reason to skip the integrity check")
	assert.Equal(t, "scheduled", run.Trigger)
}
