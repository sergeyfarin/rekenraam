package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"rekenraam/backend/internal/db"
)

// T-98. The investment commands are the only writers allowed to post to a
// subledger-managed holding account, and they earn that exemption by writing
// the matching lot facts in the same database transaction. The exemption is
// granted per command, not per posting, so until now a command could spend it
// on a posting it wrote no lot for: a buy naming one holding account as both
// the holding and the cash leg produced two holding postings that cancelled to
// zero shares in the journal, plus a lot holding ten. Both halves passed their
// own checks — the entry balanced, the lot conserved — and the position was
// nonetheless incoherent.
//
// Checking that the account ids differ would not be enough, because two
// different holding accounts fail in exactly the same way. What has to be true
// is that each account plays the role the command means it to play, so that is
// what is checked here, against the account as of the transaction's own date.
//
// These rules describe the write boundary, not the UI: the pickers already
// offer only sensible combinations. They exist because the API is reachable
// without them.

// accountRoleForInvestment names the part an account plays in an investment
// command, for both the rule applied and the message a rejection carries.
type accountRoleForInvestment struct {
	label string
	// allowedClasses restricts the account_class the role accepts. Empty means
	// the role is defined by its kind rather than its class.
	allowedClasses []string
	// subledgerManaged is whether the role's account must be one whose holdings
	// the investment subledger owns.
	subledgerManaged bool
}

var (
	holdingRole = accountRoleForInvestment{
		label:            "holding account",
		allowedClasses:   []string{"asset"},
		subledgerManaged: true,
	}
	// Settlement is where the money actually moves: an asset the cash comes
	// from or lands in, or a liability such as a margin or card balance. This
	// is the same scope the cashflow report treats as cash, and it is never a
	// system account — the commodity_trading account is resolved internally and
	// is not a settlement choice.
	settlementRole = accountRoleForInvestment{
		label:          "cash account",
		allowedClasses: []string{"asset", "liability"},
	}
	incomeRole = accountRoleForInvestment{
		label:          "dividend income account",
		allowedClasses: []string{"income"},
	}
	// Withholding is the tax kept back at source. Most books record it as an
	// expense, some as a receivable or a liability; what it can never be is the
	// income it was deducted from, or equity.
	withholdingRole = accountRoleForInvestment{
		label:          "withholding account",
		allowedClasses: []string{"asset", "liability", "expense"},
	}
)

// accountInRole reads an account as of date and rejects it unless it can play
// role. It repeats the postable checks cleanPosting makes, because a role
// violation must be refused before the plan is built rather than reported as a
// posting error about an account the caller never named directly.
//
// The version it reads is recorded in dependencies, when the caller supplies
// one. A role decision made here is only honest for as long as the account
// still is what it was read as, and the command that acts on it commits later,
// in a database transaction of its own — so the version has to travel with the
// write and be checked there (T-100). Previews pass nil: they decide nothing
// that outlives the request.
func (s *InvestmentService) accountInRole(ctx context.Context, accountID int64, date string, role accountRoleForInvestment, dependencies *accountRuleDependencies) (db.PostingAccountRule, error) {
	if accountID <= 0 {
		return db.PostingAccountRule{}, ValidationError{Message: role.label + " is required"}
	}
	rule, err := s.transactionService.repository.PostingAccountRule(ctx, BookID, accountID, date)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.PostingAccountRule{}, ValidationError{Message: role.label + " is invalid"}
		}
		return db.PostingAccountRule{}, fmt.Errorf("read %s rule: %w", role.label, err)
	}
	// Recorded before the first rule is applied, so a rejection and an
	// acceptance bind to the same version.
	dependencies.observe(rule)
	if rule.Status != "active" || !rule.AllowsPostings {
		return db.PostingAccountRule{}, ValidationError{Message: role.label + " is not active for postings"}
	}
	if rule.IsSystem {
		return db.PostingAccountRule{}, ValidationError{Message: role.label + " cannot be a system account"}
	}
	if len(role.allowedClasses) > 0 && !slices.Contains(role.allowedClasses, rule.AccountClass) {
		return db.PostingAccountRule{}, ValidationError{Message: role.label + " has the wrong account class for this role"}
	}
	switch {
	case role.subledgerManaged && !isSubledgerManagedAccount(rule):
		return db.PostingAccountRule{}, ValidationError{Message: role.label + " must be a holding account managed by the investment subledger"}
	case !role.subledgerManaged && isSubledgerManagedAccount(rule):
		// The exemption this command carries would otherwise let the posting
		// through, and nothing downstream would write a lot for it.
		return db.PostingAccountRule{}, ValidationError{Message: role.label + " cannot be a holding account managed by the investment subledger"}
	}
	return rule, nil
}

