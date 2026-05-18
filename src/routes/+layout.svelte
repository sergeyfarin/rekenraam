<script lang="ts">
  import "../app.css";
  import { onMount } from "svelte";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import * as Dialog from "$lib/components/ui/dialog";
  import {
    completeMfaLogin,
    createFirstAdmin,
    getBootstrapStatus,
    getCurrentUser,
    login,
    logout,
    type AuthMe
  } from "$lib/api/auth";
  import { initializeBookContext } from "$lib/book-context";
  import { formatApiError } from "$lib/utils";

  type Tab = { label: string; href: string };

  const tabs = [
    { label: "Home", href: "/" },
    { label: "Transactions", href: "/transactions" },
    { label: "Accounts", href: "/accounts" },
    { label: "Investments", href: "/investments" },
    { label: "Reports", href: "/reports" },
    { label: "Import/Export", href: "/import-export" },
    { label: "Planning", href: "/planning" },
    { label: "Tax", href: "/tax" },
    { label: "Settings", href: "/settings" },
    { label: "About", href: "/about" }
  ] satisfies Tab[];

  $: currentPath = page.url.pathname;

  let helpOpen = false;
  let authLoading = true;
  let bootstrapRequired = false;
  let authUser: AuthMe | null = null;
  let authError = "";
  let authSubmitting = false;
  let displayName = "";
  let email = "";
  let password = "";
  let mfaCode = "";
  let mfaChallengeToken = "";

  onMount(() => {
    void refreshAuth();
  });

  async function refreshAuth() {
    authLoading = true;
    authError = "";
    try {
      authUser = await getCurrentUser();
      bootstrapRequired = false;
      await initializeBookContext();
    } catch {
      authUser = null;
      try {
        const status = await getBootstrapStatus();
        bootstrapRequired = status.bootstrap_required;
      } catch (error) {
        authError = formatApiError(error);
      }
    } finally {
      authLoading = false;
    }
  }

  async function submitAuth() {
    authSubmitting = true;
    authError = "";
    try {
      authUser = bootstrapRequired
        ? await createFirstAdmin({ email, password, display_name: displayName })
        : null;
      if (!bootstrapRequired) {
        const result = await login({ email, password });
        if (result.mfa_required && result.challenge_token) {
          mfaChallengeToken = result.challenge_token;
          password = "";
          return;
        }
        if (result.user && result.session) {
          authUser = { user: result.user, session: result.session };
        }
      }
      bootstrapRequired = false;
      password = "";
      await initializeBookContext();
    } catch (error) {
      authError = formatApiError(error);
    } finally {
      authSubmitting = false;
    }
  }

  async function submitMfa() {
    authSubmitting = true;
    authError = "";
    try {
      authUser = await completeMfaLogin({ challenge_token: mfaChallengeToken, code: mfaCode });
      mfaChallengeToken = "";
      mfaCode = "";
      bootstrapRequired = false;
    } catch (error) {
      authError = formatApiError(error);
    } finally {
      authSubmitting = false;
    }
  }

  async function handleLogout() {
    authError = "";
    try {
      await logout();
    } finally {
      authUser = null;
      await refreshAuth();
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    // Ignore shortcuts when user is typing in an input, textarea, select, or contenteditable
    const target = e.target as HTMLElement;
    if (
      target.tagName === "INPUT" ||
      target.tagName === "TEXTAREA" ||
      target.tagName === "SELECT" ||
      target.isContentEditable
    ) return;

    // Ignore if a dialog is open (avoid double-handling)
    if (document.querySelector('[data-slot="dialog-content"]')) return;

    // ? — help
    if (e.key === "?" && !e.ctrlKey && !e.metaKey) {
      helpOpen = true;
      return;
    }

    // N — new transaction (navigate to transactions page and open dialog)
    if (e.key === "n" && !e.ctrlKey && !e.metaKey && !e.shiftKey) {
      goto("/transactions?new=1");
      return;
    }

  }
</script>

<svelte:window on:keydown={handleKeydown} />

{#if authLoading}
  <main class="grid min-h-screen place-items-center bg-background px-6 text-sm text-muted-foreground">
    Loading
  </main>
{:else if !authUser}
  <main class="grid min-h-screen place-items-center bg-background px-6">
    <form class="w-full max-w-sm space-y-4" on:submit|preventDefault={mfaChallengeToken ? submitMfa : submitAuth}>
      <div class="space-y-1">
        <h1 class="text-2xl font-semibold">{mfaChallengeToken ? "Two-factor code" : bootstrapRequired ? "Create Owner" : "Sign In"}</h1>
        <p class="text-sm text-muted-foreground">
          {bootstrapRequired
            ? "Trusted local network setup only. Use HTTPS and seeded setup for public deployments."
            : "Rekenraam"}
        </p>
      </div>
      {#if mfaChallengeToken}
        <Input bind:value={mfaCode} placeholder="Authenticator or recovery code" autocomplete="one-time-code" required />
      {:else}
      {#if bootstrapRequired}
        <Input bind:value={displayName} placeholder="Display name" autocomplete="name" required />
      {/if}
      <Input bind:value={email} placeholder="Email" autocomplete="email" type="email" required />
      <Input
        bind:value={password}
        placeholder="Password"
        autocomplete={bootstrapRequired ? "new-password" : "current-password"}
        type="password"
        required
        minlength={bootstrapRequired ? 12 : 1}
      />
      {/if}
      {#if authError}
        <p class="text-sm text-destructive">{authError}</p>
      {/if}
      <Button type="submit" class="w-full" disabled={authSubmitting}>
        {authSubmitting ? "Please wait" : mfaChallengeToken ? "Verify" : bootstrapRequired ? "Create Owner" : "Sign In"}
      </Button>
      {#if !bootstrapRequired && !mfaChallengeToken}
        <div class="flex justify-between text-sm">
          <a class="text-primary hover:underline" href="/reset-password">Forgot password</a>
          <a class="text-primary hover:underline" href="/accept-invite">Accept invite</a>
        </div>
      {/if}
    </form>
  </main>
{:else}
<header class="sticky top-0 z-10 border-b border-border bg-background">
  <div class="container mx-auto flex h-16 items-center justify-between gap-4 px-6">
    <div class="font-semibold text-lg">Rekenraam 🪙</div>
    <nav class="flex flex-wrap gap-1" aria-label="Primary">
      {#each tabs as tab}
        <a
          href={tab.href}
          class="px-3 py-1.5 rounded-md font-medium text-sm transition-colors {currentPath === tab.href
            ? 'bg-accent text-foreground'
            : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'}"
        >
          {tab.label}
        </a>
      {/each}
    </nav>
    <div class="flex items-center gap-2 text-sm">
      <span class="max-w-40 truncate text-muted-foreground">{authUser.user.display_name}</span>
      <Button variant="outline" size="sm" onclick={handleLogout}>Logout</Button>
    </div>
  </div>
</header>

<slot />
{/if}

<!-- Help / keyboard shortcut modal -->
<Dialog.Root bind:open={helpOpen}>
  <Dialog.Content class="max-w-sm">
    <Dialog.Header>
      <Dialog.Title>Keyboard Shortcuts</Dialog.Title>
    </Dialog.Header>
    <div class="space-y-2 py-2 text-sm">
      <div class="grid grid-cols-2 gap-x-4 gap-y-1">
        <kbd class="font-mono bg-muted rounded px-1">N</kbd>
        <span>New transaction</span>
        <kbd class="font-mono bg-muted rounded px-1">?</kbd>
        <span>Show this help</span>
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="outline" onclick={() => helpOpen = false}>Close</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
