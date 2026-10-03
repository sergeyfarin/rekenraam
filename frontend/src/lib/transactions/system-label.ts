import { m } from '$lib/paraglide/messages.js';
import type { SystemLabel } from './transaction-title';

// Localized text for a system-posted journal's stable label code (T-136).
export function systemLabelText(code: SystemLabel): string {
  switch (code) {
    case 'split_adjustment':
      return m.transactions_system_label_split_adjustment();
  }
}
