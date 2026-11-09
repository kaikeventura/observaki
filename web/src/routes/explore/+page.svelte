<script lang="ts">
	import { onMount } from 'svelte';
	import { activeProject } from '#lib/projects';
	import QueryBar from '#lib/components/QueryBar.svelte';
	import SignalTabs from '#lib/components/SignalTabs.svelte';
	import StateMessage from '#lib/components/StateMessage.svelte';
	import StatusPill from '#lib/components/StatusPill.svelte';
	import TimeSeriesChart from '#lib/components/TimeSeriesChart.svelte';
	import TraceTable from '#lib/components/TraceTable.svelte';
	import VirtualLogTable from '#lib/components/VirtualLogTable.svelte';
	import { DEFAULT_LOG_ROWS, generateLogs, type LogRow } from '#lib/table/logs';
	import {
		ALL_SERVICES,
		DEFAULT_EXPLORE_STATE,
		SERVICE_OPTIONS,
		SIGNALS,
		TIME_RANGES,
		metricForQuery,
		parseExploreSearch,
		rangeById,
		signalLabel,
		signalNoun,
		stepRange,
		switchSignal,
		toExploreSearch,
		type ExploreState,
		type Signal,
		type TimeRangeId
	} from '#lib/explore/filters';
	import {
		TRACE_LIMIT,
		filterByService,
		filterLogs,
		generateTraces,
		metricSeries,
		statusFromQuery
	} from '#lib/explore/mock';

	const project = $derived($activeProject);
	let explore = $state<ExploreState>({ ...DEFAULT_EXPLORE_STATE });
	let mounted = $state(false);
	let runs = $state(0);
	let announcement = $state('');
	let endSeconds = $state(0);
	let allLogs = $state<LogRow[]>([]);

	const range = $derived(rangeById(explore.range));
	const serviceLabel = $derived(
		explore.service === ALL_SERVICES ? 'all services' : explore.service
	);
	const metric = $derived(metricForQuery(explore.query));

	const logs = $derived(filterLogs(filterByService(allLogs, explore.service), explore.query));
	const traces = $derived(
		mounted && explore.signal === 'traces'
			? generateTraces({
					rows: 2_400,
					service: explore.service,
					range: explore.range,
					endSeconds,
					status: statusFromQuery(explore.query)
				})
			: []
	);
	const series = $derived(
		mounted && explore.signal === 'metrics' ? metricSeries(metric, explore.range, endSeconds) : null
	);
	const resultCount = $derived(
		explore.signal === 'logs' ? logs.length : explore.signal === 'traces' ? traces.length : null
	);

	onMount(() => {
		endSeconds = Math.floor(Date.now() / 1000);
		allLogs = generateLogs({ rows: DEFAULT_LOG_ROWS, endSeconds });
		explore = parseExploreSearch(window.location.search);
		mounted = true;
	});

	// Keep the URL reproducible (EXPERIENCE.md): signal, range, service and query.
	$effect(() => {
		if (!mounted) return;
		const search = toExploreSearch(explore);
		const next = `${window.location.pathname}${search}`;
		if (`${window.location.pathname}${window.location.search}` !== next) {
			history.replaceState(null, '', next);
		}
	});

	function selectSignal(next: Signal): void {
		if (next === explore.signal) return;
		const result = switchSignal(explore, next);
		explore = result.state;
		if (result.announcement) announcement = result.announcement;
	}

	function chooseRange(id: TimeRangeId): void {
		if (id === explore.range) return;
		explore.range = id;
		announcement = `Time range ${rangeById(id).label}. Kept ${serviceLabel} and the query.`;
	}

	function chooseService(value: string): void {
		explore.service = value;
		announcement = `Showing ${value === ALL_SERVICES ? 'all services' : value} over ${range.label}.`;
	}

	function run(): void {
		runs += 1;
		announcement = `Query accepted for ${signalNoun(explore.signal)} over ${range.label} (${serviceLabel}). Results are deterministic mock data; the Query Service arrives in Phase 1.`;
	}

	function clearFilters(): void {
		explore.service = ALL_SERVICES;
		explore.query = '';
		announcement = 'Filters cleared.';
	}

	function onkeydown(event: KeyboardEvent): void {
		if (event.metaKey || event.ctrlKey || event.altKey) return;
		const target = event.target as HTMLElement | null;
		if (target?.closest('input, textarea, select, [contenteditable="true"]')) return;

		if (event.key === 't') {
			event.preventDefault();
			const index = SIGNALS.findIndex((entry) => entry.id === explore.signal);
			selectSignal(SIGNALS[(index + 1) % SIGNALS.length].id);
		} else if (event.key === '/') {
			event.preventDefault();
			document.querySelector<HTMLInputElement>('[data-query-input]')?.focus();
		} else if (event.key === '[') {
			event.preventDefault();
			chooseRange(stepRange(explore.range, -1));
		} else if (event.key === ']') {
			event.preventDefault();
			chooseRange(stepRange(explore.range, 1));
		}
	}
