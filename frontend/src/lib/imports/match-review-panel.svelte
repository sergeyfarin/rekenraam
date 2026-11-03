<script lang="ts">
  import { parseISO } from 'date-fns';
  import Loader from '@lucide/svelte/icons/loader-circle';
  import Link2 from '@lucide/svelte/icons/link-2';
  import Panel from '#lib/components/panel.svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { parseNormalized, type ImportStagedRow } from '#lib/api/imports.ts';
  import type { CurrencyResponse } from '#lib/api/currencies.ts';
  import { parseDecimalAmount } from '#lib/money/amount.ts';
  import { formatQuantity, joinCommodityAmount } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import {
    choiceKey,
    keepAsNew,
    linkChoice,
    undecidedMatchRows,
    type ImportMatchCandidate,
    type ImportMatchChoice,
    type ImportRowMatchReview
  } from '#lib/imports/match-review.ts';

  let { reviews, rows, choices, currencies, pending, error, decide, onCheck, onChoose }: {
    /** The preview's match reviews; null until the owner checks. */
    reviews: ImportRowMatchReview[] | null;
    rows: ImportStagedRow[];
    choices: ReadonlyMap<number, ImportMatchChoice>;
    currencies: CurrencyResponse[];
    pending: boolean;
    error: unknown;
    /** Commit stopped because rows still need a decision. */
    decide: boolean;
    onCheck: () => void;
    onChoose: (rowId: number, choice: ImportMatchChoice) => void;
  } = $props();

  // #485: rows that match a posting already in the ledger are suggestions.
  // The server found them; this panel shows each candidate with why it
  // matched, and the owner links the row or keeps it as new.
  const locale = $derived(getLocale());
  const dateFormatter = $derived(new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' }));
  const rowsByID = $derived(new Map(rows.map((row) => [row.id, row])));
  const undecided = $derived(new Set(reviews ? undecidedMatchRows(reviews, choices) : []));

  function date(value: string): string {
    return value ? dateFormatter.format(parseISO(value)) : '';
  }

  function money(value: string, scale: number, commodityID: number): string {
    const currency = currencies.find((item) => item.id === commodityID);
    return joinCommodityAmount(currency?.display_symbol || currency?.code || '', formatQuantity(value, scale, locale));
  }

  function rowHeading(review: ImportRowMatchReview): string {
    const row = rowsByID.get(review.row_id);
    const normalized = row ? parseNormalized(row) : undefined;
    // The staged amount is canonical dot-decimal, whatever the file used.
    const amount = normalized ? parseDecimalAmount(normalized.amount, { locale: 'en' }) : null;
    const commodityID = review.candidates[0]?.commodity_id ?? 0;
    return m.import_match_row_heading({
      row: review.row_index + 1,
      date: normalized ? date(normalized.date) : '',
      amount: amount ? money(amount.value, amount.scale, commodityID) : (normalized?.amount ?? '')
    });
  }

  function reason(candidate: ImportMatchCandidate): string {
    const amount = money(candidate.quantity_value, candidate.quantity_scale, candidate.commodity_id);
    const days = Math.abs(candidate.day_offset);
    const n = new Intl.NumberFormat(locale).format(days);
    if (candidate.day_offset > 0) return m.import_match_reason_after({ amount, count: days, n });
    if (candidate.day_offset < 0) return m.import_match_reason_before({ amount, count: days, n });
    return m.import_match_reason_same_day({ amount });
  }

  function candidateLabel(candidate: ImportMatchCandidate): string {
    return m.import_match_link_option({
      description: candidate.description || candidate.payee_name || m.import_match_untitled(),
      date: date(candidate.transaction_date)
    });
  }
</script>

<Panel>
  <div class="flex items-center gap-2">
    <Link2 size={16} class="shrink-0 text-muted" aria-hidden="true" />
    <h2 class="text-sm font-semibold text-foreground">{m.import_match_title()}</h2>
  </div>

  {#if pending}
    <p role="status" class="mt-3 flex items-center gap-2 text-sm text-muted">
      <Loader size={16} class="animate-spin" aria-hidden="true" />
      {m.import_match_checking()}
    </p>
  {:else if reviews === null}
    <button
      type="button"
      class="mt-3 inline-flex min-h-11 items-center rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition-colors hover:bg-control-hover"
      onclick={onCheck}
    >
      {m.import_match_check()}
    </button>
  {:else if reviews.length === 0}
    <p role="status" class="mt-3 text-sm text-muted">{m.import_match_empty()}</p>
  {:else}
    <p class="mt-3 text-sm text-muted">{m.import_match_intro({ days: reviews[0].window_days })}</p>
    {#if decide}
      <p role="alert" class="mt-3 text-sm font-semibold text-warning">{m.import_match_decide()}</p>
    {/if}
    <div class="mt-4 space-y-4">
      {#each reviews as review (review.row_id)}
        {@const current = choiceKey(choices.get(review.row_id))}
        <fieldset class="rounded-(--radius-control) border border-border p-3" data-testid="import-match-row">
          <legend class="px-1 text-sm font-semibold text-foreground">{rowHeading(review)}</legend>
          {#if review.candidates.length > 1 && undecided.has(review.row_id)}
            <p class="mb-2 text-sm text-warning">{m.import_match_choose()}</p>
          {/if}
          <div class="space-y-1">
            {#each review.candidates as candidate (`${candidate.transaction_id}:${candidate.line_key}`)}
              {@const choice = linkChoice(candidate)}
              <label class="flex min-h-11 cursor-pointer items-start gap-3 rounded-(--radius-control) px-2 py-2 hover:bg-control-hover">
                <input
                  type="radio"
                  class="mt-1"
                  name={`import-match-${review.row_id}`}
                  value={choiceKey(choice)}
                  checked={current === choiceKey(choice)}
                  onchange={() => onChoose(review.row_id, choice)}
                />
                <span class="min-w-0">
                  <span class="block text-sm text-foreground">{candidateLabel(candidate)}</span>
                  <span class="block text-sm text-muted">{reason(candidate)}</span>
                </span>
              </label>
            {/each}
            <label class="flex min-h-11 cursor-pointer items-center gap-3 rounded-(--radius-control) px-2 py-2 hover:bg-control-hover">
              <input
                type="radio"
                name={`import-match-${review.row_id}`}
                value="new"
                checked={current === 'new'}
                onchange={() => onChoose(review.row_id, keepAsNew)}
              />
              <span class="text-sm text-foreground">{m.import_match_keep_new()}</span>
            </label>
          </div>
        </fieldset>
      {/each}
    </div>
  {/if}
  <APIFormError {error} id="import-match-error" />
</Panel>
