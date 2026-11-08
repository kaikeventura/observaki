<script lang="ts">
	import { onMount } from 'svelte';
	import { getHealth, type Health } from '#lib/api';
	import { activeProject } from '#lib/projects';
	import StateMessage from '#lib/components/StateMessage.svelte';
	import StatusPill from '#lib/components/StatusPill.svelte';

	let phase: 'loading' | 'ready' | 'error' = $state('loading');
	let health: Health | null = $state(null);
	let errorMessage = $state('');

	const project = $derived($activeProject);

	async function load() {
		phase = 'loading';
		try {
			health = await getHealth();
			phase = 'ready';
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : 'unknown error';
			phase = 'error';
		}
	}

	onMount(() => {
		void load();
	});
</script>

<svelte:head>
	<title>Settings · Observaki</title>
	<meta name="description" content="Project and workspace settings for Observaki." />
</svelte:head>

<div class="mx-auto max-w-6xl space-y-6 px-6 py-8">
	<header class="space-y-1">
		<h1 class="text-2xl font-semibold tracking-tight">Settings</h1>
		<p class="text-foreground-muted max-w-[72ch] text-sm">
			Project and workspace settings. Members, retention and token management arrive in Phase 1.
		</p>
	</header>

	<div class="grid gap-4 md:grid-cols-2">
		<section
			aria-labelledby="project-settings-heading"
			class="border-border bg-surface-1 space-y-3 rounded-lg border p-5"
		>
			<h2 id="project-settings-heading" class="text-sm font-medium">Project</h2>
			<dl class="grid gap-2 text-xs">
				<div class="flex items-center justify-between gap-3">
					<dt class="text-foreground-muted">Organization</dt>
					<dd class="font-mono">{project.organization}</dd>
				</div>
				<div class="flex items-center justify-between gap-3">
					<dt class="text-foreground-muted">Project</dt>
					<dd class="font-mono">{project.name}</dd>
				</div>
				<div class="flex items-center justify-between gap-3">
					<dt class="text-foreground-muted">Environment</dt>
					<dd><StatusPill label={project.environment} /></dd>
				</div>
				<div class="flex items-center justify-between gap-3">
					<dt class="text-foreground-muted">Project ID</dt>
					<dd class="font-mono">{project.id}</dd>
				</div>
			</dl>
			<p class="text-foreground-subtle text-[11px]">
				Switch projects from the top bar. The selection is remembered for the next visit.
			</p>
		</section>

		<section
			aria-labelledby="api-settings-heading"
			class="border-border bg-surface-1 space-y-3 rounded-lg border p-5"
		>
			<h2 id="api-settings-heading" class="text-sm font-medium">API connection</h2>
			{#if phase === 'loading'}
				<StateMessage
					variant="loading"
					title="Checking the API"
					body="Reading the build version."
				/>
			{:else if phase === 'error'}
				<StateMessage
					variant="error"
					title="The API is unavailable."
					body="The control plane did not answer. Retry, or check the system health page for the failing component."
					primaryLabel="Retry"
					onprimary={() => void load()}
				/>
				<p class="text-foreground-muted text-xs" role="status">{errorMessage}</p>
				<a
					href="/health"
					class="text-primary focus-visible:outline-primary text-xs underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2"
					>Open system health</a
				>
			{:else if health}
				<dl class="grid gap-2 text-xs">
					<div class="flex items-center justify-between gap-3">
						<dt class="text-foreground-muted">Status</dt>
						<dd><StatusPill label={health.status} tone="success" /></dd>
					</div>
					{#if health.version}
						<div class="flex items-center justify-between gap-3">
							<dt class="text-foreground-muted">Version</dt>
							<dd class="font-mono">{health.version}</dd>
						</div>
					{/if}
				</dl>
			{/if}
		</section>
	</div>
</div>
