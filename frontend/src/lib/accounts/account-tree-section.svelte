<script lang="ts">
  import Archive from '@lucide/svelte/icons/archive';
  import Building2 from '@lucide/svelte/icons/building-2';
  import Edit3 from '@lucide/svelte/icons/edit-3';
  import Lock from '@lucide/svelte/icons/lock';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import type { AccountResponse } from '$lib/api/accounts';
  import type { CurrencyResponse } from '$lib/api/currencies';
  import type { InstitutionResponse } from '$lib/api/institutions';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import TreeNameCell from '$lib/components/tree-name-cell.svelte';
  import { m } from '$lib/paraglide/messages.js';
  import {
    accountClassRank,
    accountCommodityLabel,
    accountDisplayName,
    accountKindGroupLabel,
    accountKindLabel,
    accountStatusLabel,
    institutionCountryLabel,
    institutionKindLabel
  } from './account-labels';
  import AccountKindIcon from './account-kind-icon.svelte';

  type InstitutionRoot = {
    kind: 'institution';
    id: string;
    label: string;
    institution?: InstitutionResponse;
    rows: AccountTreeRow[];
  };

  type AccountTreeRow = {
    account: AccountResponse;
    depth: number;
  };

  let {
    accounts,
    institutions,
    currenciesByID,
    countryNames,
    collapsedNodeIDs,
    onToggle,
    onEditAccount,
    onCloseAccount,
    onArchiveAccount,
    onDeleteAccount,
    onEditInstitution,
    onDeleteInstitution,
    institutionAccountCounts,
    allAccountChildCounts,
    actionPendingKey
  } = $props<{
    accounts: AccountResponse[];
    institutions: InstitutionResponse[];
    currenciesByID: ReadonlyMap<number, CurrencyResponse>;
    countryNames: Intl.DisplayNames;
    collapsedNodeIDs: ReadonlySet<string>;
    onToggle: (nodeID: string) => void;
    onEditAccount: (account: AccountResponse) => void;
    onCloseAccount: (account: AccountResponse) => void;
    onArchiveAccount: (account: AccountResponse) => void;
    onDeleteAccount: (account: AccountResponse) => void;
    onEditInstitution: (institution: InstitutionResponse) => void;
    onDeleteInstitution: (institution: InstitutionResponse) => void;
    institutionAccountCounts: ReadonlyMap<number, number>;
    allAccountChildCounts: ReadonlyMap<number, number>;
    actionPendingKey: string;
  }>();

  const collator = new Intl.Collator(undefined, { sensitivity: 'base', numeric: true });
  const accountTreeRoots = $derived.by(() => buildInstitutionRoots(accounts, institutions));
  const visibleAccountChildCounts = $derived.by(() => countAccountChildren(accounts));

  function buildInstitutionRoots(visibleAccounts: AccountResponse[], visibleInstitutions: InstitutionResponse[]): InstitutionRoot[] {
    const rootsByID = new Map<string, InstitutionRoot>();

    for (const institution of visibleInstitutions) {
      rootsByID.set(institutionNodeID(institution.id), {
        kind: 'institution',
        id: institutionNodeID(institution.id),
        label: institution.name,
        institution,
        rows: []
      });
    }

    for (const account of visibleAccounts) {
      const rootID = account.institution_id ? institutionNodeID(account.institution_id) : noInstitutionNodeID();
      if (!rootsByID.has(rootID)) {
        rootsByID.set(rootID, {
          kind: 'institution',
          id: rootID,
          label: account.institution_id ? m.accounts_group_unknown_institution() : m.accounts_group_no_institution(),
          rows: []
        });
      }
    }

    const accountsByRootID = new Map<string, AccountResponse[]>();
    for (const account of visibleAccounts) {
      const rootID = account.institution_id ? institutionNodeID(account.institution_id) : noInstitutionNodeID();
      const siblings = accountsByRootID.get(rootID) ?? [];
      siblings.push(account);
      accountsByRootID.set(rootID, siblings);
    }

    for (const [rootID, rootAccounts] of accountsByRootID) {
      rootsByID.get(rootID)?.rows.push(...buildAccountRows(rootAccounts));
    }

    return Array.from(rootsByID.values())
      .filter((root) => root.institution || root.rows.length > 0)
      .toSorted(compareRoots);
  }

  function buildAccountRows(rootAccounts: AccountResponse[]): AccountTreeRow[] {
    const accountsByID = new Map(rootAccounts.map((account) => [account.id, account]));
    const childrenByParentID = new Map<number, AccountResponse[]>();
    const roots: AccountResponse[] = [];

    for (const account of rootAccounts) {
      const parentID = account.parent_account_id;
      const parent = parentID ? accountsByID.get(parentID) : undefined;

      if (parent) {
        const siblings = childrenByParentID.get(parent.id) ?? [];
        siblings.push(account);
        childrenByParentID.set(parent.id, siblings);
      } else {
        roots.push(account);
      }
    }

    for (const siblings of childrenByParentID.values()) {
      siblings.sort(compareAccounts);
    }
    roots.sort(compareAccounts);

    const rows: AccountTreeRow[] = [];
    for (const root of roots) {
      appendAccountRows(root, 1, childrenByParentID, rows);
    }

    return rows;
  }

  function appendAccountRows(
    account: AccountResponse,
    depth: number,
    childrenByParentID: ReadonlyMap<number, AccountResponse[]>,
    rows: AccountTreeRow[]
  ) {
    rows.push({ account, depth });

    if (collapsedNodeIDs.has(accountNodeID(account.id))) {
      return;
    }

    const children = childrenByParentID.get(account.id) ?? [];
    for (const child of children) {
      appendAccountRows(child, depth + 1, childrenByParentID, rows);
    }
  }

  function countAccountChildren(values: AccountResponse[]): Map<number, number> {
    const visibleIDs = new Set(values.map((account) => account.id));
    const counts = new Map<number, number>();

    for (const account of values) {
      const parentID = account.parent_account_id;
      if (parentID && visibleIDs.has(parentID)) {
        counts.set(parentID, (counts.get(parentID) ?? 0) + 1);
      }
    }

    return counts;
  }

  function compareRoots(left: InstitutionRoot, right: InstitutionRoot): number {
    if (left.id === noInstitutionNodeID()) {
      return 1;
    }
    if (right.id === noInstitutionNodeID()) {
      return -1;
    }

    return collator.compare(left.label, right.label);
  }

  function compareAccounts(left: AccountResponse, right: AccountResponse): number {
    return accountClassRank(left.account_class) - accountClassRank(right.account_class)
      || collator.compare(accountDisplayName(left), accountDisplayName(right))
      || left.id - right.id;
  }

  function institutionNodeID(institutionID: number): string {
    return `institution:${institutionID}`;
  }

  function noInstitutionNodeID(): string {
    return 'institution:none';
  }

  function accountNodeID(accountID: number): string {
    return `account:${accountID}`;
  }
