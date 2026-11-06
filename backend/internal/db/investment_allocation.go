package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrAllocationStale = errors.New("allocation revision is stale")
var ErrAllocationReference = errors.New("allocation reference is invalid")

type AllocationCommodity struct {
	ID         int64  `json:"id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Targetable bool   `json:"targetable"`
}

func AllocationCommodities(ctx context.Context, database *sql.DB, bookID int64) ([]AllocationCommodity, error) {
	rows, err := database.QueryContext(ctx, `SELECT c.id,c.code,COALESCE(iv.display_name,cv.name,''),c.kind,i.id IS NOT NULL
  FROM commodities c
  JOIN `+CurrentReferenceVersions(ctx, "commodity")+` cv ON cv.commodity_id=c.id
  LEFT JOIN investment_instruments i ON i.book_id=c.book_id AND i.commodity_id=c.id
  LEFT JOIN `+CurrentReferenceVersions(ctx, "investment_instrument")+` iv ON iv.instrument_id=i.id
  WHERE c.book_id=? ORDER BY c.id`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AllocationCommodity{}
	for rows.Next() {
		var v AllocationCommodity
		if err := rows.Scan(&v.ID, &v.Code, &v.Name, &v.Kind, &v.Targetable); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// A zero commodity ID is the combined Cash and deposits bucket.
type AllocationTarget struct {
	CommodityID int64 `json:"commodity_id"`
	Units       int   `json:"units"`
}

type AllocationRevision struct {
	ID         int64              `json:"id"`
	SavedAt    string             `json:"saved_at"`
	AccountIDs []int64            `json:"account_ids"`
	Targets    []AllocationTarget `json:"targets"`
}

func AllocationRevisions(ctx context.Context, database *sql.DB, bookID int64) ([]AllocationRevision, error) {
	rows, err := database.QueryContext(ctx, `SELECT id, saved_at, account_ids_json, targets_json FROM investment_allocation_revisions WHERE book_id=? ORDER BY id DESC`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read allocation revisions: %w", err)
	}
	defer rows.Close()
	out := []AllocationRevision{}
	for rows.Next() {
		var v AllocationRevision
		var accounts, targets string
		if err := rows.Scan(&v.ID, &v.SavedAt, &accounts, &targets); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(accounts), &v.AccountIDs); err != nil {
			return nil, fmt.Errorf("decode allocation accounts: %w", err)
		}
		if err := json.Unmarshal([]byte(targets), &v.Targets); err != nil {
			return nil, fmt.Errorf("decode allocation targets: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *InvestmentRepository) SaveAllocationRevision(ctx context.Context, expected int64, revision AllocationRevision, audit AuditEventParams) (AllocationRevision, error) {
	accounts, err := json.Marshal(revision.AccountIDs)
	if err != nil {
		return revision, err
	}
	targets, err := json.Marshal(revision.Targets)
	if err != nil {
		return revision, err
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return revision, err
	}
	defer rollbackTx(ctx, tx)
	// Obtain SQLite's writer reservation before reading the comparison token.
	if _, err = tx.ExecContext(ctx, `UPDATE accounts SET id=id WHERE 0`); err != nil {
		return revision, err
	}
	var latest int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM investment_allocation_revisions WHERE book_id=?`, audit.BookID).Scan(&latest); err != nil {
		return revision, err
	}
	if latest != expected {
		return revision, ErrAllocationStale
	}
	var invalid int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM json_each(?) j LEFT JOIN accounts a ON a.id=j.value AND a.book_id=? LEFT JOIN `+CurrentReferenceVersions(ctx, "account")+` av ON av.account_id=a.id WHERE a.id IS NULL OR av.id IS NULL OR a.system_role IS NOT NULL OR av.account_class NOT IN ('asset','liability')`, string(accounts), audit.BookID).Scan(&invalid); err != nil {
		return revision, err
	}
	if invalid != 0 {
		return revision, ErrAllocationReference
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM json_each(?) j LEFT JOIN investment_instruments i ON i.commodity_id=json_extract(j.value,'$.commodity_id') AND i.book_id=? WHERE json_extract(j.value,'$.commodity_id')!=0 AND i.id IS NULL`, string(targets), audit.BookID).Scan(&invalid); err != nil {
		return revision, err
	}
	if invalid != 0 {
		return revision, ErrAllocationReference
	}
	if latest != 0 && len(revision.Targets) == 0 {
		var previous string
		if err = tx.QueryRowContext(ctx, `SELECT targets_json FROM investment_allocation_revisions WHERE id=?`, latest).Scan(&previous); err != nil {
			return revision, err
		}
		if previous != "[]" {
			return revision, ErrAllocationReference
		}
	}
	auditID, err := insertAuditEvent(ctx, tx, audit)
	if err != nil {
		return revision, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO investment_allocation_revisions(book_id,saved_at,created_by_user_id,audit_event_id,account_ids_json,targets_json) VALUES(?,?,?,?,?,?)`, audit.BookID, revision.SavedAt, audit.ActorUserID, auditID, string(accounts), string(targets))
	if err != nil {
		return revision, err
	}
	revision.ID, err = result.LastInsertId()
	if err != nil {
		return revision, err
	}
	if err = tx.Commit(); err != nil {
		return revision, err
	}
	return revision, nil
}
