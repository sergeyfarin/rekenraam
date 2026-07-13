package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// MoveTransaction swaps the transaction_day_sequence of the target transaction
// with the adjacent current transaction on the same date, atomically.
// Returns the updated record of the moved transaction. Returns ErrNotFound when
// there is no adjacent transaction to swap with.
func (r *TransactionRepository) MoveTransaction(ctx context.Context, params MoveTransactionParams) (TransactionRecord, error) {
	return withTransactionRecordTx(r, ctx, "move transaction", func(tx *sql.Tx) (TransactionRecord, error) {

		if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
			return TransactionRecord{}, err
		}
		current, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
		if err != nil {
			return TransactionRecord{}, err
		}

		// Find the adjacent transaction in the requested direction.
		var adjTransactionID int64
		var adjSeq int64

		var adjQuery string
		if params.Direction == "earlier" {
			adjQuery = `
			SELECT ctv.transaction_id, ctv.transaction_day_sequence
			FROM current_transaction_versions ctv
			JOIN transactions t ON t.id = ctv.transaction_id
			WHERE ctv.book_id = ?
				AND ctv.transaction_date = ?
				AND ctv.transaction_day_sequence < ?
				AND t.deleted_at IS NULL
			ORDER BY ctv.transaction_day_sequence DESC
			LIMIT 1
		`
		} else {
			adjQuery = `
			SELECT ctv.transaction_id, ctv.transaction_day_sequence
			FROM current_transaction_versions ctv
			JOIN transactions t ON t.id = ctv.transaction_id
			WHERE ctv.book_id = ?
				AND ctv.transaction_date = ?
				AND ctv.transaction_day_sequence > ?
				AND t.deleted_at IS NULL
			ORDER BY ctv.transaction_day_sequence ASC
			LIMIT 1
		`
		}
		err = tx.QueryRowContext(ctx, adjQuery, params.BookID, current.TransactionDate, current.TransactionDaySequence).
			Scan(&adjTransactionID, &adjSeq)
		if errors.Is(err, sql.ErrNoRows) {
			return TransactionRecord{}, ErrNotFound
		}
		if err != nil {
			return TransactionRecord{}, fmt.Errorf("find adjacent transaction: %w", err)
		}

		auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
			BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID,
			OccurredAt: params.RecordedAt, RequestID: params.RequestID, OriginType: params.OriginType,
			Operation: "transaction.move", Reason: "moved " + params.Direction,
		})
		if err != nil {
			return TransactionRecord{}, err
		}

		// Load full current record (with children) for the current transaction.
		currentRecords := []TransactionRecord{current}
		if err := loadTransactionChildrenTx(ctx, tx, currentRecords); err != nil {
			return TransactionRecord{}, err
		}
		current = currentRecords[0]

		// Load adjacent transaction's spec so we can write new versions.
		adj, err := transactionByID(ctx, tx, params.BookID, adjTransactionID)
		if err != nil {
			return TransactionRecord{}, err
		}
		adjRecords := []TransactionRecord{adj}
		if err := loadTransactionChildrenTx(ctx, tx, adjRecords); err != nil {
			return TransactionRecord{}, err
		}
		adj = adjRecords[0]

		// Write new version of the current transaction with the adjacent transaction's sequence.
		movedRecord, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
			BookID: params.BookID, TransactionID: params.TransactionID, VersionSeq: current.VersionSeq + 1,
			SupersedesVersionID: sql.NullInt64{Int64: current.VersionID, Valid: true},
			Spec:                transactionSpecFromRecord(current), ReplaceTags: false,
			RecordedAt: params.RecordedAt, ChangedByUserID: params.ActorUserID,
			ChangeReason: "moved " + params.Direction, ChangeAuditEventID: auditEventID,
			RequestID: params.RequestID, TransactionDaySequence: adjSeq,
		})
		if err != nil {
			return TransactionRecord{}, fmt.Errorf("insert moved transaction version: %w", err)
		}

		// Write new version of the adjacent transaction with the current transaction's sequence.
		_, err = r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
			BookID: params.BookID, TransactionID: adjTransactionID, VersionSeq: adj.VersionSeq + 1,
			SupersedesVersionID: sql.NullInt64{Int64: adj.VersionID, Valid: true},
			Spec:                transactionSpecFromRecord(adj), ReplaceTags: false,
			RecordedAt: params.RecordedAt, ChangedByUserID: params.ActorUserID,
			ChangeReason: "swapped for moved transaction", ChangeAuditEventID: auditEventID,
			RequestID: params.RequestID, TransactionDaySequence: current.TransactionDaySequence,
		})
		if err != nil {
			return TransactionRecord{}, fmt.Errorf("insert adjacent transaction version after move: %w", err)
		}

		return movedRecord, nil
	})
}

