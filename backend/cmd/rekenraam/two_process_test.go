//go:build unix

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/config"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/lockfile"
	"rekenraam/backend/internal/testdb"
)

const processTestEnv = "REKENRAAM_TWO_PROCESS_TEST"
const processNow = "2026-10-14T08:00:00Z"
const processLease = "2026-10-14T08:05:00Z"

type processConfig struct {
	Mode, DatabaseURL, HTTPAddr, BackupPath string
}

type processRequest struct {
	Operation, Now, Lease string
}

type processResult struct {
	PID                   int
	Status                string
	ItemID, RunID         int64
	Attempts, ExitCode    int
	Owner, Stdout, Stderr string
}

// Re-exec the current test binary to retain the same native driver/build tags
// and race instrumentation. Only this helper runs in children. Repository
// probes deliberately bypass the runtime lock: two servers are not supported.
func TestTwoProcessHelper(t *testing.T) {
	encoded := os.Getenv(processTestEnv)
	if encoded == "" {
		t.Skip("subprocess entry point")
	}
	var cfg processConfig
	require.NoError(t, json.Unmarshal([]byte(encoded), &cfg))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out := json.NewEncoder(os.Stdout)
	in := json.NewDecoder(os.Stdin)
	owner := strconv.Itoa(os.Getpid())
	appCfg := config.Config{AppEnv: "development", DatabaseURL: cfg.DatabaseURL,
		HTTPAddr: cfg.HTTPAddr, SessionLifetime: 24 * time.Hour}
	var commandDone chan int
	if cfg.Mode == "serve" {
		commandDone = make(chan int, 1)
		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
		go func() { commandDone <- runServe(ctx, appCfg, logger) }()
		client := &http.Client{Timeout: 100 * time.Millisecond}
		require.Eventually(t, func() bool {
			response, err := client.Get("http://" + cfg.HTTPAddr + "/healthz")
			if err != nil {
				return false
			}
			response.Body.Close()
			return response.StatusCode == http.StatusNoContent
		}, 10*time.Second, 10*time.Millisecond, "server readiness")
	}
	var work *db.BackgroundWorkRepository
	var backups *db.BackupRepository
	var preopened *os.File
	var held *lockfile.Lock
	defer func() {
		if held != nil {
			require.NoError(t, held.Close())
		}
	}()
	if cfg.Mode == "preopened_lock" {
		path, err := db.ResolveSQLiteFilePath(cfg.DatabaseURL)
		require.NoError(t, err)
		preopened, err = os.OpenFile(lockfile.Path(path), os.O_RDWR, 0o600)
		require.NoError(t, err)
		defer preopened.Close()
	}
	if cfg.Mode == "database" {
		database, err := db.Open(ctx, cfg.DatabaseURL)
		require.NoError(t, err)
		defer database.Close()
		work = db.NewBackgroundWorkRepository(database)
		backups = db.NewBackupRepository(database)
	}
	require.NoError(t, out.Encode(processResult{PID: os.Getpid(), Status: "ready"}))
	for {
		var request processRequest
		err := in.Decode(&request)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		result := processResult{PID: os.Getpid(), Status: "ok"}
		switch request.Operation {
		case "claim":
			item, err := work.ClaimBackgroundWork(ctx, "two_process_probe", owner, request.Now, request.Lease)
			switch {
			case errors.Is(err, db.ErrNotFound):
				result.Status = "empty"
			case db.IsSQLiteBusy(err):
				result.Status = "busy"
			default:
				require.NoError(t, err)
				result.ItemID, result.Attempts, result.Owner = item.ID, item.Attempts, item.LeaseOwner.String
			}
		case "occurrence":
			run, item, err := backups.CreateBackupRunWithWork(ctx, db.CreateBackupRunParams{
				BookID: 1, Trigger: "scheduled", OccurrenceKey: "scheduled:2026-10-14",
				ScheduledForLocalDate: "2026-10-14", RequestedByUserID: 1,
				TargetPath: cfg.BackupPath, WorkKind: "two_process_probe", Now: processNow,
			})
			if errors.Is(err, db.ErrBackupOccurrenceExists) {
				result.Status = "duplicate"
			} else {
				require.NoError(t, err)
				result.RunID, result.ItemID = run.ID, item.ID
			}
		case "lock_preopened", "unlock_preopened":
			// Model Acquire paused after OpenFile, before its flock. The OS
			// lock must still share an inode with later production Acquires.
			operation := syscall.LOCK_EX | syscall.LOCK_NB
			if request.Operation == "unlock_preopened" {
				operation = syscall.LOCK_UN
			}
			require.NoError(t, syscall.Flock(int(preopened.Fd()), operation))
		case "acquire_lock":
			path, err := db.ResolveSQLiteFilePath(cfg.DatabaseURL)
			require.NoError(t, err)
			held, err = lockfile.Acquire(path)
			if _, ok := errors.AsType[lockfile.ErrLocked](err); ok {
				result.Status = "locked"
			} else {
				require.NoError(t, err)
			}
		case "release_lock":
			require.NoError(t, held.Close())
			held = nil
		case "restore_paused_before", "restore_paused_after":
			var stdout, stderr bytes.Buffer
			pause := func() {
				require.NoError(t, out.Encode(processResult{PID: os.Getpid(), Status: "paused"}))
				var resume processRequest
				require.NoError(t, in.Decode(&resume))
				require.Equal(t, "resume", resume.Operation)
			}
			result.ExitCode = runRestoreWith(ctx, appCfg, []string{"--from", cfg.BackupPath}, &stdout, &stderr,
				func(ctx context.Context, backupPath, databaseURL string) (db.RestoreResult, error) {
					if request.Operation == "restore_paused_before" {
						pause()
					}
					restored, err := db.RestoreSQLiteDatabase(ctx, backupPath, databaseURL)
					require.NoError(t, err)
					if request.Operation == "restore_paused_after" {
						pause()
					}
					return restored, err
				})
			result.Stdout, result.Stderr = stdout.String(), stderr.String()
		case "restore", "serve", "serve_once":
			var stdout, stderr bytes.Buffer
			if request.Operation == "restore" {
				result.ExitCode = runRestore(ctx, appCfg, []string{"--from", cfg.BackupPath}, &stdout, &stderr)
			} else {
				serveCtx := ctx
				cancelServe := func() {}
				if request.Operation == "serve_once" {
					serveCtx, cancelServe = context.WithTimeout(ctx, time.Second)
				}
				result.ExitCode = runServe(serveCtx, appCfg, slog.New(slog.NewTextHandler(&stderr, nil)))
				cancelServe()
			}
			result.Stdout, result.Stderr = stdout.String(), stderr.String()
		default:
			t.Fatalf("unknown subprocess operation %q", request.Operation)
		}
		require.NoError(t, out.Encode(result))
	}
	if commandDone != nil {
		cancel()
		require.Zero(t, <-commandDone, "server shutdown")
	}
}

