<script lang="ts">
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import { modalOpen } from '#lib/components/modal-focus.ts';
  import { addTransaction } from '#lib/transactions/add-transaction.svelte.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { shellShortcut, shortcutList } from './keyboard.ts';

  // The shell's shortcut owner (#459): N adds a transaction and `?` opens this
  // help. Neither acts in an editable field or while a modal owns the keyboard.
  let open = $state(false);

  function keydown(event: KeyboardEvent) {
    const shortcut = shellShortcut(event);
    if (!shortcut || modalOpen()) return;
    event.preventDefault();
    if (shortcut === 'help') open = true;
    else void addTransaction();
  }
</script>

<svelte:window onkeydown={keydown} />

{#if open}
  <ModalDialog labelledby="keyboard-help-title" onCancel={() => (open = false)} closeOnBackdrop class="max-w-md p-5 sm:p-6">
    <h2 id="keyboard-help-title" class="text-lg font-semibold tracking-tight">{m.keyboard_help_title()}</h2>
    <dl class="mt-4 divide-y divide-border text-sm">
      {#each shortcutList() as shortcut (shortcut.label)}
        <div class="flex items-center justify-between gap-4 py-2.5">
          <dt class="flex flex-wrap items-center gap-1.5">
            {#each shortcut.keys as combination, index (combination.join('+'))}
              {#if index > 0}<span class="text-muted">{m.keyboard_help_or()}</span>{/if}
              <span class="inline-flex gap-1">
                {#each combination as key (key)}
                  <kbd class="min-w-6 rounded-sm border border-border bg-control px-1.5 py-0.5 text-center font-mono text-xs font-semibold">{key}</kbd>
                {/each}
              </span>
            {/each}
          </dt>
          <dd class="text-right text-muted">{shortcut.label}</dd>
        </div>
      {/each}
    </dl>
    <p class="mt-3 text-sm text-muted">{m.keyboard_help_note()}</p>
    <div class="mt-5 flex justify-end">
      <button
        type="button"
        class="min-h-11 rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
        onclick={() => (open = false)}
      >
        {m.keyboard_help_close()}
      </button>
    </div>
  </ModalDialog>
{/if}
