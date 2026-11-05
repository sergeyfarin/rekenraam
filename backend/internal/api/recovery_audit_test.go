package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/lockfile"
	"rekenraam/backend/internal/testdb"
)

// Recovery audit (#277, area 9 of #260). The #266 reference book is seeded
// through the public API, backed up through the product's own backup run,
// restored onto a fresh instance and compared with the hand-derived register;
// its export bundle is checked and the register is rebuilt from the bundle
// alone. No figure below is read back from the code under test.

// recoveryAuditBook is a seeded reference book on a real database file, with
// the instance that serves it.
type recoveryAuditBook struct {
	seeded       seededReferenceBook
	instance     testInstance
	database     *sql.DB
	databaseURL  string
	databasePath string
}

func seedRecoveryAuditBook(t *testing.T) recoveryAuditBook {
	t.Helper()
	database, databaseURL := testdb.Open(t)
	databasePath, err := db.ResolveSQLiteFilePath(databaseURL)
	require.NoError(t, err)
	instance := newTestInstanceOnDatabase(t, database, databaseURL, HandlerOptions{})
	seeded := seedReferenceBook(t, instance.handler)
	return recoveryAuditBook{seeded: seeded, instance: instance, database: database, databaseURL: databaseURL, databasePath: databasePath}
}

// backUp takes a manual backup through the API and runs it to completion with
// the production backup run, then returns the verified backup file.
func (b recoveryAuditBook) backUp(t *testing.T) string {
	t.Helper()
	body := mutateBackup(t, b.instance.handler, b.seeded.sessionCookie, b.seeded.csrfToken, http.MethodPost,
		"/api/v1/maintenance/backups", ``, http.StatusAccepted)
	var run backupRunResponse
	require.NoError(t, json.Unmarshal(body, &run))
	require.NoError(t, b.instance.backup.RunBackup(context.Background(), run.ID))

	status := readBackupStatus(t, b.instance.handler, b.seeded.sessionCookie)
	require.NotEmpty(t, status.Runs)
	require.Equal(t, run.ID, status.Runs[0].ID)
	require.Equal(t, "completed", status.Runs[0].Status, status.Runs[0].ErrorSummary)
	require.True(t, status.Runs[0].Verified)
	require.FileExists(t, status.Runs[0].TargetPath)
	return status.Runs[0].TargetPath
}

// restoreOntoFreshInstance installs backupPath where no database exists yet,
// holding the server lock as `rekenraam restore` does, then opens it the way
// `serve` does (migrations re-admitted) and wires a new instance over it.
func restoreOntoFreshInstance(t *testing.T, backupPath string) (testInstance, *sql.DB, string) {
	t.Helper()
	ctx := context.Background()
	restoredPath := filepath.Join(t.TempDir(), "restored", "rekenraam.sqlite")
	require.NoError(t, os.MkdirAll(filepath.Dir(restoredPath), 0o700))
	restoredURL := db.SQLiteFileURL(restoredPath)

	held, err := lockfile.Acquire(restoredPath)
	require.NoError(t, err)
	result, err := db.RestoreSQLiteDatabase(ctx, backupPath, restoredURL)
	require.NoError(t, err)
	require.NoError(t, held.Close())
	assert.True(t, result.Installed)
	assert.Empty(t, result.PreservedFiles, "a fresh location has nothing to preserve")

	restored, err := db.Open(ctx, restoredURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, restored.Close()) })
	require.NoError(t, db.Migrate(ctx, restored))
	return newTestInstanceOnDatabase(t, restored, restoredURL, HandlerOptions{}), restored, restoredPath
}

// signInOwner signs in with the reference book's owner password: a restored
// instance is proved usable by a fresh sign-in, not by a carried-over cookie.
func signInOwner(t *testing.T, handler http.Handler) (*http.Cookie, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	cookie := sessionCookieFrom(t, res.Result().Cookies())

	sessionReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	sessionReq.AddCookie(cookie)
	sessionRes := httptest.NewRecorder()
	handler.ServeHTTP(sessionRes, sessionReq)
	require.Equal(t, http.StatusOK, sessionRes.Code)
	var session authSessionResponse
	require.NoError(t, json.NewDecoder(sessionRes.Body).Decode(&session))
	return cookie, session.CSRFToken
}

func runSelfCheckThroughAPI(t *testing.T, handler http.Handler, cookie *http.Cookie, csrf string) selfCheckRunResponse {
	t.Helper()
	body := mutateBackup(t, handler, cookie, csrf, http.MethodPost, "/api/v1/maintenance/self-check", ``, http.StatusOK)
	var run selfCheckRunResponse
	require.NoError(t, json.Unmarshal(body, &run))
	return run
}

// referenceSelfCheckVerdict is the independent expectation for the healthy
// reference book: G1's basis is supported unknown (#266), so lot
// reconciliation reports one unavailable lot and no finding (#313), the run
// is incomplete, attachments are reserved, and every other check passes.
func assertReferenceSelfCheckVerdict(t *testing.T, run selfCheckRunResponse, giftLotID int64) {
	t.Helper()
	assert.Equal(t, "incomplete", run.Status)
	assert.Zero(t, run.FailedCheckCount)
	assert.Equal(t, int64(1), run.IncompleteCheckCount)
	for _, result := range run.Results {
		switch result.CheckID {
		case "lot_reconciliation":
			assert.Equal(t, "incomplete", result.Status)
			assert.Zero(t, result.FindingCount)
			assert.Equal(t, int64(1), result.UnavailableCount)
			assert.Equal(t, []int64{giftLotID}, result.UnavailableSample)
		case "attachments":
			assert.Equal(t, "not_applicable", result.Status)
		default:
			assert.Equalf(t, "passed", result.Status, "%s: %s", result.CheckID, result.Summary)
			assert.Zero(t, result.FindingCount, result.CheckID)
			assert.Zero(t, result.UnavailableCount, result.CheckID)
		}
	}
}

