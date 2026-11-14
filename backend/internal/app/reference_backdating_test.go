package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
)

// Exercise the real services and read the resulting historical window through
// the shared dated selector, rather than only checking the mutation response.
type referenceBackdatingFixture struct {
	referenceVersionFixture
	id         int64
	table, key string
	rename     func(string, string) (string, error)
	lifecycle  func(string, bool) (string, error)
	read       func(string) (string, error)
}

func newReferenceBackdatingFixture(t *testing.T, kind string) referenceBackdatingFixture {
	t.Helper()
	f := newReferenceVersionFixture(t)
	ctx := context.Background()
	b := referenceBackdatingFixture{referenceVersionFixture: f}
	switch kind {
	case "category":
		item, err := f.categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Original", CategoryType: "expense"})
		require.NoError(t, err)
		b.id, b.table, b.key = item.ID, "account_versions", "account_id"
		b.rename = func(name, date string) (string, error) {
			v, err := f.categories.UpdateCategory(ctx, UpdateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.update", CategoryID: item.ID, Name: &name, EffectiveFrom: date})
			return v.Status, err
		}
		b.lifecycle = func(date string, archive bool) (string, error) {
			input := CategoryLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", CategoryID: item.ID, EffectiveFrom: date}
			if archive {
				v, err := f.categories.DisableCategory(ctx, input)
				return v.Status, err
			}
			v, err := f.categories.RestoreCategory(ctx, input)
			return v.Status, err
		}
		b.read = func(date string) (string, error) {
			v, err := db.NewCategoryRepository(f.database).CategoryByID(db.WithReferenceDate(ctx, date), BookID, item.ID)
			return v.Status, err
		}
	case "payee":
		item, err := f.payees.CreatePayee(ctx, CreatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.create", Name: "Original", EffectiveFrom: "2026-01-01"})
		require.NoError(t, err)
		b.id, b.table, b.key = item.ID, "payee_versions", "payee_id"
		b.rename = func(name, date string) (string, error) {
			v, err := f.payees.UpdatePayee(ctx, UpdatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.update", PayeeID: item.ID, Name: &name, EffectiveFrom: date})
			return v.Status, err
		}
		b.lifecycle = func(date string, archive bool) (string, error) {
			input := PayeeLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", PayeeID: item.ID, EffectiveFrom: date}
			if archive {
				v, err := f.payees.ArchivePayee(ctx, input)
				return v.Status, err
			}
			v, err := f.payees.RestorePayee(ctx, input)
			return v.Status, err
		}
		b.read = func(date string) (string, error) {
			v, err := db.NewPayeeRepository(f.database).PayeeByID(db.WithReferenceDate(ctx, date), BookID, item.ID)
			return v.Status, err
		}
	case "institution":
		item, err := f.institutions.CreateInstitution(ctx, CreateInstitutionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "institution.create", Name: "Original", Kind: "bank", EffectiveFrom: "2026-01-01"})
		require.NoError(t, err)
		b.id, b.table, b.key = item.ID, "institution_versions", "institution_id"
		b.rename = func(name, date string) (string, error) {
			v, err := f.institutions.UpdateInstitution(ctx, UpdateInstitutionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "institution.update", InstitutionID: item.ID, Name: new(name), EffectiveFrom: date})
			return v.Status, err
		}
		b.lifecycle = func(date string, archive bool) (string, error) {
			input := InstitutionLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", InstitutionID: item.ID, EffectiveFrom: date}
			if archive {
				v, err := f.institutions.ArchiveInstitution(ctx, input)
				return v.Status, err
			}
			v, err := f.institutions.RestoreInstitution(ctx, input)
			return v.Status, err
		}
		b.read = func(date string) (string, error) {
			v, err := db.NewInstitutionRepository(f.database).CurrentInstitutionByID(db.WithReferenceDate(ctx, date), BookID, item.ID)
			return v.Status, err
		}
	default:
		t.Fatalf("unknown reference kind %q", kind)
	}
	return b
}

func (f referenceBackdatingFixture) counts(t *testing.T) [2]int {
	t.Helper()
	var counts [2]int
	require.NoError(t, f.database.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", f.table, f.key), f.id).Scan(&counts[0]))
	require.NoError(t, f.database.QueryRow("SELECT COUNT(*) FROM audit_events").Scan(&counts[1]))
	return counts
}

func TestReferenceBackdatedDescriptionPreservesHistoricalLifecycle(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"category", "payee", "institution"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newReferenceBackdatingFixture(t, kind)
			_, err := f.lifecycle("2026-06-01", true)
			require.NoError(t, err)
			status, err := f.rename("March correction", "2026-03-01")
			require.NoError(t, err)
			assert.Equal(t, "archived", status, "the response is today's state, not the past window's")
			status, err = f.read("2026-04-01")
			require.NoError(t, err)
			assert.Equal(t, "active", status)
			status, err = f.read("2026-10-15")
			require.NoError(t, err)
			assert.Equal(t, "archived", status)

			_, err = f.lifecycle("2026-09-01", false)
			require.NoError(t, err)
			status, err = f.rename("July correction", "2026-07-01")
			require.NoError(t, err)
			assert.Equal(t, "active", status, "the response is today's state, not the past window's")
			status, err = f.read("2026-08-01")
			require.NoError(t, err)
			assert.Equal(t, "archived", status)
			status, err = f.read("2026-10-15")
			require.NoError(t, err)
			assert.Equal(t, "active", status)
			if kind == "category" {
				var closedOn sql.NullString
				require.NoError(t, f.database.QueryRow("SELECT closed_on FROM account_versions WHERE account_id = ? AND effective_from = '2026-07-01'", f.id).Scan(&closedOn))
				assert.Equal(t, sql.NullString{String: "2026-06-01", Valid: true}, closedOn)
			}
		})
	}
}

