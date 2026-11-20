import { readFileSync, readdirSync, statSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

/**
 * Every screen names an account through `account-labels.ts`
 * (`accountDisplayName`, `accountLabelByID`, `accountLabelFromParts`), never by
 * hand (bug class 13, #597 sweep).
 *
 * The starter cash account has no name of its own (code `cash:USD`, metadata
 * `starter_key: "cash"`). A hand-built `name || code` showed it as raw
 * `cash:USD` on the register heading, Returns, Allocation, Forecast and
 * Budgets, and a bare `{account.name}` drew it as a blank option in every
 * investment and import cash-account picker, while Ledger read "Cash USD".
 *
 * Flagged, per line: a `.name` read on an account-like value (`a`, `acc`,
 * `brokerage`, anything ending in `account`/`Account`, or an `accounts…find()`
 * result); a server-sent `…account_name`; and any `name || … code` /
 * `name ?? … code` fallback. A line that hands the parts to the owner (the
 * call starts on it or on the line before) passes. Everything else needs an
 * entry below with its reason.
 */

const src = fileURLToPath(new URL('../..', import.meta.url));
const owner = 'lib/accounts/account-labels.ts';

const accountNameRead = /(?<![\w$])(?:a|acc|brokerage|\w*[aA]ccount)\??\.name\b/;
const foundAccountNameRead = /\baccounts\b[^\n]*\.find\([^\n]*\)\??\.name\b/;
const serverAccountName = /\.\w*account_name\b/;
const nameOrCode = /\bname\b(?:\??\.trim\(\))?\s*(?:\|\||\?\?)\s*[^\n{};,]*\.code\b/;
const ownerCall = /\b(?:accountDisplayName|accountLabelByID|accountLabelFromParts)\(/;

// file → line fragments that may match, each with its reason. A fragment
// that no longer matches anything fails the test, so the list cannot rot.
const allowed: Record<string, { fragment: string; reason: string }[]> = {
  'lib/budgets/budgets-screen.svelte': [
    { fragment: 'return category.name || category.code', reason: 'a category, not an account (category label class, #619)' },
    { fragment: '{category.name || category.code}', reason: 'a category, not an account (category label class, #619)' }
  ],
  'lib/forecast/forecast-learning.svelte': [
    { fragment: 'option.name ?? option.code', reason: 'expense category options, not accounts (category label class, #619)' }
  ],
  'lib/reports/spending-view.svelte': [
    { fragment: 'return row.name || row.code', reason: 'a payee or category row after categoryDisplayName; never an account' }
  ],
  'lib/home/widgets.ts': [
    { fragment: "category.name.trim() || (category.builtin_key", reason: 'a category label with its built-in key; never an account' }
  ],
  'lib/investments/allocation-screen.svelte': [
    { fragment: 'name: c.name || c.code', reason: 'an instrument commodity, not an account' },
    { fragment: 'commodity?.name || commodity?.code', reason: 'an instrument commodity, not an account' }
  ],
  'lib/investments/split-activity.svelte': [
    { fragment: '{row.account_name}', reason: 'a split names a security holding account, which always has a name (the API requires one; only starter cash is unnamed)' }
  ],
  'lib/investments/event-suggestions.svelte': [
    { fragment: 'candidate.holding_account_name', reason: 'a holding account, which always has a name (only starter cash is unnamed)' }
  ]
};

function sourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = path.join(dir, entry);
    if (statSync(full).isDirectory()) return entry === 'paraglide' ? [] : sourceFiles(full);
    return /\.(ts|svelte)$/.test(entry) && !entry.endsWith('.test.ts') && !entry.endsWith('.d.ts') ? [full] : [];
  });
}

function handBuiltLabels(text: string): { line: number; text: string }[] {
  const lines = text.split('\n');
  return lines.flatMap((line, index) => {
    const matches = [accountNameRead, foundAccountNameRead, serverAccountName, nameOrCode].some((pattern) => pattern.test(line));
    const handedToOwner = ownerCall.test(line) || (index > 0 && ownerCall.test(lines[index - 1]));
    return matches && !handedToOwner ? [{ line: index + 1, text: line.trim() }] : [];
  });
}

describe('account labels have one owner', () => {
  const files = sourceFiles(src).map((full) => path.relative(src, full).split(path.sep).join('/')).filter((file) => file !== owner);

  it('finds no hand-built account label outside account-labels.ts', () => {
    const offenders: string[] = [];
    const used = new Set<string>();
    for (const file of files) {
      for (const hit of handBuiltLabels(readFileSync(path.join(src, file), 'utf8'))) {
        const entry = allowed[file]?.find((candidate) => hit.text.includes(candidate.fragment));
        if (entry) used.add(`${file}::${entry.fragment}`);
        else offenders.push(`${file}:${hit.line}: ${hit.text}`);
      }
    }
    expect(offenders).toEqual([]);
    const stale = Object.entries(allowed).flatMap(([file, entries]) =>
      entries.filter((entry) => !used.has(`${file}::${entry.fragment}`)).map((entry) => `${file}: ${entry.fragment}`));
    expect(stale).toEqual([]);
  });

  it('flags each spelling the sweep removed', () => {
    for (const line of [
      'const accountName = $derived(account?.name || account?.code || m.accounts_title());',
      '{ label: brokerage.name || brokerage.code || String(brokerage.id), href }',
      '<option value={String(acc.id)}>{acc.name}</option>',
      '{#each cashAccounts as a(a.id)}<option value={String(a.id)}>{a.name}</option>{/each}',
      '{(accountsQuery.data?.accounts ?? []).find((account) => String(account.id) === cashAccountID)?.name ?? cashAccountID}',
      '{accounts.find((account) => account.id === holding.accountID)?.name}',
      'return account.name?.trim() || account.code?.trim() || m.forecast_account_number({ id: account.id });',
      'label: posting.account_system_role ? systemRoleLabel(posting.account_system_role) : (posting.account_name ?? String(posting.account_id)),',
      '{#if account?.name}{sourceAccount?.name}{/if}',
      '<a href={href}>{row.account.name}</a>'
    ]) {
      expect(handBuiltLabels(line), line).toHaveLength(1);
    }
  });

  it('passes the owner and values that only look alike', () => {
    for (const text of [
      '<option value={account.id}>{accountDisplayName(account)}</option>',
      'return accountLabelByID(accounts, id) ?? `#${id}`;',
      "label: accountLabelFromParts({\n        id: account.account_id, name: account.name, code: account.code,",
      '<option value={source.id}>{source.name}</option>',
      '{institution.name}',
      "{sources.find(s => s.id === assignment.source_id)?.name ?? ''}"
    ]) {
      expect(handBuiltLabels(text), text).toEqual([]);
    }
  });
});
