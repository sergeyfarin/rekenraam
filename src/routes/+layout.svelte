<script lang="ts">
  import "../app.css";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { Button } from "$lib/components/ui/button";
  import * as Dialog from "$lib/components/ui/dialog";

  type Tab = { label: string; href: string };

  const tabs = [
    { label: "Home", href: "/" },
    { label: "Transactions", href: "/transactions" },
    { label: "Accounts", href: "/accounts" },
    { label: "Investments", href: "/investments" },
    { label: "Reports", href: "/reports" },
    { label: "Planning", href: "/planning" },
    { label: "Tax", href: "/tax" },
    { label: "Settings", href: "/settings" },
    { label: "About", href: "/about" }
  ] satisfies Tab[];

  $: currentPath = page.url.pathname;

  let helpOpen = false;

  function handleKeydown(e: KeyboardEvent) {
    // Ignore shortcuts when user is typing in an input, textarea, select, or contenteditable
    const target = e.target as HTMLElement;
    if (
      target.tagName === "INPUT" ||
      target.tagName === "TEXTAREA" ||
      target.tagName === "SELECT" ||
      target.isContentEditable
    ) return;

    // Ignore if a dialog is open (avoid double-handling)
    if (document.querySelector('[data-slot="dialog-content"]')) return;

    // ? — help
    if (e.key === "?" && !e.ctrlKey && !e.metaKey) {
      helpOpen = true;
      return;
    }

    // N — new transaction (navigate to transactions page and open dialog)
    if (e.key === "n" && !e.ctrlKey && !e.metaKey && !e.shiftKey) {
      goto("/transactions?new=1");
      return;
    }

  }
</script>

<svelte:window on:keydown={handleKeydown} />

<header class="sticky top-0 z-10 border-b border-border bg-background">
  <div class="container mx-auto flex h-16 items-center justify-between gap-4 px-6">
    <div class="font-semibold text-lg">Rekenraam 🪙</div>
    <nav class="flex flex-wrap gap-1" aria-label="Primary">
      {#each tabs as tab}
        <a
          href={tab.href}
          class="px-3 py-1.5 rounded-md font-medium text-sm transition-colors {currentPath === tab.href
            ? 'bg-accent text-foreground'
            : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'}"
        >
          {tab.label}
        </a>
      {/each}
    </nav>
  </div>
</header>

<slot />

<!-- Help / keyboard shortcut modal -->
<Dialog.Root bind:open={helpOpen}>
  <Dialog.Content class="max-w-sm">
    <Dialog.Header>
      <Dialog.Title>Keyboard Shortcuts</Dialog.Title>
    </Dialog.Header>
    <div class="space-y-2 py-2 text-sm">
      <div class="grid grid-cols-2 gap-x-4 gap-y-1">
        <kbd class="font-mono bg-muted rounded px-1">N</kbd>
        <span>New transaction</span>
        <kbd class="font-mono bg-muted rounded px-1">?</kbd>
        <span>Show this help</span>
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="outline" onclick={() => helpOpen = false}>Close</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
