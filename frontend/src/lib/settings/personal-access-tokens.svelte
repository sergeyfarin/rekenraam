<script lang="ts">
  import { createInfiniteQuery, createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { tick } from 'svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import Panel from '#lib/components/panel.svelte';
  import { authSessionQueryOptions, authenticationEventsQueryKey } from '#lib/api/auth.ts';
  import {
    createPersonalAccessToken, personalAccessTokensQueryKey, personalAccessTokensQueryOptions,
    revokePersonalAccessToken, type PersonalAccessToken, type TokenScope
  } from '#lib/api/auth-tokens.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  const queryClient = useQueryClient();
  const session = createQuery(() => authSessionQueryOptions());
  const tokens = createInfiniteQuery(() => personalAccessTokensQueryOptions());
  const credentials = $derived(tokens.data?.pages.flatMap((page) => page.tokens) ?? []);
  let name = $state('');
  let password = $state('');
  let days = $state(30);
  let ledger = $state(true);
  let reports = $state(false);
  let secret = $state('');
  let secretInput: HTMLTextAreaElement | undefined = $state();
  let passwordInput: HTMLInputElement | undefined = $state();
  let pending = $state(false);
  let error = $state<unknown>();
  let revoked = $state(false);
  let revokeTarget = $state<PersonalAccessToken>();
  let disclosureGeneration = 0;

  const inputClass = 'mt-1.5 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground outline-none focus:border-accent disabled:opacity-60';
  const buttonClass = 'rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition-colors hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60';

  function clearSecrets() {
    disclosureGeneration += 1;
    secret = '';
    password = '';
    // Scrub the DOM immediately too, before a document can enter bfcache.
    if (secretInput) secretInput.value = '';
    if (passwordInput) passwordInput.value = '';
  }

  function scopeLabel(scope: TokenScope) {
    return scope === 'ledger:read' ? m.security_tokens_ledger() : m.security_tokens_reports();
  }

  function timestamp(value: string) {
    return new Intl.DateTimeFormat(getLocale(), { dateStyle: 'medium', timeStyle: 'short' }).format(Date.parse(value));
  }

  async function refresh() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: personalAccessTokensQueryKey }),
      queryClient.invalidateQueries({ queryKey: authenticationEventsQueryKey })
    ]);
  }

  async function create(event: SubmitEvent) {
    event.preventDefault();
    pending = true;
    error = undefined;
    revoked = false;
    secret = '';
    const generation = disclosureGeneration;
    const scopes: TokenScope[] = [];
    if (ledger) scopes.push('ledger:read');
    if (reports) scopes.push('reports:read');
    try {
      const result = await createPersonalAccessToken({ name, password, scopes, expires_in_days: days }, session.data?.csrf_token ?? '');
      // A response delayed beyond navigation must not restore a scrubbed secret.
      if (generation === disclosureGeneration) secret = result.token;
      name = '';
      await tick();
      secretInput?.focus();
      await refresh();
    } catch (cause) {
      error = cause;
    } finally {
      password = '';
      pending = false;
    }
  }

  async function revoke() {
    if (!revokeTarget) return;
    pending = true;
    error = undefined;
    try {
      await revokePersonalAccessToken(revokeTarget.id, session.data?.csrf_token ?? '');
      secret = '';
      revokeTarget = undefined;
      revoked = true;
      await refresh();
    } catch (cause) {
      error = cause;
    } finally {
      pending = false;
    }
  }
</script>

<svelte:window onpagehide={clearSecrets} />

