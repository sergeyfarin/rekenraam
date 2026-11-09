package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
)

type referenceVersionFixture struct {
	*investmentsTestFixture
	institutions *InstitutionService
	categories   *CategoryService
	payees       *PayeeService
}

func newReferenceVersionFixture(t *testing.T) referenceVersionFixture {
	t.Helper()
	f := newInvestmentsTestFixture(t)
	settings := NewSettingsService(db.NewSettingsRepository(f.database))
	now := func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
	settings.SetNowForTest(now)
	institutions := NewInstitutionService(db.NewInstitutionRepository(f.database))
	categories := NewCategoryService(db.NewCategoryRepository(f.database), nil)
	payees := NewPayeeService(db.NewPayeeRepository(f.database), db.NewAccountRepository(f.database))
	for _, service := range []interface {
		SetSettings(*SettingsService)
		SetNowForTest(func() time.Time)
	}{f.accountService, institutions, categories, payees, f.investmentService} {
		service.SetSettings(settings)
		service.SetNowForTest(now)
	}
	return referenceVersionFixture{investmentsTestFixture: f, institutions: institutions, categories: categories, payees: payees}
}

func requireLatestVersionSeq(t *testing.T, f referenceVersionFixture, table, column string, id, want int64) {
	t.Helper()
	var got int64
	require.NoError(t, f.database.QueryRow(
		fmt.Sprintf(`SELECT MAX(version_seq) FROM %s WHERE %s = ?`, table, column), id,
	).Scan(&got))
	assert.Equal(t, want, got, "%s numbering follows the newest row", table)
}

func updateAccountVersion(t *testing.T, f referenceVersionFixture, accountID int64, name, effectiveFrom string) Account {
	t.Helper()
	current, err := f.accountService.Account(context.Background(), accountID)
	require.NoError(t, err)
	allowsPostings := current.AllowsPostings
	updated, err := f.accountService.UpdateAccount(context.Background(), UpdateAccountInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.update", AccountID: accountID,
		Code: current.Code, Name: name, AccountClass: current.AccountClass, AccountKind: current.AccountKind,
		ParentAccountID: current.ParentAccountID, InstitutionID: current.InstitutionID,
		CountryCode: current.CountryCode, DefaultCommodityID: current.DefaultCommodityID,
		QuantityScaleOverride: current.QuantityScaleOverride, AllowsPostings: &allowsPostings,
		NumberLast4: current.NumberLast4, ExternalRefHint: current.ExternalRefHint,
		CommentMarkdown: current.CommentMarkdown, MetadataJSON: current.MetadataJSON,
		OpenedOn: current.OpenedOn, CostBasisMethod: current.CostBasisMethod,
		EffectiveFrom: effectiveFrom, ChangeReason: "version sequence regression",
	})
	require.NoError(t, err)
	return updated
}

func updateInstitutionVersion(t *testing.T, f referenceVersionFixture, institutionID int64, name, effectiveFrom string) Institution {
	t.Helper()
	current, err := f.institutions.Institution(context.Background(), institutionID)
	require.NoError(t, err)
	updated, err := f.institutions.UpdateInstitution(context.Background(), UpdateInstitutionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "institution.update",
		InstitutionID: institutionID, Name: name, Kind: current.Kind, CountryCode: current.CountryCode,
		Website: current.Website, AddressJSON: current.AddressJSON, CommentMarkdown: current.CommentMarkdown,
		MetadataJSON: current.MetadataJSON, EffectiveFrom: effectiveFrom, ChangeReason: "version sequence regression",
	})
	require.NoError(t, err)
	return updated
}

func updatePayeeVersion(t *testing.T, f referenceVersionFixture, payeeID int64, name, effectiveFrom string) Payee {
	t.Helper()
	updated, err := f.payees.UpdatePayee(context.Background(), UpdatePayeeInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.update",
		PayeeID: payeeID, Name: &name, EffectiveFrom: effectiveFrom, ChangeReason: "version sequence regression",
	})
	require.NoError(t, err)
	return updated
}

