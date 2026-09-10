package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

var ErrBudgetSubjectNotFound = errors.New("budget subject not found")

type BudgetAmount struct {
	CommodityID   int64
	QuantityValue exact.Coefficient
	QuantityScale int
}
type BudgetCategoryRow struct {
	ID                       int64
	Code, Name, CategoryType string
	AllowsPostings           bool
	Amounts                  []BudgetCategoryAmount
}
type BudgetCategoryAmount struct {
	CommodityID               int64
	Target, Actual, Remaining BudgetAmount
}
type BudgetTypeTotal struct {
	CategoryType string
	Amount       BudgetCategoryAmount
}
type BudgetAccount struct {
	ID                                               int64
	Code, Name, AccountClass, AccountKind, Treatment string
}
type BudgetCommodity struct {
	ID           int64
	Code, Symbol string
	Scale        int
}
type BudgetMonth struct {
	PeriodStart, PeriodEnd string
	Categories             []BudgetCategoryRow
	Totals                 []BudgetTypeTotal
	Accounts               []BudgetAccount
	Commodities            []BudgetCommodity
	RolloverPolicy         string
}

type BudgetMutationContext struct {
	OwnerUserID, AuthSessionID    int64
	RequestID, OriginType, Reason string
}
type SetBudgetTargetInput struct {
	BudgetMutationContext
	CategoryID, CommodityID int64
	PeriodStart             string
	QuantityValue           exact.Coefficient
	QuantityScale           int
}
type SetBudgetTreatmentInput struct {
	BudgetMutationContext
	AccountID                int64
	EffectiveFrom, Treatment string
}

type BudgetService struct {
	repository *db.BudgetRepository
	settings   *SettingsService
	now        func() time.Time
}

func NewBudgetService(r *db.BudgetRepository, settings *SettingsService) *BudgetService {
	return &BudgetService{repository: r, settings: settings, now: time.Now}
}

func (s *BudgetService) SetNowForTest(now func() time.Time) { s.now = now }

func (s *BudgetService) CurrentMonth(ctx context.Context, userID int64) (BudgetMonth, error) {
	if s.settings == nil {
		return BudgetMonth{}, errors.New("budget settings service is required")
	}
	preferences, err := s.settings.Preferences(ctx, userID)
	if err != nil {
		return BudgetMonth{}, fmt.Errorf("read budget time zone: %w", err)
	}
	location, err := time.LoadLocation(preferences.TimeZone)
	if err != nil {
		return BudgetMonth{}, fmt.Errorf("load budget time zone: %w", err)
	}
	return s.Month(ctx, s.now().In(location).Format("2006-01")+"-01")
}

func budgetPeriod(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	t, err := time.Parse("2006-01-02", raw)
	if err != nil || t.Format("2006-01-02") != raw || t.Day() != 1 {
		return "", "", ValidationError{Message: "period_start must be the first day of a month"}
	}
	return raw, t.AddDate(0, 1, -1).Format("2006-01-02"), nil
}
func validDate(raw, field string) (string, error) {
	raw = strings.TrimSpace(raw)
	t, err := time.Parse("2006-01-02", raw)
	if err != nil || t.Format("2006-01-02") != raw {
		return "", ValidationError{Message: field + " is invalid"}
	}
	return raw, nil
}

