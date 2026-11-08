<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { getHealthOverview, type HealthOverview, type HealthStatus } from '#lib/api';

	const REFRESH_MS = 10_000;

	let overview: HealthOverview | null = $state(null);
	let phase: 'loading' | 'ready' | 'error' = $state('loading');
	let errorMessage = $state('');
	let lastSnapshotAt: string | null = $state(null);
	let timer: ReturnType<typeof setInterval> | undefined;

	async function refresh() {
		try {
			overview = await getHealthOverview();
			lastSnapshotAt = overview.generated_at;
			phase = 'ready';
			errorMessage = '';
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : 'unknown error';
			phase = 'error';
		}
	}

	onMount(() => {
		void refresh();
		timer = setInterval(() => void refresh(), REFRESH_MS);
	});

	onDestroy(() => {
		if (timer) clearInterval(timer);
	});

	const statusText: Record<HealthStatus, string> = {
		healthy: 'Healthy',
		degraded: 'Degraded',
		critical: 'Critical',
		no_data: 'No data',
		idle: 'Idle',
		not_configured: 'Not configured',
		disabled: 'Disabled',
		unknown: 'Unknown'
	};

	const statusTone: Record<HealthStatus, string> = {
		healthy: 'text-success border-success/40',
		degraded: 'text-warning border-warning/40',
		critical: 'text-danger border-danger/40',
		no_data: 'text-foreground-muted border-border',
		idle: 'text-foreground-muted border-border',
		not_configured: 'text-foreground-muted border-border',
		disabled: 'text-foreground-muted border-border',
		unknown: 'text-warning border-warning/40'
	};

	const statusMarker: Record<HealthStatus, string> = {
		healthy: '●',
		degraded: '▲',
		critical: '■',
		no_data: '○',
		idle: '○',
		not_configured: '–',
		disabled: '–',
		unknown: '?'
	};

	function label(status: HealthStatus): string {
		return statusText[status] ?? status;
	}

	function tone(status: HealthStatus): string {
		return statusTone[status] ?? statusTone.unknown;
	}

	function marker(status: HealthStatus): string {
		return statusMarker[status] ?? statusMarker.unknown;
	}

	function formatDuration(seconds?: number): string {
		if (seconds === undefined || Number.isNaN(seconds)) return 'No data';
		if (seconds < 1) return `${Math.round(seconds * 1000)} ms`;
		return `${seconds.toFixed(seconds < 10 ? 1 : 0)} s`;
	}

	function formatBytes(bytes?: number): string {
		if (bytes === undefined || Number.isNaN(bytes)) return 'No data';
		const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
		let value = bytes;
		let index = 0;
		while (value >= 1024 && index < units.length - 1) {
			value /= 1024;
			index += 1;
		}
		return `${value.toFixed(value >= 10 || index === 0 ? 0 : 1)} ${units[index]}`;
	}

	function formatCount(value?: number): string {
		if (value === undefined || Number.isNaN(value)) return 'No data';
		return new Intl.NumberFormat().format(value);
	}

	function formatPercent(rate: number): string {
		return `${(rate * 100).toFixed(2)}%`;
	}

	function formatTime(value?: string | null): string {
		if (!value) return 'unknown';
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return date.toLocaleString();
	}
</script>

<svelte:head>
	<title>System health · Observaki</title>
	<meta
		name="description"
		content="Observaki self-observability: ingestion lag, query error rate, storage usage and component health."
	/>
</svelte:head>