func processExchange(t *testing.T, p *testProcess, operation string) processResult {
	t.Helper()
	require.NoError(t, p.send.Encode(processRequest{Operation: operation}))
	var result processResult
	require.NoError(t, p.receive.Decode(&result))
	require.Equal(t, p.pid, result.PID)
	return result
}

func TestTwoProcessRestoreOwnsLockThroughoutReplacement(t *testing.T) {
	for _, phase := range []string{"before", "after"} {
		t.Run(phase, func(t *testing.T) {
			databaseURL, backupPath := backupOf(t)
			path, err := db.ResolveSQLiteFilePath(databaseURL)
			require.NoError(t, err)
			cfg := processConfig{Mode: "command", DatabaseURL: databaseURL, HTTPAddr: "127.0.0.1:0", BackupPath: backupPath}
			restorer := startTestProcess(t, cfg)
			contender := startTestProcess(t, cfg)
			require.Equal(t, "paused", processExchange(t, restorer, "restore_paused_"+phase).Status)
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			preservedBefore, err := filepath.Glob(path + ".before-restore-*")
			require.NoError(t, err)
			for _, operation := range []string{"serve_once", "restore"} {
				result := processExchange(t, contender, operation)
				assert.Equal(t, 1, result.ExitCode, operation+" must refuse during restore")
				assert.Contains(t, result.Stderr, "locked by process "+strconv.Itoa(restorer.pid))
			}
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.True(t, bytes.Equal(before, after), "refused contenders must leave the database intact")
			preservedAfter, err := filepath.Glob(path + ".before-restore-*")
			require.NoError(t, err)
			assert.Equal(t, preservedBefore, preservedAfter, "a refused second restore must not preserve or replace files")
			completed := processExchange(t, restorer, "resume")
			require.Zerof(t, completed.ExitCode, "restore stderr: %s", completed.Stderr)
			assert.Contains(t, completed.Stdout, "previous data:")
			inspection, err := db.InspectBackup(context.Background(), path)
			require.NoError(t, err)
			assert.True(t, inspection.IntegrityOK)
			assert.NoError(t, lockfile.CheckAvailable(path), "restore must release its lock")
			started := processExchange(t, contender, "serve_once")
			require.Zerof(t, started.ExitCode, "server stderr: %s", started.Stderr)
			assert.Contains(t, started.Stderr, "server starting")
		})
	}
}

