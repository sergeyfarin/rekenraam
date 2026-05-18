<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { acceptInvite } from "$lib/api/auth";
  import { initializeBookContext } from "$lib/book-context";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Alert from "$lib/components/ui/alert";
  import { formatApiError } from "$lib/utils";

  let token = page.url.searchParams.get("token") ?? "";
  let password = "";
  let confirmPassword = "";
  let busy = false;
  let error = "";
  let status = "";

  async function submit() {
    error = "";
    status = "";
    if (password !== confirmPassword) {
      error = "Passwords do not match.";
      return;
    }
    busy = true;
    try {
      await acceptInvite({ token: token.trim(), password });
      await initializeBookContext();
      status = "Invite accepted. Redirecting...";
      await goto("/");
    } catch (caught) {
      error = formatApiError(caught);
    } finally {
      busy = false;
    }
  }
</script>

<main class="grid min-h-screen place-items-center bg-background px-6 py-10">
  <form class="w-full max-w-md space-y-5" onsubmit={(event) => { event.preventDefault(); void submit(); }}>
    <div class="space-y-1">
      <h1 class="text-2xl font-semibold">Accept Invite</h1>
      <p class="text-sm text-muted-foreground">Create your password and start using the assigned book.</p>
    </div>

    {#if error}
      <Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>
    {/if}
    {#if status}
      <Alert.Root><Alert.Description>{status}</Alert.Description></Alert.Root>
    {/if}

    <div class="space-y-2">
      <Label for="invite-token">Invite token</Label>
      <Input id="invite-token" bind:value={token} autocomplete="off" required />
    </div>
    <div class="space-y-2">
      <Label for="invite-password">Password</Label>
      <Input id="invite-password" type="password" bind:value={password} autocomplete="new-password" minlength={12} required />
    </div>
    <div class="space-y-2">
      <Label for="invite-confirm-password">Confirm password</Label>
      <Input id="invite-confirm-password" type="password" bind:value={confirmPassword} autocomplete="new-password" minlength={12} required />
    </div>
    <div class="flex gap-2">
      <Button type="submit" disabled={busy || !token.trim() || password.length < 12 || confirmPassword.length < 12}>
        {busy ? "Accepting" : "Accept Invite"}
      </Button>
      <Button type="button" variant="outline" onclick={() => goto("/")}>Sign In</Button>
    </div>
  </form>
</main>
