<script lang="ts">
	import type { Snippet } from 'svelte';
	import NavRail from './NavRail.svelte';
	import ProjectSwitcher from './ProjectSwitcher.svelte';

	let {
		activePath = '/',
		children
	}: {
		activePath?: string;
		children: Snippet;
	} = $props();
</script>

<a
	href="#main-content"
	class="bg-primary text-primary-foreground focus-visible:outline-primary sr-only focus:not-sr-only focus:absolute focus:top-3 focus:left-3 focus:z-50 focus:rounded-md focus:px-3 focus:py-2 focus:text-sm focus:font-medium focus-visible:outline-2"
>
	Skip to main content
</a>

<div class="bg-background min-h-dvh lg:grid lg:grid-cols-[240px_minmax(0,1fr)]">
	<NavRail {activePath} />
	<div class="flex min-h-dvh min-w-0 flex-col">
		<header
			class="border-border bg-background/95 sticky top-0 z-20 flex h-14 items-center gap-4 border-b px-4 backdrop-blur"
		>
			<ProjectSwitcher />
			<div class="ml-auto flex items-center gap-3">
				<a
					href="/health"
					class="text-foreground-muted hover:text-foreground focus-visible:outline-primary rounded-md px-2 py-1 text-xs focus-visible:outline-2 focus-visible:outline-offset-2"
				>
					System health
				</a>
			</div>
		</header>
		<main id="main-content" tabindex="-1" class="min-w-0 flex-1">
			{@render children()}
		</main>
	</div>
</div>