// requireCommodityKind rejects a commodity that cannot play its part in the
// trade: the instrument being held is never a currency, and the leg that
// settles it always is. Without this a trade can name its security as its own
// settlement commodity, which balances perfectly and means nothing.
func (s *InvestmentService) requireCommodityKind(ctx context.Context, commodityID int64, date string, label string, wantCurrency bool) error {
	if commodityID <= 0 {
		return ValidationError{Message: label + " is required"}
	}
	rule, err := s.transactionService.repository.PostingCommodityRule(ctx, BookID, commodityID, date)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ValidationError{Message: label + " is invalid"}
		}
		return fmt.Errorf("read %s rule: %w", label, err)
	}
	if rule.Status != "active" {
		return ValidationError{Message: label + " is not active"}
	}
	isCurrency := rule.CommodityKind == "currency"
	if wantCurrency && !isCurrency {
		return ValidationError{Message: label + " must be a currency"}
	}
	if !wantCurrency && isCurrency {
		return ValidationError{Message: label + " must be an instrument, not a currency"}
	}
	return nil
}

// validateTradeRoles checks a buy, sell or write-off. A write-off has no cash
// leg at all, so it has no settlement role to check.
func (s *InvestmentService) validateTradeRoles(ctx context.Context, input InvestmentTradeInput, dependencies *accountRuleDependencies) error {
	date := input.TransactionDate
	if _, err := s.accountInRole(ctx, input.HoldingAccountID, date, holdingRole, dependencies); err != nil {
		return err
	}
	if err := s.requireCommodityKind(ctx, input.CommodityID, date, "traded commodity", false); err != nil {
		return err
	}
	if input.WriteOff {
		return nil
	}
	if _, err := s.accountInRole(ctx, input.CashAccountID, date, settlementRole, dependencies); err != nil {
		return err
	}
	return s.requireCommodityKind(ctx, input.CashCommodityID, date, "settlement commodity", true)
}

// validateDividendRoles checks a cash dividend. It writes no lots, so it names
// no holding account — and must not be able to reach one.
func (s *InvestmentService) validateDividendRoles(ctx context.Context, input DividendInput, date string, incomeAccountID int64, dependencies *accountRuleDependencies) error {
	if _, err := s.accountInRole(ctx, input.CashAccountID, date, settlementRole, dependencies); err != nil {
		return err
	}
	if _, err := s.accountInRole(ctx, incomeAccountID, date, incomeRole, dependencies); err != nil {
		return err
	}
	if input.WithholdingValue != nil && *input.WithholdingValue > 0 {
		withholdingAccountID := int64(0)
		if input.WithholdingAccountID != nil {
			withholdingAccountID = *input.WithholdingAccountID
		}
		if _, err := s.accountInRole(ctx, withholdingAccountID, date, withholdingRole, dependencies); err != nil {
			return err
		}
	}
	if input.CommodityID != nil {
		if err := s.requireCommodityKind(ctx, *input.CommodityID, date, "traded commodity", false); err != nil {
			return err
		}
	}
	return s.requireCommodityKind(ctx, input.CashCommodityID, date, "settlement commodity", true)
}

// validateReinvestedDividendRoles checks a reinvestment, which acquires shares
// against income instead of against cash: a holding account and an income
// account, and no settlement account at all.
func (s *InvestmentService) validateReinvestedDividendRoles(ctx context.Context, input ReinvestedDividendInput, date string, incomeAccountID int64, dependencies *accountRuleDependencies) error {
	if _, err := s.accountInRole(ctx, input.HoldingAccountID, date, holdingRole, dependencies); err != nil {
		return err
	}
	if _, err := s.accountInRole(ctx, incomeAccountID, date, incomeRole, dependencies); err != nil {
		return err
	}
	if err := s.requireCommodityKind(ctx, input.CommodityID, date, "traded commodity", false); err != nil {
		return err
	}
	return s.requireCommodityKind(ctx, input.CashCommodityID, date, "settlement commodity", true)
}
