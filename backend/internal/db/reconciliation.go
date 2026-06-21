package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"rekenraam/backend/internal/exact"
)

var (
	ErrReconciliationNotFound    = errors.New("reconciliation not found")
	ErrReconciliationClosed      = errors.New("reconciliation is not open")
	ErrReconciliationNotBalanced = errors.New("reconciliation difference must be zero")
	ErrReconciliationPosting     = errors.New("reconciliation posting is invalid")
	ErrReconciliationCheckpoint  = errors.New("reconciliation checkpoint is invalid")
)

type ReconciliationSessionRecord struct {
	ID                    int64
	BookID                int64
	AccountID             int64
	CommodityID           int64
	Status                string
	SourceKind            string
	StatementDate         string
	StatementBalanceValue exact.Coefficient
	StatementBalanceScale int
	StartingCheckpointID  sql.NullInt64
	StartingBalanceValue  exact.Coefficient
	StartingBalanceScale  int
	SelectedBalanceValue  exact.Coefficient
	SelectedBalanceScale  int
	DifferenceValue       exact.Coefficient
	DifferenceScale       int
	MetadataJSON          string
	CreatedAt             string
	CreatedByUserID       int64
	UpdatedAt             string
	UpdatedByUserID       int64
	FinishedAt            sql.NullString
	VoidedAt              sql.NullString
	ChangeReason          string
	Candidates            []ReconciliationPostingRecord
	SelectedPostings      []ReconciliationPostingRecord
}

type ReconciliationCheckpointRecord struct {
	ID                    int64
	BookID                int64
	AccountID             int64
	CommodityID           int64
	SessionID             sql.NullInt64
	PreviousCheckpointID  sql.NullInt64
	Status                string
	StatementDate         string
	StatementBalanceValue exact.Coefficient
	StatementBalanceScale int
	CreatedAt             string
	CreatedByUserID       int64
	InvalidatedAt         sql.NullString
	InvalidationReason    string
	VoidedAt              sql.NullString
	VoidReason            string
}

type ReconciliationPostingRecord struct {
	PostingID            int64
	BookID               int64
	TransactionID        int64
	TransactionVersionID int64
	PostingLineID        int64
	AccountID            int64
	CommodityID          int64
	EntryDate            string
	QuantityValue        exact.Coefficient
	QuantityScale        int
	ReconciliationStatus string
	PayeeName            sql.NullString
	Description          string
	Selected             bool
}

type CreateReconciliationSessionParams struct {
	BookID                int64
	AccountID             int64
	CommodityID           int64
	SourceKind            string
	StatementDate         string
	StatementBalanceValue exact.Coefficient
	StatementBalanceScale int
	MetadataJSON          string
	ActorUserID           int64
	AuthSessionID         int64
	RequestID             string
	OriginType            string
	OccurredAt            string
	ChangeReason          string
}

type ReconciliationSessionSelectionParams struct {
	BookID               int64
	SessionID            int64
	PostingVersionIDs    []int64
	SelectedBalanceValue exact.Coefficient
	SelectedBalanceScale int
	DifferenceValue      exact.Coefficient
	DifferenceScale      int
	ActorUserID          int64
	AuthSessionID        int64
	RequestID            string
	OriginType           string
	OccurredAt           string
	Operation            string
	ChangeReason         string
}

type FinishReconciliationSessionParams struct {
	BookID        int64
	SessionID     int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	OccurredAt    string
	ChangeReason  string
}

type VoidReconciliationParams struct {
	BookID        int64
	ID            int64
	ActorUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	OccurredAt    string
	Operation     string
	ChangeReason  string
}

type ChangePostingReconciliationStatusParams struct {
	BookID            int64
	AccountID         int64
	PostingVersionIDs []int64
	Status            string
	ClearedOn         sql.NullString
	ActorUserID       int64
	AuthSessionID     int64
	RequestID         string
	OriginType        string
	OccurredAt        string
	ChangeReason      string
}

