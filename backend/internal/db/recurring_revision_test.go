package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecurringTemplateStalePatchCannotOverwriteEditOrWatermark(t *testing.T) {
	for _, concurrent := range []string{"edit", "watermark", "archive"} {
		t.Run(concurrent, func(t *testing.T) {
			ctx := context.Background()
			repo := NewRecurringRepository(newRecurringTestDatabase(t))
			created := createRentTemplate(t, repo)
			params := UpdateRecurringTemplateParams{BookID: 1, TemplateID: created.ID, ActorUserID: 1, UpdatedAt: "2026-08-30T00:00:00Z", Spec: rentTemplateSpec(), ExpectedRevision: &created.Revision}
			switch concurrent {
			case "edit":
				_, err := repo.UpdateRecurringTemplate(ctx, params)
				require.NoError(t, err)
			case "watermark":
				require.NoError(t, repo.SetRecurringTemplateGenerateFrom(ctx, 1, created.ID, "2026-10-01", params.UpdatedAt))
			case "archive":
				_, err := repo.ArchiveRecurringTemplate(ctx, ArchiveRecurringTemplateParams{BookID: 1, TemplateID: created.ID, ActorUserID: 1, ArchivedAt: params.UpdatedAt})
				require.NoError(t, err)
			}
			before, err := repo.RecurringTemplateByID(ctx, 1, created.ID)
			require.NoError(t, err)
			var auditBefore int
			require.NoError(t, repo.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditBefore))
			params.Spec.Name = "Stale edit"
			_, err = repo.UpdateRecurringTemplate(ctx, params)
			if concurrent == "archive" {
				require.ErrorIs(t, err, ErrRecurringTemplateArchived)
			} else {
				require.ErrorIs(t, err, ErrRecurringTemplateConflict)
			}
			after, err := repo.RecurringTemplateByID(ctx, 1, created.ID)
			require.NoError(t, err)
			assert.Equal(t, before, after)
			var auditAfter int
			require.NoError(t, repo.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&auditAfter))
			assert.Equal(t, auditBefore, auditAfter, "a rejected stale write must leave no audit or child mutations")
		})
	}
}

func TestRecurringTemplateArchiveIsIdempotentInRepository(t *testing.T) {
	ctx := context.Background()
	repo := NewRecurringRepository(newRecurringTestDatabase(t))
	created := createRentTemplate(t, repo)
	params := ArchiveRecurringTemplateParams{BookID: 1, TemplateID: created.ID, ActorUserID: 1, ArchivedAt: "2026-08-30T00:00:00Z"}
	archived, err := repo.ArchiveRecurringTemplate(ctx, params)
	require.NoError(t, err)
	var before int
	require.NoError(t, repo.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&before))
	params.ArchivedAt = "2026-08-30T00:00:01Z"
	repeated, err := repo.ArchiveRecurringTemplate(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, archived, repeated)
	var after int
	require.NoError(t, repo.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&after))
	assert.Equal(t, before, after)
}
