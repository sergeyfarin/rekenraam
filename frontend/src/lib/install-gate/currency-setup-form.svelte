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
    selectedCodes = $bindable<string[]>([]),
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
    selectedCodes: string[];
    defaultCode: string;
    searchCode: string;
    pending: boolean;
    onsubmit: (event: SubmitEvent) => void;
  } = $props();

  const selectedCurrencies = $derived.by(() =>
    selectedCodes.map((code) => catalog.find((currency) => currency.code === code)).filter((currency) => currency !== undefined)
  );

  const availableCurrencyOptions = $derived.by(() => catalog.filter((currency) => !selectedCodes.includes(currency.code)));

  function toggleCurrency(code: string) {
    if (selectedCodes.includes(code)) {
      selectedCodes = selectedCodes.filter((selectedCode) => selectedCode !== code);

      if (defaultCode === code) {
        defaultCode = selectedCodes[0] ?? '';
      }

      return;
    }

    selectedCodes = [...selectedCodes, code];
    defaultCode ||= code;
  }

  function addCurrencyFromSearch() {
    const code = searchCode.trim().toUpperCase();

    if (!code || selectedCodes.includes(code)) {
      searchCode = '';
      return;
    }

    if (!catalog.some((currency) => currency.code === code)) {
      return;
    }

    selectedCodes = [...selectedCodes, code];
    defaultCode ||= code;
    searchCode = '';
  }

  function removeSelectedCurrency(code: string) {
    selectedCodes = selectedCodes.filter((selectedCode) => selectedCode !== code);

    if (defaultCode === code) {
      defaultCode = selectedCodes[0] ?? '';
    }
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
              class:border-accent={selectedCodes.includes(code)}
              class:bg-accent={selectedCodes.includes(code)}
              class:text-accent-foreground={selectedCodes.includes(code)}
              class:bg-surface-strong={!selectedCodes.includes(code)}
              class:text-foreground={!selectedCodes.includes(code)}
              class="min-h-16 rounded-2xl border px-3 py-2 text-left transition hover:border-accent"
              aria-pressed={selectedCodes.includes(code)}
              onclick={() => toggleCurrency(code)}
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
        <span class="text-sm font-medium text-foreground">{m.install_gate_add_currency_label()}</span>
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
              addCurrencyFromSearch();
            }
          }}
        />
      </label>
      <datalist id="currency-options">
        {#each availableCurrencyOptions as currency}
          <option value={currency.code}>{currency.label}</option>
        {/each}
      </datalist>
      <button
        type="button"
        class="inline-flex w-full items-center justify-center rounded-full border border-border bg-surface px-5 py-3 text-sm font-semibold text-foreground transition hover:bg-surface-strong disabled:cursor-not-allowed disabled:opacity-60"
        onclick={addCurrencyFromSearch}
        disabled={searchCode.trim() === ''}
      >
        {m.install_gate_add_currency_submit()}
      </button>
    </div>

    <div class="space-y-3">
      <span class="text-sm font-medium text-foreground">{m.install_gate_selected_currencies_label()}</span>

      {#if selectedCurrencies.length === 0}
        <div class="rounded-[1.75rem] border border-border bg-surface-strong/60 p-5 text-sm leading-6 text-muted">
          {m.install_gate_selected_currencies_empty()}
        </div>
      {:else}
        <div class="flex flex-wrap gap-2">
          {#each selectedCurrencies as currency}
            <span class="inline-flex items-center gap-2 rounded-full border border-border bg-surface-strong px-3 py-2 text-sm text-foreground">
              <span class="font-semibold">{currency.code}</span>
              <span class="max-w-40 truncate text-muted">{currency.name}</span>
              <button
                type="button"
                class="rounded-full px-2 text-muted transition hover:bg-surface hover:text-foreground"
                aria-label={m.install_gate_remove_currency({ code: currency.code })}
                onclick={() => removeSelectedCurrency(currency.code)}
              >
                x
              </button>
            </span>
          {/each}
        </div>
      {/if}
    </div>

    <label class="block space-y-2">
      <span class="text-sm font-medium text-foreground">{m.install_gate_default_currency_label()}</span>
      <select
        bind:value={defaultCode}
        name="default-currency"
        class="w-full rounded-2xl border border-border bg-surface-strong/40 px-4 py-3 text-base text-foreground"
        required
      >
        {#each selectedCurrencies as currency}
          <option value={currency.code}>{currency.label}</option>
        {/each}
      </select>
    </label>

    <button
      type="submit"
      class="inline-flex w-full items-center justify-center rounded-full bg-foreground px-5 py-3 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
      disabled={pending || catalogPending || selectedCurrencies.length === 0 || !defaultCode}
    >
      {pending ? m.install_gate_currencies_submit_pending() : m.install_gate_currencies_submit()}
    </button>
  {/if}
</form>
