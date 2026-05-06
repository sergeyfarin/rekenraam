<script lang="ts">
  import * as Card from "$lib/components/ui/card";
  import * as Alert from "$lib/components/ui/alert";
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <div>
      <h1 class="text-3xl font-bold tracking-tight">Tax</h1>
      <p class="text-muted-foreground">Prepare tax-related summaries.</p>
    </div>

    <Alert.Root>
      <Alert.Title>Post-v1 tax modules</Alert.Title>
      <Alert.Description>
        Country-specific tax calculations are deferred. Use the investment reports for raw realized gains, unrealized gains, distributions, and corporate action history.
      </Alert.Description>
    </Alert.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Raw Gain Reports</Card.Title>
        <Card.Description>Accounting data without tax interpretation</Card.Description>
      </Card.Header>
      <Card.Content>
        <p class="text-sm text-muted-foreground">
          Realized and unrealized gain reports support reusable cost-basis profiles for personal, broker, or future tax views.
        </p>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Distribution Data</Card.Title>
        <Card.Description>Dividends, interest, and reinvested activity</Card.Description>
      </Card.Header>
      <Card.Content>
        <p class="text-sm text-muted-foreground">
          Investment reports expose distributions as accounting data. Qualified, exempt, or country-specific treatment is intentionally not calculated in v1.
        </p>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Future Extensions</Card.Title>
        <Card.Description>Reserved for post-v1 tax modules</Card.Description>
      </Card.Header>
      <Card.Content>
        <p class="text-sm text-muted-foreground">
          Future trusted plugins or optional modules can add country-specific rules and exports without changing the core investment ledger.
        </p>
      </Card.Content>
    </Card.Root>
  </div>
</main>
