<script lang="ts">
	import SurfacePlaceholder from '#lib/components/SurfacePlaceholder.svelte';
</script>

<svelte:head>
	<title>Dashboards · Observaki</title>
	<meta
		name="description"
		content="Browse and manage Observaki dashboards for the active project."
	/>
</svelte:head>

<SurfacePlaceholder
	title="Dashboards"
	description="Browse, view and edit dashboards for the active project. Panels inherit the global time range; the editor arrives in Phase 1."
	stateTitle="No dashboards in this project yet."
	stateBody="Create a dashboard from a template or start from a blank grid. Panels inherit the global time range."
	primaryLabel="Create dashboard"
	primaryOutcome="Mock surface: the dashboard editor arrives in Phase 1."
	secondaryLabel="Browse templates"
	secondaryOutcome="Mock surface: the template gallery arrives in Phase 1."
/>