func (r *TransactionRepository) CreateReconciliationSession(ctx context.Context, params CreateReconciliationSessionParams) (ReconciliationSessionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("begin create reconciliation: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return ReconciliationSessionRecord{}, err
	}
	checkpoint, err := latestActiveReconciliationCheckpoint(ctx, tx, params.BookID, params.AccountID, params.CommodityID)
	if err != nil && !errors.Is(err, ErrReconciliationCheckpoint) {
		return ReconciliationSessionRecord{}, err
	}
	startingCheckpointID := sql.NullInt64{}
	startingValue := exact.New(0)
	startingScale := 0
	if err == nil {
		startingCheckpointID = sql.NullInt64{Int64: checkpoint.ID, Valid: true}
		startingValue = checkpoint.StatementBalanceValue
		startingScale = checkpoint.StatementBalanceScale
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.OccurredAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     "reconciliation.start",
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO reconciliation_sessions (
			book_id,
			account_id,
			commodity_id,
			status,
			source_kind,
			statement_date,
			statement_balance_value,
			statement_balance_scale,
			starting_checkpoint_id,
			starting_balance_value,
			starting_balance_scale,
			metadata_json,
			created_at,
			created_by_user_id,
			updated_at,
			updated_by_user_id,
			change_reason,
			created_audit_event_id,
			updated_audit_event_id
		)
		VALUES (?, ?, ?, 'open', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, params.BookID, params.AccountID, params.CommodityID, params.SourceKind, params.StatementDate, params.StatementBalanceValue, params.StatementBalanceScale, nullableInt64Value(startingCheckpointID), startingValue, startingScale, params.MetadataJSON, params.OccurredAt, params.ActorUserID, params.OccurredAt, params.ActorUserID, params.ChangeReason, auditEventID, auditEventID)
	if err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("insert reconciliation session: %w", err)
	}
	sessionID, err := result.LastInsertId()
	if err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("read reconciliation session id: %w", err)
	}

	session, err := reconciliationSessionByID(ctx, tx, params.BookID, sessionID)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	session.Candidates, err = reconciliationCandidates(ctx, tx, session)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("commit create reconciliation: %w", err)
	}
	committed = true
	return session, nil
}

func (r *TransactionRepository) ReconciliationSessionByID(ctx context.Context, bookID int64, sessionID int64) (ReconciliationSessionRecord, error) {
	session, err := reconciliationSessionByID(ctx, r.database, bookID, sessionID)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	session.Candidates, err = reconciliationCandidates(ctx, r.database, session)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	session.SelectedPostings, err = reconciliationSessionPostings(ctx, r.database, bookID, sessionID)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	return session, nil
}

func (r *TransactionRepository) ReplaceReconciliationSessionSelection(ctx context.Context, params ReconciliationSessionSelectionParams) (ReconciliationSessionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("begin update reconciliation selection: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return ReconciliationSessionRecord{}, err
	}
	session, err := reconciliationSessionByID(ctx, tx, params.BookID, params.SessionID)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	if session.Status != "open" {
		return ReconciliationSessionRecord{}, ErrReconciliationClosed
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.OccurredAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	postings, err := currentReconciliationPostingsByID(ctx, tx, session, params.PostingVersionIDs)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	if err := replaceSessionPostings(ctx, tx, session.ID, postings, params.OccurredAt, params.ActorUserID, auditEventID); err != nil {
		return ReconciliationSessionRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE reconciliation_sessions
		SET selected_balance_value = ?,
			selected_balance_scale = ?,
			difference_value = ?,
			difference_scale = ?,
			updated_at = ?,
			updated_by_user_id = ?,
			change_reason = ?,
			updated_audit_event_id = ?
		WHERE book_id = ? AND id = ?
	`, params.SelectedBalanceValue, params.SelectedBalanceScale, params.DifferenceValue, params.DifferenceScale, params.OccurredAt, params.ActorUserID, params.ChangeReason, auditEventID, params.BookID, params.SessionID); err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("update reconciliation session: %w", err)
	}

	session, err = reconciliationSessionByID(ctx, tx, params.BookID, params.SessionID)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	session.Candidates, err = reconciliationCandidates(ctx, tx, session)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	session.SelectedPostings = postings

	if err := tx.Commit(); err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("commit update reconciliation selection: %w", err)
	}
	committed = true
	return session, nil
}

func (r *TransactionRepository) FinishReconciliationSession(ctx context.Context, params FinishReconciliationSessionParams) (ReconciliationSessionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("begin finish reconciliation: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return ReconciliationSessionRecord{}, err
	}
	session, err := reconciliationSessionByID(ctx, tx, params.BookID, params.SessionID)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	if session.Status != "open" {
		return ReconciliationSessionRecord{}, ErrReconciliationClosed
	}
	if session.DifferenceValue.Sign() != 0 {
		return ReconciliationSessionRecord{}, ErrReconciliationNotBalanced
	}
	selected, err := reconciliationSessionPostings(ctx, tx, params.BookID, params.SessionID)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	if len(selected) == 0 {
		return ReconciliationSessionRecord{}, ErrReconciliationPosting
	}
	currentSelected, err := currentReconciliationPostingsByID(ctx, tx, session, postingIDs(selected))
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.OccurredAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     "reconciliation.finish",
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}

	reconciledPostingIDs, err := r.appendReconciliationStatusVersions(ctx, tx, appendReconciliationStatusParams{
		BookID:             params.BookID,
		Postings:           currentSelected,
		Status:             "reconciled",
		ClearedOn:          sql.NullString{String: session.StatementDate, Valid: true},
		ActorUserID:        params.ActorUserID,
		RequestID:          params.RequestID,
		RecordedAt:         params.OccurredAt,
		ChangeReason:       params.ChangeReason,
		ChangeAuditEventID: auditEventID,
	})
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}

	checkpointResult, err := tx.ExecContext(ctx, `
		INSERT INTO reconciliation_checkpoints (
			book_id,
			account_id,
			commodity_id,
			session_id,
			previous_checkpoint_id,
			status,
			statement_date,
			statement_balance_value,
			statement_balance_scale,
			created_at,
			created_by_user_id,
			created_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?, ?)
	`, params.BookID, session.AccountID, session.CommodityID, session.ID, nullableInt64Value(session.StartingCheckpointID), session.StatementDate, session.StatementBalanceValue, session.StatementBalanceScale, params.OccurredAt, params.ActorUserID, auditEventID)
	if err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("insert reconciliation checkpoint: %w", err)
	}
	checkpointID, err := checkpointResult.LastInsertId()
	if err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("read reconciliation checkpoint id: %w", err)
	}
	for _, posting := range reconciledPostingIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO reconciliation_checkpoint_postings (
				checkpoint_id,
				posting_version_id,
				book_id,
				account_id,
				commodity_id,
				entry_date,
				quantity_value,
				quantity_scale
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, checkpointID, posting.PostingID, posting.BookID, posting.AccountID, posting.CommodityID, posting.EntryDate, posting.QuantityValue, posting.QuantityScale); err != nil {
			return ReconciliationSessionRecord{}, fmt.Errorf("insert reconciliation checkpoint posting: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE reconciliation_sessions
		SET status = 'finished',
			finished_at = ?,
			updated_at = ?,
			updated_by_user_id = ?,
			change_reason = ?,
			updated_audit_event_id = ?
		WHERE book_id = ? AND id = ?
	`, params.OccurredAt, params.OccurredAt, params.ActorUserID, params.ChangeReason, auditEventID, params.BookID, params.SessionID); err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("finish reconciliation session: %w", err)
	}

	session, err = reconciliationSessionByID(ctx, tx, params.BookID, params.SessionID)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	session.SelectedPostings = reconciledPostingIDs

	if err := tx.Commit(); err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("commit finish reconciliation: %w", err)
	}
	committed = true
	return session, nil
}

