<script lang="ts">
  import { tick } from 'svelte';
  import { createQuery } from '@tanstack/svelte-query';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import {
    archivePayee, payeeDirectoryQueryOptions, restorePayee, updatePayee, type PayeeDirectoryRow, type PayeeDirectoryStatus
  } from '#lib/api/payees.ts';
  import { getAPIClientErrorMessage } from '#lib/api-error-messages.ts';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import Panel from '#lib/components/panel.svelte';
  import StatePanel from '#lib/components/state-panel.svelte';
  import StatusBadge from '#lib/components/status-badge.svelte';
  import { formatCalendarDate } from '#lib/dates/calendar-date.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { payeeTransactionsHref } from '#lib/transactions/transaction-url-filters.ts';

  type Action = 'rename' | 'archive' | 'restore';

  const inputClass = 'min-h-11 w-full rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground focus:border-accent';
  const buttonClass = 'inline-flex min-h-11 items-center justify-center rounded-(--radius-control) border border-border bg-control px-3 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-50';
  const primaryClass = 'min-h-11 rounded-(--radius-control) bg-accent px-4 text-sm font-semibold text-accent-foreground disabled:opacity-50';

  let search = $state('');
  let query = $state('');
  let status = $state<PayeeDirectoryStatus>('active');
  let cursors = $state<(string | undefined)[]>([undefined]);
  let searchInput: HTMLInputElement;
  const cursor = $derived(cursors.at(-1));
  const directory = createQuery(() => payeeDirectoryQueryOptions(status, query, cursor));
  const session = createQuery(() => authSessionQueryOptions());
  const formatter = $derived(new Intl.NumberFormat(getLocale()));

  let editing = $state<{ action: Action; row: PayeeDirectoryRow }>();
  let name = $state('');
  let pending = $state(false);
  let error = $state<unknown>();
  let notice = $state('');

  $effect(() => {
    const term = search.trim();
    if (term === query) return;
    const timer = setTimeout(() => {
      query = term;
      cursors = [undefined];
    }, 250);
    return () => clearTimeout(timer);
  });

  function selectStatus(next: PayeeDirectoryStatus) {
    status = next;
    cursors = [undefined];
  }

  function startAction(row: PayeeDirectoryRow, action: Action) {
    editing = { action, row };
    name = row.name;
    error = undefined;
    notice = '';
  }

  function actionLabel(action: Action) {
    switch (action) {
      case 'rename': return m.payees_rename();
      case 'archive': return m.payees_archive();
      case 'restore': return m.payees_restore();
    }
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    const token = session.data?.csrf_token;
    if (!editing || !token || pending) return;
    const { action, row } = editing;
    const trimmed = name.trim();
    if (action === 'rename' && (!trimmed || trimmed === row.name)) return;
    pending = true;
    error = undefined;
    try {
      switch (action) {
        case 'rename':
          await updatePayee(row.id, { name: trimmed }, token);
          notice = m.payees_renamed({ name: trimmed });
          break;
        case 'archive':
          await archivePayee(row.id, token);
          notice = m.payees_archived({ name: row.name });
          break;
        case 'restore':
          await restorePayee(row.id, token);
          notice = m.payees_restored({ name: row.name });
          break;
      }
      editing = undefined;
      await tick();
      if (status !== 'all' && action !== 'rename') searchInput.focus();
    } catch (cause) {
      error = cause;
    } finally {
      pending = false;
    }
  }
</script>

