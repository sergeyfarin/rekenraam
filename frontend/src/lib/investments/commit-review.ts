import type { RequestKey } from '#lib/api/request-key.ts';
import type { GainImpact, ReconciliationImpactResponse } from '#lib/api/investments.ts';
import { hasGainChanges, impactNeedsReview, isStaleImpactRefusal } from '#lib/investments/gain-impact.ts';
import { reviewGuards, type ReviewGuards } from '#lib/investments/preview-review.ts';

/**
 * The review-at-commit protocol (#400): the form asks the server for the
 * write's impact just before recording and opens a review only when it would
 * invalidate checkpoints or, under the gain protocol, change committed gains.
 * Corrections, basis resolution, trades, payments in lieu and provider
 * completions share it; preview-then-record forms use previewReview instead.
 *
 * A stale refusal of the confirmed write asks for the impact again and opens
 * a refreshed review; it never retries the write or reuses an
 * acknowledgement. A checkpoint-only protocol never acquires a gain
 * requirement, whatever the impact response carries. The host keeps its own
 * reactive state: this only moves it.
 */
type CheckpointImpact = ReconciliationImpactResponse['affected_checkpoints'][number];

export type CommitReviewState<S> = {
  impacts: CheckpointImpact[];
  gainImpact: GainImpact | null;
  gainRefreshed: boolean;
  subject: S;
};

export type CommitImpact = {
  affected_checkpoints: CheckpointImpact[];
  gain_impact?: GainImpact | null;
};

export type CommitReviewHost<S, I extends CommitImpact> = {
  /** gains: checkpoints and changed gains; checkpoints: checkpoints only. */
  readonly protocol: 'gains' | 'checkpoints';
  readonly review: CommitReviewState<S> | null;
  /** The impact the write would have now, simulated and rolled back. */
  impact(subject: S): Promise<I>;
  /**
   * The entry's request key, when the write is a keyed create sent through
   * `requestKey.send(subject, …)`. Its unresolved write is sent again as it
   * was, without asking for the impact first.
   */
  readonly requestKey?: RequestKey;
  /** Records once with exactly the reviewed guards; may invalidate and close. */
  write(subject: S, guards: ReviewGuards): Promise<void>;
  setReview(review: CommitReviewState<S> | null): void;
  setPending(pending: boolean): void;
  setError(error: unknown): void;
};

export function commitReview<S, I extends CommitImpact>(host: CommitReviewHost<S, I>) {
  /** Opens a review when the write needs a decision; false when it needs none. */
  async function open(subject: S, refreshed: boolean): Promise<boolean> {
    const impact = await host.impact(subject);
    const gainImpact = host.protocol === 'gains' && hasGainChanges(impact.gain_impact) ? impact.gain_impact : null;
    if (!impactNeedsReview({ affected_checkpoints: impact.affected_checkpoints, gain_impact: gainImpact })) return false;
    host.setReview({ impacts: impact.affected_checkpoints, gainImpact, gainRefreshed: refreshed && !!gainImpact, subject });
    return true;
  }

  /** After a refused write: a stale refusal re-opens the current review. */
  async function recover(error: unknown, subject: S) {
    try {
      if (!isStaleImpactRefusal(error) || !(await open(subject, true))) host.setError(error);
    } catch (previewError) {
      host.setError(previewError);
    }
  }

  return {
    open,
    recover,
    /**
     * Saves the subject: an unchanged retry of a write with an unknown
     * outcome is sent again as it was; otherwise the impact decides whether
     * a review opens first. Throws what the write throws; pass it to recover.
     */
    async submit(subject: S): Promise<void> {
      // The write's impact may already be in the ledger: a lost sale's
      // impact is refused because its units are gone. An unchanged entry
      // whose last write has an unknown outcome goes straight to the write,
      // which sends that write again through the request key (#660).
      if (host.requestKey?.isUnresolved(subject)) {
        await host.write(subject, {});
        return;
      }
      if (await open(subject, false)) return;
      await host.write(subject, {});
    },
    /** Records with exactly the checkpoints and gain set the review showed. */
    async confirm() {
      const current = host.review;
      if (!current) return;
      host.setReview(null);
      host.setPending(true);
      host.setError(undefined);
      try {
        await host.write(current.subject, reviewGuards(current.impacts, current.gainImpact));
      } catch (error) {
        await recover(error, current.subject);
      } finally {
        host.setPending(false);
      }
    },
    cancel() {
      host.setReview(null);
    }
  };
}
