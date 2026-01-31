<script lang="ts">
  import * as Card from "$lib/components/ui/card";
  import * as Alert from "$lib/components/ui/alert";
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <div>
      <h1 class="text-3xl font-bold tracking-tight">Planning</h1>
      <p class="text-muted-foreground">Plan budgets and forecasts.</p>
    </div>

    <Alert.Root>
      <Alert.Title>Coming Soon</Alert.Title>
      <Alert.Description>
        Budget planning and forecasting features are currently under development.
      </Alert.Description>
    </Alert.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Budget Planning</Card.Title>
        <Card.Description>Set spending limits by category</Card.Description>
      </Card.Header>
      <Card.Content>
        <p class="text-sm text-muted-foreground">
          This feature will allow you to create monthly or yearly budgets, track spending against limits, and receive alerts when approaching budget thresholds.
        </p>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Financial Forecasting</Card.Title>
        <Card.Description>Project future balances</Card.Description>
      </Card.Header>
      <Card.Content>
        <p class="text-sm text-muted-foreground">
          Forecast your financial future based on recurring transactions, scheduled payments, and historical spending patterns.
        </p>
      </Card.Content>
    </Card.Root>
  </div>
</main>