func TestReferenceBackdatedLifecycleRefusesWithoutWritesAndSameDateWorks(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"category", "payee", "institution"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newReferenceBackdatingFixture(t, kind)
			_, err := f.lifecycle("2026-06-01", true)
			require.NoError(t, err)
			for _, archive := range []bool{true, false} {
				before := f.counts(t)
				_, err := f.lifecycle("2026-03-01", archive)
				require.Error(t, err, "even a lifecycle request matching today's status must be refused")
				assert.Contains(t, err.Error(), "2026-06-01")
				assert.Equal(t, before, f.counts(t), "refusal writes neither a version nor an audit event")
			}
			before := f.counts(t)
			status, err := f.lifecycle("2026-06-01", false)
			require.NoError(t, err)
			assert.Equal(t, "active", status)
			assert.Equal(t, [2]int{before[0] + 1, before[1] + 1}, f.counts(t))
			status, err = f.read("2026-06-01")
			require.NoError(t, err)
			assert.Equal(t, "active", status, "same-date version_seq wins")
		})
	}
}

func TestReferenceCorrectionBeforeFirstVersionIsRefused(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"payee", "institution"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newReferenceBackdatingFixture(t, kind)
			// A later version must not stand in for the first one: the
			// refusal names the earliest date a correction may use.
			_, err := f.rename("June name", "2026-06-01")
			require.NoError(t, err)
			before := f.counts(t)
			_, err = f.rename("Before existence", "2025-01-01")
			require.ErrorIs(t, err, ErrReferenceVersionBeforeFirst)
			beforeFirst, ok := errors.AsType[ReferenceVersionBeforeFirstError](err)
			require.True(t, ok)
			assert.Equal(t, "2026-01-01", beforeFirst.FirstEffectiveFrom)
			assert.Equal(t, before, f.counts(t))
		})
	}
}