func updateCategoryVersion(t *testing.T, f referenceVersionFixture, categoryID int64, name, effectiveFrom string) Category {
	t.Helper()
	updated, err := f.categories.UpdateCategory(context.Background(), UpdateCategoryInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.update",
		CategoryID: categoryID, Name: &name, EffectiveFrom: effectiveFrom, ChangeReason: "version sequence regression",
	})
	require.NoError(t, err)
	return updated
}

func updateInstrumentVersion(t *testing.T, f referenceVersionFixture, instrumentID int64, name, effectiveFrom string) InvestmentInstrument {
	t.Helper()
	current, err := f.investmentService.Instrument(context.Background(), instrumentID)
	require.NoError(t, err)
	return updateInstrumentFields(t, f, instrumentID, name, current.QuoteCommodityID, current.IdentifiersJSON, current.MetadataJSON, effectiveFrom)
}

func updateInstrumentFields(t *testing.T, f referenceVersionFixture, instrumentID int64, name string, quoteCommodityID *int64, identifiersJSON, metadataJSON, effectiveFrom string) InvestmentInstrument {
	t.Helper()
	current, err := f.investmentService.Instrument(context.Background(), instrumentID)
	require.NoError(t, err)
	tradingCommodityID := current.TradingCommodityID
	if quoteCommodityID != nil {
		tradingCommodityID = quoteCommodityID
	}
	updated, err := f.investmentService.UpdateInstrument(context.Background(), InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.update",
		InstrumentID: instrumentID, CommodityCode: current.CommodityCode, InstrumentType: current.InstrumentType,
		DisplayName: name, Symbol: current.Symbol, ExchangeCode: current.ExchangeCode, MIC: current.MIC,
		Issuer: current.Issuer, CountryCode: current.CountryCode, QuoteCommodityID: quoteCommodityID,
		TradingCommodityID: tradingCommodityID, QuantityScale: current.QuantityScale, PriceScale: current.PriceScale,
		IdentifiersJSON: identifiersJSON, MetadataJSON: metadataJSON,
		EffectiveFrom: effectiveFrom, ChangeReason: "version sequence regression",
	})
	require.NoError(t, err)
	return updated
}

// #549: a backdated descriptive version must not make the next update reuse
// the version_seq of the version still in effect.
func TestReferenceVersionSequenceFollowsTheNewestRowAfterBackdating(t *testing.T) {
	t.Parallel()
	f := newReferenceVersionFixture(t)
	ctx := context.Background()

	account, err := f.accountService.CreateAccount(ctx, CreateAccountInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.create",
		Name: "Versioned account", AccountClass: "asset", AccountKind: "checking",
		DefaultCommodityID: &f.eurCommodityID, OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	updateAccountVersion(t, f, account.ID, "Account forward", "2026-06-01")
	updateAccountVersion(t, f, account.ID, "Account backdated", "2026-03-01")
	updateAccountVersion(t, f, account.ID, "Account final", "2026-07-01")
	requireLatestVersionSeq(t, f, "account_versions", "account_id", account.ID, 4)

	institution, err := f.institutions.CreateInstitution(ctx, CreateInstitutionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "institution.create",
		Name: "Versioned institution", Kind: "bank", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	updateInstitutionVersion(t, f, institution.ID, "Institution forward", "2026-06-01")
	updateInstitutionVersion(t, f, institution.ID, "Institution backdated", "2026-03-01")
	updateInstitutionVersion(t, f, institution.ID, "Institution final", "2026-07-01")
	requireLatestVersionSeq(t, f, "institution_versions", "institution_id", institution.ID, 4)

	payee, err := f.payees.CreatePayee(ctx, CreatePayeeInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.create",
		Name: "Versioned payee", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	updatePayeeVersion(t, f, payee.ID, "Payee forward", "2026-06-01")
	updatePayeeVersion(t, f, payee.ID, "Payee backdated", "2026-03-01")
	updatePayeeVersion(t, f, payee.ID, "Payee final", "2026-07-01")
	requireLatestVersionSeq(t, f, "payee_versions", "payee_id", payee.ID, 4)

	category, err := f.categories.CreateCategory(ctx, CreateCategoryInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create",
		Name: "Versioned category", CategoryType: "expense",
	})
	require.NoError(t, err)
	updateCategoryVersion(t, f, category.ID, "Category backdated", "2026-09-01")
	updateCategoryVersion(t, f, category.ID, "Category final", "2026-10-15")
	requireLatestVersionSeq(t, f, "account_versions", "account_id", category.ID, 3)

	statusCategory, err := f.categories.CreateCategory(ctx, CreateCategoryInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create",
		Name: "Versioned status category", CategoryType: "expense",
	})
	require.NoError(t, err)
	updateCategoryVersion(t, f, statusCategory.ID, "Status category backdated", "2026-09-01")
	_, err = f.categories.DisableCategory(ctx, CategoryLifecycleInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		CategoryID: statusCategory.ID, EffectiveFrom: "2026-10-15", ChangeReason: "version sequence regression",
	})
	require.NoError(t, err)
	requireLatestVersionSeq(t, f, "account_versions", "account_id", statusCategory.ID, 3)

	instrument, err := f.investmentService.CreateInstrument(ctx, InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.create",
		CommodityCode: "VERSIONED", InstrumentType: "stock", DisplayName: "Versioned instrument",
		QuantityScale: 6, PriceScale: 4,
	})
	require.NoError(t, err)
	updateInstrumentVersion(t, f, instrument.ID, "Instrument forward", "2026-06-01")
	updateInstrumentVersion(t, f, instrument.ID, "Instrument backdated", "2026-03-01")
	updateInstrumentVersion(t, f, instrument.ID, "Instrument final", "2026-07-01")
	requireLatestVersionSeq(t, f, "investment_instrument_versions", "instrument_id", instrument.ID, 4)
}