// databaseFingerprint hashes every row of every table, keyed by table, so a
// read can be shown to have written nothing outside the tables it names.
func databaseFingerprint(t *testing.T, database *sql.DB) map[string]string {
	t.Helper()
	ctx := context.Background()
	rows, err := database.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	require.NoError(t, err)
	tables := []string{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())

	fingerprint := map[string]string{}
	for _, table := range tables {
		tableRows, err := database.QueryContext(ctx, `SELECT * FROM "`+table+`"`)
		require.NoError(t, err, table)
		columns, err := tableRows.Columns()
		require.NoError(t, err)
		encoded := []string{}
		for tableRows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			require.NoError(t, tableRows.Scan(pointers...))
			line := make([]string, len(values))
			for i, value := range values {
				if raw, ok := value.([]byte); ok {
					value = string(raw)
				}
				line[i] = fmt.Sprintf("%T:%v", value, value)
			}
			encoded = append(encoded, strings.Join(line, "\x1f"))
		}
		require.NoError(t, tableRows.Err())
		require.NoError(t, tableRows.Close())
		// Row order is not part of what a table holds.
		sort.Strings(encoded)
		sum := sha256.Sum256([]byte(strings.Join(encoded, "\x1e")))
		fingerprint[table] = hex.EncodeToString(sum[:])
	}
	return fingerprint
}

