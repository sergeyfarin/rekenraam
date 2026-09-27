<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import type { AccountResponse } from '$lib/api/accounts';
  import type { CurrencyResponse } from '$lib/api/currencies';
  import { newTradeCharge, type TradeChargeDraft } from './trade-economics';

  let {
    exactMode = $bindable(), gross = $bindable(), settlementDate = $bindable(), charges = $bindable(),
    side, cashCommodityID, accounts, currencies
  }: {
    exactMode: boolean;
    gross: string;
    settlementDate: string;
    charges: TradeChargeDraft[];
    side: 'buy' | 'sell';
    cashCommodityID: number | undefined;
    accounts: AccountResponse[];
    currencies: CurrencyResponse[];
  } = $props();

  const cashAccounts = $derived(accounts.filter((a) => a.status === 'active' && a.allows_postings && a.account_class === 'asset' && a.account_kind !== 'security_holding' && a.account_kind !== 'fund_holding'));

  function removeCharge(index: number) {
    charges = charges.filter((_, current) => current !== index);
  }
</script>

<details class="rounded-(--radius-panel) border border-border bg-surface p-3" open={exactMode}>
  <summary class="cursor-pointer text-sm font-medium text-foreground">{m.investments_trade_exact_title()}</summary>
  <div class="mt-3 space-y-3">
    <label class="flex items-center gap-2 text-sm text-foreground">
      <input type="checkbox" bind:checked={exactMode} />
      {m.investments_trade_exact_enable()}
    </label>
    {#if exactMode}
      <p class="text-xs text-muted">{m.investments_trade_exact_help()}</p>
      <div>
        <label for="trade-gross-{side}" class="mb-1 block text-sm font-medium text-foreground">{m.investments_trade_gross()}</label>
        <input id="trade-gross-{side}" type="text" inputmode="decimal" bind:value={gross} required
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
      </div>
      <div>
        <label for="trade-settlement-{side}" class="mb-1 block text-sm font-medium text-foreground">{m.investments_trade_settlement_date()}</label>
        <input id="trade-settlement-{side}" type="date" bind:value={settlementDate}
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
      </div>
      {#each charges as charge, index (index)}
        <fieldset class="space-y-2 rounded-(--radius-control) border border-border p-3">
          <legend class="px-1 text-sm text-foreground">{m.investments_trade_charge()}</legend>
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <label class="text-xs text-muted">{m.investments_trade_charge_kind()}
              <select bind:value={charge.kind} class="mt-1 w-full rounded-(--radius-control) border border-border bg-control px-2 py-2 text-sm text-foreground">
                <option value="commission">{m.investments_trade_commission()}</option>
                <option value="transaction_tax">{m.investments_trade_transaction_tax()}</option>
                <option value="other_fee">{m.investments_trade_other_fee()}</option>
                <option value="rebate">{m.investments_trade_rebate()}</option>
              </select>
            </label>
            <label class="text-xs text-muted">{m.investments_trade_charge_amount()}
              <input type="text" inputmode="decimal" bind:value={charge.amount} required
                class="mt-1 w-full rounded-(--radius-control) border border-border bg-control px-2 py-2 text-sm font-mono text-foreground" />
            </label>
            <label class="text-xs text-muted">{m.investments_trade_charge_currency()}
              <select bind:value={charge.commodityID} class="mt-1 w-full rounded-(--radius-control) border border-border bg-control px-2 py-2 text-sm text-foreground">
                <option value="">{m.investments_trade_settlement_currency()}</option>
                {#each currencies as currency (currency.id)}<option value={String(currency.id)}>{currency.code}</option>{/each}
              </select>
            </label>
            <label class="text-xs text-muted">{m.investments_trade_charge_treatment()}
              <select bind:value={charge.treatment} class="mt-1 w-full rounded-(--radius-control) border border-border bg-control px-2 py-2 text-sm text-foreground">
                <option value="">{m.investments_trade_policy_default()}</option>
                <option value="clearing_included">{m.investments_trade_capitalize()}</option>
                <option value="separately_expensed">{m.investments_trade_expense()}</option>
              </select>
            </label>
          </div>
          {#if charge.commodityID === '' || Number(charge.commodityID) === cashCommodityID}
            <label class="flex items-center gap-2 text-xs text-foreground">
              <input type="checkbox" bind:checked={charge.separatePayment} />
              {m.investments_trade_separate_payment()}
            </label>
          {/if}
          {#if charge.treatment === 'separately_expensed' || (charge.commodityID !== '' && Number(charge.commodityID) !== cashCommodityID)}
            <label class="block text-xs text-muted">{m.investments_trade_charge_account()}
              <select bind:value={charge.chargeAccountID} class="mt-1 w-full rounded-(--radius-control) border border-border bg-control px-2 py-2 text-sm text-foreground">
                <option value="">{m.investments_form_select_account()}</option>
                {#each accounts.filter((account) => account.status === 'active' && account.allows_postings && account.account_class === (charge.kind === 'rebate' ? 'income' : 'expense')) as account (account.id)}
                  <option value={String(account.id)}>{account.name}</option>
                {/each}
              </select>
            </label>
          {/if}
          {#if charge.separatePayment || (charge.commodityID !== '' && Number(charge.commodityID) !== cashCommodityID)}
            <label class="block text-xs text-muted">{m.investments_trade_foreign_cash_account()}
              <select bind:value={charge.cashAccountID} class="mt-1 w-full rounded-(--radius-control) border border-border bg-control px-2 py-2 text-sm text-foreground">
                <option value="">{m.investments_form_select_account()}</option>
                {#each cashAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
              </select>
            </label>
            <label class="block text-xs text-muted">{m.investments_trade_charge_paid_on()}
              <input type="date" bind:value={charge.paidOn} class="mt-1 w-full rounded-(--radius-control) border border-border bg-control px-2 py-2 text-sm text-foreground" />
            </label>
          {/if}
          <button type="button" onclick={() => removeCharge(index)} class="text-xs text-muted underline hover:text-foreground">{m.investments_trade_remove_charge()}</button>
        </fieldset>
      {/each}
      <button type="button" onclick={() => (charges = [...charges, newTradeCharge(cashCommodityID)])}
        class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground hover:bg-control-hover">
        {m.investments_trade_add_charge()}
      </button>
    {/if}
  </div>
</details>
