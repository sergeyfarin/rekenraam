<script lang="ts">
  import { onMount } from "svelte";
  import { getPreferences, updatePreferences, type UserPreferences } from "$lib/api/preferences";
  import {
    beginMfaSetup,
    changePassword,
    confirmMfaSetup,
    disableMfa,
    getCurrentUser,
    getMfaStatus,
    updateProfile,
    type AuthMe,
    type MfaStatus
  } from "$lib/api/auth";
  import { listBooks, type BookSummary } from "$lib/api/books";
  import * as Card from "$lib/components/ui/card";
  import * as Alert from "$lib/components/ui/alert";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { formatApiError } from "$lib/utils";

  let preferences: UserPreferences = { default_book_id: null, locale: "", date_format: "iso", number_format: "system", theme: "system" };
  let books: BookSummary[] = [];
  let error = "";
  let status = "";
  let authUser: AuthMe | null = null;
  let displayName = "";
  let currentProfilePassword = "";
  let newProfilePassword = "";
  let confirmProfilePassword = "";
  let mfaStatus: MfaStatus | null = null;
  let currentPassword = "";
  let disablePassword = "";
  let disableCode = "";
  let setupCode = "";
  let setupKey = "";
  let otpauthUri = "";
  let recoveryCodes: string[] = [];

  onMount(load);

  async function load() {
    try {
      [preferences, books, mfaStatus, authUser] = await Promise.all([getPreferences(), listBooks(), getMfaStatus(), getCurrentUser()]);
      displayName = authUser.user.display_name;
    } catch (e) {
      error = formatApiError(e);
    }
  }

  async function startMfaSetup() {
    error = "";
    status = "";
    recoveryCodes = [];
    try {
      const setup = await beginMfaSetup(currentPassword);
      setupKey = setup.setup_key;
      otpauthUri = setup.otpauth_uri;
      currentPassword = "";
    } catch (e) {
      error = formatApiError(e);
    }
  }

  async function confirmMfa() {
    error = "";
    status = "";
    try {
      const result = await confirmMfaSetup(setupCode);
      recoveryCodes = result.recovery_codes;
      setupCode = "";
      setupKey = "";
      otpauthUri = "";
      status = "MFA enabled. Store the recovery codes now; they will not be shown again.";
      mfaStatus = await getMfaStatus();
    } catch (e) {
      error = formatApiError(e);
    }
  }

  async function turnOffMfa() {
    error = "";
    status = "";
    try {
      await disableMfa({ current_password: disablePassword, code: disableCode });
      disablePassword = "";
      disableCode = "";
      status = "MFA disabled.";
      mfaStatus = await getMfaStatus();
    } catch (e) {
      error = formatApiError(e);
    }
  }

  async function save() {
    error = "";
    status = "";
    try {
      preferences = await updatePreferences(preferences);
      status = "Preferences saved.";
    } catch (e) {
      error = formatApiError(e);
    }
  }

  async function saveProfile() {
    error = "";
    status = "";
    try {
      authUser = await updateProfile({ display_name: displayName.trim() });
      displayName = authUser.user.display_name;
      status = "Profile saved.";
    } catch (e) {
      error = formatApiError(e);
    }
  }

  async function savePassword() {
    error = "";
    status = "";
    if (newProfilePassword !== confirmProfilePassword) {
      error = "Passwords do not match.";
      return;
    }
    try {
      await changePassword({
        current_password: currentProfilePassword,
        new_password: newProfilePassword,
      });
      currentProfilePassword = "";
      newProfilePassword = "";
      confirmProfilePassword = "";
      status = "Password changed.";
    } catch (e) {
      error = formatApiError(e);
    }
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Preferences</Card.Title>
    <Card.Description>Set display defaults for your user.</Card.Description>
  </Card.Header>
  <Card.Content class="max-w-xl space-y-4">
    {#if error}<Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
    {#if status}<Alert.Root><Alert.Description>{status}</Alert.Description></Alert.Root>{/if}
    <div class="space-y-2">
      <Label for="pref-book">Default book</Label>
      <select id="pref-book" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={preferences.default_book_id}>
        <option value={null}>First readable book</option>
        {#each books as book}
          <option value={book.id}>{book.name}</option>
        {/each}
      </select>
    </div>
    <div class="space-y-2">
      <Label for="pref-locale">Locale</Label>
      <Input id="pref-locale" bind:value={preferences.locale} placeholder="en-US" />
    </div>
    <div class="grid gap-3 md:grid-cols-3">
      <div class="space-y-2">
        <Label for="pref-date">Date</Label>
        <Input id="pref-date" bind:value={preferences.date_format} />
      </div>
      <div class="space-y-2">
        <Label for="pref-number">Number</Label>
        <Input id="pref-number" bind:value={preferences.number_format} />
      </div>
      <div class="space-y-2">
        <Label for="pref-theme">Theme</Label>
        <Input id="pref-theme" bind:value={preferences.theme} />
      </div>
    </div>
    <Button onclick={save}>Save</Button>
  </Card.Content>
</Card.Root>

<Card.Root>
  <Card.Header>
    <Card.Title>Profile</Card.Title>
    <Card.Description>Update your display name and password.</Card.Description>
  </Card.Header>
  <Card.Content class="max-w-xl space-y-4">
    <div class="space-y-2">
      <Label for="profile-email">Email</Label>
      <Input id="profile-email" value={authUser?.user.email ?? ""} disabled />
    </div>
    <div class="space-y-2">
      <Label for="profile-name">Display name</Label>
      <Input id="profile-name" bind:value={displayName} autocomplete="name" />
    </div>
    <Button onclick={saveProfile} disabled={!displayName.trim()}>Save Profile</Button>

    <div class="grid gap-3 md:grid-cols-3">
      <div class="space-y-2">
        <Label for="profile-current-password">Current password</Label>
        <Input id="profile-current-password" type="password" bind:value={currentProfilePassword} autocomplete="current-password" />
      </div>
      <div class="space-y-2">
        <Label for="profile-new-password">New password</Label>
        <Input id="profile-new-password" type="password" bind:value={newProfilePassword} autocomplete="new-password" />
      </div>
      <div class="space-y-2">
        <Label for="profile-confirm-password">Confirm password</Label>
        <Input id="profile-confirm-password" type="password" bind:value={confirmProfilePassword} autocomplete="new-password" />
      </div>
    </div>
    <Button
      variant="secondary"
      onclick={savePassword}
      disabled={!currentProfilePassword || newProfilePassword.length < 12 || confirmProfilePassword.length < 12}
    >
      Change Password
    </Button>
  </Card.Content>
</Card.Root>

<Card.Root>
  <Card.Header>
    <Card.Title>Two-factor authentication</Card.Title>
    <Card.Description>Use a TOTP authenticator app for stronger sign-in protection.</Card.Description>
  </Card.Header>
  <Card.Content class="max-w-xl space-y-4">
    <p class="text-sm text-muted-foreground">
      Status: {mfaStatus?.enabled ? "enabled" : "not enabled"}{mfaStatus?.enforced ? " · enforced for this deployment" : ""}
    </p>
    {#if mfaStatus?.enabled}
      <p class="text-sm text-muted-foreground">Unused recovery codes: {mfaStatus.recovery_codes_remaining}</p>
      <div class="grid gap-3 md:grid-cols-2">
        <div class="space-y-2">
          <Label for="mfa-disable-password">Current password</Label>
          <Input id="mfa-disable-password" type="password" bind:value={disablePassword} autocomplete="current-password" />
        </div>
        <div class="space-y-2">
          <Label for="mfa-disable-code">Authenticator or recovery code</Label>
          <Input id="mfa-disable-code" bind:value={disableCode} autocomplete="one-time-code" />
        </div>
      </div>
      <Button variant="secondary" onclick={turnOffMfa} disabled={!disablePassword || disableCode.length < 6}>Disable MFA</Button>
    {:else}
      <div class="space-y-2">
        <Label for="mfa-password">Current password</Label>
        <Input id="mfa-password" type="password" bind:value={currentPassword} autocomplete="current-password" />
      </div>
      <Button variant="secondary" onclick={startMfaSetup} disabled={!currentPassword}>Start setup</Button>
    {/if}

    {#if setupKey}
      <div class="space-y-2">
        <Label>Setup key</Label>
        <p class="break-all rounded-md border border-border p-2 font-mono text-xs">{setupKey}</p>
        <p class="break-all text-xs text-muted-foreground">{otpauthUri}</p>
      </div>
      <div class="space-y-2">
        <Label for="mfa-code">Authenticator code</Label>
        <Input id="mfa-code" bind:value={setupCode} autocomplete="one-time-code" />
      </div>
      <Button onclick={confirmMfa} disabled={setupCode.length < 6}>Confirm MFA</Button>
    {/if}

    {#if recoveryCodes.length > 0}
      <div class="rounded-md border border-border p-3">
        <p class="mb-2 text-sm font-medium">Recovery codes</p>
        <div class="grid gap-1 font-mono text-sm md:grid-cols-2">
          {#each recoveryCodes as code}
            <span>{code}</span>
          {/each}
        </div>
      </div>
    {/if}
  </Card.Content>
</Card.Root>
