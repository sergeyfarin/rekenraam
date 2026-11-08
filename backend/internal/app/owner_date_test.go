package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
)

func TestOwnerTodayUsesServerClockAndSavedZone(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, instant, zone, date string }{
		{"east of UTC midnight", "2026-10-13T23:30:00Z", "Pacific/Kiritimati", "2026-10-14"},
		{"west of UTC midnight", "2026-10-14T00:30:00Z", "Pacific/Honolulu", "2026-10-13"},
		{"owner year ahead", "2026-12-31T23:30:00Z", "Europe/Amsterdam", "2027-01-01"},
		{"owner year behind", "2027-01-01T00:30:00Z", "America/Los_Angeles", "2026-12-31"},
		{"before DST jump", "2026-03-29T00:30:00Z", "Europe/Amsterdam", "2026-03-29"},
		{"after DST jump", "2026-03-29T01:30:00Z", "Europe/Amsterdam", "2026-03-29"},
		{"before DST repeat", "2026-10-25T00:30:00Z", "Europe/Amsterdam", "2026-10-25"},
		{"after DST repeat", "2026-10-25T01:30:00Z", "Europe/Amsterdam", "2026-10-25"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			service := NewSettingsService(db.NewSettingsRepository(f.database))
			now, err := time.Parse(time.RFC3339, tc.instant)
			require.NoError(t, err)
			service.now = func() time.Time { return now }
			_, err = service.SavePreferences(context.Background(), SaveUserPreferencesInput{UserID: f.ownerUserID, TimeZone: tc.zone})
			require.NoError(t, err)
			today, err := service.OwnerToday(context.Background(), f.ownerUserID)
			require.NoError(t, err)
			assert.Equal(t, tc.date, today)
		})
	}
}

func TestOwnerTodayRefreshesZoneAndClockWithoutWriting(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	service := NewSettingsService(db.NewSettingsRepository(f.database))
	now := time.Date(2026, 12, 31, 23, 30, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	_, err := service.SavePreferences(ctx, SaveUserPreferencesInput{UserID: f.ownerUserID, TimeZone: "Europe/Amsterdam"})
	require.NoError(t, err)
	today, err := service.OwnerToday(ctx, f.ownerUserID)
	require.NoError(t, err)
	assert.Equal(t, "2027-01-01", today)
	_, err = service.SavePreferences(ctx, SaveUserPreferencesInput{UserID: f.ownerUserID, TimeZone: "Pacific/Honolulu"})
	require.NoError(t, err)
	today, err = service.OwnerToday(ctx, f.ownerUserID)
	require.NoError(t, err)
	assert.Equal(t, "2026-12-31", today)
	now = now.Add(24 * time.Hour)
	today, err = service.OwnerToday(ctx, f.ownerUserID)
	require.NoError(t, err)
	assert.Equal(t, "2027-01-01", today)
	var before int
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`).Scan(&before))
	_, err = service.OwnerToday(ctx, f.ownerUserID)
	require.NoError(t, err)
	var after int
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`).Scan(&after))
	assert.Equal(t, before, after)
	_, err = f.database.ExecContext(ctx, `UPDATE user_preferences SET time_zone = 'Not/AZone' WHERE user_id = ?`, f.ownerUserID)
	require.NoError(t, err)
	_, err = service.OwnerToday(ctx, f.ownerUserID)
	require.Error(t, err)
	// Match the existing Preferences contract when no row is stored.
	_, err = f.database.ExecContext(ctx, `DELETE FROM user_preferences WHERE user_id = ?`, f.ownerUserID)
	require.NoError(t, err)
	today, err = service.OwnerToday(ctx, f.ownerUserID)
	require.NoError(t, err)
	assert.Equal(t, "2027-01-01", today)
}

