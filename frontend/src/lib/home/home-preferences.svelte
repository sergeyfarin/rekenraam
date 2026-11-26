<script lang="ts">
  import Panel from '#lib/components/panel.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import HomeLayoutFeedback from './home-layout-feedback.svelte';
  import { useHomePersonalization } from './personalization.svelte.ts';

  const personalization = useHomePersonalization();
  const buttonClass = 'min-h-11 rounded-(--radius-control) border border-border bg-control px-4 text-sm font-semibold hover:bg-control-hover';
</script>

<Panel>
  <h2 class="text-lg font-semibold">{m.home_layout_section()}</h2>
  <HomeLayoutFeedback />
  <div class="mt-3 flex flex-wrap gap-2">
    <button type="button" class={buttonClass} onclick={() => personalization.open()}>{m.home_customize()}</button>
    <button type="button" class={buttonClass} onclick={() => personalization.open('reset')}>{m.home_layout_reset()}</button>
  </div>
</Panel>
