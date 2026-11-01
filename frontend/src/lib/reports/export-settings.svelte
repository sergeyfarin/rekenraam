<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import Download from '@lucide/svelte/icons/download';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import Panel from '#lib/components/panel.svelte';
  import {
    downloadExport,
    exportPreviewQueryOptions,
    type ExportScope
  } from '#lib/api/maintenance.ts';
  import { getAPIClientErrorMessage } from '#lib/api-error-messages.ts';
  import { m } from '#lib/paraglide/messages.js';

  type ExportFormat = 'bundle' | 'csv' | 'qif';

  // The export screen owns these four choices. An export is a file, not a
  // saved setting: the selections live until the download leaves the page.
  let format = $state<ExportFormat>('bundle');
  let from = $state('');
  let to = $state('');
  let qifDateLayout = $state<'mdy' | 'dmy'>('mdy');
  let allowPartialQIF = $state(false);

  // The flat CSV is always the whole ledger, so a range on it would be a
  // promise the endpoint refuses to keep.
  const scopeApplies = $derived(format !== 'csv');
  const scope = $derived<ExportScope>(
    scopeApplies
      ? { from: from || undefined, to: to || undefined, qifDateLayout }
      : { qifDateLayout }
  );

  const previewQuery = createQuery(() => exportPreviewQueryOptions(scope));

  let exportPending = $state(false);
  let exportError = $state<unknown>(undefined);

  const unsupportedQIF = $derived(previewQuery.data?.qif.unsupported_accounts ?? []);
  const needsQIFConfirmation = $derived(format === 'qif' && unsupportedQIF.length > 0);

  const inputClass =
    'mt-1.5 h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted focus:border-accent disabled:cursor-not-allowed disabled:opacity-70';
  const labelClass = 'block text-xs font-semibold uppercase tracking-[0.12em] text-muted';
  const buttonClass =
    'inline-flex h-10 items-center justify-center gap-2 rounded-(--radius-control) bg-accent px-4 text-sm font-semibold text-accent-foreground transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60';

  async function download() {
    exportPending = true;
    exportError = undefined;
    try {
      const extra = new URLSearchParams();
      if (format === 'qif' && allowPartialQIF) extra.set('allow_partial', 'true');

      const path =
        format === 'bundle'
          ? '/api/v1/exports/bundle.zip'
          : format === 'csv'
            ? '/api/v1/exports/ledger.csv'
            : '/api/v1/exports/qif';

      const { blob, filename } = await downloadExport(path, format === 'csv' ? {} : scope, extra);
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = filename;
      anchor.click();
      // Revoked on a later tick, not on this one. The click only *starts* the
      // download; revoking in the same task can pull the blob out from under a
      // browser that had not begun reading it yet, and the failure mode is a
      // download that silently never arrives — on a screen whose whole purpose
      // is getting the data out.
      setTimeout(() => URL.revokeObjectURL(url), 60_000);
    } catch (error) {
      exportError = error;
    } finally {
      exportPending = false;
    }
  }
</script>