// MovePosting swaps the account_day_sequence of the target posting with the
// adjacent current posting in the same (account_id, entry_date) scope.
// Returns the parent TransactionRecord of the moved posting. Returns ErrNotFound
// when there is no adjacent posting to swap with.
func (r *TransactionRepository) MovePosting(ctx context.Context, params MovePostingParams) (TransactionRecord, error) {
	return withTransactionRecordTx(r, ctx, "move posting", func(tx *sql.Tx) (TransactionRecord, error) {

		if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
			return TransactionRecord{}, err
		}

		// Read the current posting version for the given posting line.
		var currentTransactionID int64
		var currentEntryDate string
		var currentSeq int64
		var currentCommodityID int64
		if err := tx.QueryRowContext(ctx, `
		SELECT t.id, je.entry_date, pv.account_day_sequence, pv.commodity_id
		FROM posting_versions pv
		JOIN journal_entries je ON je.id = pv.journal_entry_id
		JOIN current_transaction_versions ctv ON ctv.id = pv.transaction_version_id
		JOIN transactions t ON t.id = ctv.transaction_id
		WHERE pv.book_id = ?
			AND pv.posting_line_id = ?
			AND pv.account_id = ?
			AND t.deleted_at IS NULL
		LIMIT 1
	`, params.BookID, params.PostingLineID, params.AccountID).Scan(
			&currentTransactionID, &currentEntryDate, &currentSeq, &currentCommodityID,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return TransactionRecord{}, ErrNotFound
			}
			return TransactionRecord{}, fmt.Errorf("read current posting: %w", err)
		}

		// Find the adjacent posting in the same (account_id, entry_date) scope.
		var adjPostingLineID int64
		var adjTransactionID int64
		var adjSeq int64

		var adjQuery string
		if params.Direction == "earlier" {
			adjQuery = `
			SELECT pv.posting_line_id, t.id, pv.account_day_sequence
			FROM posting_versions pv
			JOIN journal_entries je ON je.id = pv.journal_entry_id
			JOIN current_transaction_versions ctv ON ctv.id = pv.transaction_version_id
			JOIN transactions t ON t.id = ctv.transaction_id
			WHERE pv.book_id = ?
				AND pv.account_id = ?
				AND je.entry_date = ?
				AND pv.account_day_sequence < ?
				AND t.deleted_at IS NULL
			ORDER BY pv.account_day_sequence DESC
			LIMIT 1
		`
		} else {
			adjQuery = `
			SELECT pv.posting_line_id, t.id, pv.account_day_sequence
			FROM posting_versions pv
			JOIN journal_entries je ON je.id = pv.journal_entry_id
			JOIN current_transaction_versions ctv ON ctv.id = pv.transaction_version_id
			JOIN transactions t ON t.id = ctv.transaction_id
			WHERE pv.book_id = ?
				AND pv.account_id = ?
				AND je.entry_date = ?
				AND pv.account_day_sequence > ?
				AND t.deleted_at IS NULL
			ORDER BY pv.account_day_sequence ASC
			LIMIT 1
		`
		}
		err := tx.QueryRowContext(ctx, adjQuery, params.BookID, params.AccountID, currentEntryDate, currentSeq).
			Scan(&adjPostingLineID, &adjTransactionID, &adjSeq)
		if errors.Is(err, sql.ErrNoRows) {
			return TransactionRecord{}, ErrNotFound
		}
		if err != nil {
			return TransactionRecord{}, fmt.Errorf("find adjacent posting: %w", err)
		}

		// Check whether the swap would move a posting across an active checkpoint
		// boundary. The current posting would take adjSeq; the adjacent posting
		// would take currentSeq. If the new position for either posting crosses
		// the boundary, require override. Checkpoints are account- AND
		// commodity-scoped, and latestActiveReconciliationCheckpoint applies the
		// same (statement_date DESC, id DESC) lock-floor ordering used by every
		// other reconciliation guard in this package.
		crossesCheckpoint := false
		checkpoint, checkpointErr := latestActiveReconciliationCheckpoint(ctx, tx, params.BookID, params.AccountID, currentCommodityID)
		if checkpointErr != nil && !errors.Is(checkpointErr, ErrReconciliationCheckpoint) {
			return TransactionRecord{}, fmt.Errorf("read checkpoint for move guard: %w", checkpointErr)
		}
		if checkpointErr == nil {
			// A posting is inside the period when: entry_date < stmtDate OR (entry_date == stmtDate AND seq <= stmtAcctSeq)
			insideBefore := func(seq int64) bool {
				return currentEntryDate < checkpoint.StatementDate ||
					(currentEntryDate == checkpoint.StatementDate && seq <= checkpoint.StatementAccountSequence)
			}
			// A swap crosses the boundary when one sequence is inside and the other is not.
			crossesCheckpoint = insideBefore(currentSeq) != insideBefore(adjSeq)
		}
		if crossesCheckpoint && !params.ReconciliationOverride {
			return TransactionRecord{}, ErrReconciliationOverrideRequired
		}

		auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
			BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID,
			OccurredAt: params.RecordedAt, RequestID: params.RequestID, OriginType: params.OriginType,
			Operation: "posting.move", Reason: "moved " + params.Direction,
		})
		if err != nil {
			return TransactionRecord{}, err
		}

		// Write new transaction version for the moved posting's transaction.
		movedTxRecord, err := transactionByID(ctx, tx, params.BookID, currentTransactionID)
		if err != nil {
			return TransactionRecord{}, err
		}
		movedTxRecords := []TransactionRecord{movedTxRecord}
		if err := loadTransactionChildrenTx(ctx, tx, movedTxRecords); err != nil {
			return TransactionRecord{}, err
		}
		movedTxRecord = movedTxRecords[0]

		// Write a new version of the moved transaction with an explicit seq
		// override for the target posting line. When the adjacent posting
		// belongs to the SAME transaction, its override must be included in this
		// same version write too — otherwise it would inherit its own prior
		// position (which is what the moved posting is taking), leaving both
		// postings at adjSeq and losing currentSeq entirely.
		postingSeqOverrides := map[int64]int64{params.PostingLineID: adjSeq}
		if adjTransactionID == currentTransactionID {
			postingSeqOverrides[adjPostingLineID] = currentSeq
		}
		movedResult, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
			BookID: params.BookID, TransactionID: currentTransactionID, VersionSeq: movedTxRecord.VersionSeq + 1,
			SupersedesVersionID: sql.NullInt64{Int64: movedTxRecord.VersionID, Valid: true},
			Spec:                transactionSpecFromRecord(movedTxRecord), ReplaceTags: false,
			RecordedAt: params.RecordedAt, ChangedByUserID: params.ActorUserID,
			ChangeReason: "posting moved " + params.Direction, ChangeAuditEventID: auditEventID,
			RequestID: params.RequestID, TransactionDaySequence: movedTxRecord.TransactionDaySequence,
			PostingSeqOverrides: postingSeqOverrides,
		})
		if err != nil {
			return TransactionRecord{}, fmt.Errorf("insert moved posting version: %w", err)
		}

		// If the adjacent posting is in a different transaction, write a new version
		// of that transaction with the swapped sequence.
		if adjTransactionID != currentTransactionID {
			adjTxRecord, err := transactionByID(ctx, tx, params.BookID, adjTransactionID)
			if err != nil {
				return TransactionRecord{}, err
			}
			adjTxRecords := []TransactionRecord{adjTxRecord}
			if err := loadTransactionChildrenTx(ctx, tx, adjTxRecords); err != nil {
				return TransactionRecord{}, err
			}
			adjTxRecord = adjTxRecords[0]
			_, err = r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
				BookID: params.BookID, TransactionID: adjTransactionID, VersionSeq: adjTxRecord.VersionSeq + 1,
				SupersedesVersionID: sql.NullInt64{Int64: adjTxRecord.VersionID, Valid: true},
				Spec:                transactionSpecFromRecord(adjTxRecord), ReplaceTags: false,
				RecordedAt: params.RecordedAt, ChangedByUserID: params.ActorUserID,
				ChangeReason: "swapped for moved posting", ChangeAuditEventID: auditEventID,
				RequestID: params.RequestID, TransactionDaySequence: adjTxRecord.TransactionDaySequence,
				PostingSeqOverrides: map[int64]int64{adjPostingLineID: currentSeq},
			})
			if err != nil {
				return TransactionRecord{}, fmt.Errorf("insert adjacent posting version after move: %w", err)
			}
		}

		// The guard permitted this move only because an override was granted for
		// a checkpoint-crossing swap — invalidate the checkpoint rather than
		// leaving it active over data it no longer accurately reflects.
		if crossesCheckpoint {
			invalidated, err := invalidateReconciliationCheckpoints(ctx, tx, checkpointInvalidationParams{
				BookID:       params.BookID,
				Refs:         []CheckpointInvalidationRef{{AccountID: params.AccountID, CommodityID: currentCommodityID, EntryDate: currentEntryDate}},
				ActorUserID:  params.ActorUserID,
				AuditEventID: auditEventID,
				OccurredAt:   params.RecordedAt,
				Reason:       "posting moved " + params.Direction + " across a reconciled checkpoint",
			})
			if err != nil {
				return TransactionRecord{}, err
			}
			movedResult.InvalidatedCheckpointIDs = invalidated
		}

		return movedResult, nil
	})
}

