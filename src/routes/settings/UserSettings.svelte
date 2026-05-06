<script lang="ts">
  import { onMount } from "svelte";
  import {
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

  onMount(load);

  async function load() {
    loading = true;
    error = "";
    try {
      [users, books] = await Promise.all([listAdminUsers(), listBooks()]);
      newBookId = books[0]?.id ?? 1;
    } catch (e) {
      error = String(e);
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
      error = String(e);
    }
  }

  async function toggleActive(user: AdminUser) {
    await updateAdminUser(user.id, { is_active: !user.is_active });
    await load();
  }

  async function toggleAdmin(user: AdminUser) {
    await updateAdminUser(user.id, { is_admin: !user.is_admin });
    await load();
  }

  async function toggleMfaRequired(user: AdminUser) {
    await updateAdminUser(user.id, { mfa_required: !user.mfa_required });
    await load();
  }

  async function resetPassword(user: AdminUser) {
    const password = window.prompt(`New temporary password for ${user.email}`);
    if (!password) return;
    await resetAdminUserPassword(user.id, password);
    status = "Password reset and sessions revoked.";
    await load();
  }

  async function revokeSessions(user: AdminUser) {
    const count = await revokeAdminUserSessions(user.id);
    status = `Revoked ${count} session${count === 1 ? "" : "s"}.`;
  }

  async function resetMfa(user: AdminUser) {
    await resetAdminUserMfa(user.id);
    status = `MFA reset for ${user.email}.`;
    await load();
  }

  async function setRole(user: AdminUser, bookId: number, role: "owner" | "editor" | "viewer") {
    await setAdminUserMembership(user.id, { book_id: bookId, role });
    await load();
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
        <Button onclick={createUser} disabled={!newEmail || !newName || newPassword.length < 12}>Create</Button>
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
                <Button variant="ghost" size="sm" onclick={() => resetPassword(user)}>Password</Button>
                <Button variant="ghost" size="sm" onclick={() => resetMfa(user)}>Reset MFA</Button>
                <Button variant="ghost" size="sm" onclick={() => revokeSessions(user)}>Revoke sessions</Button>
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
