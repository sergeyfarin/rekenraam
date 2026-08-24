package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"rekenraam/backend/internal/db"
)

// StartScheduler arranges the nightly backup.
//
// Wall-clock local, following the pricing scheduler rather than the import
// scheduler's "older than N hours": "nightly at 03:15" is a promise made to a
// person, and a person means their own 03:15. The stored local time plus the
// owner's IANA zone is the DST-correct way to keep it; runs are recorded in
// UTC.
func (s *BackupService) StartScheduler(ctx context.Context, logger *slog.Logger) {
	if s.repository == nil || s.backgroundWork == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	go func() {
		s.scheduleBackupIfDue(ctx, logger)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.scheduleBackupIfDue(ctx, logger)
			}
		}
	}()
}

func (s *BackupService) scheduleBackupIfDue(ctx context.Context, logger *slog.Logger) {
	policy, err := s.Policy(ctx)
	if err != nil {
		logger.WarnContext(ctx, "read backup policy", slog.Any("err", err))
		return
	}
	if !policy.Enabled {
		return
	}

	location, err := time.LoadLocation(policy.TimeZone)
	if err != nil {
		logger.WarnContext(ctx, "load backup time zone", slog.String("time_zone", policy.TimeZone), slog.Any("err", err))
		return
	}

	localNow := s.now().UTC().In(location)
	dueAt := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), policy.HourLocal, policy.MinuteLocal, 0, 0, location)
	if localNow.Before(dueAt) {
		return
	}

	localDate := localNow.Format(time.DateOnly)
	exists, err := s.repository.ScheduledRunExistsForLocalDate(ctx, BookID, localDate)
	if err != nil {
		logger.WarnContext(ctx, "read scheduled backup state", slog.Any("err", err))
		return
	}
	if exists {
		return
	}

	ownerID, err := s.repository.CurrentBookOwnerID(ctx, BookID)
	if err != nil {
		if !errors.Is(err, db.ErrNotFound) {
			logger.WarnContext(ctx, "read book owner for scheduled backup", slog.Any("err", err))
		}
		return
	}

	run, err := s.createRun(ctx, "scheduled", "scheduled:"+localDate, localDate, ownerID)
	if err != nil {
		// Losing the race with another process is the normal outcome of two
		// schedulers agreeing on the same night, not a failure worth logging
		// as one.
		if errors.Is(err, db.ErrBackupOccurrenceExists) {
			return
		}
		logger.WarnContext(ctx, "schedule backup", slog.Any("err", err))
		return
	}

	logger.InfoContext(ctx, "scheduled backup enqueued",
		slog.Int64("run_id", run.ID), slog.String("local_date", localDate))
}
