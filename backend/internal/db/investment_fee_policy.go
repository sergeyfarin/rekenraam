package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type ResolvedInvestmentFeePolicy struct {
	PolicyID        int64
	VersionID       int64
	ResolutionTier  string
	Treatment       string
	ChargeAccountID sql.NullInt64
}

// ResolveInvestmentFeePolicy reads the dated account override first, then
// the book default. A missing policy is left to the application fallback.
func (r *InvestmentRepository) ResolveInvestmentFeePolicy(ctx context.Context, bookID, accountID int64, kind, date string) (ResolvedInvestmentFeePolicy, bool, error) {
	var policy ResolvedInvestmentFeePolicy
	err := r.database.QueryRowContext(ctx, `
		SELECT p.id, v.id,
			CASE WHEN p.account_id IS NULL THEN 'global' ELSE 'account' END,
			v.treatment, v.charge_account_id
		FROM investment_fee_policies p
		JOIN investment_fee_policy_versions v ON v.policy_id = p.id
		WHERE p.book_id = ? AND p.charge_kind = ?
			AND (p.account_id = ? OR p.account_id IS NULL)
			AND v.effective_from <= ?
		ORDER BY (p.account_id IS NOT NULL) DESC, v.effective_from DESC, v.version_seq DESC
		LIMIT 1
	`, bookID, kind, accountID, date).Scan(
		&policy.PolicyID, &policy.VersionID, &policy.ResolutionTier,
		&policy.Treatment, &policy.ChargeAccountID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ResolvedInvestmentFeePolicy{}, false, nil
	}
	if err != nil {
		return ResolvedInvestmentFeePolicy{}, false, fmt.Errorf("resolve investment fee policy: %w", err)
	}
	return policy, true, nil
}