// TestOwnerEnteredDatesFollowOwnerDay pins #339: account opened dates and
// version effective dates are bounded and defaulted by the owner's day, not
// the server's UTC day, on both sides of UTC midnight.
func TestOwnerEnteredDatesFollowOwnerDay(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, instant, zone, today, refused string
	}{
		// 00:01 CEST: the owner's today is still the UTC yesterday-plus-one.
		{"owner ahead just after local midnight", "2026-10-18T22:01:00Z", "Europe/Amsterdam", "2026-10-19", "2026-10-20"},
		// 20:00 PDT: the UTC day is already the owner's tomorrow.
		{"owner behind on the UTC day that is their tomorrow", "2026-10-19T03:00:00Z", "America/Los_Angeles", "2026-10-18", "2026-10-19"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			now, err := time.Parse(time.RFC3339, tc.instant)
			require.NoError(t, err)
			clock := func() time.Time { return now }
			settings := NewSettingsService(db.NewSettingsRepository(f.database))
			settings.now = clock
			_, err = settings.SavePreferences(ctx, SaveUserPreferencesInput{UserID: f.ownerUserID, TimeZone: tc.zone})
			require.NoError(t, err)
			accounts := f.accountService
			accounts.SetSettings(settings)
			accounts.now = clock
			institutions := NewInstitutionService(db.NewInstitutionRepository(f.database))
			institutions.SetSettings(settings)
			institutions.now = clock

			account := func(name, openedOn, effectiveFrom string) (Account, error) {
				return accounts.CreateAccount(ctx, CreateAccountInput{
					OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.create",
					Name: name, AccountClass: "asset", AccountKind: "checking",
					DefaultCommodityID: &f.eurCommodityID, OpenedOn: openedOn, EffectiveFrom: effectiveFrom,
				})
			}
			opened, err := account("Opened today", tc.today, "")
			require.NoError(t, err)
			assert.Equal(t, tc.today, opened.OpenedOn)
			assert.Equal(t, tc.today, opened.EffectiveFrom)
			_, err = account("Opened tomorrow", tc.refused, "")
			assert.ErrorContains(t, err, "account opened date must not be in the future")
			_, err = account("Version tomorrow", tc.today, tc.refused)
			assert.ErrorContains(t, err, "effective date must not be in the future")
			blank, err := account("Blank dates", "", "")
			require.NoError(t, err)
			assert.Equal(t, tc.today, blank.OpenedOn)
			assert.Equal(t, tc.today, blank.EffectiveFrom)

			_, err = accounts.CloseAccount(ctx, AccountLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", AccountID: opened.ID, ClosedOn: tc.refused})
			assert.ErrorContains(t, err, "account closed date must not be in the future")
			closed, err := accounts.CloseAccount(ctx, AccountLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", AccountID: opened.ID})
			require.NoError(t, err)
			assert.Equal(t, tc.today, closed.ClosedOn)

			institution := func(name, effectiveFrom string) (Institution, error) {
				return institutions.CreateInstitution(ctx, CreateInstitutionInput{
					OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "institution.create",
					Name: name, Kind: "bank", EffectiveFrom: effectiveFrom,
				})
			}
			dated, err := institution("Dated today", tc.today)
			require.NoError(t, err)
			assert.Equal(t, tc.today, dated.EffectiveFrom)
			_, err = institution("Dated tomorrow", tc.refused)
			assert.ErrorContains(t, err, "effective date must not be in the future")
			blankVersion, err := institution("Blank version", "")
			require.NoError(t, err)
			assert.Equal(t, tc.today, blankVersion.EffectiveFrom)
		})
	}
}

// #385: a write and a current read must use the same owner-local day.
func TestCurrentReferenceReadsFollowOwnerDay(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, instant, zone, today string }{
		{"east just after local midnight", "2027-01-01T23:01:00Z", "Europe/Amsterdam", "2027-01-02"},
		{"west while UTC is tomorrow", "2027-01-02T03:00:00Z", "America/Los_Angeles", "2027-01-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newInvestmentsTestFixture(t)
			now, err := time.Parse(time.RFC3339, tc.instant)
			require.NoError(t, err)
			settings := NewSettingsService(db.NewSettingsRepository(f.database))
			settings.now = func() time.Time { return now }
			_, err = settings.SavePreferences(ctx, SaveUserPreferencesInput{UserID: f.ownerUserID, TimeZone: tc.zone})
			require.NoError(t, err)
			accounts := f.accountService
			accounts.SetSettings(settings)
			accounts.now = settings.now
			created, err := accounts.CreateAccount(ctx, CreateAccountInput{
				OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.create", Name: "Owner today",
				AccountClass: "asset", AccountKind: "checking", DefaultCommodityID: &f.eurCommodityID,
			})
			require.NoError(t, err)
			require.Equal(t, tc.today, created.EffectiveFrom)
			read, err := accounts.Account(ctx, created.ID)
			require.NoError(t, err)
			assert.Equal(t, created.VersionID, read.VersionID)
			listed, err := accounts.ListAccounts(ctx, ListAccountsInput{Query: "Owner today"})
			require.NoError(t, err)
			require.Len(t, listed.Accounts, 1)
			assert.Equal(t, created.ID, listed.Accounts[0].ID)
		})
	}
}