func (r *TransactionRepository) VoidReconciliationSession(ctx context.Context, params VoidReconciliationParams) (ReconciliationSessionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("begin void reconciliation session: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return ReconciliationSessionRecord{}, err
	}
	session, err := reconciliationSessionByID(ctx, tx, params.BookID, params.ID)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	if session.Status == "voided" {
		if err := loadReconciliationSessionChildren(ctx, tx, &session); err != nil {
			return ReconciliationSessionRecord{}, err
		}
		if err := tx.Commit(); err != nil {
			return ReconciliationSessionRecord{}, fmt.Errorf("commit void reconciliation session: %w", err)
		}
		committed = true
		return session, nil
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.OccurredAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE reconciliation_sessions
		SET status = 'voided',
			voided_at = ?,
			updated_at = ?,
			updated_by_user_id = ?,
			change_reason = ?,
			updated_audit_event_id = ?
		WHERE book_id = ? AND id = ?
	`, params.OccurredAt, params.OccurredAt, params.ActorUserID, params.ChangeReason, auditEventID, params.BookID, params.ID); err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("void reconciliation session: %w", err)
	}
	session, err = reconciliationSessionByID(ctx, tx, params.BookID, params.ID)
	if err != nil {
		return ReconciliationSessionRecord{}, err
	}
	if err := loadReconciliationSessionChildren(ctx, tx, &session); err != nil {
		return ReconciliationSessionRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReconciliationSessionRecord{}, fmt.Errorf("commit void reconciliation session: %w", err)
	}
	committed = true
	return session, nil
}

func (r *TransactionRepository) ListReconciliationCheckpoints(ctx context.Context, bookID int64, accountID int64, commodityID int64) ([]ReconciliationCheckpointRecord, error) {
	where := []string{"book_id = ?", "account_id = ?"}
	args := []any{bookID, accountID}
	if commodityID > 0 {
		where = append(where, "commodity_id = ?")
		args = append(args, commodityID)
	}
	rows, err := r.database.QueryContext(ctx, `
		SELECT
			id, book_id, account_id, commodity_id, session_id, previous_checkpoint_id,
			status, statement_date, statement_balance_value, statement_balance_scale,
			created_at, created_by_user_id, invalidated_at, invalidation_reason,
			voided_at, void_reason
		FROM reconciliation_checkpoints
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY statement_date DESC, id DESC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list reconciliation checkpoints: %w", err)
	}
	defer rows.Close()
	return scanReconciliationCheckpoints(rows)
}

