<script lang="ts">
  import type { LedgerAccount, LedgerOverview } from '#lib/api/ledger.ts';
  import type { InstitutionResponse } from '#lib/api/institutions.ts';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { m } from '#lib/paraglide/messages.js';
  import Panel from '#lib/components/panel.svelte';
  import { institutionCountryLabel } from './account-labels.ts';
  let { institutions, accounts, onEdit, onArchive, onRestore, pending }: {
    institutions: InstitutionResponse[]; accounts: LedgerAccount[];
    onEdit: (institution: InstitutionResponse) => void;
    onArchive: (institution: InstitutionResponse) => void;
    onRestore: (institution: InstitutionResponse) => void;
    pending: string;
  } = $props();
  let showArchived = $state(false);
  const visibleInstitutions = $derived(institutions.filter(i => showArchived || i.status !== 'archived'));
  const countries = $derived(new Intl.DisplayNames(getLocale(), { type: 'region' }));
  const sourceLabel = { file: m.ledger_source_file, profile: m.ledger_source_profile, online: m.ledger_source_online };
</script>
<div class="space-y-4">
  <label class="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" bind:checked={showArchived} />{m.recurring_show_archived()}</label>
  {#if institutions.length === 0}<Panel><p class="text-sm text-muted">{m.ledger_no_institutions()}</p></Panel>{/if}
  {#each visibleInstitutions as institution (institution.id)}
    {@const members = accounts.filter(row => row.account.institution_id === institution.id)}
    <Panel>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="min-w-0"><h2 class="wrap-anywhere font-semibold">{institution.name}</h2><p class="text-sm text-muted">{institutionCountryLabel(institution, countries)}</p></div>
        <div class="flex flex-wrap gap-2">
          <button type="button" class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 text-sm" onclick={() => onEdit(institution)}>{m.ledger_edit_institution()}</button>
          {#if institution.status === 'archived'}
            <button type="button" class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 text-sm" disabled={!!pending} onclick={() => onRestore(institution)}>{m.ledger_restore_institution()}</button>
          {:else}
            <button type="button" class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 text-sm" disabled={!!pending} onclick={() => onArchive(institution)}>{m.ledger_archive_institution()}</button>
          {/if}
        </div>
      </div>
      <ul class="mt-3 divide-y divide-border">
        {#each members as row (row.account.id)}
          <li class="py-2 text-sm">
            <a class="inline-flex min-h-11 items-center wrap-anywhere font-semibold underline" href={`/app/ledger/accounts/${row.account.id}/register`}>{row.account.name || row.account.code}</a>
            {#each row.sources as source}<p class="wrap-anywhere text-muted">{sourceLabel[source.kind]({ name: source.name })}</p>{/each}
            {#if row.sources.length === 0}<p class="text-muted">{m.ledger_source_none()}</p>{/if}
          </li>
        {/each}
      </ul>
      {#if members.length === 0}<p class="mt-3 text-sm text-muted">{m.ledger_no_institution_accounts()}</p>{/if}
    </Panel>
  {/each}
</div>
