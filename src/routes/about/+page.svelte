<script lang="ts">
  import * as Card from "$lib/components/ui/card";
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <div>
      <h1 class="text-3xl font-bold tracking-tight">About</h1>
      <p class="text-muted-foreground">Self-hosted personal finance management.</p>
    </div>

    <Card.Root>
      <Card.Header>
        <Card.Title>Rekenraam</Card.Title>
        <Card.Description>Personal finance management</Card.Description>
      </Card.Header>
      <Card.Content class="space-y-4">
        <p class="text-sm text-muted-foreground">
          A personal finance application built for a self-hosted SvelteKit, FastAPI, and SQLite stack.
        </p>
        <ul class="list-disc list-inside text-sm text-muted-foreground space-y-1">
          <li>Self-hosted: Run the web app and database in Docker</li>
          <li>Private by design: No hosted service or telemetry requirement</li>
          <li>Web-native: Shared access across browsers and devices</li>
          <li>Double-entry accounting: Professional-grade bookkeeping</li>
          <li>Investment tracking: Lots, cost basis, and capital gains</li>
        </ul>
      </Card.Content>
    </Card.Root>
  </div>
</main>
