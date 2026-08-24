package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"rekenraam/backend/internal/db"
)

type backupWorkPayload struct {
	RunID int64 `json:"run_id"`
}

func (s *BackupService) StartBackgroundWorker(ctx context.Context, logger *slog.Logger) {
	if s.backgroundWork == nil || s.readOnly == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	workerID := uuid.NewString()
	go func() {
		s.runDueBackups(ctx, logger, workerID)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runDueBackups(ctx, logger, workerID)
			}
		}
	}()
}

func (s *BackupService) runDueBackups(ctx context.Context, logger *slog.Logger, workerID string) {
	for range 4 {
		now := s.now().UTC()
		item, err := s.backgroundWork.ClaimBackgroundWork(ctx, BackupWorkKind, workerID,
			now.Format(time.RFC3339), now.Add(30*time.Minute).Format(time.RFC3339))
		if errors.Is(err, db.ErrNotFound) || ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.WarnContext(ctx, "claim backup work", slog.Any("err", err))
			return
		}
		s.processBackupWork(ctx, logger, workerID, item)
	}
}

func (s *BackupService) processBackupWork(ctx context.Context, logger *slog.Logger, workerID string, item db.BackgroundWorkItemRecord) {
	var payload backupWorkPayload
	if json.Unmarshal([]byte(item.PayloadJSON), &payload) != nil || payload.RunID <= 0 {
		// A payload that cannot name a run can never succeed; retrying it would
		// only postpone the same conclusion.
		if err := s.backgroundWork.FailBackgroundWork(ctx, item.ID, workerID, s.now().UTC().Format(time.RFC3339), "invalid backup work payload"); err != nil {
			logger.WarnContext(ctx, "fail invalid backup work", slog.Int64("work_id", item.ID), slog.Any("err", err))
		}
		return
	}

	err := s.RunBackup(ctx, payload.RunID)
	now := s.now().UTC().Format(time.RFC3339)
	if err == nil {
		if completeErr := s.backgroundWork.CompleteBackgroundWork(ctx, item.ID, workerID, now); completeErr != nil {
			logger.WarnContext(ctx, "complete backup work", slog.Int64("work_id", item.ID), slog.Any("err", completeErr))
		}
		return
	}
	if ctx.Err() != nil {
		return
	}

	// Everything a backup hits is retryable except a payload that names no run:
	// a full disk, a busy database, a copy that outran its deadline — an
	// operator freeing space or a quiet minute makes the identical work
	// succeed. What stops it is the attempt cap, not a judgement about the
	// error.
	logger.WarnContext(ctx, "backup attempt failed",
		slog.Int64("work_id", item.ID), slog.Int64("run_id", payload.RunID), slog.Any("err", err))

	if item.Attempts >= maxBackupAttempts {
		if failErr := s.backgroundWork.FailBackgroundWork(ctx, item.ID, workerID, now, backupErrorSummary(err)); failErr != nil {
			logger.WarnContext(ctx, "fail backup work", slog.Int64("work_id", item.ID), slog.Any("err", failErr))
		}
		return
	}

	backoff := time.Duration(1<<uint(item.Attempts)) * time.Minute
	if backoff > time.Hour {
		backoff = time.Hour
	}
	availableAt := s.now().UTC().Add(backoff).Format(time.RFC3339)
	if retryErr := s.backgroundWork.RetryBackgroundWork(ctx, item.ID, workerID, availableAt, now, backupErrorSummary(err)); retryErr != nil {
		logger.WarnContext(ctx, "retry backup work", slog.Int64("work_id", item.ID), slog.Any("err", retryErr))
	}
}