// #549 / #550 review: the structural comparison is against the version in
// effect on the new row's date, not today's version. A backdated descriptive
// correction uses the old structure; a backdated structural change is refused.
func TestInstrumentBackdatedStructureUsesTheVersionInEffectOnItsDate(t *testing.T) {
	t.Parallel()
	f := newReferenceVersionFixture(t)
	ctx := context.Background()
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	gbp := seedTestCurrencyCommodity(t, f.database, "GBP")

	instrument, err := f.investmentService.CreateInstrument(ctx, InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.create",
		CommodityCode: "ASOF", InstrumentType: "stock", DisplayName: "As-of instrument",
		QuoteCommodityID: &usd, TradingCommodityID: &usd, QuantityScale: 6, PriceScale: 4,
	})
	require.NoError(t, err)
	updateInstrumentFields(t, f, instrument.ID, "Forward GBP", &gbp, "", "", "2026-06-01")

	// March is governed by v1 (USD), so a descriptive correction carrying USD
	// is allowed and stays out of effect behind the June GBP version.
	updateInstrumentFields(t, f, instrument.ID, "March correction", &usd, "", "", "2026-03-01")

	var versionsBefore, auditsBefore int64
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_instrument_versions WHERE instrument_id = ?`, instrument.ID).Scan(&versionsBefore))
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsBefore))

	_, err = f.investmentService.UpdateInstrument(ctx, InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.update",
		InstrumentID: instrument.ID, CommodityCode: instrument.CommodityCode, InstrumentType: instrument.InstrumentType,
		DisplayName: "March structural change", QuoteCommodityID: &gbp, TradingCommodityID: &gbp,
		QuantityScale: instrument.QuantityScale, PriceScale: instrument.PriceScale,
		EffectiveFrom: "2026-03-01", ChangeReason: "backdated quote currency",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvestmentInstrumentVersionBackdated), "March is governed by USD, so GBP is structural: %v", err)
	var versionsAfter, auditsAfter int64
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_instrument_versions WHERE instrument_id = ?`, instrument.ID).Scan(&versionsAfter))
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditsAfter))
	assert.Equal(t, versionsBefore, versionsAfter, "the refusal writes no version")
	assert.Equal(t, auditsBefore, auditsAfter, "the refusal writes no audit event")

	// A same-date structural version is allowed and supersedes by version_seq.
	eur := f.eurCommodityID
	updateInstrumentFields(t, f, instrument.ID, "Same-date EUR", &eur, "", "", "2026-06-01")
	requireLatestVersionSeq(t, f, "investment_instrument_versions", "instrument_id", instrument.ID, 4)
}

