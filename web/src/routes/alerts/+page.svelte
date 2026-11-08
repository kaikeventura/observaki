<script lang="ts">
	import SurfacePlaceholder from '#lib/components/SurfacePlaceholder.svelte';
</script>

<svelte:head>
	<title>Alerts · Observaki</title>
	<meta name="description" content="Manage Observaki alert rules, silences and channels." />
</svelte:head>

<SurfacePlaceholder
	title="Alerts"
	description="Firing alerts, rules, silences and channels for the active project. The rule builder arrives in Phase 1."
	stateTitle="No alert rules in this project."
	stateBody="Create a rule from a query, preview it against the last 24 hours and route it to a webhook or Slack channel."
	primaryLabel="Create alert rule"
	primaryOutcome="Mock surface: the alert rule builder arrives in Phase 1."
	secondaryLabel="Open alert settings"
	secondaryOutcome="Mock surface: alert channels and silences arrive in Phase 1."
/>
