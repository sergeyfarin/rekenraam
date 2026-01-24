<script lang="ts">
//   import { page } from "$app/stores";
  import { page } from "$app/state";
  import "carbon-components-svelte/css/all.css";

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
</script>

<nav class="app-tabs" aria-label="Primary">
  <div class="bx--tabs bx--tabs--scrollable">
    <ul class="bx--tabs__nav" role="tablist">
      {#each tabs as tab}
        <li
          class={`bx--tabs__nav-item ${page.url.pathname === tab.href ? "bx--tabs__nav-item--selected" : ""}`}
          role="presentation"
        >
          <a class="bx--tabs__nav-link" href={tab.href} role="tab" aria-selected={page.url.pathname === tab.href}>
            {tab.label}
          </a>
        </li>
      {/each}
    </ul>
  </div>
</nav>

<slot />

<style>
  .app-tabs {
    position: sticky;
    top: 0;
    z-index: 10;
    background: var(--cds-layer, #f4f4f4);
    border-bottom: 1px solid var(--cds-border-subtle, #e0e0e0);
  }

  .app-tabs :global(.bx--tabs) {
    padding: 0.5rem 1rem 0.25rem;
  }

  .app-tabs :global(.bx--tabs__nav-link) {
    text-decoration: none;
  }
</style>
