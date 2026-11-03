<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import Panel from '#lib/components/panel.svelte';
  import StatePanel from '#lib/components/state-panel.svelte';
  import {
    investmentEventSuggestionsQueryOptions,
    investmentEventSuggestionsQueryKey,
    investmentInstrumentsQueryOptions,
    acceptEventSuggestion,
    ignoreEventSuggestion,
    linkEventSuggestion,
    queueDividendEventFetch,
    type InvestmentEventSuggestionResponse
  } from '#lib/api/investments.ts';
  import { formatQuantity } from '#lib/money/format.ts';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { m } from '#lib/paraglide/messages.js';
  import { invalidateInvestmentReads } from './invalidate';
  import ProviderDividendCompletion from './provider-dividend-completion.svelte';
  import ProviderPaymentInLieuCompletion from './provider-payment-in-lieu-completion.svelte';
  import {
    parseProviderDividendEvidence,
    paymentInLieuProposal,
    proposalCommodityID,
    proposalShape,
    providerDividendRevisions,
    type ProviderDecimal,
    type ProviderDividendContent,
    type ProviderDividendEvidence
  } from './provider-dividend.ts';

  interface Props {
    csrfToken: string;
  }

  const { csrfToken }: Props = $props();

  const queryClient = useQueryClient();
  // After queueing a fetch, poll briefly so staged events appear without a reload.
  let pollUntil = $state(0);
  const suggestionsQuery = createQuery(() => ({
    ...investmentEventSuggestionsQueryOptions(),
    refetchInterval: () => (Date.now() < pollUntil ? 5_000 : false)
  }));
  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());

  const suggestions = $derived(suggestionsQuery.data?.suggestions ?? []);
  const pending = $derived(suggestions.filter((s) => s.status === 'suggested'));
  const history = $derived(suggestions.filter((s) => s.status !== 'suggested'));
  const instrumentNames = $derived(
    new Map((instrumentsQuery.data?.instruments ?? []).map((instrument) => [instrument.id, instrument.display_name]))
  );

  let busy = $state<Set<number>>(new Set());
  let errors = $state<Map<number, string>>(new Map());
  let completing = $state<number | null>(null);
  let fetchPending = $state(false);
  let fetchMessage = $state<{ tone: 'info' | 'error'; text: string } | null>(null);

  function setBusy(id: number, on: boolean) {
    busy = on ? new Set([...busy, id]) : new Set([...busy].filter((other) => other !== id));
  }

  function setError(id: number, message: string | null) {
    const next = new Map([...errors].filter(([key]) => key !== id));
    if (message) next.set(id, message);
    errors = next;
  }

  async function refreshAfterPosting() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: investmentEventSuggestionsQueryKey }),
      invalidateInvestmentReads(queryClient)
    ]);
  }

  async function act(s: InvestmentEventSuggestionResponse, action: () => Promise<unknown>, failure: string, posts: boolean) {
    setBusy(s.id, true);
    setError(s.id, null);
    try {
      await action();
      if (posts) {
        await refreshAfterPosting();
      } else {
        await queryClient.invalidateQueries({ queryKey: investmentEventSuggestionsQueryKey });
      }
    } catch {
      setError(s.id, failure);
    } finally {
      setBusy(s.id, false);
    }
  }

  async function checkProviders() {
    fetchPending = true;
    fetchMessage = null;
    try {
      const result = await queueDividendEventFetch(csrfToken);
      if (result.queued_count === 0) {
        fetchMessage = { tone: 'info', text: m.investments_suggestions_fetch_none() };
      } else {
        fetchMessage = { tone: 'info', text: m.investments_suggestions_fetch_queued({ count: result.queued_count }) };
        pollUntil = Date.now() + 60_000;
        await queryClient.invalidateQueries({ queryKey: investmentEventSuggestionsQueryKey });
      }
    } catch {
      fetchMessage = { tone: 'error', text: m.investments_suggestions_fetch_error() };
    } finally {
      fetchPending = false;
    }
  }

  function statusLabel(status: string): string {
    switch (status) {
      case 'suggested': return m.investments_suggestion_status_suggested();
      case 'accepted': return m.investments_suggestion_status_accepted();
      case 'linked': return m.investments_suggestion_status_linked();
      case 'ignored': return m.investments_suggestion_status_ignored();
      case 'auto_posted': return m.investments_suggestion_status_auto_posted();
      case 'failed': return m.investments_suggestion_status_failed();
      case 'superseded': return m.investments_suggestion_status_superseded();
      default: return status;
    }
  }

  function statusClass(status: string): string {
    switch (status) {
      case 'suggested': return 'bg-warning-soft text-warning';
      case 'accepted':
      case 'linked':
      case 'auto_posted': return 'bg-positive-soft text-positive';
      case 'ignored':
      case 'superseded': return 'bg-surface-strong text-foreground';
      case 'failed': return 'bg-danger-soft text-danger';
      default: return 'bg-surface-strong text-foreground';
    }
  }

  function missingLabel(code: string): string {
    switch (code) {
      case 'payment_date': return m.investments_suggestion_missing_payment_date();
      case 'cash_account': return m.investments_suggestion_missing_cash_account();
      case 'received_amount': return m.investments_suggestion_missing_received_amount();
      case 'dividend_currency': return m.investments_suggestion_missing_dividend_currency();
      case 'amount_per_share': return m.investments_suggestion_missing_amount_per_share();
      case 'amount_owed': return m.investments_suggestion_missing_amount_owed();
      case 'expense_account': return m.investments_suggestion_missing_expense_account();
      default: return code;
    }
  }

  function sourceLabel(code: string): string {
    return code === 'twelve_data' ? m.investments_suggestion_source_twelve_data() : code;
  }

  function amount(decimal: ProviderDecimal | null): string {
    return decimal ? formatQuantity(decimal.value, decimal.scale, getLocale()) : m.investments_suggestion_unknown();
  }

  function content(value: ProviderDividendContent): string {
    const text = amount(value.amountPerShare);
    return value.listingCurrency ? `${text} ${value.listingCurrency}` : text;
  }

  function observed(value: string): string {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat(getLocale(), { dateStyle: 'medium' }).format(date);
  }

  function title(s: InvestmentEventSuggestionResponse): string {
    return (s.instrument_id !== undefined && instrumentNames.get(s.instrument_id)) || m.investments_suggestion_instrument_unknown();
  }

  function confidencePct(bps: number): string {
    return new Intl.NumberFormat(getLocale(), { style: 'percent', maximumFractionDigits: 0 }).format(bps / 10_000);
  }
