<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import Database from '@lucide/svelte/icons/database';
  import Download from '@lucide/svelte/icons/download';
  import HeartPulse from '@lucide/svelte/icons/heart-pulse';
  import KeyRound from '@lucide/svelte/icons/key-round';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import Panel from '#lib/components/panel.svelte';
  import StatusBadge from '#lib/components/status-badge.svelte';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import {
    backupStatusQueryKey,
    backupStatusQueryOptions,
    requestBackup,
    retryBackupRun,
    runSelfCheck,
    saveBackupPolicy,
    selfCheckQueryKey,
    selfCheckQueryOptions,
    type SelfCheckRun
  } from '#lib/api/maintenance.ts';
  import { getAPIClientErrorMessage } from '#lib/api-error-messages.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { attributionReason } from '#lib/investments/attribution-reason.ts';

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

  const backupQuery = createQuery(() => backupStatusQueryOptions());
  const selfCheckQuery = createQuery(() => selfCheckQueryOptions());

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
      case 'incomplete':
        return m.settings_data_status_incomplete();
      case 'warning':
        return m.settings_data_status_warning();
      case 'errored':
        return m.settings_data_status_errored();
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

  function statusTone(status: string): 'positive' | 'danger' | 'warning' | 'neutral' {
    if (status === 'passed' || status === 'completed') return 'positive';
    if (status === 'failed' || status === 'errored') return 'danger';
    // Incomplete is neither a pass nor damage: some information is unknown.
    if (status === 'incomplete' || status === 'warning') return 'warning';
    return 'neutral';
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
    selfCheckJustRun = undefined;
    try {
      selfCheckJustRun = await runSelfCheck(csrfToken);
      await queryClient.invalidateQueries({ queryKey: selfCheckQueryKey });
    } catch (error) {
      selfCheckError = error;
      // A failed request can still have durably recorded an `errored` run.
      // Refetch so the new attempt replaces any older verdict on screen.
      await queryClient.invalidateQueries({ queryKey: selfCheckQueryKey });
    } finally {
      selfCheckPending = false;
    }
  }
</script>

