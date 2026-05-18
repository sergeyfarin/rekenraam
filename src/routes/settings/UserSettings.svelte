<script lang="ts">
  import { onMount } from "svelte";
  import {
    createAdminInvite,
    createAdminUser,
    listAdminUsers,
    resetAdminUserMfa,
    resetAdminUserPassword,
    revokeAdminUserSessions,
    setAdminUserMembership,
    updateAdminUser,
    type AdminUser,
  } from "$lib/api/admin";
  import { listBooks, type BookSummary } from "$lib/api/books";
  import * as Card from "$lib/components/ui/card";
  import * as Alert from "$lib/components/ui/alert";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { formatApiError } from "$lib/utils";

  let users: AdminUser[] = [];
  let books: BookSummary[] = [];
  let loading = false;
  let error = "";
  let status = "";
  let newEmail = "";
  let newName = "";
  let newPassword = "";
  let newIsAdmin = false;
  let newBookId = 1;
  let newRole: "owner" | "editor" | "viewer" = "viewer";
  let inviteToken = "";
  let inviteExpiresAt = "";
  let passwordResetUserId: number | null = null;
  let passwordResetValue = "";

  onMount(load);

  async function load() {
    loading = true;
    error = "";
    try {
      [users, books] = await Promise.all([listAdminUsers(), listBooks()]);
      newBookId = books[0]?.id ?? 1;
    } catch (e) {
      error = formatApiError(e);
    } finally {
      loading = false;
    }
  }

  async function createUser() {
    error = "";
    status = "";
    try {
      await createAdminUser({
        email: newEmail,
        display_name: newName,
        password: newPassword,
        is_admin: newIsAdmin,
        memberships: [[newBookId, newRole]],
      });
      newEmail = "";
      newName = "";
      newPassword = "";
      newIsAdmin = false;
      status = "User created.";
      await load();
    } catch (e) {
      error = formatApiError(e);
    }
  }

  async function inviteUser() {
    error = "";
    status = "";
    inviteToken = "";
    inviteExpiresAt = "";
    try {
      const invite = await createAdminInvite({
        email: newEmail,
        display_name: newName,
        is_admin: newIsAdmin,
        memberships: [[newBookId, newRole]],
      });
      newEmail = "";
      newName = "";
      newPassword = "";
      newIsAdmin = false;
      inviteToken = invite.token;
      inviteExpiresAt = invite.expires_at;
      status = "Invite created.";
      await load();
    } catch (e) {
      error = formatApiError(e);
    }
  }

  async function toggleActive(user: AdminUser) {
    try {
      await updateAdminUser(user.id, { is_active: !user.is_active });
      await load();
    } catch (e) {
      error = formatApiError(e);
    }
  }

  async function toggleAdmin(user: AdminUser) {
    try {
      await updateAdminUser(user.id, { is_admin: !user.is_admin });
      await load();
    } catch (e) {
      error = formatApiError(e);
    }
  }

  async function toggleMfaRequired(user: AdminUser) {
    try {
      await updateAdminUser(user.id, { mfa_required: !user.mfa_required });
      await load();
    } catch (e) {
      error = formatApiError(e);
    }
  }

  function beginPasswordReset(user: AdminUser) {
    passwordResetUserId = user.id;
    passwordResetValue = "";
  }

  async function resetPassword(user: AdminUser) {
    error = "";
    status = "";
    try {
      await resetAdminUserPassword(user.id, passwordResetValue);
      status = "Password reset and sessions revoked.";
      passwordResetUserId = null;
      passwordResetValue = "";
      await load();
    } catch (e) {
      error = formatApiError(e);
    }
  }

  async function revokeSessions(user: AdminUser) {
    try {
      const count = await revokeAdminUserSessions(user.id);
      status = `Revoked ${count} session${count === 1 ? "" : "s"}.`;
    } catch (e) {
      error = formatApiError(e);
    }
  }

  async function resetMfa(user: AdminUser) {
    try {
      await resetAdminUserMfa(user.id);
      status = `MFA reset for ${user.email}.`;
      await load();
    } catch (e) {
      error = formatApiError(e);
    }
  }

  async function setRole(user: AdminUser, bookId: number, role: "owner" | "editor" | "viewer") {
    try {
      await setAdminUserMembership(user.id, { book_id: bookId, role });
      await load();
    } catch (e) {
      error = formatApiError(e);
    }
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Users</Card.Title>
    <Card.Description>Manage local users and book roles.</Card.Description>
  </Card.Header>
  <Card.Content class="space-y-4">
    {#if error}
      <Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>
    {/if}
    {#if status}
      <Alert.Root><Alert.Description>{status}</Alert.Description></Alert.Root>
    {/if}
    {#if inviteToken}
      <Alert.Root>
        <Alert.Description>
          Invite token expires {new Date(inviteExpiresAt).toLocaleString()}:
          <span class="ml-1 break-all font-mono">{inviteToken}</span>
        </Alert.Description>
      </Alert.Root>
    {/if}

    <div class="grid gap-3 md:grid-cols-6">
      <div class="space-y-2 md:col-span-2">
        <Label for="new-user-email">Email</Label>
        <Input id="new-user-email" bind:value={newEmail} />
      </div>
      <div class="space-y-2">
        <Label for="new-user-name">Name</Label>
        <Input id="new-user-name" bind:value={newName} />
      </div>
      <div class="space-y-2">
        <Label for="new-user-password">Temporary password</Label>
        <Input id="new-user-password" type="password" bind:value={newPassword} />
      </div>
      <div class="space-y-2">
        <Label for="new-user-role">Role</Label>
        <select id="new-user-role" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={newRole}>
          <option value="viewer">Viewer</option>
          <option value="editor">Editor</option>
          <option value="owner">Owner</option>
        </select>
      </div>
      <div class="flex items-end gap-2">
        <label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={newIsAdmin} /> Admin</label>
        <Button onclick={inviteUser} disabled={!newEmail || !newName}>Invite</Button>
        <Button variant="outline" onclick={createUser} disabled={!newEmail || !newName || newPassword.length < 12}>Create</Button>
      </div>
    </div>

    <div class="overflow-x-auto">
      <table class="w-full text-sm">
        <thead>
          <tr class="border-b text-left">
            <th class="py-2">User</th>
            <th>State</th>
            <th>Book role</th>
            <th class="text-right">Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each users as user}
            <tr class="border-b">
              <td class="py-2">
                <div class="font-medium">{user.display_name}</div>
                <div class="text-xs text-muted-foreground">{user.email}</div>
              </td>
              <td>{user.is_active ? "active" : "deactivated"}{user.is_admin ? " · admin" : ""}{user.mfa_required ? " · MFA required" : ""}</td>
              <td>
                {#each books as book}
                  <select
                    class="h-8 rounded-md border border-input bg-transparent px-2 text-xs"
                    value={user.memberships.find((m) => m.book_id === book.id)?.role ?? ""}
                    onchange={(event) => setRole(user, book.id, event.currentTarget.value as "owner" | "editor" | "viewer")}
                  >
                    <option value="viewer">{book.name}: viewer</option>
                    <option value="editor">{book.name}: editor</option>
                    <option value="owner">{book.name}: owner</option>
                  </select>
                {/each}
              </td>
              <td class="text-right">
                <Button variant="ghost" size="sm" onclick={() => toggleAdmin(user)}>{user.is_admin ? "Remove admin" : "Make admin"}</Button>
                <Button variant="ghost" size="sm" onclick={() => toggleMfaRequired(user)}>{user.mfa_required ? "MFA optional" : "Require MFA"}</Button>
                <Button variant="ghost" size="sm" onclick={() => toggleActive(user)}>{user.is_active ? "Deactivate" : "Reactivate"}</Button>
                <Button variant="ghost" size="sm" onclick={() => beginPasswordReset(user)}>Password</Button>
                <Button variant="ghost" size="sm" onclick={() => resetMfa(user)}>Reset MFA</Button>
                <Button variant="ghost" size="sm" onclick={() => revokeSessions(user)}>Revoke sessions</Button>
                {#if passwordResetUserId === user.id}
                  <div class="mt-2 flex justify-end gap-2">
                    <Input
                      aria-label={`New password for ${user.email}`}
                      type="password"
                      class="max-w-56"
                      bind:value={passwordResetValue}
                    />
                    <Button size="sm" disabled={passwordResetValue.length < 12} onclick={() => resetPassword(user)}>Save</Button>
                    <Button size="sm" variant="outline" onclick={() => { passwordResetUserId = null; passwordResetValue = ""; }}>Cancel</Button>
                  </div>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if loading}
      <p class="text-sm text-muted-foreground">Loading</p>
    {/if}
  </Card.Content>
</Card.Root>
