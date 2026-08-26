package app

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// ExportService writes the portable ledger: the shape a user leaves with.
//
// Two rules govern everything here and are worth stating before the code says
// it obliquely:
//
//  1. The export includes system accounts. Reports exclude commodity_trading
//     and friends because a report answers a human question; an export that
//     dropped them would emit transactions that do not balance.
//  2. Amounts are rendered from the stored coefficient and scale through
//     exact.Decimal. No float, no locale, no display formatter.
type ExportService struct {
	repository *db.ExportRepository
	now        func() time.Time
}

func NewExportService(repository *db.ExportRepository) *ExportService {
	return &ExportService{repository: repository, now: time.Now}
}

func (s *ExportService) SetNowForTest(now func() time.Time) { s.now = now }

// LedgerExportColumns is the stable schema of ledger.csv, in order. It is a
// published contract (ADR 0011): columns are appended, never reordered or
// renamed, and the preview endpoint serves this same list so a consumer can
// check what it is about to parse.
var LedgerExportColumns = []string{
	"transaction_id",
	"transaction_version_id",
	"transaction_kind",
	"status",
	"transaction_date",
	"correction_of_transaction_id",
	"payee_id",
	"payee",
	"description",
	"external_ref_hint",
	"needs_review",
	"transaction_tags",
	"transaction_complete",
	"journal_entry_id",
	"entry_seq",
	"entry_kind",
	"entry_date",
	"entry_memo",
	"posting_line_key",
	"line_seq",
	"account_id",
	"account_path",
	"account_class",
	"account_kind",
	"quantity",
	"commodity_id",
	"commodity",
	"commodity_kind",
	"posting_memo",
	"reconciliation_status",
	"cleared_on",
	"posting_tags",
}

// utf8ByteOrderMark is written ahead of the header so a spreadsheet opens the
// file as UTF-8. Spelled by code point rather than pasted, because a literal
// BOM in a Go source file is a compile error.
const utf8ByteOrderMark = "\uFEFF"

// ExportedRecordPolicy states, in one sentence a manifest can carry verbatim,
// which ledger records leave through the export.
const ExportedRecordPolicy = "the current version of every posted, non-deleted transaction; drafts, voided versions, superseded versions, and soft-deleted records are not exported"

// ExportSelectionUnit names what a filter selects. It is the journal entry,
// never the individual posting, because selecting postings would break the
// per-entry balance the export guarantees.
const ExportSelectionUnit = "journal_entry"

// ExportExcludedData is what a portable export deliberately leaves behind. The
// list is served with every preview so nobody has to infer it: this is an
// export of the ledger, not a copy of the installation.
var ExportExcludedData = []string{
	"credentials and secrets",
	"authentication sessions and multi-factor enrolment",
	"audit events and authentication events",
	"import profiles, batches, and staged rows",
	"connection configuration",
	"background work items",
	"superseded, draft, and voided transaction versions",
}

// ExportFilter is a scoped export's request, as the caller wrote it.
//
// It is validated and expanded into a db.LedgerExportSelection before any read;
// the expansion is echoed back so a manifest can state what the filter actually
// resolved to rather than only what was asked for.
type ExportFilter struct {
	From               string
	To                 string
	DateBasis          string
	AccountIDs         []int64
	IncludeDescendants bool
	CommodityIDs       []int64
}

// "Is anything restricted?" is answered once, by
// db.LedgerExportSelection.Filtered, on the resolved selection rather than on
// the request. A second copy here had no callers and would have drifted the
// moment one rule gained a dimension the other did not — which is how the
// decimal-comma bug shipped three times (T-36/T-45/T-47).

// ResolvedExportFilter is what the filter became: the normalized request plus
// the account expansion, so an archive can be reproduced from its own manifest.
type ResolvedExportFilter struct {
	From               string   `json:"from,omitempty"`
	To                 string   `json:"to,omitempty"`
	DateBasis          string   `json:"date_basis"`
	AccountIDs         []int64  `json:"account_ids"`
	IncludeDescendants bool     `json:"include_descendants"`
	ResolvedAccountIDs []int64  `json:"resolved_account_ids"`
	CommodityIDs       []int64  `json:"commodity_ids"`
	SelectionUnit      string   `json:"selection_unit"`
	Notes              []string `json:"notes,omitempty"`
}

