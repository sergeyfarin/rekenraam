<script lang="ts">
//   import { page } from "$app/stores";
  import "../app.css";
  import { page } from "$app/state";

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

<header class="app-header">
  <div class="container header-inner">
    <div class="brand">Rekenraam 🪙</div>
    <nav class="nav" aria-label="Primary">
      {#each tabs as tab}
        <a href={tab.href} class:active={currentPath === tab.href}>{tab.label}</a>
      {/each}
    </nav>
  </div>
</header>

<slot />
