<script lang="ts">
  import { createCategory, deleteCategory as deleteCategoryCommand, listCategories, type CategorySummary, updateCategory } from "$lib/api/metadata";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import { Badge } from "$lib/components/ui/badge";

  export let categories: CategorySummary[] = [];
  export let busy = false;

  let categoryError = "";
  let categoryStatus = "";
  let editingCategory: CategorySummary | null = null;
  let newCategory = { name: "", kind: "expense", color: "", parent_id: null as number | null };
  let showCategoryDialog = false;

  export async function loadCategories() {
    try {
      categories = await listCategories(1);
    } catch (e) {
      categoryError = `Failed to load categories: ${String(e)}`;
    }
  }

  function openNewCategory() {
    editingCategory = null;
    newCategory = { name: "", kind: "expense", color: "", parent_id: null };
    showCategoryDialog = true;
  }

  function openEditCategory(cat: CategorySummary) {
    editingCategory = cat;
    newCategory = { name: cat.name, kind: cat.kind, color: cat.color || "", parent_id: cat.parent_id };
    showCategoryDialog = true;
  }

  function closeCategoryDialog() {
    showCategoryDialog = false;
    editingCategory = null;
  }

  async function saveCategory() {
    categoryError = "";
    categoryStatus = "";
    busy = true;
    try {
      if (editingCategory) {
        await updateCategory({
          id: editingCategory.id,
          book_id: 1,
          parent_id: newCategory.parent_id || null,
          name: newCategory.name,
          kind: newCategory.kind,
          color: newCategory.color || null,
        });
        categoryStatus = "Category updated.";
      } else {
        await createCategory({
          book_id: 1,
          parent_id: newCategory.parent_id || null,
          name: newCategory.name,
          kind: newCategory.kind,
          color: newCategory.color || null,
        });
        categoryStatus = "Category created.";
      }
      closeCategoryDialog();
      await loadCategories();
    } catch (e) {
      categoryError = `Failed to save category: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function deleteCategory(cat: CategorySummary) {
    if (!confirm(`Delete category "${cat.name}"?`)) return;
    categoryError = "";
    categoryStatus = "";
    busy = true;
    try {
      await deleteCategoryCommand(cat.id, 1);
      categoryStatus = "Category deleted.";
      await loadCategories();
    } catch (e) {
      categoryError = `Failed to delete category: ${String(e)}`;
    } finally {
      busy = false;
    }
  }
</script>

<Card.Root>
  <Card.Header class="flex flex-row items-center justify-between">
    <Card.Title>Categories</Card.Title>
    <Button onclick={openNewCategory} disabled={busy}>
      Add Category
    </Button>
  </Card.Header>
  <Card.Content>
    {#if categoryStatus}
      <p class="text-sm text-green-600">{categoryStatus}</p>
    {/if}
    {#if categoryError}
      <p class="text-sm text-destructive">{categoryError}</p>
    {/if}

    <Table.Root>
      <Table.Header>
        <Table.Row>
          <Table.Head>Name</Table.Head>
          <Table.Head>Kind</Table.Head>
          <Table.Head>Color</Table.Head>
          <Table.Head class="text-right">Actions</Table.Head>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {#each categories as cat}
          <Table.Row>
            <Table.Cell>{cat.name}</Table.Cell>
            <Table.Cell>
              <Badge variant={cat.kind === "expense" ? "destructive" : cat.kind === "income" ? "default" : "secondary"}>{cat.kind}</Badge>
            </Table.Cell>
            <Table.Cell>
              {#if cat.color}
                <span class="inline-block w-6 h-6 rounded border" style="background-color: {cat.color}"></span>
              {:else}
                —
              {/if}
            </Table.Cell>
            <Table.Cell class="text-right">
              <Button variant="ghost" size="sm" onclick={() => openEditCategory(cat)}>Edit</Button>
              <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deleteCategory(cat)}>Delete</Button>
            </Table.Cell>
          </Table.Row>
        {:else}
          <Table.Row>
            <Table.Cell colspan={4} class="text-muted-foreground">No categories found.</Table.Cell>
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
  </Card.Content>
</Card.Root>

<!-- Category Dialog -->
<Dialog.Root bind:open={showCategoryDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>{editingCategory ? "Edit Category" : "New Category"}</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="category-name">Name</Label>
        <Input id="category-name" type="text" bind:value={newCategory.name} placeholder="Category name" />
      </div>
      <div class="grid gap-2">
        <Label for="category-kind">Kind</Label>
        <select id="category-kind" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newCategory.kind}>
          <option value="expense">Expense</option>
          <option value="income">Income</option>
          <option value="transfer">Transfer</option>
          <option value="other">Other</option>
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="category-parent">Parent Category</Label>
        <select id="category-parent" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newCategory.parent_id}>
          <option value={null}>None (Top-level)</option>
          {#each categories.filter((c) => c.id !== editingCategory?.id) as cat}
            <option value={cat.id}>{cat.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="category-color">Color</Label>
        <Input id="category-color" type="color" bind:value={newCategory.color} />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeCategoryDialog}>Cancel</Button>
      <Button onclick={saveCategory} disabled={busy || !newCategory.name}>
        {editingCategory ? "Update" : "Create"}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
