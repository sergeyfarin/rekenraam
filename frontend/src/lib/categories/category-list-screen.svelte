<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import Archive from '@lucide/svelte/icons/archive';
  import Edit3 from '@lucide/svelte/icons/edit-3';
  import Plus from '@lucide/svelte/icons/plus';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import Tags from '@lucide/svelte/icons/tags';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import TreeNameCell from '$lib/components/tree-name-cell.svelte';
  import { authSessionQueryOptions } from '$lib/api/auth';
  import {
    categoriesQueryOptions,
    completeCategoriesSetup,
    deleteCategory,
    disableCategory,
    restoreCategory,
    type CategoryResponse
  } from '$lib/api/categories';
  import { setupStatusQueryOptions } from '$lib/api/setup';
  import { m } from '$lib/paraglide/messages.js';
  import CategoryIcon from './category-icon.svelte';
  import CategoryEditor from './category-editor.svelte';
  import { categoryIconName } from './category-icons';
  import {
    categoryDisplayName,
    categoryParentLabel,
    categoryStatusLabel,
    categoryTypeLabel,
    categoryTypeRank,
    type CategoryStatus,
    type CategoryType
  } from './category-labels';

  type TypeFilter = CategoryType | 'all';
  type StatusFilter = CategoryStatus | 'all';
  type EditorState = { type: 'none' } | { type: 'create' } | { type: 'edit'; category: CategoryResponse };

  type CategoryGroup = {
    key: CategoryType;
    label: string;
    rows: CategoryTreeRow[];
  };

  type CategoryTreeRow = {
    category: CategoryResponse;
    depth: number;
  };

  const categoriesQuery = createQuery(() => categoriesQueryOptions({ includeArchived: true }));
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const setupQuery = createQuery(() => setupStatusQueryOptions());
  const collator = new Intl.Collator(undefined, { sensitivity: 'base', numeric: true });

  let query = $state('');
  let typeFilter = $state<TypeFilter>('all');
  let statusFilter = $state<StatusFilter>('active');
  let editor = $state<EditorState>({ type: 'none' });
  let actionError = $state<unknown>(undefined);
  let actionPendingKey = $state('');
  let setupPending = $state(false);
  let setupError = $state<unknown>(undefined);
  let collapsedCategoryIDs = $state<Set<number>>(new Set());

  const allCategories = $derived(categoriesQuery.data?.categories ?? []);
  const activeCategories = $derived(allCategories.filter((category) => category.status === 'active'));
  const categoriesByID = $derived.by(() => new Map<number, CategoryResponse>(allCategories.map((category) => [category.id, category])));
  const activeChildCounts = $derived.by(() => countChildren(allCategories.filter((category) => category.status === 'active')));
  const anyChildCounts = $derived.by(() => countChildren(allCategories));
  const csrfToken = $derived(sessionQuery.data?.csrf_token);
  const shellError = $derived(categoriesQuery.error ?? sessionQuery.error ?? setupQuery.error ?? setupError);
  const categorySetupPending = $derived(
    setupQuery.data?.steps.some((step) => step.key === 'categories' && step.status === 'pending') ?? false
  );

  const matchedCategories = $derived.by(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();

    return allCategories
      .filter((category) => {
        if (typeFilter !== 'all' && category.category_type !== typeFilter) {
          return false;
        }

        if (statusFilter !== 'all' && category.status !== statusFilter) {
          return false;
        }

        if (normalizedQuery === '') {
          return true;
        }

        const haystack = [
          categoryDisplayName(category),
          category.code ?? '',
          category.builtin_key ?? '',
          categoryTypeLabel(category.category_type),
          categoryStatusLabel(category.status),
          categoryParentLabel(category, categoriesByID)
        ]
          .join(' ')
          .toLocaleLowerCase();

        return haystack.includes(normalizedQuery);
      })
      .toSorted(compareCategories);
  });

  const groupedCategories = $derived.by<CategoryGroup[]>(() => {
    const groups = new Map<CategoryType, CategoryGroup>();
    const visibleTreeRows = buildVisibleTreeRows(matchedCategories);

    for (const row of visibleTreeRows) {
      const group = groups.get(row.category.category_type);
      if (group) {
        group.rows.push(row);
      } else {
        groups.set(row.category.category_type, {
          key: row.category.category_type,
          label: categoryTypeLabel(row.category.category_type),
          rows: [row]
        });
      }
    }

    return Array.from(groups.values()).toSorted((left, right) => categoryTypeRank(left.key) - categoryTypeRank(right.key));
  });
  const treeChildCounts = $derived.by(() => countVisibleTreeChildren(buildVisibleCategoryMap(matchedCategories)));

  const activeCount = $derived(allCategories.filter((category) => category.status === 'active').length);
  const archivedCount = $derived(allCategories.filter((category) => category.status === 'archived').length);
  const builtInCount = $derived(allCategories.filter((category) => category.is_builtin).length);
  const starterCount = $derived(allCategories.filter((category) => category.is_starter).length);
  const customCount = $derived(allCategories.filter((category) => !category.is_builtin && !category.is_starter).length);

  const screenState = $derived.by<'loading' | 'error' | 'empty' | 'ready'>(() => {
    if (categoriesQuery.isPending || sessionQuery.isPending || setupQuery.isPending || setupPending) {
      return 'loading';
    }

    if (categoriesQuery.isError || sessionQuery.isError || setupQuery.isError || setupError) {
      return 'error';
    }

    if (allCategories.length === 0) {
      return 'empty';
    }

    return 'ready';
  });

  $effect(() => {
    if (!categorySetupPending || setupPending || setupError || !csrfToken) {
      return;
    }

    void seedStarterCategories();
  });

  function compareCategories(left: CategoryResponse, right: CategoryResponse): number {
    const typeComparison = categoryTypeRank(left.category_type) - categoryTypeRank(right.category_type);
    if (typeComparison !== 0) {
      return typeComparison;
    }

    return collator.compare(categoryDisplayName(left), categoryDisplayName(right)) || left.id - right.id;
  }

  function buildVisibleTreeRows(categories: CategoryResponse[]): CategoryTreeRow[] {
    const visibleByID = buildVisibleCategoryMap(categories);

    const childrenByParentID = new Map<number, CategoryResponse[]>();
    const roots: CategoryResponse[] = [];

    for (const category of visibleByID.values()) {
      const parentID = category.parent_category_id;
      const parent = parentID ? visibleByID.get(parentID) : undefined;

      if (parent && parent.category_type === category.category_type) {
        const siblings = childrenByParentID.get(parent.id) ?? [];
        siblings.push(category);
        childrenByParentID.set(parent.id, siblings);
      } else {
        roots.push(category);
      }
    }

    for (const siblings of childrenByParentID.values()) {
      siblings.sort(compareCategories);
    }
    roots.sort(compareCategories);

    const rows: CategoryTreeRow[] = [];
    for (const root of roots) {
      appendTreeRows(root, 0, childrenByParentID, rows);
    }

    return rows;
  }

  function buildVisibleCategoryMap(categories: CategoryResponse[]): Map<number, CategoryResponse> {
    const visibleByID = new Map<number, CategoryResponse>();
    const normalizedQuery = query.trim();

    for (const category of categories) {
      visibleByID.set(category.id, category);

      if (normalizedQuery !== '') {
        addVisibleAncestors(category, visibleByID);
      }
    }

    return visibleByID;
  }

  function countVisibleTreeChildren(categoriesByID: ReadonlyMap<number, CategoryResponse>): Map<number, number> {
    const counts = new Map<number, number>();

    for (const category of categoriesByID.values()) {
      const parentID = category.parent_category_id;
      const parent = parentID ? categoriesByID.get(parentID) : undefined;

      if (parent && parent.category_type === category.category_type) {
        counts.set(parent.id, (counts.get(parent.id) ?? 0) + 1);
      }
    }

    return counts;
  }

  function addVisibleAncestors(category: CategoryResponse, visibleByID: Map<number, CategoryResponse>) {
    let parentID = category.parent_category_id;
    const seen = new Set<number>([category.id]);

    while (parentID) {
      if (seen.has(parentID)) {
        break;
      }
      seen.add(parentID);

      const parent = categoriesByID.get(parentID);
      if (!parent || parent.category_type !== category.category_type) {
        break;
      }

      if (typeFilter !== 'all' && parent.category_type !== typeFilter) {
        break;
      }
      if (statusFilter !== 'all' && parent.status !== statusFilter) {
        break;
      }

      visibleByID.set(parent.id, parent);
      parentID = parent.parent_category_id;
    }
  }

  function appendTreeRows(
    category: CategoryResponse,
    depth: number,
    childrenByParentID: ReadonlyMap<number, CategoryResponse[]>,
    rows: CategoryTreeRow[]
  ) {
    rows.push({ category, depth });

    if (collapsedCategoryIDs.has(category.id)) {
      return;
    }

    const children = childrenByParentID.get(category.id) ?? [];
    for (const child of children) {
      appendTreeRows(child, depth + 1, childrenByParentID, rows);
    }
  }

  function countChildren(categories: CategoryResponse[]): Map<number, number> {
    const counts = new Map<number, number>();

    for (const category of categories) {
      if (category.parent_category_id) {
        counts.set(category.parent_category_id, (counts.get(category.parent_category_id) ?? 0) + 1);
      }
    }

    return counts;
  }

  function toggleCategory(categoryID: number) {
    const next = new Set(collapsedCategoryIDs);
    if (next.has(categoryID)) {
      next.delete(categoryID);
    } else {
      next.add(categoryID);
    }
    collapsedCategoryIDs = next;
  }

  async function refreshCategories() {
    await Promise.all([categoriesQuery.refetch(), sessionQuery.refetch(), setupQuery.refetch()]);
  }

  async function seedStarterCategories() {
    setupPending = true;
    setupError = undefined;

    try {
      await completeCategoriesSetup(csrfToken ?? '');
      await refreshCategories();
    } catch (error) {
      setupError = error;
    } finally {
      setupPending = false;
    }
  }

  async function getCSRFToken(): Promise<string> {
    if (csrfToken) {
      return csrfToken;
    }

    const refreshedSession = await sessionQuery.refetch();
    const token = refreshedSession.data?.csrf_token;
    if (!token) {
      throw new Error(m.categories_form_missing_session());
    }

    return token;
  }

  async function handleSaved() {
    editor = { type: 'none' };
    await refreshCategories();
  }

  async function handleArchiveCategory(category: CategoryResponse) {
    await runCategoryAction(`archive:${category.id}`, async (token) => {
      await disableCategory(category.id, token);
    });
  }

  async function handleRestoreCategory(category: CategoryResponse) {
    await runCategoryAction(`restore:${category.id}`, async (token) => {
      await restoreCategory(category.id, token);
    });
  }

  async function handleDeleteCategory(category: CategoryResponse) {
    if (!confirm(m.categories_delete_confirm({ name: categoryDisplayName(category) }))) {
      return;
    }

    await runCategoryAction(`delete:${category.id}`, async (token) => {
      await deleteCategory(category.id, token);
    });
  }

  async function runCategoryAction(key: string, action: (csrfToken: string) => Promise<void>) {
    actionPendingKey = key;
    actionError = undefined;

    try {
      const token = await getCSRFToken();
      await action(token);
      await refreshCategories();
    } catch (error) {
      actionError = error;
    } finally {
      actionPendingKey = '';
    }
  }
