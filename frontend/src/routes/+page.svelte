<script lang="ts">
  import { onMount } from 'svelte';
  import { m } from '$lib/paraglide/messages.js';

  type HealthResponse = {
    status: string;
  };

  let healthState = $state<'loading' | 'success' | 'error'>('loading');
  let healthMessage = $state(m.checking_backend_health());

  onMount(async () => {
    try {
      const response = await fetch('/api/v1/health');
      if (!response.ok) {
        throw new Error(`health request failed: ${response.status}`);
      }

      const body = (await response.json()) as HealthResponse;
      healthState = 'success';
      healthMessage = m.backend_status({ status: body.status });
    } catch {
      healthState = 'error';
      healthMessage = m.backend_status_unavailable();
    }
  });
</script>

<main>
  <h1>{m.app_name()}</h1>
  <p data-state={healthState}>{healthMessage}</p>
</main>

<style>
  main {
    min-height: 100vh;
    display: grid;
    place-content: center;
    gap: 1rem;
    font-family: system-ui, sans-serif;
    text-align: center;
  }

  h1,
  p {
    margin: 0;
  }

  p[data-state='error'] {
    color: #8a1c1c;
  }
</style>
