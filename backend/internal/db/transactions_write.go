package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (r *TransactionRepository) CreateTransaction(ctx context.Context, params CreateTransactionParams) (TransactionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("begin create transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	record, err := createTransactionTx(ctx, tx, params)
	if err != nil {
		return TransactionRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, fmt.Errorf("commit create transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *TransactionRepository) CreateTransactionInTx(ctx context.Context, tx *sql.Tx, params CreateTransactionParams) (TransactionRecord, error) {
	return createTransactionTx(ctx, tx, params)
}

func createTransactionTx(ctx context.Context, tx *sql.Tx, params CreateTransactionParams) (TransactionRecord, error) {
	record, auditEventID, err := createTransactionWithAuditTx(ctx, tx, params)
	if err != nil {
		return TransactionRecord{}, err
	}
	if len(params.InvalidateCheckpointRefs) > 0 {
		invalidatedIDs, err := invalidateReconciliationCheckpoints(ctx, tx, checkpointInvalidationParams{
			BookID:       params.BookID,
			Refs:         params.InvalidateCheckpointRefs,
			ActorUserID:  params.ActorUserID,
			AuditEventID: auditEventID,
			OccurredAt:   params.CreatedAt,
			Reason:       params.InvalidateCheckpointReason,
		})
		if err != nil {
			return TransactionRecord{}, err
		}
		record.InvalidatedCheckpointIDs = invalidatedIDs
	}
	return record, nil
}

func createTransactionWithAuditTx(ctx context.Context, tx *sql.Tx, params CreateTransactionParams) (TransactionRecord, int64, error) {
	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return TransactionRecord{}, 0, err
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return TransactionRecord{}, 0, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO transactions (
			book_id,
			correction_of_transaction_id,
			created_at,
			created_by_user_id,
			created_request_id,
			created_audit_event_id
		)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), ?)
	`, params.BookID, nullableInt64Value(params.CorrectionOfTransactionID), params.CreatedAt, params.ActorUserID, params.RequestID, auditEventID)
	if err != nil {
		return TransactionRecord{}, 0, fmt.Errorf("insert transaction: %w", err)
	}
	transactionID, err := result.LastInsertId()
	if err != nil {
		return TransactionRecord{}, 0, fmt.Errorf("read transaction id: %w", err)
	}

	repository := &TransactionRepository{}
	record, err := repository.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
		BookID:             params.BookID,
		TransactionID:      transactionID,
		VersionSeq:         1,
		Spec:               params.Spec,
		ReplaceTags:        true,
		RecordedAt:         params.CreatedAt,
		ChangedByUserID:    params.ActorUserID,
		ChangeReason:       params.ChangeReason,
		ChangeAuditEventID: auditEventID,
		RequestID:          params.RequestID,
	})
	if err != nil {
		return TransactionRecord{}, 0, err
	}

	return record, auditEventID, nil
}

func (r *TransactionRepository) UpdateTransaction(ctx context.Context, params UpdateTransactionParams) (TransactionRecord, error) {
	return withTransactionRecordTx(r, ctx, "update transaction", func(tx *sql.Tx) (TransactionRecord, error) {

		if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
			return TransactionRecord{}, err
		}

		current, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
		if err != nil {
			return TransactionRecord{}, err
		}
		if current.Status == "voided" {
			return TransactionRecord{}, ErrTransactionVoided
		}
		if current.DeletedAt.Valid {
			return TransactionRecord{}, ErrTransactionDeleted
		}
		auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
			BookID:        params.BookID,
			ActorUserID:   params.ActorUserID,
			AuthSessionID: params.AuthSessionID,
			OccurredAt:    params.RecordedAt,
			RequestID:     params.RequestID,
			OriginType:    params.OriginType,
			Operation:     params.Operation,
			Reason:        params.ChangeReason,
		})
		if err != nil {
			return TransactionRecord{}, err
		}

		// Inherit the existing day sequence when the date is unchanged; otherwise
		// the transaction moves to the end of its new date (0 triggers MAX+1).
		inheritedTxDaySeq := int64(0)
		if params.Spec.TransactionDate == current.TransactionDate {
			inheritedTxDaySeq = current.TransactionDaySequence
		}

		record, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
			BookID:                 params.BookID,
			TransactionID:          params.TransactionID,
			VersionSeq:             current.VersionSeq + 1,
			SupersedesVersionID:    sql.NullInt64{Int64: current.VersionID, Valid: true},
			Spec:                   params.Spec,
			ReplaceTags:            true,
			RecordedAt:             params.RecordedAt,
			ChangedByUserID:        params.ActorUserID,
			ChangeReason:           params.ChangeReason,
			ChangeAuditEventID:     auditEventID,
			RequestID:              params.RequestID,
			TransactionDaySequence: inheritedTxDaySeq,
		})
		if err != nil {
			return TransactionRecord{}, mapTransactionConstraintError(err)
		}
		invalidatedCheckpointIDs, err := invalidateReconciliationCheckpoints(ctx, tx, checkpointInvalidationParams{
			BookID:       params.BookID,
			Refs:         params.InvalidateCheckpointRefs,
			ActorUserID:  params.ActorUserID,
			AuditEventID: auditEventID,
			OccurredAt:   params.RecordedAt,
			Reason:       params.InvalidateCheckpointReason,
		})
		if err != nil {
			return TransactionRecord{}, err
		}
		record.InvalidatedCheckpointIDs = invalidatedCheckpointIDs

		return record, nil
	})
}

func (r *TransactionRepository) VoidTransaction(ctx context.Context, params VoidTransactionParams) (TransactionRecord, error) {
	return withTransactionRecordTx(r, ctx, "void transaction", func(tx *sql.Tx) (TransactionRecord, error) {

		if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
			return TransactionRecord{}, err
		}
		current, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
		if err != nil {
			return TransactionRecord{}, err
		}
		if current.Status == "voided" {
			records := []TransactionRecord{current}
			if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
				return TransactionRecord{}, err
			}
			return records[0], nil
		}
		if current.DeletedAt.Valid {
			return TransactionRecord{}, ErrTransactionDeleted
		}
		currentRecords := []TransactionRecord{current}
		if err := loadTransactionChildrenTx(ctx, tx, currentRecords); err != nil {
			return TransactionRecord{}, err
		}
		current = currentRecords[0]

		auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
			BookID:        params.BookID,
			ActorUserID:   params.ActorUserID,
			AuthSessionID: params.AuthSessionID,
			OccurredAt:    params.RecordedAt,
			RequestID:     params.RequestID,
			OriginType:    params.OriginType,
			Operation:     params.Operation,
			Reason:        params.ChangeReason,
		})
		if err != nil {
			return TransactionRecord{}, err
		}

		spec := transactionSpecFromRecord(current)
		spec.Status = "voided"
		record, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
			BookID:                 params.BookID,
			TransactionID:          params.TransactionID,
			VersionSeq:             current.VersionSeq + 1,
			SupersedesVersionID:    sql.NullInt64{Int64: current.VersionID, Valid: true},
			Spec:                   spec,
			ReplaceTags:            false,
			RecordedAt:             params.RecordedAt,
			ChangedByUserID:        params.ActorUserID,
			ChangeReason:           params.ChangeReason,
			ChangeAuditEventID:     auditEventID,
			RequestID:              params.RequestID,
			TransactionDaySequence: current.TransactionDaySequence,
		})
		if err != nil {
			return TransactionRecord{}, mapTransactionConstraintError(err)
		}
		invalidatedCheckpointIDs, err := invalidateReconciliationCheckpoints(ctx, tx, checkpointInvalidationParams{
			BookID:       params.BookID,
			Refs:         params.InvalidateCheckpointRefs,
			ActorUserID:  params.ActorUserID,
			AuditEventID: auditEventID,
			OccurredAt:   params.RecordedAt,
			Reason:       params.InvalidateCheckpointReason,
		})
		if err != nil {
			return TransactionRecord{}, err
		}
		record.InvalidatedCheckpointIDs = invalidatedCheckpointIDs

		return record, nil
	})
}

func (r *TransactionRepository) UnvoidTransaction(ctx context.Context, params TransactionLifecycleParams) (TransactionRecord, error) {
	return withTransactionRecordTx(r, ctx, "unvoid transaction", func(tx *sql.Tx) (TransactionRecord, error) {

		if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
			return TransactionRecord{}, err
		}
		current, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
		if err != nil {
			return TransactionRecord{}, err
		}
		if current.DeletedAt.Valid {
			return TransactionRecord{}, ErrTransactionDeleted
		}
		if current.Status != "voided" {
			records := []TransactionRecord{current}
			if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
				return TransactionRecord{}, err
			}
			return records[0], nil
		}

		var prior TransactionRecord
		if err := scanTransactionRecord(tx.QueryRowContext(ctx, transactionVersionSelect("transaction_versions", `
		WHERE tv.id = (
			SELECT prior.id FROM transaction_versions prior
			WHERE prior.transaction_id = ? AND prior.status <> 'voided'
			ORDER BY prior.version_seq DESC, prior.id DESC LIMIT 1
		)
	`), params.TransactionID), &prior); err != nil {
			return TransactionRecord{}, err
		}
		priorRecords := []TransactionRecord{prior}
		if err := loadTransactionChildrenTx(ctx, tx, priorRecords); err != nil {
			return TransactionRecord{}, err
		}
		prior = priorRecords[0]

		auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
			BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID,
			OccurredAt: params.RecordedAt, RequestID: params.RequestID, OriginType: params.OriginType,
			Operation: params.Operation, Reason: params.ChangeReason,
		})
		if err != nil {
			return TransactionRecord{}, err
		}
		record, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
			BookID: params.BookID, TransactionID: params.TransactionID, VersionSeq: current.VersionSeq + 1,
			SupersedesVersionID: sql.NullInt64{Int64: current.VersionID, Valid: true},
			Spec:                transactionSpecFromRecord(prior), ReplaceTags: false, RecordedAt: params.RecordedAt,
			ChangedByUserID: params.ActorUserID, ChangeReason: params.ChangeReason,
			ChangeAuditEventID: auditEventID, RequestID: params.RequestID,
			TransactionDaySequence: prior.TransactionDaySequence,
		})
		if err != nil {
			return TransactionRecord{}, mapTransactionConstraintError(err)
		}
		invalidated, err := invalidateReconciliationCheckpoints(ctx, tx, checkpointInvalidationParams{
			BookID: params.BookID, Refs: params.InvalidateCheckpointRefs, ActorUserID: params.ActorUserID,
			AuditEventID: auditEventID, OccurredAt: params.RecordedAt, Reason: params.InvalidateCheckpointReason,
		})
		if err != nil {
			return TransactionRecord{}, err
		}
		record.InvalidatedCheckpointIDs = invalidated
		return record, nil
	})
}

func (r *TransactionRepository) SetTransactionDeleted(ctx context.Context, params SetTransactionDeletedParams) (TransactionRecord, error) {
	return withTransactionRecordTx(r, ctx, "set transaction deleted", func(tx *sql.Tx) (TransactionRecord, error) {
		if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
			return TransactionRecord{}, err
		}
		current, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
		if err != nil {
			return TransactionRecord{}, err
		}
		if current.DeletedAt.Valid == params.Deleted {
			records := []TransactionRecord{current}
			if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
				return TransactionRecord{}, err
			}
			return records[0], nil
		}
		if params.Deleted && current.Status == "draft" {
			return TransactionRecord{}, ErrTransactionHasPostedVersions
		}
		auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
			BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID,
			OccurredAt: params.RecordedAt, RequestID: params.RequestID, OriginType: params.OriginType,
			Operation: params.Operation, Reason: params.ChangeReason,
		})
		if err != nil {
			return TransactionRecord{}, err
		}
		if params.Deleted {
			_, err = tx.ExecContext(ctx, `UPDATE transactions SET deleted_at = ?, deleted_by_user_id = ?, deleted_audit_event_id = ?, delete_reason = ? WHERE book_id = ? AND id = ?`, params.RecordedAt, params.ActorUserID, auditEventID, params.ChangeReason, params.BookID, params.TransactionID)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE transactions SET deleted_at = NULL WHERE book_id = ? AND id = ?`, params.BookID, params.TransactionID)
		}
		if err != nil {
			return TransactionRecord{}, fmt.Errorf("set transaction deleted: %w", err)
		}
		action := "restore"
		if params.Deleted {
			action = "soft_delete"
		}
		if _, err := tx.ExecContext(ctx, `
		INSERT INTO transaction_deletion_events (book_id, transaction_id, action, occurred_at, actor_user_id, audit_event_id, reason)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, params.BookID, params.TransactionID, action, params.RecordedAt, params.ActorUserID, auditEventID, params.ChangeReason); err != nil {
			return TransactionRecord{}, fmt.Errorf("insert transaction deletion event: %w", err)
		}
		invalidated, err := invalidateReconciliationCheckpoints(ctx, tx, checkpointInvalidationParams{
			BookID: params.BookID, Refs: params.InvalidateCheckpointRefs, ActorUserID: params.ActorUserID,
			AuditEventID: auditEventID, OccurredAt: params.RecordedAt, Reason: params.InvalidateCheckpointReason,
		})
		if err != nil {
			return TransactionRecord{}, err
		}
		record, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
		if err != nil {
			return TransactionRecord{}, err
		}
		records := []TransactionRecord{record}
		if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
			return TransactionRecord{}, err
		}
		record = records[0]
		record.InvalidatedCheckpointIDs = invalidated
		return record, nil
	})
}

func (r *TransactionRepository) ApproveTransaction(ctx context.Context, params ApproveTransactionParams) (TransactionRecord, error) {
	return withTransactionRecordTx(r, ctx, "approve transaction", func(tx *sql.Tx) (TransactionRecord, error) {

		if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
			return TransactionRecord{}, err
		}
		current, err := transactionByID(ctx, tx, params.BookID, params.TransactionID)
		if err != nil {
			return TransactionRecord{}, err
		}
		if !current.NeedsReview {
			records := []TransactionRecord{current}
			if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
				return TransactionRecord{}, err
			}
			return records[0], nil
		}

		auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
			BookID:        params.BookID,
			ActorUserID:   params.ActorUserID,
			AuthSessionID: params.AuthSessionID,
			OccurredAt:    params.RecordedAt,
			RequestID:     params.RequestID,
			OriginType:    params.OriginType,
			Operation:     params.Operation,
			Reason:        params.ChangeReason,
		})
		if err != nil {
			return TransactionRecord{}, err
		}

		spec := transactionSpecFromRecord(current)
		spec.NeedsReview = false
		record, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
			BookID:                 params.BookID,
			TransactionID:          params.TransactionID,
			VersionSeq:             current.VersionSeq + 1,
			SupersedesVersionID:    sql.NullInt64{Int64: current.VersionID, Valid: true},
			Spec:                   spec,
			ReplaceTags:            false,
			RecordedAt:             params.RecordedAt,
			ChangedByUserID:        params.ActorUserID,
			ChangeReason:           params.ChangeReason,
			ChangeAuditEventID:     auditEventID,
			RequestID:              params.RequestID,
			TransactionDaySequence: current.TransactionDaySequence,
		})
		if err != nil {
			return TransactionRecord{}, err
		}

		return record, nil
	})
}

func (r *TransactionRepository) DeleteDraftTransaction(ctx context.Context, params DeleteDraftTransactionParams) error {
	return r.withTx(ctx, "delete draft transaction", func(tx *sql.Tx) error {

		if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
			return err
		}
		if _, err := transactionByID(ctx, tx, params.BookID, params.TransactionID); err != nil {
			return err
		}

		var durableCount int
		if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM transaction_versions
		WHERE transaction_id = ?
			AND status IN ('posted', 'voided')
	`, params.TransactionID).Scan(&durableCount); err != nil {
			return fmt.Errorf("count durable transaction versions: %w", err)
		}
		if durableCount > 0 {
			return ErrTransactionHasPostedVersions
		}

		if _, err := tx.ExecContext(ctx, `
		DELETE FROM posting_tags
		WHERE posting_line_id IN (
			SELECT id FROM posting_lines WHERE transaction_id = ?
		)
	`, params.TransactionID); err != nil {
			return fmt.Errorf("delete posting tags: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM transaction_tags WHERE transaction_id = ?", params.TransactionID); err != nil {
			return fmt.Errorf("delete transaction tags: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
		DELETE FROM posting_versions
		WHERE transaction_version_id IN (
			SELECT id FROM transaction_versions WHERE transaction_id = ?
		)
	`, params.TransactionID); err != nil {
			return fmt.Errorf("delete posting versions: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
		DELETE FROM journal_entries
		WHERE transaction_version_id IN (
			SELECT id FROM transaction_versions WHERE transaction_id = ?
		)
	`, params.TransactionID); err != nil {
			return fmt.Errorf("delete journal entries: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM transaction_search WHERE transaction_id = ?", params.TransactionID); err != nil {
			return fmt.Errorf("delete transaction search rows: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM transaction_versions WHERE transaction_id = ?", params.TransactionID); err != nil {
			return fmt.Errorf("delete transaction versions: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM posting_lines WHERE transaction_id = ?", params.TransactionID); err != nil {
			return fmt.Errorf("delete posting lines: %w", err)
		}
		result, err := tx.ExecContext(ctx, "DELETE FROM transactions WHERE book_id = ? AND id = ?", params.BookID, params.TransactionID)
		if err != nil {
			return fmt.Errorf("delete transaction: %w", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("read delete transaction rows affected: %w", err)
		}
		if rowsAffected == 0 {
			return ErrNotFound
		}

		return nil
	})
}

func (r *TransactionRepository) insertTransactionVersion(ctx context.Context, tx *sql.Tx, params insertTransactionVersionParams) (TransactionRecord, error) {
	txDaySeq := params.TransactionDaySequence
	if txDaySeq <= 0 {
		// Allocate the next sequence position in (book_id, transaction_date).
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(transaction_day_sequence), 0) + 1
			FROM transaction_versions
			WHERE book_id = ? AND transaction_date = ?
		`, params.BookID, params.Spec.TransactionDate).Scan(&txDaySeq); err != nil {
			return TransactionRecord{}, fmt.Errorf("compute transaction day sequence: %w", err)
		}
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO transaction_versions (
			book_id,
			transaction_id,
			version_seq,
			supersedes_version_id,
			status,
			transaction_kind,
			transaction_date,
			transaction_day_sequence,
			payee_id,
			payee_name,
			description,
			external_ref_hint,
			note_markdown,
			metadata_json,
			needs_review,
			recorded_at,
			changed_by_user_id,
			change_reason,
			change_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?)
	`, params.BookID, params.TransactionID, params.VersionSeq, nullableInt64Value(params.SupersedesVersionID), params.Spec.Status, params.Spec.TransactionKind, params.Spec.TransactionDate, txDaySeq, nullableInt64Value(params.Spec.PayeeID), nullableStringValue(params.Spec.PayeeName), params.Spec.Description, params.Spec.ExternalRefHint, params.Spec.NoteMarkdown, params.Spec.MetadataJSON, boolToInt(params.Spec.NeedsReview), params.RecordedAt, params.ChangedByUserID, params.ChangeReason, params.ChangeAuditEventID)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("insert transaction version: %w", err)
	}
	transactionVersionID, err := result.LastInsertId()
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("read transaction version id: %w", err)
	}

	if params.ReplaceTags {
		if err := replaceTransactionTags(ctx, tx, params.BookID, params.TransactionID, params.Spec.TagIDs, params.RecordedAt, params.ChangedByUserID, params.ChangeAuditEventID); err != nil {
			return TransactionRecord{}, err
		}
	}

	postingLineIDs := make(map[int64]bool)
	for entryIndex, entry := range params.Spec.JournalEntries {
		entryResult, err := tx.ExecContext(ctx, `
			INSERT INTO journal_entries (
				book_id,
				transaction_version_id,
				entry_seq,
				entry_date,
				entry_kind,
				memo,
				metadata_json
			)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, params.BookID, transactionVersionID, entryIndex+1, entry.EntryDate, entry.EntryKind, entry.Memo, entry.MetadataJSON)
		if err != nil {
			return TransactionRecord{}, mapTransactionConstraintError(fmt.Errorf("insert journal entry: %w", err))
		}
		journalEntryID, err := entryResult.LastInsertId()
		if err != nil {
			return TransactionRecord{}, fmt.Errorf("read journal entry id: %w", err)
		}

		for postingIndex, posting := range entry.Postings {
			postingLineID, err := ensurePostingLine(ctx, tx, params.BookID, params.TransactionID, posting.LineKey, params.RecordedAt, params.ChangedByUserID, params.RequestID, params.ChangeAuditEventID)
			if err != nil {
				return TransactionRecord{}, err
			}
			postingLineIDs[postingLineID] = true

			acctDaySeq, overridden := params.PostingSeqOverrides[postingLineID]
			if !overridden {
				acctDaySeq, err = allocateOrInheritAccountDaySeq(ctx, tx, params.BookID, params.SupersedesVersionID.Int64, postingLineID, posting.AccountID, entry.EntryDate)
				if err != nil {
					return TransactionRecord{}, err
				}
			}

			postingResult, err := tx.ExecContext(ctx, `
				INSERT INTO posting_versions (
					book_id,
					transaction_version_id,
					journal_entry_id,
					posting_line_id,
					line_seq,
					account_id,
					account_day_sequence,
					quantity_value,
					quantity_scale,
					commodity_id,
					memo,
					reconciliation_status,
					cleared_on,
					metadata_json
				)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, params.BookID, transactionVersionID, journalEntryID, postingLineID, postingIndex+1, posting.AccountID, acctDaySeq, posting.QuantityValue, posting.QuantityScale, posting.CommodityID, posting.Memo, posting.ReconciliationStatus, nullableStringValue(posting.ClearedOn), posting.MetadataJSON)
			if err != nil {
				return TransactionRecord{}, mapTransactionConstraintError(fmt.Errorf("insert posting version: %w", err))
			}
			if _, err := postingResult.LastInsertId(); err != nil {
				return TransactionRecord{}, fmt.Errorf("read posting version id: %w", err)
			}

			if params.ReplaceTags {
				if err := replacePostingTags(ctx, tx, params.BookID, postingLineID, posting.TagIDs, params.RecordedAt, params.ChangedByUserID, params.ChangeAuditEventID); err != nil {
					return TransactionRecord{}, err
				}
			}
		}
	}
	if params.ReplaceTags {
		if err := clearUnusedPostingLineTags(ctx, tx, params.BookID, params.TransactionID, postingLineIDs); err != nil {
			return TransactionRecord{}, err
		}
	}

	record, err := transactionVersionByID(ctx, tx, transactionVersionID)
	if err != nil {
		return TransactionRecord{}, err
	}
	records := []TransactionRecord{record}
	if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
		return TransactionRecord{}, err
	}

	return records[0], nil
}

