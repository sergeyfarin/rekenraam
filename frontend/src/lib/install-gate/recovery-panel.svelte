<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
</script>

<div class="space-y-4">
  <div class="rounded-[1.75rem] border border-danger/25 bg-danger-soft p-5 text-sm leading-6 text-danger shadow-[var(--shadow-panel)]">
    <p class="font-semibold text-foreground">{m.install_gate_recovery_command_label()}</p>
    <p class="mt-2 font-mono text-xs leading-6 sm:text-sm">
      printf '%s\n' 'new-password' | DATABASE_URL=file:backend/var/dev.sqlite go run ./backend/cmd/rekenraam recover-owner --password-stdin
    </p>
  </div>

  <p class="text-sm leading-6 text-muted">{m.install_gate_recovery_backup_note()}</p>
</div>
