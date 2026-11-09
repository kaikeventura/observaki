<script lang="ts">
	import { onMount } from 'svelte';
	import { activeProject } from '#lib/projects';
	import ChartSpike from './ChartSpike.svelte';

	const project = $derived($activeProject);
	let mounted = $state(false);

	onMount(() => {
		mounted = true;
	});
</script>

<svelte:head>
	<title>Dashboards · Observaki</title>
	<meta
		name="description"
		content="Browse and manage Observaki dashboards for the active project."
	/>
</svelte:head>

<div class="mx-auto max-w-6xl space-y-6 px-6 py-8">
	<header class="space-y-1">
		<p class="text-foreground-muted font-mono text-[11px]">
			{project.organization} / {project.name} · {project.environment}
		</p>
		<h1 class="text-2xl font-semibold tracking-tight">Dashboards</h1>
		<p class="text-foreground-muted max-w-[72ch] text-sm">
			Chart spike for Phase 0: a panel that renders 100k points, stays interactive and follows live
			data without rebuilding the plot. The dashboard grid and editor arrive in Phase 1.
		</p>
	</header>

	{#if mounted}
		<ChartSpike />
	{:else}
		<div class="border-border bg-surface-1 h-[26rem] rounded-lg border" aria-hidden="true"></div>
	{/if}

	<section
		aria-labelledby="chart-notes-heading"
		class="border-border bg-surface-1 rounded-lg border p-4"
	>
		<h2 id="chart-notes-heading" class="text-sm font-medium">What this panel proves</h2>
		<ul class="text-foreground-muted mt-2 max-w-[80ch] list-disc space-y-1 pl-5 text-xs">
			<li>100k points render in one uPlot instance; the plot is reused for live updates.</li>
			<li>
				Zoom, pan and hover work with the mouse, the wheel and the keyboard; values are read from
				the same arrays the chart draws.
			</li>
			<li>
				Every chart ships an accessible table with the visible range aggregated into 12 buckets, so
				the data is never color- or pointer-only.
			</li>
			<li>
				The panel reports its own first-paint and update timings so the budget is visible, not
				guessed.
			</li>
		</ul>
	</section>
</div>