func replaceTransactionTags(ctx context.Context, tx *sql.Tx, bookID int64, transactionID int64, tagIDs []int64, recordedAt string, actorUserID int64, auditEventID int64) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM transaction_tags WHERE book_id = ? AND transaction_id = ?", bookID, transactionID); err != nil {
		return fmt.Errorf("delete transaction tags: %w", err)
	}
	for _, tagID := range tagIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transaction_tags (
				book_id,
				transaction_id,
				tag_id,
				created_at,
				created_by_user_id,
				created_audit_event_id
			)
			VALUES (?, ?, ?, ?, ?, ?)
		`, bookID, transactionID, tagID, recordedAt, actorUserID, auditEventID); err != nil {
			return mapTransactionConstraintError(fmt.Errorf("insert transaction tag: %w", err))
		}
	}
	return nil
}

func replacePostingTags(ctx context.Context, tx *sql.Tx, bookID int64, postingLineID int64, tagIDs []int64, recordedAt string, actorUserID int64, auditEventID int64) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM posting_tags WHERE book_id = ? AND posting_line_id = ?", bookID, postingLineID); err != nil {
		return fmt.Errorf("delete posting tags: %w", err)
	}
	for _, tagID := range tagIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO posting_tags (
				book_id,
				posting_line_id,
				tag_id,
				created_at,
				created_by_user_id,
				created_audit_event_id
			)
			VALUES (?, ?, ?, ?, ?, ?)
		`, bookID, postingLineID, tagID, recordedAt, actorUserID, auditEventID); err != nil {
			return mapTransactionConstraintError(fmt.Errorf("insert posting tag: %w", err))
		}
	}
	return nil
}

