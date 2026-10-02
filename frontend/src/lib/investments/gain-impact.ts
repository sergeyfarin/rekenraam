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

/** True when the preview requires the user to acknowledge changed gains. */
export function hasGainChanges(impact: GainImpact | undefined | null): impact is GainImpact {
  return !!impact && impact.changes.length > 0;
}
