import type { ImportStagedRow } from '$lib/api/imports';

export function sourceCorrectionKind(row: ImportStagedRow): 'buy' | 'sale' | null {
  if (!row.source_changed || !row.source_transaction_id ||
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
