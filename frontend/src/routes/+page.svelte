<script lang="ts">
  import { onMount } from 'svelte';

  let message = 'loading backend hello...';

  onMount(async () => {
    const response = await fetch('/api/hello');
    const body = await response.json();
    message = body.message;
  });
</script>

<main>
  <h1>Rekenraam</h1>
  <p>{message}</p>
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
</style>