func (r *TransactionRepository) VoidReconciliationCheckpoint(ctx context.Context, params VoidReconciliationParams) (ReconciliationCheckpointRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return ReconciliationCheckpointRecord{}, fmt.Errorf("begin void reconciliation checkpoint: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return ReconciliationCheckpointRecord{}, err
	}
	checkpoint, err := reconciliationCheckpointByID(ctx, tx, params.BookID, params.ID)
	if err != nil {
		return ReconciliationCheckpointRecord{}, err
	}
	if checkpoint.Status == "voided" {
		if err := tx.Commit(); err != nil {
			return ReconciliationCheckpointRecord{}, fmt.Errorf("commit void reconciliation checkpoint: %w", err)
		}
		committed = true
		return checkpoint, nil
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.OccurredAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return ReconciliationCheckpointRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE reconciliation_checkpoints
		SET status = 'voided',
			voided_at = ?,
			voided_by_user_id = ?,
			void_reason = ?,
			voided_audit_event_id = ?
		WHERE book_id = ? AND id = ?
	`, params.OccurredAt, params.ActorUserID, params.ChangeReason, auditEventID, params.BookID, params.ID); err != nil {
		return ReconciliationCheckpointRecord{}, fmt.Errorf("void reconciliation checkpoint: %w", err)
	}
	checkpoint, err = reconciliationCheckpointByID(ctx, tx, params.BookID, params.ID)
	if err != nil {
		return ReconciliationCheckpointRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReconciliationCheckpointRecord{}, fmt.Errorf("commit void reconciliation checkpoint: %w", err)
	}
	committed = true
	return checkpoint, nil
}

func (r *TransactionRepository) ChangePostingReconciliationStatus(ctx context.Context, params ChangePostingReconciliationStatusParams) ([]TransactionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin change posting reconciliation status: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return nil, err
	}
	postings, err := currentAccountPostingsByID(ctx, tx, params.BookID, params.AccountID, params.PostingVersionIDs)
	if err != nil {
		return nil, err
	}
	for _, posting := range postings {
		if posting.ReconciliationStatus == "reconciled" {
			return nil, ErrReconciliationPosting
		}
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.OccurredAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     "reconciliation.posting_status",
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return nil, err
	}
	_, err = r.appendReconciliationStatusVersions(ctx, tx, appendReconciliationStatusParams{
		BookID:             params.BookID,
		Postings:           postings,
		Status:             params.Status,
		ClearedOn:          params.ClearedOn,
		ActorUserID:        params.ActorUserID,
		RequestID:          params.RequestID,
		RecordedAt:         params.OccurredAt,
		ChangeReason:       params.ChangeReason,
		ChangeAuditEventID: auditEventID,
	})
	if err != nil {
		return nil, err
	}
	records := make([]TransactionRecord, 0)
	seen := map[int64]bool{}
	for _, posting := range postings {
		if seen[posting.TransactionID] {
			continue
		}
		seen[posting.TransactionID] = true
		record, err := transactionByID(ctx, tx, params.BookID, posting.TransactionID)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit change posting reconciliation status: %w", err)
	}
	committed = true
	return records, nil
}

type appendReconciliationStatusParams struct {
	BookID             int64
	Postings           []ReconciliationPostingRecord
	Status             string
	ClearedOn          sql.NullString
	ActorUserID        int64
	RequestID          string
	RecordedAt         string
	ChangeReason       string
	ChangeAuditEventID int64
}

func (r *TransactionRepository) appendReconciliationStatusVersions(ctx context.Context, tx *sql.Tx, params appendReconciliationStatusParams) ([]ReconciliationPostingRecord, error) {
	byTransaction := map[int64]map[int64]bool{}
	for _, posting := range params.Postings {
		lineIDs := byTransaction[posting.TransactionID]
		if lineIDs == nil {
			lineIDs = map[int64]bool{}
			byTransaction[posting.TransactionID] = lineIDs
		}
		lineIDs[posting.PostingLineID] = true
	}
	reconciled := make([]ReconciliationPostingRecord, 0, len(params.Postings))
	for transactionID, lineIDs := range byTransaction {
		current, err := transactionByID(ctx, tx, params.BookID, transactionID)
		if err != nil {
			return nil, err
		}
		records := []TransactionRecord{current}
		if err := loadTransactionChildrenTx(ctx, tx, records); err != nil {
			return nil, err
		}
		current = records[0]
		spec := transactionSpecFromRecord(current)
		for entryIndex := range spec.JournalEntries {
			for postingIndex := range spec.JournalEntries[entryIndex].Postings {
				posting := &spec.JournalEntries[entryIndex].Postings[postingIndex]
				if lineIDs[postingLineIDByKey(current, posting.LineKey)] {
					posting.ReconciliationStatus = params.Status
					if params.Status == "uncleared" {
						posting.ClearedOn = sql.NullString{}
					} else {
						posting.ClearedOn = params.ClearedOn
					}
				}
			}
		}
		newRecord, err := r.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{
			BookID:              params.BookID,
			TransactionID:       transactionID,
			VersionSeq:          current.VersionSeq + 1,
			SupersedesVersionID: sql.NullInt64{Int64: current.VersionID, Valid: true},
			Spec:                spec,
			ReplaceTags:         false,
			RecordedAt:          params.RecordedAt,
			ChangedByUserID:     params.ActorUserID,
			ChangeReason:        params.ChangeReason,
			ChangeAuditEventID:  params.ChangeAuditEventID,
			RequestID:           params.RequestID,
		})
		if err != nil {
			return nil, err
		}
		for _, entry := range newRecord.JournalEntries {
			for _, posting := range entry.Postings {
				if lineIDs[posting.PostingLineID] {
					reconciled = append(reconciled, ReconciliationPostingRecord{
						PostingID:            posting.ID,
						BookID:               posting.BookID,
						TransactionID:        newRecord.ID,
						TransactionVersionID: posting.TransactionVersionID,
						PostingLineID:        posting.PostingLineID,
						AccountID:            posting.AccountID,
						CommodityID:          posting.CommodityID,
						EntryDate:            entry.EntryDate,
						QuantityValue:        posting.QuantityValue,
						QuantityScale:        posting.QuantityScale,
						ReconciliationStatus: posting.ReconciliationStatus,
						PayeeName:            newRecord.PayeeName,
						Description:          newRecord.Description,
					})
				}
			}
		}
	}
	return reconciled, nil
}

func transactionSpecFromRecord(record TransactionRecord) TransactionSpec {
	entries := make([]JournalEntrySpec, 0, len(record.JournalEntries))
	for _, entry := range record.JournalEntries {
		postings := make([]PostingSpec, 0, len(entry.Postings))
		for _, posting := range entry.Postings {
			postings = append(postings, PostingSpec{
				LineKey:              posting.LineKey,
				AccountID:            posting.AccountID,
				QuantityValue:        posting.QuantityValue,
				QuantityScale:        posting.QuantityScale,
				CommodityID:          posting.CommodityID,
				Memo:                 posting.Memo,
				ReconciliationStatus: posting.ReconciliationStatus,
				ClearedOn:            posting.ClearedOn,
				MetadataJSON:         posting.MetadataJSON,
				TagIDs:               posting.TagIDs,
			})
		}
		entries = append(entries, JournalEntrySpec{
			EntryDate:    entry.EntryDate,
			EntryKind:    entry.EntryKind,
			Memo:         entry.Memo,
			MetadataJSON: entry.MetadataJSON,
			Postings:     postings,
		})
	}
	return TransactionSpec{
		Status:          record.Status,
		TransactionKind: record.TransactionKind,
		TransactionDate: record.TransactionDate,
		PayeeID:         record.PayeeID,
		PayeeName:       record.PayeeName,
		Description:     record.Description,
		ExternalRefHint: nullStringText(record.ExternalRefHint),
		NoteMarkdown:    record.NoteMarkdown,
		MetadataJSON:    record.MetadataJSON,
		NeedsReview:     record.NeedsReview,
		TagIDs:          record.TagIDs,
		JournalEntries:  entries,
	}
}

func postingLineIDByKey(record TransactionRecord, lineKey string) int64 {
	for _, entry := range record.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.LineKey == lineKey {
				return posting.PostingLineID
			}
		}
	}
	return 0
}

type checkpointInvalidationParams struct {
	BookID       int64
	Refs         []CheckpointInvalidationRef
	ActorUserID  int64
	AuditEventID int64
	OccurredAt   string
	Reason       string
}

func invalidateReconciliationCheckpoints(ctx context.Context, tx *sql.Tx, params checkpointInvalidationParams) ([]int64, error) {
	if len(params.Refs) == 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	ids := map[int64]bool{}
	for _, ref := range params.Refs {
		if ref.AccountID <= 0 || ref.CommodityID <= 0 || ref.EntryDate == "" {
			continue
		}
		key := fmt.Sprintf("%d|%d|%s", ref.AccountID, ref.CommodityID, ref.EntryDate)
		if seen[key] {
			continue
		}
		seen[key] = true
		rows, err := tx.QueryContext(ctx, `
			SELECT id
			FROM reconciliation_checkpoints
			WHERE book_id = ?
				AND account_id = ?
				AND commodity_id = ?
				AND status = 'active'
				AND statement_date >= ?
			ORDER BY statement_date, id
		`, params.BookID, ref.AccountID, ref.CommodityID, ref.EntryDate)
		if err != nil {
			return nil, fmt.Errorf("read reconciliation checkpoints to invalidate: %w", err)
		}
		var checkpointIDs []int64
		for rows.Next() {
			var checkpointID int64
			if err := rows.Scan(&checkpointID); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan reconciliation checkpoint to invalidate: %w", err)
			}
			checkpointIDs = append(checkpointIDs, checkpointID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("iterate reconciliation checkpoints to invalidate: %w", err)
		}
		rows.Close()
		for _, checkpointID := range checkpointIDs {
			if _, err := tx.ExecContext(ctx, `
				UPDATE reconciliation_checkpoints
				SET status = 'invalidated',
					invalidated_at = ?,
					invalidated_by_user_id = ?,
					invalidation_reason = ?,
					invalidated_audit_event_id = ?
				WHERE book_id = ? AND id = ? AND status = 'active'
			`, params.OccurredAt, params.ActorUserID, params.Reason, params.AuditEventID, params.BookID, checkpointID); err != nil {
				return nil, fmt.Errorf("invalidate reconciliation checkpoint: %w", err)
			}
			ids[checkpointID] = true
		}
	}
	result := make([]int64, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	return result, nil
}

func reconciliationSessionByID(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, bookID int64, sessionID int64) (ReconciliationSessionRecord, error) {
	var session ReconciliationSessionRecord
	if err := queryer.QueryRowContext(ctx, `
		SELECT
			id, book_id, account_id, commodity_id, status, source_kind, statement_date,
			statement_balance_value, statement_balance_scale, starting_checkpoint_id,
			starting_balance_value, starting_balance_scale, selected_balance_value,
			selected_balance_scale, difference_value, difference_scale, metadata_json,
			created_at, created_by_user_id, updated_at, updated_by_user_id,
			finished_at, voided_at, change_reason
		FROM reconciliation_sessions
		WHERE book_id = ? AND id = ?
	`, bookID, sessionID).Scan(
		&session.ID,
		&session.BookID,
		&session.AccountID,
		&session.CommodityID,
		&session.Status,
		&session.SourceKind,
		&session.StatementDate,
		&session.StatementBalanceValue,
		&session.StatementBalanceScale,
		&session.StartingCheckpointID,
		&session.StartingBalanceValue,
		&session.StartingBalanceScale,
		&session.SelectedBalanceValue,
		&session.SelectedBalanceScale,
		&session.DifferenceValue,
		&session.DifferenceScale,
		&session.MetadataJSON,
		&session.CreatedAt,
		&session.CreatedByUserID,
		&session.UpdatedAt,
		&session.UpdatedByUserID,
		&session.FinishedAt,
		&session.VoidedAt,
		&session.ChangeReason,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ReconciliationSessionRecord{}, ErrReconciliationNotFound
		}
		return ReconciliationSessionRecord{}, fmt.Errorf("read reconciliation session: %w", err)
	}
	return session, nil
}

func latestActiveReconciliationCheckpoint(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, bookID int64, accountID int64, commodityID int64) (ReconciliationCheckpointRecord, error) {
	var checkpoint ReconciliationCheckpointRecord
	if err := scanReconciliationCheckpoint(queryer.QueryRowContext(ctx, `
		SELECT
			id, book_id, account_id, commodity_id, session_id, previous_checkpoint_id,
			status, statement_date, statement_balance_value, statement_balance_scale,
			created_at, created_by_user_id, invalidated_at, invalidation_reason,
			voided_at, void_reason
		FROM reconciliation_checkpoints
		WHERE book_id = ?
			AND account_id = ?
			AND commodity_id = ?
			AND status = 'active'
		ORDER BY statement_date DESC, id DESC
		LIMIT 1
	`, bookID, accountID, commodityID), &checkpoint); err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrNotFound) {
			return ReconciliationCheckpointRecord{}, ErrReconciliationCheckpoint
		}
		return ReconciliationCheckpointRecord{}, err
	}
	return checkpoint, nil
}

func loadReconciliationSessionChildren(ctx context.Context, queryer queryer, session *ReconciliationSessionRecord) error {
	candidates, err := reconciliationCandidates(ctx, queryer, *session)
	if err != nil {
		return err
	}
	selected, err := reconciliationSessionPostings(ctx, queryer, session.BookID, session.ID)
	if err != nil {
		return err
	}
	session.Candidates = candidates
	session.SelectedPostings = selected
	return nil
}

func reconciliationCheckpointByID(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, bookID int64, checkpointID int64) (ReconciliationCheckpointRecord, error) {
	var checkpoint ReconciliationCheckpointRecord
	if err := scanReconciliationCheckpoint(queryer.QueryRowContext(ctx, `
		SELECT
			id, book_id, account_id, commodity_id, session_id, previous_checkpoint_id,
			status, statement_date, statement_balance_value, statement_balance_scale,
			created_at, created_by_user_id, invalidated_at, invalidation_reason,
			voided_at, void_reason
		FROM reconciliation_checkpoints
		WHERE book_id = ? AND id = ?
	`, bookID, checkpointID), &checkpoint); err != nil {
		return ReconciliationCheckpointRecord{}, err
	}
	return checkpoint, nil
}

func reconciliationCandidates(ctx context.Context, queryer queryer, session ReconciliationSessionRecord) ([]ReconciliationPostingRecord, error) {
	rows, err := queryer.QueryContext(ctx, reconciliationPostingSelect(`
		WHERE tv.book_id = ?
			AND tv.status = 'posted'
			AND pv.account_id = ?
			AND pv.commodity_id = ?
			AND je.entry_date <= ?
			AND pv.reconciliation_status != 'reconciled'
		ORDER BY je.entry_date, pv.id
	`), session.BookID, session.AccountID, session.CommodityID, session.StatementDate)
	if err != nil {
		return nil, fmt.Errorf("read reconciliation candidates: %w", err)
	}
	defer rows.Close()
	return scanReconciliationPostings(rows)
}

func reconciliationSessionPostings(ctx context.Context, queryer queryer, bookID int64, sessionID int64) ([]ReconciliationPostingRecord, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT
			posting_version_id, book_id, transaction_id, transaction_version_id,
			posting_line_id, account_id, commodity_id, entry_date,
			quantity_value, quantity_scale, '' AS reconciliation_status,
			payee_name, description
		FROM reconciliation_session_postings
		WHERE book_id = ? AND session_id = ?
		ORDER BY entry_date, posting_version_id
	`, bookID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("read reconciliation session postings: %w", err)
	}
	defer rows.Close()
	postings, err := scanReconciliationPostings(rows)
	for index := range postings {
		postings[index].Selected = true
	}
	return postings, err
}