// resolveExportFilter validates the request and expands the account selection
// against the same snapshot the export reads, so resolution and content cannot
// disagree.
//
// Unlike the reports filter it accepts any account class: "everything that
// touched Groceries" is a reasonable export and an unreasonable net-worth
// chart. Descendants expand from current parent links, matching the account
// paths the export writes.
func (s *ExportService) resolveExportFilter(ctx context.Context, snapshot *sql.Tx, filter ExportFilter, accounts []db.ExportAccountRecord) (db.LedgerExportSelection, ResolvedExportFilter, error) {
	from, err := cleanExportDate(filter.From, "from")
	if err != nil {
		return db.LedgerExportSelection{}, ResolvedExportFilter{}, err
	}
	to, err := cleanExportDate(filter.To, "to")
	if err != nil {
		return db.LedgerExportSelection{}, ResolvedExportFilter{}, err
	}
	if from != "" && to != "" && from > to {
		return db.LedgerExportSelection{}, ResolvedExportFilter{}, ValidationError{Message: "export range starts after it ends"}
	}

	basis := strings.TrimSpace(filter.DateBasis)
	if basis == "" {
		basis = db.DateBasisEntry
	}
	if basis != db.DateBasisEntry && basis != db.DateBasisTransaction {
		return db.LedgerExportSelection{}, ResolvedExportFilter{}, ValidationError{Message: "date_basis must be entry or transaction"}
	}

	accountIDs := dedupeIDs(filter.AccountIDs)
	for _, accountID := range accountIDs {
		exists, err := s.repository.AccountExistsInBook(ctx, snapshot, BookID, accountID)
		if err != nil {
			return db.LedgerExportSelection{}, ResolvedExportFilter{}, err
		}
		if !exists {
			return db.LedgerExportSelection{}, ResolvedExportFilter{}, ValidationError{Message: "account filter is invalid"}
		}
	}

	commodityIDs := dedupeIDs(filter.CommodityIDs)
	for _, commodityID := range commodityIDs {
		exists, err := s.repository.CommodityExistsInBook(ctx, snapshot, BookID, commodityID)
		if err != nil {
			return db.LedgerExportSelection{}, ResolvedExportFilter{}, err
		}
		if !exists {
			return db.LedgerExportSelection{}, ResolvedExportFilter{}, ValidationError{Message: "commodity filter is invalid"}
		}
	}

	resolvedAccountIDs := accountIDs
	if filter.IncludeDescendants && len(accountIDs) > 0 {
		resolvedAccountIDs = expandAccountDescendants(accounts, accountIDs)
	}

	selection := db.LedgerExportSelection{
		From:         from,
		To:           to,
		DateBasis:    basis,
		AccountIDs:   resolvedAccountIDs,
		CommodityIDs: commodityIDs,
	}

	resolved := ResolvedExportFilter{
		From:               from,
		To:                 to,
		DateBasis:          basis,
		AccountIDs:         emptyIfNilIDs(accountIDs),
		IncludeDescendants: filter.IncludeDescendants,
		ResolvedAccountIDs: emptyIfNilIDs(resolvedAccountIDs),
		CommodityIDs:       emptyIfNilIDs(commodityIDs),
		SelectionUnit:      ExportSelectionUnit,
	}
	if selection.Filtered() {
		resolved.Notes = append(resolved.Notes,
			"a filter selects whole journal entries, so this export also carries postings in accounts, commodities, and dates it did not name")
	}
	if basis == db.DateBasisTransaction {
		resolved.Notes = append(resolved.Notes,
			"transaction basis: a transaction in range brings every one of its entries, including entries dated outside the range")
	}

	return selection, resolved, nil
}

func cleanExportDate(value string, field string) (string, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return "", nil
	}
	if _, err := time.Parse(time.DateOnly, cleaned); err != nil {
		return "", ValidationError{Message: field + " must use YYYY-MM-DD"}
	}
	return cleaned, nil
}

// expandAccountDescendants walks the current parent links so an account filter
// means the subtree the user sees, and cannot loop on a cyclic chain.
func expandAccountDescendants(accounts []db.ExportAccountRecord, accountIDs []int64) []int64 {
	children := map[int64][]int64{}
	for _, account := range accounts {
		if account.ParentAccountID.Valid {
			children[account.ParentAccountID.Int64] = append(children[account.ParentAccountID.Int64], account.AccountID)
		}
	}

	selected := map[int64]bool{}
	queue := append([]int64(nil), accountIDs...)
	for len(queue) > 0 {
		accountID := queue[0]
		queue = queue[1:]
		if selected[accountID] {
			continue
		}
		selected[accountID] = true
		queue = append(queue, children[accountID]...)
	}

	expanded := make([]int64, 0, len(selected))
	for accountID := range selected {
		expanded = append(expanded, accountID)
	}
	sort.Slice(expanded, func(i, j int) bool { return expanded[i] < expanded[j] })
	return expanded
}

