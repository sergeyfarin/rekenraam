package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (r *TransactionRepository) PostingAccountRule(ctx context.Context, bookID int64, accountID int64, entryDate string) (PostingAccountRule, error) {
	var rule PostingAccountRule
	var systemRole sql.NullString
	var allowsPostings int
	if err := r.database.QueryRowContext(ctx, `
		SELECT
			a.id,
			av.account_class,
			av.status,
			av.opened_on,
			av.closed_on,
			av.default_commodity_id,
			av.quantity_scale_override,
			av.allows_postings,
			a.system_role
		FROM accounts a
		JOIN account_versions av ON av.account_id = a.id
		WHERE a.book_id = ?
			AND a.id = ?
			AND av.id = (
				SELECT asof_av.id
				FROM account_versions asof_av
				WHERE asof_av.account_id = a.id
					AND asof_av.effective_from <= ?
				ORDER BY asof_av.effective_from DESC, asof_av.version_seq DESC
				LIMIT 1
			)
	`, bookID, accountID, entryDate).Scan(
		&rule.AccountID,
		&rule.AccountClass,
		&rule.Status,
		&rule.OpenedOn,
		&rule.ClosedOn,
		&rule.DefaultCommodityID,
		&rule.QuantityScaleOverride,
		&allowsPostings,
		&systemRole,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PostingAccountRule{}, ErrNotFound
		}
		return PostingAccountRule{}, fmt.Errorf("read posting account rule: %w", err)
	}
	rule.AllowsPostings = allowsPostings == 1
	rule.IsSystem = systemRole.Valid

	return rule, nil
}

// EarliestPostingAccountRule returns the account's first version, ignoring the
// entry date. It exists so callers can tell "there is no such account" apart
// from "the account did not exist yet on that date" — PostingAccountRule's
// as-of lookup returns ErrNotFound for both, which turns the ordinary mistake
// of entering history for a later-opened account into a message claiming the
// account itself is invalid.
func (r *TransactionRepository) EarliestPostingAccountRule(ctx context.Context, bookID int64, accountID int64) (PostingAccountRule, error) {
	var rule PostingAccountRule
	var systemRole sql.NullString
	var allowsPostings int
	if err := r.database.QueryRowContext(ctx, `
		SELECT
			a.id,
			av.account_class,
			av.status,
			av.opened_on,
			av.closed_on,
			av.default_commodity_id,
			av.quantity_scale_override,
			av.allows_postings,
			a.system_role
		FROM accounts a
		JOIN account_versions av ON av.account_id = a.id
		WHERE a.book_id = ?
			AND a.id = ?
			AND av.id = (
				SELECT first_av.id
				FROM account_versions first_av
				WHERE first_av.account_id = a.id
				ORDER BY first_av.effective_from, first_av.version_seq
				LIMIT 1
			)
	`, bookID, accountID).Scan(
		&rule.AccountID,
		&rule.AccountClass,
		&rule.Status,
		&rule.OpenedOn,
		&rule.ClosedOn,
		&rule.DefaultCommodityID,
		&rule.QuantityScaleOverride,
		&allowsPostings,
		&systemRole,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PostingAccountRule{}, ErrNotFound
		}
		return PostingAccountRule{}, fmt.Errorf("read earliest posting account rule: %w", err)
	}
	rule.AllowsPostings = allowsPostings == 1
	rule.IsSystem = systemRole.Valid

	return rule, nil
}

// CommodityExists reports whether the commodity exists in the book at all,
// regardless of when its first version became effective. Same purpose as
// EarliestPostingAccountRule: separate "no such commodity" from "not yet
// enabled on that date".
func (r *TransactionRepository) CommodityExists(ctx context.Context, bookID int64, commodityID int64) (bool, error) {
	var exists int
	if err := r.database.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM commodities c
			JOIN commodity_versions cv ON cv.commodity_id = c.id
			WHERE c.book_id = ? AND c.id = ?
		)
	`, bookID, commodityID).Scan(&exists); err != nil {
		return false, fmt.Errorf("read commodity existence: %w", err)
	}

	return exists == 1, nil
}

func (r *TransactionRepository) PostingCommodityRule(ctx context.Context, bookID int64, commodityID int64, entryDate string) (PostingCommodityRule, error) {
	var rule PostingCommodityRule
	if err := r.database.QueryRowContext(ctx, `
		SELECT
			c.id,
			c.book_id,
			c.kind,
			cv.status,
			cv.standard_scale,
			cv.max_quantity_scale
		FROM commodities c
		JOIN commodity_versions cv ON cv.commodity_id = c.id
		WHERE c.book_id = ?
			AND c.id = ?
			AND cv.id = (
				SELECT asof_cv.id
				FROM commodity_versions asof_cv
				WHERE asof_cv.commodity_id = c.id
					AND asof_cv.effective_from <= ?
				ORDER BY asof_cv.effective_from DESC, asof_cv.version_seq DESC
				LIMIT 1
			)
	`, bookID, commodityID, entryDate).Scan(&rule.CommodityID, &rule.CommodityBookID, &rule.CommodityKind, &rule.Status, &rule.StandardScale, &rule.MaxQuantityScale); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PostingCommodityRule{}, ErrNotFound
		}
		return PostingCommodityRule{}, fmt.Errorf("read posting commodity rule: %w", err)
	}

	return rule, nil
}

func (r *TransactionRepository) ActiveTagIDs(ctx context.Context, bookID int64, tagIDs []int64) (map[int64]bool, error) {
	if len(tagIDs) == 0 {
		return map[int64]bool{}, nil
	}

	placeholders := make([]string, 0, len(tagIDs))
	args := []any{bookID}
	for _, tagID := range tagIDs {
		placeholders = append(placeholders, "?")
		args = append(args, tagID)
	}

	rows, err := r.database.QueryContext(ctx, `
		SELECT id
		FROM tags
		WHERE book_id = ?
			AND status = 'active'
			AND id IN (`+strings.Join(placeholders, ",")+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("read active tags: %w", err)
	}
	defer rows.Close()

	result := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan active tag: %w", err)
		}
		result[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active tags: %w", err)
	}

	return result, nil
}
