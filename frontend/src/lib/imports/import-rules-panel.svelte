<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import Plus from '@lucide/svelte/icons/plus';
  import Panel from '$lib/components/panel.svelte';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { categoriesQueryOptions } from '$lib/api/categories';
  import { payeesQueryOptions } from '$lib/api/payees';
  import { tagsQueryOptions } from '$lib/api/tags';
  import {
    createImportRule,
    deleteImportRule,
    importRulesQueryKey,
    importRulesQueryOptions,
    updateImportRule,
    type ImportRule
  } from '$lib/api/import-rules';
  import { m } from '$lib/paraglide/messages.js';

  let { csrfToken }: { csrfToken: string } = $props();
  const queryClient = useQueryClient();
  const rulesQuery = createQuery(() => importRulesQueryOptions());
  const categoriesQuery = createQuery(() => categoriesQueryOptions());
  const payeesQuery = createQuery(() => payeesQueryOptions());
  const tagsQuery = createQuery(() => tagsQueryOptions());
  const categories = $derived(categoriesQuery.data?.categories ?? []);
  const payees = $derived(payeesQuery.data?.payees ?? []);
  const tags = $derived(tagsQuery.data?.tags ?? []);

  let showForm = $state(false);
  let editingID = $state<number | null>(null);
  let name = $state('');
  let priority = $state(100);
  let enabled = $state(true);
  let matchField = $state<'payee' | 'description'>('payee');
  let containsText = $state('');
  let categoryID = $state('');
  let payeeID = $state('');
  let tagIDs = $state<number[]>([]);
  let saving = $state(false);
  let deletingID = $state<number | null>(null);
  let confirmDeleteID = $state<number | null>(null);
  let mutationError = $state<unknown>(undefined);
  let savedMessage = $state('');

  const targetsReady = $derived(!categoriesQuery.isLoading && !payeesQuery.isLoading && !tagsQuery.isLoading);
  const valid = $derived(!!name.trim() && !!containsText.trim() && (!!categoryID || !!payeeID || tagIDs.length > 0));

  function resetForm() {
    showForm = false;
    editingID = null;
    name = '';
    priority = 100;
    enabled = true;
    matchField = 'payee';
    containsText = '';
    categoryID = '';
    payeeID = '';
    tagIDs = [];
    mutationError = undefined;
  }

  function beginCreate() {
    resetForm();
    showForm = true;
    savedMessage = '';
  }

  function beginEdit(rule: ImportRule) {
    showForm = true;
    editingID = rule.id;
    name = rule.name;
    priority = rule.priority;
    enabled = rule.enabled;
    matchField = rule.match_field;
    containsText = rule.contains_text;
    categoryID = rule.category_id ? String(rule.category_id) : '';
    payeeID = rule.payee_id ? String(rule.payee_id) : '';
    tagIDs = [...rule.tag_ids];
    mutationError = undefined;
    savedMessage = '';
  }

  function toggleTag(tagID: number) {
    tagIDs = tagIDs.includes(tagID) ? tagIDs.filter((id) => id !== tagID) : [...tagIDs, tagID];
  }

  async function saveRule() {
    if (!valid) return;
    saving = true;
    mutationError = undefined;
    try {
      const common = {
        name: name.trim(), priority, enabled, match_field: matchField, contains_text: containsText.trim(),
        tag_ids: tagIDs, ...(categoryID ? { category_id: Number(categoryID) } : {}),
        ...(payeeID ? { payee_id: Number(payeeID) } : {})
      };
      if (editingID) {
        await updateImportRule(editingID, {
          ...common, clear_category: !categoryID, clear_payee: !payeeID
        }, csrfToken);
      } else {
        await createImportRule(common, csrfToken);
      }
      await queryClient.invalidateQueries({ queryKey: importRulesQueryKey });
      resetForm();
      savedMessage = m.import_rules_saved();
    } catch (error) {
      mutationError = error;
    } finally {
      saving = false;
    }
  }

  async function removeRule(ruleID: number) {
    deletingID = ruleID;
    mutationError = undefined;
    try {
      await deleteImportRule(ruleID, csrfToken);
      await queryClient.invalidateQueries({ queryKey: importRulesQueryKey });
      confirmDeleteID = null;
      savedMessage = m.import_rules_deleted();
    } catch (error) {
      mutationError = error;
    } finally {
      deletingID = null;
    }
  }

  function actionSummary(rule: ImportRule): string {
    const actions: string[] = [];
    const category = categories.find((item) => item.id === rule.category_id);
    const payee = payees.find((item) => item.id === rule.payee_id);
    if (category) actions.push(m.import_rules_action_category({ name: category.name ?? '' }));
    if (payee) actions.push(m.import_rules_action_payee({ name: payee.name }));
    if (rule.tag_ids.length) actions.push(m.import_rules_action_tags({ names: rule.tag_ids.map((id) => tags.find((tag) => tag.id === id)?.name ?? String(id)).join(', ') }));
    return actions.join(' · ');
  }
</script>

