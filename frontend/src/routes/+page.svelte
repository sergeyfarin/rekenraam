<script lang="ts">
  import InstallGate from '$lib/install-gate/install-gate.svelte';
  import { m } from '$lib/paraglide/messages.js';
</script>

<!-- The first screen a new user meets still needs a name (R3a). -->
<svelte:head>
  <title>{m.app_shell_nav_title()}</title>
</svelte:head>

<InstallGate />
