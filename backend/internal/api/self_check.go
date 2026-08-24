package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type selfCheckResultResponse struct {
	CheckID      string  `json:"check_id"`
	Status       string  `json:"status"`
	FindingCount int64   `json:"finding_count"`
	Sample       []int64 `json:"sample"`
	Summary      string  `json:"summary"`
	Explanation  string  `json:"explanation"`
	NextStep     string  `json:"next_step"`
}

type selfCheckRunResponse struct {
	ID               int64                     `json:"id"`
	Trigger          string                    `json:"trigger"`
	Status           string                    `json:"status"`
	FailedCheckCount int64                     `json:"failed_check_count"`
	StartedAt        string                    `json:"started_at"`
	FinishedAt       string                    `json:"finished_at,omitempty"`
	Results          []selfCheckResultResponse `json:"results"`
}

type selfCheckStatusResponse struct {
	// HasRun is false for a book that has never been checked. That is a real
	// answer — "not checked yet" — and not an error, so the screen can say it
	// instead of showing an empty table that looks like a pass.
	HasRun    bool                  `json:"has_run"`
	ReadOnly  bool                  `json:"read_only"`
	LatestRun *selfCheckRunResponse `json:"latest_run,omitempty"`
}

func toSelfCheckRunResponse(run app.SelfCheckRun) selfCheckRunResponse {
	results := make([]selfCheckResultResponse, 0, len(run.Results))
	for _, result := range run.Results {
		sample := result.Sample
		if sample == nil {
			sample = []int64{}
		}
		results = append(results, selfCheckResultResponse{
			CheckID:      result.CheckID,
			Status:       result.Status,
			FindingCount: result.FindingCount,
			Sample:       sample,
			Summary:      result.Summary,
			Explanation:  result.Explanation,
			NextStep:     result.NextStep,
		})
	}

	return selfCheckRunResponse{
		ID:               run.ID,
		Trigger:          run.Trigger,
		Status:           run.Status,
		FailedCheckCount: run.FailedCheckCount,
		StartedAt:        run.StartedAt,
		FinishedAt:       run.FinishedAt,
		Results:          results,
	}
}

func selfCheckStatus(logger *slog.Logger, authService *app.AuthService, selfCheckService *app.SelfCheckService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		run, hasRun, err := selfCheckService.LatestSelfCheck(r.Context())
		if err != nil {
			writeServiceInternalError(w, r, logger, "read self-check status", err)
			return
		}

		response := selfCheckStatusResponse{HasRun: hasRun, ReadOnly: true}
		if hasRun {
			latest := toSelfCheckRunResponse(run)
			response.LatestRun = &latest
		}

		writeJSON(w, http.StatusOK, response)
	}
}

func runSelfCheck(logger *slog.Logger, authService *app.AuthService, selfCheckService *app.SelfCheckService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedMutationOwner(w, r); !ok {
			return
		}

		// A POST that reads: the check itself changes nothing in the ledger, but
		// running one records that it ran, and recording is a write. The verb
		// describes the request, and read_only in the response describes the
		// check.
		run, err := selfCheckService.RunSelfCheck(r.Context(), "manual")
		if err != nil {
			if writeExportValidationError(w, err) {
				return
			}
			writeServiceInternalError(w, r, logger, "run self-check", err)
			return
		}

		// 200, not 202: unlike a backup, this finished before responding.
		writeJSON(w, http.StatusOK, toSelfCheckRunResponse(run))
	}))
}
