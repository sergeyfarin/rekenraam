package api

import (
	"errors"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

// #485 review: a statement row linked to a posting can be inspected,
// unlinked and moved to another posting. Corrections name the link the owner
// reviewed and a reason; each is one audit event and keeps the old link.

type importStatementLinkResponse struct {
	IdentityID    int64  `json:"identity_id"`
	LinkID        int64  `json:"link_id"`
	TransactionID int64  `json:"transaction_id"`
	LineKey       string `json:"line_key"`
	AccountID     int64  `json:"account_id"`
	CommodityID   int64  `json:"commodity_id"`
	Active        bool   `json:"active"`
	LinkedAt      string `json:"linked_at"`
	RetiredAt     string `json:"retired_at,omitempty"`
	RetireReason  string `json:"retire_reason,omitempty"`
	SourceKind    string `json:"source_kind"`
	BatchID       int64  `json:"batch_id"`
	Filename      string `json:"filename"`
	RowDate       string `json:"row_date"`
	RowAmount     string `json:"row_amount_value"`
	RowScale      int    `json:"row_amount_scale"`
	RowPayee      string `json:"row_payee"`
}

type listStatementLinksResponse struct {
	Links []importStatementLinkResponse `json:"links"`
}

type statementLinkCandidatesResponse struct {
	Link       importStatementLinkResponse    `json:"link"`
	Candidates []importMatchCandidateResponse `json:"candidates"`
	WindowDays int                            `json:"window_days"`
}

type correctStatementLinkRequest struct {
	ExpectedLinkID int64 `json:"expected_link_id"`
	// ExpectedLinkActive is the reviewed state of that link; required.
	ExpectedLinkActive *bool  `json:"expected_link_active"`
	Reason             string `json:"reason"`
	TransactionID      int64  `json:"transaction_id,omitempty"`
	LineKey            string `json:"line_key,omitempty"`
	PostedOn           string `json:"posted_on,omitempty"`
}

func toImportStatementLinkResponse(link app.ImportStatementLink) importStatementLinkResponse {
	return importStatementLinkResponse{
		IdentityID: link.IdentityID, LinkID: link.LinkID, TransactionID: link.TransactionID, LineKey: link.LineKey,
		AccountID: link.AccountID, CommodityID: link.CommodityID, Active: link.Active, LinkedAt: link.LinkedAt,
		RetiredAt: link.RetiredAt, RetireReason: link.RetireReason, SourceKind: link.SourceKind,
		BatchID: link.BatchID, Filename: link.Filename, RowDate: link.RowDate,
		RowAmount: string(link.RowAmount), RowScale: link.RowScale, RowPayee: link.RowPayee,
	}
}

func listTransactionStatementLinks(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "id", "transaction id")
		if !ok {
			return
		}
		links, err := importService.StatementLinks(r.Context(), transactionID)
		if err != nil {
			writeStatementLinkError(w, r, logger, "list statement links", err)
			return
		}
		response := listStatementLinksResponse{Links: make([]importStatementLinkResponse, 0, len(links))}
		for _, link := range links {
			response.Links = append(response.Links, toImportStatementLinkResponse(link))
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func statementLinkCandidates(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		identityID, ok := readPathInt64(w, r, "identity_id", "identity id")
		if !ok {
			return
		}
		link, candidates, err := importService.StatementLinkCandidates(r.Context(), identityID)
		if err != nil {
			writeStatementLinkError(w, r, logger, "statement link candidates", err)
			return
		}
		reviews := toRowMatchReviewResponses([]app.RowMatchReview{{Candidates: candidates}})
		writeJSON(w, http.StatusOK, statementLinkCandidatesResponse{
			Link: toImportStatementLinkResponse(link), Candidates: reviews[0].Candidates, WindowDays: app.ImportMatchWindowDays,
		})
	}
}

func correctStatementLink(logger *slog.Logger, authService *app.AuthService, importService *app.ImportService, options HandlerOptions, reassign bool) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		identityID, ok := readPathInt64(w, r, "identity_id", "identity id")
		if !ok {
			return
		}
		var request correctStatementLinkRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		input := app.CorrectStatementLinkInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			IdentityID: identityID, ExpectedLinkID: request.ExpectedLinkID, ExpectedLinkActive: request.ExpectedLinkActive, Reason: request.Reason,
			TransactionID: request.TransactionID, LineKey: request.LineKey, PostedOn: request.PostedOn,
		}
		var err error
		if reassign {
			err = importService.ReassignStatementRow(r.Context(), input)
		} else {
			err = importService.UnlinkStatementRow(r.Context(), input)
		}
		if err != nil {
			writeStatementLinkError(w, r, logger, "correct statement link", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func writeStatementLinkError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, op string, err error) {
	switch {
	case errors.Is(err, app.ErrImportStatementLinkNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "statement link not found")
	case errors.Is(err, app.ErrImportStatementLinkStale):
		writeAPIError(w, http.StatusConflict, "IMPORT_LINK_STALE", "the statement link or the posting it would move to changed after review; review it again")
	default:
		writeImportServiceError(w, r, logger, op, err)
	}
}
