<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import type { AccountResponse } from '#lib/api/accounts.ts';
  import { importShortCandidates, importShortCandidatesQueryKey, linkImportShort, type ImportShortCandidate } from '#lib/api/imports.ts';
  import { maskFinancialText, formatQuantity, joinCommodityAmount } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  let { batchId, rowId, csrfToken, accounts, onLinked, disabled = false }: {
    batchId: number;
    rowId: number;
    csrfToken: string;
    accounts: AccountResponse[];
    onLinked: () => void | Promise<void>;
    // The import screen holds writes while it re-reads the batch (#567).
    disabled?: boolean;
  } = $props();

  // #245: the owner asserts which recorded short sale or cover this pending
  // unsupported fill evidences. The row is never classified by itself, and a
  // candidate that disagrees with any known source fact cannot be chosen.
  const candidatesQuery = createQuery(() => ({
    queryKey: [...importShortCandidatesQueryKey, batchId, rowId],
    queryFn: () => importShortCandidates(batchId, rowId)
  }));
  let selected = $state('');
  let pending = $state(false);
  let linkError = $state<unknown>(undefined);

  const locale = $derived(getLocale());
  const dateFormatter = $derived(new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' }));
  const source = $derived(candidatesQuery.data?.source);
  const candidates = $derived(candidatesQuery.data?.candidates ?? []);
  const isCover = $derived(source?.operation_kind === 'short_cover');

  /** Canonical dot-decimal from the API, shown in the reader's locale. */
  function decimal(value: string): string {
    const [whole, fraction = ''] = value.split('.');
    return formatQuantity(whole + fraction, fraction.length, locale);
  }

  function date(value: string): string {
    return value ? dateFormatter.format(parseISO(value)) : m.import_preview_short_link_unknown();
  }

  function accountName(id: number): string {
    return accounts.find((account) => account.id === id)?.name ?? `#${id}`;
  }

  const fieldLabels: Record<string, () => string> = {
    date: () => m.import_preview_short_link_field_date(),
    quantity: () => m.import_preview_short_link_field_quantity(),
    cash_amount: () => m.import_preview_short_link_field_cash_amount(),
    cash_currency: () => m.import_preview_short_link_field_cash_currency(),
    holding_account: () => m.import_preview_short_link_field_holding_account(),
    cash_account: () => m.import_preview_short_link_field_cash_account()
  };

  function mismatchText(candidate: ImportShortCandidate): string {
    return m.import_preview_short_link_mismatch({ fields: candidate.mismatches.map((field) => fieldLabels[field]?.() ?? field).join(', ') });
  }

  function linkable(candidate: ImportShortCandidate): boolean {
    return !candidate.linked && candidate.mismatches.length === 0;
  }

  async function link(event: SubmitEvent) {
    event.preventDefault();
    if (!selected || pending || disabled || !csrfToken) return;
    pending = true;
    linkError = undefined;
    try {
      await linkImportShort(batchId, rowId, Number(selected), csrfToken);
      await onLinked();
    } catch (error) {
      linkError = error;
      void candidatesQuery.refetch();
    } finally {
      pending = false;
    }
  }
</script>

<form class="max-w-xl space-y-3" onsubmit={link} aria-busy={pending}>
  <p class="text-sm text-muted">{m.import_preview_short_link_scope()}</p>
  {#if candidatesQuery.isPending}
    <p class="text-sm text-muted" role="status">{m.import_preview_short_link_loading()}</p>
  {:else if candidatesQuery.isError}
    <APIFormError error={candidatesQuery.error} id={`short-link-load-error-${rowId}`} />
  {:else if source}
    <section aria-labelledby={`short-link-source-${rowId}`} class="rounded-(--radius-control) border border-border bg-surface p-3">
      <h3 id={`short-link-source-${rowId}`} class="text-sm font-semibold text-foreground">{m.import_preview_short_link_source()}</h3>
      <p class="text-xs text-muted">{m.import_preview_short_link_source_side({ side: source.side, fillType: source.fill_type || m.import_preview_short_link_unknown() })} · {source.ticker || source.isin}</p>
      <dl class="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
        <dt class="text-muted">{m.import_preview_short_link_source_date()}</dt>
        <dd class="text-foreground tabular-nums">{date(source.date)}</dd>
        <dt class="text-muted">{m.import_preview_short_link_source_quantity()}</dt>
        <dd class="text-foreground tabular-nums break-all">{maskFinancialText(source.quantity) || m.import_preview_short_link_unknown()}</dd>
        <dt class="text-muted">{m.import_preview_short_link_source_cash()}</dt>
        <dd class="text-foreground tabular-nums break-all">{source.net_value ? joinCommodityAmount(source.net_currency, maskFinancialText(source.net_value)) : m.import_preview_short_link_unknown()}</dd>
      </dl>
    </section>
    {#if candidates.length === 0}
      <p class="text-sm text-foreground" role="status">
        {isCover ? m.import_preview_short_link_empty_cover() : m.import_preview_short_link_empty_sale()}
        <a href="/app/investments" class="font-semibold underline underline-offset-2">{m.import_preview_short_link_record()}</a>
      </p>
    {:else}
      <fieldset class="space-y-2">
        <legend class="text-sm font-medium text-foreground">{isCover ? m.import_preview_short_link_choose_cover() : m.import_preview_short_link_choose_sale()}</legend>
        {#each candidates as candidate (candidate.operation_id)}
          {@const descriptionID = `short-link-note-${rowId}-${candidate.operation_id}`}
          <label class="flex items-start gap-2 text-sm text-foreground" class:opacity-60={!linkable(candidate)}>
            <input type="radio" name={`short-link-${rowId}`} value={String(candidate.operation_id)}
              bind:group={selected} disabled={!linkable(candidate) || pending} class="mt-1 h-4 w-4"
              aria-describedby={linkable(candidate) ? undefined : descriptionID} />
            <span class="min-w-0 break-words">
              {m.import_preview_short_link_option({
                date: date(candidate.event_date),
                quantity: decimal(candidate.quantity),
                cash: joinCommodityAmount(candidate.cash_currency, decimal(candidate.net_amount)),
                account: accountName(candidate.holding_account_id)
              })}
              {#if candidate.linked}
                <span id={descriptionID} class="block text-xs text-muted">{m.import_preview_short_link_already()}</span>
              {:else if candidate.mismatches.length > 0}
                <span id={descriptionID} class="block text-xs text-warning">{mismatchText(candidate)}</span>
              {/if}
            </span>
          </label>
        {/each}
      </fieldset>
      <APIFormError error={linkError} id={`short-link-error-${rowId}`} />
      <button type="submit" disabled={!selected || pending || disabled || !csrfToken}
        class="rounded-(--radius-control) bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground hover:opacity-95 disabled:cursor-not-allowed disabled:opacity-60">
        {pending ? m.import_preview_short_link_pending() : m.import_preview_short_link_submit()}
      </button>
    {/if}
  {/if}
</form>