</script>

<svelte:head>
	<title>Explore · Observaki</title>
	<meta
		name="description"
		content="Query metrics, logs and traces for the active project in Observaki."
	/>
</svelte:head>

<svelte:window {onkeydown} />

<div class="mx-auto max-w-6xl space-y-6 px-6 py-8">
	<header class="space-y-1">
		<p class="text-foreground-muted font-mono text-[11px]">
			{project.organization} / {project.name} · {project.environment}
		</p>
		<h1 class="text-2xl font-semibold tracking-tight">Explore</h1>
		<p class="text-foreground-muted max-w-[72ch] text-sm">
			Mock Explore for Phase 0: switch between metrics, logs and traces without losing the sticky
			time range or the service filter, and type a query with autocomplete from mock fields. The
			real Query Service and URL-driven query results arrive in Phase 1.
		</p>
	</header>

	<section
		aria-labelledby="query-heading"
		class="border-border bg-surface-1 space-y-4 rounded-lg border p-4"
	>
		<div class="flex flex-wrap items-center justify-between gap-2">
			<h2 id="query-heading" class="text-sm font-medium">Query</h2>
			<StatusPill label="Mock data" tone="info" marker="i" />
		</div>

		<SignalTabs signal={explore.signal} panelId="explore-results" ontab={selectSignal} />

		<div class="flex flex-wrap items-end gap-x-6 gap-y-3">
			<div role="group" aria-label="Time range" class="space-y-1">
				<span id="range-label" class="text-foreground-muted block text-xs font-medium"
					>Time range</span
				>
				<div class="flex flex-wrap gap-1">
					{#each TIME_RANGES as entry (entry.id)}
						<button
							type="button"
							aria-pressed={explore.range === entry.id}
							data-range={entry.id}
							class="focus-visible:outline-primary rounded-md border px-2.5 py-1 font-mono text-xs focus-visible:outline-2 focus-visible:outline-offset-2 {explore.range ===
							entry.id
								? 'border-primary bg-surface-3 text-primary'
								: 'border-border-strong bg-surface-1 text-foreground-muted hover:bg-surface-3 hover:text-foreground'}"
							onclick={() => chooseRange(entry.id)}
						>
							{entry.label}
						</button>
					{/each}
				</div>
			</div>

			<div class="space-y-1">
				<label for="service-filter" class="text-foreground-muted block text-xs font-medium"
					>Service</label
				>
				<select
					id="service-filter"
					class="border-border-strong bg-surface-2 text-foreground focus-visible:outline-primary rounded-md border px-2.5 py-1 font-mono text-xs focus-visible:outline-2 focus-visible:outline-offset-2"
					value={explore.service}
					onchange={(event) => chooseService(event.currentTarget.value)}
				>
					{#each SERVICE_OPTIONS as service (service)}
						<option value={service}>{service === ALL_SERVICES ? 'All services' : service}</option>
					{/each}
				</select>
			</div>

			<p class="text-foreground-muted font-mono text-xs" data-timezone>
				{range.label} · UTC · {serviceLabel} · {runs} run{runs === 1 ? '' : 's'}
			</p>
		</div>

		<QueryBar bind:value={explore.query} signal={explore.signal} onrun={run} />

		<p
			class="text-foreground-subtle min-h-[1.25rem] text-xs"
			aria-live="polite"
			data-explore-status
		>
			{announcement}
		</p>
	</section>

	<section aria-labelledby="results-heading" class="space-y-3">
		<div class="flex flex-wrap items-baseline justify-between gap-2">
			<h2 id="results-heading" class="text-sm font-medium">
				{signalLabel(explore.signal)} results
			</h2>
			<p class="text-foreground-muted font-mono text-xs" data-result-summary>
				{#if explore.signal === 'metrics'}
					{metric.name} · {range.label} · {serviceLabel}
				{:else}
					{resultCount?.toLocaleString('en-US') ?? 0} rows · {range.label} · {serviceLabel}
				{/if}
			</p>
		</div>

		<div
			id="explore-results"
			role="tabpanel"
			aria-label={`${signalLabel(explore.signal)} results`}
			data-explore-panel={explore.signal}
		>
			{#if !mounted}
				<div
					class="border-border bg-surface-1 rounded-lg border"
					style:height="{488 + 44}px"
					aria-hidden="true"
				></div>
			{:else if explore.signal === 'metrics'}
				{#if series}
					<TimeSeriesChart
						{series}
						label={metric.name}
						unit={metric.unit}
						height={320}
						live={false}
					/>
				{/if}
				<p class="text-foreground-subtle mt-2 text-xs">
					Mock series for {serviceLabel} over {range.label}. The chart keeps the same decimation,
					zoom and accessible-table behaviour as the dashboard panel.
				</p>
			{:else if explore.signal === 'logs'}
				<VirtualLogTable
					rows={logs}
					status="ready"
					onretry={run}
					label="Logs"
					caption="Mock log query results"
					height={440}
				/>
			{:else if traces.length === 0}
				<StateMessage
					variant="empty"
					title="No traces match this filter."
					body={`No ${signalNoun(explore.signal)} in the last ${range.label} for ${serviceLabel}. Clear the filters or widen the time range.`}
					primaryLabel="Clear filters"
					onprimary={clearFilters}
				/>
			{:else}
				<TraceTable {traces} total={traces.length} />
				<p class="text-foreground-subtle mt-2 text-xs">
					Showing the newest {Math.min(TRACE_LIMIT, traces.length)} of {traces.length.toLocaleString(
						'en-US'
					)} traces. The waterfall and span detail arrive in Phase 1.
				</p>
			{/if}
		</div>
	</section>

	<section
		aria-labelledby="explore-notes-heading"
		class="border-border bg-surface-1 rounded-lg border p-4"
	>
		<h2 id="explore-notes-heading" class="text-sm font-medium">What this mock proves</h2>
		<ul class="text-foreground-muted mt-2 max-w-[80ch] list-disc space-y-1 pl-5 text-xs">
			<li>
				The time range and the service filter are sticky: switching metrics → logs → traces keeps
				both and announces exactly what was kept or dropped.
			</li>
			<li>
				The query bar autocompletes mock fields, values, metric names and MetricsQL functions with a
				real combobox pattern, Arrow/Enter/Escape and ⌘Enter to run.
			</li>
			<li>
				The investigation lives in the URL: signal, range, service and query survive a reload and a
				copied link.
			</li>
			<li>
				Keyboard: <kbd class="border-border rounded border px-1 font-mono text-[10px]">t</kbd>
				cycles signals, <kbd class="border-border rounded border px-1 font-mono text-[10px]">/</kbd>
				focuses the query, and
				<kbd class="border-border rounded border px-1 font-mono text-[10px]">[</kbd>/<kbd
					class="border-border rounded border px-1 font-mono text-[10px]">]</kbd
				>
				step the time range.
			</li>
		</ul>
	</section>
</div>
