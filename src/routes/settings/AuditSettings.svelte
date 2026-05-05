<script lang="ts">
  import { onMount } from "svelte";
  import { listAuditEvents, type AuditEvent } from "$lib/api/admin";
  import * as Card from "$lib/components/ui/card";
  import * as Alert from "$lib/components/ui/alert";
  import { Button } from "$lib/components/ui/button";

  let events: AuditEvent[] = [];
  let error = "";

  onMount(load);

  async function load() {
    error = "";
    try {
      events = await listAuditEvents({ limit: 100 });
    } catch (e) {
      error = String(e);
    }
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Audit</Card.Title>
    <Card.Description>Review recent administrative and account events.</Card.Description>
  </Card.Header>
  <Card.Content class="space-y-4">
    {#if error}<Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
    <Button variant="outline" onclick={load}>Refresh</Button>
    <div class="overflow-x-auto">
      <table class="w-full text-sm">
        <thead>
          <tr class="border-b text-left">
            <th class="py-2">When</th>
            <th>Event</th>
            <th>Actor</th>
            <th>Summary</th>
          </tr>
        </thead>
        <tbody>
          {#each events as event}
            <tr class="border-b">
              <td class="py-2">{new Date(event.created_at).toLocaleString()}</td>
              <td>{event.event_type}</td>
              <td>{event.actor_user_id ?? "system"}</td>
              <td>{event.summary}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </Card.Content>
</Card.Root>
