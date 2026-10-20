import { mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { expect, repoRoot, test, type Locator, type Page } from './support/fixtures';
import { seedReferenceBook } from './support/reference-book';
import { accessibilityViolations, type AxeViolation } from './support/a11y';
import { apiJSON, csrfTokenFor } from './support/api';
import { withRenderedContrast } from './support/rendered-contrast';
import { ensureBrowserSession } from './support/session';
import { setTrading212Dividends, setTrading212Fills, trading212StubAvailable } from './support/trading212';

// #331 — the app's interactive sub-views, on the #266 reference book.
//
// The #278 route sweep checks every route as it first renders. This sweep
// opens what a route only shows after an action — modal dialogs, the record
// and correction forms, the side panels, the import review sub-views and the
// editors — and, for each, runs axe in the light and dark themes at desktop
// and 390 px and in de and ru, checks that neither the page nor the sub-view
// scrolls sideways and that the sub-view fits the viewport, then closes it and
// checks it is gone. Dialog focus behaviour is #327's (`dialog-focus.spec.ts`).
//
// Each configuration writes its row of the sub-view × check matrix to
// test-results/subview-sweep/, which the dated report summarises.
//
// No sub-view may write: every non-GET request is aborted unless it only
// previews (impact/preview paths) or the sub-view declares the write it needs
// to reach its state (an upload, or a write the server refuses for review).

type Locale = 'en' | 'de' | 'ru';

/** Every catalog of a locale, merged; `t` answers an exact accessible-name pattern. */
function catalog(locale: Locale) {
  const root = path.join(repoRoot, 'frontend/messages');
  const merged: Record<string, string> = {};
  for (const domain of readdirSync(root)) {
    Object.assign(merged, JSON.parse(readFileSync(path.join(root, domain, `${locale}.json`), 'utf8')));
  }
  // `prefix`: a control whose wrapping label also names its current value.
  return (key: string, prefix = false): RegExp => {
    const text = merged[key];
    if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
    const pattern = text.split(/\{[a-zA-Z_]+\}/).map((part) => part.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('.+?');
    return new RegExp(`^\\s*${pattern}${prefix ? '' : '\\s*$'}`);
  };
}

type Fixture = {
  ids: Map<string, number>;
  /** The owner's calendar day as the server sees it. */
  today: string;
  /** Investment transactions by transaction date. */
  investment: Map<string, number[]>;
  /** Ordinary book transactions by description. */
  ordinary: Map<string, number>;
  /** A position whose first sale reverses, and first buy corrects, into a gain review. */
  review: { firstBuy: number; firstSale: number };
  importConnection?: string;
  /** An owned position with 24-place quantities, rather than the first shared row. */
  lotInstrument: string;
};

type Context = { page: Page; t: (key: string, prefix?: boolean) => RegExp; fx: Fixture };

type SubView = {
  name: string;
  group: 'dialog' | 'record form' | 'correction' | 'panel' | 'import review' | 'editor';
  path: (fx: Fixture) => string;
  /** Opens the sub-view and returns its container. */
  open: (c: Context) => Promise<Locator>;
  /** Closes it; dialogs default to Escape. `null`: an inline state with nothing to close. */
  close?: ((c: Context, view: Locator) => Promise<void>) | null;
  writes?: RegExp;
  /** Set when the sub-view cannot be reached on this fixture, with why. */
  unreachable?: string;
};

const openDialog = (page: Page) => page.locator('dialog[open]').last();
const detailPanel = (c: Context) => c.page.getByRole('complementary', { name: c.t('transactions_detail_actions') });

async function clickAndDialog(c: Context, name: string, within?: Locator): Promise<Locator> {
  await (within ?? c.page).getByRole('button', { name: c.t(name) }).first().click();
  const dialog = openDialog(c.page);
  await expect(dialog).toBeVisible();
  await c.page.waitForLoadState('networkidle');
  return dialog;
}

/** Shows the reference instrument's dollar price history. */
async function pricingPair(c: Context) {
  await c.page.locator('#pricing-history-base').selectOption(String(c.fx.ids.get('REF')));
  await c.page.locator('#pricing-history-quote').selectOption(String(c.fx.ids.get('USD')));
  await c.page.waitForLoadState('networkidle');
}

/** The transaction detail panel of the first of `ids` that offers `action`. */
async function transactionWith(c: Context, ids: number[], action: string): Promise<void> {
  for (const id of ids) {
    await c.page.goto(`/app/transactions?transaction_id=${id}`);
    await settle(c.page);
    const button = detailPanel(c).getByRole('button', { name: c.t(action) });
    if (await button.first().waitFor({ state: 'visible', timeout: 5_000 }).then(() => true, () => false)) return;
  }
  throw new Error(`no transaction among ${ids.join(', ')} offers ${action}`);
}

function investmentOn(fx: Fixture, ...dates: string[]): number[] {
  return dates.flatMap((date) => fx.investment.get(date) ?? []);
}

function correction(name: string, dates: string[], action: string): SubView {
  return {
    name, group: 'correction', path: () => '/app/transactions',
    open: async (c) => {
      await transactionWith(c, investmentOn(c.fx, ...dates), action);
      return clickAndDialog(c, action, detailPanel(c));
    }
  };
}

const recordForms: Array<[string, string]> = [
  ['buy', 'investments_record_buy'],
  ['sell', 'investments_record_sell'],
  ['short sale', 'investments_record_short_sale'],
  ['short cover', 'investments_record_short_cover'],
  ['payment in lieu', 'investments_record_payment_in_lieu'],
  ['dividend', 'investments_record_dividend'],
  ['withholding', 'investments_record_withholding'],
  ['reinvested dividend', 'investments_record_reinvested'],
  ['transfer in', 'investments_record_external_transfer_in'],
  ['move between holdings', 'investments_record_internal_transfer'],
  ['transfer out', 'investments_record_external_transfer_out'],
  ['split', 'investments_record_split'],
  ['share exchange', 'investments_record_share_exchange'],
  ['cash-and-stock merger', 'investments_record_merger'],
  ['spin-off', 'investments_record_spin_off'],
  ['return of capital', 'investments_record_capital_return'],
  ['write-off', 'investments_record_write_off']
];

const subViews: SubView[] = [
  // ── Transaction lifecycle and reconciliation dialogs ──────────────────
  {
    name: 'void reason', group: 'dialog', path: () => '/app/transactions',
    open: async (c) => { await transactionWith(c, [c.fx.ordinary.get('Groceries')!], 'transactions_panel_void'); return clickAndDialog(c, 'transactions_panel_void', detailPanel(c)); }
  },
  {
    name: 'unvoid reason', group: 'dialog', path: () => '/app/transactions',
    open: async (c) => { await transactionWith(c, [c.fx.ids.get('t13')!], 'transactions_panel_unvoid'); return clickAndDialog(c, 'transactions_panel_unvoid', detailPanel(c)); }
  },
  {
    name: 'delete reason', group: 'dialog', path: () => '/app/transactions',
    open: async (c) => { await transactionWith(c, [c.fx.ordinary.get('Groceries')!], 'transactions_panel_soft_delete'); return clickAndDialog(c, 'transactions_panel_soft_delete', detailPanel(c)); }
  },
  {
    name: 'restore reason (detail panel)', group: 'dialog', path: () => '/app/transactions',
    open: async () => { throw new Error('unreachable'); },
    unreachable: 'transaction-detail-panel keeps a `restore` reason branch that nothing opens: a deleted transaction is not served by the ordinary read, and restore lives in Settings → Trash'
  },
  {
    name: 'void reconciliation override', group: 'dialog', path: () => '/app/transactions',
    // The void itself is refused by the reconciled period; that refusal opens the review.
    writes: /\/transactions\/\d+\/void$/,
    open: async (c) => {
      const reasonDialog = await (async () => { await transactionWith(c, [c.fx.ordinary.get('Groceries')!], 'transactions_panel_void'); return clickAndDialog(c, 'transactions_panel_void', detailPanel(c)); })();
      await reasonDialog.getByRole('textbox').fill('sweep');
      await reasonDialog.getByRole('button', { name: c.t('transactions_void_confirm') }).click();
      const warning = c.page.getByRole('alertdialog');
      await expect(warning.getByRole('listitem').first()).toBeVisible();
      return warning;
    }
  },
  {
    name: 'edit reconciliation override', group: 'dialog', path: () => '/app/transactions',
    writes: /\/transactions\/\d+$/,
    open: async (c) => {
      await transactionWith(c, [c.fx.ordinary.get('Groceries')!], 'transactions_panel_edit');
      await detailPanel(c).getByRole('button', { name: c.t('transactions_panel_edit') }).click();
      const date = c.page.getByLabel(c.t('transactions_field_date'));
      await date.fill('2026-01-09');
      await c.page.getByRole('button', { name: c.t('transactions_form_save') }).click();
      const warning = c.page.getByRole('alertdialog');
      await expect(warning.getByRole('listitem').first()).toBeVisible();
      return warning;
    }
  },
  {
    name: 'checkpoint void', group: 'dialog', path: () => '/app/reconcile',
    open: async (c) => {
      await c.page.locator('#reconcile-account').selectOption(String(c.fx.ids.get('checking')));
      await c.page.getByRole('button', { name: c.t('reconcile_checkpoint_void_aria') }).first().click();
      const dialog = openDialog(c.page);
      await expect(dialog).toBeVisible();
      return dialog;
    }
  },
  {
    name: 'trash restore override', group: 'dialog', path: () => '/app/settings/trash',
    open: async (c) => {
      // Phone rows repeat the action for the stacked layout; take the visible one.
      await c.page.getByRole('row').filter({ hasText: 'Sweep restore subject' }).getByRole('button', { name: c.t('trash_restore_guarded') }).filter({ visible: true }).first().click();
      const dialog = openDialog(c.page);
      await expect(dialog.getByRole('listitem').first()).toBeVisible();
      return dialog;
    }
  },
  {
    name: 'add currency', group: 'dialog', path: () => '/app/settings/currencies',
    open: (c) => clickAndDialog(c, 'currencies_add')
  },
  {
    name: 'account currency picker', group: 'dialog', path: () => '/app/accounts',
    open: async (c) => {
      await c.page.getByRole('button', { name: c.t('accounts_add_account') }).first().click();
      await c.page.getByRole('combobox', { name: c.t('accounts_field_currency', true) }).selectOption('__other_currency__');
      const dialog = openDialog(c.page);
      await expect(dialog).toBeVisible();
      return dialog;
    }
  },
  {
    name: 'price form (add)', group: 'dialog', path: () => '/app/pricing',
    open: (c) => clickAndDialog(c, 'pricing_add')
  },
  {
    name: 'price form (correct)', group: 'dialog', path: () => '/app/pricing',
    open: async (c) => { await pricingPair(c); return clickAndDialog(c, 'pricing_correct'); }
  },
  {
    name: 'price form (withdraw)', group: 'dialog', path: () => '/app/pricing',
    open: async (c) => { await pricingPair(c); return clickAndDialog(c, 'pricing_void'); }
  },
  {
    name: 'recurring post review', group: 'dialog', path: () => '/app/recurring',
    open: (c) => clickAndDialog(c, 'recurring_post')
  },
  {
    name: 'recurring draft edit', group: 'dialog', path: () => '/app/recurring',
    open: (c) => clickAndDialog(c, 'recurring_edit_draft')
  },
  {
    name: 'recurring discard', group: 'dialog', path: () => '/app/recurring',
    open: (c) => clickAndDialog(c, 'recurring_discard')
  },
  {
    name: 'recurring template editor', group: 'editor', path: () => '/app/recurring',
    open: (c) => clickAndDialog(c, 'recurring_new_template')
  },
  {
    name: 'split activity: correct', group: 'correction', path: () => '/app/investments?tab=splits',
    open: (c) => clickAndDialog(c, 'transactions_investment_replace_split_action', c.page.getByRole('article'))
  },
  {
    name: 'split activity: reverse', group: 'correction', path: () => '/app/investments?tab=splits',
    open: (c) => clickAndDialog(c, 'transactions_investment_reverse_split_action', c.page.getByRole('article'))
  },
  {
    // The book's later FIFO sale depends on the split's units, so the server
    // may refuse instead of asking for review; either answer is the sub-view.
    name: 'split activity: reverse submitted', group: 'correction', path: () => '/app/investments?tab=splits',
    open: async (c) => {
      const dialog = await clickAndDialog(c, 'transactions_investment_reverse_split_action', c.page.getByRole('article'));
      await dialog.getByRole('textbox').fill('sweep');
      await dialog.getByRole('button', { name: c.t('transactions_investment_reverse_confirm') }).click();
      await expect(c.page.getByRole('alertdialog').or(dialog.getByRole('alert')).first()).toBeVisible();
      return openDialog(c.page);
    }
  },
  {
    name: 'split activity: cash in lieu', group: 'correction', path: () => '/app/investments?tab=splits',
    open: (c) => clickAndDialog(c, 'investments_cash_in_lieu_title', c.page.getByRole('article'))
  },

  // ── Record forms (investment entry) ──────────────────────────────────
  ...recordForms.map(([name, key]): SubView => ({
    name: `record ${name}`, group: 'record form', path: () => '/app/investments',
    open: (c) => clickAndDialog(c, key)
  })),

  // ── Investment corrections on the book's operations ──────────────────
  correction('correct buy', ['2026-02-17'], 'transactions_investment_replace_buy_action'),
  correction('reverse buy', ['2026-02-17'], 'transactions_investment_reverse_buy_action'),
  {
    // Every book buy has a dependant, so its reversal is refused; the review
    // position's first sale is not, and reversing it moves the second sale's lots.
    name: 'reverse sale: gain review', group: 'correction', path: () => '/app/transactions',
    open: async (c) => {
      await transactionWith(c, [c.fx.review.firstSale], 'transactions_investment_reverse_action');
      const dialog = await clickAndDialog(c, 'transactions_investment_reverse_action', detailPanel(c));
      await dialog.getByRole('textbox').fill('sweep');
      await dialog.getByRole('button', { name: c.t('transactions_investment_reverse_confirm') }).click();
      await expect(dialog.getByRole('textbox')).toHaveCount(0);
      return dialog;
    }
  },
  {
    name: 'correct buy: reconciliation confirm', group: 'correction', path: () => '/app/transactions',
    open: async (c) => {
      await transactionWith(c, [c.fx.review.firstBuy], 'transactions_investment_replace_buy_action');
      const dialog = await clickAndDialog(c, 'transactions_investment_replace_buy_action', detailPanel(c));
      await dialog.locator('#buy-correction-reason').fill('sweep');
      await dialog.locator('#buy-cash-amount').fill('800');
      await dialog.getByRole('button', { name: c.t('transactions_investment_replace_submit') }).click();
      const review = c.page.getByRole('alertdialog');
      await expect(review).toBeVisible();
      return review;
    }
  },
  correction('correct sale', ['2026-02-23'], 'transactions_investment_replace_sale_action'),
  correction('reverse sale', ['2026-02-23'], 'transactions_investment_reverse_action'),
  correction('correct dividend', ['2026-02-20'], 'transactions_investment_replace_dividend_action'),
  correction('correct split', ['2026-02-16'], 'transactions_investment_replace_split_action'),
  correction('correct transfer in', ['2026-02-19'], 'transactions_investment_replace_transfer_action'),
  correction('resolve unknown basis', ['2026-02-19'], 'transactions_investment_resolve_basis_action'),
  correction('correct short sale', ['2026-03-02'], 'investments_short_sale_correct_action'),
  correction('correct short cover', ['2026-03-16'], 'investments_short_cover_correct_action'),
  correction('correct return of capital', ['2026-03-02', '2026-03-05'], 'investments_capital_return_correct'),
  correction('correct payment in lieu', ['2026-03-12'], 'investments_payment_in_lieu_correct_action'),
  ...([
    ['correct write-off', 'no write-off in the reference book; its form is the record write-off form plus write-off-correction-form, checked by investments-write-off-correction.spec.ts'],
    ['correct share exchange / spin-off / merger', 'no corporate action in the reference book; the correction dialogs render the same forms as the record forms swept above'],
    ['correct cash in lieu', 'no settled cash in lieu in the reference book; the dialog renders the cash-in-lieu form swept under split activity'],
    ['correct resolved basis', 'the reference book leaves G1 unresolved; resolving it would change every later configuration']
  ] as const).map(([name, why]): SubView => ({
    name, group: 'correction', path: () => '/app/transactions', open: async () => { throw new Error(why); }, unreachable: why
  })),

  // ── Side panels ──────────────────────────────────────────────────────
  {
    name: 'transaction detail panel', group: 'panel', path: () => '/app/transactions',
    open: async (c) => {
      await transactionWith(c, [c.fx.ordinary.get('Groceries')!], 'transactions_panel_void');
      return c.page.getByRole('complementary', { name: c.t('transactions_detail_actions') });
    },
    close: async (c) => { await c.page.getByRole('button', { name: c.t('transactions_detail_close') }).click(); }
  },
  {
    name: 'investment detail panel', group: 'panel', path: () => '/app/transactions',
    open: async (c) => {
      await transactionWith(c, investmentOn(c.fx, '2026-02-23'), 'transactions_investment_replace_sale_action');
      return c.page.getByRole('complementary', { name: c.t('transactions_detail_actions') });
    },
    close: async (c) => { await c.page.getByRole('button', { name: c.t('transactions_detail_close') }).click(); }
  },
  {
    name: 'transaction editor', group: 'panel', path: () => '/app/transactions',
    open: async (c) => {
      await transactionWith(c, [c.fx.ordinary.get('Card split')!], 'transactions_panel_edit');
      await detailPanel(c).getByRole('button', { name: c.t('transactions_panel_edit') }).click();
      // Cancel returns the panel to the transaction's detail, so the editor's form is the sub-view.
      const editor = detailPanel(c).locator('form').filter({ has: c.page.getByRole('button', { name: c.t('transactions_form_save') }) });
      await expect(editor).toBeVisible();
      return editor;
    },
    close: async (c, view) => { await view.getByRole('button', { name: c.t('transactions_form_cancel') }).first().click(); }
  },
  {
    name: 'lot panel', group: 'panel', path: () => '/app/investments',
    open: async (c) => {
      await c.page.locator('tbody tr[role="button"]').filter({ hasText: c.fx.lotInstrument }).click();
      const panel = c.page.getByRole('complementary', { name: c.t('investments_lots_detail_label') });
      await expect(panel).toBeVisible();
      await c.page.waitForLoadState('networkidle');
      return panel;
    },
    close: async (c) => { await c.page.getByRole('button', { name: c.t('investments_lots_close') }).click(); }
  },

  // ── Editors ──────────────────────────────────────────────────────────
  {
    name: 'account editor (new)', group: 'editor', path: () => '/app/accounts',
    open: async (c) => {
      await c.page.getByRole('button', { name: c.t('accounts_add_account') }).first().click();
      return c.page.locator('main form').first();
    },
    close: async (c, view) => { await view.getByRole('button', { name: c.t('accounts_form_cancel') }).first().click(); }
  },
  {
    name: 'institution editor (new)', group: 'editor', path: () => '/app/accounts',
    open: async (c) => {
      await c.page.getByRole('button', { name: c.t('institutions_add_institution') }).first().click();
      return c.page.locator('main form').first();
    },
    close: async (c, view) => { await view.getByRole('button', { name: c.t('accounts_form_cancel') }).first().click(); }
  },
  {
    name: 'category editor (new)', group: 'editor', path: () => '/app/categories',
    open: async (c) => {
      await c.page.getByRole('button', { name: c.t('categories_add_category') }).first().click();
      return c.page.locator('main form').first();
    },
    close: async (c, view) => { await view.getByRole('button', { name: c.t('categories_form_cancel') }).first().click(); }
  },
  {
    name: 'budget target: invalid amount', group: 'editor', path: () => '/app/budgets',
    writes: /\/budgets\//,
    open: async (c) => {
      const target = c.page.getByRole('textbox', { name: c.t('budgets_target_for') }).first();
      await target.fill('abc');
      await target.press('Enter');
      await expect(c.page.getByRole('alert').first()).toBeVisible();
      return c.page.locator('main');
    },
    close: null
  },

  // ── Import review ────────────────────────────────────────────────────
  {
    name: 'staged CSV preview with payee resolution', group: 'import review', path: () => '/app/import',
    writes: /\/imports(\/|$)|\/import-profiles/,
    open: async (c) => {
      await uploadCSV(c, `Sweep payee ${Date.now()}`);
      await expect(c.page.getByRole('heading', { name: c.t('import_payees_title') })).toBeVisible();
      return c.page.locator('main');
    },
    close: discardImport
  },
  {
    name: 'staged CSV preview with a rejected row', group: 'import review', path: () => '/app/import',
    writes: /\/imports(\/|$)|\/import-profiles/,
    open: async (c) => {
      await uploadCSV(c, `Sweep reject ${Date.now()}`, '-12.50');
      await expect(c.page.getByRole('button', { name: c.t('import_commit_button') })).toBeVisible();
      return c.page.locator('main');
    },
    close: discardImport
  },
  {
    // One row left without an account fails at commit; its review shows only that row.
    name: 'failed-row review', group: 'import review', path: () => '/app/import',
    writes: /\/imports(\/|$)|\/import-profiles|\/payees/,
    open: async (c) => {
      const stamp = Date.now();
      await uploadCSV(c, `Sweep kept ${stamp}`, undefined, `Sweep failed ${stamp}`);
      await c.page.getByRole('combobox', { name: c.t('import_preview_account_label') }).selectOption(String(c.fx.ids.get('savings')));
      await c.page.getByRole('combobox', { name: c.t('import_preview_currency_label') }).selectOption(String(c.fx.ids.get('EUR')));
      await c.page.getByRole('combobox', { name: c.t('import_preview_category_label') }).selectOption({ index: 1 });
      await c.page.getByRole('button', { name: c.t('import_preview_apply_all') }).click();
      await c.page.getByRole('row', { name: new RegExp(`Sweep failed ${stamp}`) }).locator('select').first().selectOption('');
      await c.page.getByRole('button', { name: c.t('import_commit_button') }).click();
      await c.page.getByRole('button', { name: c.t('import_result_review_remaining') }).click();
      await expect(c.page.getByRole('row', { name: new RegExp(`Sweep failed ${stamp}`) }).locator('select').first()).toBeVisible();
      return c.page.locator('main');
    },
    close: async (c) => {
      // Fixing the row commits it, so the next configuration starts from an empty import.
      await c.page.getByRole('row', { name: /Sweep failed/ }).locator('select').first().selectOption(String(c.fx.ids.get('savings')));
      await c.page.getByRole('button', { name: c.t('import_commit_button') }).click();
      await expect(c.page.getByRole('button', { name: c.t('import_result_import_another') })).toBeVisible();
    }
  },
  {
    name: 'provider preview: short link', group: 'import review', path: () => '/app/import',
    writes: /\/import-connections\/\d+\/refresh|\/imports(\/|$)/,
    open: async (c) => {
      await fetchProvider(c);
      await c.page.getByRole('button', { name: c.t('import_preview_short_link') }).click();
      await expect(c.page.getByText(c.t('import_preview_short_link_source'))).toBeVisible();
      return c.page.locator('main');
    },
    close: discardImport
  },
  {
    name: 'provider preview: split link', group: 'import review', path: () => '/app/import',
    writes: /\/import-connections\/\d+\/refresh|\/imports(\/|$)/,
    open: async (c) => {
      await fetchProvider(c);
      await c.page.getByRole('button', { name: c.t('import_preview_split_link') }).click();
      await expect(c.page.getByText(c.t('import_preview_split_link_choose')).first()).toBeVisible();
      return c.page.locator('main');
    },
    close: discardImport
  },
  {
    name: 'pending dividend review', group: 'import review', path: () => '/app/import/dividends',
    open: async (c) => {
      await expect(c.page.getByRole('heading', { name: c.t('import_dividend_pending') }).first()).toBeVisible();
      return c.page.locator('main');
    },
    close: null
  }
];

function todayDMY(fx: Fixture): string {
  const [year, month, day] = fx.today.split('-');
  return `${day}/${month}/${year}`;
}

async function uploadCSV(c: Context, payee: string, rejected?: string, secondPayee?: string) {
  const rows = [`${todayDMY(c.fx)};${payee};-12,34`, ...(rejected ? [`${todayDMY(c.fx)};${payee};${rejected}`] : []),
    ...(secondPayee ? [`${todayDMY(c.fx)};${secondPayee};-5,00`] : [])];
  await c.page.locator('input[type="file"]').setInputFiles({
    name: 'sweep.csv', mimeType: 'text/csv', buffer: Buffer.from(`Datum;Omschrijving;Bedrag\n${rows.join('\n')}\n`)
  });
  // The first upload saves a mapping that later uploads reuse.
  const mappingName = c.page.getByLabel(c.t('import_csv_profile_name'));
  const button = c.page.getByRole('button', { name: c.t('import_upload_submit') });
  // A new file shows the mapping-name field; a recognised one preselects its saved mapping.
  const saved = c.page.getByRole('combobox', { name: c.t('import_csv_saved_profile', true) });
  await expect(mappingName.or(saved).first()).toBeVisible();
  if (await mappingName.isVisible()) await mappingName.fill(`Sweep ${Date.now()}`);
  await expect(button).toBeEnabled();
  await button.click();
  await c.page.waitForLoadState('networkidle');
}

async function fetchProvider(c: Context) {
  if (!c.fx.importConnection) throw new Error('the Trading 212 stub is not running');
  await c.page.getByRole('row').filter({ hasText: c.fx.importConnection }).getByRole('button').first().click();
  await expect(c.page.getByRole('button', { name: c.t('import_commit_button') })).toBeVisible({ timeout: 15_000 });
}

async function discardImport(c: Context) {
  await c.page.getByRole('button', { name: c.t('import_discard_button') }).click();
  // The confirmation's button repeats the label in some locales.
  await c.page.getByRole('button', { name: c.t('import_discard_confirm_button') }).last().click();
  await expect(c.page.getByRole('button', { name: c.t('import_commit_button') })).toHaveCount(0);
}

async function settle(page: Page) {
  await expect(page.locator('main h1, main h2').first()).toBeVisible({ timeout: 15_000 });
  await page.waitForLoadState('networkidle');
}

function describeViolations(violations: AxeViolation[]): string {
  return violations.map((v) => `[${v.impact ?? 'unknown'}] ${v.id}: ${v.targets.join(' | ')}${v.summary ? ` — ${v.summary}` : ''}`).join('; ');
}

/** Book fixtures beyond the reference book: a statement over a deleted transaction, and a provider import. */
async function seedFixture(page: Page): Promise<Fixture> {
  const { ids, transactions } = await seedReferenceBook(page);
  const csrf = await csrfTokenFor(page);
  // The server's calendar day, not the browser's: they differ around midnight
  // when the owner's time zone is not the machine's.
  const today = (await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session')).owner_today;
  const spend = (description: string, cents: number) =>
    apiJSON<{ id: number }>(page, 'POST', '/api/v1/transactions', csrf, {
      transaction_date: today, status: 'posted', description,
      journal_entries: [{ entry_date: today, postings: [
        { account_id: ids.get('checking'), commodity_id: ids.get('EUR'), quantity_value: String(-cents), quantity_scale: 2 },
        { account_id: ids.get('groceries'), commodity_id: ids.get('EUR'), quantity_value: String(cents), quantity_scale: 2 }
      ] }]
    });
  // A deleted transaction under a later statement: its restore needs an override.
  // Same-day statements cover the day up to their last posting, so the
  // subject comes first.
  const restoreSubject = await spend('Sweep restore subject', 700);
  await apiJSON(page, 'POST', `/api/v1/transactions/${restoreSubject.id}/soft-delete`, csrf, { change_reason: 'sweep' });
  await spend('Sweep kept', 500);
  // The statement balance is the session's own starting balance plus every
  // candidate, so it holds whatever calendar day the server calls today.
  type Quantity = { quantity_value: string; quantity_scale: number };
  type Session = { id: number; starting_balance: Quantity; candidates: Array<Quantity & { posting_id: number }> };
  const start = (balance: Quantity) => apiJSON<Session>(page, 'POST', `/api/v1/accounts/${ids.get('checking')}/reconciliations/start`, csrf, {
    commodity_id: ids.get('EUR'), source_kind: 'statement', statement_date: today,
    statement_balance_value: balance.quantity_value, statement_balance_scale: balance.quantity_scale
  });
  const probe = await start({ quantity_value: '0', quantity_scale: 2 });
  const scale = Math.max(probe.starting_balance.quantity_scale, ...probe.candidates.map((q) => q.quantity_scale));
  const scaled = (q: Quantity) => BigInt(q.quantity_value) * 10n ** BigInt(scale - q.quantity_scale);
  const total = probe.candidates.reduce((sum, q) => sum + scaled(q), scaled(probe.starting_balance));
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${probe.id}/void`, csrf, { change_reason: 'sweep probe' });
  const session = await start({ quantity_value: total.toString(), quantity_scale: scale });
  await apiJSON(page, 'PATCH', `/api/v1/reconciliations/${session.id}`, csrf, {
    posting_version_ids: session.candidates.map((candidate) => candidate.posting_id), change_reason: 'sweep'
  });
  await apiJSON(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, csrf, { change_reason: 'sweep' });
  const deleted = await apiJSON<{ transactions: Array<{ id: number; restore_blocked_by_reconciliation?: boolean }> }>(
    page, 'GET', '/api/v1/transactions/deleted');
  const blocked = deleted.transactions.find((row) => row.id === restoreSubject.id)?.restore_blocked_by_reconciliation;
  if (!blocked) throw new Error('the restore subject is not guarded by the new statement');

  // The replay's write responses own these IDs. A shared register page can
  // omit all our historical dates, or contain another replay on the same day.
  const investment = new Map<string, number[]>();
  for (const tx of transactions.investment) {
    investment.set(tx.transaction_date, [...(investment.get(tx.transaction_date) ?? []), tx.id]);
  }
  const ordinary = new Map<string, number>();
  for (const tx of transactions.ordinary) if (tx.description) ordinary.set(tx.description, tx.id);
  const lotInstrument = await seedPreciseLot(page, csrf, ids);
  // Long option text caused the bulk-assignment controls to outgrow their
  // phone container (#344), even when the selected option was still blank.
  await apiJSON(page, 'POST', '/api/v1/accounts', csrf, {
    name: `Sweep long account for import assignment ${Date.now()}`,
    account_class: 'asset', account_kind: 'cash', default_commodity_id: ids.get('EUR'),
    allows_postings: true, opened_on: '2026-01-01', effective_from: '2026-01-01'
  });

  let importConnection: string | undefined;
  if (trading212StubAvailable()) importConnection = await seedProviderImport(page, csrf, ids, today);
  const review = await seedReviewPosition(page, csrf, ids);
  return { ids, today, investment, ordinary, review, importConnection, lotInstrument };
}

/** Exact crypto quantities exposed the desktop lot card overflow (#344). */
async function seedPreciseLot(page: Page, csrf: string, ids: Map<string, number>): Promise<string> {
  const suffix = `SWC${Date.now()}`;
  const name = `Sweep precise crypto ${suffix}`;
  const opened = '2026-04-01';
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrf, {
    commodity_code: suffix, instrument_type: 'crypto', display_name: name, symbol: suffix,
    quote_commodity_id: ids.get('USD'), quantity_scale: 24, price_scale: 6, effective_from: opened
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrf, {
    instrument_id: instrument.id, name: `Sweep precise holding ${suffix}`, opened_on: opened, effective_from: opened
  });
  await apiJSON(page, 'POST', '/api/v1/investments/buy', csrf, {
    transaction_date: opened, commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: ids.get('usd_checking'), quantity_value: '1000000000000000000000001', quantity_scale: 24,
    cash_amount_value: '10000', cash_amount_scale: 2, cash_commodity_id: ids.get('USD'), cost_basis_method: 'fifo'
  });
  return name;
}

/**
 * FIFO: 10 at 100.00 and 10 at 200.00, then sales of 8 and 4. The second sale
 * takes 2 from each lot; without the first sale it takes 4 from the first
 * lot, and a cheaper first lot changes the first sale's basis.
 */
async function seedReviewPosition(page: Page, csrf: string, ids: Map<string, number>) {
  const suffix = `SWR${Date.now()}`;
  const usd = ids.get('USD')!;
  const opened = '2026-04-01';
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrf, {
    commodity_code: suffix, instrument_type: 'stock', display_name: `Sweep review ${suffix}`, symbol: suffix,
    quote_commodity_id: usd, trading_commodity_id: usd, quantity_scale: 0, price_scale: 2, effective_from: opened
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrf, {
    instrument_id: instrument.id, name: `Sweep review holding ${suffix}`, opened_on: opened, effective_from: opened
  });
  const trade = (side: 'buy' | 'sell', date: string, quantity: string, cents: string) =>
    apiJSON<{ transaction: { id: number } }>(page, 'POST', `/api/v1/investments/${side}`, csrf, {
      transaction_date: date, commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      cash_account_id: ids.get('usd_checking'), quantity_value: quantity, quantity_scale: 0, cash_amount_value: cents,
      cash_amount_scale: 2, cash_commodity_id: usd, cost_basis_method: 'fifo'
    });
  const firstBuy = await trade('buy', '2026-04-02', '10', '100000');
  await trade('buy', '2026-04-03', '10', '200000');
  const firstSale = await trade('sell', '2026-04-06', '8', '240000');
  await trade('sell', '2026-04-07', '4', '120000');
  return { firstBuy: firstBuy.transaction.id, firstSale: firstSale.transaction.id };
}

/**
 * A provider connection whose fetch stages a synthetic short fill and a
 * STOCK_SPLIT fill (each with a recorded operation to link to), and a
 * committed dividend that waits in review.
 */
async function seedProviderImport(page: Page, csrf: string, ids: Map<string, number>, day: string): Promise<string> {
  const suffix = `SWP${Date.now()}`;
  const ticker = `${suffix}_US_EQ`;
  const usd = ids.get('USD')!;
  // A fixed past day holds at any hour in any runner zone.
  const opened = '2026-04-01';
  const cash = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrf, {
    name: `Sweep broker cash ${suffix}`, account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: usd, allows_postings: true, opened_on: opened, effective_from: opened
  });
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrf, {
    commodity_code: ticker, instrument_type: 'stock', display_name: `Sweep ${suffix}`, symbol: ticker,
    quote_commodity_id: usd, trading_commodity_id: usd, quantity_scale: 0, price_scale: 2, effective_from: opened
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrf, {
    instrument_id: instrument.id, name: `Sweep holding ${suffix}`, opened_on: opened, effective_from: opened
  });
  const longHolding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrf, {
    instrument_id: instrument.id, name: `Sweep long holding ${suffix}`, opened_on: opened, effective_from: opened
  });
  await apiJSON(page, 'POST', '/api/v1/investments/short-sale', csrf, {
    transaction_date: day, commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, quantity_value: '10', quantity_scale: 0, cash_amount_value: '100000',
    cash_amount_scale: 2, cash_commodity_id: usd
  });
  await apiJSON(page, 'POST', '/api/v1/investments/buy', csrf, {
    transaction_date: day, commodity_id: instrument.commodity_id, holding_account_id: longHolding.id,
    cash_account_id: cash.id, quantity_value: '4', quantity_scale: 0, cash_amount_value: '40000',
    cash_amount_scale: 2, cash_commodity_id: usd, cost_basis_method: 'fifo'
  });
  await apiJSON(page, 'POST', '/api/v1/investments/splits', csrf, {
    effective_on: day, holding_account_id: longHolding.id, commodity_id: instrument.commodity_id,
    ratio_numerator: 2, ratio_denominator: 1, change_reason: 'sweep'
  });
  const stubInstrument = { ticker, isin: '', currency: 'USD' };
  const dividendKey = `sweep-dividend-${suffix}`;
  await setTrading212Dividends(dividendKey, stubInstrument, [{ reference: `dividend-${suffix}`, amount: 12, date: day }]);
  const dividendConnection = `Sweep dividends ${suffix}`;
  await apiJSON(page, 'POST', '/api/v1/import-connections', csrf, {
    source: 'trading212', display_name: dividendConnection, api_key: dividendKey, cash_account_id: cash.id
  });
  const fillKey = `sweep-fills-${suffix}`;
  await setTrading212Fills(fillKey, stubInstrument, [
    { id: 1, side: 'SELL', quantity: '10', price: '100', netValue: '1000', date: day, type: 'SYNTHETIC_SHORT' },
    { id: 2, side: 'BUY', quantity: '4', price: '0', netValue: '0', date: day, type: 'STOCK_SPLIT' }
  ]);
  const fillConnection = `Sweep fills ${suffix}`;
  await apiJSON(page, 'POST', '/api/v1/import-connections', csrf, {
    source: 'trading212', display_name: fillConnection, api_key: fillKey, cash_account_id: cash.id
  });
  // The dividend batch is committed now, so it waits in review for every configuration.
  await page.goto('/app/import');
  await page.getByRole('row').filter({ hasText: dividendConnection }).getByRole('button', { name: 'Import', exact: true }).click();
  await page.getByRole('button', { name: 'Commit to ledger' }).click();
  await expect(page.getByText('1 dividend payments remain in review.')).toBeVisible({ timeout: 15_000 });
  return fillConnection;
}

const configurations = [
  { name: 'light desktop', theme: 'light', locale: 'en', viewport: { width: 1280, height: 900 } },
  { name: 'light 390', theme: 'light', locale: 'en', viewport: { width: 390, height: 844 } },
  { name: 'dark desktop', theme: 'dark', locale: 'en', viewport: { width: 1280, height: 900 } },
  { name: 'dark 390', theme: 'dark', locale: 'en', viewport: { width: 390, height: 844 } },
  { name: 'de desktop', theme: 'light', locale: 'de', viewport: { width: 1280, height: 900 } },
  { name: 'de 390', theme: 'light', locale: 'de', viewport: { width: 390, height: 844 } },
  { name: 'ru desktop', theme: 'light', locale: 'ru', viewport: { width: 1280, height: 900 } },
  { name: 'ru 390', theme: 'light', locale: 'ru', viewport: { width: 390, height: 844 } }
] as const;

type Cell = { axe: string; fits: string; closes: string; note?: string };

test.describe('sub-view sweep (#331)', () => {
  test.describe.configure({ mode: 'parallel' });
  let fx: Fixture;

  test.beforeAll(async ({ browser, backend }) => {
    test.setTimeout(240_000);
    const page = await browser.newPage({ baseURL: backend.url });
    fx = await seedFixture(page);
    await page.close();
  });

  for (const config of configurations) {
    test(`every sub-view passes axe, fits and closes: ${config.name}`, async ({ page }) => {
      test.setTimeout(1_200_000);
      const t = catalog(config.locale);
      // One stuck sub-view must not eat the whole run.
      page.setDefaultTimeout(10_000);
      await ensureBrowserSession(page);
      await page.evaluate(({ theme, locale }) => {
        window.localStorage.setItem('rekenraam-theme', theme);
        window.localStorage.setItem('PARAGLIDE_LOCALE', locale);
      }, config);
      await page.setViewportSize(config.viewport);
      let writes: RegExp | undefined;
      await page.route('**/api/v1/**', async (route) => {
        const request = route.request();
        const url = new URL(request.url()).pathname;
        if (request.method() === 'GET' || /impact|preview/.test(url) || writes?.test(url)) return route.continue();
        await route.abort();
      });

      const matrix: Record<string, Cell | string> = {};
      const failures: string[] = [];
      const c: Context = { page, t, fx };
      const only = process.env.SWEEP_ONLY ? new RegExp(process.env.SWEEP_ONLY) : undefined;
      for (const view of subViews.filter((v) => !only || only.test(v.name))) {
        if (view.unreachable) { matrix[view.name] = `not reachable: ${view.unreachable}`; continue; }
        const cell: Cell = { axe: '—', fits: '—', closes: '—' };
        matrix[view.name] = cell;
        writes = view.writes;
        try {
          await page.goto(view.path(fx));
          await settle(page);
          const container = await view.open(c);
          await expect(container).toBeVisible();

          const notes: string[] = [];
          const violations = await withRenderedContrast(page, await accessibilityViolations(page), notes);
          cell.axe = violations.length === 0 ? 'pass' : describeViolations(violations);
          if (notes.length) cell.note = notes.join('; ');
          const fit = await container.evaluate((element) => {
            const box = element.getBoundingClientRect();
            const width = document.documentElement.clientWidth;
            // The innermost elements past the right edge name the cause.
            const outside = [...document.body.querySelectorAll<HTMLElement>('*')].filter((node) =>
              node.getBoundingClientRect().right > width + 1 &&
              ![...node.children].some((child) => child.getBoundingClientRect().right > width + 1));
            const culprits = outside.slice(0, 3).map((node) =>
              `<${node.tagName.toLowerCase()} ${(node.getAttribute('class') ?? '').slice(0, 60)}> "${(node.textContent ?? '').trim().slice(0, 40)}"`);
            return {
              page: document.documentElement.scrollWidth - width,
              own: element.scrollWidth - element.clientWidth,
              left: box.left, right: box.right - width, culprits
            };
          });
          const misfit = [
            fit.page > 0 && `page scrolls ${fit.page}px sideways (${fit.culprits.join(', ')})`,
            fit.own > 1 && `sub-view scrolls ${fit.own}px sideways`,
            (fit.left < -1 || fit.right > 1) && `sub-view leaves the viewport (${Math.round(fit.left)}…${Math.round(fit.right)})`
          ].filter(Boolean);
          cell.fits = misfit.length === 0 ? 'pass' : misfit.join('; ');

          if (view.close === null) cell.closes = 'n/a';
          else {
            // A nested review closes back into the dialog that opened it.
            const isDialog = await container.evaluate((element) => element.tagName === 'DIALOG');
            const expectedOpen = (await page.locator('dialog[open]').count()) - (isDialog ? 1 : 0);
            if (view.close) await view.close(c, container);
            else await page.keyboard.press('Escape');
            // An import review closes into the next step of the same page; its close asserts that itself.
            const gone = view.group === 'import review' ||
              await container.waitFor({ state: 'hidden', timeout: 5_000 }).then(() => true, () => false);
            const stray = await page.locator('dialog[open]').count();
            cell.closes = gone && stray === expectedOpen
              ? 'pass' : `still open (${stray} open dialog${stray === 1 ? '' : 's'}, expected ${expectedOpen})`;
          }
        } catch (error) {
          const shown = await page.getByRole('alert').first().innerText({ timeout: 1_000 }).catch(() => '');
          const message = (error instanceof Error ? error.message.split('\n')[0] : String(error)) +
            (shown ? ` (alert: ${shown.replace(/\s+/g, ' ').slice(0, 160)})` : '');
          if (cell.axe === '—') cell.axe = `could not open: ${message}`;
          else cell.closes = `failed: ${message}`;
        }
        for (const [check, result] of Object.entries(cell)) {
          if (check !== 'note' && result !== 'pass' && result !== 'n/a') failures.push(`${view.name} [${check}]: ${result}`);
        }
      }
      writes = undefined;

      const dir = path.join(repoRoot, 'e2e/test-results/subview-sweep');
      mkdirSync(dir, { recursive: true });
      writeFileSync(path.join(dir, `${config.name.replace(/\s+/g, '-')}.json`), JSON.stringify(matrix, null, 2));
      expect(failures, failures.join('\n')).toEqual([]);
    });
  }
});