<div class="space-y-4">
  <Panel variant="toolbar" padding="sm">
    <div class="flex flex-wrap items-end gap-3">
      <label class="min-w-0 flex-1 basis-48 space-y-1">
        <span class="text-sm font-medium text-foreground">{m.payees_search()}</span>
        <input bind:this={searchInput} bind:value={search} type="search" class={inputClass} />
      </label>
      <label class="min-w-32 space-y-1">
        <span class="text-sm font-medium text-foreground">{m.payees_status()}</span>
        <select value={status} onchange={(event) => selectStatus(event.currentTarget.value as PayeeDirectoryStatus)} class={inputClass}>
          <option value="active">{m.payees_active()}</option>
          <option value="archived">{m.payees_archived_option()}</option>
          <option value="all">{m.search_all()}</option>
        </select>
      </label>
    </div>
  </Panel>

  <div role="status" class={notice ? 'text-sm text-foreground' : 'sr-only'}>{notice}</div>
  {#if directory.isPending}
    <div role="status"><Panel padding="sm"><p class="text-sm text-muted">{m.payees_loading()}</p></Panel></div>
  {:else if directory.isError}
    <StatePanel title={m.payees_load_error()} copy={getAPIClientErrorMessage(directory.error)} alert>
      <button type="button" class={buttonClass} onclick={() => void directory.refetch()}>{m.search_retry()}</button>
    </StatePanel>
  {:else if directory.data.payees.length === 0}
    <Panel padding="sm"><p class="text-sm text-muted">{m.payees_empty()}</p></Panel>
  {:else}
    <Panel padding="none">
      <ul class="divide-y divide-border">
        {#each directory.data.payees as row (row.id)}
          {@const lifecycle = row.status === 'archived' ? 'restore' : 'archive'}
          <li class="grid min-w-0 items-center gap-x-5 gap-y-2 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_minmax(10rem,auto)_auto]">
            <div class="min-w-0">
              <a class="inline-flex min-h-11 items-center font-semibold text-accent underline underline-offset-2 wrap-anywhere"
                href={payeeTransactionsHref(row.id)}>{row.name}</a>
              {#if row.status === 'archived'}
                <StatusBadge>{m.payees_archived_option()}</StatusBadge>
              {/if}
            </div>
            <dl class="flex flex-wrap gap-x-5 gap-y-2 text-sm">
              <div>
                <dt class="text-xs text-muted">{m.transactions_page_title()}</dt>
                <dd class="tabular-nums">{formatter.format(row.transaction_count)}</dd>
              </div>
              <div>
                <dt class="text-xs text-muted">{m.payees_last_used()}</dt>
                <dd class="tabular-nums">
                  {row.last_used_on ? formatCalendarDate(row.last_used_on, getLocale()) : m.payees_unused()}
                </dd>
              </div>
            </dl>
            <div class="flex flex-wrap gap-2">
              <button type="button" class={buttonClass} disabled={!session.data?.csrf_token}
                aria-label={m.payees_action_for({ action: m.payees_rename(), name: row.name })}
                onclick={() => startAction(row, 'rename')}>{m.payees_rename()}</button>
              <button type="button" class={buttonClass} disabled={!session.data?.csrf_token}
                aria-label={m.payees_action_for({ action: actionLabel(lifecycle), name: row.name })}
                onclick={() => startAction(row, lifecycle)}>{actionLabel(lifecycle)}</button>
            </div>
          </li>
        {/each}
      </ul>
    </Panel>
  {/if}

  {#if cursors.length > 1 || directory.data?.next_cursor}
    <div class="flex justify-between gap-2">
      <button type="button" class={buttonClass} disabled={cursors.length === 1}
        onclick={() => { cursors = cursors.slice(0, -1); }}>{m.payees_previous()}</button>
      <button type="button" class={buttonClass} disabled={!directory.data?.next_cursor}
        onclick={() => {
          const next = directory.data?.next_cursor;
          if (next) cursors = [...cursors, next];
        }}>{m.payees_next()}</button>
    </div>
  {/if}
</div>

{#if editing}
  <ModalDialog labelledby="payee-action-heading" {pending} onCancel={() => { editing = undefined; }}
    fallbackFocus={() => searchInput} class="max-w-md p-5">
    <form class="space-y-4" aria-busy={pending} onsubmit={submit}>
      <h2 id="payee-action-heading" class="text-lg font-semibold wrap-anywhere">
        {m.payees_action_for({ action: actionLabel(editing.action), name: editing.row.name })}
      </h2>
      <APIFormError {error} />
      {#if editing.action === 'rename'}
        <label class="block space-y-1">
          <span class="text-sm font-medium">{m.payees_name()}</span>
          <input bind:value={name} required maxlength="200" data-initial-focus class={inputClass} disabled={pending} />
        </label>
      {:else}
        <p class="text-sm text-muted">{editing.action === 'archive' ? m.payees_archive_copy() : m.payees_restore_copy()}</p>
      {/if}
      <div class="flex flex-wrap justify-end gap-2">
        <button type="button" class={buttonClass} disabled={pending} onclick={() => { editing = undefined; }}>
          {m.transactions_form_cancel()}
        </button>
        <button type="submit" class={primaryClass}
          disabled={pending || !session.data?.csrf_token || (editing.action === 'rename' && (!name.trim() || name.trim() === editing.row.name))}>
          {pending ? m.transactions_form_save_pending() : editing.action === 'rename' ? m.payees_save() : actionLabel(editing.action)}
        </button>
      </div>
    </form>
  </ModalDialog>
{/if}