func currentReconciliationPostingsByID(ctx context.Context, queryer queryer, session ReconciliationSessionRecord, postingVersionIDs []int64) ([]ReconciliationPostingRecord, error) {
	if len(postingVersionIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, 0, len(postingVersionIDs))
	args := []any{session.BookID, session.AccountID, session.CommodityID, session.StatementDate}
	for _, postingID := range postingVersionIDs {
		placeholders = append(placeholders, "?")
		args = append(args, postingID)
	}
	rows, err := queryer.QueryContext(ctx, reconciliationPostingSelect(`
		WHERE tv.book_id = ?
			AND tv.status = 'posted'
			AND pv.account_id = ?
			AND pv.commodity_id = ?
			AND je.entry_date <= ?
			AND pv.reconciliation_status != 'reconciled'
			AND pv.id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY je.entry_date, pv.id
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("read selected reconciliation postings: %w", err)
	}
	defer rows.Close()
	postings, err := scanReconciliationPostings(rows)
	if err != nil {
		return nil, err
	}
	if len(postings) != len(uniqueInt64(postingVersionIDs)) {
		return nil, ErrReconciliationPosting
	}
	for index := range postings {
		postings[index].Selected = true
	}
	return postings, nil
}

func currentAccountPostingsByID(ctx context.Context, queryer queryer, bookID int64, accountID int64, postingVersionIDs []int64) ([]ReconciliationPostingRecord, error) {
	if len(postingVersionIDs) == 0 {
		return nil, ErrReconciliationPosting
	}
	placeholders := make([]string, 0, len(postingVersionIDs))
	args := []any{bookID, accountID}
	for _, postingID := range postingVersionIDs {
		placeholders = append(placeholders, "?")
		args = append(args, postingID)
	}
	rows, err := queryer.QueryContext(ctx, reconciliationPostingSelect(`
		WHERE tv.book_id = ?
			AND tv.status = 'posted'
			AND pv.account_id = ?
			AND pv.id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY je.entry_date, pv.id
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("read account reconciliation postings: %w", err)
	}
	defer rows.Close()
	postings, err := scanReconciliationPostings(rows)
	if err != nil {
		return nil, err
	}
	if len(postings) != len(uniqueInt64(postingVersionIDs)) {
		return nil, ErrReconciliationPosting
	}
	return postings, nil
}

func replaceSessionPostings(ctx context.Context, tx *sql.Tx, sessionID int64, postings []ReconciliationPostingRecord, occurredAt string, actorUserID int64, auditEventID int64) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM reconciliation_session_postings WHERE session_id = ?", sessionID); err != nil {
		return fmt.Errorf("delete reconciliation session postings: %w", err)
	}
	for _, posting := range postings {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO reconciliation_session_postings (
				session_id,
				posting_version_id,
				book_id,
				transaction_id,
				transaction_version_id,
				posting_line_id,
				account_id,
				commodity_id,
				entry_date,
				quantity_value,
				quantity_scale,
				payee_name,
				description,
				created_at,
				created_by_user_id,
				created_audit_event_id
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, sessionID, posting.PostingID, posting.BookID, posting.TransactionID, posting.TransactionVersionID, posting.PostingLineID, posting.AccountID, posting.CommodityID, posting.EntryDate, posting.QuantityValue, posting.QuantityScale, nullableStringValue(posting.PayeeName), posting.Description, occurredAt, actorUserID, auditEventID); err != nil {
			return fmt.Errorf("insert reconciliation session posting: %w", err)
		}
	}
	return nil
}

