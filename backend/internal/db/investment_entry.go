package db

import (
	"context"
	"database/sql"
	"fmt"
)

type InvestmentEntryAccountSnapshot struct {
	Accounts   []AccountRecord
	BaseKinds  map[string]string
	Currencies map[int64]bool
}

// InvestmentEntryAccounts reads the full dated set without a list limit or
// per-account queries. The three reads share one consistent snapshot.
func (r *AccountRepository) InvestmentEntryAccounts(ctx context.Context, bookID int64, date string) (InvestmentEntryAccountSnapshot, error) {
	result := InvestmentEntryAccountSnapshot{BaseKinds: map[string]string{}, Currencies: map[int64]bool{}}
	tx, err := r.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, fmt.Errorf("begin entry account snapshot: %w", err)
	}
	defer rollbackTx(ctx, tx)
	rows, err := tx.QueryContext(ctx, accountSelect("account_versions", `WHERE a.book_id = ? AND av.id = (
 SELECT v.id FROM account_versions v WHERE v.account_id = a.id AND v.effective_from <= ?
 ORDER BY v.effective_from DESC, v.version_seq DESC LIMIT 1) ORDER BY av.name, a.id`), bookID, date)
	if err != nil {
		return result, fmt.Errorf("read dated entry accounts: %w", err)
	}
	result.Accounts, err = scanAccountRecords(rows)
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT account_class || ':' || code, base_kind FROM account_kinds`)
	if err != nil {
		return result, fmt.Errorf("read entry account kinds: %w", err)
	}
	for rows.Next() {
		var key, base string
		if err := rows.Scan(&key, &base); err != nil {
			rows.Close()
			return result, err
		}
		result.BaseKinds[key] = base
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT c.id FROM commodities c JOIN commodity_versions cv ON cv.commodity_id = c.id
 WHERE c.book_id = ? AND c.kind = 'currency' AND cv.status = 'active' AND cv.id = (
 SELECT v.id FROM commodity_versions v WHERE v.commodity_id = c.id AND v.effective_from <= ?
 ORDER BY v.effective_from DESC, v.version_seq DESC LIMIT 1)`, bookID, date)
	if err != nil {
		return result, fmt.Errorf("read entry currencies: %w", err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return result, err
		}
		result.Currencies[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("finish entry account snapshot: %w", err)
	}
	return result, nil
}
