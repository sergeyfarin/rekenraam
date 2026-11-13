package db

import (
	"context"
	"database/sql"
	"fmt"
)

// SearchCatalogItem carries reference identity only, never transaction content.
type SearchCatalogItem struct {
	Kind        string
	ID          int64
	Name        string
	Code        string
	BuiltinKey  string
	Status      string
	AccountKind string
}

type SearchRepository struct{ database *sql.DB }

func NewSearchRepository(database *sql.DB) *SearchRepository {
	return &SearchRepository{database: database}
}

func (r *SearchRepository) Catalog(ctx context.Context, bookID int64, afterKind string, afterID int64, limit int) ([]SearchCatalogItem, error) {
	// A single composed read. Cursor by stable identity, never translated name.
	rows, err := r.database.QueryContext(ctx, `WITH catalog AS (
 SELECT CASE WHEN `+categoryAccountPredicate("av")+` THEN 'category' ELSE 'account' END AS kind,
 a.id, COALESCE(av.name,'' ) AS name, COALESCE(av.code,'') AS code,
 COALESCE(json_extract(av.metadata_json, '$.category.builtin_key'),'') AS builtin_key,
 av.status, av.account_kind
 FROM accounts a JOIN `+CurrentReferenceVersions(ctx, "account")+` av ON av.account_id=a.id
 WHERE a.book_id=? AND a.system_role IS NULL AND av.account_kind NOT IN ('security_holding','fund_holding')
 UNION ALL
 SELECT 'instrument', ii.commodity_id, iiv.display_name,
 COALESCE(iiv.symbol,'') || ' ' || c.code, '', iiv.status, ''
 FROM investment_instruments ii JOIN commodities c ON c.id=ii.commodity_id
 JOIN `+CurrentReferenceVersions(ctx, "investment_instrument")+` iiv ON iiv.instrument_id=ii.id
 WHERE ii.book_id=?
 ) SELECT kind,id,name,code,builtin_key,status,account_kind FROM catalog
 WHERE kind > ? OR (kind = ? AND id > ?) ORDER BY kind,id LIMIT ?`, bookID, bookID, afterKind, afterKind, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("read search catalog: %w", err)
	}
	defer rows.Close()
	items := make([]SearchCatalogItem, 0)
	for rows.Next() {
		var item SearchCatalogItem
		if err := rows.Scan(&item.Kind, &item.ID, &item.Name, &item.Code, &item.BuiltinKey, &item.Status, &item.AccountKind); err != nil {
			return nil, fmt.Errorf("scan search catalog: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
