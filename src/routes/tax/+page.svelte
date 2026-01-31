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
      <Alert.Title>Coming Soon</Alert.Title>
      <Alert.Description>
        Tax reporting features are currently under development.
      </Alert.Description>
    </Alert.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Capital Gains Summary</Card.Title>
        <Card.Description>Track realized gains and losses</Card.Description>
      </Card.Header>
      <Card.Content>
        <p class="text-sm text-muted-foreground">
          Generate reports of realized capital gains and losses for tax year reporting. Includes short-term vs long-term classification.
        </p>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Dividend Income</Card.Title>
        <Card.Description>Summarize dividend payments</Card.Description>
      </Card.Header>
      <Card.Content>
        <p class="text-sm text-muted-foreground">
          Track and categorize dividend income by source, including qualified vs ordinary dividends for tax purposes.
        </p>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Tax Year Reports</Card.Title>
        <Card.Description>Export data for tax preparation</Card.Description>
      </Card.Header>
      <Card.Content>
        <p class="text-sm text-muted-foreground">
          Export investment summaries and transaction reports in formats suitable for tax software or accountants.
        </p>
      </Card.Content>
    </Card.Root>
  </div>
</main>