func TestTwoProcessLockHandoffsKeepOneFileIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "handoff.sqlite")
	databaseURL := db.SQLiteFileURL(path)
	for round := range 8 {
		t.Run(fmt.Sprintf("handoff_%02d", round), func(t *testing.T) {
			first, err := lockfile.Acquire(path)
			require.NoError(t, err)
			defer first.Close()
			identity, err := os.Stat(lockfile.Path(path))
			require.NoError(t, err)
			waiter := startTestProcess(t, processConfig{Mode: "preopened_lock", DatabaseURL: databaseURL})
			contender := startTestProcess(t, processConfig{Mode: "command", DatabaseURL: databaseURL})
			require.NoError(t, first.Close())
			// The waiter opened the original inode before release, and takes
			// its lock only afterwards: no timing window needs to be guessed.
			require.Equal(t, "ok", processExchange(t, waiter, "lock_preopened").Status)
			result := processExchange(t, contender, "acquire_lock")
			assert.Equal(t, "locked", result.Status, "a second process must not acquire a replacement inode")
			current, err := os.Stat(lockfile.Path(path))
			require.NoError(t, err)
			assert.True(t, os.SameFile(identity, current), "handoff must preserve the lock file")
			if result.Status == "ok" {
				processExchange(t, contender, "release_lock")
			}
			processExchange(t, waiter, "unlock_preopened")
			require.Equal(t, "ok", processExchange(t, contender, "acquire_lock").Status)
			processExchange(t, contender, "release_lock")
		})
	}
}

type testProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	send    *json.Encoder
	receive *json.Decoder
	stderr  bytes.Buffer
	pid     int
	closed  bool
}

func startTestProcess(t *testing.T, cfg processConfig) *testProcess {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	encoded, err := json.Marshal(cfg)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	p := &testProcess{cmd: exec.CommandContext(ctx, executable, "-test.run=^TestTwoProcessHelper$", "-test.count=1")}
	p.cmd.Env = append(os.Environ(), processTestEnv+"="+string(encoded))
	p.cmd.Stderr = &p.stderr
	p.stdin, err = p.cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := p.cmd.StdoutPipe()
	require.NoError(t, err)
	p.send, p.receive = json.NewEncoder(p.stdin), json.NewDecoder(stdout)
	require.NoError(t, p.cmd.Start())
	t.Cleanup(func() { p.close(t) })
	var ready processResult
	require.NoError(t, p.receive.Decode(&ready))
	require.Equal(t, "ready", ready.Status)
	require.Equal(t, p.cmd.Process.Pid, ready.PID)
	require.NotEqual(t, os.Getpid(), ready.PID)
	p.pid = ready.PID
	return p
}

func (p *testProcess) close(t *testing.T) {
	t.Helper()
	if p.closed {
		return
	}
	p.closed = true
	_ = p.stdin.Close()
	require.NoErrorf(t, p.cmd.Wait(), "subprocess stderr: %s", p.stderr.String())
}

// Both children have opened their own pools before either operation is sent.
// Send to both before receiving either result; no sleep defines the race.
func processPair(t *testing.T, pair [2]*testProcess, request processRequest) [2]processResult {
	t.Helper()
	for _, p := range pair {
		require.NoError(t, p.send.Encode(request))
	}
	var results [2]processResult
	for i, p := range pair {
		require.NoError(t, p.receive.Decode(&results[i]))
		require.Equal(t, p.pid, results[i].PID)
	}
	return results
}

