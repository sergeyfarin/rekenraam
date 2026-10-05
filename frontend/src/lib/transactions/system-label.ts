import { m } from '#lib/paraglide/messages.js';
import type { SystemLabel } from './transaction-title';

// Localized text for a system-posted journal's stable label code (T-136):
// a split adjustment, or the cost basis an outbound transfer carries out.
export function systemLabelText(code: SystemLabel): string {
  switch (code) {
    case 'split_adjustment':
      return m.transactions_system_label_split_adjustment();
    case 'transfer_bridge':
      return m.transactions_system_label_transfer_bridge();
  }
}