<Panel>
  <div class="min-w-0 space-y-5">
    <div>
      <h2 class="text-base font-semibold text-foreground">{m.security_tokens_title()}</h2>
      <p class="mt-2 max-w-prose text-sm text-muted">{m.security_tokens_hint()}</p>
    </div>
    {#if error && !revokeTarget}<APIFormError {error} />{/if}
    {#if revoked}<p role="status" class="text-sm text-foreground">{m.security_tokens_revoked_success()}</p>{/if}

    {#if secret}
      <div class="space-y-3">
        <p role="status" class="text-sm font-semibold text-foreground">{m.security_tokens_once()}</p>
        <label class="block text-sm text-foreground">
          {m.security_tokens_secret()}
          <textarea bind:this={secretInput} class={`${inputClass} font-mono break-all`} value={secret} readonly rows="3" spellcheck="false" autocomplete="off"></textarea>
        </label>
        <button class={buttonClass} type="button" onclick={() => (secret = '')}>{m.security_tokens_dismiss()}</button>
      </div>
    {:else}
      <form class="grid max-w-lg gap-4" onsubmit={create}>
        <label class="text-sm text-foreground">
          {m.security_tokens_name()}
          <input class={inputClass} bind:value={name} required maxlength="80" autocomplete="off" disabled={pending} />
        </label>
        <fieldset class="space-y-2" disabled={pending}>
          <legend class="mb-2 text-sm font-semibold text-foreground">{m.security_tokens_scopes()}</legend>
          <label class="flex items-start gap-2 text-sm text-foreground"><input type="checkbox" bind:checked={ledger} class="mt-1" />{m.security_tokens_ledger()}</label>
          <label class="flex items-start gap-2 text-sm text-foreground"><input type="checkbox" bind:checked={reports} class="mt-1" />{m.security_tokens_reports()}</label>
        </fieldset>
        <label class="text-sm text-foreground">
          {m.security_tokens_days()}
          <input class={inputClass} type="number" min="1" max="365" step="1" required bind:value={days} disabled={pending} />
        </label>
        <label class="text-sm text-foreground">
          {m.security_tokens_password()}
          <input bind:this={passwordInput} class={inputClass} type="password" bind:value={password} required maxlength="1024" autocomplete="current-password" disabled={pending} />
        </label>
        <div><button type="submit" class={buttonClass} disabled={pending || (!ledger && !reports) || !session.data?.csrf_token}>{m.security_tokens_create()}</button></div>
      </form>
    {/if}

    {#if tokens.isPending}
      <p role="status" class="text-sm text-muted">{m.security_tokens_loading()}</p>
    {:else if tokens.isError}
      <APIFormError error={tokens.error} />
      <button type="button" class={buttonClass} onclick={() => tokens.refetch()}>{m.security_tokens_retry()}</button>
    {:else if credentials.length === 0}
      <p class="text-sm text-muted">{m.security_tokens_empty()}</p>
    {:else}
      <ul class="space-y-3">
        {#each credentials as credential (credential.id)}
          <li class="min-w-0 rounded-(--radius-control) border border-border p-3">
            <div class="flex min-w-0 flex-wrap items-start justify-between gap-3">
              <div class="min-w-0 space-y-1 text-sm">
                <p class="font-semibold break-all text-foreground">{credential.name}</p>
                <ul class="text-muted">{#each credential.scopes as scope (scope)}<li>{scopeLabel(scope)}</li>{/each}</ul>
                <p class="text-muted">{m.security_tokens_expires({ when: timestamp(credential.expires_at) })}</p>
                {#if credential.last_used_at}<p class="text-muted">{m.security_tokens_last_used({ when: timestamp(credential.last_used_at) })}</p>{/if}
                {#if credential.revoked_at}<p class="font-semibold text-foreground">{m.security_tokens_revoked()}</p>{/if}
              </div>
              {#if !credential.revoked_at}<button type="button" class={buttonClass} disabled={pending} onclick={() => { error = undefined; revokeTarget = credential; }}>{m.security_tokens_revoke()}</button>{/if}
            </div>
          </li>
        {/each}
      </ul>
      {#if tokens.hasNextPage}<button type="button" class={buttonClass} disabled={tokens.isFetchingNextPage} onclick={() => tokens.fetchNextPage()}>{m.security_tokens_more()}</button>{/if}
    {/if}
  </div>
</Panel>

{#if revokeTarget}
  <ModalDialog label={m.security_tokens_revoke()} {pending} onCancel={() => (revokeTarget = undefined)}>
    <h2 class="mb-3 text-base font-semibold text-foreground">{m.security_tokens_revoke()}</h2>
    <p class="text-sm text-foreground break-all">{m.security_tokens_confirm({ name: revokeTarget.name })}</p>
    {#if error}<APIFormError {error} />{/if}
    <div class="mt-5 flex flex-wrap justify-end gap-3">
      <button type="button" class={buttonClass} disabled={pending} onclick={() => (revokeTarget = undefined)}>{m.security_tokens_cancel()}</button>
      <button type="button" class={buttonClass} disabled={pending} onclick={revoke}>{m.security_tokens_revoke()}</button>
    </div>
  </ModalDialog>
{/if}