func changedTables(before, after map[string]string) []string {
	changed := []string{}
	for table, sum := range after {
		if before[table] != sum {
			changed = append(changed, table)
		}
	}
	for table := range before {
		if _, ok := after[table]; !ok {
			changed = append(changed, table)
		}
	}
	sort.Strings(changed)
	return changed
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func getBody(t *testing.T, handler http.Handler, cookie *http.Cookie, path string) []byte {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equalf(t, http.StatusOK, res.Code, "GET %s: %s", path, res.Body.String())
	return res.Body.Bytes()
}

func unzipFiles(t *testing.T, body []byte) map[string][]byte {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	require.NoError(t, err)
	files := map[string][]byte{}
	for _, file := range archive.File {
		reader, err := file.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(reader)
		require.NoError(t, reader.Close())
		require.NoError(t, err)
		files[file.Name] = content
	}
	return files
}

// recoveryExports is everything an owner can download about the book.
type recoveryExports struct {
	preview []byte
	ledger  []byte
	bundle  map[string][]byte
	qif     map[string][]byte
}

func downloadRecoveryExports(t *testing.T, handler http.Handler, cookie *http.Cookie) recoveryExports {
	t.Helper()
	return recoveryExports{
		preview: getBody(t, handler, cookie, "/api/v1/exports/preview"),
		ledger:  getBody(t, handler, cookie, "/api/v1/exports/ledger.csv"),
		bundle:  downloadBundle(t, handler, cookie, "").files,
		// The whole-book QIF skips investment accounts without refusing.
		qif: unzipFiles(t, getBody(t, handler, cookie, "/api/v1/exports/qif")),
	}
}

// withoutGeneratedAt drops the one field that names when a file was written.
func withoutGeneratedAt(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	delete(decoded, "generated_at")
	return decoded
}

// The restore drill: seed the #266 book, back it up through the product's
// backup run, restore it onto a fresh instance, sign in again, and compare
// every register figure, every self-check verdict and every exported byte.
// Reads, previews and checks along the way are shown to write nothing beyond
// their own documented records.
func TestRecoveryAuditReferenceBookRestoreDrill(t *testing.T) {
	t.Parallel()

	source := seedRecoveryAuditBook(t)
	cookie, csrf := source.seeded.sessionCookie, source.seeded.csrfToken
	giftLot := source.seeded.id(t, "lot_g1")
	assertReferenceBookRegister(t, source.instance.handler, source.seeded)

	// Previews, downloads and status reads write nothing at all.
	before := databaseFingerprint(t, source.database)
	sourceExports := downloadRecoveryExports(t, source.instance.handler, cookie)
	readSelfCheckStatus(t, source.instance.handler, cookie)
	readBackupStatus(t, source.instance.handler, cookie)
	assertReferenceBookRegister(t, source.instance.handler, source.seeded)
	assert.Empty(t, changedTables(before, databaseFingerprint(t, source.database)),
		"exports, previews, report reads and status reads must not write")

	// A self-check writes only its own run record.
	before = databaseFingerprint(t, source.database)
	sourceRun := runSelfCheckThroughAPI(t, source.instance.handler, cookie, csrf)
	assertReferenceSelfCheckVerdict(t, sourceRun, giftLot)
	assert.Equal(t, []string{"self_check_results", "self_check_runs"}, changedTables(before, databaseFingerprint(t, source.database)))

	// The backup run writes its run, its work item and the self-check it
	// chains (plan: Backups, step 7; Trial-balance self-check).
	before = databaseFingerprint(t, source.database)
	backupPath := source.backUp(t)
	assert.Equal(t, []string{"background_work_items", "backup_runs", "self_check_results", "self_check_runs"},
		changedTables(before, databaseFingerprint(t, source.database)))
	var chained selfCheckStatusResponse = readSelfCheckStatus(t, source.instance.handler, cookie)
	require.NotNil(t, chained.LatestRun)
	assert.Equal(t, "scheduled", chained.LatestRun.Trigger, "the backup chains its own self-check")
	assertReferenceSelfCheckVerdict(t, *chained.LatestRun, giftLot)

	// verify-backup reads the file without changing a byte of it.
	backupSum := fileSHA256(t, backupPath)
	inspection, err := db.InspectBackup(context.Background(), backupPath)
	require.NoError(t, err)
	assert.True(t, inspection.IntegrityOK)
	assert.Equal(t, inspection.BinaryVersion, inspection.SchemaVersion)
	assert.Equal(t, backupSum, fileSHA256(t, backupPath), "verification must not modify the backup")

	restored, restoredDatabase, _ := restoreOntoFreshInstance(t, backupPath)
	restoredCookie, restoredCSRF := signInOwner(t, restored.handler)
	drill := source.seeded
	drill.sessionCookie, drill.csrfToken = restoredCookie, restoredCSRF
	assertReferenceBookRegister(t, restored.handler, drill)

	restoredRun := runSelfCheckThroughAPI(t, restored.handler, restoredCookie, restoredCSRF)
	assertReferenceSelfCheckVerdict(t, restoredRun, giftLot)

	before = databaseFingerprint(t, restoredDatabase)
	restoredExports := downloadRecoveryExports(t, restored.handler, restoredCookie)
	assert.Empty(t, changedTables(before, databaseFingerprint(t, restoredDatabase)))

	assert.Equal(t, withoutGeneratedAt(t, sourceExports.preview), withoutGeneratedAt(t, restoredExports.preview), "export preview")
	assert.Equal(t, string(sourceExports.ledger), string(restoredExports.ledger), "ledger.csv")
	require.Equal(t, len(sourceExports.qif), len(restoredExports.qif))
	for name, content := range sourceExports.qif {
		if name == "manifest.json" {
			assert.Equal(t, withoutGeneratedAt(t, content), withoutGeneratedAt(t, restoredExports.qif[name]), "QIF manifest")
			continue
		}
		assert.Equal(t, string(content), string(restoredExports.qif[name]), "QIF %s", name)
	}
	require.Equal(t, len(sourceExports.bundle), len(restoredExports.bundle))
	for name, content := range sourceExports.bundle {
		if name == "manifest.json" {
			assert.Equal(t, withoutGeneratedAt(t, content), withoutGeneratedAt(t, restoredExports.bundle[name]), "manifest")
			continue
		}
		assert.Equal(t, string(content), string(restoredExports.bundle[name]), "bundle %s", name)
	}
}

// corruptRestoredCopy restores the backup onto a fresh instance, applies one
// deliberate corruption with raw SQL, and returns the instance and database.
func corruptRestoredCopy(t *testing.T, backupPath string, corrupt func(*testing.T, *sql.Conn)) (http.Handler, *sql.DB) {
	t.Helper()
	instance, database, _ := restoreOntoFreshInstance(t, backupPath)
	ctx := context.Background()
	conn, err := database.Conn(ctx)
	require.NoError(t, err)
	corrupt(t, conn)
	require.NoError(t, conn.Close())
	return instance.handler, database
}

func execOn(t *testing.T, conn *sql.Conn, query string, args ...any) {
	t.Helper()
	_, err := conn.ExecContext(context.Background(), query, args...)
	require.NoError(t, err, query)
}

// Every corruption class named by #277 is detected on a restored copy of the
// reference book, beside — never instead of — G1's unavailable basis, and the
// failing check writes only its own run record.
func TestRecoveryAuditSelfCheckDetectsCorruptionOfARestoredReferenceBook(t *testing.T) {
	t.Parallel()

	source := seedRecoveryAuditBook(t)
	backupPath := source.backUp(t)
	ids := source.seeded
	checking, lotB2, giftLot := ids.id(t, "checking"), ids.id(t, "lot_b2"), ids.id(t, "lot_g1")
	eur := ids.id(t, "EUR")

	for _, testCase := range []struct {
		name    string
		checkID string
		corrupt func(*testing.T, *sql.Conn)
	}{
		{
			// A backfill that writes a one-sided posting into the current
			// posted version of checking's opening balance. (The same row on a
			// superseded version is history, which the check rightly ignores.)
			name: "unbalanced entry", checkID: "entry_balance",
			corrupt: func(t *testing.T, conn *sql.Conn) {
				var transactionID, versionID, entryID int64
				require.NoError(t, conn.QueryRowContext(context.Background(), `
					SELECT pv.transaction_version_id, tv.transaction_id, pv.journal_entry_id
					FROM posting_versions pv JOIN transaction_versions tv ON tv.id = pv.transaction_version_id
					WHERE pv.account_id = ? AND tv.status = 'posted' AND tv.version_seq =
						(SELECT MAX(version_seq) FROM transaction_versions WHERE transaction_id = tv.transaction_id)
					ORDER BY pv.id LIMIT 1`, checking).Scan(&versionID, &transactionID, &entryID))
				execOn(t, conn, `INSERT INTO posting_lines (id, book_id, transaction_id, line_key, created_at, created_by_user_id)
					VALUES (9001, 1, ?, 'audit-backfill', '2026-10-18T00:00:00Z', 1)`, transactionID)
				execOn(t, conn, `INSERT INTO posting_versions (book_id, transaction_version_id, journal_entry_id, posting_line_id,
					line_seq, account_id, quantity_value, quantity_scale, commodity_id, reconciliation_status)
					VALUES (1, ?, ?, 9001, 99, ?, '1000', 2, ?, 'uncleared')`, versionID, entryID, checking, eur)
			},
		},
		{
			// B2 claims 6 units where the FIFO holding has 5.
			name: "lot quantity mismatch", checkID: "lot_reconciliation",
			corrupt: func(t *testing.T, conn *sql.Conn) {
				execOn(t, conn, `UPDATE investment_lot_state SET remaining_quantity_value = '60000' WHERE lot_id = ?`, lotB2)
			},
		},
		{
			// Recurring occurrences whose template is gone: only reachable
			// with foreign keys off, as an external repair tool might do.
			name: "orphaned foreign key", checkID: "sqlite_integrity",
			corrupt: func(t *testing.T, conn *sql.Conn) {
				execOn(t, conn, `PRAGMA foreign_keys = OFF`)
				execOn(t, conn, `DELETE FROM recurring_template_tags`)
				execOn(t, conn, `DELETE FROM recurring_template_postings`)
				execOn(t, conn, `DELETE FROM recurring_templates`)
				execOn(t, conn, `PRAGMA foreign_keys = ON`)
			},
		},
		{
			// A journal-backed investment operation with no posted version.
			name: "orphaned investment operation", checkID: "investment_foundation",
			corrupt: func(t *testing.T, conn *sql.Conn) {
				execOn(t, conn, `INSERT INTO audit_events (book_id, actor_user_id, occurred_at, origin_type, operation)
					VALUES (1, 1, '2026-10-18T00:00:00Z', 'internal', 'audit.orphan_investment_operation')`)
				execOn(t, conn, `INSERT INTO investment_operations (book_id, operation_kind, event_date, created_at, created_audit_event_id)
					VALUES (1, 'buy', '2026-02-02', '2026-10-18T00:00:00Z', last_insert_rowid())`)
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			handler, database := corruptRestoredCopy(t, backupPath, testCase.corrupt)
			cookie, csrf := signInOwner(t, handler)

			before := databaseFingerprint(t, database)
			run := runSelfCheckThroughAPI(t, handler, cookie, csrf)
			assert.Equal(t, []string{"self_check_results", "self_check_runs"}, changedTables(before, databaseFingerprint(t, database)),
				"a failing check repairs nothing")

			assert.Equal(t, "failed", run.Status)
			assert.Positive(t, run.FailedCheckCount)
			for _, result := range run.Results {
				if result.CheckID == testCase.checkID {
					assert.Equal(t, "failed", result.Status, result.Summary)
					assert.Positive(t, result.FindingCount)
					assert.NotEmpty(t, result.NextStep)
				}
				if result.CheckID == "lot_reconciliation" {
					// Unknown basis stays reported as unavailable coverage,
					// apart from any finding.
					assert.Equal(t, int64(1), result.UnavailableCount)
					assert.Contains(t, result.UnavailableSample, giftLot)
				}
			}
		})
	}
}

// --- Bundle reconstruction -------------------------------------------------

func ratOf(t *testing.T, decimal string) *big.Rat {
	t.Helper()
	value, ok := new(big.Rat).SetString(decimal)
	require.Truef(t, ok, "not an exact decimal: %q", decimal)
	return value
}

// coefficientRat reads a value/scale pair; a blank value is unknown (nil).
func coefficientRat(t *testing.T, value, scale string) *big.Rat {
	t.Helper()
	if value == "" {
		return nil
	}
	coefficient := ratOf(t, value)
	exponent, ok := new(big.Int).SetString(scale, 10)
	require.True(t, ok, scale)
	divisor := new(big.Int).Exp(big.NewInt(10), exponent, nil)
	return coefficient.Quo(coefficient, new(big.Rat).SetInt(divisor))
}

func ratText(value *big.Rat) string {
	if value == nil {
		return "null"
	}
	return value.RatString()
}

// csvRecords reads one archive CSV into maps keyed by header.
func csvRecords(t *testing.T, files map[string][]byte, name string) []map[string]string {
	t.Helper()
	raw, ok := files[name]
	require.Truef(t, ok, "%s is missing", name)
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte(utf8BOM)))).ReadAll()
	require.NoError(t, err, name)
	require.NotEmpty(t, records, name)
	out := make([]map[string]string, 0, len(records)-1)
	for _, record := range records[1:] {
		row := map[string]string{}
		for i, column := range records[0] {
			row[column] = record[i]
		}
		out = append(out, row)
	}
	return out
}