// RunBackup executes one recorded backup occurrence.
//
// It is idempotent by design, because at-least-once delivery means a crash at
// any point is followed by a re-run:
//
//   - a run already completed is left alone;
//   - a final file that exists and verifies is adopted rather than rewritten,
//     which is the crash-after-rename case;
//   - a partial file is deleted, which is the crash-during-copy case;
//   - the run record is written last, so a crash before it leaves a file that
//     the next attempt adopts instead of a duplicate.
func (s *BackupService) RunBackup(ctx context.Context, runID int64) error {
	run, err := s.repository.BackupRunByID(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status == "completed" {
		return nil
	}

	now := s.now().UTC().Format(time.RFC3339)
	if err := s.repository.UpdateBackupRun(ctx, db.UpdateBackupRunParams{
		ID:           run.ID,
		Status:       "running",
		StartedAt:    now,
		Now:          now,
		BumpAttempts: true,
	}); err != nil {
		return err
	}

	result, err := s.produceBackup(ctx, run)
	if err != nil {
		failedAt := s.now().UTC().Format(time.RFC3339)
		if updateErr := s.repository.UpdateBackupRun(ctx, db.UpdateBackupRunParams{
			ID:           run.ID,
			Status:       "failed",
			ErrorSummary: backupErrorSummary(err),
			Now:          failedAt,
		}); updateErr != nil {
			return fmt.Errorf("%w (and recording the failure failed: %v)", err, updateErr)
		}
		return err
	}

	finishedAt := s.now().UTC().Format(time.RFC3339)
	pageCount := int64(result.PageCount)
	byteSize := result.ByteSize
	if err := s.repository.UpdateBackupRun(ctx, db.UpdateBackupRunParams{
		ID:         run.ID,
		Status:     "completed",
		Verified:   true,
		ByteSize:   &byteSize,
		PageCount:  &pageCount,
		FinishedAt: finishedAt,
		Now:        finishedAt,
	}); err != nil {
		return err
	}

	// Pruning failing must not fail the backup: the copy exists and is
	// verified, which is the part that matters.
	if err := s.pruneBackups(ctx); err != nil {
		return nil
	}

	// "Backed up nightly and provably balanced" is one sentence, so the check
	// runs where the backup does rather than waiting for someone to press a
	// button. Its failure is not the backup's: a book that does not balance
	// still deserves the copy that was just made of it.
	if s.selfCheck != nil {
		if _, err := s.selfCheck.RunSelfCheck(ctx, "scheduled"); err != nil {
			return nil
		}
	}
	return nil
}

// produceBackup does the file work: reconcile what a previous attempt left,
// copy, verify, then publish under the final name.
func (s *BackupService) produceBackup(ctx context.Context, run db.BackupRunRecord) (db.OnlineBackupResult, error) {
	directory := filepath.Dir(run.TargetPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return db.OnlineBackupResult{}, fmt.Errorf("create backup directory: %w", err)
	}

	partialPath := run.TargetPath + backupPartialMark

	// Crash after rename, before the run was recorded: the file is already
	// there and correct. Verify before adopting it — an unverified file is not
	// a backup, however plausible its name.
	if info, err := os.Stat(run.TargetPath); err == nil {
		if verifyErr := db.VerifySQLiteBackup(ctx, run.TargetPath); verifyErr == nil {
			return db.OnlineBackupResult{ByteSize: info.Size()}, nil
		}
		if removeErr := os.Remove(run.TargetPath); removeErr != nil {
			return db.OnlineBackupResult{}, fmt.Errorf("remove unverifiable backup: %w", removeErr)
		}
	}

	// Crash during a copy: a .part file is always a failed attempt and never a
	// backup, so it is removed rather than resumed — sidecars included, since a
	// half-written WAL is exactly the thing that must not be adopted later.
	if err := removePartialBackup(partialPath); err != nil {
		return db.OnlineBackupResult{}, err
	}

	databasePath, err := db.ResolveSQLiteFilePath(s.databaseURL)
	if err != nil {
		return db.OnlineBackupResult{}, err
	}
	if err := preflightBackupSpace(directory, databasePath); err != nil {
		return db.OnlineBackupResult{}, err
	}

	result, err := db.OnlineBackupSQLiteDatabase(ctx, s.readOnly, partialPath, db.OnlineBackupOptions{})
	if err != nil {
		// The copy cleans up its own destination; anything left is not a
		// backup and must not survive under a name that suggests otherwise.
		_ = removePartialBackup(partialPath)
		return db.OnlineBackupResult{}, err
	}

	if err := db.VerifySQLiteBackup(ctx, partialPath); err != nil {
		_ = removePartialBackup(partialPath)
		return db.OnlineBackupResult{}, fmt.Errorf("verify backup: %w", err)
	}

	// Durable before it is visible: fsync the copy, then rename, then fsync the
	// directory that now names it. Without the last step a crash can lose the
	// rename and leave nothing at all.
	if err := db.SyncFileAndParent(partialPath); err != nil {
		_ = removePartialBackup(partialPath)
		return db.OnlineBackupResult{}, err
	}
	if err := os.Rename(partialPath, run.TargetPath); err != nil {
		_ = removePartialBackup(partialPath)
		return db.OnlineBackupResult{}, fmt.Errorf("publish backup: %w", err)
	}
	if err := db.SyncDirectory(directory); err != nil {
		return db.OnlineBackupResult{}, err
	}

	return result, nil
}

// removePartialBackup clears a failed attempt and anything SQLite left beside
// it. Renaming only the main file would leave orphaned sidecars in the backup
// directory, which is how a directory of backups becomes a directory of
// wreckage nobody dares delete.
func removePartialBackup(partialPath string) error {
	for _, path := range []string{partialPath, partialPath + "-wal", partialPath + "-shm"} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove partial backup: %w", err)
		}
	}
	return nil
}

