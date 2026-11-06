package app

import (
	"context"
	"database/sql"
	"slices"

	"rekenraam/backend/internal/db"
)

// investmentPoolScope owns exact saved membership for allocation consumers.
// Returns deliberately keeps its separately selected/default investment scope.
type investmentPoolScope struct {
	Current   db.AllocationRevision
	Revisions []db.AllocationRevision
	Accounts  []AllocationAccount
	Members   map[int64]bool
}

func resolveInvestmentPool(ctx context.Context, snapshot *sql.DB, accounts []db.LedgerAccountRecord, bases map[string]string) (investmentPoolScope, error) {
	out := investmentPoolScope{Accounts: []AllocationAccount{}, Members: map[int64]bool{}}
	var err error
	out.Revisions, err = db.AllocationRevisions(ctx, snapshot, BookID)
	if err != nil {
		return out, err
	}
	out.Current = db.AllocationRevision{AccountIDs: DefaultInvestmentPool(accounts, bases), Targets: []db.AllocationTarget{}}
	if len(out.Revisions) > 0 {
		out.Current = out.Revisions[0]
	}
	for _, id := range out.Current.AccountIDs {
		out.Members[id] = true
	}
	for _, a := range accounts {
		if !a.SystemRole.Valid && (a.AccountClass == "asset" || a.AccountClass == "liability") {
			out.Accounts = append(out.Accounts, AllocationAccount{ID: a.ID, Name: a.Name.String, Code: a.Code.String, Kind: a.AccountKind, Status: a.Status, Included: out.Members[a.ID]})
		}
	}
	return out, nil
}

// DefaultInvestmentPool is shared scope selection, not an available-cash or
// net-worth policy. Saved membership always wins, including an empty pool.
func DefaultInvestmentPool(accounts []db.LedgerAccountRecord, baseKinds map[string]string) []int64 {
	ids := []int64{}
	for _, a := range accounts {
		base := baseKinds[a.AccountKind]
		if !a.SystemRole.Valid && a.AccountClass == "asset" && (returnsDefaultBaseKinds[base] || base == "cash" || base == "bank_account" || a.AccountKind == "term_deposit") {
			ids = append(ids, a.ID)
		}
	}
	slices.Sort(ids)
	return ids
}
