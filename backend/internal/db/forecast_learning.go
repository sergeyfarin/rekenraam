package db

import (
	"context"
	"database/sql"
	"fmt"

	"rekenraam/backend/internal/exact"
)

const ForecastLearningPostingLimit = 100000

// ForecastLearningPostingRecord is one sibling posting from a complete current
// journal entry. Learning must classify the whole entry; selecting only an
// expense or funding leg would make transfers and split funding look eligible.
type ForecastLearningPostingRecord struct {
	TransactionID         int64
	TransactionVersionID  int64
	TransactionKind       string
	JournalEntryID        int64
	EntryKind             string
	EntryDate             string
	PostingID             int64
	LineSeq               int
	AccountID             int64
	CommodityID           int64
	QuantityValue         exact.Coefficient
	QuantityScale         int
	RecurringOccurrenceID sql.NullInt64
}

type ForecastLearningSnapshotRequest struct {
	BookID       int64
	AccountIDs   []int64
	HistoryStart string
	HistoryEnd   string
	PostingLimit int
	Limits       ForecastSnapshotLimits
}

type ForecastLearningSnapshot struct {
	AccountVersions   []ForecastAccountVersionRecord
	CommodityVersions []ForecastCommodityVersionRecord
	Postings          []ForecastLearningPostingRecord
	Templates         []ForecastTemplateRecord
	TemplatePostings  []ForecastTemplatePostingRecord
	Occurrences       []ForecastOccurrenceRecord
	DraftPostings     []ForecastDraftPostingRecord
}

// LoadLearningSnapshot reads history and recurring-overlap inputs in one
// deferred read transaction. It is intentionally separate from the public
// forecast path until M3, where both core and learning will share one reader.
func (r *ForecastRepository) LoadLearningSnapshot(ctx context.Context, request ForecastLearningSnapshotRequest) (ForecastLearningSnapshot, error) {
	limits := request.Limits
	if limits == (ForecastSnapshotLimits{}) {
		limits = DefaultForecastSnapshotLimits()
	}
	postingLimit := request.PostingLimit
	if postingLimit == 0 {
		postingLimit = ForecastLearningPostingLimit
	}
	var result ForecastLearningSnapshot
	err := r.WithSnapshot(ctx, func(reader *ForecastSnapshotReader) error {
		var err error
		result.AccountVersions, err = reader.accountVersions(ctx, request.BookID, limits.VersionRows)
		if err != nil {
			return err
		}
		result.CommodityVersions, err = reader.commodityVersions(ctx, request.BookID, limits.VersionRows-len(result.AccountVersions))
		if err != nil {
			return err
		}
		result.Postings, err = reader.learningPostings(ctx, request.BookID, request.HistoryStart, request.HistoryEnd, postingLimit)
		if err != nil {
			return err
		}
		result.Templates, result.TemplatePostings, result.Occurrences, result.DraftPostings, err = reader.recurringInputs(ctx, request.BookID, request.AccountIDs, limits)
		return err
	})
	if err != nil {
		return ForecastLearningSnapshot{}, err
	}
	return result, nil
}

func (r *ForecastSnapshotReader) learningPostings(ctx context.Context, bookID int64, startDate, endDate string, limit int) ([]ForecastLearningPostingRecord, error) {
	if limit < 0 {
		return nil, ErrForecastInputTooLarge
	}
	rows, err := r.transaction.QueryContext(ctx, `
		SELECT tv.transaction_id, tv.id, tv.transaction_kind, je.id, je.entry_kind,
			je.entry_date, pv.id, pv.line_seq, pv.account_id, pv.commodity_id,
			pv.quantity_value, pv.quantity_scale, ro.id
		FROM current_transaction_versions tv
		JOIN transactions t ON t.id = tv.transaction_id AND t.book_id = tv.book_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id AND je.book_id = tv.book_id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id AND pv.transaction_version_id = tv.id AND pv.book_id = tv.book_id
		LEFT JOIN recurring_occurrences ro ON ro.book_id = tv.book_id AND ro.transaction_id = tv.transaction_id
		WHERE tv.book_id = ? AND tv.status = 'posted' AND t.deleted_at IS NULL
			AND je.entry_date >= ? AND je.entry_date <= ?
		ORDER BY je.entry_date, tv.transaction_id, tv.id, je.entry_seq, pv.line_seq, pv.id
		LIMIT ?`, bookID, startDate, endDate, plusOne(limit))
	if err != nil {
		return nil, fmt.Errorf("read forecast learning postings: %w", err)
	}
	defer rows.Close()
	records := make([]ForecastLearningPostingRecord, 0)
	for rows.Next() {
		var record ForecastLearningPostingRecord
		if err := rows.Scan(&record.TransactionID, &record.TransactionVersionID, &record.TransactionKind,
			&record.JournalEntryID, &record.EntryKind, &record.EntryDate, &record.PostingID,
			&record.LineSeq, &record.AccountID, &record.CommodityID, &record.QuantityValue,
			&record.QuantityScale, &record.RecurringOccurrenceID); err != nil {
			return nil, fmt.Errorf("scan forecast learning posting: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forecast learning postings: %w", err)
	}
	if len(records) > limit {
		return nil, ErrForecastInputTooLarge
	}
	return records, nil
}
