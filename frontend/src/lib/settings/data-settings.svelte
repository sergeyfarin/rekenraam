<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import Database from '@lucide/svelte/icons/database';
  import Download from '@lucide/svelte/icons/download';
  import HeartPulse from '@lucide/svelte/icons/heart-pulse';
  import KeyRound from '@lucide/svelte/icons/key-round';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import Panel from '$lib/components/panel.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import { authSessionQueryOptions } from '$lib/api/auth';
  import {
    backupStatusQueryKey,
    backupStatusQueryOptions,
    downloadExport,
    exportPreviewQueryOptions,
    requestBackup,
    retryBackupRun,
    runSelfCheck,
    saveBackupPolicy,
    selfCheckQueryKey,
    selfCheckQueryOptions,
    type ExportScope,
    type SelfCheckRun
  } from '$lib/api/maintenance';
  import { getAPIClientErrorMessage } from '$lib/api-error-messages';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';

  type ExportFormat = 'bundle' | 'csv' | 'qif';

  /**
   * The four fields this form owns. Spelled out rather than reusing
   * BackupPolicyRequest, whose fields are all optional because a partial update
   * is a valid request — the form always holds all four.
   */
  type BackupPolicyDraft = {
    enabled: boolean;
    hour_local: number;
    minute_local: number;
    retention_count: number;
  };

  const queryClient = useQueryClient();
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const csrfToken = $derived(sessionQuery.data?.csrf_token ?? '');
  const locale = $derived(getLocale());

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
  const backupQuery = createQuery(() => backupStatusQueryOptions());
  const selfCheckQuery = createQuery(() => selfCheckQueryOptions());

  let exportPending = $state(false);
  let exportError = $state<unknown>(undefined);

  let policyPending = $state(false);
  let policyError = $state<unknown>(undefined);
  let policySaved = $state('');
  let backupPending = $state(false);
  let backupQueued = $state('');

  let selfCheckPending = $state(false);
  let selfCheckError = $state<unknown>(undefined);
  let selfCheckJustRun = $state<SelfCheckRun | undefined>(undefined);

  const serverPolicy = $derived(backupQuery.data?.policy);

  /**
   * The backup form's own state.
   *
   * Binding the inputs straight to `backupQuery.data.policy` wrote every
   * keystroke into the TanStack cache, which is shared and outlives this
   * component: an edit the user abandoned came back on the next visit reading
   * as the configured schedule. The form therefore holds a copy, and the cache
   * holds only what the server said.
   */
  let policy = $state<BackupPolicyDraft | undefined>(undefined);
  // What the server last said, so a routine background refetch returning the
  // same answer leaves an in-progress edit alone while a real change — someone
  // saving from another tab — still wins.
  let seededFrom = $state('');

  $effect(() => {
    if (!serverPolicy) return;
    const fingerprint = JSON.stringify([
      serverPolicy.enabled,
      serverPolicy.hour_local,
      serverPolicy.minute_local,
      serverPolicy.retention_count
    ]);
    if (fingerprint === seededFrom) return;
    seededFrom = fingerprint;
    policy = {
      enabled: serverPolicy.enabled,
      hour_local: serverPolicy.hour_local,
      minute_local: serverPolicy.minute_local,
      retention_count: serverPolicy.retention_count
    };
  });
  const unsupportedQIF = $derived(previewQuery.data?.qif.unsupported_accounts ?? []);
  const needsQIFConfirmation = $derived(format === 'qif' && unsupportedQIF.length > 0);

  const latestSelfCheck = $derived(selfCheckJustRun ?? selfCheckQuery.data?.latest_run);

  const inputClass =
    'mt-1.5 h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted focus:border-accent disabled:cursor-not-allowed disabled:opacity-70';
  const labelClass = 'block text-xs font-semibold uppercase tracking-[0.12em] text-muted';
  const buttonClass =
    'inline-flex h-10 items-center justify-center gap-2 rounded-(--radius-control) bg-accent px-4 text-sm font-semibold text-accent-foreground transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60';
  const secondaryButtonClass =
    'inline-flex h-9 items-center justify-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 text-sm font-medium text-foreground transition hover:border-accent disabled:cursor-not-allowed disabled:opacity-60';

  function formatDateTime(value: string | undefined): string {
    if (!value) return '—';
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime())
      ? value
      : new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(parsed);
  }

  function formatBytes(value: number | undefined): string {
    if (value === undefined) return '—';
    const units = ['B', 'kB', 'MB', 'GB'];
    let size = value;
    let unit = 0;
    while (size >= 1024 && unit < units.length - 1) {
      size /= 1024;
      unit++;
    }
    return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(size)} ${units[unit]}`;
  }

  function statusLabel(status: string): string {
    switch (status) {
      case 'passed':
        return m.settings_data_status_passed();
      case 'failed':
        return m.settings_data_status_failed();
      case 'not_applicable':
        return m.settings_data_status_not_applicable();
      case 'pending':
        return m.settings_data_status_pending();
      case 'running':
        return m.settings_data_status_running();
      case 'completed':
        return m.settings_data_status_completed();
      default:
        return status;
    }
  }

  function statusTone(status: string): 'positive' | 'danger' | 'neutral' {
    if (status === 'passed' || status === 'completed') return 'positive';
    if (status === 'failed') return 'danger';
    return 'neutral';
  }

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

  async function savePolicy(event: SubmitEvent) {
    event.preventDefault();
    if (!policy) return;

    policyPending = true;
    policyError = undefined;
    policySaved = '';
    try {
      await saveBackupPolicy(
        {
          enabled: policy.enabled,
          hour_local: policy.hour_local,
          minute_local: policy.minute_local,
          retention_count: policy.retention_count
        },
        csrfToken
      );
      policySaved = m.settings_data_backups_saved();
      await queryClient.invalidateQueries({ queryKey: backupStatusQueryKey });
    } catch (error) {
      policyError = error;
    } finally {
      policyPending = false;
    }
  }

  async function backUpNow() {
    backupPending = true;
    policyError = undefined;
    backupQueued = '';
    try {
      await requestBackup(csrfToken);
      backupQueued = m.settings_data_backups_queued();
      await queryClient.invalidateQueries({ queryKey: backupStatusQueryKey });
    } catch (error) {
      policyError = error;
    } finally {
      backupPending = false;
    }
  }

  async function retryRun(runID: number) {
    policyError = undefined;
    try {
      await retryBackupRun(runID, csrfToken);
      await queryClient.invalidateQueries({ queryKey: backupStatusQueryKey });
    } catch (error) {
      policyError = error;
    }
  }

  async function check() {
    selfCheckPending = true;
    selfCheckError = undefined;
    try {
      selfCheckJustRun = await runSelfCheck(csrfToken);
      await queryClient.invalidateQueries({ queryKey: selfCheckQueryKey });
    } catch (error) {
      selfCheckError = error;
    } finally {
      selfCheckPending = false;
    }
  }
</script>

<div class="space-y-6" data-testid="data-settings">
  <!-- Export -->
  <Panel>
    <div class="flex items-start gap-3">
      <Download class="mt-0.5 size-5 shrink-0 text-muted" aria-hidden="true" />
      <div class="min-w-0 flex-1">
        <h2 class="text-sm font-semibold text-foreground">{m.settings_data_export_title()}</h2>
        <p class="mt-1 max-w-3xl text-sm leading-6 text-muted">{m.settings_data_export_copy()}</p>

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

        <div class="mt-5 rounded-(--radius-control) border border-border bg-subtle p-4">
          {#if previewQuery.isPending}
            <p class="text-sm text-muted">{m.settings_status_loading()}</p>
          {:else if previewQuery.isError}
            <p class="text-sm text-danger">{getAPIClientErrorMessage(previewQuery.error)}</p>
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
          <div class="mt-4 rounded-(--radius-control) border border-warning-border bg-warning-subtle p-4">
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

  <!-- Backups -->
  <Panel>
    <div class="flex items-start gap-3">
      <Database class="mt-0.5 size-5 shrink-0 text-muted" aria-hidden="true" />
      <div class="min-w-0 flex-1">
        <h2 class="text-sm font-semibold text-foreground">{m.settings_data_backups_title()}</h2>
        <p class="mt-1 max-w-3xl text-sm leading-6 text-muted">{m.settings_data_backups_copy()}</p>

        {#if backupQuery.isPending}
          <p class="mt-4 text-sm text-muted">{m.settings_status_loading()}</p>
        {:else if backupQuery.isError}
          <p class="mt-4 text-sm text-danger">{getAPIClientErrorMessage(backupQuery.error)}</p>
        {:else if backupQuery.data}
          <dl class="mt-4 grid gap-4 sm:grid-cols-3">
            <div>
              <dt class={labelClass}>{m.settings_data_backups_last()}</dt>
              <dd class="mt-1 text-sm text-foreground" data-testid="backup-last">
                {backupQuery.data.last_success
                  ? formatDateTime(backupQuery.data.last_success.finished_at)
                  : m.settings_data_backups_never()}
              </dd>
            </div>
            <div>
              <dt class={labelClass}>{m.settings_data_backups_next()}</dt>
              <dd class="mt-1 text-sm text-foreground">{formatDateTime(backupQuery.data.next_run_at)}</dd>
            </div>
            <div class="min-w-0">
              <dt class={labelClass}>{m.settings_data_backups_directory()}</dt>
              <dd class="mt-1 truncate font-mono text-xs text-muted" title={backupQuery.data.directory}>
                {backupQuery.data.directory}
              </dd>
            </div>
          </dl>

          <div class="mt-4 flex items-start gap-3 rounded-(--radius-control) border border-border bg-subtle p-4">
            <KeyRound class="mt-0.5 size-4 shrink-0 text-muted" aria-hidden="true" />
            <div>
              <p class="text-sm font-medium text-foreground">{m.settings_data_backups_key_title()}</p>
              <p class="mt-1 text-xs leading-5 text-muted">{backupQuery.data.secret_key.consequence}</p>
            </div>
          </div>

          {#if policy}
            <form class="mt-5 grid gap-4 sm:grid-cols-3" onsubmit={savePolicy}>
              <label class="flex items-center gap-2 text-sm text-foreground sm:col-span-3">
                <input type="checkbox" bind:checked={policy.enabled} data-testid="backup-enabled" />
                {m.settings_data_backups_enabled()}
              </label>
              <label class="block">
                <span class={labelClass}>{m.settings_data_backups_time()}</span>
                <input
                  type="time"
                  class={inputClass}
                  value={`${String(policy.hour_local).padStart(2, '0')}:${String(policy.minute_local).padStart(2, '0')}`}
                  onchange={(event) => {
                    const [hour, minute] = event.currentTarget.value.split(':');
                    if (policy) {
                      policy.hour_local = Number(hour);
                      policy.minute_local = Number(minute);
                    }
                  }}
                />
                <span class="mt-1.5 block text-xs text-muted">{serverPolicy?.time_zone}</span>
              </label>
              <label class="block">
                <span class={labelClass}>{m.settings_data_backups_retention()}</span>
                <input type="number" min="1" bind:value={policy.retention_count} class={inputClass} data-testid="backup-retention" />
                <span class="mt-1.5 block text-xs text-muted">
                  {m.settings_data_backups_retention_unit()}
                </span>
              </label>
              <div class="flex items-end gap-3">
                <button type="submit" class={buttonClass} disabled={policyPending}>
                  {m.settings_data_backups_save()}
                </button>
                <button
                  type="button"
                  class={secondaryButtonClass}
                  onclick={backUpNow}
                  disabled={backupPending}
                  data-testid="backup-now"
                >
                  {m.settings_data_backups_run_now()}
                </button>
              </div>
            </form>
            {#if policySaved}
              <p class="mt-2 text-sm text-positive">{policySaved}</p>
            {/if}
            {#if backupQueued}
              <p class="mt-2 text-sm text-muted" data-testid="backup-queued">{backupQueued}</p>
            {/if}
            <APIFormError error={policyError} />
          {/if}

          {#if backupQuery.data.runs.length > 0}
            <h3 class="mt-6 text-xs font-semibold uppercase tracking-[0.12em] text-muted">
              {m.settings_data_backups_history()}
            </h3>
            <table class="mt-2 w-full text-left text-sm">
              <thead class="text-xs uppercase tracking-[0.08em] text-muted">
                <tr>
                  <th scope="col" class="py-2 pr-3 font-medium">{m.settings_data_backups_last()}</th>
                  <th scope="col" class="py-2 pr-3 font-medium">{m.settings_data_status_passed()}</th>
                  <th scope="col" class="py-2 pr-3 font-medium text-right">{m.settings_data_backups_retention()}</th>
                  <th scope="col" class="py-2"><span class="sr-only">{m.settings_data_backups_retry()}</span></th>
                </tr>
              </thead>
              <tbody>
                {#each backupQuery.data.runs as run (run.id)}
                  <tr class="border-t border-border" data-testid="backup-run">
                    <td class="py-2 pr-3 text-foreground">{formatDateTime(run.finished_at || run.created_at)}</td>
                    <td class="py-2 pr-3">
                      <StatusBadge tone={run.will_retry ? 'neutral' : statusTone(run.status)}>
                        {run.status === 'failed'
                          ? run.will_retry
                            ? m.settings_data_backups_will_retry()
                            : m.settings_data_backups_gave_up()
                          : statusLabel(run.status)}
                      </StatusBadge>
                      {#if run.error_summary}
                        <span class="ml-2 text-xs text-danger">{run.error_summary}</span>
                      {/if}
                    </td>
                    <td class="py-2 pr-3 text-right tabular-nums text-muted">{formatBytes(run.byte_size)}</td>
                    <td class="py-2 text-right">
                      {#if run.status === 'failed' && !run.will_retry}
                        <button type="button" class={secondaryButtonClass} onclick={() => retryRun(run.id)}>
                          {m.settings_data_backups_retry()}
                        </button>
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        {/if}
      </div>
    </div>
  </Panel>

  <!-- Health check -->
  <Panel>
    <div class="flex items-start gap-3">
      <HeartPulse class="mt-0.5 size-5 shrink-0 text-muted" aria-hidden="true" />
      <div class="min-w-0 flex-1">
        <h2 class="text-sm font-semibold text-foreground">{m.settings_data_selfcheck_title()}</h2>
        <p class="mt-1 max-w-3xl text-sm leading-6 text-muted">{m.settings_data_selfcheck_copy()}</p>

        <div class="mt-4 flex flex-wrap items-center gap-3">
          <button
            type="button"
            class={buttonClass}
            onclick={check}
            disabled={selfCheckPending}
            data-testid="selfcheck-run"
          >
            {selfCheckPending ? m.settings_data_selfcheck_running() : m.settings_data_selfcheck_run()}
          </button>
          {#if latestSelfCheck}
            <span class="text-sm text-muted">
              {m.settings_data_selfcheck_last_run({ when: formatDateTime(latestSelfCheck.finished_at) })}
            </span>
          {/if}
        </div>
        <APIFormError error={selfCheckError} />

        {#if selfCheckQuery.isPending && !latestSelfCheck}
          <p class="mt-4 text-sm text-muted">{m.settings_status_loading()}</p>
        {:else if selfCheckQuery.isError && !latestSelfCheck}
          <p class="mt-4 text-sm text-danger">{getAPIClientErrorMessage(selfCheckQuery.error)}</p>
        {:else if !latestSelfCheck}
          <p class="mt-4 text-sm text-muted" data-testid="selfcheck-summary">
            {m.settings_data_selfcheck_never()}
          </p>
        {:else}
          <p class="mt-4 text-sm text-foreground" data-testid="selfcheck-summary">
            {latestSelfCheck.status === 'passed'
              ? m.settings_data_selfcheck_passed()
              : m.settings_data_selfcheck_failed({ count: latestSelfCheck.failed_check_count })}
          </p>
          <table class="mt-3 w-full text-left text-sm">
            <thead class="text-xs uppercase tracking-[0.08em] text-muted">
              <tr>
                <th scope="col" class="py-2 pr-3 font-medium">{m.settings_data_selfcheck_title()}</th>
                <th scope="col" class="py-2 pr-3 font-medium">{m.settings_data_status_passed()}</th>
              </tr>
            </thead>
            <tbody>
              {#each latestSelfCheck.results as result (result.check_id)}
                <tr class="border-t border-border align-top" data-testid="selfcheck-result">
                  <td class="py-2 pr-3">
                    <span class="block font-medium text-foreground">{result.summary}</span>
                    <span class="mt-0.5 block text-xs leading-5 text-muted">{result.explanation}</span>
                    {#if result.status === 'failed'}
                      <span class="mt-1 block text-xs leading-5 text-danger">{result.next_step}</span>
                    {/if}
                  </td>
                  <td class="py-2 pr-3">
                    <StatusBadge tone={statusTone(result.status)}>{statusLabel(result.status)}</StatusBadge>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </div>
    </div>
  </Panel>
</div>
