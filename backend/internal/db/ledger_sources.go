package db

import (
	"context"
	"fmt"
)

type LedgerImportSource struct {
	AccountID int64
	Kind      string
	Name      string
}

// LedgerImportSources describes configured online settlement accounts and
// observed committed file/profile imports. No inference from institution names
// and no connection credentials or configuration cross this read boundary.
func (r *AccountRepository) LedgerImportSources(ctx context.Context, bookID int64) ([]LedgerImportSource, error) {
	rows, err := r.database.QueryContext(ctx, `
 SELECT DISTINCT CAST(json_extract(s.resolution_json,'$.account_id') AS INTEGER),
 CASE WHEN b.connection_id IS NOT NULL THEN 'online' WHEN b.profile_id IS NOT NULL THEN 'profile' ELSE 'file' END,
 COALESCE(c.display_name,p.name,b.original_filename)
 FROM import_staged_rows s JOIN import_batches b ON b.id=s.batch_id AND b.book_id=s.book_id
 LEFT JOIN import_profiles p ON p.id=b.profile_id AND p.book_id=b.book_id
 LEFT JOIN import_connections c ON c.id=b.connection_id AND c.book_id=b.book_id
 WHERE s.book_id=? AND s.commit_status='committed' AND json_extract(s.resolution_json,'$.account_id') IS NOT NULL
 UNION
 SELECT cash_account_id,'online',display_name FROM import_connections WHERE book_id=? AND cash_account_id IS NOT NULL
 ORDER BY 1,2,3`, bookID, bookID)
	if err != nil {
		return nil, fmt.Errorf("read ledger import sources: %w", err)
	}
	defer rows.Close()
	result := []LedgerImportSource{}
	for rows.Next() {
		var source LedgerImportSource
		if err := rows.Scan(&source.AccountID, &source.Kind, &source.Name); err != nil {
			return nil, err
		}
		result = append(result, source)
	}
	return result, rows.Err()
}
