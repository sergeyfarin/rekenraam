package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"rekenraam/backend/internal/db"
)

// BackupWorkKind is the durable work queue kind processed by
// BackupService.StartBackgroundWorker.
const BackupWorkKind = "maintenance.backup"

// maxBackupAttempts bounds retries. Every worker in this codebase needs a cap
// (T-39 was the one that did not have one and retried at the backoff ceiling
// forever), and a cap is only safe with a way back — hence RetryBackupRun.
const maxBackupAttempts = 5

// backupFreeSpaceFactor is how much room the preflight wants relative to the
// database: the copy is roughly the same size, plus room for the destination's
// own journal. It is advisory — space can vanish mid-copy, so ENOSPC is handled
// on its own terms rather than assumed away.
const backupFreeSpaceFactor = 1.2

// ErrBackupSpaceUnavailable means the backup filesystem does not have room.
// It is retryable: nothing about the request is wrong, and an operator freeing
// space makes the identical work succeed.
var ErrBackupSpaceUnavailable = errors.New("not enough free space on the backup filesystem")

// ErrBackupDirectoryUnsafe means a path pruning was asked to delete does not
// resolve to a regular file inside the backup directory.
var ErrBackupDirectoryUnsafe = errors.New("backup path does not resolve inside the backup directory")

// backupFilePrefix and backupFileSuffix are the app's own naming. Pruning
// requires this *and* a recorded run *and* a resolved path inside the backup
// directory before it will unlink anything.
const (
	backupFilePrefix  = "rekenraam-"
	backupFileSuffix  = ".sqlite"
	backupPartialMark = ".part"
)

type BackupService struct {
	repository     *db.BackupRepository
	backgroundWork *db.BackgroundWorkRepository
	// readOnly is the source of the copy: the second pool, so a nightly backup
	// of a large book never holds the single write connection.
	readOnly    *sql.DB
	databaseURL string
	backupDir   string
	// selfCheck runs after a successful backup, so "backed up nightly and
	// provably balanced" is one nightly fact rather than two features that
	// happen to exist.
	selfCheck *SelfCheckService
	now       func() time.Time
}

// BackupPolicy is the schedule and retention the owner controls.
type BackupPolicy struct {
	Enabled             bool
	HourLocal           int
	MinuteLocal         int
	RetentionCount      int
	RetentionMaxAgeDays *int64
	TimeZone            string
	UpdatedAt           string
}

// DefaultBackupPolicy is what a book that has never configured one gets:
// nightly at 03:15 local, keeping a fortnight. Fourteen dailies is what covers
// "I noticed a week later" without thinking about disk.
func DefaultBackupPolicy() BackupPolicy {
	return BackupPolicy{
		Enabled:        true,
		HourLocal:      3,
		MinuteLocal:    15,
		RetentionCount: 14,
	}
}

func NewBackupService(repository *db.BackupRepository, backgroundWork *db.BackgroundWorkRepository, readOnly *sql.DB, databaseURL string, backupDir string) *BackupService {
	return &BackupService{
		repository:     repository,
		backgroundWork: backgroundWork,
		readOnly:       readOnly,
		databaseURL:    databaseURL,
		backupDir:      strings.TrimSpace(backupDir),
		now:            time.Now,
	}
}

func (s *BackupService) SetNowForTest(now func() time.Time) { s.now = now }

// SetSelfCheck chains the ledger self-check onto every successful backup.
func (s *BackupService) SetSelfCheck(selfCheck *SelfCheckService) { s.selfCheck = selfCheck }

// BackupDirectory is where backups land: BACKUP_DIR, or a directory beside the
// database when unset. The deployment docs recommend a different device, which
// only an operator can arrange — the default is convenient, not safe against
// disk loss, and the docs say so.
func (s *BackupService) BackupDirectory() (string, error) {
	if s.backupDir != "" {
		return filepath.Clean(s.backupDir), nil
	}
	databasePath, err := db.ResolveSQLiteFilePath(s.databaseURL)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(databasePath), "backups"), nil
}

func (s *BackupService) Policy(ctx context.Context) (BackupPolicy, error) {
	policy := DefaultBackupPolicy()

	record, err := s.repository.BackupPolicy(ctx, BookID)
	switch {
	case err == nil:
		policy.Enabled = record.Enabled
		policy.HourLocal = record.HourLocal
		policy.MinuteLocal = record.MinuteLocal
		policy.RetentionCount = record.RetentionCount
		if record.RetentionMaxAgeDays.Valid {
			maxAge := record.RetentionMaxAgeDays.Int64
			policy.RetentionMaxAgeDays = &maxAge
		}
		policy.UpdatedAt = record.UpdatedAt.String
	case errors.Is(err, db.ErrNotFound):
		// Never configured: the defaults above, which is also what the
		// scheduler runs on.
	default:
		return BackupPolicy{}, err
	}

	timeZone, err := s.repository.BookOwnerTimeZone(ctx, BookID)
	if err != nil {
		return BackupPolicy{}, err
	}
	policy.TimeZone = timeZone

	return policy, nil
}

type SaveBackupPolicyInput struct {
	UserID              int64
	Enabled             bool
	HourLocal           int
	MinuteLocal         int
	RetentionCount      int
	RetentionMaxAgeDays *int64
}

