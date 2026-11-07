package app

import (
	"context"

	"rekenraam/backend/internal/db"
)

type InvestmentEntryAccount struct {
	Account
	InvestmentHolding    bool
	InvestmentSettlement bool
}

func (s *InvestmentService) EntryAccounts(ctx context.Context, asOf string) ([]InvestmentEntryAccount, error) {
	date, err := cleanRequiredDate(asOf, "as-of date")
	if err != nil {
		return nil, err
	}
	snapshot, err := s.accountService.repository.InvestmentEntryAccounts(ctx, BookID, date)
	if err != nil {
		return nil, err
	}
	result := make([]InvestmentEntryAccount, 0, len(snapshot.Accounts))
	for _, account := range snapshot.Accounts {
		if account.IsSystem {
			continue
		}
		rule := db.PostingAccountRule{AccountID: account.ID, AccountClass: account.AccountClass,
			Status: account.Status, OpenedOn: account.OpenedOn, ClosedOn: account.ClosedOn,
			EffectiveFrom: account.EffectiveFrom, AllowsPostings: account.AllowsPostings,
			IsSystem: account.IsSystem, AccountKind: account.AccountKind,
			BaseKind: snapshot.BaseKinds[account.AccountClass+":"+account.AccountKind]}
		result = append(result, InvestmentEntryAccount{Account: toAccount(account),
			InvestmentHolding: investmentAccountRoleError(rule, date, holdingRole) == nil,
			InvestmentSettlement: investmentAccountRoleError(rule, date, settlementRole) == nil &&
				account.DefaultCommodityID.Valid && snapshot.Currencies[account.DefaultCommodityID.Int64]})
	}
	return result, nil
}