func processBook(t *testing.T) (string, *db.BackgroundWorkRepository, *db.BackupRepository) {
	t.Helper()
	database, databaseURL := testdb.Open(t)
	_, err := database.Exec(`INSERT INTO users (id, username, password_hash, is_owner, created_at, updated_at)
		VALUES (1, 'owner', 'unused', 1, ?, ?)`, processNow, processNow)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO books (id, code, name, owner_user_id, created_at, updated_at, updated_by_user_id)
		VALUES (1, 'personal', 'Personal', 1, ?, ?, 1)`, processNow, processNow)
	require.NoError(t, err)
	return databaseURL, db.NewBackgroundWorkRepository(database), db.NewBackupRepository(database)
}

func TestTwoProcessWorkClaims(t *testing.T) {
	databaseURL, work, _ := processBook(t)
	cfg := processConfig{Mode: "database", DatabaseURL: databaseURL}
	pair := [2]*testProcess{startTestProcess(t, cfg), startTestProcess(t, cfg)}
	require.NotEqual(t, pair[0].pid, pair[1].pid)
	ctx := context.Background()
	for round := range 12 {
		t.Run(fmt.Sprintf("pending_item_%02d", round), func(t *testing.T) {
			item, err := work.EnqueueBackgroundWork(ctx, 1, "two_process_probe", fmt.Sprintf(`{"round":%d}`, round), processNow)
			require.NoError(t, err)
			results := processPair(t, pair, processRequest{"claim", processNow, processLease})
			winner := requireOneClaim(t, results, item.ID, 1)
			stored, err := work.BackgroundWorkByID(ctx, item.ID)
			require.NoError(t, err)
			assert.Equal(t, "running", stored.Status)
			assert.Equal(t, winner.Owner, stored.LeaseOwner.String)
			assert.Equal(t, processLease, stored.LeaseExpiresAt.String)
			assert.Equal(t, 1, stored.Attempts)
		})
	}
	t.Run("live_leases_are_not_stolen", func(t *testing.T) {
		for _, result := range processPair(t, pair, processRequest{"claim", processNow, processLease}) {
			assert.Equal(t, "empty", result.Status)
		}
	})
	t.Run("writer_contention_is_retryable_without_partial_claims", func(t *testing.T) {
		item, err := work.EnqueueBackgroundWork(ctx, 1, "two_process_probe", `{"contention":true}`, processNow)
		require.NoError(t, err)
		writer, err := db.Open(ctx, databaseURL)
		require.NoError(t, err)
		defer writer.Close()
		tx, err := writer.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer tx.Rollback()
		// Hold SQLite's writer lock, while both child processes can still
		// SELECT the pending row. Their read-to-write upgrades must report
		// BUSY, rather than hand out work or change its attempt count.
		_, err = tx.ExecContext(ctx, `UPDATE background_work_items SET attempts = attempts WHERE id = ?`, item.ID)
		require.NoError(t, err)
		for _, result := range processPair(t, pair, processRequest{"claim", processNow, processLease}) {
			assert.Equal(t, "busy", result.Status)
			assert.Zero(t, result.ItemID)
		}
		require.NoError(t, tx.Rollback())
		stored, err := work.BackgroundWorkByID(ctx, item.ID)
		require.NoError(t, err)
		assert.Equal(t, "pending", stored.Status)
		assert.Zero(t, stored.Attempts)
		assert.False(t, stored.LeaseOwner.Valid)
		assert.False(t, stored.LeaseExpiresAt.Valid)
		// Retry is an explicit next operation after releasing the writer,
		// not a test-only busy loop hidden around the production method.
		requireOneClaim(t, processPair(t, pair, processRequest{"claim", processNow, processLease}), item.ID, 1)
	})
	t.Run("expired_lease_has_one_new_owner", func(t *testing.T) {
		// Complete the other items so only this crashed worker's lease is due.
		items, err := work.ListBackgroundWorkByStatus(ctx, 1, "two_process_probe", "running", 20)
		require.NoError(t, err)
		require.Len(t, items, 13)
		for _, item := range items[1:] {
			require.NoError(t, work.CompleteBackgroundWork(ctx, item.ID, item.LeaseOwner.String, processNow))
		}
		item := items[0]
		results := processPair(t, pair, processRequest{"claim", processLease, "2026-10-14T08:10:00Z"})
		winner := requireOneClaim(t, results, item.ID, 2)
		stored, err := work.BackgroundWorkByID(ctx, item.ID)
		require.NoError(t, err)
		assert.Equal(t, winner.Owner, stored.LeaseOwner.String)
		assert.Equal(t, 2, stored.Attempts)
		assert.Equal(t, "2026-10-14T08:10:00Z", stored.LeaseExpiresAt.String)
	})
}

func requireOneClaim(t *testing.T, results [2]processResult, id int64, attempts int) processResult {
	t.Helper()
	var winners []processResult
	for _, result := range results {
		if result.Status == "ok" {
			assert.Equal(t, id, result.ItemID)
			assert.Equal(t, attempts, result.Attempts)
			assert.Equal(t, strconv.Itoa(result.PID), result.Owner)
			winners = append(winners, result)
		} else {
			assert.Contains(t, []string{"empty", "busy"}, result.Status)
		}
	}
	require.Len(t, winners, 1, "exactly one process owns the eligible item")
	return winners[0]
}

func TestTwoProcessScheduledBackupOccurrence(t *testing.T) {
	databaseURL, work, backups := processBook(t)
	cfg := processConfig{Mode: "database", DatabaseURL: databaseURL, BackupPath: filepath.Join(t.TempDir(), "scheduled.sqlite")}
	pair := [2]*testProcess{startTestProcess(t, cfg), startTestProcess(t, cfg)}
	results := processPair(t, pair, processRequest{Operation: "occurrence"})
	statuses := []string{results[0].Status, results[1].Status}
	require.ElementsMatch(t, []string{"ok", "duplicate"}, statuses)
	ctx := context.Background()
	runs, err := backups.ListBackupRuns(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	items, err := work.ListBackgroundWorkByStatus(ctx, 1, "two_process_probe", "pending", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, items[0].ID, runs[0].WorkItemID.Int64)
	assert.Equal(t, "scheduled:2026-10-14", runs[0].OccurrenceKey)
	assert.Equal(t, "2026-10-14", runs[0].ScheduledForLocalDate.String)
	for _, result := range results {
		if result.Status == "ok" {
			assert.Equal(t, runs[0].ID, result.RunID)
			assert.Equal(t, items[0].ID, result.ItemID)
		}
	}
	item, err := work.ClaimBackgroundWork(ctx, "two_process_probe", "parent", processNow, processLease)
	require.NoError(t, err)
	require.NoError(t, work.CompleteBackgroundWork(ctx, item.ID, "parent", processNow))
	require.NoError(t, backups.UpdateBackupRun(ctx, db.UpdateBackupRunParams{ID: runs[0].ID, Status: "completed", Now: processNow}))
	// The occurrence key still protects completed work, when the queue's
	// active-only uniqueness no longer does.
	for _, result := range processPair(t, pair, processRequest{Operation: "occurrence"}) {
		assert.Equal(t, "duplicate", result.Status)
	}
	runs, err = backups.ListBackupRuns(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	items, err = work.ListBackgroundWorkByStatus(ctx, 1, "two_process_probe", "pending", 10)
	require.NoError(t, err)
	assert.Empty(t, items, "duplicate occurrence leaves no orphan work")
}

func TestTwoProcessServerAndRestoreGuards(t *testing.T) {
	databaseURL, backupPath := backupOf(t)
	path, err := db.ResolveSQLiteFilePath(databaseURL)
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	server := startTestProcess(t, processConfig{Mode: "serve", DatabaseURL: databaseURL, HTTPAddr: addr})
	command := startTestProcess(t, processConfig{Mode: "command", DatabaseURL: databaseURL, HTTPAddr: "127.0.0.1:0", BackupPath: backupPath})
	for _, operation := range []string{"restore", "serve"} {
		require.NoError(t, command.send.Encode(processRequest{Operation: operation}))
		var result processResult
		require.NoError(t, command.receive.Decode(&result))
		assert.Equal(t, 1, result.ExitCode)
		assert.Contains(t, result.Stderr, "locked by process "+strconv.Itoa(server.pid))
		if operation == "restore" {
			assert.Contains(t, result.Stderr, "Stop the server or wait for the other restore to finish")
			assert.Empty(t, result.Stdout)
		}
	}
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "refused commands preserve the database")
	preserved, err := filepath.Glob(path + ".before-restore-*")
	require.NoError(t, err)
	assert.Empty(t, preserved, "refusal never starts replacement")
	server.close(t)
	require.NoError(t, command.send.Encode(processRequest{Operation: "restore"}))
	var restored processResult
	require.NoError(t, command.receive.Decode(&restored))
	require.Zerof(t, restored.ExitCode, "restore stderr: %s", restored.Stderr)
	assert.Contains(t, restored.Stdout, "restored:")
	assert.Contains(t, restored.Stdout, "previous data:")
	inspection, err := db.InspectBackup(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, inspection.BinaryVersion, inspection.SchemaVersion)
}
