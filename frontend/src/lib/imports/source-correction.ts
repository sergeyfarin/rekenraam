import type { ImportStagedRow } from '$lib/api/imports';

export function sourceCorrectionKind(row: ImportStagedRow): 'buy' | 'sale' | null {
  if (unsupportedSourceFill(row) || !row.source_changed || !row.source_transaction_id ||
    (row.commit_status !== 'pending' && row.commit_status !== 'skipped')) return null;
  try {
    const raw = JSON.parse(row.raw) as { kind?: string; side?: string };
    if (raw?.kind !== 'trading212_order_fill' || typeof raw.side !== 'string') return null;
    const side = raw.side.trim().toUpperCase();
    if (side === 'BUY' && row.source_buy_operation) return 'buy';
    if (side === 'SELL' && row.source_sale_operation) return 'sale';
    return null;
  } catch {
    return null;
  }
}

/** Only documented ordinary executions may enter the trade import path. */
export function unsupportedSourceFill(row: Pick<ImportStagedRow, 'raw'>): boolean {
  try {
    const raw = JSON.parse(row.raw) as { kind?: string; fill_type?: string } | null;
    return raw?.kind === 'trading212_order_fill' && raw.fill_type !== 'TRADE';
  } catch { return false; }
}

/**
 * A pending Trading 212 STOCK_SPLIT fill. Its fill type supplies no ratio or
 * entitlement, so it never posts; the user may link it to a split they
 * recorded, which keeps a re-fetch from posting it again (T-122).
 */
export function linkableSplitFill(row: Pick<ImportStagedRow, 'raw' | 'commit_status' | 'dedupe_status'>): boolean {
  if (row.commit_status === 'committed' || row.dedupe_status === 'duplicate') return false;
  try {
    const raw = JSON.parse(row.raw) as { kind?: string; fill_type?: string } | null;
    return raw?.kind === 'trading212_order_fill' && raw.fill_type === 'STOCK_SPLIT';
  } catch { return false; }
}