func reconciliationPostingSelect(extra string) string {
	return `
		SELECT
			pv.id,
			tv.book_id,
			t.id,
			tv.id,
			pv.posting_line_id,
			pv.account_id,
			pv.commodity_id,
			je.entry_date,
			pv.quantity_value,
			pv.quantity_scale,
			pv.reconciliation_status,
			tv.payee_name,
			tv.description
		FROM transactions t
		JOIN current_transaction_versions tv ON tv.transaction_id = t.id AND t.deleted_at IS NULL
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
	` + extra
}

func scanReconciliationPostings(rows *sql.Rows) ([]ReconciliationPostingRecord, error) {
	var postings []ReconciliationPostingRecord
	for rows.Next() {
		var posting ReconciliationPostingRecord
		if err := rows.Scan(
			&posting.PostingID,
			&posting.BookID,
			&posting.TransactionID,
			&posting.TransactionVersionID,
			&posting.PostingLineID,
			&posting.AccountID,
			&posting.CommodityID,
			&posting.EntryDate,
			&posting.QuantityValue,
			&posting.QuantityScale,
			&posting.ReconciliationStatus,
			&posting.PayeeName,
			&posting.Description,
		); err != nil {
			return nil, fmt.Errorf("scan reconciliation posting: %w", err)
		}
		postings = append(postings, posting)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reconciliation postings: %w", err)
	}
	return postings, nil
}

