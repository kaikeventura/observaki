<script lang="ts">
	import { onMount } from 'svelte';
	import { getHealth, type Health } from '#lib/api';
	import { primaryNav, secondaryNav } from '#lib/navigation';
	import StatusPill from '#lib/components/StatusPill.svelte';

	type State = 'loading' | 'ok' | 'error';

	let status: State = $state('loading');
	let health: Health | null = $state(null);
	let errorMessage: string = $state('');

	onMount(async () => {
		try {
			health = await getHealth();
			status = 'ok';
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : 'unknown error';
			status = 'error';
		}
	});
</script>

<svelte:head>
	<title>Overview · Observaki</title>
	<meta
		name="description"
		content="Open-source, self-hosted, OTel-native observability platform."
	/>
</svelte:head>

<div class="mx-auto max-w-6xl space-y-8 px-6 py-8">
	<header class="space-y-2">
		<p class="text-primary font-mono text-xs tracking-widest uppercase">observaki</p>
		<h1 class="text-3xl font-semibold tracking-tight">Overview</h1>
		<p class="text-foreground-muted max-w-[72ch]">
			Open-source, self-hosted, OTel-native observability for metrics, logs and traces. The Phase 0
			shell wires navigation, the project switcher and the UI states; query surfaces arrive with the
			Query Service in Phase 1.
		</p>
	</header>

	<section
		aria-labelledby="api-status-heading"
		class="border-border bg-surface-1 rounded-lg border p-5"
	>
		<h2 id="api-status-heading" class="text-foreground-muted text-sm font-medium">API status</h2>

		<div class="mt-2 flex min-h-7 flex-wrap items-center gap-2 font-mono text-sm" role="status">
			{#if status === 'loading'}
				<span class="text-foreground-muted">Checking the API…</span>
			{:else if status === 'error'}
				<span class="text-danger" role="alert">API unavailable: {errorMessage}</span>
			{:else if health}
				<StatusPill label="Operational" tone="success" />
				{#if health.version}
					<span class="text-foreground-muted">{health.version}</span>
				{/if}
			{/if}
		</div>
	</section>

	<section aria-labelledby="surfaces-heading" class="space-y-3">
		<h2 id="surfaces-heading" class="text-foreground-muted text-sm font-medium">Surfaces</h2>
		<ul class="grid gap-3 sm:grid-cols-2">
			{#each primaryNav as item (item.href)}
				<li>
					<a
						href={item.href}
						class="border-border bg-surface-1 hover:bg-surface-2 focus-visible:outline-primary flex items-center justify-between rounded-lg border px-5 py-4 text-sm focus-visible:outline-2 focus-visible:outline-offset-2"
					>
						<span class="font-medium">{item.label}</span>
						<span class="text-foreground-muted" aria-hidden="true">→</span>
					</a>
				</li>
			{/each}
		</ul>
	</section>

	<nav aria-label="Reference surfaces">
		<ul class="flex flex-wrap gap-4 text-xs">
			{#each secondaryNav as item (item.href)}
				<li>
					<a
						href={item.href}
						class="text-foreground-muted hover:text-foreground focus-visible:outline-primary rounded-md px-1 py-0.5 underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2"
						>{item.label}</a
					>
				</li>
			{/each}
		</ul>
	</nav>
</div>