<Panel>
  <div class="flex flex-wrap items-start justify-between gap-4">
    <div>
      <p class="text-sm font-semibold text-foreground">{m.import_rules_title()}</p>
      <p class="mt-1 text-sm text-muted">{m.import_rules_copy()}</p>
    </div>
    <button type="button" class="inline-flex items-center gap-1.5 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-medium text-foreground hover:bg-control-hover" onclick={beginCreate}>
      <Plus size={14} aria-hidden="true" /> {m.import_rules_add()}
    </button>
  </div>

  {#if rulesQuery.isLoading}
    <p class="mt-4 text-sm text-muted">{m.import_rules_loading()}</p>
  {:else if rulesQuery.isError}
    <p class="mt-4 text-sm text-warning">{m.import_rules_error()}</p>
  {:else if rulesQuery.data?.rules.length === 0 && !showForm}
    <p class="mt-4 text-sm text-muted">{m.import_rules_empty()}</p>
  {:else}
    <ul class="mt-4 space-y-2">
      {#each rulesQuery.data?.rules ?? [] as rule (rule.id)}
        <li class="rounded-(--radius-control) border border-border p-3" class:opacity-60={!rule.enabled}>
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p class="text-sm font-semibold text-foreground">{rule.name}</p>
              <p class="mt-1 text-xs text-muted">{m.import_rules_match_summary({ priority: rule.priority, field: rule.match_field === 'payee' ? m.import_rules_field_payee() : m.import_rules_field_description(), text: rule.contains_text })}</p>
              <p class="mt-1 text-xs text-muted">{actionSummary(rule)}</p>
            </div>
            <div class="flex items-center gap-2">
              <button type="button" class="text-sm text-muted hover:text-foreground" onclick={() => beginEdit(rule)}>{m.import_rules_edit()}</button>
              {#if confirmDeleteID === rule.id}
                <button type="button" class="text-sm text-muted hover:text-foreground" onclick={() => { confirmDeleteID = null; }}>{m.import_rules_cancel()}</button>
                <button type="button" class="text-sm font-semibold text-warning disabled:opacity-60" disabled={deletingID === rule.id} onclick={() => removeRule(rule.id)}>{m.import_rules_delete_confirm()}</button>
              {:else}
                <button type="button" class="text-sm text-muted hover:text-warning" onclick={() => { confirmDeleteID = rule.id; savedMessage = ''; }}>{m.import_rules_delete()}</button>
              {/if}
            </div>
          </div>
        </li>
      {/each}
    </ul>
  {/if}

  {#if showForm}
    <fieldset class="mt-4 space-y-4 rounded-(--radius-control) border border-border p-4">
      <legend class="px-1 text-sm font-semibold text-foreground">{editingID ? m.import_rules_edit_title() : m.import_rules_add_title()}</legend>
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">{m.import_rules_name()}<input class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={name} /></label>
        <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">{m.import_rules_priority()}<input type="number" min="0" max="1000000" class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={priority} /></label>
        <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">{m.import_rules_field()}<select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={matchField}><option value="payee">{m.import_rules_field_payee()}</option><option value="description">{m.import_rules_field_description()}</option></select></label>
        <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">{m.import_rules_contains()}<input class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={containsText} /></label>
        <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">{m.import_rules_category()}<select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={categoryID}><option value="">{m.import_rules_no_action()}</option>{#each categories as category}<option value={String(category.id)}>{category.name}</option>{/each}</select></label>
        <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">{m.import_rules_payee()}<select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={payeeID}><option value="">{m.import_rules_no_action()}</option>{#each payees as payee}<option value={String(payee.id)}>{payee.name}</option>{/each}</select></label>
      </div>
      <fieldset>
        <legend class="text-xs font-medium text-muted">{m.import_rules_tags()}</legend>
        <div class="mt-2 flex flex-wrap gap-3">{#each tags as tag}<label class="flex items-center gap-1.5 text-sm text-foreground"><input type="checkbox" checked={tagIDs.includes(tag.id)} onchange={() => toggleTag(tag.id)} />{tag.name}</label>{/each}</div>
      </fieldset>
      <label class="flex items-center gap-2 text-sm text-foreground"><input type="checkbox" bind:checked={enabled} />{m.import_rules_enabled()}</label>
      {#if !valid}<p class="text-xs text-muted">{m.import_rules_action_required()}</p>{/if}
      <APIFormError error={mutationError} id="import-rule-error" />
      <div class="flex gap-3">
        <button type="button" class="rounded-(--radius-control) bg-foreground px-3 py-2 text-sm font-semibold text-background disabled:opacity-60" disabled={!valid || saving || !targetsReady} onclick={saveRule}>{saving ? m.import_rules_saving() : m.import_rules_save()}</button>
        <button type="button" class="text-sm text-muted hover:text-foreground" onclick={resetForm}>{m.import_rules_cancel()}</button>
      </div>
    </fieldset>
  {/if}
  {#if savedMessage}<p class="mt-3 text-sm text-muted" role="status">{savedMessage}</p>{/if}
  {#if mutationError && !showForm}<div class="mt-3"><APIFormError error={mutationError} id="import-rule-list-error" /></div>{/if}
</Panel>
