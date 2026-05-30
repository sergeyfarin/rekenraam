<script lang="ts">
  import { QueryClientProvider } from '@tanstack/svelte-query';
  import { onMount } from 'svelte';
  import '../app.css';
  import { createQueryClient } from '$lib/query-client';
  import ThemeToggle from '$lib/components/theme-toggle.svelte';
  import { initializeTheme } from '$lib/theme.svelte';

  let { children } = $props();
  const queryClient = createQueryClient();

  onMount(() => {
    initializeTheme();
  });
</script>

<QueryClientProvider client={queryClient}>
  <div class="relative">
    <div class="pointer-events-none fixed inset-x-0 top-0 z-20 flex justify-end px-4 py-4 sm:px-6 sm:py-6">
      <div class="pointer-events-auto">
        <ThemeToggle />
      </div>
    </div>

    {@render children()}
  </div>
</QueryClientProvider>