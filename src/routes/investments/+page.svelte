<script lang="ts">
  import { onMount } from "svelte";
  import { invoke } from "@tauri-apps/api/core";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Tabs from "$lib/components/ui/tabs";
  import * as Table from "$lib/components/ui/table";
  import * as Alert from "$lib/components/ui/alert";
  import { Badge } from "$lib/components/ui/badge";

  // Tab state
  let activeTab: "holdings" | "transactions" | "lots" = "holdings";

  // Loading state
  let busy = false;
  let error = "";
  let status = "";

  // Holdings data
  type PositionLot = {
    lot_id: number;
    opened_date: string | null;
    quantity_minor: number;
    cost_basis_minor: number;
    remaining_cost_basis_minor: number;
    converted_value_minor: number | null;
    converted_cost_basis_minor: number | null;
    price_missing: boolean | null;
  };

  type Position = {
    account_id: number;
    account_name: string;
    account_type: string;
    commodity_id: number;
    commodity_name: string;
    commodity_scale: number;
    balance_minor: number;
    lots: PositionLot[];
  };

  type ConvertedPosition = Position & {
    value_minor: number;
    price_missing: boolean;
  };

  type LotWithHoldingPeriod = {
    lot_id: number;
    account_id: number;
    account_name: string;
    commodity_id: number;
    commodity_name: string;
    opened_date: string | null;
    quantity_minor: number;
    cost_basis_minor: number;
    commodity_scale: number;
    holding_days: number | null;
    is_long_term: boolean;
  };

  type Account = {
    id: number;
    name: string;
    type: string;
    commodity_id: number;
  };

  type Commodity = {
    id: number;
    kind: string;
    symbol: string | null;
    name: string;
    scale: number;
  };

  let positions: Position[] = [];
  let convertedPositions: ConvertedPosition[] = [];
  let lotsWithHolding: LotWithHoldingPeriod[] = [];
  let accounts: Account[] = [];
  let commodities: Commodity[] = [];
  let quoteCommodityId: number | null = null;
  let asOfDate: string = "";

  // Totals
  let totalValue = 0;
  let totalCostBasis = 0;
  let totalGainLoss = 0;

  // Buy/Sell dialog
  let showTradeDialog = false;
  let tradeType: "buy" | "sell" = "buy";
  let tradeForm = {
    txn_date: "",
    investment_account_id: null as number | null,
    cash_account_id: null as number | null,
    commodity_id: null as number | null,
    quantity: "",
    cash_amount: "",
    memo: "",
  };

  // Dividend dialog
  let showDividendDialog = false;
  let dividendForm = {
    txn_date: "",
    investment_account_id: null as number | null,
    cash_account_id: null as number | null,
    income_account_id: null as number | null,
    commodity_id: null as number | null,
    amount: "",
    memo: "",
  };

  onMount(async () => {
    asOfDate = new Date().toISOString().split("T")[0];
    await loadAccounts();
    await loadCommodities();
    await loadPositions();
  });

  async function loadAccounts() {
    try {
      accounts = await invoke<Account[]>("list_accounts", { bookId: 1, includeClosed: false });
    } catch (e) {
      error = `Failed to load accounts: ${String(e)}`;
    }
  }

  async function loadCommodities() {
    try {
      commodities = await invoke<Commodity[]>("list_commodities", { bookId: 1 });
      const currencies = commodities.filter((c) => c.kind === "currency");
      if (currencies.length > 0 && !quoteCommodityId) {
        quoteCommodityId = currencies[0].id;
      }
    } catch (e) {
      error = `Failed to load commodities: ${String(e)}`;
    }
  }

  async function loadPositions() {
    error = "";
    busy = true;
    try {
      positions = await invoke<Position[]>("get_positions", {
        bookId: 1,
        asOfDate: asOfDate || null,
      });

      // Convert positions to base currency if quote commodity is selected
      if (quoteCommodityId) {
        convertedPositions = await invoke<ConvertedPosition[]>("convert_positions", {
          baseCommodityId: quoteCommodityId,
          asOfDate: asOfDate || null,
        });

        // Calculate totals
        totalValue = convertedPositions.reduce((sum, p) => sum + p.value_minor, 0);
        totalCostBasis = convertedPositions.reduce((sum, p) => {
          return sum + p.lots.reduce((lotSum, lot) => lotSum + (lot.converted_cost_basis_minor || lot.remaining_cost_basis_minor), 0);
        }, 0);
        totalGainLoss = totalValue - totalCostBasis;
      }
    } catch (e) {
      error = `Failed to load positions: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function loadLots() {
    error = "";
    busy = true;
    try {
      lotsWithHolding = await invoke<LotWithHoldingPeriod[]>("list_lots_with_holding_period", {
        bookId: 1,
        asOfDate: asOfDate || null,
      });
    } catch (e) {
      error = `Failed to load lots: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function handleTabChange(tab: typeof activeTab) {
    activeTab = tab;
    if (tab === "lots" && lotsWithHolding.length === 0) {
      await loadLots();
    }
  }

  // Trade dialog functions
  function openBuyDialog() {
    tradeType = "buy";
    tradeForm = {
      txn_date: new Date().toISOString().split("T")[0],
      investment_account_id: null,
      cash_account_id: null,
      commodity_id: null,
      quantity: "",
      cash_amount: "",
      memo: "",
    };
    showTradeDialog = true;
  }

  function openSellDialog() {
    tradeType = "sell";
    tradeForm = {
      txn_date: new Date().toISOString().split("T")[0],
      investment_account_id: null,
      cash_account_id: null,
      commodity_id: null,
      quantity: "",
      cash_amount: "",
      memo: "",
    };
    showTradeDialog = true;
  }

  function closeTradeDialog() {
    showTradeDialog = false;
  }

  async function submitTrade() {
    if (!tradeForm.investment_account_id || !tradeForm.cash_account_id || !tradeForm.commodity_id) {
      error = "Please fill in all required fields";
      return;
    }

    const commodity = commodities.find((c) => c.id === tradeForm.commodity_id);
    const quantityMinor = Math.round(parseFloat(tradeForm.quantity) * (commodity?.scale || 1000000));
    const cashAmountMinor = Math.round(parseFloat(tradeForm.cash_amount) * 100);

    error = "";
    status = "";
    busy = true;

    try {
      if (tradeType === "buy") {
        await invoke("buy_commodity", {
          input: {
            txn_date: tradeForm.txn_date,
            investment_account_id: tradeForm.investment_account_id,
            cash_account_id: tradeForm.cash_account_id,
            commodity_id: tradeForm.commodity_id,
            quantity_minor: quantityMinor,
            cash_amount_minor: cashAmountMinor,
            memo: tradeForm.memo || null,
            payee_id: null,
            status: null,
          },
        });
        status = "Buy transaction created successfully";
      } else {
        await invoke("sell_commodity", {
          input: {
            txn_date: tradeForm.txn_date,
            investment_account_id: tradeForm.investment_account_id,
            cash_account_id: tradeForm.cash_account_id,
            commodity_id: tradeForm.commodity_id,
            quantity_minor: quantityMinor,
            proceeds_minor: cashAmountMinor,
            lot_allocation_method: "FIFO",
            lot_allocations: null,
            memo: tradeForm.memo || null,
            payee_id: null,
            status: null,
          },
        });
        status = "Sell transaction created successfully";
      }

      closeTradeDialog();
      await loadPositions();
      if (activeTab === "lots") {
        await loadLots();
      }
    } catch (e) {
      error = `Failed to create ${tradeType} transaction: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  // Dividend dialog functions
  function openDividendDialog() {
    dividendForm = {
      txn_date: new Date().toISOString().split("T")[0],
      investment_account_id: null,
      cash_account_id: null,
      income_account_id: null,
      commodity_id: null,
      amount: "",
      memo: "",
    };
    showDividendDialog = true;
  }

  function closeDividendDialog() {
    showDividendDialog = false;
  }

  async function submitDividend() {
    if (!dividendForm.investment_account_id || !dividendForm.cash_account_id || !dividendForm.income_account_id) {
      error = "Please fill in all required fields";
      return;
    }

    const amountMinor = Math.round(parseFloat(dividendForm.amount) * 100);

    error = "";
    status = "";
    busy = true;

    try {
      await invoke("create_dividend_transaction", {
        input: {
          txn_date: dividendForm.txn_date,
          investment_account_id: dividendForm.investment_account_id,
          cash_account_id: dividendForm.cash_account_id,
          income_account_id: dividendForm.income_account_id,
          amount_minor: amountMinor,
          commodity_id: dividendForm.commodity_id,
          memo: dividendForm.memo || null,
          payee_id: null,
          status: null,
        },
      });
      status = "Dividend transaction created successfully";
      closeDividendDialog();
      await loadPositions();
    } catch (e) {
      error = `Failed to create dividend transaction: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  function formatCurrency(minor: number, scale = 100) {
    return (minor / scale).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  }

  function formatQuantity(minor: number, scale: number) {
    return (minor / scale).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 6 });
  }

  function formatDate(dateStr: string | null) {
    if (!dateStr) return "—";
    return new Date(dateStr).toLocaleDateString();
  }

  $: investmentAccounts = accounts.filter((a) => a.type === "investment" || a.type === "brokerage" || a.type === "stock");
  $: cashAccounts = accounts.filter((a) => a.type === "checking" || a.type === "savings" || a.type === "cash" || a.type === "asset");
  $: incomeAccounts = accounts.filter((a) => a.type === "income");
  $: securities = commodities.filter((c) => c.kind === "security" || c.kind === "stock" || c.kind === "mutual_fund");
  $: currencies = commodities.filter((c) => c.kind === "currency");
</script>

<main class="py-6">
  <div class="container mx-auto px-6 space-y-6">
    <div>
      <h1 class="text-3xl font-bold tracking-tight">Investments</h1>
      <p class="text-muted-foreground">Track holdings and portfolio performance.</p>
    </div>

    <!-- Actions & Filters -->
    <Card.Root>
      <Card.Content class="pt-6">
        <div class="flex flex-wrap items-center justify-between gap-4">
          <div class="flex gap-2">
            <Button onclick={openBuyDialog} disabled={busy}>Buy</Button>
            <Button variant="secondary" onclick={openSellDialog} disabled={busy}>Sell</Button>
            <Button variant="secondary" onclick={openDividendDialog} disabled={busy}>Dividend</Button>
          </div>
          <div class="flex flex-wrap items-center gap-4">
            <div class="flex items-center gap-2">
              <Label for="as-of-date">As of:</Label>
              <Input id="as-of-date" type="date" bind:value={asOfDate} class="w-40" />
            </div>
            <div class="flex items-center gap-2">
              <Label for="quote-currency">Quote:</Label>
              <select id="quote-currency" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={quoteCommodityId}>
                {#each currencies as c}
                  <option value={c.id}>{c.symbol || c.name}</option>
                {/each}
              </select>
            </div>
            <Button variant="outline" onclick={loadPositions} disabled={busy}>
              {busy ? "Loading..." : "Refresh"}
            </Button>
          </div>
        </div>
      </Card.Content>
    </Card.Root>

    {#if error}
      <Alert.Root variant="destructive">
        <Alert.Title>Error</Alert.Title>
        <Alert.Description>{error}</Alert.Description>
      </Alert.Root>
    {/if}

    {#if status}
      <Alert.Root>
        <Alert.Title>Success</Alert.Title>
        <Alert.Description>{status}</Alert.Description>
      </Alert.Root>
    {/if}

    <!-- Summary Cards -->
    <div class="grid gap-4 md:grid-cols-3">
      <Card.Root>
        <Card.Header class="pb-2">
          <Card.Description>Total Value</Card.Description>
        </Card.Header>
        <Card.Content>
          <div class="text-2xl font-bold">{formatCurrency(totalValue)}</div>
        </Card.Content>
      </Card.Root>
      <Card.Root>
        <Card.Header class="pb-2">
          <Card.Description>Total Cost Basis</Card.Description>
        </Card.Header>
        <Card.Content>
          <div class="text-2xl font-bold">{formatCurrency(totalCostBasis)}</div>
        </Card.Content>
      </Card.Root>
      <Card.Root class={totalGainLoss >= 0 ? "border-green-200 bg-green-50" : "border-red-200 bg-red-50"}>
        <Card.Header class="pb-2">
          <Card.Description>Unrealized Gain/Loss</Card.Description>
        </Card.Header>
        <Card.Content>
          <div class="text-2xl font-bold">{formatCurrency(totalGainLoss)}</div>
        </Card.Content>
      </Card.Root>
    </div>

    <!-- Tabs -->
    <Tabs.Root value={activeTab} onValueChange={(v) => handleTabChange(v as typeof activeTab)}>
      <Tabs.List>
        <Tabs.Trigger value="holdings">Holdings</Tabs.Trigger>
        <Tabs.Trigger value="lots">Tax Lots</Tabs.Trigger>
      </Tabs.List>

      <Tabs.Content value="holdings">
        <Card.Root>
          <Card.Content class="pt-6">
            {#if convertedPositions.length === 0 && positions.length === 0}
              <p class="text-muted-foreground">No holdings found. Use Buy to add securities.</p>
            {:else}
              <Table.Root>
                <Table.Header>
                  <Table.Row>
                    <Table.Head>Account</Table.Head>
                    <Table.Head>Security</Table.Head>
                    <Table.Head class="text-right">Quantity</Table.Head>
                    <Table.Head class="text-right">Value</Table.Head>
                    <Table.Head class="text-right">Cost Basis</Table.Head>
                    <Table.Head class="text-right">Gain/Loss</Table.Head>
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {#each convertedPositions.length > 0 ? convertedPositions : positions as pos}
                    {@const costBasis = pos.lots.reduce((sum, lot) => sum + (lot.converted_cost_basis_minor || lot.remaining_cost_basis_minor), 0)}
                    {@const value = 'value_minor' in pos ? (pos.value_minor as number) : 0}
                    {@const gainLoss = value - costBasis}
                    <Table.Row>
                      <Table.Cell>{pos.account_name}</Table.Cell>
                      <Table.Cell>{pos.commodity_name}</Table.Cell>
                      <Table.Cell class="text-right">{formatQuantity(pos.balance_minor, pos.commodity_scale)}</Table.Cell>
                      <Table.Cell class="text-right">
                        {formatCurrency(value)}
                        {#if 'price_missing' in pos && pos.price_missing}
                          <span class="text-yellow-600" title="Price missing">⚠</span>
                        {/if}
                      </Table.Cell>
                      <Table.Cell class="text-right">{formatCurrency(costBasis)}</Table.Cell>
                      <Table.Cell class="text-right {gainLoss >= 0 ? 'text-green-600' : 'text-red-600'}">
                        {formatCurrency(gainLoss)}
                      </Table.Cell>
                    </Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
            {/if}
          </Card.Content>
        </Card.Root>
      </Tabs.Content>

      <Tabs.Content value="lots">
        <Card.Root>
          <Card.Content class="pt-6">
            {#if lotsWithHolding.length === 0}
              <p class="text-muted-foreground">No tax lots found.</p>
            {:else}
              <Table.Root>
                <Table.Header>
                  <Table.Row>
                    <Table.Head>Account</Table.Head>
                    <Table.Head>Security</Table.Head>
                    <Table.Head>Opened</Table.Head>
                    <Table.Head class="text-right">Quantity</Table.Head>
                    <Table.Head class="text-right">Cost Basis</Table.Head>
                    <Table.Head class="text-right">Days Held</Table.Head>
                    <Table.Head>Term</Table.Head>
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {#each lotsWithHolding as lot}
                    <Table.Row>
                      <Table.Cell>{lot.account_name}</Table.Cell>
                      <Table.Cell>{lot.commodity_name}</Table.Cell>
                      <Table.Cell>{formatDate(lot.opened_date)}</Table.Cell>
                      <Table.Cell class="text-right">{formatQuantity(lot.quantity_minor, lot.commodity_scale)}</Table.Cell>
                      <Table.Cell class="text-right">{formatCurrency(lot.cost_basis_minor)}</Table.Cell>
                      <Table.Cell class="text-right">{lot.holding_days ?? "—"}</Table.Cell>
                      <Table.Cell>
                        <Badge variant={lot.is_long_term ? "default" : "secondary"}>
                          {lot.is_long_term ? "Long" : "Short"}
                        </Badge>
                      </Table.Cell>
                    </Table.Row>
                  {/each}
                </Table.Body>
              </Table.Root>
            {/if}
          </Card.Content>
        </Card.Root>
      </Tabs.Content>
    </Tabs.Root>
  </div>
</main>

<!-- Buy/Sell Dialog -->
<Dialog.Root bind:open={showTradeDialog}>
  <Dialog.Content class="max-w-md">
    <Dialog.Header>
      <Dialog.Title>{tradeType === "buy" ? "Buy Security" : "Sell Security"}</Dialog.Title>
    </Dialog.Header>
    <div class="space-y-4 py-4">
      <div class="space-y-2">
        <Label for="trade-date">Date</Label>
        <Input id="trade-date" type="date" bind:value={tradeForm.txn_date} />
      </div>
      <div class="space-y-2">
        <Label for="trade-inv-account">Investment Account</Label>
        <select id="trade-inv-account" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={tradeForm.investment_account_id}>
          <option value={null}>Select account...</option>
          {#each investmentAccounts as a}
            <option value={a.id}>{a.name}</option>
          {/each}
        </select>
      </div>
      <div class="space-y-2">
        <Label for="trade-cash-account">Cash Account</Label>
        <select id="trade-cash-account" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={tradeForm.cash_account_id}>
          <option value={null}>Select account...</option>
          {#each cashAccounts as a}
            <option value={a.id}>{a.name}</option>
          {/each}
        </select>
      </div>
      <div class="space-y-2">
        <Label for="trade-security">Security</Label>
        <select id="trade-security" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={tradeForm.commodity_id}>
          <option value={null}>Select security...</option>
          {#each securities as c}
            <option value={c.id}>{c.symbol || c.name}</option>
          {/each}
        </select>
      </div>
      <div class="space-y-2">
        <Label for="trade-quantity">Quantity</Label>
        <Input id="trade-quantity" type="number" step="0.000001" bind:value={tradeForm.quantity} placeholder="Number of shares" />
      </div>
      <div class="space-y-2">
        <Label for="trade-amount">{tradeType === "buy" ? "Total Cost" : "Proceeds"}</Label>
        <Input id="trade-amount" type="number" step="0.01" bind:value={tradeForm.cash_amount} placeholder="Amount" />
      </div>
      <div class="space-y-2">
        <Label for="trade-memo">Memo (optional)</Label>
        <Input id="trade-memo" bind:value={tradeForm.memo} placeholder="Description" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="outline" onclick={closeTradeDialog}>Cancel</Button>
      <Button onclick={submitTrade} disabled={busy}>
        {tradeType === "buy" ? "Buy" : "Sell"}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Dividend Dialog -->
<Dialog.Root bind:open={showDividendDialog}>
  <Dialog.Content class="max-w-md">
    <Dialog.Header>
      <Dialog.Title>Record Dividend</Dialog.Title>
    </Dialog.Header>
    <div class="space-y-4 py-4">
      <div class="space-y-2">
        <Label for="div-date">Date</Label>
        <Input id="div-date" type="date" bind:value={dividendForm.txn_date} />
      </div>
      <div class="space-y-2">
        <Label for="div-inv-account">Investment Account (source)</Label>
        <select id="div-inv-account" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={dividendForm.investment_account_id}>
          <option value={null}>Select account...</option>
          {#each investmentAccounts as a}
            <option value={a.id}>{a.name}</option>
          {/each}
        </select>
      </div>
      <div class="space-y-2">
        <Label for="div-cash-account">Cash Account (destination)</Label>
        <select id="div-cash-account" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={dividendForm.cash_account_id}>
          <option value={null}>Select account...</option>
          {#each cashAccounts as a}
            <option value={a.id}>{a.name}</option>
          {/each}
        </select>
      </div>
      <div class="space-y-2">
        <Label for="div-income-account">Income Account</Label>
        <select id="div-income-account" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={dividendForm.income_account_id}>
          <option value={null}>Select account...</option>
          {#each incomeAccounts as a}
            <option value={a.id}>{a.name}</option>
          {/each}
        </select>
      </div>
      <div class="space-y-2">
        <Label for="div-security">Security (optional)</Label>
        <select id="div-security" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={dividendForm.commodity_id}>
          <option value={null}>None</option>
          {#each securities as c}
            <option value={c.id}>{c.symbol || c.name}</option>
          {/each}
        </select>
      </div>
      <div class="space-y-2">
        <Label for="div-amount">Amount</Label>
        <Input id="div-amount" type="number" step="0.01" bind:value={dividendForm.amount} placeholder="Dividend amount" />
      </div>
      <div class="space-y-2">
        <Label for="div-memo">Memo (optional)</Label>
        <Input id="div-memo" bind:value={dividendForm.memo} placeholder="Description" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="outline" onclick={closeDividendDialog}>Cancel</Button>
      <Button onclick={submitDividend} disabled={busy}>
        Record Dividend
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
