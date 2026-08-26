package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
)

type backupPolicyResponse struct {
	Enabled             bool   `json:"enabled"`
	HourLocal           int    `json:"hour_local"`
	MinuteLocal         int    `json:"minute_local"`
	TimeZone            string `json:"time_zone"`
	RetentionCount      int    `json:"retention_count"`
	RetentionMaxAgeDays *int64 `json:"retention_max_age_days"`
	UpdatedAt           string `json:"updated_at,omitempty"`
}

type backupRunResponse struct {
	ID                    int64  `json:"id"`
	Trigger               string `json:"trigger"`
	Status                string `json:"status"`
	TargetPath            string `json:"target_path"`
	ScheduledForLocalDate string `json:"scheduled_for_local_date,omitempty"`
	ByteSize              *int64 `json:"byte_size,omitempty"`
	PageCount             *int64 `json:"page_count,omitempty"`
	Verified              bool   `json:"verified"`
	Attempts              int    `json:"attempts"`
	// WillRetry distinguishes a backup that failed and is queued to try again
	// from one that has spent its attempts. The run row says "failed" for both,
	// and showing one word for two outcomes tells a reader nothing they can act
	// on (T-69).
	WillRetry    bool   `json:"will_retry"`
	ErrorSummary string `json:"error_summary,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	FinishedAt   string `json:"finished_at,omitempty"`
	PrunedAt     string `json:"pruned_at,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// backupKeyNoticeResponse is the part of the protection story a database backup
// cannot carry. It is served with the status rather than left to the docs,
// because the consequence only becomes visible during a restore, which is the
// worst moment to discover it.
type backupKeyNoticeResponse struct {
	IncludedInBackup    bool   `json:"included_in_backup"`
	EnvironmentVariable string `json:"environment_variable"`
	Protects            string `json:"protects"`
	Consequence         string `json:"consequence"`
}

type backupStatusResponse struct {
	Policy      backupPolicyResponse    `json:"policy"`
	Directory   string                  `json:"directory"`
	NextRunAt   string                  `json:"next_run_at,omitempty"`
	LastSuccess *backupRunResponse      `json:"last_success,omitempty"`
	Runs        []backupRunResponse     `json:"runs"`
	KeyNotice   backupKeyNoticeResponse `json:"secret_key"`
}

type backupPolicyRequest struct {
	Enabled             *bool  `json:"enabled"`
	HourLocal           *int   `json:"hour_local"`
	MinuteLocal         *int   `json:"minute_local"`
	RetentionCount      *int   `json:"retention_count"`
	RetentionMaxAgeDays *int64 `json:"retention_max_age_days"`
}

func toBackupPolicyResponse(policy app.BackupPolicy) backupPolicyResponse {
	return backupPolicyResponse{
		Enabled:             policy.Enabled,
		HourLocal:           policy.HourLocal,
		MinuteLocal:         policy.MinuteLocal,
		TimeZone:            policy.TimeZone,
		RetentionCount:      policy.RetentionCount,
		RetentionMaxAgeDays: policy.RetentionMaxAgeDays,
		UpdatedAt:           policy.UpdatedAt,
	}
}

func toBackupRunResponse(run db.BackupRunRecord) backupRunResponse {
	response := backupRunResponse{
		ID:                    run.ID,
		Trigger:               run.Trigger,
		Status:                run.Status,
		TargetPath:            run.TargetPath,
		ScheduledForLocalDate: run.ScheduledForLocalDate.String,
		Verified:              run.Verified,
		Attempts:              run.Attempts,
		WillRetry:             run.Status == "failed" && run.WorkStatus.String == "pending",
		ErrorSummary:          run.ErrorSummary,
		StartedAt:             run.StartedAt.String,
		FinishedAt:            run.FinishedAt.String,
		PrunedAt:              run.PrunedAt.String,
		CreatedAt:             run.CreatedAt,
	}
	if run.ByteSize.Valid {
		size := run.ByteSize.Int64
		response.ByteSize = &size
	}
	if run.PageCount.Valid {
		pages := run.PageCount.Int64
		response.PageCount = &pages
	}
	return response
}

func backupKeyNotice() backupKeyNoticeResponse {
	return backupKeyNoticeResponse{
		IncludedInBackup:    false,
		EnvironmentVariable: "REKENRAAM_SECRET_KEY",
		Protects:            "multi-factor enrolment and connection credentials",
		Consequence:         "restoring this database without the original key gives you an intact ledger, unreadable multi-factor enrolment, and unusable connection credentials; keep the key somewhere other than the backup directory",
	}
}

func backupStatus(logger *slog.Logger, authService *app.AuthService, backupService *app.BackupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		status, err := backupService.Status(r.Context())
		if err != nil {
			writeServiceInternalError(w, r, logger, "read backup status", err)
			return
		}

		runs := make([]backupRunResponse, 0, len(status.Runs))
		for _, run := range status.Runs {
			runs = append(runs, toBackupRunResponse(run))
		}
		response := backupStatusResponse{
			Policy:    toBackupPolicyResponse(status.Policy),
			Directory: status.Directory,
			Runs:      runs,
			KeyNotice: backupKeyNotice(),
		}
		if !status.NextRunAt.IsZero() {
			response.NextRunAt = status.NextRunAt.UTC().Format(time.RFC3339)
		}
		if status.LastSuccess != nil {
			last := toBackupRunResponse(*status.LastSuccess)
			response.LastSuccess = &last
		}

		writeJSON(w, http.StatusOK, response)
	}
}

