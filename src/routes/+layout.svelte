<script lang="ts">
//   import { page } from "$app/stores";
  import { page } from "$app/state";
  import { Column,
    Content,
    Grid,
    Header,
    HeaderNav,
    HeaderNavItem,
    HeaderNavMenu,
    Row,
    SkipToContent, } from "carbon-components-svelte";
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

<Header companyName="" platformName="Rekenraam 🪙">
  <!-- <svelte:fragment slot="skipToContent">
    <SkipToContent />
  </svelte:fragment> -->
  <HeaderNav>
  <!-- <HeaderNavigation aria-label="Primary"> -->
    {#each tabs as tab}
      <HeaderNavItem href={tab.href} text={tab.label} />
    {/each}
  </HeaderNav>
</Header>

<div class="bx--content">
  <slot />
</div>
