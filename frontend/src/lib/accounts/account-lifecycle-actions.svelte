<script lang="ts">
  import { goto } from '$app/navigation';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { m } from '#lib/paraglide/messages.js';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import {
    accountsQueryKey,
    accountsQueryOptions,
    archiveAccount,
    closeAccount,
    deleteAccount,
    reopenAccount,
    restoreAccount,
    type AccountResponse
  } from '#lib/api/accounts.ts';
  import { ledgerQueryKey } from '#lib/api/ledger.ts';
  import { accountDisplayName, accountStatusLabel } from './account-labels.ts';
  import { childDependents, noDependents } from './dependents.ts';

  // Lifecycle actions on an account's own register (#567 review). Every
  // account the Ledger index manages has them here: accounts it folds into a
  // brokerage row or lists apart have no tree row, and a closed or archived
  // top-level account is reopened or restored here. The child rules are the
  // index's own (dependents.ts), read from the same full catalog the index
  // counts, archived accounts included; the server still keeps every rule it
  // owns, and its refusals are shown.
  let { account, csrfToken, afterDelete }: { account: AccountResponse; csrfToken: string | undefined; afterDelete: string } = $props();

  const queryClient = useQueryClient();
  // The catalog is read only once the owner opens the actions.
  let open = $state(false);
  const catalogQuery = createQuery(() => ({ ...accountsQueryOptions(true, false), enabled: open }));
  const dependents = $derived(catalogQuery.data ? (childDependents(catalogQuery.data.accounts).get(account.id) ?? noDependents) : undefined);
  // Until the catalog is read, actions with a child rule wait for it.
  const blocked = $derived({
    close: !dependents || dependents.active > 0,
    archive: !dependents || dependents.unarchived > 0,
    delete: !dependents || dependents.all > 0
  });
  const childrenBlock = $derived(!!dependents && dependents.all > 0);
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

<!-- Collapsed by default: the register is for transactions, and close or
     delete should never sit beside them one tap away. -->
<section aria-label={m.ledger_account_actions()}>
  <details bind:open>
  <summary class="inline-flex min-h-11 cursor-pointer items-center gap-2 text-sm font-semibold text-foreground">
    {m.ledger_account_actions()}
    <span class="font-normal text-muted">{m.ledger_account_status({ status: accountStatusLabel(account.status) })}</span>
  </summary>
  <div class="mt-2 space-y-2">
  <div class="flex flex-wrap gap-2">
    {#if account.status === 'active'}
      <button type="button" class={button} disabled={!!pending || !csrfToken || blocked.close} onclick={() => run('close', token => closeAccount(account.id, token))}>{m.accounts_action_close({ name })}</button>
    {:else if account.status === 'closed'}
      <button type="button" class={button} disabled={!!pending || !csrfToken} onclick={() => run('reopen', token => reopenAccount(account.id, token))}>{m.ledger_account_reopen({ name })}</button>
      <button type="button" class={button} disabled={!!pending || !csrfToken || blocked.archive} onclick={() => run('archive', token => archiveAccount(account.id, token))}>{m.accounts_action_archive({ name })}</button>
    {:else}
      <button type="button" class={button} disabled={!!pending || !csrfToken} onclick={() => run('restore', token => restoreAccount(account.id, token))}>{m.ledger_account_restore({ name })}</button>
    {/if}
    <button type="button" class="inline-flex min-h-11 items-center rounded-(--radius-control) border border-danger/30 bg-danger-soft px-3 text-sm font-semibold text-danger transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60" disabled={!!pending || !csrfToken || blocked.delete} onclick={remove}>{m.accounts_action_delete({ name })}</button>
  </div>
  {#if childrenBlock}<p class="text-sm text-muted">{m.ledger_account_children_block()}</p>{/if}
  {#if catalogQuery.isError}<APIFormError error={catalogQuery.error} />{/if}
  </div>
  </details>
  <APIFormError {error} />
</section>
