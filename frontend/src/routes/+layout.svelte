<script lang="ts">
  import { onMount } from 'svelte';
  import '../app.css';
  import ThemeToggle from '$lib/components/theme-toggle.svelte';
  import { initializeTheme } from '$lib/theme.svelte';

  let { children } = $props();

  onMount(() => {
    initializeTheme();
  });
</script>

<div class="relative">
  <div class="pointer-events-none fixed inset-x-0 top-0 z-20 flex justify-end px-4 py-4 sm:px-6 sm:py-6">
    <div class="pointer-events-auto">
      <ThemeToggle />
    </div>
  </div>

  {@render children()}
</div>