func (s *BackupService) SavePolicy(ctx context.Context, input SaveBackupPolicyInput) (BackupPolicy, error) {
	if input.HourLocal < 0 || input.HourLocal > 23 {
		return BackupPolicy{}, ValidationError{Message: "backup hour must be between 0 and 23"}
	}
	if input.MinuteLocal < 0 || input.MinuteLocal > 59 {
		return BackupPolicy{}, ValidationError{Message: "backup minute must be between 0 and 59"}
	}
	if input.RetentionCount < 1 {
		return BackupPolicy{}, ValidationError{Message: "retention must keep at least one backup"}
	}
	if input.RetentionMaxAgeDays != nil && *input.RetentionMaxAgeDays < 1 {
		return BackupPolicy{}, ValidationError{Message: "retention age must be at least one day"}
	}

	if _, err := s.repository.SaveBackupPolicy(ctx, db.SaveBackupPolicyParams{
		BookID:              BookID,
		Enabled:             input.Enabled,
		HourLocal:           input.HourLocal,
		MinuteLocal:         input.MinuteLocal,
		RetentionCount:      input.RetentionCount,
		RetentionMaxAgeDays: input.RetentionMaxAgeDays,
		UpdatedByUserID:     input.UserID,
		Now:                 s.now().UTC().Format(time.RFC3339),
	}); err != nil {
		return BackupPolicy{}, err
	}

	return s.Policy(ctx)
}

// RequestBackup enqueues a backup the user asked for now.
//
// Asking twice is a reasonable thing to do, so a manual run gets its own
// occurrence identity rather than colliding with the day's scheduled one.
func (s *BackupService) RequestBackup(ctx context.Context, userID int64) (db.BackupRunRecord, error) {
	occurrence := "manual:" + uuid.NewString()
	return s.createRun(ctx, "manual", occurrence, "", userID)
}

func (s *BackupService) createRun(ctx context.Context, trigger string, occurrenceKey string, localDate string, userID int64) (db.BackupRunRecord, error) {
	directory, err := s.BackupDirectory()
	if err != nil {
		return db.BackupRunRecord{}, err
	}

	run, _, err := s.repository.CreateBackupRunWithWork(ctx, db.CreateBackupRunParams{
		BookID:                BookID,
		Trigger:               trigger,
		OccurrenceKey:         occurrenceKey,
		TargetPath:            filepath.Join(directory, backupFilename(trigger, occurrenceKey, localDate, s.now().UTC())),
		ScheduledForLocalDate: localDate,
		RequestedByUserID:     userID,
		WorkKind:              BackupWorkKind,
		Now:                   s.now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return db.BackupRunRecord{}, err
	}

	return run, nil
}

// backupFilename is derived from the occurrence, not from the clock at
// execution time: a retry must write to the same path rather than scatter
// near-duplicates that pruning then has to reason about.
func backupFilename(trigger string, occurrenceKey string, localDate string, now time.Time) string {
	if trigger == "scheduled" && localDate != "" {
		return backupFilePrefix + localDate + backupFileSuffix
	}
	suffix := strings.TrimPrefix(occurrenceKey, "manual:")
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	return backupFilePrefix + "manual-" + now.Format("20060102T150405Z") + "-" + suffix + backupFileSuffix
}

// isAppBackupFile reports whether a name follows the app's own pattern. One of
// three conditions pruning requires; on its own it is not authority to delete
// anything.
func isAppBackupFile(name string) bool {
	return strings.HasPrefix(name, backupFilePrefix) && strings.HasSuffix(name, backupFileSuffix)
}

// preflightBackupSpace checks the filesystem that will hold the backup.
//
// Advisory by nature: it is a snapshot of free space taken before a copy that
// takes time, and the space can be gone by the time it matters. It exists to
// turn the common case — a disk that is already full — into a clear, retryable
// failure instead of a half-written file.
func preflightBackupSpace(directory string, databasePath string) error {
	info, err := os.Stat(databasePath)
	if err != nil {
		// No database to size means nothing useful to compare against; the copy
		// itself will report the real problem.
		return nil
	}

	available, known := availableBytes(directory)
	if !known {
		return nil
	}

	needed := int64(float64(info.Size()) * backupFreeSpaceFactor)
	if available < needed {
		return fmt.Errorf("%w: %d bytes free, about %d needed", ErrBackupSpaceUnavailable, available, needed)
	}
	return nil
}

// BackupStatus is the read model the Data screen polls: the policy, where files
// land, when the next run is due, and what recent attempts did.
type BackupStatus struct {
	Policy      BackupPolicy
	Directory   string
	NextRunAt   time.Time
	LastSuccess *db.BackupRunRecord
	Runs        []db.BackupRunRecord
}

func (s *BackupService) Status(ctx context.Context) (BackupStatus, error) {
	policy, err := s.Policy(ctx)
	if err != nil {
		return BackupStatus{}, err
	}
	directory, err := s.BackupDirectory()
	if err != nil {
		return BackupStatus{}, err
	}
	runs, err := s.repository.ListBackupRuns(ctx, BookID, 20)
	if err != nil {
		return BackupStatus{}, err
	}

	status := BackupStatus{Policy: policy, Directory: directory, Runs: runs}
	for index := range runs {
		if runs[index].Status == "completed" {
			status.LastSuccess = &runs[index]
			break
		}
	}
	if policy.Enabled {
		status.NextRunAt = nextBackupRun(s.now().UTC(), policy)
	}

	return status, nil
}

// nextBackupRun is the next local occurrence of the policy's time, expressed as
// an instant. Computed from the owner's zone so the answer survives DST rather
// than drifting an hour twice a year.
func nextBackupRun(now time.Time, policy BackupPolicy) time.Time {
	location, err := time.LoadLocation(policy.TimeZone)
	if err != nil {
		location = time.UTC
	}
	localNow := now.In(location)
	next := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), policy.HourLocal, policy.MinuteLocal, 0, 0, location)
	if !next.After(localNow) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