// The bundle's manifest and references are checked independently of the
// exporter, and the register is rebuilt from the archive alone: balances,
// reconciled statement, spending, cash movement, net worth, lots, positions
// and realized/unrealized gains under every method. Budget targets are not in
// the archive, which is stated rather than assumed.
func TestRecoveryAuditBundleReconstructsTheRegister(t *testing.T) {
	t.Parallel()

	book := seedRecoveryAuditBook(t)
	s := book.seeded
	register := s.book.Register
	files := downloadBundle(t, book.instance.handler, s.sessionCookie, "").files

	// Manifest: every file listed with its own bytes, rows and digest, and
	// nothing in the archive left unlisted.
	var manifest struct {
		SchemaVersion int `json:"schema_version"`
		Files         []struct {
			Name   string `json:"name"`
			Rows   int64  `json:"rows"`
			Bytes  int64  `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
		Excluded []string `json:"excluded"`
	}
	require.NoError(t, json.Unmarshal(files["manifest.json"], &manifest))
	assert.Equal(t, 19, manifest.SchemaVersion)
	listed := map[string]bool{}
	for _, file := range manifest.Files {
		listed[file.Name] = true
		content, ok := files[file.Name]
		if !assert.Truef(t, ok, "manifest lists absent %s", file.Name) {
			continue
		}
		sum := sha256.Sum256(content)
		assert.Equal(t, hex.EncodeToString(sum[:]), file.SHA256, file.Name)
		assert.Equal(t, int64(len(content)), file.Bytes, file.Name)
		if strings.HasSuffix(file.Name, ".csv") {
			assert.Equal(t, int64(len(csvRecords(t, files, file.Name))), file.Rows, file.Name)
		}
	}
	for name := range files {
		if name != "manifest.json" {
			assert.Truef(t, listed[name], "%s is in the archive but not the manifest", name)
		}
	}
	// What the archive leaves out is said, not assumed (#321).
	for _, excluded := range []string{"budget", "recurring", "reconciliation checkpoints"} {
		found := false
		for _, entry := range manifest.Excluded {
			found = found || strings.Contains(entry, excluded)
		}
		assert.Truef(t, found, "the manifest's exclusion list must name %s", excluded)
	}
	assert.Contains(t, string(files["README.txt"]), "budget targets")

	// References: every id column resolves inside the archive, except audit
	// event and user attribution, which are documented exclusions (#322).
	idSets := map[string]map[string]bool{}
	collect := func(set, file, column string) {
		if idSets[set] == nil {
			idSets[set] = map[string]bool{}
		}
		for _, row := range csvRecords(t, files, file) {
			idSets[set][row[column]] = true
		}
	}
	collect("account", "accounts.csv", "account_id")
	collect("commodity", "commodities.csv", "commodity_id")
	collect("transaction", "ledger.csv", "transaction_id")
	collect("transaction_version", "ledger.csv", "transaction_version_id")
	collect("posting_version", "ledger.csv", "posting_version_id")
	collect("lot", "lots.csv", "lot_id")
	collect("lot_event", "investment-lot-events.csv", "lot_event_id")
	collect("operation", "investment-operations.csv", "operation_id")
	collect("decision", "disposal-decisions.csv", "decision_id")
	collect("fee_policy", "investment-fee-policies.csv", "policy_id")
	collect("fee_policy_version", "investment-fee-policy-versions.csv", "policy_version_id")
	targets := map[string]string{
		"account_id": "account", "parent_account_id": "account", "holding_account_id": "account", "cash_account_id": "account",
		"charge_account_id": "account", "source_account_id": "account", "destination_account_id": "account",
		"commodity_id": "commodity", "cost_commodity_id": "commodity", "base_commodity_id": "commodity",
		"quote_commodity_id": "commodity", "destination_commodity_id": "commodity",
		"transaction_id": "transaction", "source_transaction_id": "transaction", "correction_of_transaction_id": "transaction",
		"transaction_version_id": "transaction_version", "source_transaction_version_id": "transaction_version",
		"lot_id": "lot", "source_lot_id": "lot", "destination_lot_id": "lot",
		"lot_event_id": "lot_event", "operation_id": "operation", "settled_operation_id": "operation",
		"split_operation_id": "operation", "correction_of_operation_id": "operation", "decision_id": "decision",
		"fee_policy_version_id": "fee_policy_version", "posting_version_id": "posting_version",
	}
	unresolved := map[string]bool{}
	for name := range files {
		if !strings.HasSuffix(name, ".csv") {
			continue
		}
		for _, row := range csvRecords(t, files, name) {
			for column, value := range row {
				if value == "" || !strings.HasSuffix(column, "_id") || column == "audit_event_id" || strings.HasSuffix(column, "user_id") {
					continue
				}
				key := name + ":" + column
				set, typed := targets[column]
				switch {
				case name == "accounts.csv" && column == "account_id",
					name == "commodities.csv" && column == "commodity_id",
					name == "lots.csv" && column == "lot_id",
					name == "investment-lot-events.csv" && column == "lot_event_id",
					name == "ledger.csv" && column == "posting_version_id",
					name == "investment-operations.csv" && column == "operation_id",
					name == "disposal-decisions.csv" && column == "decision_id",
					column == "policy_id" || column == "policy_version_id" || column == "version_id" || column == "payee_id" ||
						column == "journal_entry_id" || column == "component_id":
					continue
				case !typed:
					unresolved[key] = true
				case !idSets[set][value]:
					t.Errorf("%s = %s does not resolve to an exported %s", key, value, set)
				}
			}
		}
	}
	assert.Empty(t, unresolved, "id columns this audit does not classify")

	ledger := csvRecords(t, files, "ledger.csv")
	accounts := map[string]map[string]string{}
	for _, row := range csvRecords(t, files, "accounts.csv") {
		accounts[row["account_id"]] = row
	}
	commodityCode := map[string]string{}
	for _, row := range csvRecords(t, files, "commodities.csv") {
		commodityCode[row["commodity_id"]] = row["code"]
	}
	key := func(name string) string { return strconvFormatInt(s.id(t, name)) }
	sumWhere := func(keep func(map[string]string) bool) map[string]*big.Rat {
		totals := map[string]*big.Rat{}
		for _, row := range ledger {
			if !keep(row) {
				continue
			}
			code := commodityCode[row["commodity_id"]]
			if totals[code] == nil {
				totals[code] = new(big.Rat)
			}
			totals[code].Add(totals[code], ratOf(t, row["quantity"]))
		}
		return totals
	}
	totalOf := func(totals map[string]*big.Rat, code string) string {
		if totals[code] == nil {
			return "0"
		}
		return totals[code].RatString()
	}

	// Balances: entry-date arithmetic over ledger.csv. "Today" is every row.
	for _, figure := range register.Balances {
		account := key(figure.Account)
		totals := sumWhere(func(row map[string]string) bool {
			return row["account_id"] == account && (figure.AsOf == "" || row["entry_date"] <= figure.AsOf)
		})
		assert.Equal(t, exactValue(t, figure.Value), totalOf(totals, figure.Commodity), "rebuilt balance %s %s on %s", figure.Account, figure.Commodity, figure.AsOf)
	}
	for _, figure := range register.SubtreeBalances {
		parent := key(figure.Account)
		totals := sumWhere(func(row map[string]string) bool {
			for id := row["account_id"]; id != ""; id = accounts[id]["parent_account_id"] {
				if id == parent {
					return row["entry_date"] <= figure.AsOf
				}
			}
			return false
		})
		assert.Equal(t, exactValue(t, figure.Value), totalOf(totals, figure.Commodity), "rebuilt subtree %s %s", figure.Account, figure.Commodity)
	}
	for _, figure := range register.AccountStatus {
		assert.Equal(t, figure.Status, accounts[key(figure.Account)]["status"], "status of %s", figure.Account)
	}
	// The reconciled statement: reconciled postings cleared by its date.
	for _, figure := range register.Reconciliation {
		account := key(figure.Account)
		totals := sumWhere(func(row map[string]string) bool {
			return row["account_id"] == account && row["reconciliation_status"] == "reconciled" && row["cleared_on"] <= figure.StatementDate
		})
		assert.Equal(t, exactValue(t, figure.StatementBalance), totalOf(totals, figure.Commodity), "rebuilt statement of %s", figure.Account)
	}
	// Drafts and voided versions are not exported (record policy).
	for _, row := range ledger {
		for _, draft := range register.Drafts {
			assert.NotEqual(t, draft.Description, row["description"], "a draft is not ledger truth")
		}
		assert.Equal(t, "posted", row["status"])
	}

	// Spending: category postings in the period, in entries that touch the
	// report's accounts. Income reads credit-positive.
	period := register.Period
	scope := func(names []string) map[string]bool {
		set := map[string]bool{}
		for _, name := range names {
			set[key(name)] = true
		}
		return set
	}
	spendingScope := scope(register.Spending.Accounts)
	entryInScope := map[string]bool{}
	for _, row := range ledger {
		if spendingScope[row["account_id"]] {
			entryInScope[row["journal_entry_id"]] = true
		}
	}
	for _, mode := range []struct {
		figures []referenceSpendingFigure
		sign    int64
	}{{register.Spending.Expense, 1}, {register.Spending.Income, -1}} {
		for _, figure := range mode.figures {
			category := key(figure.Category)
			totals := sumWhere(func(row map[string]string) bool {
				return row["account_id"] == category && entryInScope[row["journal_entry_id"]] &&
					row["entry_date"] >= period.StartDate && row["entry_date"] <= period.EndDate
			})
			got := new(big.Rat)
			if totals[figure.Commodity] != nil {
				got.Mul(totals[figure.Commodity], big.NewRat(mode.sign, 1))
			}
			assert.Equal(t, exactValue(t, figure.Amount), got.RatString(), "rebuilt spending %s", figure.Category)
		}
	}
	// Cash movement: the cashflow report's net movement per commodity.
	cashScope := scope(register.Cashflow.Accounts)
	movement := sumWhere(func(row map[string]string) bool {
		return cashScope[row["account_id"]] && row["entry_date"] >= period.StartDate && row["entry_date"] <= period.EndDate
	})
	for _, row := range register.Cashflow.Rows {
		assert.Equal(t, exactValue(t, row.NetMovement), totalOf(movement, row.Commodity), "rebuilt net movement %s", row.Commodity)
	}

	// Net worth: asset and liability accounts (system accounts are equity),
	// converted with a rate or price at most 7 days old on the bucket date.
	prices := csvRecords(t, files, "prices.csv")
	eurCode := "EUR"
	priceOn := func(base, quote, asOf string) *big.Rat {
		var best map[string]string
		for _, row := range prices {
			if commodityCode[row["base_commodity_id"]] != base || commodityCode[row["quote_commodity_id"]] != quote || row["valuation_date"] > asOf {
				continue
			}
			if best == nil || row["valuation_date"] > best["valuation_date"] ||
				(row["valuation_date"] == best["valuation_date"] && row["is_manual"] == "true") {
				best = row
			}
		}
		if best == nil || daysBetween(t, best["valuation_date"], asOf) > 7 {
			return nil
		}
		return new(big.Rat).Quo(ratOf(t, best["price"]), ratOf(t, best["base_quantity"]))
	}
	for _, figure := range register.NetWorth {
		totals := sumWhere(func(row map[string]string) bool {
			class := accounts[row["account_id"]]["account_class"]
			return (class == "asset" || class == "liability") && row["entry_date"] <= figure.AsOf
		})
		want, got := []string{}, []string{}
		for _, total := range figure.Totals {
			want = append(want, total.Commodity+" "+exactValue(t, total.Value))
		}
		converted := new(big.Rat)
		complete := true
		for code, total := range totals {
			if total.Sign() == 0 {
				continue
			}
			got = append(got, code+" "+total.RatString())
			value := new(big.Rat).Set(total)
			if code != eurCode {
				quote := "USD"
				if code == "USD" {
					quote = eurCode
				}
				price := priceOn(code, quote, figure.AsOf)
				if price == nil {
					complete = false
					continue
				}
				value.Mul(value, price)
				if quote != eurCode {
					rate := priceOn(quote, eurCode, figure.AsOf)
					if rate == nil {
						complete = false
						continue
					}
					value.Mul(value, rate)
				}
			}
			converted.Add(converted, value)
		}
		sort.Strings(want)
		sort.Strings(got)
		assert.Equal(t, want, got, "rebuilt net worth totals on %s", figure.AsOf)
		gotConverted := "null"
		if complete {
			gotConverted = converted.RatString()
		}
		assert.Equal(t, optionalExpected(t, figure.ConvertedEUR), gotConverted, "rebuilt net worth in EUR on %s", figure.AsOf)
	}

	// Lots: current state from lots.csv, and quantities and position basis
	// again from the immutable lot events alone. Under average cost a disposal
	// event carries the pool rate, so one lot's events need not sum to its own
	// basis; a position's events do.
	lotsCSV := map[string]map[string]string{}
	for _, row := range csvRecords(t, files, "lots.csv") {
		lotsCSV[row["lot_id"]] = row
	}
	eventQuantity, positionBasis, positionUnknown := map[string]*big.Rat{}, map[string]*big.Rat{}, map[string]bool{}
	for _, row := range csvRecords(t, files, "investment-lot-events.csv") {
		lot, account := row["lot_id"], lotsCSV[row["lot_id"]]["account_id"]
		if eventQuantity[lot] == nil {
			eventQuantity[lot] = new(big.Rat)
		}
		if positionBasis[account] == nil {
			positionBasis[account] = new(big.Rat)
		}
		eventQuantity[lot].Add(eventQuantity[lot], coefficientRat(t, row["quantity_value"], row["quantity_scale"]))
		if row["basis_knowledge"] == "unknown" {
			positionUnknown[account] = true
			continue
		}
		positionBasis[account].Add(positionBasis[account], coefficientRat(t, row["cost_basis_value"], row["cost_basis_scale"]))
	}
	for _, figure := range register.Lots {
		lot := key(figure.Lot)
		row := lotsCSV[lot]
		assert.Equal(t, exactValue(t, figure.RemainingQuantity), new(big.Rat).Abs(eventQuantity[lot]).RatString(), "lot %s quantity from events", figure.Lot)
		assert.Equal(t, figure.Status, row["status"], "lot %s status", figure.Lot)
		assert.Equal(t, figure.BasisKnowledge, row["basis_knowledge"], "lot %s knowledge", figure.Lot)
		assert.Equal(t, exactValue(t, figure.RemainingQuantity), ratOf(t, row["remaining_quantity"]).RatString(), "lot %s quantity in lots.csv", figure.Lot)
		csvBasis := "null"
		if row["remaining_cost_basis"] != "" {
			csvBasis = ratOf(t, row["remaining_cost_basis"]).RatString()
		}
		assert.Equal(t, optionalExpected(t, figure.RemainingBasis), csvBasis, "lot %s basis in lots.csv", figure.Lot)
	}
	for _, figure := range register.Positions {
		account := key(figure.Account)
		basis := new(big.Rat).Abs(positionBasis[account])
		got := basis.RatString()
		if positionUnknown[account] {
			got = "null"
		}
		assert.Equal(t, optionalExpected(t, figure.RemainingBasis), got, "position %s basis from events", figure.Account)
	}

	// Gains under every method, replayed from the opening facts, split facts,
	// capital-return facts, disposal facts and prices.
	replay := replayBundleInvestments(t, files)
	for _, figure := range register.Positions {
		position := replay["operational"].positions[key(figure.Account)]
		require.NotNil(t, position, figure.Account)
		assert.Equal(t, figure.Side, position.side, "position side %s", figure.Account)
		assert.Equal(t, exactValue(t, figure.Quantity), position.quantity().RatString(), "position quantity %s", figure.Account)
		assert.Equal(t, optionalExpected(t, figure.RemainingBasis), ratText(position.basis()), "position basis %s", figure.Account)
	}
	valuationAsOf := register.Gains.ValuationAsOf
	for _, method := range register.Gains.Methods {
		result := replay[method.Method]
		require.NotNil(t, result, method.Method)
		total := new(big.Rat)
		for _, figure := range method.Realized {
			realized := result.realized[key(figure.Account)+"@"+figure.DisposalDate]
			assert.Equal(t, exactValue(t, figure.Gain), ratText(realized), "%s realized %s", method.Method, figure.Account)
			if realized != nil {
				total.Add(total, realized)
			}
		}
		assert.Len(t, result.realized, len(method.Realized), "%s realized entries", method.Method)
		assert.Equal(t, exactValue(t, method.RealizedTotal), total.RatString(), "%s realized total", method.Method)
		for _, figure := range method.Unrealized {
			position := result.positions[key(figure.Account)]
			require.NotNil(t, position, figure.Account)
			price := priceOn(position.commodity, "USD", valuationAsOf)
			require.NotNil(t, price, "price of %s", position.commodity)
			assert.Equal(t, optionalExpected(t, figure.Gain), ratText(position.unrealized(price)), "%s unrealized %s", method.Method, figure.Account)
		}
	}

	// Dividends and payments in lieu are typed by their facts and land on the
	// register's categories through the journal.
	for _, row := range csvRecords(t, files, "investment-dividend-facts.csv") {
		assert.Equal(t, "REF", commodityCode[row["commodity_id"]])
	}
	for _, row := range csvRecords(t, files, "investment-payment-in-lieu-facts.csv") {
		assert.Equal(t, key("hold_short"), row["holding_account_id"])
	}

	// Budget targets and account budget treatment have no file: the budget
	// register cannot be rebuilt from the archive (it is restored natively).
	for name := range files {
		assert.NotContains(t, name, "budget")
	}
}

func daysBetween(t *testing.T, from, to string) int {
	t.Helper()
	start, err := time.Parse(time.DateOnly, from)
	require.NoError(t, err)
	end, err := time.Parse(time.DateOnly, to)
	require.NoError(t, err)
	return int(end.Sub(start).Hours() / 24)
}

// replayLot is one lot during an independent replay.
type replayLot struct {
	id       string
	opened   string
	quantity *big.Rat
	basis    *big.Rat // nil when unknown
}

type replayPosition struct {
	side      string
	commodity string
	lots      []*replayLot
}

func (p *replayPosition) quantity() *big.Rat {
	total := new(big.Rat)
	for _, lot := range p.lots {
		total.Add(total, lot.quantity)
	}
	return total
}

func (p *replayPosition) basis() *big.Rat {
	total := new(big.Rat)
	for _, lot := range p.lots {
		if lot.quantity.Sign() == 0 {
			continue
		}
		if lot.basis == nil {
			return nil
		}
		total.Add(total, lot.basis)
	}
	return total
}

// unrealized is market value less basis for a long, retained proceeds less
// the cost to cover for a short; unknown basis has no gain.
func (p *replayPosition) unrealized(price *big.Rat) *big.Rat {
	basis := p.basis()
	if basis == nil {
		return nil
	}
	value := new(big.Rat).Mul(p.quantity(), price)
	if p.side == "short" {
		return value.Sub(basis, value)
	}
	return value.Sub(value, basis)
}

type replayResult struct {
	positions map[string]*replayPosition
	realized  map[string]*big.Rat // account@date
}

// replayBundleInvestments recomputes positions and gains from the archive's
// opening, split, capital-return and disposal facts. "operational" uses each
// disposal decision's own method; the others apply one method throughout.
// Average cost depletes lots oldest-first and restates survivors at the pool
// rate, as the investments plan specifies.
func replayBundleInvestments(t *testing.T, files map[string][]byte) map[string]*replayResult {
	t.Helper()
	type event struct {
		date, kind, account, order string
		row                        map[string]string
	}
	events := []event{}
	for _, row := range csvRecords(t, files, "investment-lot-facts.csv") {
		events = append(events, event{row["opened_on"], "0acquire", row["account_id"], row["lot_id"], row})
	}
	for _, row := range csvRecords(t, files, "investment-split-facts.csv") {
		events = append(events, event{row["effective_on"], "1split", row["account_id"], row["operation_id"], row})
	}
	for _, row := range csvRecords(t, files, "investment-capital-return-facts.csv") {
		events = append(events, event{row["effective_on"], "2capital", row["account_id"], row["operation_id"], row})
	}
	for _, row := range csvRecords(t, files, "disposal-decisions.csv") {
		events = append(events, event{row["event_date"], "3dispose", row["account_id"], row["decision_id"], row})
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].date != events[j].date {
			return events[i].date < events[j].date
		}
		if events[i].kind != events[j].kind {
			return events[i].kind < events[j].kind
		}
		return events[i].order < events[j].order
	})

	results := map[string]*replayResult{}
	for _, method := range []string{"operational", "fifo", "lifo", "average_cost"} {
		result := &replayResult{positions: map[string]*replayPosition{}, realized: map[string]*big.Rat{}}
		for _, ev := range events {
			position := result.positions[ev.account]
			switch ev.kind {
			case "0acquire":
				if position == nil {
					position = &replayPosition{side: ev.row["position_side"], commodity: codeOf(t, files, ev.row["commodity_id"])}
					result.positions[ev.account] = position
				}
				position.lots = append(position.lots, &replayLot{
					id: ev.row["lot_id"], opened: ev.row["opened_on"],
					quantity: coefficientRat(t, ev.row["quantity_value"], ev.row["quantity_scale"]),
					basis:    coefficientRat(t, ev.row["consideration_value"], ev.row["consideration_scale"]),
				})
			case "1split":
				ratio := new(big.Rat).SetFrac(ratInt(t, ev.row["ratio_numerator"]), ratInt(t, ev.row["ratio_denominator"]))
				for _, lot := range position.lots {
					lot.quantity.Mul(lot.quantity, ratio)
				}
			case "2capital":
				amount := coefficientRat(t, ev.row["amount_value"], ev.row["amount_scale"])
				held := position.quantity()
				for _, lot := range position.lots {
					if lot.quantity.Sign() == 0 || lot.basis == nil {
						continue
					}
					share := new(big.Rat).Mul(amount, new(big.Rat).Quo(lot.quantity, held))
					lot.basis.Sub(lot.basis, share)
				}
			case "3dispose":
				lotMethod := method
				if method == "operational" {
					lotMethod = ev.row["cost_basis_method"]
				}
				quantity := ratOf(t, ev.row["quantity"])
				proceeds := coefficientRat(t, ev.row["proceeds_value"], ev.row["proceeds_scale"])
				basis := disposeFromPosition(t, position, quantity, lotMethod)
				gain := new(big.Rat)
				if basis == nil {
					gain = nil
				} else if position.side == "short" {
					gain.Add(basis, proceeds)
				} else {
					gain.Sub(proceeds, basis)
				}
				result.realized[ev.account+"@"+ev.date] = gain
			}
		}
		results[method] = result
	}
	return results
}

// disposeFromPosition removes quantity by method and returns the basis it
// carried away (nil if any of it was unknown).
func disposeFromPosition(t *testing.T, position *replayPosition, quantity *big.Rat, method string) *big.Rat {
	t.Helper()
	order := append([]*replayLot(nil), position.lots...)
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].opened != order[j].opened {
			return order[i].opened < order[j].opened
		}
		return order[i].id < order[j].id
	})
	if method == "lifo" {
		for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
			order[i], order[j] = order[j], order[i]
		}
	}
	if method == "average_cost" {
		poolBasis, poolQuantity := position.basis(), position.quantity()
		if poolBasis == nil {
			return nil
		}
		rate := new(big.Rat).Quo(poolBasis, poolQuantity)
		disposed := new(big.Rat).Mul(rate, quantity)
		takeQuantity(order, quantity, false)
		for _, lot := range position.lots {
			lot.basis = new(big.Rat).Mul(rate, lot.quantity)
		}
		return disposed
	}
	require.Contains(t, []string{"fifo", "lifo"}, method)
	return takeQuantity(order, quantity, true)
}

func takeQuantity(order []*replayLot, quantity *big.Rat, withBasis bool) *big.Rat {
	remaining := new(big.Rat).Set(quantity)
	disposed := new(big.Rat)
	unknown := false
	for _, lot := range order {
		if remaining.Sign() == 0 {
			break
		}
		if lot.quantity.Sign() == 0 {
			continue
		}
		take := new(big.Rat).Set(remaining)
		if take.Cmp(lot.quantity) > 0 {
			take.Set(lot.quantity)
		}
		if withBasis {
			if lot.basis == nil {
				unknown = true
			} else {
				portion := new(big.Rat).Mul(lot.basis, new(big.Rat).Quo(take, lot.quantity))
				disposed.Add(disposed, portion)
				lot.basis.Sub(lot.basis, portion)
			}
		}
		lot.quantity.Sub(lot.quantity, take)
		remaining.Sub(remaining, take)
	}
	if unknown {
		return nil
	}
	return disposed
}

func ratInt(t *testing.T, text string) *big.Int {
	t.Helper()
	value, ok := new(big.Int).SetString(text, 10)
	require.True(t, ok, text)
	return value
}

func codeOf(t *testing.T, files map[string][]byte, commodityID string) string {
	t.Helper()
	for _, row := range csvRecords(t, files, "commodities.csv") {
		if row["commodity_id"] == commodityID {
			return row["code"]
		}
	}
	t.Fatalf("no commodity %s", commodityID)
	return ""
}