func clearUnusedPostingLineTags(ctx context.Context, tx *sql.Tx, bookID int64, transactionID int64, usedPostingLineIDs map[int64]bool) error {
	if len(usedPostingLineIDs) == 0 {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM posting_tags
			WHERE book_id = ?
				AND posting_line_id IN (
					SELECT id FROM posting_lines WHERE transaction_id = ?
				)
		`, bookID, transactionID); err != nil {
			return fmt.Errorf("delete all posting tags for transaction: %w", err)
		}
		return nil
	}

	placeholders := make([]string, 0, len(usedPostingLineIDs))
	args := []any{bookID, transactionID}
	for postingLineID := range usedPostingLineIDs {
		placeholders = append(placeholders, "?")
		args = append(args, postingLineID)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM posting_tags
		WHERE book_id = ?
			AND posting_line_id IN (
				SELECT id FROM posting_lines WHERE transaction_id = ?
			)
			AND posting_line_id NOT IN (`+strings.Join(placeholders, ",")+`)
	`, args...); err != nil {
		return fmt.Errorf("delete unused posting line tags: %w", err)
	}
	return nil
}

func ensurePostingLine(ctx context.Context, tx *sql.Tx, bookID int64, transactionID int64, lineKey string, createdAt string, actorUserID int64, requestID string, auditEventID int64) (int64, error) {
	var postingLineID int64
	err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM posting_lines
		WHERE book_id = ?
			AND transaction_id = ?
			AND line_key = ?
	`, bookID, transactionID, lineKey).Scan(&postingLineID)
	if err == nil {
		return postingLineID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("read posting line: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO posting_lines (
			book_id,
			transaction_id,
			line_key,
			created_at,
			created_by_user_id,
			created_request_id,
			created_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), ?)
	`, bookID, transactionID, lineKey, createdAt, actorUserID, requestID, auditEventID)
	if err != nil {
		return 0, fmt.Errorf("insert posting line: %w", err)
	}

	postingLineID, err = result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read posting line id: %w", err)
	}

	return postingLineID, nil
}
