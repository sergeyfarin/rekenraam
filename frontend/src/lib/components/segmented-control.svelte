<script lang="ts" generics="T extends string">
  type Option = { value: T; label: string };

  let {
    legend,
    label,
    value,
    options,
    onChange
  }: {
    /** Accessible name for the group when the visible label is abbreviated. */
    legend: string;
    label: string;
    value: T;
    options: Option[];
    onChange: (value: T) => void;
  } = $props();
</script>

<!--
  One control, not a row of separate buttons: only the outer corners round and
  neighbouring segments share a border. Each segment keeps its own
  `aria-pressed` state so assistive technology still hears a toggle group.
-->
<fieldset class="flex flex-wrap items-center gap-2">
  <legend class="sr-only">{legend}</legend>
  <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{label}</span>
  <div class="inline-flex max-w-full">
    {#each options as option (option.value)}
      <button
        type="button"
        aria-pressed={value === option.value}
        onclick={() => onChange(option.value)}
        class={`relative h-9 border border-border px-3 text-sm font-medium whitespace-nowrap transition-colors first:rounded-l-(--radius-control) last:rounded-r-(--radius-control) -ml-px first:ml-0 focus-visible:z-20 ${
          value === option.value
            ? 'z-10 border-accent bg-accent text-accent-foreground'
            : 'bg-control text-foreground hover:bg-control-hover'
        }`}
      >
        {option.label}
      </button>
    {/each}
  </div>
</fieldset>
