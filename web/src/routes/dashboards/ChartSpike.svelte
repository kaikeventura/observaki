<script lang="ts">
	import { appendPoint, DEFAULT_POINTS, generateSeries, sampleValue } from '#lib/chart/series';
	import TimeSeriesChart from '#lib/components/TimeSeriesChart.svelte';

	// Created in the browser only (the page mounts this component after
	// hydration) so the prerendered HTML never carries a stale time range.
	const series = generateSeries();

	let revision = $state(0);
	let live = $state(true);

	function append(timestamp: number) {
		const last = series.timestamps[series.timestamps.length - 1];
		const next = timestamp > last ? timestamp : last + 1;
		if (appendPoint(series, next, sampleValue(next))) revision += 1;
	}

	function appendOne() {
		append(series.timestamps[series.timestamps.length - 1] + 1);
	}

	$effect(() => {
		if (!live) return;
		const id = setInterval(() => {
			append(series.timestamps[series.timestamps.length - 1] + 1);
		}, 1000);
		return () => clearInterval(id);
	});
</script>

<div class="space-y-3">
	<div class="flex flex-wrap items-center justify-between gap-3">
		<p class="text-foreground-muted text-xs">
			{DEFAULT_POINTS.toLocaleString('en-US')} deterministic points at 1 s resolution. The window slides,
			so the typed arrays never grow.
		</p>
		<div class="flex flex-wrap gap-2">
			<button
				type="button"
				class="border-border-strong bg-surface-1 hover:bg-surface-3 focus-visible:outline-primary rounded-md border px-2.5 py-1.5 text-xs font-medium focus-visible:outline-2 focus-visible:outline-offset-2"
				aria-pressed={live}
				onclick={() => (live = !live)}
			>
				{live ? 'Pause live' : 'Resume live'}
			</button>
			<button
				type="button"
				class="border-border-strong bg-surface-1 hover:bg-surface-3 focus-visible:outline-primary rounded-md border px-2.5 py-1.5 text-xs font-medium focus-visible:outline-2 focus-visible:outline-offset-2"
				onclick={appendOne}
			>
				Append one point
			</button>
		</div>
	</div>

	<TimeSeriesChart {series} {revision} {live} label="Request rate" unit="req/s" height={320} />
</div>
