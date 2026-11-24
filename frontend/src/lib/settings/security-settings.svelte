<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import KeyRound from '@lucide/svelte/icons/key-round';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { autofocus } from '#lib/components/autofocus.ts';
  import Panel from '#lib/components/panel.svelte';
  import StatusBadge from '#lib/components/status-badge.svelte';
  import {
    activateMFATOTP,
    authSessionQueryOptions,
    disableMFA,
    enrollMFATOTP,
    getMFAStatus,
    mfaStatusQueryKey,
    mfaStatusQueryOptions,
    regenerateMFARecoveryCodes
  } from '#lib/api/auth.ts';
  import { saveOutcomeUnknown } from '#lib/api/request-key.ts';
  import { applySavedRead } from '#lib/api/saved-read.ts';
  import { m } from '#lib/paraglide/messages.js';

  const queryClient = useQueryClient();
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const statusQuery = createQuery(() => mfaStatusQueryOptions());

  const csrfToken = $derived(sessionQuery.data?.csrf_token ?? '');
  const status = $derived(statusQuery.data?.status ?? 'disabled');
  const configured = $derived(statusQuery.data?.configured ?? false);

  let password = $state('');
  let code = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  // The secret and the recovery codes are returned exactly once, so they live
  // in component state until the owner navigates away — never re-fetchable.
  let secret = $state('');
  let otpauthURI = $state('');
  let recoveryCodes = $state<string[]>([]);
  // A failed write that hands out recovery codes may still have committed
  // (#661). Once the saved status is read again, these say what the owner
  // has lost and lead to new codes; regenerating is safe to repeat. The
  // warning stays through refused retries until a write succeeds: new codes
  // shown, or MFA turned off.
  let failedCodesWrite = $state<'activate' | 'regenerate' | null>(null);
  // Whether the current error is the one the warning already explains.
  let errorExplained = $state(false);
  // From a failed write until its saved status is read: the cached status may
  // be from before the write, so the page claims neither state.
  let statusUnresolved = $state(false);
  // Only from a status that was read successfully: while the read fails, the
  // write's own error stays on screen beside the status error.
  const savedActive = $derived(statusQuery.isSuccess && status === 'active');
  const activatedWithoutCodes = $derived(failedCodesWrite === 'activate' && savedActive);
  const codesMaybeReplaced = $derived(failedCodesWrite === 'regenerate' && savedActive);

  const inputClass =
    'mt-1.5 h-10 w-full rounded-(--radius-control) border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted focus:border-accent disabled:cursor-not-allowed disabled:opacity-70';
  const labelClass = 'block text-xs font-semibold uppercase tracking-[0.12em] text-muted';
  const buttonClass =
    'inline-flex items-center gap-2 rounded-(--radius-control) bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition hover:opacity-95 disabled:cursor-not-allowed disabled:opacity-60';
  const secondaryButtonClass =
    'inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60';

  async function refreshStatus() {
    await queryClient.invalidateQueries({ queryKey: mfaStatusQueryKey });
  }

  // After a failed write, read the saved status as a request of its own: a
  // cached refetch can be cancelled and revert to the status from before the
  // write (bug class 86). If that read fails too, the query shows its error
  // rather than the old status.
  async function readSavedStatus() {
    statusUnresolved = true;
    try {
      const saved = await applySavedRead(queryClient, mfaStatusQueryKey, getMFAStatus);
      if (saved.status === 'active') {
        // The enrollment it belonged to is finished; drop the shared secret.
        secret = '';
        otpauthURI = '';
        code = '';
      }
    } catch {
      await refreshStatus();
    } finally {
      statusUnresolved = false;
    }
  }

  async function run(action: () => Promise<void>, codesWrite?: 'activate' | 'regenerate') {
    pending = true;
    formError = undefined;
    errorExplained = false;
    try {
      await action();
      failedCodesWrite = null;
    } catch (error) {
      formError = error;
      // A refused activation can mean an earlier lost one already committed,
      // so any activation failure checks; a refused regeneration replaced
      // nothing. Codes never shown after an activation stay the warning.
      if (codesWrite === 'activate' || (codesWrite === 'regenerate' && saveOutcomeUnknown(error))) {
        if (failedCodesWrite !== 'activate') failedCodesWrite = codesWrite;
        errorExplained = true;
      }
      await readSavedStatus();
    } finally {
      pending = false;
    }
  }

  async function handleEnroll(event: SubmitEvent) {
    event.preventDefault();
    await run(async () => {
      const enrollment = await enrollMFATOTP(password, csrfToken);
      secret = enrollment.secret;
      otpauthURI = enrollment.otpauth_uri;
      password = '';
      await refreshStatus();
    });
  }

  async function handleActivate(event: SubmitEvent) {
    event.preventDefault();
    await run(async () => {
      const result = await activateMFATOTP(code, csrfToken);
      recoveryCodes = result.recovery_codes;
      code = '';
      secret = '';
      otpauthURI = '';
      await refreshStatus();
    }, 'activate');
  }

  async function handleDisable() {
    await run(async () => {
      await disableMFA(password, csrfToken);
      password = '';
      recoveryCodes = [];
      await refreshStatus();
    });
  }

  async function handleRegenerate(event: SubmitEvent) {
    event.preventDefault();
    await run(async () => {
      const result = await regenerateMFARecoveryCodes(password, csrfToken);
      recoveryCodes = result.recovery_codes;
      password = '';
      await refreshStatus();
    }, 'regenerate');
  }
</script>

