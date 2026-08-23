package app

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
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

// LedgerExportPreview answers "what would I get" before a byte is written. A
// streamed response cannot change its status code after the first byte, so
// this is also where a caller learns that its request is sound.
type LedgerExportPreview struct {
	GeneratedAt             string
	SelectionUnit           string
	RecordPolicy            string
	AllTransactionsComplete bool
	IncludesSystemAccounts  bool
	Columns                 []string
	Excluded                []string
	PostingCount            int64
	JournalEntryCount       int64
	TransactionCount        int64
	AccountCount            int64
	CommodityCount          int64
	EarliestEntryDate       string
	LatestEntryDate         string
	Attachments             ExportAttachments
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

func (s *ExportService) Preview(ctx context.Context) (LedgerExportPreview, error) {
	snapshot, err := s.repository.Snapshot(ctx)
	if err != nil {
		return LedgerExportPreview{}, err
	}
	defer func() { _ = snapshot.Rollback() }()

	totals, err := s.repository.LedgerExportTotals(ctx, snapshot, BookID)
	if err != nil {
		return LedgerExportPreview{}, err
	}

	return LedgerExportPreview{
		GeneratedAt:   s.now().UTC().Format(time.RFC3339),
		SelectionUnit: ExportSelectionUnit,
		RecordPolicy:  ExportedRecordPolicy,
		// The flat CSV takes no filters, so every transaction is whole by
		// construction. Scoped exports arrive with bundle.zip, which carries
		// its own manifest.
		AllTransactionsComplete: true,
		IncludesSystemAccounts:  true,
		Columns:                 LedgerExportColumns,
		Excluded:                ExportExcludedData,
		PostingCount:            totals.PostingCount,
		JournalEntryCount:       totals.JournalEntryCount,
		TransactionCount:        totals.TransactionCount,
		AccountCount:            totals.AccountCount,
		CommodityCount:          totals.CommodityCount,
		EarliestEntryDate:       totals.EarliestEntryDate.String,
		LatestEntryDate:         totals.LatestEntryDate.String,
		Attachments:             emptyAttachments(),
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
	streamErr := s.repository.StreamLedgerPostings(ctx, snapshot, BookID, func(record db.LedgerExportPostingRecord) error {
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
		// Unfiltered: every entry of every transaction is present. Slice 2
		// computes this per transaction once filters exist.
		boolToken(true),
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