<main class="mx-auto max-w-6xl space-y-8 px-6 py-10">
	<header class="space-y-3">
		<div class="flex flex-wrap items-end justify-between gap-4">
			<div class="space-y-1">
				<p class="text-primary font-mono text-xs tracking-widest uppercase">observaki</p>
				<h1 class="text-3xl font-semibold tracking-tight">System health</h1>
			</div>
			<a
				href="/"
				class="text-foreground-muted hover:text-foreground focus-visible:outline-primary rounded-md px-2 py-1 text-sm focus-visible:outline-2"
				>← Home</a
			>
		</div>
		<p class="text-foreground-muted max-w-2xl text-sm">
			Ingestion, query, storage and component health. Refreshes every 10 seconds; when a source is
			unavailable the last known values stay visible with their observation time.
		</p>
	</header>

	{#if phase === 'error'}
		<div
			role="alert"
			class="border-danger/40 bg-danger/10 text-danger rounded-lg border px-4 py-3 text-sm"
		>
			Health overview unavailable: {errorMessage}. Retrying every 10 seconds.
			{#if lastSnapshotAt}
				Showing the last snapshot from {formatTime(lastSnapshotAt)}.
			{/if}
		</div>
	{/if}

	{#if phase === 'loading' && !overview}
		<p role="status" class="text-foreground-muted font-mono text-sm">
			Loading the health overview…
		</p>
	{:else if overview}
		<section aria-labelledby="overall-heading" class="space-y-3">
			<h2 id="overall-heading" class="text-foreground-muted text-sm font-medium">Overall</h2>
			<p role="status" aria-live="polite" class="flex flex-wrap items-center gap-3 text-sm">
				<span
					class="inline-flex items-center gap-2 rounded-full border px-3 py-1 font-mono text-xs {tone(
						overview.status
					)}"
				>
					<span aria-hidden="true">{marker(overview.status)}</span>
					<span>{label(overview.status)}</span>
				</span>
				<span class="text-foreground-muted">{overview.summary.join(' · ')}</span>
			</p>
			<p class="text-foreground-muted font-mono text-xs">
				Snapshot {formatTime(overview.generated_at)}
			</p>
		</section>

		<section aria-label="Signals" class="grid gap-4 md:grid-cols-3">
			<article
				aria-labelledby="ingestion-heading"
				class="border-border bg-surface-1 space-y-3 rounded-lg border p-5"
			>
				<div class="flex items-start justify-between gap-3">
					<h2 id="ingestion-heading" class="text-sm font-medium">Ingestion lag</h2>
					<span
						class="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 font-mono text-xs {tone(
							overview.ingestion.status
						)}"
					>
						<span aria-hidden="true">{marker(overview.ingestion.status)}</span>
						{label(overview.ingestion.status)}
					</span>
				</div>
				<p class="font-mono text-2xl">
					{formatDuration(overview.ingestion.lag_p95_seconds)}
					<span class="text-foreground-muted text-sm">p95</span>
				</p>
				<dl class="text-foreground-muted grid grid-cols-2 gap-2 text-xs">
					<div>
						<dt>Pending messages</dt>
						<dd class="text-foreground font-mono">
							{formatCount(overview.ingestion.pending_messages)}
						</dd>
					</div>
					<div>
						<dt>Dead letters</dt>
						<dd class="text-foreground font-mono">
							{formatCount(overview.ingestion.dlq_messages)}
						</dd>
					</div>
					<div>
						<dt>Confirmed events</dt>
						<dd class="text-foreground font-mono">
							{formatCount(overview.ingestion.written_events)}
						</dd>
					</div>
					<div>
						<dt>Writer WAL</dt>
						<dd class="text-foreground font-mono">
							{overview.ingestion.writer_connected === undefined
								? 'No data'
								: overview.ingestion.writer_connected
									? 'Connected'
									: 'Disconnected'}
						</dd>
					</div>
				</dl>
				{#if overview.ingestion.observed_at}
					<p class="text-foreground-muted text-xs">
						Observed {formatTime(overview.ingestion.observed_at)}
					</p>
				{/if}
				{#if overview.ingestion.detail}
					<p class="text-foreground-muted text-xs">{overview.ingestion.detail}</p>
				{/if}
			</article>

			<article
				aria-labelledby="query-heading"
				class="border-border bg-surface-1 space-y-3 rounded-lg border p-5"
			>
				<div class="flex items-start justify-between gap-3">
					<h2 id="query-heading" class="text-sm font-medium">Query error rate</h2>
					<span
						class="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 font-mono text-xs {tone(
							overview.query.status
						)}"
					>
						<span aria-hidden="true">{marker(overview.query.status)}</span>
						{label(overview.query.status)}
					</span>
				</div>
				<p class="font-mono text-2xl">{formatPercent(overview.query.error_rate)}</p>
				<dl class="text-foreground-muted grid grid-cols-2 gap-2 text-xs">
					<div>
						<dt>Requests</dt>
						<dd class="text-foreground font-mono">{formatCount(overview.query.requests)}</dd>
					</div>
					<div>
						<dt>Server errors</dt>
						<dd class="text-foreground font-mono">{formatCount(overview.query.errors)}</dd>
					</div>
					<div>
						<dt>Client errors</dt>
						<dd class="text-foreground font-mono">{formatCount(overview.query.client_errors)}</dd>
					</div>
					<div>
						<dt>Window since</dt>
						<dd class="text-foreground font-mono">{formatTime(overview.query.since)}</dd>
					</div>
				</dl>
				{#if overview.query.detail}
					<p class="text-foreground-muted text-xs">{overview.query.detail}</p>
				{/if}
			</article>

			<article
				aria-labelledby="storage-heading"
				class="border-border bg-surface-1 space-y-3 rounded-lg border p-5"
			>
				<div class="flex items-start justify-between gap-3">
					<h2 id="storage-heading" class="text-sm font-medium">Storage usage</h2>
					<span
						class="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 font-mono text-xs {tone(
							overview.storage.status
						)}"
					>
						<span aria-hidden="true">{marker(overview.storage.status)}</span>
						{label(overview.storage.status)}
					</span>
				</div>
				<dl class="space-y-2 text-xs">
					<div class="flex items-center justify-between gap-3">
						<dt class="text-foreground-muted">ClickHouse</dt>
						<dd class="font-mono">
							{formatBytes(overview.storage.clickhouse_bytes)}
							<span class="text-foreground-muted"
								>({formatCount(overview.storage.clickhouse_parts)} parts)</span
							>
						</dd>
					</div>
					<div class="flex items-center justify-between gap-3">
						<dt class="text-foreground-muted">VictoriaMetrics</dt>
						<dd class="font-mono">
							{formatBytes(overview.storage.victoriametrics_bytes)}
							<span class="text-foreground-muted"
								>({formatCount(overview.storage.victoriametrics_series)} series)</span
							>
						</dd>
					</div>
				</dl>
				{#if overview.storage.observed_at}
					<p class="text-foreground-muted text-xs">
						Observed {formatTime(overview.storage.observed_at)}
					</p>
				{/if}
				{#if overview.storage.detail}
					<p class="text-foreground-muted text-xs">{overview.storage.detail}</p>
				{/if}
			</article>
		</section>

		<section aria-labelledby="components-heading" class="space-y-3">
			<h2 id="components-heading" class="text-foreground-muted text-sm font-medium">Components</h2>
			<div class="border-border overflow-x-auto rounded-lg border">
				<table class="w-full border-collapse text-left text-sm">
					<caption class="sr-only">Health check of each Observaki component</caption>
					<thead class="bg-surface-2 text-foreground-muted text-xs uppercase">
						<tr>
							<th scope="col" class="px-4 py-2 font-medium">Component</th>
							<th scope="col" class="px-4 py-2 font-medium">Status</th>
							<th scope="col" class="px-4 py-2 font-medium">Latency</th>
							<th scope="col" class="px-4 py-2 font-medium">Detail</th>
						</tr>
					</thead>
					<tbody>
						{#each overview.components as component (component.name)}
							<tr class="border-border border-t">
								<th scope="row" class="px-4 py-2 font-mono text-xs font-normal">{component.name}</th
								>
								<td class="px-4 py-2">
									<span
										class="inline-flex items-center gap-1.5 font-mono text-xs {tone(
											component.status
										)}"
									>
										<span aria-hidden="true">{marker(component.status)}</span>
										{label(component.status)}
									</span>
								</td>
								<td class="px-4 py-2 font-mono text-xs">{component.latency_ms.toFixed(1)} ms</td>
								<td class="text-foreground-muted px-4 py-2 text-xs">
									{component.detail ?? ''}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>

		<section
			aria-labelledby="alerting-heading"
			class="border-border bg-surface-1 space-y-3 rounded-lg border p-5"
		>
			<div class="flex items-start justify-between gap-3">
				<h2 id="alerting-heading" class="text-sm font-medium">Internal alerting</h2>
				<span
					class="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 font-mono text-xs {tone(
						overview.alerting.status
					)}"
				>
					<span aria-hidden="true">{marker(overview.alerting.status)}</span>
					{label(overview.alerting.status)}
				</span>
			</div>
			<dl class="text-foreground-muted grid grid-cols-2 gap-2 text-xs md:grid-cols-4">
				<div>
					<dt>Firing</dt>
					<dd class="text-foreground font-mono">{formatCount(overview.alerting.firing)}</dd>
				</div>
				<div>
					<dt>Pending</dt>
					<dd class="text-foreground font-mono">{formatCount(overview.alerting.pending)}</dd>
				</div>
				<div>
					<dt>Notifications queued</dt>
					<dd class="text-foreground font-mono">
						{formatCount(overview.alerting.notifications_pending)}
					</dd>
				</div>
				<div>
					<dt>Last evaluation</dt>
					<dd class="text-foreground font-mono">
						{formatDuration(overview.alerting.evaluation_age_seconds)} ago
					</dd>
				</div>
			</dl>
			{#if overview.alerting.detail}
				<p class="text-foreground-muted text-xs">{overview.alerting.detail}</p>
			{/if}
			{#if overview.alerting.last_error}
				<p class="text-danger text-xs" role="alert">Last error: {overview.alerting.last_error}</p>
			{/if}
		</section>
	{/if}
</main>