func (s *BudgetService) Month(ctx context.Context, periodStart string) (BudgetMonth, error) {
	start, end, err := budgetPeriod(periodStart)
	if err != nil {
		return BudgetMonth{}, err
	}
	rec, err := s.repository.Snapshot(ctx, BookID, start, end)
	if err != nil {
		return BudgetMonth{}, fmt.Errorf("read budget month: %w", err)
	}
	if len(rec.Categories) > 5000 || len(rec.Accounts) > 5000 {
		return BudgetMonth{}, ValidationError{Message: "budget is too large"}
	}
	type key struct{ c, m int64 }
	targets := map[key]*exact.ScaledInt{}
	actuals := map[key]*exact.ScaledInt{}
	for _, v := range rec.Targets {
		c, e := exact.Parse(v.QuantityValue)
		if e != nil {
			return BudgetMonth{}, e
		}
		targets[key{v.CategoryID, v.CommodityID}] = exact.ScaledIntFromCoefficient(c, v.QuantityScale)
	}
	categoryType := map[int64]string{}
	for _, v := range rec.Categories {
		categoryType[v.ID] = v.CategoryType
	}
	for _, v := range rec.Actuals {
		c, e := exact.Parse(v.QuantityValue)
		if e != nil {
			return BudgetMonth{}, e
		}
		if categoryType[v.CategoryID] == "income" {
			c = c.Negated()
		}
		k := key{v.CategoryID, v.CommodityID}
		if actuals[k] == nil {
			actuals[k] = exact.NewScaledInt()
		}
		actuals[k].AddCoefficient(c, v.QuantityScale)
	}
	amountFor := func(k key) (BudgetCategoryAmount, error) {
		t := targets[k]
		if t == nil {
			t = exact.NewScaledInt()
		}
		a := actuals[k]
		if a == nil {
			a = exact.NewScaledInt()
		}
		rem := exact.ScaledIntFromBig(t.BigInt(), t.Scale())
		rem.SubScaled(a)
		tc, e := t.Coefficient()
		if e != nil {
			return BudgetCategoryAmount{}, LedgerOverflowError{CommodityID: k.m}
		}
		ac, e := a.Coefficient()
		if e != nil {
			return BudgetCategoryAmount{}, LedgerOverflowError{CommodityID: k.m}
		}
		rc, e := rem.Coefficient()
		if e != nil {
			return BudgetCategoryAmount{}, LedgerOverflowError{CommodityID: k.m}
		}
		return BudgetCategoryAmount{CommodityID: k.m, Target: BudgetAmount{k.m, tc, t.Scale()}, Actual: BudgetAmount{k.m, ac, a.Scale()}, Remaining: BudgetAmount{k.m, rc, rem.Scale()}}, nil
	}
	result := BudgetMonth{PeriodStart: start, PeriodEnd: end, RolloverPolicy: "none"}
	for _, v := range rec.Categories {
		row := BudgetCategoryRow{ID: v.ID, Code: v.Code, Name: v.Name, CategoryType: v.CategoryType, AllowsPostings: v.AllowsPostings}
		seen := map[int64]bool{}
		for k := range targets {
			if k.c == v.ID {
				seen[k.m] = true
			}
		}
		for k := range actuals {
			if k.c == v.ID {
				seen[k.m] = true
			}
		}
		ids := make([]int64, 0, len(seen))
		for id := range seen {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			a, e := amountFor(key{v.ID, id})
			if e != nil {
				return BudgetMonth{}, e
			}
			row.Amounts = append(row.Amounts, a)
		}
		result.Categories = append(result.Categories, row)
	}
	for _, v := range rec.Accounts {
		result.Accounts = append(result.Accounts, BudgetAccount(v))
	}
	for _, v := range rec.Commodities {
		result.Commodities = append(result.Commodities, BudgetCommodity(v))
	}
	for _, categoryKind := range []string{"income", "expense"} {
		for _, commodity := range rec.Commodities {
			tt, aa := exact.NewScaledInt(), exact.NewScaledInt()
			for k, v := range targets {
				if k.m == commodity.ID && categoryType[k.c] == categoryKind {
					tt.AddScaled(v)
				}
			}
			for k, v := range actuals {
				if k.m == commodity.ID && categoryType[k.c] == categoryKind {
					aa.AddScaled(v)
				}
			}
			rem := exact.ScaledIntFromBig(tt.BigInt(), tt.Scale())
			rem.SubScaled(aa)
			tc, e := tt.Coefficient()
			if e != nil {
				return BudgetMonth{}, LedgerOverflowError{CommodityID: commodity.ID}
			}
			ac, e := aa.Coefficient()
			if e != nil {
				return BudgetMonth{}, LedgerOverflowError{CommodityID: commodity.ID}
			}
			rc, e := rem.Coefficient()
			if e != nil {
				return BudgetMonth{}, LedgerOverflowError{CommodityID: commodity.ID}
			}
			result.Totals = append(result.Totals, BudgetTypeTotal{CategoryType: categoryKind, Amount: BudgetCategoryAmount{CommodityID: commodity.ID, Target: BudgetAmount{commodity.ID, tc, tt.Scale()}, Actual: BudgetAmount{commodity.ID, ac, aa.Scale()}, Remaining: BudgetAmount{commodity.ID, rc, rem.Scale()}}})
		}
	}
	return result, nil
}

