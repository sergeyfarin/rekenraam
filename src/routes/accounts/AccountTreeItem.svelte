<script lang="ts">
  export let node: {
    id: number;
    name: string;
    account_type: string;
    commodity_name: string;
    commodity_scale: number;
    institution_name?: string | null;
    country_name?: string | null;
    balance_minor: number;
    rollup_balance_minor: number;
    children: any[];
  };

  export let formatMinorWithScale: (amountMinor: number, scale: number) => string;
</script>

<li class="tree-item">
  <div class="tree-row">
    <div class="tree-title">
      <a class="bx--link" href={`/accounts/${node.id}`}>{node.name}</a>
      <span class="tree-meta">
        {node.account_type}
        {#if node.institution_name}
          · {node.institution_name}
        {/if}
        {#if node.country_name}
          · {node.country_name}
        {/if}
      </span>
    </div>
    <div class="tree-balances">
      <span class="tree-balance">
        {formatMinorWithScale(node.balance_minor, node.commodity_scale)} {node.commodity_name}
      </span>
      <span class="tree-rollup">
        Rollup: {formatMinorWithScale(node.rollup_balance_minor, node.commodity_scale)} {node.commodity_name}
      </span>
    </div>
  </div>
  {#if node.children && node.children.length > 0}
    <ul class="tree">
      {#each node.children as child}
        <svelte:self node={child} {formatMinorWithScale} />
      {/each}
    </ul>
  {/if}
</li>
