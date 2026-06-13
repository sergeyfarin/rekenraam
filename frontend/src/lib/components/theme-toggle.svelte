<script lang="ts">
    import MoonStar from '@lucide/svelte/icons/moon-star';
    import SunMedium from '@lucide/svelte/icons/sun-medium';
    import { m } from "$lib/paraglide/messages.js";
    import { themeState, toggleTheme } from "$lib/theme.svelte";

    const currentThemeLabel = $derived(
        themeState.name === "light"
            ? m.theme_name_light()
            : m.theme_name_dark(),
    );
    const switchThemeLabel = $derived(
        themeState.name === "light"
            ? m.theme_switch_to_dark()
            : m.theme_switch_to_light(),
    );
</script>

<button
    type="button"
    class="inline-flex h-7 items-center gap-2 rounded-(--radius-control) border border-topbar-border bg-topbar-control px-3 text-left text-topbar-foreground transition hover:bg-topbar-control-hover"
    aria-label={switchThemeLabel}
    title={switchThemeLabel}
    onclick={toggleTheme}
>
    <span class="flex size-5 items-center justify-center text-topbar-muted">
        {#if themeState.name === "light"}
            <SunMedium class="size-4" aria-hidden="true" />
        {:else}
            <MoonStar class="size-4" aria-hidden="true" />
        {/if}
    </span>

    <span class="hidden min-w-0 sm:block">
        <span class="block text-sm font-semibold">
            {currentThemeLabel}
        </span>
    </span>
</button>
