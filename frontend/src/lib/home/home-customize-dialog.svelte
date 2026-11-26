<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import X from '@lucide/svelte/icons/x';
  import { untrack } from 'svelte';
  import { homeLayoutQueryOptions, type HomeLayout, type HomeLayoutState, type HomeWidgetID } from '#lib/api/home.ts';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { widgetCatalog } from './explore.ts';
  import HomeLayoutFeedback from './home-layout-feedback.svelte';
  import HomeLayoutOrder from './home-layout-order.svelte';
  import { layoutAccountLabel, moveHomeItem, wideWidgets } from './layout-model.ts';
  import { useHomePersonalization, type CustomizeTarget } from './personalization.svelte.ts';
  import { homeShortcuts, shortcutCatalog, type ShortcutID } from './shortcuts.ts';

  let { target }: { target: CustomizeTarget } = $props();

  const personalization = useHomePersonalization();
  const query = createQuery(() => homeLayoutQueryOptions);
  const catalog = $derived(widgetCatalog());
  const widget = $derived(catalog.find(item => item.id === target));
  const shortcuts = $derived(shortcutCatalog());
  const shortcutEditor = $derived(target === 'all' || target === 'shortcuts');
  const heading = $derived(widget?.title ?? (target === 'reset' ? m.home_layout_reset() : m.home_customize()));

  let source = $state.raw<HomeLayoutState>();
  let draft = $state<HomeLayout>();
  let groupName = $state('');
  let groupAccounts = $state<number[]>([]);
  let shortcutToAdd = $state<ShortcutID | ''>('');
  let shortcutAccount = $state<number | undefined>();
  let widgetToAdd = $state<HomeWidgetID | ''>('');

  function replaceDraft(state: HomeLayoutState) {
    source = state;
    draft = structuredClone(state.layout);
    groupName = state.layout.account_set?.name ?? '';
    groupAccounts = state.layout.account_set?.account_ids ?? [];
    shortcutToAdd = '';
    shortcutAccount = undefined;
    widgetToAdd = '';
  }

  // Freeze the document and revision together while the owner is editing.
  $effect(() => {
    if (query.data && !query.isFetching && !query.isError && !untrack(() => source)) replaceDraft(query.data);
  });

  const canSave = $derived(draft && (draft.headline !== 'account' || !!draft.account_id) &&
    (draft.headline !== 'account_set' || (!!groupName.trim() && groupAccounts.length > 0 && groupAccounts.length <= 100)) &&
    (draft.widget_options.spending?.account_ids?.length ?? 0) <= 100);

  async function save(reset: boolean) {
    if (!source || !draft || (!reset && !canSave)) return;
    const next = $state.snapshot(draft);
    if (groupName.trim() && groupAccounts.length) next.account_set = { name: groupName.trim(), account_ids: $state.snapshot(groupAccounts) };
    for (const item of next.shortcuts) {
      item.label = item.label?.trim() || undefined;
    }
    const result = await personalization.save(source, reset ? null : next);
    if (result.status === 'saved') personalization.close();
    if (result.status === 'conflict') replaceDraft(result.state);
  }

  function widgetTitle(id: HomeWidgetID): string {
    return catalog.find(item => item.id === id)?.title ?? '';
  }

  function addShortcut() {
    if (!draft || !shortcutToAdd || (shortcutToAdd === 'account' && shortcutAccount === undefined)) return;
    draft.shortcuts.push({ id: shortcutToAdd, account_id: shortcutToAdd === 'account' ? shortcutAccount : undefined });
    shortcutToAdd = '';
    shortcutAccount = undefined;
  }

  function moveShortcut(from: number, to: number) {
    if (!draft || !source) return;
    const label = homeShortcuts([draft.shortcuts[from]], source.accounts)[0]?.label ?? '';
    draft.shortcuts = moveHomeItem(draft.shortcuts, from, to);
    announceMove(label, to, draft.shortcuts.length);
  }

  function moveWidget(from: number, to: number) {
    if (!draft) return;
    const label = widgetTitle(draft.widgets[from]);
    draft.widgets = moveHomeItem(draft.widgets, from, to);
    announceMove(label, to, draft.widgets.length);
  }

  function announceMove(item: string, to: number, length: number) {
    const format = new Intl.NumberFormat(getLocale());
    personalization.announce(m.home_layout_moved({ item, position: format.format(to + 1), total: format.format(length) }));
  }

  const fieldClass = 'block min-h-11 w-full min-w-0 rounded-(--radius-control) border border-border bg-control px-2 text-sm';
  const buttonClass = 'inline-flex min-h-11 items-center rounded-(--radius-control) border border-border bg-control px-3 text-sm font-medium hover:bg-control-hover disabled:opacity-50';
  const actionClass = 'min-h-11 rounded-(--radius-control) px-2 text-sm text-accent hover:bg-control-hover';
</script>

