<script lang="ts">
  import { onMount } from 'svelte';

  type HealthResponse = {
    status: string;
  };

  let healthState = $state<'loading' | 'success' | 'error'>('loading');
  let healthMessage = $state('Checking backend health...');

  onMount(async () => {
    try {
      const response = await fetch('/api/v1/health');
      if (!response.ok) {
        throw new Error(`health request failed: ${response.status}`);
      }

      const body = (await response.json()) as HealthResponse;
      healthState = 'success';
      healthMessage = `Backend status: ${body.status}`;
    } catch {
      healthState = 'error';
      healthMessage = 'Backend status unavailable';
    }
  });
</script>

<main>
  <h1>Rekenraam</h1>
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