</script>

<div class="space-y-4">
  <Panel variant="toolbar">
    <div class="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
      <div class="grid min-w-fit grid-cols-2 gap-2 text-sm sm:grid-cols-5">
        <div class="rounded-[var(--radius-control)] border border-border bg-surface px-3 py-2">
          <p class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.categories_stat_active()}</p>
          <p class="mt-1 text-lg font-semibold text-foreground">{activeCount}</p>
        </div>
        <div class="rounded-[var(--radius-control)] border border-border bg-surface px-3 py-2">
          <p class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.categories_stat_archived()}</p>
          <p class="mt-1 text-lg font-semibold text-foreground">{archivedCount}</p>
        </div>
        <div class="rounded-[var(--radius-control)] border border-border bg-surface px-3 py-2">
          <p class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.categories_stat_builtin()}</p>
          <p class="mt-1 text-lg font-semibold text-foreground">{builtInCount}</p>
        </div>
        <div class="rounded-[var(--radius-control)] border border-border bg-surface px-3 py-2">
          <p class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.categories_stat_starter()}</p>
          <p class="mt-1 text-lg font-semibold text-foreground">{starterCount}</p>
        </div>
        <div class="rounded-[var(--radius-control)] border border-border bg-surface px-3 py-2">
          <p class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.categories_stat_custom()}</p>
          <p class="mt-1 text-lg font-semibold text-foreground">{customCount}</p>
        </div>
      </div>

      <div class="grid min-w-0 flex-1 gap-3 md:grid-cols-[minmax(12rem,1fr)_10rem_10rem]">
        <label>
          <span class="block text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.categories_filter_search()}</span>
          <input
            bind:value={query}
            class="mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent"
            placeholder={m.categories_filter_search_placeholder()}
          />
        </label>

        <label>
          <span class="block text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.categories_filter_type()}</span>
          <select bind:value={typeFilter} class="mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground outline-none transition hover:bg-control-hover focus:border-accent">
            <option value="all">{m.categories_filter_type_all()}</option>
            <option value="income">{categoryTypeLabel('income')}</option>
            <option value="expense">{categoryTypeLabel('expense')}</option>
          </select>
        </label>

        <label>
          <span class="block text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.categories_filter_status()}</span>
          <select bind:value={statusFilter} class="mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground outline-none transition hover:bg-control-hover focus:border-accent">
            <option value="active">{categoryStatusLabel('active')}</option>
            <option value="archived">{categoryStatusLabel('archived')}</option>
            <option value="all">{m.categories_filter_status_all()}</option>
          </select>
        </label>
      </div>

      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
        onclick={() => (editor = { type: 'create' })}
      >
        <Plus size={16} aria-hidden="true" />
        {m.categories_add_category()}
      </button>
    </div>
  </Panel>

  <APIFormError error={actionError} id="categories-action-error" />

  {#if editor.type !== 'none'}
    <Panel>
      <CategoryEditor
        mode={editor.type === 'edit' ? 'edit' : 'create'}
        category={editor.type === 'edit' ? editor.category : undefined}
        categories={activeCategories}
        {csrfToken}
        onSaved={handleSaved}
        onCancel={() => (editor = { type: 'none' })}
      />
    </Panel>
  {/if}

  {#if screenState === 'loading'}
    <StatePanel title={m.categories_loading_title()} copy={categorySetupPending || setupPending ? m.categories_setup_loading_copy() : m.categories_loading_copy()} />
  {:else if screenState === 'error'}
    <StatePanel title={m.categories_error_title()} copy={m.categories_error_copy()}>
      <APIFormError error={shellError} id="categories-error" />
      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
        onclick={refreshCategories}
      >
        <RefreshCw size={16} aria-hidden="true" />
        {m.categories_retry()}
      </button>
    </StatePanel>
  {:else if screenState === 'empty'}
    <StatePanel title={m.categories_empty_title()} copy={m.categories_empty_copy()}>
      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
        onclick={() => (editor = { type: 'create' })}
      >
        <Tags size={16} aria-hidden="true" />
        {m.categories_add_category()}
      </button>
    </StatePanel>
  {:else if matchedCategories.length === 0}
    <StatePanel title={m.categories_no_results_title()} copy={m.categories_no_results_copy()} />
  {:else}
    <div class="space-y-4">
      {#each groupedCategories as group (group.key)}
        <section class="overflow-hidden rounded-[var(--radius-panel)] border border-border bg-surface shadow-[var(--shadow-panel)]">
          <div class="flex items-center justify-between gap-4 border-b border-border bg-toolbar px-4 py-3">
            <h2 class="truncate text-sm font-semibold tracking-tight text-foreground">{group.label}</h2>
            <span class="rounded-[var(--radius-control)] bg-surface-strong px-2.5 py-1 text-xs font-semibold text-muted">
              {m.categories_group_count({ count: group.rows.length })}
            </span>
          </div>

          <div class="divide-y divide-border">
            {#each group.rows as row (row.category.id)}
              {@const category = row.category}
              {@const hasTreeChildren = (treeChildCounts.get(category.id) ?? 0) > 0}
              {@const isCollapsed = collapsedCategoryIDs.has(category.id)}
              <article class="grid gap-3 px-4 py-3 transition hover:bg-row-hover lg:grid-cols-[minmax(12rem,1fr)_minmax(10rem,0.75fr)_minmax(7rem,0.45fr)_minmax(9rem,0.55fr)_7.25rem] lg:items-center">
                <TreeNameCell
                  depth={row.depth}
                  expandable={hasTreeChildren}
                  expanded={!isCollapsed}
                  expandLabel={m.categories_tree_expand({ name: categoryDisplayName(category) })}
                  collapseLabel={m.categories_tree_collapse({ name: categoryDisplayName(category) })}
                  onToggle={() => toggleCategory(category.id)}
                >
                  <div class="flex min-w-0 items-center gap-2">
                    <span class="inline-flex size-8 shrink-0 items-center justify-center rounded-[var(--radius-control)] bg-accent-soft text-accent">
                      <CategoryIcon icon={categoryIconName(category)} className="size-4" />
                    </span>
                    <p class="min-w-0 truncate text-sm font-semibold text-foreground">{categoryDisplayName(category)}</p>
                  </div>
                  <p class="mt-1 truncate text-xs text-muted">
                    {#if category.is_builtin}
                      {m.categories_builtin_marker()}
                    {:else if category.is_starter}
                      {m.categories_starter_marker()}
                    {:else}
                      {m.categories_custom_marker()}
                    {/if}
                    {#if category.code}
                      <span aria-hidden="true"> · </span>{category.code}
                    {/if}
                  </p>
                </TreeNameCell>

                <div class="min-w-0 text-sm">
                  <p class="truncate text-foreground">{categoryParentLabel(category, categoriesByID)}</p>
                  <p class="mt-1 truncate text-xs text-muted">{category.allows_postings ? m.categories_allows_postings_yes() : m.categories_allows_postings_no()}</p>
                </div>

                <div class="flex min-w-0 items-center justify-start">
                  <StatusBadge tone={category.status === 'archived' ? 'danger' : 'accent'}>
                    {categoryStatusLabel(category.status)}
                  </StatusBadge>
                </div>

                <div class="min-w-0 text-xs text-muted">
                  {#if category.status === 'archived' && category.closed_on}
                    <p class="truncate">{m.categories_closed_on({ date: category.closed_on })}</p>
                  {:else if (activeChildCounts.get(category.id) ?? 0) > 0}
                    <p class="truncate">{m.categories_child_count({ count: activeChildCounts.get(category.id) ?? 0 })}</p>
                  {:else}
                    <p class="truncate">{m.categories_no_active_children()}</p>
                  {/if}
                </div>

                <div class="flex min-w-0 items-center justify-start gap-1 lg:justify-end">
                  <button
                    type="button"
                    class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-50"
                    onclick={() => (editor = { type: 'edit', category })}
                    disabled={category.is_builtin}
                    aria-label={m.categories_action_edit({ name: categoryDisplayName(category) })}
                    title={m.categories_action_edit({ name: categoryDisplayName(category) })}
                  >
                    <Edit3 size={15} aria-hidden="true" />
                  </button>

                  {#if category.status === 'active'}
                    <button
                      type="button"
                      class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-50"
                      onclick={() => handleArchiveCategory(category)}
                      disabled={category.is_builtin || (activeChildCounts.get(category.id) ?? 0) > 0 || actionPendingKey === `archive:${category.id}`}
                      aria-label={m.categories_action_archive({ name: categoryDisplayName(category) })}
                      title={m.categories_action_archive({ name: categoryDisplayName(category) })}
                    >
                      <Archive size={15} aria-hidden="true" />
                    </button>
                  {:else}
                    <button
                      type="button"
                      class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-50"
                      onclick={() => handleRestoreCategory(category)}
                      disabled={category.is_builtin || actionPendingKey === `restore:${category.id}`}
                      aria-label={m.categories_action_restore({ name: categoryDisplayName(category) })}
                      title={m.categories_action_restore({ name: categoryDisplayName(category) })}
                    >
                      <RotateCcw size={15} aria-hidden="true" />
                    </button>
                  {/if}

                  <button
                    type="button"
                    class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-danger/30 bg-danger-soft text-danger transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
                    onclick={() => handleDeleteCategory(category)}
                    disabled={category.is_builtin || (anyChildCounts.get(category.id) ?? 0) > 0 || actionPendingKey === `delete:${category.id}`}
                    aria-label={m.categories_action_delete({ name: categoryDisplayName(category) })}
                    title={m.categories_action_delete({ name: categoryDisplayName(category) })}
                  >
                    <Trash2 size={15} aria-hidden="true" />
                  </button>
                </div>
              </article>
            {/each}
          </div>
        </section>
      {/each}
    </div>
  {/if}
</div>