// A before-first edit is refused as such before any field filled from today's
// version is validated: today's default account being closed must not turn
// the coded refusal into a validation error.
func TestPayeeBeforeFirstRefusalPrecedesHintValidation(t *testing.T) {
	t.Parallel()
	f := newReferenceBackdatingFixture(t, "payee")
	ctx := context.Background()
	account, err := f.accountService.CreateAccount(ctx, CreateAccountInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.create", Name: "Closing", AccountClass: "asset", AccountKind: "checking", DefaultCommodityID: &f.eurCommodityID, OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01"})
	require.NoError(t, err)
	_, err = f.payees.UpdatePayee(ctx, UpdatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.update", PayeeID: f.id, DefaultAccountID: &account.ID, EffectiveFrom: "2026-02-01"})
	require.NoError(t, err)
	_, err = f.accountService.CloseAccount(ctx, AccountLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", AccountID: account.ID, EffectiveFrom: "2026-09-01", ClosedOn: "2026-09-01"})
	require.NoError(t, err)

	_, err = f.rename("Before existence", "2025-01-01")
	require.ErrorIs(t, err, ErrReferenceVersionBeforeFirst)
	var validation ValidationError
	assert.False(t, errors.As(err, &validation))
}

// The conflict names the other record as stored, not the requested spelling:
// uniqueness ignores case, so the two can differ.
func TestReferenceNameConflictNamesTheOtherRecord(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"category", "payee"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newReferenceBackdatingFixture(t, kind)
			ctx := context.Background()
			rule := "category_name_taken"
			if kind == "category" {
				_, err := f.categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "ACME", CategoryType: "expense"})
				require.NoError(t, err)
			} else {
				rule = "payee_name_taken"
				_, err := f.payees.CreatePayee(ctx, CreatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.create", Name: "ACME", EffectiveFrom: "2026-01-01"})
				require.NoError(t, err)
			}
			_, err := f.rename("Acme", "2026-08-01")
			requireReferenceConflict(t, err, rule, "2026-08-01", "ACME")
		})
	}
}

func TestCategoryBackdatedStructureIsRefused(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"parent", "posting eligibility"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			f := newReferenceBackdatingFixture(t, "category")
			_, err := f.rename("June name", "2026-06-01")
			require.NoError(t, err)
			input := UpdateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.update", CategoryID: f.id, EffectiveFrom: "2026-03-01"}
			if field == "parent" {
				parent, err := f.categories.CreateCategory(context.Background(), CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Parent", CategoryType: "expense"})
				require.NoError(t, err)
				input.ParentCategoryID = &parent.ID
			} else {
				value := false
				input.AllowsPostings = &value
			}
			before := f.counts(t)
			_, err = f.categories.UpdateCategory(context.Background(), input)
			require.Error(t, err)
			assert.Equal(t, before, f.counts(t))
			input.EffectiveFrom = "2026-06-01"
			_, err = f.categories.UpdateCategory(context.Background(), input)
			require.NoError(t, err)
		})
	}
}

func TestReferenceBackdatedNameUniquenessUsesTheEffectiveDate(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"category", "payee"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newReferenceBackdatingFixture(t, kind)
			ctx := context.Background()
			switch kind {
			case "category":
				v, err := f.categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Occupied in March", CategoryType: "expense"})
				require.NoError(t, err)
				updateCategoryVersion(t, f.referenceVersionFixture, v.ID, "Occupied today", "2026-06-01")
			case "payee":
				v, err := f.payees.CreatePayee(ctx, CreatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.create", Name: "Occupied in March", EffectiveFrom: "2026-01-01"})
				require.NoError(t, err)
				updatePayeeVersion(t, f.referenceVersionFixture, v.ID, "Occupied today", "2026-06-01")
			}
			_, err := f.rename("Original today", "2026-06-01")
			require.NoError(t, err)
			before := f.counts(t)
			_, err = f.rename("Occupied in March", "2026-03-01")
			require.Error(t, err, "a name available today was already occupied at the correction date")
			assert.Equal(t, before, f.counts(t))
			_, err = f.rename("Occupied today", "2026-03-01")
			require.NoError(t, err, "a name occupied today was available at the correction date")
		})
	}
}

