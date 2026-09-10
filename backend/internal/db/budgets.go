package db

import (
	"context"
	"database/sql"
	"fmt"
)

type BudgetRepository struct{ database *sql.DB }

func NewBudgetRepository(database *sql.DB) *BudgetRepository {
	return &BudgetRepository{database: database}
}

type BudgetCategoryRecord struct {
	ID                       int64
	Code, Name, CategoryType string
	AllowsPostings           bool
}

type BudgetAccountRecord struct {
	ID                                               int64
	Code, Name, AccountClass, AccountKind, Treatment string
}

type BudgetCommodityRecord struct {
	ID           int64
	Code, Symbol string
	Scale        int
}

type BudgetAmountRecord struct {
	CategoryID, CommodityID int64
	QuantityValue           string
	QuantityScale           int
}

type BudgetActualRecord = BudgetAmountRecord

type BudgetSnapshotRecord struct {
	Categories  []BudgetCategoryRecord
	Accounts    []BudgetAccountRecord
	Commodities []BudgetCommodityRecord
	Targets     []BudgetAmountRecord
	Actuals     []BudgetActualRecord
}

func (r *BudgetRepository) Snapshot(ctx context.Context, bookID int64, start, end string) (BudgetSnapshotRecord, error) {
	var out BudgetSnapshotRecord
	rows, err := r.database.QueryContext(ctx, `
		SELECT a.id, COALESCE(av.code,''), COALESCE(av.name,''), av.account_class, av.allows_postings
		FROM accounts a JOIN current_account_versions av ON av.account_id=a.id
		WHERE a.book_id=? AND a.system_role IS NULL AND av.status='active'
		  AND av.account_class IN ('income','expense')
		ORDER BY av.account_class, COALESCE(av.name,av.code), a.id`, bookID)
	if err != nil {
		return out, fmt.Errorf("read budget categories: %w", err)
	}
	for rows.Next() {
		var v BudgetCategoryRecord
		if err := rows.Scan(&v.ID, &v.Code, &v.Name, &v.CategoryType, &v.AllowsPostings); err != nil {
			rows.Close()
			return out, err
		}
		out.Categories = append(out.Categories, v)
	}
	if err := rows.Close(); err != nil {
		return out, err
	}

	rows, err = r.database.QueryContext(ctx, `
		SELECT a.id, COALESCE(av.code,''), COALESCE(av.name,''), av.account_class, av.account_kind,
		COALESCE((SELECT btv.treatment FROM account_budget_treatment_versions btv
		 WHERE btv.account_id=a.id AND btv.effective_from<=?
		 ORDER BY btv.effective_from DESC, btv.version_seq DESC LIMIT 1),
		 CASE WHEN av.account_kind IN ('cash','checking','savings','credit_card','line_of_credit')
		      THEN 'on_budget' ELSE 'off_budget' END)
		FROM accounts a JOIN current_account_versions av ON av.account_id=a.id
		WHERE a.book_id=? AND a.system_role IS NULL AND av.status!='archived'
		  AND av.account_class IN ('asset','liability')
		ORDER BY av.account_class, COALESCE(av.name,av.code), a.id`, end, bookID)
	if err != nil {
		return out, fmt.Errorf("read budget accounts: %w", err)
	}
	for rows.Next() {
		var v BudgetAccountRecord
		if err := rows.Scan(&v.ID, &v.Code, &v.Name, &v.AccountClass, &v.AccountKind, &v.Treatment); err != nil {
			rows.Close()
			return out, err
		}
		out.Accounts = append(out.Accounts, v)
	}
	if err := rows.Close(); err != nil {
		return out, err
	}

	rows, err = r.database.QueryContext(ctx, `SELECT c.id,c.code,COALESCE(NULLIF(cv.display_symbol,''),cv.symbol),cv.standard_scale
		FROM commodities c JOIN current_commodity_versions cv ON cv.commodity_id=c.id
		WHERE c.book_id=? AND c.kind='currency' AND cv.status='active' ORDER BY c.code`, bookID)
	if err != nil {
		return out, fmt.Errorf("read budget currencies: %w", err)
	}
	for rows.Next() {
		var v BudgetCommodityRecord
		if err := rows.Scan(&v.ID, &v.Code, &v.Symbol, &v.Scale); err != nil {
			rows.Close()
			return out, err
		}
		out.Commodities = append(out.Commodities, v)
	}
	if err := rows.Close(); err != nil {
		return out, err
	}

	rows, err = r.database.QueryContext(ctx, `SELECT category_account_id,commodity_id,quantity_value,quantity_scale FROM budget_targets WHERE book_id=? AND period_start=? ORDER BY category_account_id,commodity_id`, bookID, start)
	if err != nil {
		return out, fmt.Errorf("read budget targets: %w", err)
	}
	for rows.Next() {
		var v BudgetAmountRecord
		if err := rows.Scan(&v.CategoryID, &v.CommodityID, &v.QuantityValue, &v.QuantityScale); err != nil {
			rows.Close()
			return out, err
		}
		out.Targets = append(out.Targets, v)
	}
	if err := rows.Close(); err != nil {
		return out, err
	}

	rows, err = r.database.QueryContext(ctx, `
		SELECT pv.account_id,pv.commodity_id,pv.quantity_value,pv.quantity_scale
		FROM current_transaction_versions tv
		JOIN transactions t ON t.id=tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id=tv.id
		JOIN posting_versions pv ON pv.journal_entry_id=je.id
		JOIN accounts ca ON ca.id=pv.account_id
		JOIN account_versions cav ON cav.id=(SELECT x.id FROM account_versions x WHERE x.account_id=ca.id AND x.effective_from<=je.entry_date ORDER BY x.effective_from DESC,x.version_seq DESC LIMIT 1)
		WHERE tv.book_id=? AND tv.status='posted' AND t.deleted_at IS NULL
		  AND je.entry_date>=? AND je.entry_date<=? AND ca.system_role IS NULL
		  AND cav.account_class IN ('income','expense') AND cav.account_kind=cav.account_class
		  AND EXISTS (
		    SELECT 1 FROM posting_versions cpv
		    JOIN accounts ba ON ba.id=cpv.account_id
		    JOIN account_versions bav ON bav.id=(SELECT y.id FROM account_versions y WHERE y.account_id=ba.id AND y.effective_from<=je.entry_date ORDER BY y.effective_from DESC,y.version_seq DESC LIMIT 1)
		    WHERE cpv.journal_entry_id=je.id AND ba.system_role IS NULL AND bav.account_class IN ('asset','liability')
		      AND COALESCE((SELECT btv.treatment FROM account_budget_treatment_versions btv WHERE btv.account_id=ba.id AND btv.effective_from<=je.entry_date ORDER BY btv.effective_from DESC,btv.version_seq DESC LIMIT 1),
		        CASE WHEN bav.account_kind IN ('cash','checking','savings','credit_card','line_of_credit') THEN 'on_budget' ELSE 'off_budget' END)='on_budget'
		  )
		ORDER BY pv.account_id,pv.commodity_id,pv.id`, bookID, start, end)
	if err != nil {
		return out, fmt.Errorf("read budget actuals: %w", err)
	}
	for rows.Next() {
		var v BudgetActualRecord
		if err := rows.Scan(&v.CategoryID, &v.CommodityID, &v.QuantityValue, &v.QuantityScale); err != nil {
			rows.Close()
			return out, err
		}
		out.Actuals = append(out.Actuals, v)
	}
	if err := rows.Close(); err != nil {
		return out, err
	}
	return out, nil
}