func cleanBudgetMutation(m BudgetMutationContext) error {
	if m.OwnerUserID <= 0 {
		return ValidationError{Message: "owner user is required"}
	}
	if strings.TrimSpace(m.Reason) == "" {
		return ValidationError{Message: "change_reason is required"}
	}
	return nil
}
func (s *BudgetService) SetTarget(ctx context.Context, in SetBudgetTargetInput) (BudgetMonth, error) {
	if err := cleanBudgetMutation(in.BudgetMutationContext); err != nil {
		return BudgetMonth{}, err
	}
	start, _, err := budgetPeriod(in.PeriodStart)
	if err != nil {
		return BudgetMonth{}, err
	}
	if in.CategoryID <= 0 || in.CommodityID <= 0 {
		return BudgetMonth{}, ValidationError{Message: "category and commodity are required"}
	}
	if in.QuantityScale < 0 || in.QuantityScale > 12 {
		return BudgetMonth{}, ValidationError{Message: "quantity_scale is invalid"}
	}
	c, err := exact.Parse(in.QuantityValue.String())
	if err != nil || c.Sign() < 0 {
		return BudgetMonth{}, ValidationError{Message: "quantity_value must be a non-negative integer coefficient"}
	}
	op := "budget.target.set"
	if c.Sign() == 0 {
		op = "budget.target.remove"
	}
	err = s.repository.SetTarget(ctx, db.SetBudgetTargetParams{BookID: BookID, CategoryID: in.CategoryID, CommodityID: in.CommodityID, UserID: in.OwnerUserID, AuthSessionID: in.AuthSessionID, PeriodStart: start, Value: c.String(), Scale: in.QuantityScale, RequestID: in.RequestID, OriginType: in.OriginType, Operation: op, Reason: strings.TrimSpace(in.Reason), Now: s.now().UTC().Truncate(time.Second).Format(time.RFC3339)})
	if errors.Is(err, db.ErrNotFound) {
		return BudgetMonth{}, ErrBudgetSubjectNotFound
	}
	if err != nil {
		return BudgetMonth{}, err
	}
	return s.Month(ctx, start)
}
func (s *BudgetService) SetTreatment(ctx context.Context, in SetBudgetTreatmentInput) (BudgetMonth, error) {
	if err := cleanBudgetMutation(in.BudgetMutationContext); err != nil {
		return BudgetMonth{}, err
	}
	date, err := validDate(in.EffectiveFrom, "effective_from")
	if err != nil {
		return BudgetMonth{}, err
	}
	if in.AccountID <= 0 {
		return BudgetMonth{}, ValidationError{Message: "account is required"}
	}
	if in.Treatment != "on_budget" && in.Treatment != "off_budget" && in.Treatment != "excluded" {
		return BudgetMonth{}, ValidationError{Message: "treatment is invalid"}
	}
	err = s.repository.SetTreatment(ctx, db.SetBudgetTreatmentParams{BookID: BookID, AccountID: in.AccountID, UserID: in.OwnerUserID, AuthSessionID: in.AuthSessionID, EffectiveFrom: date, Treatment: in.Treatment, RequestID: in.RequestID, OriginType: in.OriginType, Operation: "budget.account_treatment.set", Reason: strings.TrimSpace(in.Reason), Now: s.now().UTC().Truncate(time.Second).Format(time.RFC3339)})
	if errors.Is(err, db.ErrNotFound) {
		return BudgetMonth{}, ErrBudgetSubjectNotFound
	}
	if err != nil {
		return BudgetMonth{}, err
	}
	start := date[:8] + "01"
	return s.Month(ctx, start)
}
