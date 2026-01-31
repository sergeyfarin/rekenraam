<script lang="ts">
  import * as Card from "$lib/components/ui/card";
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <div>
      <h1 class="text-3xl font-bold tracking-tight">About</h1>
      <p class="text-muted-foreground">Rekenraam desktop app — local, private, and fast.</p>
    </div>

    <Card.Root>
      <Card.Header>
        <Card.Title>Rekenraam</Card.Title>
        <Card.Description>Personal finance management</Card.Description>
      </Card.Header>
      <Card.Content class="space-y-4">
        <p class="text-sm text-muted-foreground">
          A feature-complete personal finance application built with Tauri, Svelte, and SQLite.
        </p>
        <ul class="list-disc list-inside text-sm text-muted-foreground space-y-1">
          <li>Local-first: Your data stays on your machine</li>
          <li>Privacy-focused: No cloud sync, no telemetry</li>
          <li>Fast: Native performance with Rust backend</li>
          <li>Double-entry accounting: Professional-grade bookkeeping</li>
          <li>Investment tracking: Lots, cost basis, and capital gains</li>
        </ul>
      </Card.Content>
    </Card.Root>
  </div>
</main>