<section class="grid gap-4">
  <Panel>
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        {#if !statusQuery.isError && !statusUnresolved}
        <StatusBadge tone={status === 'active' ? 'positive' : 'neutral'}>
          {status === 'active' ? m.security_mfa_badge_active() : m.security_mfa_badge_inactive()}
        </StatusBadge>
        {/if}
        <h2 class="mt-3 flex items-center gap-2 text-lg font-semibold tracking-tight text-balance">
          <ShieldCheck size={20} aria-hidden="true" />
          {m.security_mfa_title()}
        </h2>
        <p class="mt-2 max-w-prose text-sm text-muted">{m.security_mfa_description()}</p>
      </div>
    </div>

    {#if !errorExplained || !(activatedWithoutCodes || codesMaybeReplaced)}
      <APIFormError error={formError} id="security-mfa-error" />
    {/if}

    {#if statusQuery.isPending || statusUnresolved}
      <p class="mt-6 text-sm text-muted">{m.security_mfa_loading()}</p>
    {:else if statusQuery.isError}
      <APIFormError error={statusQuery.error} id="security-mfa-status-error" />
    {:else if !configured}
      <p class="mt-6 max-w-prose text-sm text-foreground">{m.security_mfa_unconfigured()}</p>
    {:else if recoveryCodes.length > 0}
      <div class="mt-6 space-y-3">
        <h3 class="flex items-center gap-2 text-sm font-semibold text-foreground">
          <KeyRound size={16} aria-hidden="true" />
          {m.security_mfa_recovery_title()}
        </h3>
        <p class="max-w-prose text-sm text-muted">{m.security_mfa_recovery_warning()}</p>
        <ul class="grid gap-2 font-mono text-sm sm:grid-cols-2">
          {#each recoveryCodes as recoveryCode (recoveryCode)}
            <li class="rounded-(--radius-control) border border-border bg-control px-3 py-2">
              {recoveryCode}
            </li>
          {/each}
        </ul>
        <button type="button" class={secondaryButtonClass} onclick={() => (recoveryCodes = [])}>
          {m.security_mfa_recovery_dismiss()}
        </button>
      </div>
    {:else if secret && status !== 'active'}
      <div class="mt-6 space-y-4">
        <h3 class="text-sm font-semibold text-foreground">{m.security_mfa_enroll_step_title()}</h3>
        <p class="max-w-prose text-sm text-muted">{m.security_mfa_enroll_step_hint()}</p>
        <dl class="grid gap-2 text-sm">
          <dt class={labelClass}>{m.security_mfa_secret_label()}</dt>
          <dd class="font-mono break-all rounded-(--radius-control) border border-border bg-control px-3 py-2">
            {secret}
          </dd>
          <dt class={labelClass}>{m.security_mfa_uri_label()}</dt>
          <dd class="font-mono break-all rounded-(--radius-control) border border-border bg-control px-3 py-2 text-xs">
            {otpauthURI}
          </dd>
        </dl>
        <form class="grid max-w-sm gap-3" onsubmit={handleActivate}>
          <label>
            <span class={labelClass}>{m.security_mfa_code_label()}</span>
            <input
              bind:value={code}
              class={inputClass}
              inputmode="numeric"
              autocomplete="one-time-code"
              maxlength="64"
              required
            />
          </label>
          <div>
            <button type="submit" class={buttonClass} disabled={pending}>
              {m.security_mfa_activate()}
            </button>
          </div>
        </form>
      </div>
    {:else if status === 'active'}
      <div class="mt-6 space-y-6">
        {#if activatedWithoutCodes || codesMaybeReplaced}
          <p role="alert" class="max-w-prose rounded-(--radius-control) border border-warning/40 bg-warning-soft p-3 text-sm text-foreground">
            {activatedWithoutCodes ? m.security_mfa_activated_codes_lost() : m.security_mfa_codes_maybe_replaced()}
          </p>
        {:else}
          <p class="text-sm text-foreground">
            {m.security_mfa_codes_remaining({ count: statusQuery.data?.recovery_codes_remaining ?? 0 })}
          </p>
        {/if}

        <form class="grid max-w-sm gap-3" onsubmit={handleRegenerate}>
          <label>
            <span class={labelClass}>{m.security_mfa_password_label()}</span>
            <input
              bind:value={password}
              {@attach activatedWithoutCodes && autofocus}
              class={inputClass}
              type="password"
              autocomplete="current-password"
              maxlength="1024"
              required
            />
          </label>
          <div class="flex flex-wrap gap-2">
            <button type="submit" class={secondaryButtonClass} disabled={pending}>
              {m.security_mfa_regenerate()}
            </button>
            <button
              type="button"
              class={secondaryButtonClass}
              disabled={pending}
              onclick={handleDisable}
            >
              {m.security_mfa_disable()}
            </button>
          </div>
          <p class="text-xs text-muted">{m.security_mfa_password_hint()}</p>
        </form>
      </div>
    {:else}
      <form class="mt-6 grid max-w-sm gap-3" onsubmit={handleEnroll}>
        {#if status === 'pending'}
          <p class="text-sm text-muted">{m.security_mfa_pending_hint()}</p>
        {/if}
        <label>
          <span class={labelClass}>{m.security_mfa_password_label()}</span>
          <input
            bind:value={password}
            class={inputClass}
            type="password"
            autocomplete="current-password"
            maxlength="1024"
            required
          />
        </label>
        <div>
          <button type="submit" class={buttonClass} disabled={pending}>
            {m.security_mfa_enroll()}
          </button>
        </div>
        <p class="text-xs text-muted">{m.security_mfa_password_hint()}</p>
      </form>
    {/if}
  </Panel>
</section>