func saveBackupPolicy(logger *slog.Logger, authService *app.AuthService, backupService *app.BackupService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var request backupPolicyRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "request body is not valid JSON")
			return
		}

		current, err := backupService.Policy(r.Context())
		if err != nil {
			writeServiceInternalError(w, r, logger, "read backup policy", err)
			return
		}

		// Omitted fields keep their current value: a PATCH-shaped body with
		// plain values would silently overwrite what it did not mention.
		input := app.SaveBackupPolicyInput{
			UserID:              owner.ID,
			Enabled:             current.Enabled,
			HourLocal:           current.HourLocal,
			MinuteLocal:         current.MinuteLocal,
			RetentionCount:      current.RetentionCount,
			RetentionMaxAgeDays: current.RetentionMaxAgeDays,
		}
		if request.Enabled != nil {
			input.Enabled = *request.Enabled
		}
		if request.HourLocal != nil {
			input.HourLocal = *request.HourLocal
		}
		if request.MinuteLocal != nil {
			input.MinuteLocal = *request.MinuteLocal
		}
		if request.RetentionCount != nil {
			input.RetentionCount = *request.RetentionCount
		}
		if request.RetentionMaxAgeDays != nil {
			input.RetentionMaxAgeDays = request.RetentionMaxAgeDays
		}

		policy, err := backupService.SavePolicy(r.Context(), input)
		if err != nil {
			if writeExportValidationError(w, err) {
				return
			}
			writeServiceInternalError(w, r, logger, "save backup policy", err)
			return
		}

		writeJSON(w, http.StatusOK, toBackupPolicyResponse(policy))
	}))
}

func requestBackup(logger *slog.Logger, authService *app.AuthService, backupService *app.BackupService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		run, err := backupService.RequestBackup(r.Context(), owner.ID)
		if err != nil {
			writeServiceInternalError(w, r, logger, "request backup", err)
			return
		}

		// 202, not 201: this reports accepted work. The copy has not happened,
		// and saying otherwise would be the claim ADR 0010 forbids.
		writeJSON(w, http.StatusAccepted, toBackupRunResponse(run))
	}))
}

func retryBackupRun(logger *slog.Logger, authService *app.AuthService, backupService *app.BackupService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedMutationOwner(w, r); !ok {
			return
		}

		runID, err := strconv.ParseInt(r.PathValue("run_id"), 10, 64)
		if err != nil || runID <= 0 {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "run_id is invalid")
			return
		}

		run, err := backupService.RetryBackupRun(r.Context(), runID)
		if err != nil {
			if writeExportValidationError(w, err) {
				return
			}
			if errors.Is(err, db.ErrNotFound) {
				writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "backup run not found")
				return
			}
			writeServiceInternalError(w, r, logger, "retry backup run", err)
			return
		}

		writeJSON(w, http.StatusAccepted, toBackupRunResponse(run))
	}))
}