</script>

<section class="overflow-hidden rounded-[var(--radius-panel)] border border-border bg-surface shadow-[var(--shadow-panel)]">
  <div class="flex items-center justify-between gap-4 border-b border-border bg-toolbar px-4 py-3">
    <h2 class="truncate text-sm font-semibold tracking-tight text-foreground">{m.accounts_tree_title()}</h2>
    <span class="rounded-[var(--radius-control)] bg-surface-strong px-2.5 py-1 text-xs font-semibold text-muted">
      {m.accounts_group_count({ count: accounts.length })}
    </span>
  </div>

  <div class="divide-y divide-border">
    {#each accountTreeRoots as root (root.id)}
      {@const rootCollapsed = collapsedNodeIDs.has(root.id)}
      <article class="grid gap-3 bg-toolbar/45 px-4 py-3 transition hover:bg-row-hover lg:grid-cols-[minmax(12rem,1fr)_minmax(10rem,0.75fr)_minmax(8rem,0.55fr)_minmax(8rem,0.5fr)_7.25rem] lg:items-center">
        <TreeNameCell
          expandable={root.rows.length > 0}
          expanded={!rootCollapsed}
          expandLabel={m.accounts_tree_expand({ name: root.label })}
          collapseLabel={m.accounts_tree_collapse({ name: root.label })}
          onToggle={() => onToggle(root.id)}
        >
          <div class="flex min-w-0 items-center gap-2">
            <span class="inline-flex size-8 shrink-0 items-center justify-center rounded-[var(--radius-control)] bg-surface-strong text-foreground">
              <Building2 size={16} aria-hidden="true" />
            </span>
            <p class="min-w-0 truncate text-sm font-semibold text-foreground">{root.label}</p>
          </div>
          <p class="mt-1 truncate text-xs text-muted">
            {#if root.institution}
              {institutionKindLabel(root.institution.kind)}
            {:else}
              {m.accounts_group_no_institution()}
            {/if}
          </p>
        </TreeNameCell>

        <div class="min-w-0 text-sm">
          {#if root.institution}
            <p class="truncate text-foreground">{institutionCountryLabel(root.institution, countryNames)}</p>
            <p class="mt-1 truncate text-xs text-muted">{root.institution.website || m.institutions_no_website()}</p>
          {:else}
            <p class="truncate text-foreground">{m.accounts_tree_unassigned_detail()}</p>
            <p class="mt-1 truncate text-xs text-muted">{m.accounts_tree_unassigned_hint()}</p>
          {/if}
        </div>

        <div class="min-w-0 text-sm">
          <p class="truncate text-foreground">{m.institutions_account_count({ count: root.rows.length })}</p>
        </div>

        <div class="flex min-w-0 items-center justify-start">
          {#if root.institution}
            <StatusBadge tone="accent">{m.account_status_active()}</StatusBadge>
          {:else}
            <StatusBadge>{m.accounts_tree_group_label()}</StatusBadge>
          {/if}
        </div>

        <div class="flex min-w-0 items-center justify-start gap-1 lg:justify-end">
          {#if root.institution}
            <button
              type="button"
              class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover"
              onclick={() => onEditInstitution(root.institution)}
              aria-label={m.institutions_action_edit({ name: root.institution.name })}
              title={m.institutions_action_edit({ name: root.institution.name })}
            >
              <Edit3 size={15} aria-hidden="true" />
            </button>
            <button
              type="button"
              class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-danger/30 bg-danger-soft text-danger transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
              onclick={() => onDeleteInstitution(root.institution)}
              disabled={(institutionAccountCounts.get(root.institution.id) ?? 0) > 0 || actionPendingKey === `institution-delete:${root.institution.id}`}
              aria-label={m.institutions_action_delete({ name: root.institution.name })}
              title={m.institutions_action_delete({ name: root.institution.name })}
            >
              <Trash2 size={15} aria-hidden="true" />
            </button>
          {/if}
        </div>
      </article>

      {#if !rootCollapsed}
        {#each root.rows as row (row.account.id)}
          {@const account = row.account}
          {@const hasChildren = (visibleAccountChildCounts.get(account.id) ?? 0) > 0}
          {@const hasAnyChildren = (allAccountChildCounts.get(account.id) ?? 0) > 0}
          {@const accountCollapsed = collapsedNodeIDs.has(accountNodeID(account.id))}
          <!--
            Stretched-link pattern: the <a> for the register covers the whole row via
            ::after. Action buttons sit at relative z-[1] so clicks on them don't
            navigate. No interactive elements are nested inside the <a>.
          -->
          <article class="relative grid gap-3 px-4 py-3 transition hover:bg-row-hover lg:grid-cols-[minmax(12rem,1fr)_minmax(10rem,0.75fr)_minmax(8rem,0.55fr)_minmax(8rem,0.5fr)_7.25rem] lg:items-center">
            <TreeNameCell
              depth={row.depth}
              expandable={hasChildren}
              expanded={!accountCollapsed}
              expandLabel={m.accounts_tree_expand({ name: accountDisplayName(account) })}
              collapseLabel={m.accounts_tree_collapse({ name: accountDisplayName(account) })}
              onToggle={() => onToggle(accountNodeID(account.id))}
            >
              <div class="flex min-w-0 items-center gap-2">
                <span class="inline-flex size-8 shrink-0 items-center justify-center rounded-[var(--radius-control)] bg-accent-soft text-accent">
                  <AccountKindIcon kind={account.account_kind} />
                </span>
                <!--
                  The stretched link: a visually hidden label inside a block <a> that
                  expands to cover the entire row via ::after. We apply
                  "before:absolute before:inset-0" via Tailwind arbitrary variant so no
                  custom CSS file is required.
                -->
                <a
                  href="/app/accounts/{account.id}/register"
                  class="min-w-0 truncate text-sm font-semibold text-foreground after:absolute after:inset-0 after:z-0"
                  aria-label={m.accounts_action_view_register({ name: accountDisplayName(account) })}
                >
                  {accountDisplayName(account)}
                </a>
              </div>
              <p class="mt-1 truncate text-xs text-muted">
                {accountKindLabel(account.account_kind)}
                {#if account.code}
                  <span aria-hidden="true"> · </span>{account.code}
                {/if}
              </p>
            </TreeNameCell>

            <div class="min-w-0 text-sm">
              <p class="truncate text-foreground">{accountKindGroupLabel(account.account_kind)}</p>
              <p class="mt-1 truncate text-xs text-muted">{account.allows_postings ? m.accounts_allows_postings_yes() : m.accounts_allows_postings_no()}</p>
            </div>

            <div class="min-w-0 text-sm">
              <p class="truncate text-foreground">{accountCommodityLabel(account, currenciesByID)}</p>
              {#if account.number_last4}
                <p class="mt-1 truncate text-xs text-muted">{m.accounts_number_hint({ last4: account.number_last4 })}</p>
              {:else}
                <p class="mt-1 truncate text-xs text-muted">{m.accounts_tree_no_number_hint()}</p>
              {/if}
            </div>

            <div class="flex min-w-0 items-center justify-start">
              <StatusBadge tone={account.status === 'closed' ? 'danger' : 'accent'}>
                {accountStatusLabel(account.status)}
              </StatusBadge>
            </div>

            <!-- Action buttons are siblings at z-[1], above the stretched-link ::after overlay -->
            <div class="relative z-[1] flex min-w-0 items-center justify-start gap-1 lg:justify-end">
              <button
                type="button"
                class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover"
                onclick={() => onEditAccount(account)}
                aria-label={m.accounts_action_edit({ name: accountDisplayName(account) })}
                title={m.accounts_action_edit({ name: accountDisplayName(account) })}
              >
                <Edit3 size={15} aria-hidden="true" />
              </button>

              {#if account.status === 'active'}
                <button
                  type="button"
                  class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-50"
                  onclick={() => onCloseAccount(account)}
                  disabled={hasAnyChildren || actionPendingKey === `close:${account.id}`}
                  aria-label={m.accounts_action_close({ name: accountDisplayName(account) })}
                  title={m.accounts_action_close({ name: accountDisplayName(account) })}
                >
                  <Lock size={15} aria-hidden="true" />
                </button>
              {:else if account.status === 'closed'}
                <button
                  type="button"
                  class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-50"
                  onclick={() => onArchiveAccount(account)}
                  disabled={hasAnyChildren || actionPendingKey === `archive:${account.id}`}
                  aria-label={m.accounts_action_archive({ name: accountDisplayName(account) })}
                  title={m.accounts_action_archive({ name: accountDisplayName(account) })}
                >
                  <Archive size={15} aria-hidden="true" />
                </button>
              {/if}

              <button
                type="button"
                class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-danger/30 bg-danger-soft text-danger transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
                onclick={() => onDeleteAccount(account)}
                disabled={hasAnyChildren || actionPendingKey === `delete:${account.id}`}
                aria-label={m.accounts_action_delete({ name: accountDisplayName(account) })}
                title={m.accounts_action_delete({ name: accountDisplayName(account) })}
              >
                <Trash2 size={15} aria-hidden="true" />
              </button>
            </div>
          </article>
        {/each}
      {/if}
    {/each}
  </div>
</section>
