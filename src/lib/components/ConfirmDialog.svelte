<script lang="ts">
  import * as Dialog from "$lib/components/ui/dialog";
  import { Button } from "$lib/components/ui/button";
  import type { Snippet } from "svelte";

  let {
    open = $bindable(false),
    title = "Are you sure?",
    message = "",
    confirmLabel = "Confirm",
    cancelLabel = "Cancel",
    destructive = false,
    onConfirm,
    onCancel,
    children,
  }: {
    open?: boolean;
    title?: string;
    message?: string;
    confirmLabel?: string;
    cancelLabel?: string;
    destructive?: boolean;
    onConfirm: () => void;
    onCancel: () => void;
    children?: Snippet;
  } = $props();
</script>

<Dialog.Root bind:open>
  <Dialog.Content showCloseButton={false} class="max-w-md">
    <Dialog.Header>
      <Dialog.Title>{title}</Dialog.Title>
      {#if message}
        <Dialog.Description>{message}</Dialog.Description>
      {/if}
    </Dialog.Header>
    {#if children}
      <div class="py-2">
        {@render children()}
      </div>
    {/if}
    <Dialog.Footer>
      <Button variant="outline" onclick={onCancel}>{cancelLabel}</Button>
      <Button variant={destructive ? "destructive" : "default"} onclick={onConfirm}>
        {confirmLabel}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