<ModalDialog labelledby="home-customize-title" pending={personalization.pending} onCancel={personalization.close} closeOnBackdrop
  fallbackFocus={() => document.getElementById('main-content')}
  class="max-w-2xl">
  <header class="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-border bg-surface px-4 py-2 sm:px-6">
    <h2 id="home-customize-title" class="text-lg font-semibold">{heading}</h2>
    <button data-initial-focus type="button" aria-label={m.home_layout_close()} onclick={personalization.close} disabled={personalization.pending}
      class="grid size-11 shrink-0 place-items-center rounded-full hover:bg-control-hover"><X size={19} aria-hidden="true" /></button>
  </header>

  <div class="space-y-4 px-4 py-4 sm:px-6">
    <HomeLayoutFeedback allowUndo={false} />
    {#if !source || !draft}
      {#if query.isError}
        <APIFormError error={query.error} />
        <button type="button" class={buttonClass} onclick={() => query.refetch()}>{m.attention_retry()}</button>
      {:else}
        <p role="status" class="text-sm text-muted">{m.home_layout_loading()}</p>
      {/if}
    {:else if target === 'reset'}
      <button type="button" class={buttonClass} disabled={personalization.pending} onclick={() => save(true)}>{m.home_layout_reset()}</button>
    {:else}
      <form onsubmit={(event) => { event.preventDefault(); void save(false); }}>
        <fieldset disabled={personalization.pending} class="space-y-5">
          {#if target === 'all'}
            <section class="space-y-3">
              <label class="block space-y-1">
                <span class="font-semibold">{m.home_layout_headline()}</span>
                <select bind:value={draft.headline} class={fieldClass}>
                  <option value="auto">{m.home_layout_auto()}</option>
                  <option value="net_worth">{m.briefing_headline_net_worth()}</option>
                  <option value="cash">{m.briefing_headline_cash()}</option>
                  <option value="account" disabled={!source.accounts.length}>{m.home_layout_account()}</option>
                  <option value="account_set" disabled={!source.accounts.length}>{m.home_layout_account_set()}</option>
                </select>
              </label>
              {#if draft.headline === 'account'}
                <label class="block space-y-1 text-sm">
                  <span>{m.home_layout_select_account()}</span>
                  <select required bind:value={draft.account_id} class={fieldClass}>
                    <option value={undefined}>{m.home_layout_select_account()}</option>
                    {#each source.accounts as account (account.id)}
                      <option value={account.id}>{layoutAccountLabel(account)}</option>
                    {/each}
                  </select>
                </label>
              {:else if draft.headline === 'account_set'}
                <label class="block space-y-1 text-sm">
                  <span>{m.home_layout_account_name()}</span>
                  <input required maxlength="80" bind:value={groupName} class={fieldClass} />
                </label>
                <fieldset class="space-y-1">
                  <legend class="text-sm">{m.home_layout_select_accounts()}</legend>
                  {#each source.accounts as account (account.id)}
                    <label class="flex min-h-11 items-center gap-2 text-sm">
                      <input type="checkbox" value={account.id} bind:group={groupAccounts} />
                      <span class="wrap-anywhere">{layoutAccountLabel(account)}</span>
                    </label>
                  {/each}
                </fieldset>
              {/if}
            </section>
          {/if}

          {#if target === 'all'}
            <label class="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" bind:checked={draft.show_review} />{m.home_layout_show_review()}</label>
          {/if}

          {#if shortcutEditor}
            <section aria-labelledby="home-customize-shortcuts" class="space-y-2">
              <h3 id="home-customize-shortcuts" class="font-semibold">{m.home_shortcuts_label()}</h3>
              <HomeLayoutOrder id="customize-shortcut" items={draft.shortcuts.map(item => item.id)} onMove={moveShortcut}>
                {#snippet children(id)}
                  {@const item = draft?.shortcuts.find(shortcut => shortcut.id === id)}
                  {#if item}
                    {@const base = homeShortcuts([{ ...item, label: undefined }], source?.accounts)[0]?.label ?? ''}
                    <div class="flex flex-wrap items-center gap-1">
                      <input type="text" bind:value={item.label} maxlength="80" placeholder={base}
                        aria-label={m.home_layout_actions({ item: base }) + ' · ' + m.home_layout_label()} class="{fieldClass} flex-1" />
                      {#if id !== 'add'}
                        <button type="button" class={actionClass} onclick={() => { if (draft) draft.shortcuts = draft.shortcuts.filter(shortcut => shortcut.id !== id); }}>{m.home_layout_remove()}</button>
                      {/if}
                    </div>
                  {/if}
                {/snippet}
              </HomeLayoutOrder>
              <div class="flex flex-wrap items-end gap-2">
                <label class="min-w-40 flex-1 space-y-1 text-sm">
                  <span>{m.home_layout_add_shortcut()}</span>
                  <select bind:value={shortcutToAdd} class={fieldClass}>
                    <option value="">{m.home_layout_add_shortcut()}</option>
                    {#each shortcuts.filter(item => !draft?.shortcuts.some(saved => saved.id === item.id)) as item (item.id)}
                      <option value={item.id} disabled={item.id === 'account' && !source.accounts.length}>{item.label}</option>
                    {/each}
                  </select>
                </label>
                {#if shortcutToAdd === 'account'}
                  <label class="min-w-40 flex-1 space-y-1 text-sm">
                    <span>{m.home_layout_select_account()}</span>
                    <select bind:value={shortcutAccount} class={fieldClass}>
                      <option value={undefined}>{m.home_layout_select_account()}</option>
                      {#each source.accounts as account (account.id)}
                        <option value={account.id}>{layoutAccountLabel(account)}</option>
                      {/each}
                    </select>
                  </label>
                {/if}
                <button type="button" class={buttonClass} onclick={addShortcut} disabled={!shortcutToAdd || (shortcutToAdd === 'account' && shortcutAccount === undefined)}>{m.home_layout_add_shortcut()}</button>
              </div>
            </section>
          {/if}

          {#if target === 'all'}
            <section aria-labelledby="home-customize-widgets" class="space-y-2">
              <h3 id="home-customize-widgets" class="font-semibold">{m.home_widgets_label()}</h3>
              <HomeLayoutOrder id="customize-widget" items={draft.widgets} onMove={moveWidget}>
                {#snippet children(id)}
                  <div class="flex flex-wrap items-center justify-between gap-x-1 rounded-(--radius-control) pl-2 text-sm">
                    <span class="wrap-anywhere">{widgetTitle(id)}</span>
                    <button type="button" class={actionClass} onclick={() => { if (draft) draft.widgets = draft.widgets.filter(item => item !== id); }}>{m.home_layout_remove()}</button>
                  </div>
                {/snippet}
              </HomeLayoutOrder>
              <div class="flex flex-wrap items-end gap-2">
                <label class="min-w-40 flex-1 space-y-1 text-sm">
                  <span>{m.home_layout_add_widget()}</span>
                  <select bind:value={widgetToAdd} class={fieldClass}>
                    <option value="">{m.home_layout_add_widget()}</option>
                    {#each catalog.filter(item => !draft?.widgets.includes(item.id)) as item (item.id)}
                      <option value={item.id}>{item.title}</option>
                    {/each}
                  </select>
                </label>
                <button type="button" class={buttonClass} disabled={!widgetToAdd} onclick={() => {
                  if (draft && widgetToAdd) draft.widgets.push(widgetToAdd);
                  widgetToAdd = '';
                }}>{m.home_layout_add_widget()}</button>
              </div>
            </section>
            <label class="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" bind:checked={draft.explore_open} />{m.home_layout_show_explore()}</label>
          {/if}

          {#if widget}
            {@const id = widget.id}
            {@const options = draft.widget_options[id] ?? {}}
            {#if wideWidgets.includes(id)}
              <label class="block space-y-1 text-sm">
                <span>{m.home_layout_size()}</span>
                <select value={options.size ?? 'standard'} class={fieldClass} onchange={(event) => {
                  if (draft) draft.widget_options[id] = { ...options, size: event.currentTarget.value === 'wide' ? 'wide' : 'standard' };
                }}>
                  <option value="standard">{m.home_layout_standard()}</option>
                  <option value="wide">{m.home_layout_wide()}</option>
                </select>
              </label>
            {/if}
            {#if id === 'cashflow'}
              <label class="block space-y-1 text-sm">
                <span>{m.home_layout_period()}</span>
                <select value={options.period ?? 'this_month'} class={fieldClass} onchange={(event) => {
                  if (draft) draft.widget_options[id] = { ...options, period: event.currentTarget.value === 'last_month' ? 'last_month' : 'this_month' };
                }}>
                  <option value="this_month">{m.home_layout_this_month()}</option>
                  <option value="last_month">{m.home_layout_last_month()}</option>
                </select>
              </label>
            {:else if id === 'spending'}
              <fieldset class="space-y-1">
                <legend class="font-semibold">{m.home_layout_select_accounts()}</legend>
                <p class="text-sm text-muted">{m.home_layout_all_accounts()}</p>
                {#each source.accounts as account (account.id)}
                  <label class="flex min-h-11 items-center gap-2 text-sm">
                    <input type="checkbox" checked={options.account_ids?.includes(account.id) ?? false}
                      onchange={(event) => {
                        if (!draft) return;
                        const ids = options.account_ids ?? [];
                        draft.widget_options[id] = { ...options, account_ids: event.currentTarget.checked ? [...ids, account.id] : ids.filter(item => item !== account.id) };
                      }} />
                    <span class="wrap-anywhere">{layoutAccountLabel(account)}</span>
                  </label>
                {/each}
              </fieldset>
            {/if}
          {/if}

          <div class="flex flex-wrap justify-between gap-2 border-t border-border pt-4">
            <button type="submit" disabled={!canSave} class="min-h-11 rounded-(--radius-control) bg-accent px-4 text-sm font-semibold text-accent-foreground disabled:opacity-50">{m.home_layout_save()}</button>
            {#if target === 'all'}
              <button type="button" class={buttonClass} onclick={() => save(true)}>{m.home_layout_reset()}</button>
            {/if}
          </div>
        </fieldset>
      </form>
    {/if}
  </div>
</ModalDialog>
