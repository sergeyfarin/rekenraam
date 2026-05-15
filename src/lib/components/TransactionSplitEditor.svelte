<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import type { CategorySummary, CommoditySummary, PayeeSummary, PersonSummary, ProjectSummary, TagSummary } from "$lib/api/metadata";
  import { exactMatchByName, fuzzyOptions } from "$lib/search/fuzzy";
  import { formatMinorWithScale } from "$lib/money";
  import { sumSplitsInMinor } from "$lib/transactions/split-balance";
  import { emptySplitDraft, type SplitDraft } from "$lib/transactions/split-draft";

  type AccountLookup = { id: number; name: string; commodity_id: number };

  let {
    open = $bindable(false),
    splits = $bindable<SplitDraft[]>([]),
    accounts,
    categories,
    tags,
    people,
    projects,
    commodities,
  }: {
    open?: boolean;
    splits?: SplitDraft[];
    accounts: AccountLookup[];
    categories: CategorySummary[];
    tags: TagSummary[];
    people: PersonSummary[];
    projects: ProjectSummary[];
    commodities: CommoditySummary[];
  } = $props();

  // Sum splits to detect imbalance. Returns null if any split is missing an account
  // (resolution prerequisite for scale lookup).
  function splitsTotalMinor(): number | null {
    const resolved: { amount: string; scale: number }[] = [];
    for (const split of splits) {
      if (!split.account_id) return null;
      const account = accounts.find((a) => a.id === split.account_id);
      if (!account) return null;
      const commodity = commodities.find((c) => c.id === account.commodity_id);
      if (!commodity) return null;
      resolved.push({ amount: split.amount, scale: commodity.scale });
    }
    return sumSplitsInMinor(resolved);
  }

  function syncSplitInput(
    split: SplitDraft,
    field: "category" | "tag" | "person" | "project",
    value: string,
  ) {
    if (field === "category") {
      split.category_input = value;
      split.category_id = exactMatchByName(categories, value)?.id ?? null;
    } else if (field === "tag") {
      split.tag_input = value;
      split.tag_id = exactMatchByName(tags, value)?.id ?? null;
    } else if (field === "person") {
      split.person_input = value;
      split.person_id = exactMatchByName(people, value)?.id ?? null;
    } else {
      split.project_input = value;
      split.project_id = exactMatchByName(projects, value)?.id ?? null;
    }
    splits = [...splits];
  }

  function addSplitRow() {
    splits = [...splits, emptySplitDraft()];
  }

  function removeSplitRow(index: number) {
    if (splits.length <= 2) return;
    splits = splits.filter((_, idx) => idx !== index);
  }

  const total = $derived(splits.length > 0 ? splitsTotalMinor() : null);
  const balanced = $derived(total === 0);
  const firstAccount = $derived(accounts.find((a) => a.id === splits[0]?.account_id));
  const firstCommodity = $derived(commodities.find((c) => c.id === firstAccount?.commodity_id));
  const balanceScale = $derived(firstCommodity?.scale ?? 2);
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="max-w-4xl">
    <Dialog.Header>
      <Dialog.Title>Split transaction</Dialog.Title>
    </Dialog.Header>
    <div class="space-y-4 py-4">
      <div class="space-y-2">
        <Label>Splits</Label>
        <Table.Root>
          <Table.Header>
            <Table.Row>
              <Table.Head>Account</Table.Head>
              <Table.Head>Category</Table.Head>
              <Table.Head>Tag</Table.Head>
              <Table.Head>Person</Table.Head>
              <Table.Head>Project</Table.Head>
              <Table.Head>Share (bps)</Table.Head>
              <Table.Head class="text-right">Amount</Table.Head>
              <Table.Head>Memo</Table.Head>
              <Table.Head class="w-24">Action</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {#each splits as split, idx}
              <Table.Row>
                <Table.Cell>
                  <select class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={split.account_id}>
                    <option value={null}>Select account</option>
                    {#each accounts as account}
                      <option value={account.id}>{account.name}</option>
                    {/each}
                  </select>
                </Table.Cell>
                <Table.Cell>
                  <Input
                    list={`split-category-options-${idx}`}
                    placeholder="Search/enter category"
                    value={split.category_input}
                    oninput={(event) => syncSplitInput(split, "category", (event.currentTarget as HTMLInputElement).value)}
                  />
                  <datalist id={`split-category-options-${idx}`}>
                    {#each fuzzyOptions(categories, split.category_input) as category}
                      <option value={category.name}></option>
                    {/each}
                  </datalist>
                </Table.Cell>
                <Table.Cell>
                  <Input
                    list={`split-tag-options-${idx}`}
                    placeholder="Search/enter tag"
                    value={split.tag_input}
                    oninput={(event) => syncSplitInput(split, "tag", (event.currentTarget as HTMLInputElement).value)}
                  />
                  <datalist id={`split-tag-options-${idx}`}>
                    {#each fuzzyOptions(tags, split.tag_input) as tag}
                      <option value={tag.name}></option>
                    {/each}
                  </datalist>
                </Table.Cell>
                <Table.Cell>
                  <Input
                    list={`split-person-options-${idx}`}
                    placeholder="Search/enter person"
                    value={split.person_input}
                    oninput={(event) => syncSplitInput(split, "person", (event.currentTarget as HTMLInputElement).value)}
                  />
                  <datalist id={`split-person-options-${idx}`}>
                    {#each fuzzyOptions(people, split.person_input) as person}
                      <option value={person.name}></option>
                    {/each}
                  </datalist>
                </Table.Cell>
                <Table.Cell>
                  <Input
                    list={`split-project-options-${idx}`}
                    placeholder="Search/enter project"
                    value={split.project_input}
                    oninput={(event) => syncSplitInput(split, "project", (event.currentTarget as HTMLInputElement).value)}
                  />
                  <datalist id={`split-project-options-${idx}`}>
                    {#each fuzzyOptions(projects, split.project_input) as project}
                      <option value={project.name}></option>
                    {/each}
                  </datalist>
                </Table.Cell>
                <Table.Cell>
                  <Input type="number" bind:value={split.share_bps} placeholder="e.g. 5000" class="w-28" />
                </Table.Cell>
                <Table.Cell class="text-right">
                  <Input bind:value={split.amount} placeholder="0.00" class="w-28 text-right" />
                </Table.Cell>
                <Table.Cell>
                  <Input bind:value={split.memo} placeholder="Split memo" />
                </Table.Cell>
                <Table.Cell>
                  <Button variant="ghost" size="sm" onclick={() => removeSplitRow(idx)} disabled={splits.length <= 2}>
                    Remove
                  </Button>
                </Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
        <Button variant="outline" size="sm" onclick={addSplitRow}>Add split</Button>
        {#if total !== null}
          <div class="flex items-center gap-2 mt-2">
            <span class="text-sm font-medium">Balance:</span>
            <span class="text-sm font-mono {balanced ? 'text-money-positive' : 'text-money-negative'}">
              {#if total === 0}
                ✓ Balanced
              {:else}
                {total > 0 ? "+" : ""}{formatMinorWithScale(total, balanceScale)} {firstCommodity?.symbol ?? ""} (unbalanced)
              {/if}
            </span>
          </div>
        {/if}
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={() => (open = false)}>Done</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
