package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"

	"rekenraam/backend/internal/config"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/lockfile"
	"rekenraam/backend/internal/secretbox"
)

func runVerifyBackup(ctx context.Context, cfg config.Config, args []string, stdout io.Writer, stderr io.Writer) int {
	flagSet := flag.NewFlagSet("verify-backup", flag.ContinueOnError)
	flagSet.SetOutput(stderr)

	var backupPath string
	flagSet.StringVar(&backupPath, "from", "", "path to the backup file to verify")
	if err := flagSet.Parse(args); err != nil {
		return 2
	}
	if backupPath == "" {
		fmt.Fprintln(stderr, "verify-backup requires --from")
		return 2
	}

	inspection, err := db.InspectBackup(ctx, backupPath)
	if err != nil {
		fmt.Fprintf(stderr, "backup is not usable: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "backup:          %s\n", inspection.Path)
	fmt.Fprintf(stdout, "size:            %d bytes\n", inspection.ByteSize)
	fmt.Fprintf(stdout, "integrity:       ok (integrity_check and foreign_key_check)\n")
	fmt.Fprintf(stdout, "schema version:  %d (this build knows %d)\n", inspection.SchemaVersion, inspection.BinaryVersion)
	if inspection.SchemaVersion > inspection.BinaryVersion {
		fmt.Fprintln(stdout, "                 this backup is NEWER than this build and cannot be restored by it")
	}

	for _, table := range sortedKeys(inspection.RowCounts) {
		fmt.Fprintf(stdout, "rows %-18s %d\n", table+":", inspection.RowCounts[table])
	}

	reportSealedData(ctx, cfg, inspection, stdout)

	return 0
}

// reportSealedData answers the question a restore only raises once it is too
// late: will the sealed parts of this database still be readable afterwards?
//
// The key lives outside the backup by design, so this is a check against the
// key the *current environment* holds — which is exactly the one a restore
// would run with.
func reportSealedData(ctx context.Context, cfg config.Config, inspection db.BackupInspection, stdout io.Writer) {
	var sealedTotal int64
	for _, count := range inspection.SealedRowCounts {
		sealedTotal += count
	}
	if sealedTotal == 0 {
		fmt.Fprintln(stdout, "sealed data:     none (no multi-factor enrolment or connection credentials)")
		return
	}

	for _, table := range sortedKeys(inspection.SealedRowCounts) {
		if inspection.SealedRowCounts[table] > 0 {
			fmt.Fprintf(stdout, "sealed rows %-11s %d\n", table+":", inspection.SealedRowCounts[table])
		}
	}

	if len(cfg.SecretKey) == 0 {
		fmt.Fprintln(stdout, "sealed data:     REKENRAAM_SECRET_KEY is not set here, so this cannot be checked.")
		fmt.Fprintln(stdout, "                 Restoring without the original key leaves the ledger intact and")
		fmt.Fprintln(stdout, "                 these rows unreadable: multi-factor enrolment and connection")
		fmt.Fprintln(stdout, "                 credentials would have to be set up again.")
		return
	}

	box, err := secretbox.New(cfg.SecretKey)
	if err != nil {
		fmt.Fprintf(stdout, "sealed data:     configured key is unusable: %v\n", err)
		return
	}

	samples, err := db.SealedSamples(ctx, inspection.Path)
	if err != nil {
		fmt.Fprintf(stdout, "sealed data:     could not read a sample: %v\n", err)
		return
	}

	failed := false
	for _, table := range sortedKeys(samples) {
		if _, err := box.Open(samples[table]); err != nil {
			fmt.Fprintf(stdout, "sealed data:     %s does NOT decrypt with the configured key\n", table)
			failed = true
		}
	}
	if !failed {
		fmt.Fprintln(stdout, "sealed data:     decrypts with the configured REKENRAAM_SECRET_KEY")
	}
}

func runRestore(ctx context.Context, cfg config.Config, args []string, stdout io.Writer, stderr io.Writer) int {
	flagSet := flag.NewFlagSet("restore", flag.ContinueOnError)
	flagSet.SetOutput(stderr)

	var backupPath string
	flagSet.StringVar(&backupPath, "from", "", "path to the verified backup to restore")
	if err := flagSet.Parse(args); err != nil {
		return 2
	}
	if backupPath == "" {
		fmt.Fprintln(stderr, "restore requires --from")
		return 2
	}

	databasePath, err := db.ResolveSQLiteFilePath(cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(stderr, "resolve database path: %v\n", err)
		return 2
	}

	// Proof, not inference: the lock a running server holds for its whole life.
	// An idle connection or an absent -wal would prove nothing.
	if err := lockfile.CheckAvailable(databasePath); err != nil {
		var locked lockfile.ErrLocked
		if errors.As(err, &locked) {
			fmt.Fprintf(stderr, "the server is still running (%v). Stop it before restoring.\n", locked)
			return 1
		}
		fmt.Fprintf(stderr, "cannot confirm the server is stopped: %v\n", err)
		return 1
	}

	result, err := db.RestoreSQLiteDatabase(ctx, backupPath, cfg.DatabaseURL)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrRestoreSourceIsDestination):
			fmt.Fprintln(stderr, "the backup and the database are the same file; nothing to restore")
		case errors.Is(err, db.ErrRestoreSchemaNewer):
			fmt.Fprintf(stderr, "%v — upgrade the app before restoring this backup\n", err)
		default:
			fmt.Fprintf(stderr, "restore failed: %v\n", err)
		}
		// Which sentence is true here depends on how far the restore got, and
		// the operator is deciding what to do next with no database in front of
		// them. Saying "preserved" for a run that moved nothing, or telling
		// someone to move files back over a restore that already succeeded,
		// is worse than saying nothing.
		switch {
		case result.Installed:
			fmt.Fprintf(stderr, "the restored database is already installed at %s; this failed afterwards.\n", result.DatabasePath)
			fmt.Fprintf(stderr, "the previous database is in %s — keep it until the restored one looks right.\n", result.PreservedDir)
		case len(result.PreservedFiles) > 0:
			fmt.Fprintf(stderr, "nothing is at %s now: the previous database was moved to %s first.\n", result.DatabasePath, result.PreservedDir)
			fmt.Fprintln(stderr, "to go back to it, move those files beside the database path under their original names.")
		default:
			fmt.Fprintln(stderr, "nothing was replaced; the database is untouched.")
		}
		return 1
	}

	fmt.Fprintf(stdout, "restored:        %s\n", result.DatabasePath)
	fmt.Fprintf(stdout, "schema version:  %d\n", result.SchemaVersion)
	if result.PreservedDir != "" {
		fmt.Fprintf(stdout, "previous data:   %s (kept — delete it once the restored app looks right)\n", result.PreservedDir)
	}
	fmt.Fprintln(stdout, "attachments:     nothing to restore (attachment storage is not implemented yet)")
	if len(cfg.SecretKey) == 0 {
		fmt.Fprintln(stdout, "secret key:      REKENRAAM_SECRET_KEY is not set. Set the ORIGINAL key before starting,")
		fmt.Fprintln(stdout, "                 or multi-factor enrolment and connection credentials will be unreadable.")
	} else {
		fmt.Fprintln(stdout, "secret key:      set — it must be the same key this backup was written under.")
	}

	return 0
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
