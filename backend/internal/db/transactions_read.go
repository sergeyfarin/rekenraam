package db

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

func (r *TransactionRepository) ListTransactions(ctx context.Context, params ListTransactionsParams) ([]TransactionRecord, error) {
	where := []string{"t.book_id = ?", "t.deleted_at IS NULL"}
	args := []any{params.BookID}

	if params.Status != "" {
		where = append(where, "tv.status = ?")
		args = append(args, params.Status)
	} else if params.ExcludeDraft {
		where = append(where, "tv.status != 'draft'")
	}
	if params.Kind != "" {
		where = append(where, "tv.transaction_kind = ?")
		args = append(args, params.Kind)
	}
	if params.PayeeID > 0 {
		where = append(where, "tv.payee_id = ?")
		args = append(args, params.PayeeID)
	}
	if params.AccountID > 0 {
		where = append(where, `EXISTS (
			SELECT 1
			FROM journal_entries account_je
			JOIN posting_versions account_pv ON account_pv.journal_entry_id = account_je.id
			WHERE account_je.transaction_version_id = tv.id
				AND account_pv.account_id = ?
		)`)
		args = append(args, params.AccountID)
	}
	if params.NeedsReview {
		where = append(where, "tv.needs_review = 1")
	}

	// On the entry-date basis every posting-shaped filter has to be satisfied by
	// one and the same journal entry — that is what reproduces the set a
	// spending row was summed from. A transaction whose groceries entry sits
	// outside the range must not match a groceries row inside it, and separate
	// EXISTS clauses would let it through.
	if params.EntryDateBasis {
		clause, clauseArgs := matchingEntryClause(params)
		where = append(where, clause)
		args = append(args, clauseArgs...)
	} else {
		if params.CategoryID > 0 {
			where = append(where, `EXISTS (
			SELECT 1
			FROM journal_entries category_je
			JOIN posting_versions category_pv ON category_pv.journal_entry_id = category_je.id
			WHERE category_je.transaction_version_id = tv.id
				AND category_pv.account_id = ?
		)`)
			args = append(args, params.CategoryID)
		}
		if params.CategoryType != "" {
			clause, clauseArgs := categoryTypeClause(params.CategoryType)
			where = append(where, clause)
			args = append(args, clauseArgs...)
		}
		dateColumn := "tv.transaction_date"
		if params.FilterEntryDate {
			dateColumn = `(SELECT MAX(date_je.entry_date) FROM journal_entries date_je WHERE date_je.transaction_version_id = tv.id)`
		}
		if params.AfterDate != "" {
			where = append(where, dateColumn+" >= ?")
			args = append(args, params.AfterDate)
		}
		if params.BeforeDate != "" {
			where = append(where, dateColumn+" <= ?")
			args = append(args, params.BeforeDate)
		}
	}
	if params.CursorDate != "" && params.CursorID > 0 {
		where = append(where, `(
			tv.transaction_date < ?
			OR (tv.transaction_date = ? AND tv.transaction_day_sequence < ?)
			OR (tv.transaction_date = ? AND tv.transaction_day_sequence = ? AND t.id < ?)
		)`)
		args = append(args,
			params.CursorDate,
			params.CursorDate, params.CursorDaySequence,
			params.CursorDate, params.CursorDaySequence, params.CursorID,
		)
	}
	if strings.TrimSpace(params.Query) != "" {
		where = append(where, `tv.id IN (
			SELECT CAST(transaction_version_id AS INTEGER)
			FROM transaction_search
			WHERE transaction_search MATCH ?
		)`)
		args = append(args, ftsPhrase(params.Query))
	}

	args = append(args, params.Limit)
	rows, err := r.database.QueryContext(ctx, transactionSelect(`
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY tv.transaction_date DESC, tv.transaction_day_sequence DESC, t.id DESC
		LIMIT ?
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}
	defer rows.Close()

	records, err := scanTransactionRecords(rows)
	if err != nil {
		return nil, err
	}
	if err := r.loadTransactionChildren(ctx, records); err != nil {
		return nil, err
	}

	return records, nil
}

func (r *TransactionRepository) AccountRegister(ctx context.Context, params ListTransactionsParams) ([]AccountRegisterEntryRecord, error) {
	where := []string{"t.book_id = ?", "t.deleted_at IS NULL", "pv.account_id = ?"}
	args := []any{params.BookID, params.AccountID}

	if params.Status != "" {
		where = append(where, "tv.status = ?")
		args = append(args, params.Status)
	} else if params.ExcludeDraft {
		where = append(where, "tv.status != 'draft'")
	}
	if params.AfterDate != "" {
		where = append(where, "je.entry_date >= ?")
		args = append(args, params.AfterDate)
	}
	if params.BeforeDate != "" {
		where = append(where, "je.entry_date <= ?")
		args = append(args, params.BeforeDate)
	}
	if params.CursorDate != "" && params.CursorID > 0 {
		where = append(where, `(
			je.entry_date < ?
			OR (je.entry_date = ? AND pv.account_day_sequence < ?)
			OR (je.entry_date = ? AND pv.account_day_sequence = ? AND pv.id < ?)
		)`)
		args = append(args,
			params.CursorDate,
			params.CursorDate, params.CursorDaySequence,
			params.CursorDate, params.CursorDaySequence, params.CursorID,
		)
	}

	args = append(args, params.Limit)
	rows, err := r.database.QueryContext(ctx, accountRegisterSelect(`
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY je.entry_date DESC, pv.account_day_sequence DESC, pv.id DESC
		LIMIT ?
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list account register: %w", err)
	}
	defer rows.Close()

	entries, err := scanAccountRegisterEntries(rows)
	if err != nil {
		return nil, err
	}
	if err := r.loadAccountRegisterTags(ctx, entries); err != nil {
		return nil, err
	}

	return entries, nil
}

func (r *TransactionRepository) TransactionByID(ctx context.Context, bookID int64, transactionID int64) (TransactionRecord, error) {
	return r.transactionByID(ctx, bookID, transactionID, false)
}

func (r *TransactionRepository) TransactionByIDIncludingDeleted(ctx context.Context, bookID int64, transactionID int64) (TransactionRecord, error) {
	return r.transactionByID(ctx, bookID, transactionID, true)
}

func (r *TransactionRepository) transactionByID(ctx context.Context, bookID int64, transactionID int64, includeDeleted bool) (TransactionRecord, error) {
	var record TransactionRecord
	deletedCondition := " AND t.deleted_at IS NULL"
	if includeDeleted {
		deletedCondition = ""
	}
	if err := scanTransactionRecord(r.database.QueryRowContext(ctx, transactionSelect(`
		WHERE t.book_id = ? AND t.id = ?`+deletedCondition+`
	`), bookID, transactionID), &record); err != nil {
		return TransactionRecord{}, err
	}

	records := []TransactionRecord{record}
	if err := r.loadTransactionChildren(ctx, records); err != nil {
		return TransactionRecord{}, err
	}

	return records[0], nil
}

func (r *TransactionRepository) loadTransactionChildren(ctx context.Context, records []TransactionRecord) error {
	return loadTransactionChildrenQuery(ctx, r.database, records)
}

func (r *TransactionRepository) loadAccountRegisterTags(ctx context.Context, entries []AccountRegisterEntryRecord) error {
	for index := range entries {
		tagIDs, err := transactionTagIDs(ctx, r.database, entries[index].Transaction.ID)
		if err != nil {
			return err
		}
		entries[index].Transaction.TagIDs = tagIDs

		postingTagIDs, err := postingTagIDs(ctx, r.database, entries[index].Posting.PostingLineID)
		if err != nil {
			return err
		}
		entries[index].Posting.TagIDs = postingTagIDs
	}

	return nil
}

func loadTransactionChildrenTx(ctx context.Context, tx *sql.Tx, records []TransactionRecord) error {
	return loadTransactionChildrenQuery(ctx, tx, records)
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func loadTransactionChildrenQuery(ctx context.Context, queryer queryer, records []TransactionRecord) error {
	for index := range records {
		tagIDs, err := transactionTagIDs(ctx, queryer, records[index].ID)
		if err != nil {
			return err
		}
		records[index].TagIDs = tagIDs

		entries, err := journalEntriesForVersion(ctx, queryer, records[index].VersionID)
		if err != nil {
			return err
		}
		records[index].JournalEntries = entries
	}

	return nil
}

func transactionTagIDs(ctx context.Context, queryer queryer, transactionID int64) ([]int64, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT tag_id
		FROM transaction_tags
		WHERE transaction_id = ?
		ORDER BY tag_id
	`, transactionID)
	if err != nil {
		return nil, fmt.Errorf("read transaction tags: %w", err)
	}
	defer rows.Close()

	var tagIDs []int64
	for rows.Next() {
		var tagID int64
		if err := rows.Scan(&tagID); err != nil {
			return nil, fmt.Errorf("scan transaction tag: %w", err)
		}
		tagIDs = append(tagIDs, tagID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transaction tags: %w", err)
	}

	return tagIDs, nil
}

func journalEntriesForVersion(ctx context.Context, queryer queryer, transactionVersionID int64) ([]JournalEntryRecord, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT id, book_id, transaction_version_id, entry_seq, entry_date, entry_kind, memo, metadata_json
		FROM journal_entries
		WHERE transaction_version_id = ?
		ORDER BY entry_seq
	`, transactionVersionID)
	if err != nil {
		return nil, fmt.Errorf("read journal entries: %w", err)
	}
	defer rows.Close()

	var entries []JournalEntryRecord
	for rows.Next() {
		var entry JournalEntryRecord
		if err := rows.Scan(&entry.ID, &entry.BookID, &entry.TransactionVersionID, &entry.EntrySeq, &entry.EntryDate, &entry.EntryKind, &entry.Memo, &entry.MetadataJSON); err != nil {
			return nil, fmt.Errorf("scan journal entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate journal entries: %w", err)
	}

	for index := range entries {
		postings, err := postingsForJournalEntry(ctx, queryer, entries[index].ID)
		if err != nil {
			return nil, err
		}
		entries[index].Postings = postings
	}

	return entries, nil
}

func postingsForJournalEntry(ctx context.Context, queryer queryer, journalEntryID int64) ([]PostingRecord, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT
			pv.id,
			pv.book_id,
			pv.transaction_version_id,
			pv.journal_entry_id,
			pv.posting_line_id,
			pl.line_key,
			pv.line_seq,
			pv.account_id,
			pv.account_day_sequence,
			pv.quantity_value,
			pv.quantity_scale,
			pv.commodity_id,
			pv.memo,
			pv.reconciliation_status,
			pv.cleared_on,
			pv.metadata_json
		FROM posting_versions pv
		JOIN posting_lines pl ON pl.id = pv.posting_line_id
		WHERE pv.journal_entry_id = ?
		ORDER BY pv.line_seq
	`, journalEntryID)
	if err != nil {
		return nil, fmt.Errorf("read posting versions: %w", err)
	}
	defer rows.Close()

	var postings []PostingRecord
	for rows.Next() {
		var posting PostingRecord
		if err := rows.Scan(&posting.ID, &posting.BookID, &posting.TransactionVersionID, &posting.JournalEntryID, &posting.PostingLineID, &posting.LineKey, &posting.LineSeq, &posting.AccountID, &posting.AccountDaySequence, &posting.QuantityValue, &posting.QuantityScale, &posting.CommodityID, &posting.Memo, &posting.ReconciliationStatus, &posting.ClearedOn, &posting.MetadataJSON); err != nil {
			return nil, fmt.Errorf("scan posting version: %w", err)
		}
		postings = append(postings, posting)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate posting versions: %w", err)
	}

	for index := range postings {
		tagIDs, err := postingTagIDs(ctx, queryer, postings[index].PostingLineID)
		if err != nil {
			return nil, err
		}
		postings[index].TagIDs = tagIDs
	}

	return postings, nil
}

func postingTagIDs(ctx context.Context, queryer queryer, postingLineID int64) ([]int64, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT tag_id
		FROM posting_tags
		WHERE posting_line_id = ?
		ORDER BY tag_id
	`, postingLineID)
	if err != nil {
		return nil, fmt.Errorf("read posting tags: %w", err)
	}
	defer rows.Close()

	var tagIDs []int64
	for rows.Next() {
		var tagID int64
		if err := rows.Scan(&tagID); err != nil {
			return nil, fmt.Errorf("scan posting tag: %w", err)
		}
		tagIDs = append(tagIDs, tagID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate posting tags: %w", err)
	}

	return tagIDs, nil
}

func transactionByID(ctx context.Context, tx *sql.Tx, bookID int64, transactionID int64) (TransactionRecord, error) {
	var record TransactionRecord
	if err := scanTransactionRecord(tx.QueryRowContext(ctx, transactionSelect(`
		WHERE t.book_id = ? AND t.id = ?
	`), bookID, transactionID), &record); err != nil {
		return TransactionRecord{}, err
	}

	return record, nil
}

func transactionVersionByID(ctx context.Context, tx *sql.Tx, transactionVersionID int64) (TransactionRecord, error) {
	var record TransactionRecord
	if err := scanTransactionRecord(tx.QueryRowContext(ctx, transactionVersionSelect("transaction_versions", `
		WHERE tv.id = ?
	`), transactionVersionID), &record); err != nil {
		return TransactionRecord{}, err
	}

	return record, nil
}

func transactionSelect(extraConditions string) string {
	return transactionVersionSelect("current_transaction_versions", extraConditions)
}

func transactionVersionSelect(source string, extraConditions string) string {
	return `
		SELECT
			t.id,
			t.book_id,
			t.correction_of_transaction_id,
			t.created_at,
			t.created_by_user_id,
			t.deleted_at,
			tv.id,
			tv.version_seq,
			tv.supersedes_version_id,
			tv.status,
			tv.transaction_kind,
			tv.transaction_date,
			tv.transaction_day_sequence,
			tv.payee_id,
			tv.payee_name,
			tv.description,
			tv.external_ref_hint,
			tv.note_markdown,
			tv.metadata_json,
			tv.needs_review,
			tv.recorded_at,
			tv.changed_by_user_id,
			tv.change_reason
		FROM transactions t
		JOIN ` + source + ` tv ON tv.transaction_id = t.id
	` + extraConditions
}

func accountRegisterSelect(extraConditions string) string {
	return `
		SELECT
			t.id,
			t.book_id,
			t.correction_of_transaction_id,
			t.created_at,
			t.created_by_user_id,
			t.deleted_at,
			tv.id,
			tv.version_seq,
			tv.supersedes_version_id,
			tv.status,
			tv.transaction_kind,
			tv.transaction_date,
			tv.transaction_day_sequence,
			tv.payee_id,
			tv.payee_name,
			tv.description,
			tv.external_ref_hint,
			tv.note_markdown,
			tv.metadata_json,
			tv.needs_review,
			tv.recorded_at,
			tv.changed_by_user_id,
			tv.change_reason,
			je.id,
			je.book_id,
			je.transaction_version_id,
			je.entry_seq,
			je.entry_date,
			je.entry_kind,
			je.memo,
			je.metadata_json,
			pv.id,
			pv.book_id,
			pv.transaction_version_id,
			pv.journal_entry_id,
			pv.posting_line_id,
			pl.line_key,
			pv.line_seq,
			pv.account_id,
			pv.account_day_sequence,
			pv.quantity_value,
			pv.quantity_scale,
			pv.commodity_id,
			pv.memo,
			pv.reconciliation_status,
			pv.cleared_on,
			pv.metadata_json
		FROM transactions t
		JOIN current_transaction_versions tv ON tv.transaction_id = t.id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		JOIN posting_lines pl ON pl.id = pv.posting_line_id
	` + extraConditions
}

func scanTransactionRecords(rows *sql.Rows) ([]TransactionRecord, error) {
	var records []TransactionRecord
	for rows.Next() {
		var record TransactionRecord
		if err := scanTransactionRecord(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}

	return records, nil
}

func scanAccountRegisterEntries(rows *sql.Rows) ([]AccountRegisterEntryRecord, error) {
	var entries []AccountRegisterEntryRecord
	for rows.Next() {
		var entry AccountRegisterEntryRecord
		if err := scanAccountRegisterEntry(rows, &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account register: %w", err)
	}

	return entries, nil
}

func scanTransactionRecord(scanner interface{ Scan(dest ...any) error }, record *TransactionRecord) error {
	if err := scanner.Scan(
		&record.ID,
		&record.BookID,
		&record.CorrectionOfTransactionID,
		&record.CreatedAt,
		&record.CreatedByUserID,
		&record.DeletedAt,
		&record.VersionID,
		&record.VersionSeq,
		&record.SupersedesVersionID,
		&record.Status,
		&record.TransactionKind,
		&record.TransactionDate,
		&record.TransactionDaySequence,
		&record.PayeeID,
		&record.PayeeName,
		&record.Description,
		&record.ExternalRefHint,
		&record.NoteMarkdown,
		&record.MetadataJSON,
		&record.NeedsReview,
		&record.RecordedAt,
		&record.ChangedByUserID,
		&record.ChangeReason,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("scan transaction: %w", err)
	}

	return nil
}

func scanAccountRegisterEntry(scanner interface{ Scan(dest ...any) error }, entry *AccountRegisterEntryRecord) error {
	if err := scanner.Scan(
		&entry.Transaction.ID,
		&entry.Transaction.BookID,
		&entry.Transaction.CorrectionOfTransactionID,
		&entry.Transaction.CreatedAt,
		&entry.Transaction.CreatedByUserID,
		&entry.Transaction.DeletedAt,
		&entry.Transaction.VersionID,
		&entry.Transaction.VersionSeq,
		&entry.Transaction.SupersedesVersionID,
		&entry.Transaction.Status,
		&entry.Transaction.TransactionKind,
		&entry.Transaction.TransactionDate,
		&entry.Transaction.TransactionDaySequence,
		&entry.Transaction.PayeeID,
		&entry.Transaction.PayeeName,
		&entry.Transaction.Description,
		&entry.Transaction.ExternalRefHint,
		&entry.Transaction.NoteMarkdown,
		&entry.Transaction.MetadataJSON,
		&entry.Transaction.NeedsReview,
		&entry.Transaction.RecordedAt,
		&entry.Transaction.ChangedByUserID,
		&entry.Transaction.ChangeReason,
		&entry.JournalEntry.ID,
		&entry.JournalEntry.BookID,
		&entry.JournalEntry.TransactionVersionID,
		&entry.JournalEntry.EntrySeq,
		&entry.JournalEntry.EntryDate,
		&entry.JournalEntry.EntryKind,
		&entry.JournalEntry.Memo,
		&entry.JournalEntry.MetadataJSON,
		&entry.Posting.ID,
		&entry.Posting.BookID,
		&entry.Posting.TransactionVersionID,
		&entry.Posting.JournalEntryID,
		&entry.Posting.PostingLineID,
		&entry.Posting.LineKey,
		&entry.Posting.LineSeq,
		&entry.Posting.AccountID,
		&entry.Posting.AccountDaySequence,
		&entry.Posting.QuantityValue,
		&entry.Posting.QuantityScale,
		&entry.Posting.CommodityID,
		&entry.Posting.Memo,
		&entry.Posting.ReconciliationStatus,
		&entry.Posting.ClearedOn,
		&entry.Posting.MetadataJSON,
	); err != nil {
		return fmt.Errorf("scan account register entry: %w", err)
	}

	return nil
}

func mapTransactionConstraintError(err error) error {
	message := err.Error()
	switch {
	case strings.Contains(message, "reconciled postings require a corrective transaction"):
		return ErrTransactionReconciled
	case strings.Contains(message, "active tag"):
		return ErrArchivedTag
	default:
		return err
	}
}

func ftsPhrase(value string) string {
	cleaned := strings.TrimSpace(value)
	cleaned = strings.ReplaceAll(cleaned, `"`, `""`)
	return `"` + cleaned + `"`
}

func nullStringText(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

// ListDeletedTransactions returns soft-deleted transactions ordered by
// deleted_at DESC, id DESC with a deletion-time cursor for pagination.
// Each record includes the delete_reason snapshot column.
func (r *TransactionRepository) ListDeletedTransactions(ctx context.Context, params ListDeletedTransactionsParams) ([]DeletedTransactionRecord, error) {
	where := []string{"t.book_id = ?", "t.deleted_at IS NOT NULL"}
	args := []any{params.BookID}

	if params.CursorDeletedAt != "" && params.CursorID > 0 {
		where = append(where, `(
			t.deleted_at < ?
			OR (t.deleted_at = ? AND t.id < ?)
		)`)
		args = append(args, params.CursorDeletedAt, params.CursorDeletedAt, params.CursorID)
	}

	args = append(args, params.Limit)
	query := `
		SELECT
			t.id,
			t.book_id,
			t.correction_of_transaction_id,
			t.created_at,
			t.created_by_user_id,
			t.deleted_at,
			tv.id,
			tv.version_seq,
			tv.supersedes_version_id,
			tv.status,
			tv.transaction_kind,
			tv.transaction_date,
			tv.transaction_day_sequence,
			tv.payee_id,
			tv.payee_name,
			tv.description,
			tv.external_ref_hint,
			tv.note_markdown,
			tv.metadata_json,
			tv.needs_review,
			tv.recorded_at,
			tv.changed_by_user_id,
			tv.change_reason,
			COALESCE(t.delete_reason, '')
		FROM transactions t
		JOIN current_transaction_versions tv ON tv.transaction_id = t.id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY t.deleted_at DESC, t.id DESC
		LIMIT ?
	`

	rows, err := r.database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list deleted transactions: %w", err)
	}
	defer rows.Close()

	var records []DeletedTransactionRecord
	for rows.Next() {
		var rec DeletedTransactionRecord
		if err := rows.Scan(
			&rec.ID,
			&rec.BookID,
			&rec.CorrectionOfTransactionID,
			&rec.CreatedAt,
			&rec.CreatedByUserID,
			&rec.DeletedAt,
			&rec.VersionID,
			&rec.VersionSeq,
			&rec.SupersedesVersionID,
			&rec.Status,
			&rec.TransactionKind,
			&rec.TransactionDate,
			&rec.TransactionDaySequence,
			&rec.PayeeID,
			&rec.PayeeName,
			&rec.Description,
			&rec.ExternalRefHint,
			&rec.NoteMarkdown,
			&rec.MetadataJSON,
			&rec.NeedsReview,
			&rec.RecordedAt,
			&rec.ChangedByUserID,
			&rec.ChangeReason,
			&rec.DeleteReason,
		); err != nil {
			return nil, fmt.Errorf("scan deleted transaction: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deleted transactions: %w", err)
	}

	baseRecords := make([]TransactionRecord, len(records))
	for i := range records {
		baseRecords[i] = records[i].TransactionRecord
	}
	if err := r.loadTransactionChildren(ctx, baseRecords); err != nil {
		return nil, err
	}
	for i := range records {
		records[i].TransactionRecord = baseRecords[i]
	}

	return records, nil
}

// EncodeTransactionCursor encodes the 3-tuple (date, day_sequence, id) used
// for stable same-day ordering of the global transaction list.
func EncodeTransactionCursor(transactionDate string, daySequence int64, transactionID int64) string {
	if transactionDate == "" || transactionID <= 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s|%d|%d", transactionDate, daySequence, transactionID)))
}

// DecodeTransactionCursor decodes a 3-tuple cursor produced by EncodeTransactionCursor.
func DecodeTransactionCursor(cursor string) (date string, daySequence int64, id int64, err error) {
	cleaned := strings.TrimSpace(cursor)
	if cleaned == "" {
		return "", 0, 0, nil
	}
	decoded, decErr := base64.RawURLEncoding.DecodeString(cleaned)
	if decErr != nil {
		return "", 0, 0, fmt.Errorf("cursor is invalid")
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 {
		return "", 0, 0, fmt.Errorf("cursor is invalid")
	}
	var seq, txID int64
	if _, scanErr := fmt.Sscanf(parts[1], "%d", &seq); scanErr != nil || seq < 0 {
		return "", 0, 0, fmt.Errorf("cursor is invalid")
	}
	if _, scanErr := fmt.Sscanf(parts[2], "%d", &txID); scanErr != nil || txID <= 0 {
		return "", 0, 0, fmt.Errorf("cursor is invalid")
	}
	return parts[0], seq, txID, nil
}

// EncodeRegisterCursor encodes the 3-tuple (entry_date, account_day_sequence, posting_version_id)
// for the account register pagination.
func EncodeRegisterCursor(entryDate string, daySequence int64, postingVersionID int64) string {
	if entryDate == "" || postingVersionID <= 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s|%d|%d", entryDate, daySequence, postingVersionID)))
}

// DecodeRegisterCursor decodes a 3-tuple cursor produced by EncodeRegisterCursor.
func DecodeRegisterCursor(cursor string) (date string, daySequence int64, id int64, err error) {
	// Register and transaction cursors share the same format; delegate.
	return DecodeTransactionCursor(cursor)
}

// EncodeDeletionCursor encodes the 2-tuple (deleted_at, id) used for trash
// pagination. deleted_at is an RFC3339 timestamp string.
func EncodeDeletionCursor(deletedAt string, id int64) string {
	if deletedAt == "" || id <= 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s|%d", deletedAt, id)))
}

// DecodeDeletionCursor decodes a cursor produced by EncodeDeletionCursor.
func DecodeDeletionCursor(cursor string) (deletedAt string, id int64, err error) {
	cleaned := strings.TrimSpace(cursor)
	if cleaned == "" {
		return "", 0, nil
	}
	decoded, decErr := base64.RawURLEncoding.DecodeString(cleaned)
	if decErr != nil {
		return "", 0, fmt.Errorf("cursor is invalid")
	}
	idx := strings.LastIndex(string(decoded), "|")
	if idx < 0 {
		return "", 0, fmt.Errorf("cursor is invalid")
	}
	ts := string(decoded[:idx])
	rest := string(decoded[idx+1:])
	var txID int64
	if _, scanErr := fmt.Sscanf(rest, "%d", &txID); scanErr != nil || txID <= 0 {
		return "", 0, fmt.Errorf("cursor is invalid")
	}
	return ts, txID, nil
}

// matchingEntryClause requires one journal entry of the transaction to satisfy
// every entry-scoped filter at once: the date range, the category account, and
// the category direction. It exists so a drill-down from a spending row lists
// exactly the transactions that row was built from.
func matchingEntryClause(params ListTransactionsParams) (string, []any) {
	conditions := []string{"range_je.transaction_version_id = tv.id"}
	args := []any{}

	if params.AfterDate != "" {
		conditions = append(conditions, "range_je.entry_date >= ?")
		args = append(args, params.AfterDate)
	}
	if params.BeforeDate != "" {
		conditions = append(conditions, "range_je.entry_date <= ?")
		args = append(args, params.BeforeDate)
	}
	if params.CategoryID > 0 {
		conditions = append(conditions, "range_pv.account_id = ?")
		args = append(args, params.CategoryID)
	}
	if params.CategoryType != "" {
		// Same shape as the spending report: the class is read from the account
		// version in effect on the entry's own date, because parent links and
		// classes are versioned, and account_kind = account_class keeps this to
		// real category accounts rather than anything merely income-classed.
		conditions = append(conditions, `EXISTS (
				SELECT 1
				FROM accounts type_account
				JOIN account_versions type_av ON type_av.account_id = type_account.id
				WHERE type_account.id = range_pv.account_id
					AND type_account.book_id = tv.book_id
					AND type_account.system_role IS NULL
					AND type_av.id = (
						SELECT asof_type_av.id
						FROM account_versions asof_type_av
						WHERE asof_type_av.account_id = type_account.id
							AND asof_type_av.effective_from <= range_je.entry_date
						ORDER BY asof_type_av.effective_from DESC, asof_type_av.version_seq DESC
						LIMIT 1
					)
					AND type_av.account_class = ?
					AND type_av.account_kind = type_av.account_class
			)`)
		args = append(args, params.CategoryType)
	}

	return `EXISTS (
			SELECT 1
			FROM journal_entries range_je
			JOIN posting_versions range_pv ON range_pv.journal_entry_id = range_je.id
			WHERE ` + strings.Join(conditions, "\n\t\t\t\t AND ") + `
		)`, args
}

// categoryTypeClause is the transaction-date-basis form: the direction still
// has to hold somewhere in the transaction, but no entry has to carry it
// together with the date.
func categoryTypeClause(categoryType string) (string, []any) {
	return `EXISTS (
			SELECT 1
			FROM journal_entries type_je
			JOIN posting_versions type_pv ON type_pv.journal_entry_id = type_je.id
			JOIN accounts type_account ON type_account.id = type_pv.account_id
			JOIN account_versions type_av ON type_av.account_id = type_account.id
			WHERE type_je.transaction_version_id = tv.id
				AND type_account.book_id = tv.book_id
				AND type_account.system_role IS NULL
				AND type_av.id = (
					SELECT asof_type_av.id
					FROM account_versions asof_type_av
					WHERE asof_type_av.account_id = type_account.id
						AND asof_type_av.effective_from <= type_je.entry_date
					ORDER BY asof_type_av.effective_from DESC, asof_type_av.version_seq DESC
					LIMIT 1
				)
				AND type_av.account_class = ?
				AND type_av.account_kind = type_av.account_class
		)`, []any{categoryType}
}
