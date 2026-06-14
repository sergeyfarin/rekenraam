<script lang="ts">
  import Save from '@lucide/svelte/icons/save';
  import X from '@lucide/svelte/icons/x';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import {
    createCategory,
    updateCategory,
    type CategoryResponse,
    type CreateCategoryRequest,
    type UpdateCategoryRequest
  } from '$lib/api/categories';
  import { m } from '$lib/paraglide/messages.js';
  import { deriveRecordCode } from '$lib/record-code';
  import CategoryIcon from './category-icon.svelte';
  import {
    categoryIconLabel,
    categoryIconName,
    categoryIconOptions,
    type CategoryIconName
  } from './category-icons';
  import {
    categoryDisplayName,
    categoryTypeLabel,
    type CategoryType
  } from './category-labels';

  type CategoryEditorMode = 'create' | 'edit';

  let {
    mode,
    category,
    categories,
    csrfToken,
    onSaved,
    onCancel
  } = $props<{
    mode: CategoryEditorMode;
    category?: CategoryResponse;
    categories: CategoryResponse[];
    csrfToken?: string;
    onSaved: () => Promise<void> | void;
    onCancel: () => void;
  }>();

  const inputClass =
    'mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent disabled:cursor-not-allowed disabled:opacity-70';
  const labelClass = 'block text-xs font-semibold uppercase tracking-[0.12em] text-muted';

  let categoryID = $state<number | undefined>(undefined);
  let name = $state('');
  let code = $state('');
  let categoryType = $state<CategoryType>('expense');
  let parentCategoryID = $state('');
  let allowsPostings = $state(true);
  let icon = $state<CategoryIconName>('tags');
  let openedOn = $state('');
  let effectiveFrom = $state('');
  let changeReason = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let codeEdited = $state(false);

  const parentOptions = $derived.by<CategoryResponse[]>(() => {
    return categories
      .filter(
        (candidate: CategoryResponse) =>
          candidate.status === 'active' &&
          candidate.category_type === categoryType &&
          candidate.id !== categoryID
      )
      .toSorted((left: CategoryResponse, right: CategoryResponse) =>
        categoryDisplayName(left).localeCompare(categoryDisplayName(right), undefined, { sensitivity: 'base', numeric: true })
      );
  });

  const title = $derived(mode === 'edit' ? m.categories_form_edit_title() : m.categories_form_create_title());
  const isAppDefined = $derived(category?.is_builtin || category?.is_starter);
  const nameLabel = $derived(isAppDefined ? m.categories_field_name_override() : m.categories_field_name());
  const namePlaceholder = $derived(isAppDefined && category ? categoryDisplayName({ ...category, name: undefined }) : '');
  const submitLabel = $derived(
    pending
      ? mode === 'edit'
        ? m.categories_form_save_pending()
        : m.categories_form_create_pending()
      : mode === 'edit'
        ? m.categories_form_save()
        : m.categories_form_create()
  );

  $effect(() => {
    categoryID = category?.id;
    name = category?.name ?? '';
    code = category?.code ?? deriveRecordCode(category ? categoryDisplayName(category) : '');
    codeEdited = mode === 'edit' && !!category?.code;
    categoryType = category?.category_type ?? 'expense';
    parentCategoryID = category?.parent_category_id ? String(category.parent_category_id) : '';
    allowsPostings = category?.allows_postings ?? true;
    icon = categoryIconName(category);
    openedOn = category?.opened_on ?? '';
    effectiveFrom = '';
    changeReason = '';
    formError = undefined;
  });

  $effect(() => {
    if (parentCategoryID !== '' && !parentOptions.some((parent: CategoryResponse) => String(parent.id) === parentCategoryID)) {
      parentCategoryID = '';
    }
  });

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();

    if (!csrfToken) {
      formError = new Error(m.categories_form_missing_session());
      return;
    }

    pending = true;
    formError = undefined;

    try {
      if (mode === 'edit' && categoryID) {
        await updateCategory(categoryID, updateRequestFromForm(), csrfToken);
      } else {
        await createCategory(createRequestFromForm(), csrfToken);
      }

      await onSaved();
    } catch (error) {
      formError = error;
    } finally {
      pending = false;
    }
  }

  function createRequestFromForm(): CreateCategoryRequest {
    const request: CreateCategoryRequest = {
      name: name.trim(),
      category_type: categoryType,
      allows_postings: allowsPostings,
      icon
    };

    assignString(request, 'code', code);
    assignNumber(request, 'parent_category_id', parentCategoryID);
    assignString(request, 'opened_on', openedOn);
    assignString(request, 'effective_from', effectiveFrom);
    assignString(request, 'change_reason', changeReason);

    return request;
  }

  function updateRequestFromForm(): UpdateCategoryRequest {
    const request: UpdateCategoryRequest = {
      name: name.trim(),
      category_type: categoryType,
      allows_postings: allowsPostings,
      icon
    };

    if (!category?.is_builtin) {
      request.code = code.trim();
    }

    if (parentCategoryID === '') {
      request.clear_parent = true;
    } else {
      assignNumber(request, 'parent_category_id', parentCategoryID);
    }

    assignString(request, 'opened_on', openedOn);
    assignString(request, 'effective_from', effectiveFrom);
    assignString(request, 'change_reason', changeReason);

    return request;
  }

  function handleNameInput(event: Event) {
    name = event.currentTarget instanceof HTMLInputElement ? event.currentTarget.value : '';
    if (!codeEdited && !category?.is_builtin) {
      code = deriveRecordCode(name || namePlaceholder);
    }
  }

  function handleCodeInput(event: Event) {
    code = event.currentTarget instanceof HTMLInputElement ? event.currentTarget.value : '';
    codeEdited = true;
  }

  function assignString<T extends Record<string, unknown>>(target: T, key: keyof T, value: string) {
    const trimmed = value.trim();
    if (trimmed !== '') {
      target[key] = trimmed as T[keyof T];
    }
  }

  function assignNumber<T extends Record<string, unknown>>(target: T, key: keyof T, value: string) {
    if (value !== '') {
      target[key] = Number(value) as T[keyof T];
    }
  }
