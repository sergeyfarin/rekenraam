package db

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"rekenraam/backend/internal/exact"
)

// RegisterStatementRecord is one active reconciliation checkpoint shown in an
// account register as a statement row (#595). It is a marker at the
// checkpoint's boundary, never a posting.
type RegisterStatementRecord struct {
	CheckpointID             int64
	CommodityID              int64
	StatementDate            string
	StatementAccountSequence int64
	StatementBalanceValue    exact.Coefficient
	StatementBalanceScale    int
}

// AccountRegisterStatements returns the account's active checkpoints in
// register order — statement_date, statement_account_sequence, id, all
// descending — after the register cursor, filtered by statement date.
//
// A statement row sits at its checkpoint's boundary: above every posting the
// boundary covers (entry_date < statement_date, or the same day with
// account_day_sequence <= statement_account_sequence) and below every posting
// after it. In the register's key (date, sequence, kind, id) a posting has kind
// 0 and a statement kind 1, so at an equal (date, sequence) the statement is
// the newer row.
func (r *TransactionRepository) AccountRegisterStatements(ctx context.Context, params ListTransactionsParams) ([]RegisterStatementRecord, error) {
	where := []string{"book_id = ?", "account_id = ?", "status = 'active'"}
	args := []any{params.BookID, params.AccountID}
	if params.AfterDate != "" {
		where = append(where, "statement_date >= ?")
		args = append(args, params.AfterDate)
	}
	if params.BeforeDate != "" {
		where = append(where, "statement_date <= ?")
		args = append(args, params.BeforeDate)
	}
	if params.CursorDate != "" && params.CursorID > 0 {
		if params.CursorStatement {
			where = append(where, `(
				statement_date < ?
				OR (statement_date = ? AND statement_account_sequence < ?)
				OR (statement_date = ? AND statement_account_sequence = ? AND id < ?)
			)`)
			args = append(args,
				params.CursorDate,
				params.CursorDate, params.CursorDaySequence,
				params.CursorDate, params.CursorDaySequence, params.CursorID,
			)
		} else {
			// After a posting at (date, sequence), a statement at the same
			// boundary is newer than it and was already returned.
			where = append(where, `(
				statement_date < ?
				OR (statement_date = ? AND statement_account_sequence < ?)
			)`)
			args = append(args, params.CursorDate, params.CursorDate, params.CursorDaySequence)
		}
	}
	args = append(args, params.Limit)
	rows, err := r.database.QueryContext(ctx, `
		SELECT id, commodity_id, statement_date, statement_account_sequence,
			statement_balance_value, statement_balance_scale
		FROM reconciliation_checkpoints
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY statement_date DESC, statement_account_sequence DESC, id DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list register statements: %w", err)
	}
	defer rows.Close()
	var records []RegisterStatementRecord
	for rows.Next() {
		var record RegisterStatementRecord
		if err := rows.Scan(&record.CheckpointID, &record.CommodityID, &record.StatementDate,
			&record.StatementAccountSequence, &record.StatementBalanceValue, &record.StatementBalanceScale); err != nil {
			return nil, fmt.Errorf("scan register statement: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate register statements: %w", err)
	}
	return records, nil
}

// registerStatementCursorKind marks a register cursor that stops on a
// statement row rather than a posting (#595).
const registerStatementCursorKind = "s"

// EncodeRegisterStatementCursor encodes a register cursor that stops on a
// statement row: (statement_date, statement_account_sequence, checkpoint_id).
func EncodeRegisterStatementCursor(statementDate string, statementAccountSequence int64, checkpointID int64) string {
	if statementDate == "" || checkpointID <= 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s|%d|%d|%s",
		statementDate, statementAccountSequence, checkpointID, registerStatementCursorKind)))
}

// RegisterCursor is a decoded register cursor. Statement is true when the
// previous page ended on a statement row; ID is then a checkpoint id, otherwise
// a posting version id.
type RegisterCursor struct {
	Date        string
	DaySequence int64
	ID          int64
	Statement   bool
}

// DecodeRegisterCursor decodes a cursor produced by EncodeRegisterCursor or
// EncodeRegisterStatementCursor. A three-part cursor is a posting cursor, as
// every register cursor was before statement rows.
func DecodeRegisterCursor(cursor string) (RegisterCursor, error) {
	cleaned := strings.TrimSpace(cursor)
	if cleaned == "" {
		return RegisterCursor{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cleaned)
	if err != nil {
		return RegisterCursor{}, fmt.Errorf("cursor is invalid")
	}
	parts := strings.Split(string(decoded), "|")
	statement := false
	switch {
	case len(parts) == 4 && parts[3] == registerStatementCursorKind:
		statement = true
		parts = parts[:3]
	case len(parts) != 3:
		return RegisterCursor{}, fmt.Errorf("cursor is invalid")
	}
	date, sequence, id, err := DecodeTransactionCursor(base64.RawURLEncoding.EncodeToString([]byte(strings.Join(parts, "|"))))
	if err != nil {
		return RegisterCursor{}, err
	}
	return RegisterCursor{Date: date, DaySequence: sequence, ID: id, Statement: statement}, nil
}