// pruneBackups deletes backups the policy no longer keeps.
//
// Three conditions must all hold before anything is unlinked: the path is
// recorded in backup_runs, it matches the app's own name pattern, and — checked
// at delete time — it resolves through symlinks to a regular file still inside
// the backup directory. A name and a database row are not authority to unlink a
// path: a symlink planted in the backup directory would otherwise redirect a
// delete anywhere this process can reach.
func (s *BackupService) pruneBackups(ctx context.Context) error {
	policy, err := s.Policy(ctx)
	if err != nil {
		return err
	}
	directory, err := s.BackupDirectory()
	if err != nil {
		return err
	}

	runs, err := s.repository.PrunableBackupRuns(ctx, BookID)
	if err != nil {
		return err
	}

	keep := policy.RetentionCount
	cutoff := ""
	if policy.RetentionMaxAgeDays != nil {
		cutoff = s.now().UTC().AddDate(0, 0, -int(*policy.RetentionMaxAgeDays)).Format(time.RFC3339)
	}

	// Oldest first: everything beyond the keep count, plus anything older than
	// the age cap when one is set.
	excess := len(runs) - keep
	for index, run := range runs {
		tooMany := index < excess
		tooOld := cutoff != "" && run.CreatedAt < cutoff
		if !tooMany && !tooOld {
			continue
		}

		if err := removeBackupFile(directory, run.TargetPath); err != nil {
			if errors.Is(err, ErrBackupDirectoryUnsafe) {
				// Refuse and keep the row: a path that no longer resolves
				// inside the backup directory is not this app's to delete, and
				// silently forgetting it would hide that.
				continue
			}
			return err
		}
		if err := s.repository.MarkBackupRunPruned(ctx, run.ID, s.now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}

	return nil
}

func removeBackupFile(directory string, path string) error {
	if !isAppBackupFile(filepath.Base(path)) {
		return fmt.Errorf("%w: %s does not match the backup naming pattern", ErrBackupDirectoryUnsafe, filepath.Base(path))
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Already gone: nothing to delete, and the run can be marked
			// pruned so it is not reconsidered forever.
			return nil
		}
		return fmt.Errorf("resolve backup path: %w", err)
	}

	resolvedDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return fmt.Errorf("resolve backup directory: %w", err)
	}

	relative, err := filepath.Rel(resolvedDirectory, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("%w: %s", ErrBackupDirectoryUnsafe, path)
	}

	info, err := os.Lstat(resolved)
	if err != nil {
		return fmt.Errorf("stat backup for removal: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrBackupDirectoryUnsafe, path)
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove backup: %w", err)
	}
	return nil
}

// RetryBackupRun puts a failed run back on the queue. A bounded attempt cap is
// only safe with a way back, and this is it.
func (s *BackupService) RetryBackupRun(ctx context.Context, runID int64) (db.BackupRunRecord, error) {
	run, err := s.repository.BackupRunByID(ctx, runID)
	if err != nil {
		return db.BackupRunRecord{}, err
	}
	if run.Status == "completed" {
		return db.BackupRunRecord{}, ValidationError{Message: "this backup already completed"}
	}
	if !run.WorkItemID.Valid {
		return db.BackupRunRecord{}, ValidationError{Message: "this backup has no work item to retry"}
	}

	now := s.now().UTC().Format(time.RFC3339)
	if _, err := s.backgroundWork.RequeueBackgroundWork(ctx, BookID, run.WorkItemID.Int64, now, now); err != nil {
		if errors.Is(err, db.ErrBackgroundWorkAlreadyActive) {
			return db.BackupRunRecord{}, ValidationError{Message: "this backup is already queued"}
		}
		return db.BackupRunRecord{}, err
	}

	if err := s.repository.UpdateBackupRun(ctx, db.UpdateBackupRunParams{
		ID:     run.ID,
		Status: "pending",
		Now:    now,
	}); err != nil {
		return db.BackupRunRecord{}, err
	}

	return s.repository.BackupRunByID(ctx, runID)
}

// backupErrorSummary keeps the queue's error column readable and free of
// anything financial — the payload rule from ADR 0010 applies to error text as
// much as to payloads.
func backupErrorSummary(err error) string {
	summary := err.Error()
	if len(summary) > 300 {
		summary = summary[:300]
	}
	return summary
}
