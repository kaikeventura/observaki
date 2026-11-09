<script lang="ts">
	import { shortenId } from '#lib/table/logs';
	import { TRACE_LIMIT, type TraceRow } from '#lib/explore/mock';
	import StatusPill from './StatusPill.svelte';

	/**
	 * Trace preview for the mock Explore (Story 6.4). A native table capped at
	 * TRACE_LIMIT rows: the real waterfall is Phase 1.
	 */
	let { traces, total }: { traces: readonly TraceRow[]; total: number } = $props();

	const rows = $derived(traces.slice(0, TRACE_LIMIT));
	const numberFormat = new Intl.NumberFormat('en-US');
	const timeFormat = new Intl.DateTimeFormat('en-GB', {
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit',
		hour12: false,
		timeZone: 'UTC'
	});

	function clock(seconds: number): string {
		return timeFormat.format(new Date(seconds * 1000));
	}

	function duration(ms: number): string {
		return ms >= 1000 ? `${(ms / 1000).toFixed(2)} s` : `${Math.round(ms)} ms`;
	}
</script>

<div
	class="border-border bg-surface-1 overflow-hidden rounded-lg border"
	data-trace-total={total}
	data-trace-rendered={rows.length}
>
	<div class="border-border flex flex-wrap items-center justify-between gap-2 border-b px-4 py-2">
		<h3 class="text-sm font-medium">Traces</h3>
		<p class="text-foreground-muted font-mono text-xs">
			{numberFormat.format(rows.length)} of {numberFormat.format(total)} traces
		</p>
	</div>

	<div class="overflow-x-auto">
		<table class="w-full border-collapse text-left text-xs">
			<caption class="sr-only">
				Trace preview, newest first, limited to {TRACE_LIMIT} rows.
			</caption>
			<thead class="bg-surface-2 text-foreground-muted">
				<tr>
					<th scope="col" class="px-3 py-2 font-medium">Trace</th>
					<th scope="col" class="px-3 py-2 font-medium">Root span</th>
					<th scope="col" class="px-3 py-2 font-medium">Service</th>
					<th scope="col" class="px-3 py-2 text-right font-medium">Duration</th>
					<th scope="col" class="px-3 py-2 text-right font-medium">Spans</th>
					<th scope="col" class="px-3 py-2 font-medium">Status</th>
					<th scope="col" class="px-3 py-2 text-right font-medium">Started (UTC)</th>
				</tr>
			</thead>
			<tbody>
				{#each rows as trace (trace.id)}
					<tr class="border-border hover:bg-surface-2 border-t">
						<td class="text-foreground-muted px-3 py-2 font-mono">{shortenId(trace.id, 12)}</td>
						<td class="px-3 py-2 font-mono">{trace.rootSpan}</td>
						<td class="px-3 py-2">{trace.service}</td>
						<td class="px-3 py-2 text-right font-mono tabular-nums">{duration(trace.durationMs)}</td
						>
						<td class="px-3 py-2 text-right font-mono tabular-nums">{trace.spans}</td>
						<td class="px-3 py-2">
							<StatusPill
								label={trace.status === 'error' ? 'Error' : 'OK'}
								tone={trace.status === 'error' ? 'danger' : 'success'}
								marker={trace.status === 'error' ? '■' : '●'}
							/>
						</td>
						<td class="text-foreground-muted px-3 py-2 text-right font-mono tabular-nums"
							>{clock(trace.startSeconds)}</td
						>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
</div>
