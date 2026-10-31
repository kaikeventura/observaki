<script lang="ts">
	import { onMount } from 'svelte';

	type Health = {
		status: string;
		version?: string;
	};

	type State = 'loading' | 'ok' | 'error';

	let status: State = $state('loading');
	let health: Health | null = $state(null);
	let errorMessage: string = $state('');

	onMount(async () => {
		try {
			const response = await fetch('/api/v1/health');
			if (!response.ok) {
				throw new Error(`HTTP ${response.status}`);
			}
			health = (await response.json()) as Health;
			status = 'ok';
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : 'unknown error';
			status = 'error';
		}
	});
</script>

<svelte:head>
	<title>Observaki</title>
	<meta
		name="description"
		content="Open-source, self-hosted, OTel-native observability platform."
	/>
</svelte:head>

<main class="grid min-h-dvh place-items-center px-6 py-12">
	<div class="w-full max-w-lg space-y-8">
		<header class="space-y-2">
			<p class="text-primary font-mono text-xs tracking-widest uppercase">observaki</p>
			<h1 class="text-3xl font-semibold tracking-tight">Observaki</h1>
			<p class="text-foreground-muted">
				Open-source, self-hosted, OTel-native observability for metrics, logs and traces.
			</p>
		</header>

		<section
			class="border-border bg-surface-1 rounded-lg border p-5"
			aria-labelledby="api-status-heading"
		>
			<h2 id="api-status-heading" class="text-foreground-muted text-sm font-medium">API status</h2>

			{#if status === 'loading'}
				<p class="mt-2 font-mono text-sm" role="status">Checking the API…</p>
			{:else if status === 'error'}
				<p class="text-danger mt-2 text-sm" role="alert">API unavailable: {errorMessage}</p>
			{:else if health}
				<p class="mt-2 flex items-center gap-2 font-mono text-sm">
					<span class="bg-success inline-block size-2 rounded-full" aria-hidden="true"></span>
					<span>Operational</span>
					{#if health.version}
						<span class="text-foreground-muted">{health.version}</span>
					{/if}
				</p>
			{/if}
		</section>
	</div>
</main>