</script>

<form class="space-y-4" onsubmit={handleSubmit}>
  <div class="flex items-start justify-between gap-3">
    <div class="min-w-0">
      <h2 class="text-base font-semibold text-foreground">{title}</h2>
      {#if mode === 'edit' && category}
        <p class="mt-1 truncate text-sm text-muted">{categoryDisplayName(category)}</p>
      {/if}
    </div>
    <button
      type="button"
      class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover"
      onclick={onCancel}
      aria-label={m.categories_form_cancel()}
      title={m.categories_form_cancel()}
    >
      <X size={16} aria-hidden="true" />
    </button>
  </div>

  <APIFormError error={formError} id="category-form-error" />

  <div class="grid gap-3 sm:grid-cols-2">
    <label>
      <span class={labelClass}>{nameLabel}</span>
      <input value={name} class={inputClass} required={!isAppDefined} maxlength="200" placeholder={namePlaceholder} oninput={handleNameInput} />
    </label>

    <label>
      <span class={labelClass}>{m.categories_field_type()}</span>
      <select bind:value={categoryType} class={inputClass} disabled={mode === 'edit'}>
        <option value="income">{categoryTypeLabel('income')}</option>
        <option value="expense">{categoryTypeLabel('expense')}</option>
      </select>
    </label>

    <label>
      <span class={labelClass}>{m.categories_field_parent()}</span>
      <select bind:value={parentCategoryID} class={inputClass}>
        <option value="">{m.categories_field_no_parent()}</option>
        {#each parentOptions as parent (parent.id)}
          <option value={String(parent.id)}>{categoryDisplayName(parent)}</option>
        {/each}
      </select>
    </label>

    <fieldset class="sm:col-span-2">
      <legend class={labelClass}>{m.categories_field_icon()}</legend>
      <div class="mt-1.5 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
        {#each categoryIconOptions as option (option)}
          <button
            type="button"
            class={`inline-flex min-h-10 items-center gap-2 rounded-[var(--radius-control)] border px-3 py-2 text-left text-sm transition ${
              icon === option
                ? 'border-accent bg-accent-soft text-foreground shadow-sm'
                : 'border-border bg-control text-muted hover:bg-control-hover hover:text-foreground'
            }`}
            aria-pressed={icon === option}
            onclick={() => (icon = option)}
          >
            <CategoryIcon icon={option} className="size-4 shrink-0" />
            <span class="min-w-0 truncate">{categoryIconLabel(option)}</span>
          </button>
        {/each}
      </div>
    </fieldset>
  </div>

  <label class="flex items-center gap-3 rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm text-foreground">
    <input bind:checked={allowsPostings} type="checkbox" class="h-4 w-4 accent-[var(--color-accent)]" />
    {m.categories_field_allows_postings()}
  </label>

  <details class="rounded-[var(--radius-control)] border border-border bg-surface">
    <summary class="cursor-pointer px-3 py-2 text-sm font-semibold text-foreground">{m.form_advanced()}</summary>
    <div class="grid gap-3 border-t border-border px-3 py-3 sm:grid-cols-2">
      <label>
        <span class={labelClass}>{m.categories_field_code()}</span>
        <input value={code} class={inputClass} maxlength="100" disabled={category?.is_builtin} oninput={handleCodeInput} />
      </label>

      <label>
        <span class={labelClass}>{m.categories_field_opened_on()}</span>
        <input bind:value={openedOn} class={inputClass} type="date" />
      </label>

      <label>
        <span class={labelClass}>{m.categories_field_effective_from()}</span>
        <input bind:value={effectiveFrom} class={inputClass} type="date" />
      </label>

      <label>
        <span class={labelClass}>{m.categories_field_reason()}</span>
        <input bind:value={changeReason} class={inputClass} maxlength="500" />
      </label>
    </div>
  </details>

  <div class="flex flex-wrap items-center justify-end gap-2">
    <button
      type="button"
      class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      onclick={onCancel}
    >
      <X size={16} aria-hidden="true" />
      {m.categories_form_cancel()}
    </button>
    <button
      type="submit"
      class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
      disabled={pending}
    >
      <Save size={16} aria-hidden="true" />
      {submitLabel}
    </button>
  </div>
</form>
