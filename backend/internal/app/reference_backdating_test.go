package app

import (
	"context"
	"database/sql"
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
			v, err := f.institutions.UpdateInstitution(ctx, UpdateInstitutionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "institution.update", InstitutionID: item.ID, Name: name, Kind: "bank", EffectiveFrom: date})
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
			assert.Equal(t, "active", status)
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
			assert.Equal(t, "archived", status)
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
			before := f.counts(t)
			_, err := f.rename("Before existence", "2025-01-01")
			require.Error(t, err)
			assert.Equal(t, before, f.counts(t))
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
	_, err = f.categories.UpdateCategory(ctx, UpdateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.update", CategoryID: f.id, ParentCategoryID: &parent.ID, EffectiveFrom: "2026-01-01"})
	require.NoError(t, err)
	noPostings := false
	_, err = f.categories.UpdateCategory(ctx, UpdateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.update", CategoryID: f.id, ClearParent: true, AllowsPostings: &noPostings, EffectiveFrom: "2026-06-01"})
	require.NoError(t, err)
	_, err = f.categories.DisableCategory(ctx, CategoryLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", CategoryID: parent.ID, EffectiveFrom: "2026-06-01"})
	require.NoError(t, err)
	_, err = f.rename("Historical rename", "2026-03-01")
	require.NoError(t, err, "omitted structure and parent validity resolve on the correction date")
	v, err := db.NewCategoryRepository(f.database).CategoryByID(db.WithReferenceDate(ctx, "2026-04-01"), BookID, f.id)
	require.NoError(t, err)
	assert.Equal(t, sql.NullInt64{Int64: parent.ID, Valid: true}, v.ParentAccountID)
	assert.True(t, v.AllowsPostings)
	current, err := f.categories.Category(ctx, f.id)
	require.NoError(t, err)
	assert.Nil(t, current.ParentCategoryID)
	assert.False(t, current.AllowsPostings)
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
