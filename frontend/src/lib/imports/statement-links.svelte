<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import StatusBadge from '#lib/components/status-badge.svelte';
  import { APIClientError } from '#lib/api/client.ts';
  import { currenciesQueryOptions } from '#lib/api/currencies.ts';
  import {
    correctStatementLink,
    getStatementLinkCandidates,
    getStatementLinks,
    statementLinksQueryKey,
    type ImportStatementLink
  } from '#lib/api/imports.ts';
  import { matchCandidateLabel, matchReasonText, type ImportMatchCandidate } from '#lib/imports/match-review.ts';
  import { formatQuantity, joinCommodityAmount } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  let { transactionId, csrfToken }: { transactionId: number; csrfToken?: string } = $props();

  // #485 review: the statement rows linked to this transaction, and the
  // audited repairs of a mistaken link. A row unlinked from here stays listed
  // so it can be linked again; every change keeps the old link as history.
  const queryClient = useQueryClient();
  const linksQuery = createQuery(() => ({
    queryKey: [...statementLinksQueryKey, transactionId],
    queryFn: () => getStatementLinks(transactionId)
  }));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  type Dialog = { kind: 'reassign' | 'unlink'; link: ImportStatementLink };
  let dialog = $state<Dialog | null>(null);
  let reason = $state('');
  let chosen = $state('');
  let pending = $state(false);
  let actionError = $state<unknown>(undefined);

  const candidatesQuery = createQuery(() => ({
    queryKey: [...statementLinksQueryKey, 'candidates', dialog?.link.identity_id ?? 0],
    queryFn: () => getStatementLinkCandidates(dialog?.link.identity_id ?? 0),
    enabled: dialog?.kind === 'reassign'
  }));

  const locale = $derived(getLocale());
  const dateFormatter = $derived(new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' }));
  const links = $derived(linksQuery.data?.links ?? []);
  const candidates = $derived(candidatesQuery.data?.candidates ?? []);
  const chosenCandidate = $derived(candidates.find((candidate) => candidateKey(candidate) === chosen));

  function date(value: string): string {
    return value ? dateFormatter.format(parseISO(value)) : '';
  }

  function money(value: string, scale: number, commodityID: number): string {
    const currency = currenciesQuery.data?.currencies.find((item) => item.id === commodityID);
    return joinCommodityAmount(currency?.display_symbol || currency?.code || '', formatQuantity(value, scale, locale));
  }

  function rowText(link: ImportStatementLink): string {
    return m.statement_links_row({
      date: date(link.row_date),
      amount: money(link.row_amount_value, link.row_amount_scale, link.commodity_id),
      payee: link.row_payee || m.import_match_untitled()
    });
  }

  function candidateKey(candidate: ImportMatchCandidate): string {
    return `${candidate.transaction_id}:${candidate.line_key}:${candidate.posted_on}`;
  }

  function open(kind: Dialog['kind'], link: ImportStatementLink) {
    dialog = { kind, link };
    reason = '';
    chosen = '';
    actionError = undefined;
  }

  function close() {
    if (!pending) dialog = null;
  }

  async function confirm() {
    if (!dialog || !csrfToken || !reason.trim()) return;
    if (dialog.kind === 'reassign' && !chosenCandidate) return;
    pending = true;
    actionError = undefined;
    try {
      await correctStatementLink(dialog.link.identity_id, dialog.kind, {
        expected_link_id: dialog.link.link_id,
        expected_link_active: dialog.link.active,
        reason: reason.trim(),
        ...(dialog.kind === 'reassign' && chosenCandidate
          ? { transaction_id: chosenCandidate.transaction_id, line_key: chosenCandidate.line_key, posted_on: chosenCandidate.posted_on }
          : {})
      }, csrfToken);
      dialog = null;
      await queryClient.invalidateQueries({ queryKey: statementLinksQueryKey });
    } catch (error) {
      actionError = error;
      if (error instanceof APIClientError && error.code === 'IMPORT_LINK_STALE' && dialog) {
        // Another tab changed the row. Reload it by its identity, not from
        // this transaction's list (a moved row is no longer listed here), so
        // the next request names the link as it is now; the owner chooses
        // again.
        chosen = '';
        const identityId = dialog.link.identity_id;
        try {
          const current = await queryClient.fetchQuery({
            queryKey: [...statementLinksQueryKey, 'candidates', identityId],
            queryFn: () => getStatementLinkCandidates(identityId),
            staleTime: 0
          });
          if (dialog?.link.identity_id === identityId) dialog = { ...dialog, link: current.link };
        } catch (refreshError) {
          actionError = refreshError;
        }
        await queryClient.invalidateQueries({ queryKey: statementLinksQueryKey, exact: false, refetchType: 'active' });
      }
    } finally {
      pending = false;
    }
  }

  const btnAction =
    'inline-flex min-h-11 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition-colors hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60';
  const btnPrimary =
    'inline-flex min-h-11 items-center rounded-[var(--radius-control)] bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60';
</script>

{#if linksQuery.isError}
  <APIFormError error={linksQuery.error} id="statement-links-error" />
{:else if links.length > 0}
  <section aria-labelledby="statement-links-title" data-testid="statement-links">
    <h3 id="statement-links-title" class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.statement_links_title()}</h3>
    <ul class="mt-2 space-y-3">
      {#each links as link (link.identity_id)}
        <li class="rounded-[var(--radius-control)] border border-border p-3">
          <div class="flex flex-wrap items-center gap-2">
            <span class="min-w-0 text-sm text-foreground">{rowText(link)}</span>
            <StatusBadge tone={link.active ? 'positive' : 'warning'}>
              {link.active ? m.statement_links_status_linked() : m.statement_links_status_unlinked()}
            </StatusBadge>
          </div>
          <p class="mt-1 text-xs text-muted">{m.statement_links_source({ filename: link.filename })}</p>
          {#if !link.active}
            <p class="mt-1 text-xs text-muted">{m.statement_links_unlinked_reason({ date: date(link.retired_at ?? ''), reason: link.retire_reason ?? '' })}</p>
          {/if}
          {#if csrfToken}
            <div class="mt-2 flex flex-wrap gap-2">
              <button type="button" class={btnAction} onclick={() => open('reassign', link)}>{m.statement_links_reassign()}</button>
              {#if link.active}
                <button type="button" class={btnAction} onclick={() => open('unlink', link)}>{m.statement_links_unlink()}</button>
              {/if}
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  </section>
{/if}

{#if dialog}
  <ModalDialog labelledby="statement-link-dialog-title" describedby="statement-link-dialog-help" {pending} onCancel={close} class="max-w-lg">
    <form
      class="space-y-4 p-4"
      aria-busy={pending}
      onsubmit={(event) => { event.preventDefault(); void confirm(); }}
    >
      <div>
        <h3 id="statement-link-dialog-title" class="text-sm font-semibold text-foreground">
          {dialog.kind === 'reassign' ? m.statement_links_reassign_title() : m.statement_links_unlink_title()}
        </h3>
        <p class="mt-1 text-sm text-foreground">{rowText(dialog.link)}</p>
        <p id="statement-link-dialog-help" class="mt-1 text-sm text-muted">
          {dialog.kind === 'reassign'
            ? m.statement_links_reassign_help({ days: candidatesQuery.data?.window_days ?? 5 })
            : m.statement_links_unlink_help()}
        </p>
      </div>

      {#if dialog.kind === 'unlink' && !dialog.link.active}
        <p role="status" class="text-sm text-warning">{m.statement_links_already_unlinked()}</p>
      {/if}

      {#if dialog.kind === 'reassign'}
        {#if candidatesQuery.isPending}
          <p role="status" class="text-sm text-muted">{m.statement_links_loading()}</p>
        {:else if candidatesQuery.isError}
          <APIFormError error={candidatesQuery.error} id="statement-link-candidates-error" />
        {:else if candidates.length === 0}
          <p role="status" class="text-sm text-muted">{m.statement_links_reassign_empty()}</p>
        {:else}
          <fieldset class="space-y-1">
            <legend class="sr-only">{m.statement_links_reassign_title()}</legend>
            {#each candidates as candidate (candidateKey(candidate))}
              <label class="flex min-h-11 cursor-pointer items-start gap-3 rounded-[var(--radius-control)] px-2 py-2 hover:bg-control-hover">
                <input type="radio" class="mt-1" name="statement-link-target" value={candidateKey(candidate)} bind:group={chosen} />
                <span class="min-w-0">
                  <span class="block text-sm text-foreground">{matchCandidateLabel(candidate, date(candidate.transaction_date))}</span>
                  <span class="block text-sm text-muted">{matchReasonText(candidate, money(candidate.quantity_value, candidate.quantity_scale, candidate.commodity_id), locale)}</span>
                </span>
              </label>
            {/each}
          </fieldset>
        {/if}
      {/if}

      <label class="block text-sm">
        <span class="font-medium text-foreground">{m.statement_links_reason_label()}</span>
        <input
          type="text"
          required
          bind:value={reason}
          class="mt-1.5 h-11 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground outline-none transition-colors hover:bg-control-hover focus:border-accent"
        />
      </label>

      <APIFormError error={actionError} id="statement-link-action-error" />

      <div class="flex flex-wrap justify-end gap-2">
        <button type="button" class={btnAction} disabled={pending} onclick={close}>{m.statement_links_cancel()}</button>
        <button
          type="submit"
          class={btnPrimary}
          disabled={pending || !reason.trim() || (dialog.kind === 'reassign' && !chosenCandidate) || (dialog.kind === 'unlink' && !dialog.link.active)}
        >
          {dialog.kind === 'reassign' ? m.statement_links_confirm_reassign() : m.statement_links_confirm_unlink()}
        </button>
      </div>
    </form>
  </ModalDialog>
{/if}