func emptyIfNilIDs(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

// LedgerExportPreview answers "what would I get" before a byte is written. A
// streamed response cannot change its status code after the first byte, so
// this is also where a caller learns that its request is sound.
type LedgerExportPreview struct {
	GeneratedAt                string
	SelectionUnit              string
	RecordPolicy               string
	Filter                     ResolvedExportFilter
	AllTransactionsComplete    bool
	IncompleteTransactionCount int64
	IncludesSystemAccounts     bool
	Columns                    []string
	Excluded                   []string
	PostingCount               int64
	JournalEntryCount          int64
	TransactionCount           int64
	AccountCount               int64
	CommodityCount             int64
	EarliestEntryDate          string
	LatestEntryDate            string
	Attachments                ExportAttachments
}

// ExportAttachments is the hook R14a fills. It is declared empty rather than
// omitted so the export never has to grow a coverage claim later — and so a
// consumer reading today's manifest already knows attachments are not in it.
type ExportAttachments struct {
	Included  bool
	Directory *string
	Reason    string
}

func emptyAttachments() ExportAttachments {
	return ExportAttachments{Included: false, Directory: nil, Reason: "not implemented"}
}

// ValidateFilter resolves a filter and throws the result away.
//
// It exists so a streaming handler can reject a bad request while an error
// envelope is still possible, without relying on the unstated promise that
// WriteBundle happens to validate before its first byte.
func (s *ExportService) ValidateFilter(ctx context.Context, filter ExportFilter) error {
	snapshot, err := s.repository.Snapshot(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = snapshot.Rollback() }()

	accounts, err := s.repository.ExportAccounts(ctx, snapshot, BookID)
	if err != nil {
		return err
	}
	_, _, err = s.resolveExportFilter(ctx, snapshot, filter, accounts)
	return err
}

func (s *ExportService) Preview(ctx context.Context, filter ExportFilter) (LedgerExportPreview, error) {
	snapshot, err := s.repository.Snapshot(ctx)
	if err != nil {
		return LedgerExportPreview{}, err
	}
	defer func() { _ = snapshot.Rollback() }()

	accounts, err := s.repository.ExportAccounts(ctx, snapshot, BookID)
	if err != nil {
		return LedgerExportPreview{}, err
	}
	selection, resolved, err := s.resolveExportFilter(ctx, snapshot, filter, accounts)
	if err != nil {
		return LedgerExportPreview{}, err
	}

	totals, err := s.repository.LedgerExportTotals(ctx, snapshot, BookID, selection)
	if err != nil {
		return LedgerExportPreview{}, err
	}

	return LedgerExportPreview{
		GeneratedAt:   s.now().UTC().Format(time.RFC3339),
		SelectionUnit: ExportSelectionUnit,
		RecordPolicy:  ExportedRecordPolicy,
		Filter:        resolved,
		// Archive-wide, not per transaction: the per-row transaction_complete
		// column carries the same fact per transaction. An unfiltered export is
		// complete by construction.
		AllTransactionsComplete:    totals.IncompleteTransactionCount == 0,
		IncompleteTransactionCount: totals.IncompleteTransactionCount,
		IncludesSystemAccounts:     true,
		Columns:                    LedgerExportColumns,
		Excluded:                   ExportExcludedData,
		PostingCount:               totals.PostingCount,
		JournalEntryCount:          totals.JournalEntryCount,
		TransactionCount:           totals.TransactionCount,
		AccountCount:               totals.AccountCount,
		CommodityCount:             totals.CommodityCount,
		EarliestEntryDate:          totals.EarliestEntryDate.String,
		LatestEntryDate:            totals.LatestEntryDate.String,
		Attachments:                emptyAttachments(),
	}, nil
}

// WriteLedgerCSV streams the whole posted ledger and returns the number of
// posting rows written.
//
// It is unfiltered by design: a downloaded flat file cannot carry the manifest
// that would make a scoped export reproducible, so scope belongs to bundle.zip
// (ADR 0011). Rows are written as they are read — nothing is buffered — and the
// read runs in one snapshot, so a save landing mid-export cannot tear it.
func (s *ExportService) WriteLedgerCSV(ctx context.Context, out io.Writer) (int64, error) {
	snapshot, err := s.repository.Snapshot(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = snapshot.Rollback() }()

	accounts, err := s.repository.ExportAccounts(ctx, snapshot, BookID)
	if err != nil {
		return 0, err
	}
	paths := accountExportPaths(accounts)

	// A BOM so a spreadsheet opens UTF-8 correctly; every other consumer
	// tolerates one. Written before the header, never between rows.
	if _, err := io.WriteString(out, utf8ByteOrderMark); err != nil {
		return 0, fmt.Errorf("write export byte order mark: %w", err)
	}

	writer := csv.NewWriter(out)
	if err := writer.Write(LedgerExportColumns); err != nil {
		return 0, fmt.Errorf("write export header: %w", err)
	}

	var written int64
	streamErr := s.repository.StreamLedgerPostings(ctx, snapshot, BookID, db.LedgerExportSelection{}, func(record db.LedgerExportPostingRecord) error {
		if err := writer.Write(ledgerExportRow(record, paths)); err != nil {
			return fmt.Errorf("write export row: %w", err)
		}
		written++
		return nil
	})
	if streamErr != nil {
		return written, streamErr
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return written, fmt.Errorf("flush export: %w", err)
	}

	return written, nil
}

func ledgerExportRow(record db.LedgerExportPostingRecord, paths map[int64]string) []string {
	return []string{
		strconv.FormatInt(record.TransactionID, 10),
		strconv.FormatInt(record.TransactionVersionID, 10),
		record.TransactionKind,
		record.Status,
		record.TransactionDate,
		nullableID(record.CorrectionOfTransactionID),
		nullableID(record.PayeeID),
		record.PayeeName,
		record.Description,
		record.ExternalRefHint.String,
		boolToken(record.NeedsReview),
		record.TransactionTags.String,
		boolToken(record.TransactionComplete),
		strconv.FormatInt(record.JournalEntryID, 10),
		strconv.FormatInt(record.EntrySeq, 10),
		record.EntryKind,
		record.EntryDate,
		record.EntryMemo,
		record.PostingLineKey,
		strconv.FormatInt(record.LineSeq, 10),
		strconv.FormatInt(record.AccountID, 10),
		paths[record.AccountID],
		record.AccountClass,
		record.AccountKind,
		exact.Decimal(record.QuantityValue, record.QuantityScale),
		strconv.FormatInt(record.CommodityID, 10),
		record.CommodityCode,
		record.CommodityKind,
		record.PostingMemo,
		record.ReconciliationStatus,
		record.ClearedOn.String,
		record.PostingTags.String,
	}
}

// accountExportPaths builds the colon-separated hierarchy every plain-text
// accounting tool expects ("Assets:Bank:Checking").
//
// The path uses each account's *current* name and parent. An as-of-entry-date
// path would let one account_id print under two different paths in one file,
// which no consumer could join on; account_id remains the stable key, and the
// export documents this choice.
func accountExportPaths(accounts []db.ExportAccountRecord) map[int64]string {
	byID := make(map[int64]db.ExportAccountRecord, len(accounts))
	for _, account := range accounts {
		byID[account.AccountID] = account
	}

	paths := make(map[int64]string, len(accounts))
	for _, account := range accounts {
		paths[account.AccountID] = buildAccountPath(account, byID)
	}
	return paths
}

func buildAccountPath(account db.ExportAccountRecord, byID map[int64]db.ExportAccountRecord) string {
	segments := []string{accountExportSegment(account)}
	seen := map[int64]bool{account.AccountID: true}

	current := account
	for current.ParentAccountID.Valid {
		parent, ok := byID[current.ParentAccountID.Int64]
		// A cycle cannot be created through the API, but a path builder that
		// loops forever on corrupt data would take the whole export with it.
		if !ok || seen[parent.AccountID] {
			break
		}
		seen[parent.AccountID] = true
		segments = append([]string{accountExportSegment(parent)}, segments...)
		current = parent
	}

	return strings.Join(segments, ":")
}

// accountExportSegment names one level of the path. A system account carries no
// user-facing name, so its role is the honest label; an account with neither is
// named by its id rather than leaving a blank segment that would collapse two
// different paths into one.
func accountExportSegment(account db.ExportAccountRecord) string {
	if name := strings.TrimSpace(account.Name.String); name != "" {
		return name
	}
	if code := strings.TrimSpace(account.Code.String); code != "" {
		return code
	}
	if role := strings.TrimSpace(account.SystemRole.String); role != "" {
		return role
	}
	return "account-" + strconv.FormatInt(account.AccountID, 10)
}

func nullableID(value sql.NullInt64) string {
	if !value.Valid {
		return ""
	}
	return strconv.FormatInt(value.Int64, 10)
}

// boolToken keeps CSV booleans as the lowercase tokens the schema promises,
// rather than Go's default formatting drifting from the contract later.
func boolToken(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
