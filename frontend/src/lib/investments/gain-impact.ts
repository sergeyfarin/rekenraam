import { APIClientError } from '$lib/api/client';
import type { GainImpact, GainImpactChange } from '$lib/api/investments';
import { formatExactMoney } from '$lib/money/format';

/**
 * Display rows for a replay gain disclosure (T-114). The backend computes and
 * compares every figure exactly; this only formats them. An unknown basis
 * yields a null gain, which is shown as unknown rather than as zero.
 */
export type GainImpactRow = {
  key: string;
  kind: GainImpactChange['change_kind'];
  date: string;
  before: string | null;
  after: string | null;
  basisBefore: string | null;
  basisAfter: string | null;
  currency: string;
};

export type GainImpactCurrency = { code: string; standardScale: number };

export function gainImpactRows(
  changes: GainImpactChange[],
  currency: (commodityID: number) => GainImpactCurrency,
  locale: string
): GainImpactRow[] {
  return changes.map((change) => {
    const before = currency(change.before.cost_commodity_id);
    const after = change.after ? currency(change.after.cost_commodity_id) : before;
    return {
      key: `${change.root_operation_id}:${change.decision_seq}`,
      kind: change.change_kind,
      date: change.after?.disposal_date ?? change.before.disposal_date,
      before: formatGain(change.before, before.standardScale, locale),
      after: change.after ? formatGain(change.after, after.standardScale, locale) : null,
      basisBefore: formatBasis(change.before, before.standardScale, locale),
      basisAfter: change.after ? formatBasis(change.after, after.standardScale, locale) : null,
      currency: before.code
    };
  });
}

// Replay can widen a gain's scale; the disclosure shows every significant
// digit without padding, and never rounds a revision away.
function formatGain(state: GainImpactChange['before'], standardScale: number, locale: string): string | null {
  if (state.realized_gain_value === null || state.realized_gain_scale === null) return null;
  return formatExactMoney(state.realized_gain_value, state.realized_gain_scale, standardScale, locale);
}

// Unknown basis stays unknown; it is never shown as zero.
function formatBasis(state: GainImpactChange['before'], standardScale: number, locale: string): string | null {
  if (state.disposed_basis_value === null || state.disposed_basis_scale === null) return null;
  return formatExactMoney(state.disposed_basis_value, state.disposed_basis_scale, standardScale, locale);
}

/** True when the preview requires the user to acknowledge changed gains. */
export function hasGainChanges(impact: GainImpact | undefined | null): impact is GainImpact {
  return !!impact && impact.changes.length > 0;
}

type CurrencyLike = { code: string; standard_scale: number };

/** Currency lookup for gainImpactRows over a loaded currency map. An unknown
 * currency keeps every stored digit (standard scale 0) rather than guessing. */
export function gainImpactCurrency(currencies: Map<number, CurrencyLike>) {
  return (commodityID: number): GainImpactCurrency => {
    const currency = currencies.get(commodityID);
    return { code: currency?.code ?? `#${commodityID}`, standardScale: currency?.standard_scale ?? 0 };
  };
}

/** A preview needs the user's decision when it would invalidate checkpoints
 * or change committed disposal gains (T-114/T-126). */
export function impactNeedsReview(impact: {
  affected_checkpoints: unknown[];
  gain_impact?: GainImpact | null;
}): boolean {
  return impact.affected_checkpoints.length > 0 || hasGainChanges(impact.gain_impact);
}

/** The acknowledgement to send with the command; empty when nothing changes. */
export function gainAcknowledgement(impact: GainImpact | null | undefined): string {
  return hasGainChanges(impact) ? impact.acknowledgement : '';
}

/** The server recomputed the change set at commit and it differs from what
 * the user accepted (or was never accepted): preview again, do not dead-end. */
export function isGainAcknowledgementRefusal(error: unknown): boolean {
  return error instanceof APIClientError && (
    error.code === 'INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_REQUIRED' ||
    error.code === 'INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_STALE'
  );
}