<div class="space-y-6" data-testid="export-settings">
  <Panel>
    <div class="flex items-start gap-3">
      <Download class="mt-0.5 size-5 shrink-0 text-muted" aria-hidden="true" />
      <div class="min-w-0 flex-1">
        <h2 class="text-sm font-semibold text-foreground">{m.settings_data_export_title()}</h2>
        <p class="mt-1 max-w-3xl text-sm leading-6 text-muted">{m.settings_data_export_copy()}</p>
        <p class="mt-2 max-w-3xl text-sm leading-6 text-muted" data-testid="export-not-backup">
          {m.settings_data_export_not_backup()}
        </p>

        <fieldset class="mt-5">
          <legend class={labelClass}>{m.settings_data_export_format()}</legend>
          <div class="mt-2 space-y-2">
            {#each [['bundle', m.settings_data_export_format_bundle(), m.settings_data_export_format_bundle_hint()], ['csv', m.settings_data_export_format_csv(), m.settings_data_export_format_csv_hint()], ['qif', m.settings_data_export_format_qif(), m.settings_data_export_format_qif_hint()]] as [value, label, hint] (value)}
              <label class="flex cursor-pointer items-start gap-3 rounded-(--radius-control) border border-border p-3 transition hover:border-accent">
                <input
                  type="radio"
                  name="export-format"
                  value={value}
                  checked={format === value}
                  onchange={() => (format = value as ExportFormat)}
                  class="mt-1"
                />
                <span class="min-w-0">
                  <span class="block text-sm font-medium text-foreground">{label}</span>
                  <span class="mt-0.5 block text-xs leading-5 text-muted">{hint}</span>
                </span>
              </label>
            {/each}
          </div>
        </fieldset>

        {#if scopeApplies}
          <div class="mt-4 grid gap-4 sm:grid-cols-2">
            <label class="block">
              <span class={labelClass}>{m.settings_data_export_from()}</span>
              <input type="date" bind:value={from} class={inputClass} />
            </label>
            <label class="block">
              <span class={labelClass}>{m.settings_data_export_to()}</span>
              <input type="date" bind:value={to} class={inputClass} />
            </label>
          </div>
        {/if}

        {#if format === 'qif'}
          <label class="mt-4 block max-w-xs">
            <span class={labelClass}>{m.settings_data_export_date_layout()}</span>
            <select bind:value={qifDateLayout} class={inputClass}>
              <option value="mdy">MM/DD/YYYY</option>
              <option value="dmy">DD/MM/YYYY</option>
            </select>
            <span class="mt-1.5 block text-xs leading-5 text-muted">
              {m.settings_data_export_date_layout_hint()}
            </span>
          </label>
        {/if}

        <div class="mt-5 rounded-(--radius-control) border border-border bg-surface-strong p-4">
          {#if previewQuery.isPending}
            <p class="text-sm text-muted">{m.settings_status_loading()}</p>
          {:else if previewQuery.isError}
            <p role="alert" class="text-sm text-danger">{getAPIClientErrorMessage(previewQuery.error)}</p>
          {:else if previewQuery.data && previewQuery.data.ledger.posting_count > 0}
            <p class="text-sm text-foreground" data-testid="export-summary">
              {m.settings_data_export_summary({
                postings: previewQuery.data.ledger.posting_count,
                accounts: previewQuery.data.ledger.account_count,
                from: previewQuery.data.ledger.earliest_entry_date ?? '—',
                to: previewQuery.data.ledger.latest_entry_date ?? '—'
              })}
            </p>
            <p class="mt-3 text-xs font-semibold uppercase tracking-[0.12em] text-muted">
              {m.settings_data_export_excluded()}
            </p>
            <ul class="mt-1.5 space-y-0.5 text-xs leading-5 text-muted">
              {#each previewQuery.data.excluded as item (item)}
                <li>{item}</li>
              {/each}
            </ul>
          {:else}
            <p class="text-sm text-muted" data-testid="export-summary">
              {m.settings_data_export_summary_empty()}
            </p>
          {/if}
        </div>

        {#if needsQIFConfirmation}
          <div class="mt-4 rounded-(--radius-control) border border-warning/40 bg-warning-soft p-4">
            <p class="text-sm font-medium text-foreground">{m.settings_data_export_qif_excluded()}</p>
            <ul class="mt-2 space-y-0.5 text-sm text-muted">
              {#each unsupportedQIF as account (account.account_id)}
                <li>{account.account_path}</li>
              {/each}
            </ul>
            <label class="mt-3 flex items-center gap-2 text-sm text-foreground">
              <input type="checkbox" bind:checked={allowPartialQIF} data-testid="qif-allow-partial" />
              {m.settings_data_export_qif_confirm()}
            </label>
          </div>
        {/if}

        <div class="mt-5 flex flex-wrap items-center gap-3">
          <button
            type="button"
            class={buttonClass}
            onclick={download}
            disabled={exportPending || (needsQIFConfirmation && !allowPartialQIF)}
            data-testid="export-download"
          >
            {exportPending ? m.settings_data_export_downloading() : m.settings_data_export_download()}
          </button>
        </div>
        <APIFormError error={exportError} />
      </div>
    </div>
  </Panel>
</div>
