<script lang="ts">
  import { goto } from '$app/navigation';
  import { useQueryClient } from '@tanstack/svelte-query';
  import { m } from '#lib/paraglide/messages.js';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import {
    accountsQueryKey,
    archiveAccount,
    closeAccount,
    deleteAccount,
    reopenAccount,
    restoreAccount,
    type AccountResponse
  } from '#lib/api/accounts.ts';
  import { ledgerQueryKey } from '#lib/api/ledger.ts';
  import { accountDisplayName, accountStatusLabel } from './account-labels.ts';

  // Lifecycle actions for an account the Ledger index folds into a brokerage
  // row (a holding, a cash child, another child) or lists apart (an
  // unparented holding). Those accounts are never tree rows, so their own
  // register is where they are closed, archived, restored or deleted; this is
  // also how the owner clears the dependents that keep a brokerage's own
  // actions disabled (#567 review). The server keeps every rule: refusals
  // are shown, never pre-empted with a guess.
  let { account, csrfToken, afterDelete }: { account: AccountResponse; csrfToken: string | undefined; afterDelete: string } = $props();

  const queryClient = useQueryClient();
  let pending = $state('');
  let error = $state<unknown>(undefined);
  const name = $derived(accountDisplayName(account));
  const button = 'inline-flex min-h-11 items-center rounded-(--radius-control) border border-border bg-control px-3 text-sm font-semibold text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60';

  async function run(key: string, action: (token: string) => Promise<unknown>, deleted = false) {
    if (!csrfToken || pending) return;
    pending = key;
    error = undefined;
    try {
      await action(csrfToken);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: accountsQueryKey }),
        queryClient.invalidateQueries({ queryKey: ledgerQueryKey })
      ]);
      if (deleted) await goto(afterDelete);
    } catch (err) {
      error = err;
    } finally {
      pending = '';
    }
  }

  function remove() {
    if (!confirm(m.accounts_delete_confirm({ name }))) return;
    void run('delete', token => deleteAccount(account.id, token), true);
  }
</script>

<section aria-label={m.ledger_account_actions()} class="space-y-2">
  <p class="text-sm text-muted">{m.ledger_account_status({ status: accountStatusLabel(account.status) })}</p>
  <div class="flex flex-wrap gap-2">
    {#if account.status === 'active'}
      <button type="button" class={button} disabled={!!pending || !csrfToken} onclick={() => run('close', token => closeAccount(account.id, token))}>{m.accounts_action_close({ name })}</button>
    {:else if account.status === 'closed'}
      <button type="button" class={button} disabled={!!pending || !csrfToken} onclick={() => run('reopen', token => reopenAccount(account.id, token))}>{m.ledger_account_reopen({ name })}</button>
      <button type="button" class={button} disabled={!!pending || !csrfToken} onclick={() => run('archive', token => archiveAccount(account.id, token))}>{m.accounts_action_archive({ name })}</button>
    {:else}
      <button type="button" class={button} disabled={!!pending || !csrfToken} onclick={() => run('restore', token => restoreAccount(account.id, token))}>{m.ledger_account_restore({ name })}</button>
    {/if}
    <button type="button" class="inline-flex min-h-11 items-center rounded-(--radius-control) border border-danger/30 bg-danger-soft px-3 text-sm font-semibold text-danger transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60" disabled={!!pending || !csrfToken} onclick={remove}>{m.accounts_action_delete({ name })}</button>
  </div>
  <APIFormError {error} />
</section>
