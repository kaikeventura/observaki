<script lang="ts">
	import SurfacePlaceholder from '#lib/components/SurfacePlaceholder.svelte';
</script>

<svelte:head>
	<title>Explore · Observaki</title>
	<meta
		name="description"
		content="Query metrics, logs and traces for the active project in Observaki."
	/>
</svelte:head>

<SurfacePlaceholder
	title="Explore"
	description="Query metrics, logs and traces for the active project. The query engine lands with the Query Service in Phase 1; this shell wires the surface, the time-range context and the keyboard path."
	stateTitle="No traces match this filter."
	stateBody="Try widening the time range or removing the status filter. The query stays in the bar; nothing was lost."
	primaryLabel="Clear filters"
	primaryOutcome="Mock surface: filters cleared. Signals arrive with the Query Service in Phase 1."
	secondaryLabel="Widen to 1h"
	secondaryOutcome="Mock surface: time range widened to 1h."
/>
