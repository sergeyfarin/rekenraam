<script lang="ts">
  import { tick } from 'svelte';
  import { replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { createInfiniteQuery, createQuery } from '@tanstack/svelte-query';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import {
    archiveTag, renameTag, restoreTag, tagDirectoryQueryOptions, type TagDirectoryRow, type TagDirectoryStatus
  } from '#lib/api/tags.ts';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import Panel from '#lib/components/panel.svelte';
  import StatusBadge from '#lib/components/status-badge.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { tagTransactionsHref } from '#lib/transactions/transaction-url-filters.ts';

  type Action = 'rename' | 'archive' | 'restore';

  const inputClass = 'min-h-11 w-full rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground focus:border-accent';
  const buttonClass = 'inline-flex min-h-11 items-center justify-center rounded-(--radius-control) border border-border bg-control px-3 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-50';
  const primaryClass = 'min-h-11 rounded-(--radius-control) bg-accent px-4 text-sm font-semibold text-accent-foreground disabled:opacity-50';

  const url = $derived(page.shallow?.url ?? page.url);
  const urlStatus = $derived(url.searchParams.get('status'));
  const status = $derived<TagDirectoryStatus>(urlStatus === 'archived' || urlStatus === 'all' ? urlStatus : 'active');
  const q = $derived(url.searchParams.get('q') ?? '');
  let searchDraft = $state<string>();
  let searchInput: HTMLInputElement;
  const directory = createInfiniteQuery(() => tagDirectoryQueryOptions(status, q));
  const tags = $derived(directory.data?.pages.flatMap((result) => result.tags) ?? []);
  const session = createQuery(() => authSessionQueryOptions());
  const formatter = $derived(new Intl.NumberFormat(getLocale()));

  const kindLabel = {
    project: m.tags_kind_project,
    person: m.tags_kind_person,
    flag: m.tags_kind_flag,
    place: m.tags_kind_place,
    custom: m.tags_kind_custom
  };

  let editing = $state<{ action: Action; tag: TagDirectoryRow }>();
  let name = $state('');
  let pending = $state(false);
  let error = $state<unknown>();
  let notice = $state('');

  function setFilters(nextStatus: string, nextSearch: string) {
    const next = new URL(url.href);
    if (nextStatus === 'active') next.searchParams.delete('status');
    else next.searchParams.set('status', nextStatus);
    if (nextSearch) next.searchParams.set('q', nextSearch);
    else next.searchParams.delete('q');
    if (next.href !== url.href) replaceState(next, page.state);
  }

  $effect(() => {
    const value = searchDraft?.trim();
    if (value === undefined || value === q) return;
    const timer = setTimeout(() => setFilters(status, value), 250);
    return () => clearTimeout(timer);
  });

  function startAction(tag: TagDirectoryRow, action: Action) {
    editing = { action, tag };
    name = tag.name;
    error = undefined;
    notice = '';
  }

  function actionLabel(action: Action) {
    switch (action) {
      case 'rename': return m.tags_rename();
      case 'archive': return m.tags_archive();
      case 'restore': return m.tags_restore();
    }
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    const token = session.data?.csrf_token;
    if (!editing || !token || pending) return;
    const { action, tag } = editing;
    const trimmed = name.trim();
    if (action === 'rename' && (!trimmed || trimmed === tag.name)) return;
    pending = true;
    error = undefined;
    try {
      switch (action) {
        case 'rename':
          await renameTag(tag.id, trimmed, token);
          notice = m.tags_renamed({ name: trimmed });
          break;
        case 'archive':
          await archiveTag(tag.id, token);
          notice = m.tags_archived({ name: tag.name });
          break;
        case 'restore':
          await restoreTag(tag.id, token);
          notice = m.tags_restored({ name: tag.name });
          break;
      }
      editing = undefined;
      await tick();
      searchInput?.focus();
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
        <span class="text-sm font-medium text-foreground">{m.tags_search()}</span>
        <input bind:this={searchInput} value={searchDraft ?? q} type="search" class={inputClass}
          oninput={(event) => { searchDraft = event.currentTarget.value; }} />
      </label>
      <label class="min-w-32 space-y-1">
        <span class="text-sm font-medium text-foreground">{m.transactions_filter_status()}</span>
        <select value={status} class={inputClass}
          onchange={(event) => setFilters(event.currentTarget.value, (searchDraft ?? q).trim())}>
          <option value="active">{m.accounts_filter_status_active()}</option>
          <option value="archived">{m.account_status_archived()}</option>
          <option value="all">{m.transactions_filter_status_all()}</option>
        </select>
      </label>
    </div>
  </Panel>

  <p role="status" class={notice ? 'text-sm text-foreground wrap-anywhere' : 'sr-only'}>{notice}</p>

  {#if directory.isError}
    <Panel padding="sm">
      <APIFormError error={directory.error} />
      <button type="button" class={buttonClass}
        onclick={() => directory.isFetchNextPageError ? directory.fetchNextPage() : directory.refetch()}>
        {m.search_retry()}
      </button>
    </Panel>
  {/if}
  {#if directory.isPending}
    <Panel padding="sm"><p role="status" class="text-sm text-muted">{m.tags_loading()}</p></Panel>
  {:else if tags.length === 0 && directory.isSuccess}
    <Panel padding="sm"><p class="text-sm text-muted">{m.tags_empty()}</p></Panel>
  {:else if tags.length > 0}
    <Panel padding="none">
      <ul class="divide-y divide-border">
        {#each tags as tag (tag.id)}
          {@const lifecycle = tag.status === 'archived' ? 'restore' : 'archive'}
          <li class="grid min-w-0 items-center gap-x-5 gap-y-2 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_auto_auto]">
            <div class="min-w-0">
              <a class="inline-flex max-w-full min-h-11 items-center font-semibold text-accent underline underline-offset-2 wrap-anywhere"
                href={tagTransactionsHref(tag.id)}>{tag.name}</a>
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-sm text-muted">{kindLabel[tag.kind]()}</span>
                {#if tag.status === 'archived'}
                  <StatusBadge>{m.account_status_archived()}</StatusBadge>
                {/if}
              </div>
            </div>
            <dl class="text-sm">
              <div>
                <dt class="text-xs text-muted">{m.transactions_page_title()}</dt>
                <dd class="tabular-nums">{formatter.format(tag.transaction_count)}</dd>
              </div>
            </dl>
            <div class="flex flex-wrap gap-2">
              {#if tag.status === 'active'}
                <button type="button" class={buttonClass} disabled={!session.data?.csrf_token}
                  aria-label={m.tags_action_for({ action: m.tags_rename(), name: tag.name })}
                  onclick={() => startAction(tag, 'rename')}>{m.tags_rename()}</button>
              {/if}
              <button type="button" class={buttonClass} disabled={!session.data?.csrf_token}
                aria-label={m.tags_action_for({ action: actionLabel(lifecycle), name: tag.name })}
                onclick={() => startAction(tag, lifecycle)}>{actionLabel(lifecycle)}</button>
            </div>
          </li>
        {/each}
      </ul>
    </Panel>
  {/if}

  {#if directory.hasNextPage && !directory.isError}
    <button type="button" class={buttonClass} disabled={directory.isFetching}
      onclick={() => void directory.fetchNextPage()}>
      {directory.isFetchingNextPage ? m.tags_loading() : m.transactions_load_more()}
    </button>
  {/if}
</div>

{#if editing}
  <ModalDialog labelledby="tag-action-heading" {pending} onCancel={() => { editing = undefined; }}
    fallbackFocus={() => searchInput} class="max-w-md p-5">
    <form class="space-y-4" aria-busy={pending} onsubmit={submit}>
      <h2 id="tag-action-heading" class="text-lg font-semibold wrap-anywhere">
        {m.tags_action_for({ action: actionLabel(editing.action), name: editing.tag.name })}
      </h2>
      <APIFormError {error} />
      {#if editing.action === 'rename'}
        <label class="block space-y-1">
          <span class="text-sm font-medium">{m.tags_name()}</span>
          <input bind:value={name} required maxlength="120" data-initial-focus class={inputClass} disabled={pending} />
        </label>
      {:else if editing.action === 'archive'}
        <p class="text-sm text-muted">{m.tags_archive_copy()}</p>
      {/if}
      <div class="flex flex-wrap justify-end gap-2">
        <button type="button" class={buttonClass} disabled={pending} onclick={() => { editing = undefined; }}>
          {m.transactions_form_cancel()}
        </button>
        <button type="submit" class={primaryClass}
          disabled={pending || !session.data?.csrf_token || (editing.action === 'rename' && (!name.trim() || name.trim() === editing.tag.name))}>
          {pending ? m.transactions_form_save_pending() : editing.action === 'rename' ? m.tags_save() : actionLabel(editing.action)}
        </button>
      </div>
    </form>
  </ModalDialog>
{/if}
