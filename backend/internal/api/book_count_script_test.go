package api

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The D11 count script (#449) is run by the owner on a copy of a backup of
// their own book. Here it runs on a backup of the #266 reference book plus a
// few objects the reference book lacks (institutions, a brokerage with cash
// and holdings, a nested category, tags, payees and an instrument never held),
// and every figure below is derived from those steps by hand.

const bookCountScript = "../../../scripts/count-book.py"

// bookCountExtras adds what the reference book does not have. Names start with
// "Count" so the privacy check can look for them in the output.
var bookCountExtras = []struct {
	method, path, body, capture string
}{
	{"POST", "/api/v1/institutions", `{"name":"Count bank","kind":"bank","country_code":"NL"}`, "count_bank"},
	{"POST", "/api/v1/institutions", `{"name":"Count broker","kind":"brokerage","country_code":"IE"}`, "count_broker"},
	{"POST", "/api/v1/institutions", `{"name":"Count old bank","kind":"bank"}`, "count_old_bank"},
	{"POST", "/api/v1/institutions/@count_old_bank/archive", `{"change_reason":"count"}`, ""},
	{"POST", "/api/v1/accounts", `{"name":"Count current","account_class":"asset","account_kind":"checking","institution_id":"@count_bank","default_commodity_id":"@EUR","allows_postings":true,"opened_on":"2026-01-01","effective_from":"2026-01-01"}`, "count_current"},
	{"POST", "/api/v1/accounts", `{"name":"Count brokerage","account_class":"asset","account_kind":"brokerage","institution_id":"@count_broker","default_commodity_id":"@USD","allows_postings":false,"opened_on":"2026-01-01","effective_from":"2026-01-01"}`, "count_brokerage"},
	{"POST", "/api/v1/accounts", `{"name":"Count brokerage cash","account_class":"asset","account_kind":"brokerage_cash","parent_account_id":"@count_brokerage","institution_id":"@count_broker","default_commodity_id":"@USD","quantity_scale_override":2,"allows_postings":true,"opened_on":"2026-01-01","effective_from":"2026-01-01"}`, "count_broker_cash"},
	{"POST", "/api/v1/investments/instruments", `{"commodity_code":"IDLE","instrument_type":"etf","display_name":"Count idle fund","symbol":"IDLE","quote_commodity_id":"@USD","quantity_scale":4,"price_scale":4,"identifiers":{},"metadata":{}}`, "idle_instrument"},
	{"POST", "/api/v1/investments/holding-accounts", `{"instrument_id":"@ref_instrument","name":"Count REF holding","parent_account_id":"@count_brokerage","opened_on":"2026-01-01","effective_from":"2026-01-01","cost_basis_method":"fifo"}`, "count_hold_ref"},
	{"POST", "/api/v1/investments/holding-accounts", `{"instrument_id":"@idle_instrument","name":"Count IDLE holding","parent_account_id":"@count_brokerage","opened_on":"2026-01-01","effective_from":"2026-01-01","cost_basis_method":"fifo"}`, "count_hold_idle"},
	// One trade inside the brokerage: a posting on its holding and one on its cash.
	{"POST", "/api/v1/investments/buy", `{"transaction_date":"2026-03-20","commodity_id":"@REF","holding_account_id":"@count_hold_ref","cash_account_id":"@count_broker_cash","quantity_value":"10000","quantity_scale":4,"cash_amount_value":"50000","cash_amount_scale":2,"cash_commodity_id":"@USD"}`, ""},
	{"POST", "/api/v1/categories", `{"name":"Count housing","category_type":"expense"}`, "count_housing"},
	{"POST", "/api/v1/categories", `{"name":"Count energy","category_type":"expense","parent_category_id":"@count_housing"}`, ""},
	{"POST", "/api/v1/tags", `{"name":"Count trip","kind":"project"}`, ""},
	{"POST", "/api/v1/tags", `{"name":"Count old flag","kind":"flag"}`, "count_old_flag"},
	{"POST", "/api/v1/tags/@count_old_flag/archive", ``, ""},
	{"POST", "/api/v1/payees", `{"name":"Count grocer"}`, ""},
	{"POST", "/api/v1/payees", `{"name":"Count old shop"}`, "count_old_shop"},
	{"POST", "/api/v1/payees/@count_old_shop/archive", `{"change_reason":"count"}`, ""},
}

// bookCountBackup seeds the reference book and the extras, takes a backup
// through the product's backup run and copies it where no lock file sits
// beside it, as the owner would.
func bookCountBackup(t *testing.T) (string, recoveryAuditBook) {
	t.Helper()
	b := seedRecoveryAuditBook(t)
	for _, step := range bookCountExtras {
		var body json.RawMessage
		if step.body != "" {
			body = b.seeded.resolveBody(t, json.RawMessage(step.body))
		}
		path := b.seeded.resolvePath(t, step.path)
		res := b.seeded.request(t, b.instance.handler, step.method, path, body)
		require.Less(t, res.Code, 300, "%s %s: %s", step.method, path, res.Body.String())
		if step.capture != "" {
			var created struct {
				ID int64 `json:"id"`
			}
			require.NoError(t, json.Unmarshal(res.Body.Bytes(), &created))
			require.NotZero(t, created.ID, step.capture)
			b.seeded.ids[step.capture] = created.ID
		}
	}
	backup := b.backUp(t)
	raw, err := os.ReadFile(backup)
	require.NoError(t, err)
	copied := filepath.Join(t.TempDir(), "copy.sqlite")
	require.NoError(t, os.WriteFile(copied, raw, 0o600))
	return copied, b
}

func runBookCount(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	python, err := exec.LookPath("python3")
	require.NoError(t, err, "the D11 script needs python3, as the owner's run does")
	cmd := exec.Command(python, append([]string{"-I", bookCountScript}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}

func bookCountReport(t *testing.T, copied string, args ...string) map[string]json.RawMessage {
	t.Helper()
	out, stderr, err := runBookCount(t, append(append([]string{"--json"}, args...), copied)...)
	require.NoError(t, err, stderr)
	var report map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	return report
}

func bookCountSection(t *testing.T, report map[string]json.RawMessage, key string) map[string]json.RawMessage {
	t.Helper()
	var section map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(report[key], &section), key)
	return section
}

func TestBookCountScriptCountsTheBackupByHand(t *testing.T) {
	t.Parallel()
	copied, _ := bookCountBackup(t)
	before, err := os.ReadFile(copied)
	require.NoError(t, err)

	// The snapshot takes every object as it stands in the backup: its version
	// in force today, when the archives above took effect, and its open lots.
	// The default window ends today, months after the last posting.
	report := bookCountReport(t, copied)
	assert.JSONEq(t, `1`, string(report["books"]))
	snapshot := bookCountSection(t, report, "snapshot")
	assert.JSONEq(t, `"`+time.Now().Format(time.DateOnly)+`"`, string(snapshot["versions_in_force_on"]))
	var schemaVersion int
	require.NoError(t, json.Unmarshal(report["schema_version"], &schemaVersion))
	assert.Positive(t, schemaVersion, "the copy's migration head")

	// Money accounts: the reference book's seven (Ref bank, checking, savings,
	// dollar checking, card, emergency fund and old cash, archived), setup's
	// starter cash accounts for EUR and USD, and the extras' current account,
	// brokerage and brokerage cash. Holdings: four reference holdings outside any
	// brokerage, two extras inside it. Index rows fold the brokerage cash into
	// its brokerage and list each outside holding as its own row: 11 + 4.
	assert.JSONEq(t, `{
		"by_group_and_status": {
			"category": {"active": 8},
			"holding": {"active": 6},
			"money": {"active": 11, "archived": 1},
			"system": {"active": 8}
		},
		"postable_by_class_kind_status": {
			"asset": {
				"brokerage_cash": {"active": 1},
				"cash": {"active": 2, "archived": 1},
				"checking": {"active": 3},
				"savings": {"active": 2},
				"security_holding": {"active": 6}
			},
			"liability": {"credit_card": {"active": 1}}
		},
		"non_postable_money_accounts": 2,
		"account_index_rows": {
			"total": 15,
			"by_status": {"active": 14, "archived": 1},
			"by_kind": {
				"brokerage": 1, "cash": 3, "checking": 3, "credit_card": 1,
				"other_money_account": 1, "savings": 2, "security_holding": 4
			},
			"holdings_listed_as_their_own_row": 4
		},
		"brokerages": 1,
		"holdings_per_brokerage": [[2, 0]],
		"holdings_outside_a_brokerage": {"active": 4}
	}`, string(snapshot["accounts"]))
	// Count bank holds the current account, Count broker the brokerage, and the
	// archived bank nothing; the other 13 rows have no institution.
	assert.JSONEq(t, `{
		"total": 3,
		"by_kind_and_status": {"bank": {"active": 1, "archived": 1}, "brokerage": {"active": 1}},
		"account_index_rows_per_institution": [1, 1, 0],
		"account_index_rows_without_institution": 13
	}`, string(snapshot["institutions"]))
	// Groceries, dining, travel, borrow costs and Count housing at the top,
	// Count energy below housing; salary and dividends.
	assert.JSONEq(t, `{
		"total": 8,
		"by_type_depth_status": {
			"expense": {"depth_1": {"active": 5}, "depth_2": {"active": 1}},
			"income": {"depth_1": {"active": 2}}
		}
	}`, string(snapshot["categories"]))
	assert.JSONEq(t, `{"total": 2, "by_kind_and_status": {"flag": {"archived": 1}, "project": {"active": 1}}}`, string(snapshot["tags"]))
	assert.JSONEq(t, `{"total": 2, "by_status": {"active": 1, "archived": 1}}`, string(snapshot["payees"]))
	// REF keeps open long lots, SHRT only the short left after the partial
	// cover, and IDLE has a holding account but was never bought.
	assert.JSONEq(t, `{
		"total": 3,
		"by_holding_and_status": {"held": {"active": 1}, "held_short": {"active": 1}, "not_held": {"active": 1}},
		"by_type": {"etf": 1, "stock": 2}
	}`, string(snapshot["instruments"]))
	// Pins are offered from the ten active rows that are not holdings (D7): the
	// 14 active index rows less the four reference holdings listed on their own.
	window := bookCountSection(t, report, "postings_in_window")
	assert.JSONEq(t, `0`, string(window["transactions"]))
	assert.JSONEq(t, `{"0": 10, "1-4": 0, "5-19": 0, "20-49": 0, "50-99": 0, "100+": 0}`, string(window["pin_eligible_account_rows"]))
	assert.JSONEq(t, `[0, 0, 0, 0, 0, 0, 0, 0, 0, 0]`, string(window["pin_eligible_account_rows_largest_first"]))

	// The window is the 90 days ending on --window-end. The last reference
	// posting is dated 2026-03-16, so a window starting 2026-03-17 holds only
	// the extras' buy of 2026-03-20: the holding, the brokerage cash and the two
	// system trading lines. The brokerage counts its holding and cash as one
	// row; the reference holdings, whose sales fall before the window, are not
	// pin candidates at all.
	onlyTheBuy := `{
		"transactions": 1, "postings": 4,
		"pin_eligible_account_rows": {"0": 9, "1-4": 1, "5-19": 0, "20-49": 0, "50-99": 0, "100+": 0},
		"pin_eligible_account_rows_largest_first": [2, 0, 0, 0, 0, 0, 0, 0, 0, 0],
		"active_categories": {"0": 8, "1-4": 0, "5-19": 0, "20-49": 0, "50-99": 0, "100+": 0}
	}`
	for end, from := range map[string]string{"2026-06-14": "2026-03-17", "2026-06-17": "2026-03-20"} {
		figures := bookCountReport(t, copied, "--window-end", end)
		window := bookCountSection(t, figures, "postings_in_window")
		assert.JSONEq(t, `"`+from+`"`, string(window["from"]), end)
		assert.JSONEq(t, `"`+end+`"`, string(window["to"]), end)
		assert.JSONEq(t, `90`, string(window["days"]), end)
		for _, key := range []string{"from", "to", "days"} {
			delete(window, key)
		}
		rest, err := json.Marshal(window)
		require.NoError(t, err)
		assert.JSONEq(t, onlyTheBuy, string(rest), end)
	}
	past := bookCountSection(t, bookCountReport(t, copied, "--window-end", "2026-06-18"), "postings_in_window")
	assert.JSONEq(t, `"2026-03-21"`, string(past["from"]))
	assert.JSONEq(t, `0`, string(past["transactions"]), "the buy's day has left the window")

	// The window end moves only the window. On 2026-01-01 no position was open
	// yet, but the snapshot still reports the backup's open lots, labelled as such.
	early := bookCountReport(t, copied, "--window-end", "2026-01-01")
	assert.JSONEq(t, string(report["snapshot"]), string(early["snapshot"]))

	after, err := os.ReadFile(copied)
	require.NoError(t, err)
	assert.Equal(t, before, after, "the copy is opened immutable")
	entries, err := os.ReadDir(filepath.Dir(copied))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "nothing is written beside the copy")
}

// TestBookCountScriptPrintsNoNamesOrAmounts reads both output forms for every
// name, symbol, description and amount the steps entered. In JSON every key is
// a schema code and every value a count or a date.
func TestBookCountScriptPrintsNoNamesOrAmounts(t *testing.T) {
	t.Parallel()
	copied, b := bookCountBackup(t)

	secrets := map[string]bool{}
	var collect func(any)
	collect = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, item := range typed {
				switch key {
				case "name", "display_name", "symbol", "commodity_code", "description", "quantity_value",
					"cash_amount_value", "amount_value", "price_value", "statement_balance_value", "country_code":
					if text, ok := item.(string); ok && len(text) > 1 {
						secrets[strings.ReplaceAll(text, "{suffix}", "")] = true
					}
				}
				collect(item)
			}
		case []any:
			for _, item := range typed {
				collect(item)
			}
		}
	}
	for _, step := range b.seeded.book.Steps {
		var body any
		if len(step.Body) > 0 {
			require.NoError(t, json.Unmarshal(step.Body, &body))
			collect(body)
		}
		if step.Reconcile != nil {
			secrets[step.Reconcile.StatementBalanceValue] = true
		}
	}
	for _, step := range bookCountExtras {
		if step.body != "" {
			var body any
			require.NoError(t, json.Unmarshal([]byte(step.body), &body))
			collect(body)
		}
	}
	require.Contains(t, secrets, "Ref checking")
	require.Contains(t, secrets, "Count idle fund")
	require.Contains(t, secrets, "250000")

	for _, asOf := range []string{"2026-03-31", "2099-12-31"} {
		text, stderr, err := runBookCount(t, "--window-end", asOf, copied)
		require.NoError(t, err, stderr)
		jsonOut, stderr, err := runBookCount(t, "--json", "--window-end", asOf, copied)
		require.NoError(t, err, stderr)
		for secret := range secrets {
			// Short digit runs are counts; only amounts of four digits or more are
			// distinctive enough to look for.
			if regexp.MustCompile(`^\d{1,3}$`).MatchString(secret) {
				continue
			}
			assert.NotContains(t, text, secret, asOf)
			assert.NotContains(t, jsonOut, secret, asOf)
		}

		var report any
		require.NoError(t, json.Unmarshal([]byte(jsonOut), &report))
		code := regexp.MustCompile(`^[a-z0-9_+-]+$`)
		date := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
		var check func(string, any)
		check = func(path string, value any) {
			switch typed := value.(type) {
			case map[string]any:
				for key, item := range typed {
					assert.Regexp(t, code, key, "%s: keys are schema codes", path)
					check(path+"."+key, item)
				}
			case []any:
				for _, item := range typed {
					check(path+"[]", item)
				}
			case float64:
				assert.Equal(t, float64(int64(typed)), typed, "%s: counts are whole", path)
			case string:
				assert.Regexp(t, date, typed, "%s: the only strings are dates", path)
			default:
				t.Errorf("%s: unexpected value %v", path, value)
			}
		}
		check("report", report)
	}
}