<div class="space-y-6" data-testid="data-settings">
  <!-- Export lives in Reports (#446); this page keeps the complete-copy work. -->
  <Panel variant="toolbar">
    <div class="flex items-start gap-3">
      <Download class="mt-0.5 size-5 shrink-0 text-muted" aria-hidden="true" />
      <div class="min-w-0 flex-1">
        <h2 class="text-sm font-semibold text-foreground">{m.settings_data_export_link_title()}</h2>
        <p class="mt-1 max-w-3xl text-sm leading-6 text-muted">{m.settings_data_export_link_copy()}</p>
        <a href="/app/reports/export" class={secondaryButtonClass + ' mt-3'}>
          {m.settings_data_export_link_action()}
        </a>
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
        <p class="mt-2 max-w-3xl text-sm leading-6 text-muted" data-testid="backup-complete-copy">
          {m.settings_data_backups_complete()}
        </p>

        {#if backupQuery.isPending}
          <p class="mt-4 text-sm text-muted">{m.settings_status_loading()}</p>
        {:else if backupQuery.isError}
          <p role="alert" class="mt-4 text-sm text-danger">{getAPIClientErrorMessage(backupQuery.error)}</p>
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

          <div class="mt-4 flex items-start gap-3 rounded-(--radius-control) border border-border bg-surface-strong p-4">
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
              {m.settings_data_selfcheck_historical()}
            </span>
          {/if}
        </div>
        <APIFormError error={selfCheckError} />

        {#if selfCheckQuery.isPending && !latestSelfCheck}
          <p class="mt-4 text-sm text-muted">{m.settings_status_loading()}</p>
        {:else if selfCheckQuery.isError && !latestSelfCheck}
          <p role="alert" class="mt-4 text-sm text-danger">{getAPIClientErrorMessage(selfCheckQuery.error)}</p>
        {:else if !latestSelfCheck}
          <p class="mt-4 text-sm text-muted" data-testid="selfcheck-summary">
            {m.settings_data_selfcheck_never()}
          </p>
        {:else}
          <p class="mt-4 text-sm text-foreground" data-testid="selfcheck-summary">
            {#if latestSelfCheck.status === 'passed'}
              {m.settings_data_selfcheck_passed()}
            {:else if latestSelfCheck.status === 'errored'}
              {m.settings_data_selfcheck_errored({ reason: latestSelfCheck.error_summary })}
            {:else if latestSelfCheck.status === 'incomplete'}
              {m.settings_data_selfcheck_incomplete({ count: latestSelfCheck.incomplete_check_count })}
            {:else if latestSelfCheck.status === 'warning'}
              {m.settings_data_selfcheck_warning({ count: latestSelfCheck.warning_check_count })}
              {#if latestSelfCheck.incomplete_check_count > 0}
                {m.settings_data_selfcheck_failed_unavailable({ count: latestSelfCheck.incomplete_check_count })}
              {/if}
            {:else}
              {m.settings_data_selfcheck_failed({ count: latestSelfCheck.failed_check_count })}
              {#if latestSelfCheck.incomplete_check_count > 0}
                {m.settings_data_selfcheck_failed_unavailable({
                  count: latestSelfCheck.incomplete_check_count
                })}
              {/if}
            {/if}
            {#if latestSelfCheck.warning_check_count > 0 && latestSelfCheck.status !== 'warning'}
              {m.settings_data_selfcheck_warning({ count: latestSelfCheck.warning_check_count })}
            {/if}
          </p>
          {#if latestSelfCheck.results.length > 0}
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
                      <span class="block font-medium text-foreground">{result.check_id === 'investment_attribution' ? m.settings_data_selfcheck_attribution_title() : result.summary}</span>
                      <span class="mt-0.5 block text-xs leading-5 text-muted">{result.check_id === 'investment_attribution' ? m.settings_data_selfcheck_attribution_explanation() : result.explanation}</span>
                      {#if result.warning_count > 0}
                        <p class="mt-1 text-xs leading-5 text-warning" data-testid="selfcheck-warnings">
                          {m.settings_data_selfcheck_warnings({ count: result.warning_count })}
                        </p>
                        <ul class="mt-1 space-y-1 text-xs leading-5">
                          {#each result.warning_sample as warning (`${warning.transaction_id}:${warning.reason}`)}
                            <li>
                              <a class="text-accent underline" href={`/app/ledger/transactions?transaction_id=${warning.transaction_id}`}>
                                {m.investments_attribution_payment_link({ id: warning.transaction_id })}
                              </a>
                              {attributionReason(warning.reason)}
                            </li>
                          {/each}
                        </ul>
                      {/if}
                      {#if result.unavailable_count > 0}
                        <span class="mt-1 block text-xs leading-5 text-warning" data-testid="selfcheck-unavailable">
                          {m.settings_data_selfcheck_unavailable({ count: result.unavailable_count })}
                        </span>
                        {#if result.check_id === 'investment_attribution'}
                          <ul class="mt-1 space-y-1 text-xs leading-5">
                            {#each result.unavailable_sample as transactionID (transactionID)}
                              <li><a class="text-accent underline" href={`/app/ledger/transactions?transaction_id=${transactionID}`}>
                                {m.investments_attribution_payment_link({ id: transactionID })}
                              </a></li>
                            {/each}
                          </ul>
                        {/if}
                      {/if}
                      {#if result.status === 'failed'}
                        <span class="mt-1 block text-xs leading-5 text-danger">{result.next_step}</span>
                      {:else if result.status === 'incomplete' || result.status === 'warning'}
                        <span class="mt-1 block text-xs leading-5 text-muted">{result.check_id === 'investment_attribution' ? m.settings_data_selfcheck_attribution_next() : result.next_step}</span>
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
        {/if}
      </div>
    </div>
  </Panel>
</div>
