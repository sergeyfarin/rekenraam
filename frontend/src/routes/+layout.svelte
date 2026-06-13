<script lang="ts">
  import { QueryClientProvider } from '@tanstack/svelte-query';
  import { onMount } from 'svelte';
  import '../app.css';
  import { createQueryClient } from '$lib/query-client';
  import TopBar from '$lib/components/top-bar.svelte';
  import { initializeTheme } from '$lib/theme.svelte';

  let { children } = $props();
  const queryClient = createQueryClient();

  onMount(() => {
    initializeTheme();
  });
</script>

<QueryClientProvider client={queryClient}>
  <div class="min-h-screen">
    <TopBar />
    {@render children()}
  </div>
</QueryClientProvider>
