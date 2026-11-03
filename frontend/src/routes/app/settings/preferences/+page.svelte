<script lang="ts">
  import AppearanceSettings from '#lib/settings/appearance-settings.svelte';
  import LanguageSettings from '#lib/settings/language-settings.svelte';
</script>

<div class="space-y-4" data-testid="preferences-settings">
  <AppearanceSettings />
  <LanguageSettings />
</div>
