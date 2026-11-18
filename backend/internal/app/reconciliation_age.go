package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"rekenraam/backend/internal/db"
)

// Reconciliation out-of-date rule (#482).
//
// One book setting says after how many days an account's reconciliation is
// out of date. The account list, Home's Reconciliation widget and any other
// place that judges reconciliation age read AccountReconciliationAges, so
// changing the setting changes all of them. Widgets add no rule of their own.

const (
	DefaultReconciliationOutOfDateAfterDays = 60
	MaxReconciliationOutOfDateAfterDays     = 3650
)

// Reconciliation age states. Never reconciled and out of date are distinct,
// and an account without posted activity is never called out of date.
const (
	ReconciliationAgeReconciled         = "reconciled"
	ReconciliationAgeOutOfDate          = "out_of_date"
	ReconciliationAgeNeverReconciled    = "never_reconciled"
	ReconciliationAgeNothingToReconcile = "nothing_to_reconcile"
)

// ReconciliationPolicy is the book's window. UpdatedAt is empty while the book
// uses the default it never saved.
type ReconciliationPolicy struct {
	OutOfDateAfterDays int
	UpdatedAt          string
}

type SaveReconciliationPolicyInput struct {
	OwnerUserID        int64
	AuthSessionID      int64
	RequestID          string
	OutOfDateAfterDays int
}

// ReconciliationAges judges every account that can be reconciled on
// owner-local Today against the window. Accounts are weakest first: never
// reconciled, out of date, reconciled (each oldest first), then those with
// nothing to reconcile.
type ReconciliationAges struct {
	Today              string
	OutOfDateAfterDays int
	Accounts           []AccountReconciliationAge
}

// AccountReconciliationAge is one account's judgement. An account in several
// commodities is judged per commodity (a checkpoint covers one), and the
// account takes its weakest commodity's state: never reconciled, then out of
// date, then reconciled. One reconciled currency never vouches for another.
type AccountReconciliationAge struct {
	AccountID    int64
	Name         string
	Code         string
	StarterKey   string
	CurrencyCode string
	State        string
	// LastStatementDate is the oldest statement date among the commodities
	// in State (how far the weakest coverage reaches); empty for never
	// reconciled and nothing to reconcile.
	LastStatementDate string
	// Commodities are the per-commodity judgements, weakest first.
	Commodities []CommodityReconciliationAge
}

type CommodityReconciliationAge struct {
	CommodityID       int64
	CommodityCode     string
	State             string
	LastStatementDate string
}

// ByAccount indexes the judgements; an account that cannot be reconciled is
// absent.
func (a ReconciliationAges) ByAccount() map[int64]AccountReconciliationAge {
	result := make(map[int64]AccountReconciliationAge, len(a.Accounts))
	for _, account := range a.Accounts {
		result[account.AccountID] = account
	}
	return result
}

func (s *TransactionService) ReconciliationPolicy(ctx context.Context) (ReconciliationPolicy, error) {
	record, err := s.repository.GetReconciliationPolicy(ctx, BookID)
	if errors.Is(err, db.ErrNotFound) {
		return ReconciliationPolicy{OutOfDateAfterDays: DefaultReconciliationOutOfDateAfterDays}, nil
	}
	if err != nil {
		return ReconciliationPolicy{}, err
	}
	return ReconciliationPolicy{OutOfDateAfterDays: record.OutOfDateAfterDays, UpdatedAt: record.UpdatedAt}, nil
}

func (s *TransactionService) SaveReconciliationPolicy(ctx context.Context, input SaveReconciliationPolicyInput) (ReconciliationPolicy, error) {
	if input.OwnerUserID <= 0 {
		return ReconciliationPolicy{}, ValidationError{Message: "owner user is required"}
	}
	if input.OutOfDateAfterDays < 1 || input.OutOfDateAfterDays > MaxReconciliationOutOfDateAfterDays {
		return ReconciliationPolicy{}, ValidationError{Message: fmt.Sprintf("out_of_date_after_days must be between 1 and %d", MaxReconciliationOutOfDateAfterDays)}
	}
	record, err := s.repository.SaveReconciliationPolicy(ctx, db.SaveReconciliationPolicyParams{
		BookID:             BookID,
		OutOfDateAfterDays: input.OutOfDateAfterDays,
		ActorUserID:        input.OwnerUserID,
		AuthSessionID:      input.AuthSessionID,
		RequestID:          input.RequestID,
		OriginType:         "browser_api",
		Operation:          "reconciliation.policy.save",
		RecordedAt:         s.now().UTC().Format(time.RFC3339),
		ChangeReason:       "saved reconciliation policy",
	})
	if err != nil {
		return ReconciliationPolicy{}, err
	}
	return ReconciliationPolicy{OutOfDateAfterDays: record.OutOfDateAfterDays, UpdatedAt: record.UpdatedAt}, nil
}

