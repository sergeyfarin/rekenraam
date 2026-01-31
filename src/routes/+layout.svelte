<script lang="ts">
  import "../app.css";
  import { page } from "$app/state";
  import { Button } from "$lib/components/ui/button";

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
</script>

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
