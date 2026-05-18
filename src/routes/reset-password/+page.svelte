<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { confirmPasswordReset, requestPasswordReset } from "$lib/api/auth";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Alert from "$lib/components/ui/alert";
  import { formatApiError } from "$lib/utils";

  let email = "";
  let token = page.url.searchParams.get("token") ?? "";
  let newPassword = "";
  let confirmPassword = "";
  let busy = false;
  let error = "";
  let status = "";

  $: confirmMode = token.trim().length > 0;

  async function submitRequest() {
    error = "";
    status = "";
    busy = true;
    try {
      await requestPasswordReset(email);
      status = "If that account exists, a reset token has been issued. Check the configured delivery channel.";
    } catch (caught) {
      error = formatApiError(caught);
    } finally {
      busy = false;
    }
  }

  async function submitConfirm() {
    error = "";
    status = "";
    if (newPassword !== confirmPassword) {
      error = "Passwords do not match.";
      return;
    }
    busy = true;
    try {
      await confirmPasswordReset({ token: token.trim(), new_password: newPassword });
      status = "Password reset. You can sign in with the new password.";
      newPassword = "";
      confirmPassword = "";
    } catch (caught) {
      error = formatApiError(caught);
    } finally {
      busy = false;
    }
  }
</script>

<main class="grid min-h-screen place-items-center bg-background px-6 py-10">
  <section class="w-full max-w-md space-y-5">
    <div class="space-y-1">
      <h1 class="text-2xl font-semibold">Reset Password</h1>
      <p class="text-sm text-muted-foreground">
        {confirmMode ? "Choose a new password for this account." : "Request a reset token for your account."}
      </p>
    </div>

    {#if error}
      <Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>
    {/if}
    {#if status}
      <Alert.Root><Alert.Description>{status}</Alert.Description></Alert.Root>
    {/if}

    {#if confirmMode}
      <form class="space-y-4" onsubmit={(event) => { event.preventDefault(); void submitConfirm(); }}>
        <div class="space-y-2">
          <Label for="reset-token">Token</Label>
          <Input id="reset-token" bind:value={token} autocomplete="off" required />
        </div>
        <div class="space-y-2">
          <Label for="new-password">New password</Label>
          <Input id="new-password" type="password" bind:value={newPassword} autocomplete="new-password" minlength={12} required />
        </div>
        <div class="space-y-2">
          <Label for="confirm-password">Confirm password</Label>
          <Input id="confirm-password" type="password" bind:value={confirmPassword} autocomplete="new-password" minlength={12} required />
        </div>
        <div class="flex gap-2">
          <Button type="submit" disabled={busy || newPassword.length < 12 || confirmPassword.length < 12}>
            {busy ? "Resetting" : "Reset Password"}
          </Button>
          <Button type="button" variant="outline" onclick={() => goto("/")}>Sign In</Button>
        </div>
      </form>
    {:else}
      <form class="space-y-4" onsubmit={(event) => { event.preventDefault(); void submitRequest(); }}>
        <div class="space-y-2">
          <Label for="reset-email">Email</Label>
          <Input id="reset-email" type="email" bind:value={email} autocomplete="email" required />
        </div>
        <div class="flex gap-2">
          <Button type="submit" disabled={busy || !email.trim()}>
            {busy ? "Sending" : "Request Reset"}
          </Button>
          <Button type="button" variant="outline" onclick={() => goto("/")}>Sign In</Button>
        </div>
      </form>
    {/if}
  </section>
</main>