func TestBookCountScriptRefusesALiveDatabase(t *testing.T) {
	t.Parallel()
	copied, b := bookCountBackup(t)

	// The database the instance is serving keeps its WAL beside it; `serve`
	// also leaves its lock file there, covered below.
	require.FileExists(t, b.databasePath+"-wal")
	_, stderr, err := runBookCount(t, b.databasePath)
	assert.Error(t, err)
	assert.Contains(t, stderr, "refused:")
	assert.Contains(t, stderr, "looks like a live database")

	// A link elsewhere has no sidecars of its own beside it. A symbolic link is
	// followed to the live file; a hard link is the live file under a second name.
	links := t.TempDir()
	symlink := filepath.Join(links, "symlink.sqlite")
	require.NoError(t, os.Symlink(b.databasePath, symlink))
	out, stderr, err := runBookCount(t, symlink)
	assert.Error(t, err)
	assert.Empty(t, out)
	assert.Contains(t, stderr, b.databasePath+"-wal exists")
	hardlink := filepath.Join(filepath.Dir(b.databasePath), "second-name.sqlite")
	if err := os.Link(b.databasePath, hardlink); err == nil {
		t.Cleanup(func() { _ = os.Remove(hardlink) })
		out, stderr, err = runBookCount(t, hardlink)
		assert.Error(t, err)
		assert.Empty(t, out)
		assert.Contains(t, stderr, "has another name")
	} else {
		t.Logf("hard link not possible here: %v", err)
	}
	// A symbolic link to a clean copy is fine: what counts is the file it reaches.
	copyLink := filepath.Join(links, "copy-link.sqlite")
	require.NoError(t, os.Symlink(copied, copyLink))
	_, stderr, err = runBookCount(t, copyLink)
	assert.NoError(t, err, stderr)

	// SQLite compares dates as text, so only canonical days are accepted.
	for _, day := range []string{"20260319", "2026-3-19", "2026-02-30"} {
		out, stderr, err := runBookCount(t, "--window-end", day, copied)
		assert.Error(t, err, day)
		assert.Empty(t, out, day)
		assert.Contains(t, stderr, "--window-end must be a YYYY-MM-DD date", day)
	}

	for _, suffix := range []string{".lock", "-wal", "-shm"} {
		require.NoError(t, os.WriteFile(copied+suffix, nil, 0o600))
		out, stderr, err := runBookCount(t, copied)
		assert.Error(t, err, suffix)
		assert.Empty(t, out, suffix)
		assert.Contains(t, stderr, copied+suffix+" exists", suffix)
		require.NoError(t, os.Remove(copied+suffix))
	}

	empty := filepath.Join(t.TempDir(), "other.sqlite")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	_, stderr, err = runBookCount(t, empty)
	assert.Error(t, err)
	assert.Contains(t, stderr, "is not a Rekenraam database")
}
