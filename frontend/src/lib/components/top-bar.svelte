<script lang="ts">
    import { ExternalLink, Search } from "@lucide/svelte";
    import { m } from "$lib/paraglide/messages.js";
    import ThemeToggle from "./theme-toggle.svelte";

    let searchTerm = $state("");
</script>

<header
    class="sticky top-0 z-30 border-b border-topbar-border bg-topbar text-topbar-foreground shadow-sm"
>
    <div
        class="mx-auto grid max-w-7xl grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-4 py-3 sm:px-6 lg:grid-cols-[12rem_minmax(16rem,36rem)_auto] lg:px-8"
    >
        <a
            href="/"
            class="min-w-0 text-lg font-semibold tracking-tight text-topbar-foreground"
        >
            {m.app_name()}
        </a>

        <label
            class="relative order-3 col-span-2 min-w-0 lg:order-0 lg:col-span-1"
            for="global-search"
        >
            <span class="sr-only">{m.top_bar_search_label()}</span>
            <Search
                class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-topbar-muted"
                aria-hidden="true"
            />
            <input
                id="global-search"
                type="search"
                bind:value={searchTerm}
                placeholder={m.top_bar_search_placeholder()}
                class="h-7 w-full rounded-(--radius-control) border border-topbar-border bg-topbar-control py-2 pl-9 pr-3 text-sm text-topbar-foreground outline-none transition placeholder:text-topbar-muted hover:bg-topbar-control-hover focus:bg-topbar-control-hover"
            />
        </label>

        <div class="flex items-center justify-end gap-2">
            <ThemeToggle />
            <a
                href="https://github.com/sergeyfarin/rekenraam"
                target="_blank"
                rel="noreferrer"
                class="inline-flex h-7 items-center gap-2 rounded-(--radius-control) border border-topbar-border bg-topbar-control px-3 text-sm font-semibold text-topbar-foreground transition hover:bg-topbar-control-hover"
                aria-label={m.top_bar_github_label()}
                title={m.top_bar_github_label()}
            >
                <ExternalLink class="size-4" aria-hidden="true" />
                <span class="hidden sm:inline">{m.top_bar_github()}</span>
            </a>
        </div>
    </div>
</header>