</script>

{#snippet evidenceValues(evidence: ProviderDividendEvidence, shape: string)}
<dl class="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-3">
  <div>
    <dt class="text-xs text-muted">{m.investments_suggestion_per_share()}</dt>
    <dd class="tabular-nums text-foreground">
      {amount(evidence.amountPerShare)}
      {#if evidence.amountPerShare && evidence.listingCurrency}
        <span class="block text-xs text-muted">{m.investments_suggestion_listing_currency_note({ currency: evidence.listingCurrency })}</span>
      {/if}
    </dd>
  </div>
  <div>
    <dt class="text-xs text-muted">{m.investments_suggestion_units({ date: evidence.entitlementDate })}</dt>
    <dd class="tabular-nums text-foreground">{amount(evidence.entitledUnits)}</dd>
  </div>
  <div>
    <dt class="text-xs text-muted">{shape === 'payment_in_lieu' ? m.investments_suggestion_estimated_owed() : m.investments_suggestion_estimated_gross()}</dt>
    <dd class="tabular-nums text-foreground">{amount(evidence.estimatedGross)}</dd>
  </div>
</dl>
{/snippet}

{#snippet revisionNotice(s: InvestmentEventSuggestionResponse, evidence: ProviderDividendEvidence | null, settled: boolean)}
  {@const steps = providerDividendRevisions(evidence, s.revisions)}
  {#if steps.length > 0}
    <div class="rounded-(--radius-panel) border border-warning bg-warning-soft p-3 text-sm" role="note" data-testid="event-suggestion-revisions">
      <p class="font-medium text-foreground">{m.investments_suggestion_revised_title()}</p>
      <ul class="mt-1 space-y-0.5">
        {#each steps as step, index (index)}
          <li class="tabular-nums text-foreground">
            {m.investments_suggestion_revised_step({ from: content(step.from), to: content(step.to), date: observed(step.observedAt) })}
          </li>
        {/each}
      </ul>
      <p class="mt-1 text-xs text-muted">{settled ? m.investments_suggestion_revised_settled() : m.investments_suggestion_revised_pending()}</p>
    </div>
  {/if}
{/snippet}

<div class="space-y-6">
  <Panel>
    <div class="flex flex-wrap items-start justify-between gap-3">
      <p class="min-w-0 flex-1 text-sm text-muted">{m.investments_suggestions_intro()}</p>
      <button type="button" onclick={checkProviders} disabled={fetchPending}
        class="rounded-(--radius-control) border border-border bg-control px-3 py-1.5 text-sm font-semibold text-foreground transition hover:bg-control-hover disabled:opacity-40">
        {fetchPending ? m.investments_suggestions_fetch_pending() : m.investments_suggestions_fetch()}
      </button>
    </div>
    {#if fetchMessage}
      <p role="status" class="mt-2 text-sm {fetchMessage.tone === 'error' ? 'text-danger' : 'text-foreground'}">{fetchMessage.text}</p>
    {/if}
  </Panel>

  {#if suggestionsQuery.isPending}
    <Panel>
      <p class="text-sm text-muted">{m.investments_suggestions_loading()}</p>
    </Panel>
  {:else if suggestionsQuery.isError}
    <Panel>
      <p class="text-sm text-danger">{m.investments_suggestions_error()}</p>
    </Panel>
  {:else}
    {#if pending.length > 0}
      <ul class="space-y-3" aria-label={m.investments_suggestions_pending_title()}>
        {#each pending as s (s.id)}
          {@const evidence = parseProviderDividendEvidence(s.evidence)}
          {@const shape = proposalShape(s.proposed_transaction)}
          {@const inLieu = paymentInLieuProposal(s.proposed_transaction)}
          <li>
            <Panel>
              <div class="space-y-2" data-testid="event-suggestion">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="rounded-full px-2 py-0.5 text-xs font-medium {statusClass(s.status)}">{statusLabel(s.status)}</span>
                  <h3 class="text-sm font-semibold text-foreground">{title(s)}</h3>
                  <span class="text-xs text-muted">{m.investments_suggestion_ex_date({ date: s.event_date })}</span>
                  <span class="text-xs text-muted">{sourceLabel(s.source_code)}</span>
                  <span class="text-xs text-muted">{m.investments_suggestion_confidence()}: {confidencePct(s.confidence_bps)}</span>
                </div>

                {#if evidence}
                  {@render evidenceValues(evidence, shape)}
                  {#if (shape === 'needs_completion' || shape === 'payment_in_lieu') && evidence.missing.length > 0}
                    <p class="text-xs text-muted">
                      {m.investments_suggestion_needs()}: {evidence.missing.map(missingLabel).join(', ')}
                    </p>
                  {/if}
                {:else if shape === 'complete'}
                  <details>
                    <summary class="cursor-pointer text-xs text-muted hover:text-foreground">{m.investments_suggestion_proposed()}</summary>
                    <pre class="mt-1 overflow-x-auto rounded bg-surface-strong p-2 text-xs text-foreground">{JSON.stringify(s.proposed_transaction, null, 2)}</pre>
                  </details>
                {/if}

                {#if shape === 'informational'}
                  <p class="text-sm text-foreground" role="note">
                    {evidence?.unsupported === 'long_and_short_position' ? m.investments_suggestion_long_and_short_position() : m.investments_suggestion_short_position()}
                  </p>
                {:else if shape === 'payment_in_lieu'}
                  <p class="text-sm text-foreground" role="note">{m.investments_suggestion_payment_in_lieu_note()}</p>
                {/if}

                {@render revisionNotice(s, evidence, false)}

                {#if (shape === 'needs_completion' || shape === 'payment_in_lieu') && s.link_candidates.length > 0}
                  <div class="rounded-(--radius-panel) border border-border p-3">
                    <p class="text-sm font-medium text-foreground">{m.investments_suggestion_link_title()}</p>
                    <ul class="mt-2 space-y-2">
                      {#each s.link_candidates as candidate (candidate.transaction_id)}
                        {@const candidateAmount = formatQuantity(candidate.gross_value, candidate.gross_scale, getLocale())}
                        <li class="flex flex-wrap items-center justify-between gap-2 text-sm" data-testid="event-suggestion-link-candidate">
                          <span class="min-w-0 tabular-nums text-foreground">
                            {#if candidate.holding_account_id !== undefined}
                              {m.investments_suggestion_link_candidate_holding({ date: candidate.transaction_date, amount: candidateAmount, currency: candidate.cash_code,
                                holding: candidate.holding_account_name || `#${candidate.holding_account_id}` })}
                            {:else}
                              {m.investments_suggestion_link_candidate({ date: candidate.transaction_date, amount: candidateAmount, currency: candidate.cash_code })}
                            {/if}
                          </span>
                          <button type="button" disabled={busy.has(s.id)}
                            onclick={() => act(s, () => linkEventSuggestion(s.id, candidate.transaction_id, csrfToken),
                              shape === 'payment_in_lieu' ? m.investments_suggestion_link_payment_in_lieu_error() : m.investments_suggestion_link_error(), false)}
                            class="rounded-(--radius-control) border border-border bg-control px-3 py-1.5 text-xs font-semibold text-foreground transition hover:bg-control-hover disabled:opacity-40">
                            {m.investments_suggestion_link()}
                          </button>
                        </li>
                      {/each}
                    </ul>
                  </div>
                {/if}

                {#if errors.has(s.id)}
                  <p class="text-xs text-danger" role="alert">{errors.get(s.id)}</p>
                {/if}

                <div class="flex flex-wrap justify-end gap-2">
                  {#if shape === 'complete'}
                    <button type="button" disabled={busy.has(s.id)}
                      onclick={() => act(s, () => acceptEventSuggestion(s.id, csrfToken), m.investments_suggestion_accept_error(), true)}
                      class="rounded-(--radius-control) bg-foreground px-3 py-1.5 text-xs font-semibold text-background transition hover:opacity-90 disabled:opacity-40">
                      {m.investments_suggestion_accept()}
                    </button>
                  {:else if (shape === 'needs_completion' || (shape === 'payment_in_lieu' && inLieu)) && completing !== s.id}
                    <button type="button" disabled={busy.has(s.id)} onclick={() => (completing = s.id)}
                      class="rounded-(--radius-control) bg-foreground px-3 py-1.5 text-xs font-semibold text-background transition hover:opacity-90 disabled:opacity-40">
                      {m.investments_suggestion_complete()}
                    </button>
                  {/if}
                  <button type="button" disabled={busy.has(s.id)}
                    onclick={() => act(s, () => ignoreEventSuggestion(s.id, csrfToken), m.investments_suggestion_ignore_error(), false)}
                    class="rounded-(--radius-control) border border-border bg-control px-3 py-1.5 text-xs font-semibold text-foreground transition hover:bg-control-hover disabled:opacity-40">
                    {m.investments_suggestion_ignore()}
                  </button>
                </div>

                {#if completing === s.id && shape === 'payment_in_lieu' && inLieu}
                  <ProviderPaymentInLieuCompletion suggestionID={s.id} proposal={inLieu} {evidence} {csrfToken}
                    onPosted={async () => { completing = null; await refreshAfterPosting(); }}
                    onCancel={() => (completing = null)} />
                {:else if completing === s.id}
                  <ProviderDividendCompletion suggestionID={s.id} commodityID={proposalCommodityID(s.proposed_transaction)} {evidence} {csrfToken}
                    onPosted={async () => { completing = null; await refreshAfterPosting(); }}
                    onCancel={() => (completing = null)} />
                {/if}
              </div>
            </Panel>
          </li>
        {/each}
      </ul>
    {:else}
      <StatePanel title={m.investments_suggestions_empty()} copy={m.investments_suggestions_empty_copy()} />
    {/if}

    {#if history.length > 0}
      <section>
        <h2 class="mb-3 text-sm font-semibold text-foreground">{m.investments_suggestions_history_title()}</h2>
        <ul class="space-y-2">
          {#each history as s (s.id)}
            {@const retained = parseProviderDividendEvidence(s.evidence)}
            {@const paymentID = s.generated_transaction_id ?? s.linked_transaction_id}
            <li>
              <Panel>
                <div class="flex flex-wrap items-center gap-2">
                  <span class="rounded-full px-2 py-0.5 text-xs font-medium {statusClass(s.status)}">{statusLabel(s.status)}</span>
                  <span class="text-sm text-foreground">{title(s)}</span>
                  <span class="text-xs text-muted">{m.investments_suggestion_ex_date({ date: s.event_date })}</span>
                  <span class="text-xs text-muted">{sourceLabel(s.source_code)}</span>
                </div>
                {#if paymentID}
                  <a class="mt-1 inline-block text-sm underline" href={`/app/ledger/transactions?transaction_id=${paymentID}`}>
                    {m.investments_attribution_payment_link({ id: String(paymentID) })}
                  </a>
                {/if}
                {#if retained}
                  <div class="mt-2" data-testid="settled-event-evidence">
                    {@render evidenceValues(retained, proposalShape(s.proposed_transaction))}
                  </div>
                {/if}
                {#if s.failure_reason}
                  <p class="mt-1 text-xs text-danger">{s.failure_reason}</p>
                {/if}
                {#if s.revisions.length > 0}
                  <div class="mt-2">
                    {@render revisionNotice(s, parseProviderDividendEvidence(s.evidence), true)}
                  </div>
                {/if}
              </Panel>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  {/if}
</div>