// allocateOrInheritAccountDaySeq returns the account_day_sequence for the new
// posting version. If the posting line has a version attached to prevVersionID
// with the same account_id and entry_date, we inherit its sequence (the
// posting's register position is unchanged). Otherwise we allocate MAX+1 within
// (book_id, account_id, entry_date).
//
// prevVersionID must be the transaction_version_id that was current BEFORE the
// new version row was inserted. We cannot use current_transaction_versions here
// because the new version row is already inserted (but has no postings yet),
// so the view would return no rows for this posting.
func allocateOrInheritAccountDaySeq(ctx context.Context, tx *sql.Tx, bookID int64, prevVersionID int64, postingLineID int64, accountID int64, entryDate string) (int64, error) {
	if prevVersionID > 0 {
		var inherited int64
		err := tx.QueryRowContext(ctx, `
			SELECT pv.account_day_sequence
			FROM posting_versions pv
			JOIN journal_entries je ON je.id = pv.journal_entry_id
			WHERE pv.book_id = ?
				AND pv.transaction_version_id = ?
				AND pv.posting_line_id = ?
				AND pv.account_id = ?
				AND je.entry_date = ?
			LIMIT 1
		`, bookID, prevVersionID, postingLineID, accountID, entryDate).Scan(&inherited)
		if err == nil {
			return inherited, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("check existing posting day sequence: %w", err)
		}
	}

	// No current version with the same account/date — allocate next position.
	var next int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(pv.account_day_sequence), 0) + 1
		FROM posting_versions pv
		JOIN journal_entries je ON je.id = pv.journal_entry_id
		WHERE pv.book_id = ?
			AND pv.account_id = ?
			AND je.entry_date = ?
	`, bookID, accountID, entryDate).Scan(&next); err != nil {
		return 0, fmt.Errorf("compute account day sequence: %w", err)
	}
	return next, nil
}