// #550 review: create ignores effective_from after validating its shape and
// future-date bound, so malformed or future input is not silently accepted.
func TestCreateInstrumentValidatesIgnoredEffectiveFrom(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, value, message, code string
	}{
		{"malformed", "not-a-date", "effective date must use YYYY-MM-DD", "VALIDATEDMALFORMED"},
		{"future", "2027-01-01", "effective date must not be in the future", "VALIDATEDFUTURE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newReferenceVersionFixture(t)
			_, err := f.investmentService.CreateInstrument(context.Background(), InvestmentInstrumentInput{
				OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.create",
				CommodityCode: tc.code, InstrumentType: "stock", DisplayName: "Validated date",
				QuantityScale: 6, PriceScale: 4, EffectiveFrom: tc.value,
			})
			require.ErrorContains(t, err, tc.message)
			var count int
			require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_instruments WHERE commodity_id IN (SELECT id FROM commodities WHERE code = ?)`, tc.code).Scan(&count))
			assert.Zero(t, count, "the refused create writes nothing")
		})
	}
}

// #550 review: omitted JSON objects in a backdated correction keep the value
// of the version in effect on that date, not today's value.
func TestInstrumentBackdatedCorrectionKeepsAsOfJSON(t *testing.T) {
	t.Parallel()
	f := newReferenceVersionFixture(t)
	ctx := context.Background()
	usd := seedTestCurrencyCommodity(t, f.database, "USD")

	instrument, err := f.investmentService.CreateInstrument(ctx, InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.create",
		CommodityCode: "ASOFJSON", InstrumentType: "stock", DisplayName: "As-of JSON instrument",
		QuoteCommodityID: &usd, TradingCommodityID: &usd, QuantityScale: 6, PriceScale: 4,
		IdentifiersJSON: `{"isin":"OLD"}`, MetadataJSON: `{"source":"old"}`,
	})
	require.NoError(t, err)
	updateInstrumentFields(t, f, instrument.ID, "Forward identifiers", &usd, `{"isin":"NEW"}`, `{"source":"new"}`, "2026-06-01")
	updateInstrumentFields(t, f, instrument.ID, "March correction", &usd, "", "", "2026-03-01")

	var identifiers, metadata string
	require.NoError(t, f.database.QueryRow(`
		SELECT identifiers_json, metadata_json
		FROM investment_instrument_versions
		WHERE instrument_id = ? AND version_seq = 3
	`, instrument.ID).Scan(&identifiers, &metadata))
	assert.JSONEq(t, `{"isin":"OLD"}`, identifiers, "the March window keeps the identifiers in effect then")
	assert.JSONEq(t, `{"source":"old"}`, metadata, "the March window keeps the metadata in effect then")
}

// #549: instruments follow the account backdating contract: a version dated
// before the latest may correct descriptive fields only.
func TestInstrumentBackdatedStructuralChangeIsRefused(t *testing.T) {
	t.Parallel()
	f := newReferenceVersionFixture(t)
	ctx := context.Background()
	usd := seedTestCurrencyCommodity(t, f.database, "USD")

	instrument, err := f.investmentService.CreateInstrument(ctx, InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.create",
		CommodityCode: "STRUCTURAL", InstrumentType: "stock", DisplayName: "Structural instrument",
		QuantityScale: 6, PriceScale: 4,
	})
	require.NoError(t, err)
	updateInstrumentVersion(t, f, instrument.ID, "Structural instrument renamed", "2026-06-01")

	_, err = f.investmentService.UpdateInstrument(ctx, InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.update",
		InstrumentID: instrument.ID, CommodityCode: instrument.CommodityCode, InstrumentType: instrument.InstrumentType,
		DisplayName: "Structural instrument renamed", QuoteCommodityID: &usd,
		QuantityScale: instrument.QuantityScale, PriceScale: instrument.PriceScale,
		EffectiveFrom: "2026-03-01", ChangeReason: "backdated quote currency",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvestmentInstrumentVersionBackdated), "a backdated structural change gets the coded refusal: %v", err)
}
