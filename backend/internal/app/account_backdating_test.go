package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
)

// #566. An earlier-dated account correction governs only its past window, so
// the update answers with the account as it stands today, as categories,
// payees, institutions (#552) and instruments (#549) already do.
func TestAccountBackdatedRenameReturnsTodaysState(t *testing.T) {
	t.Parallel()
	f := newReferenceBackdatingFixture(t, "account")
	ctx := context.Background()
	today := func() Account {
		t.Helper()
		account, err := f.accountService.Account(ctx, f.id)
		require.NoError(t, err)
		return account
	}
	asOf := func(date string) db.AccountRecord {
		t.Helper()
		record, err := db.NewAccountRepository(f.database).CurrentAccountByID(db.WithReferenceDate(ctx, date), BookID, f.id)
		require.NoError(t, err)
		return record
	}

	updateAccountVersion(t, f.referenceVersionFixture, f.id, "May name", "2026-05-01")
	_, err := f.accountService.CloseAccount(ctx, AccountLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", AccountID: f.id, ClosedOn: "2026-06-01"})
	require.NoError(t, err)

	corrected := updateAccountVersion(t, f.referenceVersionFixture, f.id, "March name", "2026-03-01")
	assert.Equal(t, today(), corrected, "the response is today's state, not the past window's")
	assert.Equal(t, "closed", corrected.Status)
	assert.Equal(t, "May name", corrected.Name)
	assert.Equal(t, "2026-06-01", corrected.ClosedOn)
	assert.Equal(t, "2026-06-01", corrected.EffectiveFrom)
	march := asOf("2026-04-01")
	assert.Equal(t, "active", march.Status)
	assert.Equal(t, "March name", march.Name.String)
	assert.False(t, march.ClosedOn.Valid)
	assert.Equal(t, "May name", asOf("2026-05-15").Name.String)

	// Ordinary edits: dated on the latest version (a same-date version
	// supersedes it) or today, the version written is today's state.
	for _, edit := range []struct{ name, date, effectiveFrom string }{
		{"Closed name", "2026-06-01", "2026-06-01"},
		{"Today name", "", "2026-10-15"},
	} {
		updated := updateAccountVersion(t, f.referenceVersionFixture, f.id, edit.name, edit.date)
		assert.Equal(t, today(), updated, edit.name)
		assert.Equal(t, edit.name, updated.Name)
		assert.Equal(t, edit.effectiveFrom, updated.EffectiveFrom, edit.name)
		assert.Equal(t, "closed", updated.Status, edit.name)
	}
}

// #566. Lifecycle commands cannot be dated before the latest version, so the
// version they write is always today's state; recorded here rather than
// assumed. A backdated command is refused with the shared #552 refusal and
// writes nothing.
func TestAccountLifecycleCommandsReturnTodaysState(t *testing.T) {
	t.Parallel()
	f := newReferenceBackdatingFixture(t, "account")
	ctx := context.Background()
	commands := []struct {
		name, date, status string
		run                func(context.Context, AccountLifecycleInput) (Account, error)
	}{
		{"close", "2026-06-01", "closed", f.accountService.CloseAccount},
		{"archive", "2026-07-01", "archived", f.accountService.ArchiveAccount},
		{"restore", "2026-08-01", "closed", f.accountService.RestoreAccount},
		{"reopen", "2026-09-01", "active", f.accountService.ReopenAccount},
	}
	latest := "2026-01-01"
	for _, command := range commands {
		input := AccountLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", AccountID: f.id}
		input.EffectiveFrom = "2025-12-01"
		if command.name == "close" {
			input.ClosedOn = input.EffectiveFrom
		}
		before := f.counts(t)
		_, err := command.run(ctx, input)
		require.ErrorIs(t, err, ErrReferenceVersionBackdated, command.name)
		backdated, ok := errors.AsType[ReferenceVersionBackdatedError](err)
		require.True(t, ok, command.name)
		assert.Equal(t, latest, backdated.LatestEffectiveFrom, command.name)
		assert.Equal(t, before, f.counts(t), "%s: a refusal writes neither a version nor an audit event", command.name)

		input.EffectiveFrom = command.date
		if command.name == "close" {
			input.ClosedOn = command.date
		}
		changed, err := command.run(ctx, input)
		require.NoError(t, err, command.name)
		current, err := f.accountService.Account(ctx, f.id)
		require.NoError(t, err)
		assert.Equal(t, current, changed, command.name)
		assert.Equal(t, command.status, changed.Status, command.name)
		latest = command.date
	}
}
