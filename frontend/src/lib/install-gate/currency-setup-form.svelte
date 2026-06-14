<script lang="ts">
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { m } from '$lib/paraglide/messages.js';
  import type { LocalizedCurrencyCatalogEntry } from './currency-options';

  let {
    error,
    catalogError,
    catalogPending,
    catalog,
    quickCodes,
    defaultCode = $bindable(''),
    searchCode = $bindable(''),
    pending,
    onsubmit
  }: {
    error: unknown;
    catalogError: unknown;
    catalogPending: boolean;
    catalog: LocalizedCurrencyCatalogEntry[];
    quickCodes: string[];
    defaultCode: string;
    searchCode: string;
    pending: boolean;
    onsubmit: (event: SubmitEvent) => void;
  } = $props();

  const selectedCurrency = $derived(catalog.find((currency) => currency.code === defaultCode));

  function chooseCurrencyFromSearch() {
    const code = searchCode.trim().toUpperCase();

    if (!code) {
      searchCode = '';
      return;
    }

    if (!catalog.some((currency) => currency.code === code)) {
      return;
    }

    defaultCode = code;
    searchCode = '';
  }
</script>

<form class="space-y-4" {onsubmit}>
  <APIFormError error={error ?? catalogError} id="currency-form-error" />

  {#if catalogPending}
    <div class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-5 text-sm leading-6 text-muted">
      {m.install_gate_currencies_loading()}
    </div>
  {:else if catalog.length === 0}
    <div class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-5 text-sm leading-6 text-muted">
      {m.install_gate_currencies_empty()}
    </div>
  {:else}
    <div class="space-y-3">
      <span class="text-sm font-medium text-foreground">{m.install_gate_quick_currencies_label()}</span>
      <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
        {#each quickCodes as code}
          {@const currency = catalog.find((entry) => entry.code === code)}
          {#if currency}
            <button
              type="button"
              class:border-accent={defaultCode === code}
              class:bg-accent={defaultCode === code}
              class:text-accent-foreground={defaultCode === code}
              class:bg-surface-strong={defaultCode !== code}
              class:text-foreground={defaultCode !== code}
              class="min-h-16 rounded-2xl border px-3 py-2 text-left transition hover:border-accent"
              aria-pressed={defaultCode === code}
              onclick={() => (defaultCode = code)}
            >
              <span class="block text-sm font-semibold">{currency.code}</span>
              <span class="block truncate text-xs opacity-80">{currency.name}</span>
            </button>
          {/if}
        {/each}
      </div>
    </div>

    <div class="space-y-2">
      <label class="block space-y-2">
        <span class="text-sm font-medium text-foreground">{m.install_gate_default_currency_label()}</span>
        <input
          bind:value={searchCode}
          list="currency-options"
          name="currency-search"
          autocomplete="off"
          class="w-full rounded-2xl border border-border bg-surface-strong/40 px-4 py-3 text-base text-foreground placeholder:text-muted"
          placeholder={m.install_gate_add_currency_placeholder()}
          onkeydown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault();
              chooseCurrencyFromSearch();
            }
          }}
        />
      </label>
      <datalist id="currency-options">
        {#each catalog as currency}
          <option value={currency.code}>{currency.label}</option>
        {/each}
      </datalist>
      <button
        type="button"
        class="inline-flex w-full items-center justify-center rounded-full border border-border bg-surface px-5 py-3 text-sm font-semibold text-foreground transition hover:bg-surface-strong disabled:cursor-not-allowed disabled:opacity-60"
        onclick={chooseCurrencyFromSearch}
        disabled={searchCode.trim() === ''}
      >
        {m.install_gate_choose_currency_submit()}
      </button>
    </div>

    {#if selectedCurrency}
      <div class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-5 text-sm leading-6 text-muted">
        {m.install_gate_default_currency_selected({ code: selectedCurrency.code, name: selectedCurrency.name })}
      </div>
    {/if}

    <button
      type="submit"
      class="inline-flex w-full items-center justify-center rounded-full bg-foreground px-5 py-3 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
      disabled={pending || catalogPending || !defaultCode}
    >
      {pending ? m.install_gate_currencies_submit_pending() : m.install_gate_currencies_submit()}
    </button>
  {/if}
</form>