func scanReconciliationCheckpoints(rows *sql.Rows) ([]ReconciliationCheckpointRecord, error) {
	var checkpoints []ReconciliationCheckpointRecord
	for rows.Next() {
		var checkpoint ReconciliationCheckpointRecord
		if err := scanReconciliationCheckpoint(rows, &checkpoint); err != nil {
			return nil, err
		}
		checkpoints = append(checkpoints, checkpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reconciliation checkpoints: %w", err)
	}
	return checkpoints, nil
}

func scanReconciliationCheckpoint(scanner interface{ Scan(dest ...any) error }, checkpoint *ReconciliationCheckpointRecord) error {
	if err := scanner.Scan(
		&checkpoint.ID,
		&checkpoint.BookID,
		&checkpoint.AccountID,
		&checkpoint.CommodityID,
		&checkpoint.SessionID,
		&checkpoint.PreviousCheckpointID,
		&checkpoint.Status,
		&checkpoint.StatementDate,
		&checkpoint.StatementBalanceValue,
		&checkpoint.StatementBalanceScale,
		&checkpoint.CreatedAt,
		&checkpoint.CreatedByUserID,
		&checkpoint.InvalidatedAt,
		&checkpoint.InvalidationReason,
		&checkpoint.VoidedAt,
		&checkpoint.VoidReason,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("scan reconciliation checkpoint: %w", err)
	}
	return nil
}

func postingIDs(postings []ReconciliationPostingRecord) []int64 {
	ids := make([]int64, 0, len(postings))
	for _, posting := range postings {
		ids = append(ids, posting.PostingID)
	}
	return ids
}

func uniqueInt64(values []int64) []int64 {
	seen := map[int64]bool{}
	unique := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}
