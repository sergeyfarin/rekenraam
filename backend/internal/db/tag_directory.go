package db

import (
	"context"
	"fmt"
)

type TagDirectoryParams struct {
	BookID    int64
	Status    string
	Query     string
	AfterName string
	AfterID   int64
	Limit     int
}

type TagDirectoryRow struct {
	ID               int64
	Name             string
	Kind             string
	Status           string
	TransactionCount int64
}

// taggedTransactionIDs uses the same tag membership for the Transactions
// filter and the directory counts. A posting line must belong to the current
// version. UNION counts a transaction only once, regardless of where it is tagged.
func taggedTransactionIDs(tagIDExpr string) string {
	return `
		SELECT tt.transaction_id FROM transaction_tags tt WHERE tt.tag_id = ` + tagIDExpr + `
		UNION
		SELECT pl.transaction_id
		FROM posting_tags pt
		JOIN posting_lines pl ON pl.id = pt.posting_line_id
		JOIN current_transaction_versions tv_tag ON tv_tag.transaction_id = pl.transaction_id
		JOIN posting_versions pv ON pv.transaction_version_id = tv_tag.id AND pv.posting_line_id = pl.id
		WHERE pt.tag_id = ` + tagIDExpr
}

func (r *TagRepository) TagDirectory(ctx context.Context, params TagDirectoryParams) ([]TagDirectoryRow, error) {
	rows, err := r.database.QueryContext(ctx, `
		WITH page AS (
			SELECT id, name, kind, status FROM tags
			WHERE book_id = ? AND (? = 'all' OR status = ?)
				AND instr(lower(name), lower(?)) > 0
				AND (name COLLATE NOCASE, id) > (?, ?)
			ORDER BY name COLLATE NOCASE, id
			LIMIT ?
		)
		SELECT page.id, page.name, page.kind, page.status, (
			SELECT count(*) FROM (`+taggedTransactionIDs("page.id")+`) tagged
			JOIN transactions t ON t.id = tagged.transaction_id AND t.deleted_at IS NULL
			JOIN current_transaction_versions tv ON tv.transaction_id = t.id AND tv.status <> 'draft'
		)
		FROM page
		ORDER BY page.name COLLATE NOCASE, page.id
	`, params.BookID, params.Status, params.Status, params.Query, params.AfterName, params.AfterID, params.Limit)
	if err != nil {
		return nil, fmt.Errorf("read tag directory: %w", err)
	}
	defer rows.Close()

	result := make([]TagDirectoryRow, 0)
	for rows.Next() {
		var row TagDirectoryRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Kind, &row.Status, &row.TransactionCount); err != nil {
			return nil, fmt.Errorf("scan tag directory: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tag directory: %w", err)
	}
	return result, nil
}