func TestCategoryBackdatedPatchKeepsAsOfStructureAndParentValidation(t *testing.T) {
	t.Parallel()
	f := newReferenceBackdatingFixture(t, "category")
	ctx := context.Background()
	parent, err := f.categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Historical parent", CategoryType: "expense"})
	require.NoError(t, err)
	_, err = f.categories.UpdateCategory(ctx, UpdateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.update", CategoryID: f.id, ParentCategoryID: &parent.ID, Code: new("HIST-CODE"), Icon: new("basket"), EffectiveFrom: "2026-01-01"})
	require.NoError(t, err)
	noPostings := false
	_, err = f.categories.UpdateCategory(ctx, UpdateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.update", CategoryID: f.id, ClearParent: true, AllowsPostings: &noPostings, Code: new("TODAY-CODE"), Icon: new("car"), EffectiveFrom: "2026-06-01"})
	require.NoError(t, err)
	_, err = f.categories.DisableCategory(ctx, CategoryLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", CategoryID: parent.ID, EffectiveFrom: "2026-06-01"})
	require.NoError(t, err)
	_, err = f.rename("Historical rename", "2026-03-01")
	require.NoError(t, err, "omitted structure and parent validity resolve on the correction date")
	v, err := db.NewCategoryRepository(f.database).CategoryByID(db.WithReferenceDate(ctx, "2026-04-01"), BookID, f.id)
	require.NoError(t, err)
	assert.Equal(t, sql.NullInt64{Int64: parent.ID, Valid: true}, v.ParentAccountID)
	assert.True(t, v.AllowsPostings)
	// Code and icon were part of the original silent rewrite: omitted, they
	// keep the correction date's values, not today's.
	assert.Equal(t, sql.NullString{String: "HIST-CODE", Valid: true}, v.Code)
	assert.Equal(t, "basket", parseCategoryMetadata(v.MetadataJSON).Icon)
	assert.Equal(t, sql.NullString{String: "Historical rename", Valid: true}, v.Name)
	current, err := f.categories.Category(ctx, f.id)
	require.NoError(t, err)
	assert.Nil(t, current.ParentCategoryID)
	assert.False(t, current.AllowsPostings)
	assert.Equal(t, "TODAY-CODE", current.Code)
	assert.Equal(t, "car", current.Icon)
}

func TestPayeeBackdatedPatchKeepsAsOfDefaultsAndMetadata(t *testing.T) {
	t.Parallel()
	f := newReferenceBackdatingFixture(t, "payee")
	ctx := context.Background()
	metadata := `{"period":"old"}`
	_, err := f.payees.UpdatePayee(ctx, UpdatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.update", PayeeID: f.id, DefaultAccountID: &f.cashAccountID, MetadataJSON: &metadata, EffectiveFrom: "2026-01-01"})
	require.NoError(t, err)
	metadata = `{"period":"new"}`
	_, err = f.payees.UpdatePayee(ctx, UpdatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.update", PayeeID: f.id, ClearDefaultAccount: true, MetadataJSON: &metadata, EffectiveFrom: "2026-06-01"})
	require.NoError(t, err)
	_, err = f.rename("Historical payee", "2026-03-01")
	require.NoError(t, err)
	v, err := db.NewPayeeRepository(f.database).PayeeByID(db.WithReferenceDate(ctx, "2026-04-01"), BookID, f.id)
	require.NoError(t, err)
	assert.Equal(t, sql.NullInt64{Int64: f.cashAccountID, Valid: true}, v.DefaultAccountID)
	assert.JSONEq(t, `{"period":"old"}`, v.MetadataJSON)
}

// A forward edit dated in the past (no later version of its own) governs
// every day through today. Its name must be free on each of those days, not
// only on its own date (#552 review).
func TestReferenceForwardEditNameUniquenessCoversTheWholeWindow(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"category", "payee"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newReferenceBackdatingFixture(t, kind)
			ctx := context.Background()
			switch kind {
			case "category":
				v, err := f.categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Other", CategoryType: "expense"})
				require.NoError(t, err)
				updateCategoryVersion(t, f.referenceVersionFixture, v.ID, "Taken in September", "2026-09-01")
				updateCategoryVersion(t, f.referenceVersionFixture, v.ID, "Free again", "2026-10-01")
			case "payee":
				v, err := f.payees.CreatePayee(ctx, CreatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.create", Name: "Other", EffectiveFrom: "2026-01-01"})
				require.NoError(t, err)
				updatePayeeVersion(t, f.referenceVersionFixture, v.ID, "Taken in September", "2026-09-01")
				updatePayeeVersion(t, f.referenceVersionFixture, v.ID, "Free again", "2026-10-01")
			}
			rule := "category_name_taken"
			if kind == "payee" {
				rule = "payee_name_taken"
			}
			for name, date := range map[string]string{"Free again": "2026-10-01", "Taken in September": "2026-09-01"} {
				before := f.counts(t)
				_, err := f.rename(name, "2026-08-01")
				require.Error(t, err, "%q is taken on a day the August version governs", name)
				if kind == "category" {
					assert.ErrorIs(t, err, ErrCategoryExists)
				} else {
					assert.ErrorIs(t, err, ErrPayeeExists)
				}
				requireReferenceConflict(t, err, rule, date, name)
				assert.Equal(t, before, f.counts(t))
			}
			_, err := f.rename("Free again", "2026-06-01")
			require.Error(t, err, "dated earlier still reaches today")
			_, err = f.rename("Never taken", "2026-08-01")
			require.NoError(t, err)
		})
	}
}

func TestCategoryForwardEditParentMustStayActiveThroughToday(t *testing.T) {
	t.Parallel()
	f := newReferenceBackdatingFixture(t, "category")
	ctx := context.Background()
	parent, err := f.categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Parent", CategoryType: "expense"})
	require.NoError(t, err)
	_, err = f.categories.DisableCategory(ctx, CategoryLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", CategoryID: parent.ID, EffectiveFrom: "2026-09-01"})
	require.NoError(t, err)
	before := f.counts(t)
	_, err = f.categories.UpdateCategory(ctx, UpdateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.update", CategoryID: f.id, ParentCategoryID: &parent.ID, EffectiveFrom: "2026-08-01"})
	require.ErrorIs(t, err, ErrCategoryParentInvalid, "the parent is archived on days the August version governs")
	requireReferenceConflict(t, err, "category_parent_archived", "2026-09-01", "Parent")
	assert.Equal(t, before, f.counts(t))
	current, err := f.categories.Category(ctx, f.id)
	require.NoError(t, err)
	assert.Nil(t, current.ParentCategoryID)
}

func TestCategoryForwardEditRefusesADatedParentCycle(t *testing.T) {
	t.Parallel()
	f := newReferenceBackdatingFixture(t, "category")
	ctx := context.Background()
	other, err := f.categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Other", CategoryType: "expense"})
	require.NoError(t, err)
	_, err = f.categories.UpdateCategory(ctx, UpdateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.update", CategoryID: other.ID, ParentCategoryID: &f.id, EffectiveFrom: "2026-09-01"})
	require.NoError(t, err)
	before := f.counts(t)
	// On its own date the tree has no loop; from September it would.
	_, err = f.categories.UpdateCategory(ctx, UpdateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.update", CategoryID: f.id, ParentCategoryID: &other.ID, EffectiveFrom: "2026-08-01"})
	require.ErrorIs(t, err, ErrCategoryParentInvalid)
	requireReferenceConflict(t, err, "category_parent_cycle", "2026-09-01", "Other")
	assert.Equal(t, before, f.counts(t))
}

func TestReferenceLifecycleRulesCoverTheWholeWindow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("category disable with a child active in the window", func(t *testing.T) {
		t.Parallel()
		f := newReferenceBackdatingFixture(t, "category")
		child, err := f.categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Child", CategoryType: "expense", ParentCategoryID: &f.id})
		require.NoError(t, err)
		_, err = f.categories.DisableCategory(ctx, CategoryLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", CategoryID: child.ID, EffectiveFrom: "2026-09-01"})
		require.NoError(t, err)
		before := f.counts(t)
		_, err = f.lifecycle("2026-08-01", true)
		require.ErrorIs(t, err, ErrCategoryHasChildren, "the child is still active in August")
		requireReferenceConflict(t, err, "category_active_child", "2026-08-01", "Child")
		assert.Equal(t, before, f.counts(t))
		_, err = f.lifecycle("2026-09-01", true)
		require.NoError(t, err)
	})
	t.Run("category restore with a name taken in the window", func(t *testing.T) {
		t.Parallel()
		f := newReferenceBackdatingFixture(t, "category")
		_, err := f.lifecycle("2026-02-01", true)
		require.NoError(t, err)
		other, err := f.categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Other", CategoryType: "expense"})
		require.NoError(t, err)
		updateCategoryVersion(t, f.referenceVersionFixture, other.ID, "Original", "2026-08-01")
		updateCategoryVersion(t, f.referenceVersionFixture, other.ID, "Other again", "2026-09-01")
		_, err = f.lifecycle("2026-07-01", false)
		require.ErrorIs(t, err, ErrCategoryExists)
		requireReferenceConflict(t, err, "category_name_taken", "2026-08-01", "Original")
		_, err = f.lifecycle("2026-09-01", false)
		require.NoError(t, err)
	})
	t.Run("payee restore with a name taken in the window", func(t *testing.T) {
		t.Parallel()
		f := newReferenceBackdatingFixture(t, "payee")
		_, err := f.lifecycle("2026-02-01", true)
		require.NoError(t, err)
		other, err := f.payees.CreatePayee(ctx, CreatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.create", Name: "Other", EffectiveFrom: "2026-01-01"})
		require.NoError(t, err)
		updatePayeeVersion(t, f.referenceVersionFixture, other.ID, "Original", "2026-08-01")
		updatePayeeVersion(t, f.referenceVersionFixture, other.ID, "Other again", "2026-09-01")
		_, err = f.lifecycle("2026-07-01", false)
		require.ErrorIs(t, err, ErrPayeeExists)
		requireReferenceConflict(t, err, "payee_name_taken", "2026-08-01", "Original")
		_, err = f.lifecycle("2026-09-01", false)
		require.NoError(t, err)
	})
	t.Run("institution archive with an account linked in the window", func(t *testing.T) {
		t.Parallel()
		f := newReferenceBackdatingFixture(t, "institution")
		institutionID := f.id
		account, err := f.accountService.CreateAccount(ctx, CreateAccountInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.create", Name: "Linked", AccountClass: "asset", AccountKind: "checking", InstitutionID: &institutionID, DefaultCommodityID: &f.eurCommodityID, OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01"})
		require.NoError(t, err)
		allowsPostings := account.AllowsPostings
		_, err = f.accountService.UpdateAccount(ctx, UpdateAccountInput{
			OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.update", AccountID: account.ID,
			Code: account.Code, Name: account.Name, AccountClass: account.AccountClass, AccountKind: account.AccountKind,
			DefaultCommodityID: account.DefaultCommodityID, AllowsPostings: &allowsPostings, MetadataJSON: account.MetadataJSON,
			OpenedOn: account.OpenedOn, CostBasisMethod: account.CostBasisMethod, EffectiveFrom: "2026-09-01",
		})
		require.NoError(t, err)
		before := f.counts(t)
		_, err = f.lifecycle("2026-08-01", true)
		require.ErrorIs(t, err, ErrInstitutionInUse, "the account still names the institution in August")
		requireReferenceConflict(t, err, "institution_linked_account", "2026-08-01", "Linked")
		assert.Equal(t, before, f.counts(t))
		_, err = f.lifecycle("2026-09-01", true)
		require.NoError(t, err)
	})
}

func TestPayeeForwardEditDefaultHintsMustBeValidToday(t *testing.T) {
	t.Parallel()
	f := newReferenceBackdatingFixture(t, "payee")
	ctx := context.Background()
	account, err := f.accountService.CreateAccount(ctx, CreateAccountInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.create", Name: "Closing", AccountClass: "asset", AccountKind: "checking", DefaultCommodityID: &f.eurCommodityID, OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01"})
	require.NoError(t, err)
	_, err = f.accountService.CloseAccount(ctx, AccountLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", AccountID: account.ID, EffectiveFrom: "2026-09-01", ClosedOn: "2026-09-01"})
	require.NoError(t, err)
	before := f.counts(t)
	_, err = f.payees.UpdatePayee(ctx, UpdatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.update", PayeeID: f.id, DefaultAccountID: &account.ID, EffectiveFrom: "2026-08-01"})
	require.Error(t, err, "the August version is today's payee, and the account is closed today")
	assert.Equal(t, before, f.counts(t))

	// A correction bounded by a later version only governs its past window.
	_, err = f.rename("October name", "2026-10-01")
	require.NoError(t, err)
	_, err = f.payees.UpdatePayee(ctx, UpdatePayeeInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.update", PayeeID: f.id, DefaultAccountID: &account.ID, EffectiveFrom: "2026-08-01"})
	require.NoError(t, err)
}

// The service fills omitted fields from the version in effect on the
// requested date before the write transaction opens. If a version is added in
// between, the transaction refuses rather than writing a stale fill.
func TestReferenceWriteRefusesAStaleBaseVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("category", func(t *testing.T) {
		t.Parallel()
		f := newReferenceBackdatingFixture(t, "category")
		repository := db.NewCategoryRepository(f.database)
		base, err := repository.CategoryByID(db.WithReferenceDate(ctx, "2026-10-15"), BookID, f.id)
		require.NoError(t, err)
		_, err = f.rename("Concurrent", "2026-10-15")
		require.NoError(t, err)
		current := toCategory(base)
		spec, err := f.categories.newCategorySpec(ctx, categorySpecInput{CategoryID: f.id, Name: "Stale", CategoryType: current.CategoryType, AllowsPostings: &current.AllowsPostings, Current: &current})
		require.NoError(t, err)
		before := f.counts(t)
		_, err = repository.UpdateCategory(db.WithReferenceDate(ctx, "2026-10-15"), db.UpdateCategoryParams{BookID: BookID, CategoryID: f.id, ChangedByUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.update", Spec: spec, BaseVersionID: base.VersionID, ChangeReason: "stale", RecordedAt: "2026-10-15T12:00:00Z", EffectiveFrom: "2026-10-15"})
		require.ErrorIs(t, err, db.ErrReferenceVersionChanged)
		assert.ErrorIs(t, categoryRepositoryError("persist", err), ErrReferenceVersionChanged)
		assert.Equal(t, before, f.counts(t))
	})
	for _, kind := range []string{"payee", "institution"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newReferenceBackdatingFixture(t, kind)
			at := db.WithReferenceDate(ctx, "2026-10-15")
			var baseVersionID int64
			var write func() error
			switch kind {
			case "payee":
				repository := db.NewPayeeRepository(f.database)
				base, err := repository.PayeeByID(at, BookID, f.id)
				require.NoError(t, err)
				baseVersionID = base.VersionID
				write = func() error {
					_, err := repository.UpdatePayee(at, db.UpdatePayeeParams{BookID: BookID, PayeeID: f.id, ActorUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.archive", Spec: db.PayeeSpec{Name: base.Name, NormalizedName: base.NormalizedName, MetadataJSON: base.MetadataJSON}, Status: "archived", BaseVersionID: baseVersionID, RecordedAt: "2026-10-15T12:00:00Z", EffectiveFrom: "2026-10-15", ChangeReason: "stale"})
					assert.ErrorIs(t, mapPayeeDBError(err), ErrReferenceVersionChanged)
					return err
				}
			case "institution":
				repository := db.NewInstitutionRepository(f.database)
				base, err := repository.CurrentInstitutionByID(at, BookID, f.id)
				require.NoError(t, err)
				baseVersionID = base.VersionID
				write = func() error {
					_, err := repository.UpdateInstitution(at, db.UpdateInstitutionParams{BookID: BookID, InstitutionID: f.id, ChangedByUserID: f.ownerUserID, OriginType: "browser_api", Operation: "institution.archive", Spec: institutionSpecFromStored(toInstitution(base)), Status: "archived", BaseVersionID: baseVersionID, ChangeReason: "stale", RecordedAt: "2026-10-15T12:00:00Z", EffectiveFrom: "2026-10-15"})
					return err
				}
			}
			_, err := f.rename("Concurrent", "2026-10-15")
			require.NoError(t, err)
			before := f.counts(t)
			require.ErrorIs(t, write(), db.ErrReferenceVersionChanged)
			assert.Equal(t, before, f.counts(t))
		})
	}
}

// requireReferenceConflict asserts the dated refusal the client explains: the
// rule, the first date it fails, and the other entity's name.
func requireReferenceConflict(t *testing.T, err error, rule, date, name string) {
	t.Helper()
	conflict, ok := errors.AsType[ReferenceRuleConflictError](err)
	require.Truef(t, ok, "want a dated reference conflict, got %v", err)
	assert.Equal(t, rule, conflict.Rule)
	assert.Equal(t, date, conflict.Date)
	assert.Equal(t, name, conflict.Name)
}

// An unnamed built-in or starter category is identified by its key, which the client
// labels in its own locale.
func TestReferenceConflictIdentifiesABuiltinCategoryByKey(t *testing.T) {
	t.Parallel()
	f := newReferenceBackdatingFixture(t, "category")
	ctx := context.Background()
	// Seed the built-ins through the repository: the service call also
	// reloads setup status, which this fixture does not wire.
	_, err := db.NewCategoryRepository(f.database).EnsureCategories(db.WithReferenceDate(ctx, "2026-10-15"), db.EnsureCategoriesParams{
		BookID: BookID, ChangedByUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.ensure",
		Specs: builtinCategorySpecs(), CreatedAt: "2026-10-15T12:00:00Z", EffectiveFrom: categoryGenesisDate,
	})
	require.NoError(t, err)
	var builtin Category
	all, err := f.categories.ListCategories(ctx, ListCategoriesInput{CategoryType: "expense"})
	require.NoError(t, err)
	for _, category := range all {
		if category.BuiltinKey != "" && category.Name == "" {
			if _, err := f.categories.DisableCategory(ctx, CategoryLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", CategoryID: category.ID, EffectiveFrom: "2026-09-01"}); err == nil {
				builtin = category
				break
			}
		}
	}
	require.NotZero(t, builtin.ID, "an unnamed, childless built-in or starter category to disable")
	_, err = f.categories.UpdateCategory(ctx, UpdateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.update", CategoryID: f.id, ParentCategoryID: &builtin.ID, EffectiveFrom: "2026-08-01"})
	conflict, ok := errors.AsType[ReferenceRuleConflictError](err)
	require.Truef(t, ok, "want a dated reference conflict, got %v", err)
	assert.Equal(t, "category_parent_archived", conflict.Rule)
	assert.Equal(t, "2026-09-01", conflict.Date)
	assert.Empty(t, conflict.Name)
	assert.Equal(t, builtin.BuiltinKey, conflict.BuiltinKey)
}
