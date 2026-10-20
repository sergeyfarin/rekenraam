package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"rekenraam/backend/internal/db"
)

// throttleReservation is one admitted attempt's place in each of its scopes'
// budgets (#285).
//
// Admission is decided atomically before any verifier runs, and the place is
// held until the attempt settles. Checking the counter first and recording
// the failure after the hash let every request that read the counter early
// verify a guess: sixteen parallel attempts got sixteen guesses against a
// budget of five. Now the places in flight count against the budget, so the
// guesses admitted within a generation never exceed its budget. Older
// generations may finish after rollover; successful resets admit a fresh budget.
//
// A failure keeps its place; a success or an attempt that never reached a
// verdict gives it back. A place a crashed process never settled counts as a
// failure and ages out with the window. Settlement only affects the generation
// that admitted it (#366); rollover and successful reset supersede old places.
type throttleReservation struct {
	service *AuthService
	scopes  []db.ThrottleScopeKey
	settled bool
}

// reserveAttempt admits one attempt in every scope, or none: a refusal in a
// later scope gives back the places already taken.
func (s *AuthService) reserveAttempt(ctx context.Context, scopes []loginThrottleScope) (*throttleReservation, error) {
	reservation := &throttleReservation{service: s}
	for _, scope := range scopes {
		generation, retryAfter, err := s.reserveScope(ctx, scope)
		if err == nil && retryAfter > 0 {
			err = RateLimitError{RetryAfter: retryAfter}
		}
		if err != nil {
			reservation.release(ctx)
			return nil, err
		}
		reservation.scopes = append(reservation.scopes, db.ThrottleScopeKey{ScopeType: scope.ScopeType, ScopeKey: scope.ScopeKey, Generation: generation})
	}

	return reservation, nil
}

// reserveScope returns a positive wait when the scope cannot admit another
// attempt.
func (s *AuthService) reserveScope(ctx context.Context, scope loginThrottleScope) (string, time.Duration, error) {
	var retryAfter time.Duration
	generation := uuid.NewString()
	err := s.repository.UpdateLoginThrottle(ctx, scope.ScopeType, scope.ScopeKey, func(current db.LoginThrottleRecord, found bool) (*db.UpsertLoginThrottleParams, error) {
		now := s.now().UTC()
		attempts := 0
		if found {
			attempts = current.FailedAttempts
			if current.Generation != "" {
				generation = current.Generation
			}
			if current.BlockedUntil.Valid {
				blockedUntil, err := time.Parse(time.RFC3339, current.BlockedUntil.String)
				if err != nil {
					return nil, fmt.Errorf("parse login throttle blocked_until: %w", err)
				}
				if blockedUntil.After(now) {
					retryAfter = blockedUntil.Sub(now)
					return nil, nil
				}
				attempts = 0
				generation = uuid.NewString()
			} else if attempts >= loginThrottleMaxFailures {
				// Every place is held by an attempt still in flight, or by one
				// a crashed process never settled; the latter expire.
				lastAdmitted, err := time.Parse(time.RFC3339, current.UpdatedAt)
				if err != nil {
					return nil, fmt.Errorf("parse login throttle updated_at: %w", err)
				}
				if heldUntil := lastAdmitted.Add(loginThrottleWindow); heldUntil.After(now) {
					retryAfter = heldUntil.Sub(now)
					return nil, nil
				}
				attempts = 0
				generation = uuid.NewString()
			}
		}

		return &db.UpsertLoginThrottleParams{
			ScopeType:      scope.ScopeType,
			ScopeKey:       scope.ScopeKey,
			FailedAttempts: attempts + 1,
			UpdatedAt:      now.Format(time.RFC3339),
			Generation:     generation,
		}, nil
	})
	if err != nil {
		return "", 0, fmt.Errorf("reserve login attempt: %w", err)
	}

	return generation, retryAfter, nil
}

// fail settles the attempt as a failure. Its place stays counted; the failure
// that fills the budget starts the block. It returns the block's remaining
// wait, or zero when the scope is not blocked.
func (r *throttleReservation) fail(ctx context.Context) (time.Duration, error) {
	r.settled = true
	var longest time.Duration
	for _, scope := range r.scopes {
		var retryAfter time.Duration
		err := r.service.repository.UpdateLoginThrottle(ctx, scope.ScopeType, scope.ScopeKey, func(current db.LoginThrottleRecord, found bool) (*db.UpsertLoginThrottleParams, error) {
			now := r.service.now().UTC()
			// A success/reset or rollover supersedes this reservation. Never
			// recreate a missing row or charge a replacement generation.
			if !found || current.Generation != scope.Generation {
				return nil, nil
			}
			if current.BlockedUntil.Valid {
				blockedUntil, err := time.Parse(time.RFC3339, current.BlockedUntil.String)
				if err != nil {
					return nil, fmt.Errorf("parse login throttle blocked_until: %w", err)
				}
				retryAfter = max(blockedUntil.Sub(now), 0)
				return nil, nil
			}
			attempts := current.FailedAttempts
			next := &db.UpsertLoginThrottleParams{
				ScopeType:      scope.ScopeType,
				ScopeKey:       scope.ScopeKey,
				FailedAttempts: attempts,
				UpdatedAt:      now.Format(time.RFC3339),
				Generation:     scope.Generation,
			}
			if attempts >= loginThrottleMaxFailures {
				blocked := now.Add(loginThrottleWindow).Format(time.RFC3339)
				next.BlockedUntil = &blocked
				next.FailedAttempts = 0
				retryAfter = loginThrottleWindow
			}
			return next, nil
		})
		if err != nil {
			return 0, fmt.Errorf("record login throttle failure: %w", err)
		}
		longest = max(longest, retryAfter)
	}

	return longest, nil
}

// clear settles a verified attempt outside a sign-in (the reauthentication
// budget): its scopes are reset as a success would.
func (r *throttleReservation) clear(ctx context.Context) error {
	r.settled = true
	clears, refunds := r.successSettlement()
	for _, scope := range clears {
		if err := r.service.repository.DeleteLoginThrottle(ctx, scope); err != nil {
			return fmt.Errorf("clear login throttle: %w", err)
		}
	}
	for _, scope := range refunds {
		if err := r.service.repository.RefundLoginAttempt(ctx, scope); err != nil {
			return fmt.Errorf("refund login attempt: %w", err)
		}
	}

	return nil
}

// successSettlement says what a success does to each scope. The username,
// device and session budgets reset. The address budget only gets this
// attempt's place back: resetting it on a success from the same address would
// un-block an attacker sharing a NAT address with the owner.
func (r *throttleReservation) successSettlement() (clears []db.ThrottleScopeKey, refunds []db.ThrottleScopeKey) {
	for _, scope := range r.scopes {
		if scope.ScopeType == throttleScopeClientIP {
			refunds = append(refunds, scope)
			continue
		}
		clears = append(clears, scope)
	}

	return clears, refunds
}

// release gives back the places of an attempt that reached no verdict — a
// verified password still owing a second factor, a missing owner, or an
// internal error. Deferred by every caller; a settled reservation is a no-op.
// Best-effort: a place it cannot give back expires with the window.
func (r *throttleReservation) release(ctx context.Context) {
	if r == nil || r.settled {
		return
	}
	r.settled = true
	for _, scope := range r.scopes {
		if err := r.service.repository.RefundLoginAttempt(ctx, scope); err != nil && r.service.logger != nil {
			r.service.logger.WarnContext(ctx, "release login attempt", slog.String("scope_type", scope.ScopeType), slog.Any("err", err))
		}
	}
}
