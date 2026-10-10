// The title a transaction row shows. System-posted journals carry no payee
// or description (built-in values are never English text), only a stable
// system_label code, which the caller localizes (T-136).

export type SystemLabel = 'split_adjustment' | 'transfer_bridge' | 'share_exchange';

export interface TitledTransaction {
  payee_name?: string | null;
  description?: string | null;
  system_label?: SystemLabel | null;
}

export function transactionTitle(
  transaction: TitledTransaction,
  systemLabel: (code: SystemLabel) => string,
  fallback: string
): string {
  if (transaction.payee_name) return transaction.payee_name;
  if (transaction.description) return transaction.description;
  if (transaction.system_label) return systemLabel(transaction.system_label);
  return fallback;
}
