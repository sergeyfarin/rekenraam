/**
 * Provider dividend evidence (#111): the normalized_json schema 1 the backend
 * stages for a per-share announcement. Decimals are exact {value, scale}
 * pairs; null means the source or the ledger did not establish the figure.
 */
export type ProviderDecimal = { value: string; scale: number };

export type ProviderDividendEvidence = {
  symbol: string;
  mic: string;
  exDate: string;
  entitlementDate: string;
  amountPerShare: ProviderDecimal | null;
  listingCurrency: string;
  entitledUnits: ProviderDecimal | null;
  estimatedGross: ProviderDecimal | null;
  missing: string[];
  unsupported: string | null;
};

function decimal(raw: unknown): ProviderDecimal | null {
  if (!raw || typeof raw !== 'object') return null;
  const { value, scale } = raw as { value?: unknown; scale?: unknown };
  if (typeof value !== 'string' || !/^-?\d+$/.test(value) || typeof scale !== 'number' || !Number.isInteger(scale) || scale < 0) {
    return null;
  }
  return { value, scale };
}

function text(raw: unknown): string {
  return typeof raw === 'string' ? raw : '';
}

/** Reads schema-1 evidence; anything else is not a provider dividend. */
export function parseProviderDividendEvidence(raw: Record<string, unknown> | null | undefined): ProviderDividendEvidence | null {
  if (!raw || raw.schema !== 1) return null;
  return {
    symbol: text(raw.symbol),
    mic: text(raw.mic),
    exDate: text(raw.ex_date),
    entitlementDate: text(raw.entitlement_date),
    amountPerShare: decimal(raw.amount_per_share),
    listingCurrency: text(raw.listing_currency),
    entitledUnits: decimal(raw.entitled_units),
    estimatedGross: decimal(raw.estimated_gross),
    missing: Array.isArray(raw.missing) ? raw.missing.filter((code): code is string => typeof code === 'string') : [],
    unsupported: typeof raw.unsupported === 'string' && raw.unsupported !== '' ? raw.unsupported : null
  };
}

export type ProposalShape = 'complete' | 'needs_completion' | 'payment_in_lieu' | 'informational' | 'unsupported';

/** How a suggestion can be acted on, from its proposed_transaction. */
export function proposalShape(proposal: Record<string, unknown> | null | undefined): ProposalShape {
  if (proposal?.kind === 'informational') return 'informational';
  if (proposal?.kind === 'payment_in_lieu') return 'payment_in_lieu';
  if (proposal?.kind === 'dividend_income') return proposal.complete === false ? 'needs_completion' : 'complete';
  return 'unsupported';
}

/**
 * A dividend whose ex-date found the book short only (#255): the owner
 * completes it into a payment in lieu for one of these short holdings.
 */
export type PaymentInLieuProposal = { commodityID: number; exDate: string; holdingAccountIDs: number[] };

export function paymentInLieuProposal(proposal: Record<string, unknown> | null | undefined): PaymentInLieuProposal | null {
  if (proposal?.kind !== 'payment_in_lieu') return null;
  const commodityID = proposalCommodityID(proposal);
  const exDate = typeof proposal.ex_date === 'string' ? proposal.ex_date : '';
  const holdingAccountIDs = Array.isArray(proposal.holding_account_ids)
    ? proposal.holding_account_ids.filter((id): id is number => typeof id === 'number' && Number.isInteger(id) && id > 0) : [];
  if (!commodityID || !exDate || holdingAccountIDs.length === 0) return null;
  return { commodityID, exDate, holdingAccountIDs };
}

export function proposalCommodityID(proposal: Record<string, unknown> | null | undefined): number | undefined {
  const id = proposal?.commodity_id;
  return typeof id === 'number' && Number.isInteger(id) && id > 0 ? id : undefined;
}

/**
 * The estimate may pre-fill the received gross only when it is stated in the
 * cash account's currency and fits that currency's scale exactly. Anything
 * else would invent a conversion or a rounding the owner never confirmed.
 */
export function grossPrefill(
  estimate: ProviderDecimal | null,
  listingCurrency: string,
  cashCurrency: string,
  cashScale: number
): ProviderDecimal | null {
  if (!estimate || !listingCurrency || listingCurrency !== cashCurrency || estimate.scale > cashScale) return null;
  const padding = cashScale - estimate.scale;
  return { value: estimate.value + '0'.repeat(padding), scale: cashScale };
}

/** What the provider itself stated: an amount (null when unreadable) and the listing currency. */
export type ProviderDividendContent = { amountPerShare: ProviderDecimal | null; listingCurrency: string };

export type ProviderDividendRevision = {
  from: ProviderDividendContent;
  to: ProviderDividendContent;
  observedAt: string;
};

/**
 * Provider revisions (#254) as "old → new" steps: the first observation, then
 * each later distinct announcement in the order it was seen. A revision is
 * evidence only; the suggestion keeps the first observation.
 */
export function providerDividendRevisions(
  first: ProviderDividendEvidence | null,
  revisions: ReadonlyArray<{ evidence: Record<string, unknown>; observed_at: string }> | null | undefined
): ProviderDividendRevision[] {
  if (!first || !revisions) return [];
  let previous: ProviderDividendContent = { amountPerShare: first.amountPerShare, listingCurrency: first.listingCurrency };
  const steps: ProviderDividendRevision[] = [];
  for (const revision of revisions) {
    if (revision.evidence?.schema !== 1) continue;
    const next = { amountPerShare: decimal(revision.evidence.amount_per_share), listingCurrency: text(revision.evidence.listing_currency) };
    steps.push({ from: previous, to: next, observedAt: revision.observed_at });
    previous = next;
  }
  return steps;
}