type SetBudgetTargetParams struct {
	BookID, CategoryID, CommodityID, UserID, AuthSessionID int64
	PeriodStart, Value                                     string
	Scale                                                  int
	RequestID, OriginType, Operation, Reason, Now          string
}

func (r *BudgetRepository) SetTarget(ctx context.Context, p SetBudgetTargetParams) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	var categoryOK, commodityOK int
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts a JOIN current_account_versions av ON av.account_id=a.id WHERE a.id=? AND a.book_id=? AND a.system_role IS NULL AND av.status='active' AND av.allows_postings=1 AND av.account_class IN ('income','expense')),
	 EXISTS(SELECT 1 FROM commodities c JOIN current_commodity_versions cv ON cv.commodity_id=c.id WHERE c.id=? AND c.book_id=? AND c.kind='currency' AND cv.status='active')`, p.CategoryID, p.BookID, p.CommodityID, p.BookID).Scan(&categoryOK, &commodityOK)
	if err != nil {
		return err
	}
	if categoryOK == 0 || commodityOK == 0 {
		return ErrNotFound
	}
	auditID, err := insertAuditEvent(ctx, tx, AuditEventParams{BookID: p.BookID, ActorUserID: p.UserID, AuthSessionID: p.AuthSessionID, OccurredAt: p.Now, RequestID: p.RequestID, OriginType: p.OriginType, Operation: p.Operation, Reason: p.Reason})
	if err != nil {
		return err
	}
	if p.Value == "0" {
		_, err = tx.ExecContext(ctx, `DELETE FROM budget_targets WHERE book_id=? AND category_account_id=? AND commodity_id=? AND period_start=?`, p.BookID, p.CategoryID, p.CommodityID, p.PeriodStart)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO budget_targets(book_id,category_account_id,commodity_id,period_start,quantity_value,quantity_scale,created_at,updated_at,updated_by_user_id,updated_audit_event_id) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(book_id,category_account_id,commodity_id,period_start) DO UPDATE SET quantity_value=excluded.quantity_value,quantity_scale=excluded.quantity_scale,updated_at=excluded.updated_at,updated_by_user_id=excluded.updated_by_user_id,updated_audit_event_id=excluded.updated_audit_event_id`, p.BookID, p.CategoryID, p.CommodityID, p.PeriodStart, p.Value, p.Scale, p.Now, p.Now, p.UserID, auditID)
	}
	if err != nil {
		return fmt.Errorf("write budget target: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

type SetBudgetTreatmentParams struct {
	BookID, AccountID, UserID, AuthSessionID                                int64
	EffectiveFrom, Treatment, RequestID, OriginType, Operation, Reason, Now string
}

func (r *BudgetRepository) SetTreatment(ctx context.Context, p SetBudgetTreatmentParams) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	var ok int
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts a JOIN current_account_versions av ON av.account_id=a.id WHERE a.id=? AND a.book_id=? AND a.system_role IS NULL AND av.account_class IN ('asset','liability'))`, p.AccountID, p.BookID).Scan(&ok); err != nil {
		return err
	}
	if ok == 0 {
		return ErrNotFound
	}
	auditID, err := insertAuditEvent(ctx, tx, AuditEventParams{BookID: p.BookID, ActorUserID: p.UserID, AuthSessionID: p.AuthSessionID, OccurredAt: p.Now, RequestID: p.RequestID, OriginType: p.OriginType, Operation: p.Operation, Reason: p.Reason})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO account_budget_treatment_versions(account_id,version_seq,effective_from,treatment,recorded_at,changed_by_user_id,change_reason,change_audit_event_id) VALUES(?,COALESCE((SELECT MAX(version_seq)+1 FROM account_budget_treatment_versions WHERE account_id=?),1),?,?,?,?,?,?)`, p.AccountID, p.AccountID, p.EffectiveFrom, p.Treatment, p.Now, p.UserID, p.Reason, auditID)
	if err != nil {
		return fmt.Errorf("write budget treatment: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
