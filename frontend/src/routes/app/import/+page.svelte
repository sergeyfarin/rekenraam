<script lang="ts">
  import { onDestroy } from 'svelte';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import Upload from '@lucide/svelte/icons/upload';
  import CheckCircle from '@lucide/svelte/icons/circle-check';
  import AlertCircle from '@lucide/svelte/icons/circle-alert';
  import Info from '@lucide/svelte/icons/info';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Plus from '@lucide/svelte/icons/plus';
  import Loader from '@lucide/svelte/icons/loader-circle';
  import Panel from '$lib/components/panel.svelte';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { authSessionQueryOptions } from '$lib/api/auth';
  import { accountsQueryOptions } from '$lib/api/accounts';
  import { currenciesQueryOptions } from '$lib/api/currencies';
  import { categoriesQueryOptions } from '$lib/api/categories';
  import {
    startImport,
    startOnlineImport,
    getImportBatch,
    getFullImportBatch,
    patchImportBatch,
    commitImportBatch,
    discardImportBatch,
    parseNormalized,
    parseResolution,
    parseBatchSourceMeta,
    type StartImportResponse,
    type ImportStagedRow,
    type CommitImportBatchResponse,
    type ImportResolution
  } from '$lib/api/imports';
  import {
    listImportConnections,
    createImportConnection,
    updateImportConnection,
    deleteImportConnection,
    refreshImportConnection,
    importConnectionsQueryKey,
    type ImportConnection
  } from '$lib/api/connections';
  import { m } from '$lib/paraglide/messages.js';

  // ── Page state ─────────────────────────────────────────────────────
  type Step = 'upload' | 'fetching' | 'preview' | 'result';

  let step = $state<Step>('upload');

  // Upload step
  let selectedFile = $state<File | null>(null);
  let uploading = $state(false);
  let uploadError = $state<unknown>(undefined);

  // Online import (fetching) step
  let startingOnlineConnectionId = $state<number | null>(null);
  let onlineImportError = $state<unknown>(undefined);
  let fetchFailed = $state(false);
  let pollTimer: ReturnType<typeof setTimeout> | null = null;

  // Preview step
  let previewData = $state<StartImportResponse | null>(null);
  let batchId = $state<number | null>(null);
  // Per-row resolution: accountId, commodityId, categoryId / transferAccountId
  let rowResolutions = $state<Map<number, ImportResolution>>(new Map());
  let globalAccountId = $state<number | undefined>(undefined);
  let globalCommodityId = $state<number | undefined>(undefined);
  let globalCategoryId = $state<number | undefined>(undefined);
  let globalTransferAccountId = $state<number | undefined>(undefined);

  // Commit step
  let committing = $state(false);
  let commitError = $state<unknown>(undefined);
  let commitResult = $state<CommitImportBatchResponse | null>(null);
  let reconciliationOverride = $state(false);

  // Discard
  let discarding = $state(false);
  let discardError = $state<unknown>(undefined);
  let showDiscardConfirm = $state(false);

  // ── Queries ────────────────────────────────────────────────────────
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const csrfToken = $derived(sessionQuery.data?.csrf_token ?? '');

  const accountsQuery = createQuery(() => accountsQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  const categoriesQuery = createQuery(() => categoriesQueryOptions());

  const accounts = $derived(accountsQuery.data?.accounts ?? []);
  const currencies = $derived(currenciesQuery.data?.currencies ?? []);
  const categories = $derived(categoriesQuery.data?.categories ?? []);

  // ── Upload ─────────────────────────────────────────────────────────
  function handleFileChange(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    selectedFile = input.files?.[0] ?? null;
  }

  async function handleUpload() {
    if (!selectedFile) return;
    uploading = true;
    uploadError = undefined;

    try {
      const result = await startImport(selectedFile, csrfToken);
      previewData = result;
      batchId = result.batch.id;
      // Initialize resolutions from existing data
      rowResolutions = new Map();
      step = 'preview';
    } catch (err) {
      uploadError = err;
    } finally {
      uploading = false;
    }
  }

  // ── Online import (Trading 212) ───────────────────────────────────────
  async function handleStartOnlineImport(connectionId: number) {
    startingOnlineConnectionId = connectionId;
    onlineImportError = undefined;
    fetchFailed = false;
    try {
      const result = await startOnlineImport(connectionId, csrfToken);
      batchId = result.batch.id;
      step = 'fetching';
      pollFetchStatus();
    } catch (err) {
      onlineImportError = err;
    } finally {
      startingOnlineConnectionId = null;
    }
  }

  async function handleRefreshConnection(connectionId: number) {
    startingOnlineConnectionId = connectionId;
    onlineImportError = undefined;
    fetchFailed = false;
    try {
      const result = await refreshImportConnection(connectionId, csrfToken);
      batchId = result.batch.id;
      step = 'fetching';
      pollFetchStatus();
    } catch (err) {
      onlineImportError = err;
    } finally {
      startingOnlineConnectionId = null;
    }
  }

  async function pollFetchStatus() {
    if (!batchId) return;
    try {
      const firstPage = await getImportBatch(batchId);
      const meta = parseBatchSourceMeta(firstPage.batch);
      if (meta.fetch_status === 'ready') {
        const result = firstPage.next_cursor ? await getFullImportBatch(batchId, firstPage) : firstPage;
        previewData = {
          batch: result.batch,
          rows: result.rows,
          warnings: meta.warnings ?? [],
          meta: {
            account_hints: meta.account_hints ?? [],
            currency_hints: meta.currency_hints ?? [],
            date_from: meta.date_from,
            date_to: meta.date_to
          }
        };
        rowResolutions = new Map();
        step = 'preview';
        return;
      }
      if (meta.fetch_status === 'failed') {
        fetchFailed = true;
        return;
      }
      pollTimer = setTimeout(pollFetchStatus, 2000);
    } catch (err) {
      onlineImportError = err;
      fetchFailed = true;
    }
  }

  async function handleCancelFetch() {
    if (pollTimer) {
      clearTimeout(pollTimer);
      pollTimer = null;
    }
    const idToDiscard = batchId;
    step = 'upload';
    batchId = null;
    fetchFailed = false;
    onlineImportError = undefined;
    if (idToDiscard) {
      // Best-effort: the batch may already be "failed" (discard would 409),
      // and there is no UI consequence either way once we've left the step.
      try {
        await discardImportBatch(idToDiscard, csrfToken);
      } catch {
        // ignore
      }
    }
  }

  onDestroy(() => {
    if (pollTimer) clearTimeout(pollTimer);
  });

  // ── Preview helpers ────────────────────────────────────────────────
  function getResolution(rowId: number): ImportResolution {
    return rowResolutions.get(rowId) ?? {};
  }

  function updateResolution(rowId: number, patch: Partial<ImportResolution>) {
    const existing = rowResolutions.get(rowId) ?? {};
    rowResolutions.set(rowId, { ...existing, ...patch });
    rowResolutions = new Map(rowResolutions); // trigger reactivity
  }

  function isTransferRow(row: ImportStagedRow): boolean {
    return !!parseNormalized(row).transfer_hint;
  }

  function applyGlobalAccount() {
    if (!previewData) return;
    for (const row of previewData.rows) {
      if (row.dedupe_status === 'excluded') continue;
      if (isTransferRow(row)) {
        updateResolution(row.id, {
          account_id: globalAccountId,
          commodity_id: globalCommodityId,
          transfer_account_id: globalTransferAccountId
        });
      } else {
        updateResolution(row.id, {
          account_id: globalAccountId,
          commodity_id: globalCommodityId,
          category_id: globalCategoryId
        });
      }
    }
  }

  function toggleExclude(row: ImportStagedRow) {
    const res = getResolution(row.id);
    updateResolution(row.id, { exclude: !res.exclude });
  }

  function dedupeStatusLabel(status: ImportStagedRow['dedupe_status']): string {
    switch (status) {
      case 'duplicate': return m.import_preview_dedupe_duplicate();
      case 'needs_attention': return m.import_preview_dedupe_needs_attention();
      case 'excluded': return m.import_preview_dedupe_excluded();
      default: return m.import_preview_dedupe_new();
    }
  }

  // ── Commit ─────────────────────────────────────────────────────────
  async function handleCommit() {
    if (!batchId) return;
    committing = true;
    commitError = undefined;

    try {
      // First patch resolutions to the server.
      const patches = (previewData?.rows ?? []).map((row) => ({
        row_id: row.id,
        dedupe_status: getResolution(row.id).exclude ? 'excluded' : row.dedupe_status,
        resolution: getResolution(row.id)
      }));

      await patchImportBatch(batchId, patches, csrfToken);

      const result = await commitImportBatch(
        batchId,
        { reconciliation_override: reconciliationOverride },
        csrfToken
      );
      commitResult = result;
      step = 'result';
    } catch (err) {
      commitError = err;
    } finally {
      committing = false;
    }
  }

  // ── Discard ────────────────────────────────────────────────────────
  async function handleDiscard() {
    if (!batchId) return;
    discarding = true;
    discardError = undefined;

    try {
      await discardImportBatch(batchId, csrfToken);
      // Reset to upload step.
      step = 'upload';
      previewData = null;
      batchId = null;
      selectedFile = null;
      showDiscardConfirm = false;
    } catch (err) {
      discardError = err;
    } finally {
      discarding = false;
    }
  }

  function handleImportAnother() {
    step = 'upload';
    previewData = null;
    batchId = null;
    selectedFile = null;
    commitResult = null;
  }

  // ── Connections ────────────────────────────────────────────────────
  const queryClient = useQueryClient();

  const connectionsQuery = createQuery(() => ({
    queryKey: importConnectionsQueryKey,
    queryFn: () => listImportConnections(),
    retry: false
  }));

  const connections = $derived(connectionsQuery.data?.connections ?? []);
  const connectionsConfigError = $derived(
    connectionsQuery.isError && (connectionsQuery.error as { code?: string })?.code === 'CONFIG_REQUIRED'
  );

  // Add-connection form
  let showAddConnection = $state(false);
  let newConnName = $state('');
  let newConnKey = $state('');
  let newConnCashAccountId = $state('');
  let addingConnection = $state(false);
  let addConnectionError = $state<unknown>(undefined);

  // Delete connection
  let deletingConnectionId = $state<number | null>(null);
  let confirmDeleteConnectionId = $state<number | null>(null);
  let deleteConnectionError = $state<unknown>(undefined);

  // Auto-refresh toggle
  let togglingAutoRefreshId = $state<number | null>(null);
  let autoRefreshError = $state<unknown>(undefined);

  // Cash account picker
  let updatingCashAccountId = $state<number | null>(null);
  let cashAccountError = $state<unknown>(undefined);

  const postableAccounts = $derived(
    accounts.filter((a) => a.allows_postings && a.status !== 'archived')
  );

  async function handleAddConnection() {
    if (!newConnName.trim() || !newConnKey.trim()) return;
    addingConnection = true;
    addConnectionError = undefined;
    try {
      await createImportConnection(
        {
          source: 'trading212',
          display_name: newConnName.trim(),
          api_key: newConnKey.trim(),
          ...(newConnCashAccountId ? { cash_account_id: Number(newConnCashAccountId) } : {})
        },
        csrfToken
      );
      await queryClient.invalidateQueries({ queryKey: importConnectionsQueryKey });
      showAddConnection = false;
      newConnName = '';
      newConnKey = '';
      newConnCashAccountId = '';
    } catch (err) {
      addConnectionError = err;
    } finally {
      addingConnection = false;
    }
  }

  async function handleSetCashAccount(conn: ImportConnection, accountId: string) {
    if (!accountId) return;
    updatingCashAccountId = conn.id;
    cashAccountError = undefined;
    try {
      await updateImportConnection(
        conn.id,
        {
          display_name: conn.display_name,
          config: conn.config,
          cash_account_id: Number(accountId)
        },
        csrfToken
      );
      await queryClient.invalidateQueries({ queryKey: importConnectionsQueryKey });
    } catch (err) {
      cashAccountError = err;
    } finally {
      updatingCashAccountId = null;
    }
  }

  async function handleDeleteConnection(id: number) {
    deletingConnectionId = id;
    deleteConnectionError = undefined;
    try {
      await deleteImportConnection(id, csrfToken);
      await queryClient.invalidateQueries({ queryKey: importConnectionsQueryKey });
      confirmDeleteConnectionId = null;
    } catch (err) {
      deleteConnectionError = err;
    } finally {
      deletingConnectionId = null;
    }
  }

  async function handleToggleAutoRefresh(conn: ImportConnection) {
    togglingAutoRefreshId = conn.id;
    autoRefreshError = undefined;
    try {
      await updateImportConnection(
        conn.id,
        {
          display_name: conn.display_name,
          config: conn.config,
          auto_refresh_enabled: !conn.auto_refresh_enabled
        },
        csrfToken
      );
      await queryClient.invalidateQueries({ queryKey: importConnectionsQueryKey });
    } catch (err) {
      autoRefreshError = err;
    } finally {
      togglingAutoRefreshId = null;
    }
  }

  function fetchStatusLabel(conn: ImportConnection): string {
    if (!conn.last_fetch_status) return m.import_connections_status_never();
    if (conn.last_fetch_status === 'fetching') return m.import_connections_status_fetching();
    if (conn.last_fetch_status === 'ready') return m.import_connections_status_ready();
    if (conn.last_fetch_status === 'failed') return m.import_connections_status_failed();
    return m.import_connections_status_never();
  }
</script>

<!-- Upload step -->
{#if step === 'upload'}
  <div class="max-w-2xl space-y-6">
    <Panel>
      <p class="text-sm font-semibold text-foreground">{m.import_upload_title()}</p>
      <p class="mt-2 text-sm leading-6 text-muted">{m.import_upload_copy()}</p>

      <div class="mt-5 space-y-4">
        <div>
          <label
            class="inline-flex cursor-pointer items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
          >
            <Upload size={16} aria-hidden="true" />
            {m.import_upload_choose_file()}
            <input type="file" accept=".qif" class="sr-only" onchange={handleFileChange} />
          </label>
          <span class="ml-3 text-sm text-muted">
            {selectedFile ? selectedFile.name : m.import_upload_no_file()}
          </span>
        </div>

        <APIFormError error={uploadError} id="upload-error" />

        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          onclick={handleUpload}
          disabled={!selectedFile || uploading}
        >
          {uploading ? m.import_upload_submitting() : m.import_upload_submit()}
        </button>
      </div>
    </Panel>

    <!-- MS Money help panel -->
    <Panel variant="subtle">
      <p class="text-sm font-semibold text-foreground">{m.import_upload_ms_money_help()}</p>
      <p class="mt-2 text-sm leading-6 text-muted">{m.import_upload_ms_money_steps()}</p>
    </Panel>

    <!-- Online connections panel -->
    <Panel>
      <div class="flex items-center justify-between gap-4">
        <div>
          <p class="text-sm font-semibold text-foreground">{m.import_connections_title()}</p>
          <p class="mt-1 text-sm text-muted">{m.import_connections_copy()}</p>
        </div>
        <button
          type="button"
          class="inline-flex shrink-0 items-center gap-1.5 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-medium text-foreground transition hover:bg-control-hover"
          onclick={() => { showAddConnection = !showAddConnection; addConnectionError = undefined; }}
        >
          <Plus size={14} aria-hidden="true" />
          {m.import_connections_add()}
        </button>
      </div>

      {#if connectionsConfigError}
        <p class="mt-4 text-sm text-warning">{m.import_connections_error_config()}</p>
      {:else if connectionsQuery.isError}
        <p class="mt-4 text-sm text-warning">{m.import_connections_error_generic()}</p>
      {:else if connections.length === 0 && !showAddConnection}
        <p class="mt-4 text-sm text-muted">{m.import_connections_empty()}</p>
      {:else if connections.length > 0}
        <div class="mt-4 overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-border text-left">
                <th class="pb-2 pr-4 font-semibold text-muted">{m.import_connections_col_name()}</th>
                <th class="pb-2 pr-4 font-semibold text-muted">{m.import_connections_col_source()}</th>
                <th class="pb-2 pr-4 font-semibold text-muted">{m.import_connections_col_key()}</th>
                <th class="pb-2 pr-4 font-semibold text-muted">{m.import_connections_col_status()}</th>
                <th class="pb-2 pr-4 font-semibold text-muted">{m.import_connections_col_auto_refresh()}</th>
                <th class="pb-2 pr-4 font-semibold text-muted">{m.import_connections_col_cash_account()}</th>
                <th class="pb-2 font-semibold text-muted">{m.import_connections_col_actions()}</th>
              </tr>
            </thead>
            <tbody>
              {#each connections as conn (conn.id)}
                <tr class="border-b border-border last:border-b-0">
                  <td class="py-2.5 pr-4 font-medium text-foreground">{conn.display_name}</td>
                  <td class="py-2.5 pr-4 text-muted">{conn.source}</td>
                  <td class="py-2.5 pr-4 font-mono text-xs text-muted">{conn.key_hint}</td>
                  <td class="py-2.5 pr-4 text-muted">{fetchStatusLabel(conn)}</td>
                  <td class="py-2.5 pr-4">
                    <button
                      type="button"
                      role="switch"
                      aria-checked={conn.auto_refresh_enabled}
                      title={conn.auto_refresh_enabled
                        ? m.import_connections_auto_refresh_on()
                        : m.import_connections_auto_refresh_off()}
                      class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition disabled:cursor-not-allowed disabled:opacity-60 {conn.auto_refresh_enabled
                        ? 'bg-foreground'
                        : 'bg-control'} border border-border"
                      onclick={() => handleToggleAutoRefresh(conn)}
                      disabled={togglingAutoRefreshId === conn.id}
                    >
                      <span
                        class="inline-block h-3.5 w-3.5 transform rounded-full bg-background transition {conn.auto_refresh_enabled
                          ? 'translate-x-4'
                          : 'translate-x-1'}"
                      ></span>
                    </button>
                  </td>
                  <td class="py-2.5 pr-4">
                    <select
                      class="rounded-(--radius-control) border border-border bg-control px-2 py-1.5 text-xs focus:outline-none focus:ring-2 focus:ring-foreground disabled:cursor-not-allowed disabled:opacity-60"
                      value={conn.cash_account_id ?? ''}
                      disabled={updatingCashAccountId === conn.id}
                      onchange={(e) => handleSetCashAccount(conn, e.currentTarget.value)}
                    >
                      <option value="" disabled>{m.import_connections_cash_account_placeholder()}</option>
                      {#each postableAccounts as account (account.id)}
                        <option value={account.id}>{account.name}</option>
                      {/each}
                    </select>
                  </td>
                  <td class="py-2.5">
                    <div class="flex items-center gap-2">
                    {#if confirmDeleteConnectionId !== conn.id}
                      <button
                        type="button"
                        class="inline-flex items-center gap-1 rounded-(--radius-control) border border-border px-2.5 py-1 text-xs font-medium text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60"
                        onclick={() =>
                          conn.last_fetch_status
                            ? handleRefreshConnection(conn.id)
                            : handleStartOnlineImport(conn.id)}
                        disabled={startingOnlineConnectionId === conn.id}
                      >
                        {#if startingOnlineConnectionId === conn.id}
                          {m.import_connections_starting()}
                        {:else if conn.last_fetch_status}
                          {m.import_connections_refresh()}
                        {:else}
                          {m.import_connections_import()}
                        {/if}
                      </button>
                    {/if}
                    {#if confirmDeleteConnectionId === conn.id}
                      <div class="flex items-center gap-2">
                        <span class="text-xs text-muted">{m.import_connections_delete_confirm()}</span>
                        <button
                          type="button"
                          class="text-xs text-muted hover:text-foreground"
                          onclick={() => { confirmDeleteConnectionId = null; deleteConnectionError = undefined; }}
                        >
                          {m.import_connections_delete_cancel()}
                        </button>
                        <button
                          type="button"
                          class="inline-flex items-center gap-1 rounded-(--radius-control) bg-foreground px-2.5 py-1 text-xs font-semibold text-background transition hover:opacity-90 disabled:opacity-60"
                          onclick={() => handleDeleteConnection(conn.id)}
                          disabled={deletingConnectionId === conn.id}
                        >
                          <Trash2 size={12} aria-hidden="true" />
                          {m.import_connections_delete_confirm_button()}
                        </button>
                      </div>
                    {:else}
                      <button
                        type="button"
                        class="inline-flex items-center gap-1 rounded-(--radius-control) border border-border px-2.5 py-1 text-xs font-medium text-muted transition hover:bg-control-hover hover:text-foreground"
                        onclick={() => { confirmDeleteConnectionId = conn.id; deleteConnectionError = undefined; }}
                      >
                        <Trash2 size={12} aria-hidden="true" />
                        {m.import_connections_delete()}
                      </button>
                    {/if}
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}

      {#if onlineImportError}
        <p class="mt-3 text-sm text-warning">{m.import_connections_fetch_error()}</p>
      {/if}

      {#if deleteConnectionError}
        <p class="mt-3 text-sm text-warning">{m.import_connections_delete_error()}</p>
      {/if}

      {#if autoRefreshError}
        <p class="mt-3 text-sm text-warning">{m.import_connections_auto_refresh_error()}</p>
      {/if}

      {#if cashAccountError}
        <p class="mt-3 text-sm text-warning">{m.import_connections_cash_account_error()}</p>
      {/if}

      <!-- Add connection form -->
      {#if showAddConnection}
        <div class="mt-5 border-t border-border pt-5">
          <p class="text-sm font-semibold text-foreground">{m.import_connections_add_title()}</p>
          <div class="mt-4 space-y-4">
            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-medium text-muted" for="conn-name">
                {m.import_connections_add_name_label()}
              </label>
              <input
                id="conn-name"
                type="text"
                class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
                placeholder={m.import_connections_add_name_placeholder()}
                bind:value={newConnName}
              />
            </div>
            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-medium text-muted" for="conn-key">
                {m.import_connections_add_key_label()}
              </label>
              <input
                id="conn-key"
                type="password"
                autocomplete="new-password"
                class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
                placeholder={m.import_connections_add_key_placeholder()}
                bind:value={newConnKey}
              />
              <p class="text-xs text-muted">{m.import_connections_add_key_help()}</p>
            </div>
            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-medium text-muted" for="conn-cash-account">
                {m.import_connections_add_cash_account_label()}
              </label>
              <select
                id="conn-cash-account"
                class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
                bind:value={newConnCashAccountId}
              >
                <option value="">{m.import_connections_add_cash_account_none()}</option>
                {#each postableAccounts as account (account.id)}
                  <option value={account.id}>{account.name}</option>
                {/each}
              </select>
              <p class="text-xs text-muted">{m.import_connections_add_cash_account_help()}</p>
            </div>

            {#if addConnectionError}
              {@const errCode = (addConnectionError as { code?: string })?.code}
              <p class="text-sm text-warning">
                {#if errCode === 'PROVIDER_ERROR'}
                  {m.import_connections_error_provider()}
                {:else if errCode === 'CONFIG_REQUIRED'}
                  {m.import_connections_error_config()}
                {:else if errCode === 'CONFLICT'}
                  {m.import_connections_error_duplicate()}
                {:else}
                  {m.import_connections_error_generic()}
                {/if}
              </p>
            {/if}

            <div class="flex items-center gap-3">
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
                onclick={handleAddConnection}
                disabled={addingConnection || !newConnName.trim() || !newConnKey.trim()}
              >
                {addingConnection ? m.import_connections_add_submitting() : m.import_connections_add_submit()}
              </button>
              <button
                type="button"
                class="text-sm text-muted hover:text-foreground"
                onclick={() => { showAddConnection = false; addConnectionError = undefined; newConnName = ''; newConnKey = ''; newConnCashAccountId = ''; }}
              >
                {m.import_discard_cancel()}
              </button>
            </div>
          </div>
        </div>
      {/if}
    </Panel>
  </div>

<!-- Fetching step (online import in progress) -->
{:else if step === 'fetching'}
  <div class="max-w-2xl space-y-6">
    <Panel>
      {#if fetchFailed}
        <div class="flex items-center gap-2">
          <AlertCircle size={20} class="text-warning shrink-0" aria-hidden="true" />
          <p class="text-sm font-semibold text-foreground">{m.import_fetching_failed_title()}</p>
        </div>
        <p class="mt-2 text-sm text-muted">{m.import_fetching_failed_copy()}</p>
      {:else}
        <div class="flex items-center gap-2">
          <Loader size={20} class="shrink-0 animate-spin text-muted" aria-hidden="true" />
          <p class="text-sm font-semibold text-foreground">{m.import_fetching_title()}</p>
        </div>
        <p class="mt-2 text-sm leading-6 text-muted">{m.import_fetching_copy()}</p>
      {/if}

      <div class="mt-5">
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
          onclick={handleCancelFetch}
        >
          {m.import_fetching_cancel()}
        </button>
      </div>
    </Panel>
  </div>

<!-- Preview step -->
{:else if step === 'preview' && previewData}
  <div class="space-y-6">
    <!-- Warnings -->
    {#if previewData.warnings.length > 0}
      <Panel>
        <div class="flex items-center gap-2">
          <AlertCircle size={16} class="text-warning shrink-0" aria-hidden="true" />
          <p class="text-sm font-semibold text-foreground">{m.import_preview_warnings_title()}</p>
        </div>
        <ul class="mt-3 space-y-1">
          {#each previewData.warnings as w}
            <li class="text-sm text-muted">Row {w.row_index + 1}: {w.message}</li>
          {/each}
        </ul>
      </Panel>
    {/if}

    <!-- Date range summary -->
    {#if previewData.meta.date_from || previewData.meta.date_to}
      <p class="text-sm text-muted">
        {m.import_preview_date_range({ from: previewData.meta.date_from ?? '?', to: previewData.meta.date_to ?? '?' })}
      </p>
    {/if}

    <!-- Global account/currency/category assignment -->
    <Panel>
      <p class="text-sm font-semibold text-foreground">{m.import_preview_apply_all()}</p>
      <div class="mt-3 flex flex-wrap gap-3">
        <div class="flex flex-col gap-1">
          <label class="text-xs font-medium text-muted" for="global-account">
            {m.import_preview_account_label()}
          </label>
          <select
            id="global-account"
            class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
            bind:value={globalAccountId}
          >
            <option value={undefined}>{m.import_preview_account_placeholder()}</option>
            {#each accounts as account}
              <option value={account.id}>{account.name ?? account.code}</option>
            {/each}
          </select>
        </div>

        <div class="flex flex-col gap-1">
          <label class="text-xs font-medium text-muted" for="global-currency">
            {m.import_preview_currency_label()}
          </label>
          <select
            id="global-currency"
            class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
            bind:value={globalCommodityId}
          >
            <option value={undefined}>{m.import_preview_currency_placeholder()}</option>
            {#each currencies as currency}
              <option value={currency.id}>{currency.code}</option>
            {/each}
          </select>
        </div>

        <div class="flex flex-col gap-1">
          <label class="text-xs font-medium text-muted" for="global-category">
            {m.import_preview_category_label()}
          </label>
          <select
            id="global-category"
            class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
            bind:value={globalCategoryId}
          >
            <option value={undefined}>{m.import_preview_category_placeholder()}</option>
            {#each categories as category}
              <option value={category.id}>{category.name}</option>
            {/each}
          </select>
        </div>

        <div class="flex flex-col gap-1">
          <label class="text-xs font-medium text-muted" for="global-transfer-account">
            {m.import_preview_transfer_account_label()}
          </label>
          <select
            id="global-transfer-account"
            class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
            bind:value={globalTransferAccountId}
          >
            <option value={undefined}>{m.import_preview_transfer_account_placeholder()}</option>
            {#each accounts as account}
              <option value={account.id}>{account.name ?? account.code}</option>
            {/each}
          </select>
        </div>

        <div class="flex items-end">
          <button
            type="button"
            class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-medium text-foreground transition hover:bg-control-hover"
            onclick={applyGlobalAccount}
          >
            {m.import_preview_apply_all()}
          </button>
        </div>
      </div>
    </Panel>

    <!-- Rows table -->
    <Panel padding="none">
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b border-border text-left">
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_col_date()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_col_payee()}</th>
              <th class="px-4 py-3 font-semibold text-muted text-right">{m.import_preview_col_amount()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_col_memo()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_col_dedupe()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_account_label()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_currency_label()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_col_category()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_col_exclude()}</th>
            </tr>
          </thead>
          <tbody>
            {#each previewData.rows as row (row.id)}
              {@const norm = parseNormalized(row)}
              {@const res = getResolution(row.id)}
              {@const isDuplicate = row.dedupe_status === 'duplicate'}
              {@const isExcluded = res.exclude || row.dedupe_status === 'excluded'}
              {@const isTransfer = !!norm.transfer_hint}
              <tr
                class:opacity-40={isDuplicate || isExcluded}
                class="border-b border-border last:border-b-0"
              >
                <td class="px-4 py-2.5 tabular-nums text-muted">{norm.date}</td>
                <td class="px-4 py-2.5 font-medium text-foreground">{norm.payee_hint || '—'}</td>
                <td class="px-4 py-2.5 tabular-nums text-right text-foreground">{norm.amount}</td>
                <td class="max-w-xs truncate px-4 py-2.5 text-muted">{norm.memo || '—'}</td>
                <td class="px-4 py-2.5">
                  <span
                    class:text-warning={row.dedupe_status === 'needs_attention'}
                    class:text-muted={row.dedupe_status === 'duplicate' || row.dedupe_status === 'excluded'}
                    class="text-xs font-medium"
                  >
                    {dedupeStatusLabel(row.dedupe_status)}
                  </span>
                </td>
                <td class="px-4 py-2.5">
                  {#if !isDuplicate}
                    <select
                      class="rounded-(--radius-control) border border-border bg-control px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-foreground"
                      value={res.account_id}
                      onchange={(e) =>
                        updateResolution(row.id, {
                          account_id: Number((e.currentTarget as HTMLSelectElement).value) || undefined
                        })}
                    >
                      <option value="">—</option>
                      {#each accounts as account}
                        <option value={account.id}>{account.name ?? account.code}</option>
                      {/each}
                    </select>
                  {:else}
                    <span class="text-xs text-muted">—</span>
                  {/if}
                </td>
                <td class="px-4 py-2.5">
                  {#if !isDuplicate}
                    <select
                      class="rounded-(--radius-control) border border-border bg-control px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-foreground"
                      value={res.commodity_id}
                      onchange={(e) =>
                        updateResolution(row.id, {
                          commodity_id: Number((e.currentTarget as HTMLSelectElement).value) || undefined
                        })}
                    >
                      <option value="">—</option>
                      {#each currencies as currency}
                        <option value={currency.id}>{currency.code}</option>
                      {/each}
                    </select>
                  {:else}
                    <span class="text-xs text-muted">—</span>
                  {/if}
                </td>
                <td class="px-4 py-2.5">
                  {#if !isDuplicate}
                    {#if isTransfer}
                      <select
                        class="rounded-(--radius-control) border border-border bg-control px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-foreground"
                        value={res.transfer_account_id}
                        onchange={(e) =>
                          updateResolution(row.id, {
                            transfer_account_id: Number((e.currentTarget as HTMLSelectElement).value) || undefined
                          })}
                      >
                        <option value="">{norm.transfer_hint} →?</option>
                        {#each accounts as account}
                          <option value={account.id}>{account.name ?? account.code}</option>
                        {/each}
                      </select>
                    {:else}
                      <select
                        class="rounded-(--radius-control) border border-border bg-control px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-foreground"
                        value={res.category_id}
                        onchange={(e) =>
                          updateResolution(row.id, {
                            category_id: Number((e.currentTarget as HTMLSelectElement).value) || undefined
                          })}
                      >
                        <option value="">—</option>
                        {#each categories as category}
                          <option value={category.id}>{category.name}</option>
                        {/each}
                      </select>
                    {/if}
                  {:else}
                    <span class="text-xs text-muted">—</span>
                  {/if}
                </td>
                <td class="px-4 py-2.5">
                  {#if !isDuplicate}
                    <input
                      type="checkbox"
                      checked={!!res.exclude}
                      onchange={() => toggleExclude(row)}
                      class="h-4 w-4 rounded border-border text-foreground focus:ring-foreground"
                    />
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>

        {#if previewData.rows.length === 0}
          <p class="px-4 py-8 text-center text-sm text-muted">{m.import_preview_no_rows()}</p>
        {/if}
      </div>
    </Panel>

    <!-- Commit controls -->
    <Panel>
      <div class="space-y-4">
        <label class="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            bind:checked={reconciliationOverride}
            class="h-4 w-4 rounded border-border focus:ring-foreground"
          />
          <span class="font-medium text-foreground">{m.import_commit_reconciliation_override()}</span>
        </label>
        <p class="text-sm text-muted">{m.import_commit_reconciliation_override_hint()}</p>

        <APIFormError error={commitError} id="commit-error" />

        <div class="flex items-center gap-3">
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            onclick={handleCommit}
            disabled={committing}
          >
            {committing ? m.import_commit_pending() : m.import_commit_button()}
          </button>

          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
            onclick={() => { showDiscardConfirm = true; }}
          >
            {m.import_discard_button()}
          </button>
        </div>
      </div>
    </Panel>

    <!-- Discard confirm -->
    {#if showDiscardConfirm}
      <Panel>
        <p class="text-sm font-semibold text-foreground">{m.import_discard_confirm()}</p>
        <APIFormError error={discardError} id="discard-error" />
        <div class="mt-4 flex gap-3">
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
            onclick={() => { showDiscardConfirm = false; discardError = undefined; }}
          >
            {m.import_discard_cancel()}
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            onclick={handleDiscard}
            disabled={discarding}
          >
            {m.import_discard_confirm_button()}
          </button>
        </div>
      </Panel>
    {/if}
  </div>

<!-- Result step -->
{:else if step === 'result' && commitResult}
  <div class="max-w-2xl space-y-6">
    <Panel>
      <div class="flex items-center gap-2">
        <CheckCircle size={20} class="text-positive shrink-0" aria-hidden="true" />
        <p class="text-sm font-semibold text-foreground">{m.import_result_title()}</p>
      </div>

      <div class="mt-4 space-y-2 text-sm">
        <p class="text-foreground">{m.import_result_committed({ count: commitResult.committed_count })}</p>
        {#if commitResult.skipped_count > 0}
          <p class="text-muted">{m.import_result_skipped({ count: commitResult.skipped_count })}</p>
        {/if}
        {#if commitResult.failed_count > 0}
          <p class="text-warning">{m.import_result_failed({ count: commitResult.failed_count })}</p>
        {/if}
      </div>

      <div class="mt-6 flex gap-3">
        <a
          href="/app/transactions"
          class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
        >
          {m.import_result_view_transactions()}
        </a>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
          onclick={handleImportAnother}
        >
          {m.import_result_import_another()}
        </button>
      </div>
    </Panel>
  </div>
{/if}