// AccountReconciliationAges is the one judgement of every reconcilable
// account's reconciliation age, on the owner's local today.
func (s *TransactionService) AccountReconciliationAges(ctx context.Context, ownerUserID int64) (ReconciliationAges, error) {
	today, err := s.settings.OwnerTodayAt(ctx, ownerUserID, s.now())
	if err != nil {
		return ReconciliationAges{}, err
	}
	policy, err := s.ReconciliationPolicy(ctx)
	if err != nil {
		return ReconciliationAges{}, err
	}
	facts, err := s.repository.ReconciliationAccountFacts(ctx, BookID, today)
	if err != nil {
		return ReconciliationAges{}, err
	}
	return judgeReconciliationAges(facts, today, policy.OutOfDateAfterDays)
}

func reconciliationAgeRank(state string) int {
	switch state {
	case ReconciliationAgeNeverReconciled:
		return 0
	case ReconciliationAgeOutOfDate:
		return 1
	case ReconciliationAgeReconciled:
		return 2
	default:
		return 3
	}
}

func compareReconciliationAge(aState, aDate string, aID int64, bState, bDate string, bID int64) int {
	return cmp.Or(cmp.Compare(reconciliationAgeRank(aState), reconciliationAgeRank(bState)), cmp.Compare(aDate, bDate), cmp.Compare(aID, bID))
}

func judgeReconciliationAges(facts []db.ReconciliationAccountFact, today string, window int) (ReconciliationAges, error) {
	result := ReconciliationAges{Today: today, OutOfDateAfterDays: window, Accounts: make([]AccountReconciliationAge, 0, len(facts))}
	for _, fact := range facts {
		age := AccountReconciliationAge{AccountID: fact.AccountID, Name: fact.Name, Code: fact.Code, StarterKey: fact.StarterKey, CurrencyCode: fact.CurrencyCode,
			State: ReconciliationAgeNothingToReconcile, Commodities: make([]CommodityReconciliationAge, 0, len(fact.Commodities))}
		for _, commodity := range fact.Commodities {
			state, err := reconciliationAgeState(commodity, today, window)
			if err != nil {
				return ReconciliationAges{}, err
			}
			age.Commodities = append(age.Commodities, CommodityReconciliationAge{CommodityID: commodity.CommodityID, CommodityCode: commodity.CommodityCode, State: state, LastStatementDate: commodity.LatestStatementDate})
		}
		slices.SortStableFunc(age.Commodities, func(a, b CommodityReconciliationAge) int {
			return compareReconciliationAge(a.State, a.LastStatementDate, a.CommodityID, b.State, b.LastStatementDate, b.CommodityID)
		})
		if len(age.Commodities) > 0 {
			// Weakest first, and within one state the oldest date first.
			age.State, age.LastStatementDate = age.Commodities[0].State, age.Commodities[0].LastStatementDate
		}
		result.Accounts = append(result.Accounts, age)
	}
	slices.SortStableFunc(result.Accounts, func(a, b AccountReconciliationAge) int {
		return compareReconciliationAge(a.State, a.LastStatementDate, a.AccountID, b.State, b.LastStatementDate, b.AccountID)
	})
	return result, nil
}

// reconciliationAgeState judges one commodity of an account. A checkpoint
// whose statement date is exactly window days before today is still within
// the window; one day earlier is out of date. The statement date, not the day
// the owner finished reconciling, is what the reconciliation vouches for, and
// the facts read already ignores statements dated after today.
func reconciliationAgeState(fact db.ReconciliationCommodityFact, today string, window int) (string, error) {
	if fact.LatestStatementDate == "" {
		if fact.HasPostedPostings {
			return ReconciliationAgeNeverReconciled, nil
		}
		return ReconciliationAgeNothingToReconcile, nil
	}
	day, err := time.Parse(time.DateOnly, today)
	if err != nil {
		return "", fmt.Errorf("parse reconciliation today: %w", err)
	}
	oldest := day.AddDate(0, 0, -window).Format(time.DateOnly)
	if fact.LatestStatementDate >= oldest {
		return ReconciliationAgeReconciled, nil
	}
	return ReconciliationAgeOutOfDate, nil
}
