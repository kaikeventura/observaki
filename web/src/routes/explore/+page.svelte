<script lang="ts">
	import { onMount } from 'svelte';
	import { activeProject } from '#lib/projects';
	import VirtualLogTable from '#lib/components/VirtualLogTable.svelte';
	import { DEFAULT_LOG_ROWS, generateLogs, type LogRow, type TableStatus } from '#lib/table/logs';

	type Scenario = 'loaded' | 'empty' | 'error';

	const project = $derived($activeProject);
	let mounted = $state(false);
	let scenario = $state<Scenario>('loaded');
	let rows = $state<LogRow[]>([]);
	let queries = $state(0);

	const status = $derived<TableStatus>(
		scenario === 'empty' ? 'empty' : scenario === 'error' ? 'error' : 'ready'
	);

	function load() {
		rows = generateLogs({ rows: DEFAULT_LOG_ROWS });
		scenario = 'loaded';
		queries += 1;
	}

	function choose(next: Scenario) {
		scenario = next;
		if (next === 'loaded' && rows.length === 0) load();
	}

	onMount(() => {
		mounted = true;
		load();
	});
</script>

<svelte:head>
	<title>Explore · Observaki</title>
	<meta
		name="description"
		content="Query metrics, logs and traces for the active project in Observaki."
	/>
</svelte:head>

<div class="mx-auto max-w-6xl space-y-6 px-6 py-8">
	<header class="space-y-1">
		<p class="text-foreground-muted font-mono text-[11px]">
			{project.organization} / {project.name} · {project.environment}
		</p>
		<h1 class="text-2xl font-semibold tracking-tight">Explore</h1>
		<p class="text-foreground-muted max-w-[72ch] text-sm">
			Table spike for Phase 0: a 10,000-row log result where only the visible window lives in the
			DOM. Sort, resize columns and open a row without freezing the browser. The real query bar,
			signal tabs and Query Service arrive in the next story and Phase 1.
		</p>
	</header>

	<section aria-labelledby="scenario-heading" class="space-y-3">
		<div class="flex flex-wrap items-end justify-between gap-3">
			<div class="space-y-1">
				<h2 id="scenario-heading" class="text-sm font-medium">Mock query</h2>
				<p class="text-foreground-muted text-xs">
					{DEFAULT_LOG_ROWS.toLocaleString('en-US')} deterministic log rows over roughly six hours · query
					{queries}
				</p>
			</div>
			<div role="group" aria-label="Table scenarios" class="flex flex-wrap gap-2">
				<button
					type="button"
					data-scenario="loaded"
					aria-pressed={scenario === 'loaded'}
					class="focus-visible:outline-primary rounded-md border px-2.5 py-1.5 text-xs font-medium focus-visible:outline-2 focus-visible:outline-offset-2 {scenario ===
					'loaded'
						? 'border-primary bg-surface-3 text-primary'
						: 'border-border-strong bg-surface-1 text-foreground-muted hover:bg-surface-3 hover:text-foreground'}"
					onclick={() => choose('loaded')}>Loaded (10k)</button
				>
				<button
					type="button"
					data-scenario="empty"
					aria-pressed={scenario === 'empty'}
					class="focus-visible:outline-primary rounded-md border px-2.5 py-1.5 text-xs font-medium focus-visible:outline-2 focus-visible:outline-offset-2 {scenario ===
					'empty'
						? 'border-primary bg-surface-3 text-primary'
						: 'border-border-strong bg-surface-1 text-foreground-muted hover:bg-surface-3 hover:text-foreground'}"
					onclick={() => choose('empty')}>Empty result</button
				>
				<button
					type="button"
					data-scenario="error"
					aria-pressed={scenario === 'error'}
					class="focus-visible:outline-primary rounded-md border px-2.5 py-1.5 text-xs font-medium focus-visible:outline-2 focus-visible:outline-offset-2 {scenario ===
					'error'
						? 'border-primary bg-surface-3 text-primary'
						: 'border-border-strong bg-surface-1 text-foreground-muted hover:bg-surface-3 hover:text-foreground'}"
					onclick={() => choose('error')}>Query error</button
				>
			</div>
		</div>

		{#if mounted}
			<VirtualLogTable
				rows={status === 'ready' ? rows : []}
				{status}
				onretry={load}
				label="Logs"
				caption="Mock log query results"
				requestId={status === 'error' ? 'req_01J8Z3K4' : undefined}
			/>
		{:else}
			<div
				class="border-border bg-surface-1 rounded-lg border"
				style:height="{544 + 44 + 32}px"
				aria-hidden="true"
			></div>
		{/if}
	</section>

	<section
		aria-labelledby="table-notes-heading"
		class="border-border bg-surface-1 rounded-lg border p-4"
	>
		<h2 id="table-notes-heading" class="text-sm font-medium">What this table proves</h2>
		<ul class="text-foreground-muted mt-2 max-w-[80ch] list-disc space-y-1 pl-5 text-xs">
			<li>
				10,000 rows cost a constant amount of DOM: the visible window plus a small overscan, with
				the rest reserved as spacer height.
			</li>
			<li>
				Sorting is explicit, column widths are adjustable by pointer or keyboard, and both survive
				selection and sorting.
			</li>
			<li>
				Rows are reachable with j/k, open with Enter or a click, and the detail panel closes with
				Escape without losing the selected row.
			</li>
			<li>
				Empty and error results keep the same frame and offer a retry, so the layout does not jump.
			</li>
		</ul>
	</section>
</div>
