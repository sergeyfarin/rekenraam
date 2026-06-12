<script lang="ts">
  import { Save, X } from '@lucide/svelte';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import {
    createInstitution,
    updateInstitution,
    type InstitutionRequest,
    type InstitutionResponse
  } from '$lib/api/institutions';
  import { m } from '$lib/paraglide/messages.js';

  type InstitutionEditorMode = 'create' | 'edit';
  type InstitutionKind = InstitutionResponse['kind'];

  let {
    mode,
    institution,
    csrfToken,
    onSaved,
    onCancel
  } = $props<{
    mode: InstitutionEditorMode;
    institution?: InstitutionResponse;
    csrfToken?: string;
    onSaved: (institution: InstitutionResponse) => Promise<void> | void;
    onCancel: () => void;
  }>();

  const inputClass =
    'mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent';
  const textAreaClass =
    'mt-1.5 min-h-20 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent';
  const labelClass = 'block text-xs font-semibold uppercase tracking-[0.12em] text-muted';

  let institutionID = $state<number | undefined>(undefined);
  let name = $state('');
  let kind = $state<InstitutionKind>('bank');
  let countryCode = $state('');
  let website = $state('');
  let effectiveFrom = $state('');
  let changeReason = $state('');
  let commentMarkdown = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);

  const title = $derived(
    mode === 'edit' ? m.institutions_form_edit_title() : m.institutions_form_create_title()
  );
  const submitLabel = $derived(
    pending
      ? mode === 'edit'
        ? m.institutions_form_save_pending()
        : m.institutions_form_create_pending()
      : mode === 'edit'
        ? m.institutions_form_save()
        : m.institutions_form_create()
  );

  $effect(() => {
    institutionID = institution?.id;
    name = institution?.name ?? '';
    kind = institution?.kind ?? 'bank';
    countryCode = institution?.country_code ?? '';
    website = institution?.website ?? '';
    effectiveFrom = mode === 'edit' ? '' : (institution?.effective_from ?? '');
    changeReason = '';
    commentMarkdown = institution?.comment_markdown ?? '';
    formError = undefined;
  });

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();

    if (!csrfToken) {
      formError = new Error(m.accounts_form_missing_session());
      return;
    }

    pending = true;
    formError = undefined;

    try {
      const input = requestFromForm();
      const saved =
        mode === 'edit' && institutionID
          ? await updateInstitution(institutionID, input, csrfToken)
          : await createInstitution(input, csrfToken);

      await onSaved(saved);
    } catch (error) {
      formError = error;
    } finally {
      pending = false;
    }
  }

  function requestFromForm(): InstitutionRequest {
    const request: InstitutionRequest = {
      name: name.trim(),
      kind
    };

    assignString(request, 'country_code', countryCode.toUpperCase());
    assignString(request, 'website', website);
    assignString(request, 'effective_from', effectiveFrom);
    assignString(request, 'change_reason', changeReason);
    assignString(request, 'comment_markdown', commentMarkdown);

    return request;
  }

  function assignString<T extends Record<string, unknown>>(target: T, key: keyof T, value: string) {
    const trimmed = value.trim();
    if (trimmed !== '') {
      target[key] = trimmed as T[keyof T];
    }
  }

  function institutionKindLabel(value: InstitutionKind): string {
    switch (value) {
      case 'bank':
        return m.institution_kind_bank();
      case 'credit_union':
        return m.institution_kind_credit_union();
      case 'brokerage':
        return m.institution_kind_brokerage();
      case 'card_issuer':
        return m.institution_kind_card_issuer();
      case 'lender':
        return m.institution_kind_lender();
      case 'insurance':
        return m.institution_kind_insurance();
      case 'employer':
        return m.institution_kind_employer();
      case 'rewards_program':
        return m.institution_kind_rewards_program();
      case 'government':
        return m.institution_kind_government();
      case 'other':
        return m.institution_kind_other();
    }
  }

  const kindOptions: InstitutionKind[] = [
    'bank',
    'credit_union',
    'brokerage',
    'card_issuer',
    'lender',
    'insurance',
    'employer',
    'rewards_program',
    'government',
    'other'
  ];
</script>

<form class="space-y-4" onsubmit={handleSubmit}>
  <div class="flex items-start justify-between gap-3">
    <div class="min-w-0">
      <h2 class="text-base font-semibold text-foreground">{title}</h2>
      {#if mode === 'edit' && institution}
        <p class="mt-1 truncate text-sm text-muted">{institution.name}</p>
      {/if}
    </div>
    <button
      type="button"
      class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover"
      onclick={onCancel}
      aria-label={m.accounts_form_cancel()}
      title={m.accounts_form_cancel()}
    >
      <X size={16} aria-hidden="true" />
    </button>
  </div>

  <APIFormError error={formError} id="institution-form-error" />

  <div class="grid gap-3 sm:grid-cols-2">
    <label>
      <span class={labelClass}>{m.institutions_field_name()}</span>
      <input bind:value={name} class={inputClass} required maxlength="200" />
    </label>

    <label>
      <span class={labelClass}>{m.institutions_field_kind()}</span>
      <select bind:value={kind} class={inputClass}>
        {#each kindOptions as option (option)}
          <option value={option}>{institutionKindLabel(option)}</option>
        {/each}
      </select>
    </label>

    <label>
      <span class={labelClass}>{m.accounts_field_country()}</span>
      <input bind:value={countryCode} class={inputClass} maxlength="2" autocapitalize="characters" />
    </label>

    <label>
      <span class={labelClass}>{m.institutions_field_website()}</span>
      <input bind:value={website} class={inputClass} type="url" maxlength="500" />
    </label>

    <label>
      <span class={labelClass}>{m.accounts_field_effective_from()}</span>
      <input bind:value={effectiveFrom} class={inputClass} type="date" />
    </label>

    <label>
      <span class={labelClass}>{m.accounts_field_reason()}</span>
      <input bind:value={changeReason} class={inputClass} maxlength="500" />
    </label>
  </div>

  <label>
    <span class={labelClass}>{m.accounts_field_comment()}</span>
    <textarea bind:value={commentMarkdown} class={textAreaClass} maxlength="5000"></textarea>
  </label>

  <div class="flex flex-wrap items-center justify-end gap-2">
    <button
      type="button"
      class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      onclick={onCancel}
    >
      <X size={16} aria-hidden="true" />
      {m.accounts_form_cancel()}
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
