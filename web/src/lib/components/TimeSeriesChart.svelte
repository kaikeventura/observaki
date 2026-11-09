<script lang="ts">
	import 'uplot/dist/uPlot.min.css';
	import { onMount, untrack } from 'svelte';
	import type uPlot from 'uplot';
	import {
		decimate,
		isFullRange,
		latestRange,
		panRange,
		seriesRange,
		summarize,
		visibleStats,
		zoomRange,
		zoomRangeAt,
		type PlotBuffer,
		type PlotSeries,
		type TimeSeries
	} from '#lib/chart/series';
	import {
		formatAxisTime,
		formatAxisValue,
		formatRangeLabel,
		formatTimestamp,
		formatValue,
		timezoneName
	} from '#lib/chart/format';
	import { readChartTheme, type ChartTheme } from '#lib/chart/theme';

	type ChartInstance = uPlot;
	type Tooltip = {
		left: number;
		top: number;
		time: string;
		value: string;
		rawTime: number;
		rawValue: number;
	};
	type RangeReason = 'zoom' | 'pan' | 'reset' | 'latest';

	let {
		series,
		revision = 0,
		label,
		unit = '',
		height = 320,
		live = false,
		onready,
		onupdate
	}: {
		series: TimeSeries;
		revision?: number;
		label: string;
		unit?: string;
		height?: number;
		live?: boolean;
		onready?: (chart: ChartInstance) => void;
		onupdate?: (elapsedMs: number) => void;
	} = $props();

	const chartId = $props.id();

	let chart: ChartInstance | undefined;
	let host: HTMLDivElement;

	let status = $state<'loading' | 'ready' | 'error'>('loading');
	let errorMessage = $state('');
	let renderMs = $state<number | null>(null);
	let paintMs = $state<number | null>(null);
	let updateMs = $state<number | null>(null);
	let instances = $state(0);
	let updates = $state(0);
	let range = $state<[number, number] | null>(null);
	let zoomed = $state(false);
	let tooltip = $state<Tooltip | null>(null);
	let announcement = $state('');
	let pendingAnnouncement = '';
	// The accessible table and the min/max/avg line are expensive (they scan the
	// visible points). They settle after the interaction instead of running on
	// every zoom frame, so dragging and scrolling stay at frame rate.
	let settledRange = $state<[number, number] | null>(null);
	let settleTimer: ReturnType<typeof setTimeout> | undefined;
	let drawnPoints = $state(0);

	let plotBuffer: PlotBuffer | null = null;
	let plotBudget = 0;
	let plotRange: [number, number] | null = null;
	let redecimating = false;

	const points = $derived(series.timestamps.length);
	const fullRange = $derived.by(() => {
		// `revision` changes when the parent appends live points in place.
		void revision;
		return seriesRange(series);
	});
	const visibleRange = $derived<[number, number]>(range ?? fullRange);
	const tableRange = $derived<[number, number]>(settledRange ?? fullRange);
	const buckets = $derived.by(() => {
		void revision;
		return summarize(series, 12, tableRange);
	});
	const stats = $derived.by(() => {
		void revision;
		return visibleStats(series, tableRange);
	});
	const zone = timezoneName();

	const reasonLabel: Record<RangeReason, string> = {
		zoom: 'Zoomed',
		pan: 'Panned',
		reset: 'Full range',
		latest: 'Latest 10%'
	};

	function buildOptions(width: number, theme: ChartTheme): uPlot.Options {
		return {
			width,
			height,
			padding: [12, 12, 0, 0],
			legend: { show: false },
			cursor: {
				y: false,
				points: { size: 6, width: 2, stroke: () => theme.primary },
				drag: { x: true, y: false, setScale: true, uni: 20 }
			},
			scales: {
				x: { time: true },
				y: {
					range: (_u, min, max) => {
						if (min == null || max == null) return [0, 1];
						const pad = Math.max(1, (max - min) * 0.08);
						return [Math.max(0, min - pad), max + pad];
					}
				}
			},
			series: [
				{},
				{
					label,
					stroke: theme.primary,
					width: 1.5,
					points: { show: false }
				}
			],
			axes: [
				{
					stroke: theme.axis,
					grid: { stroke: theme.grid, width: 1 },
					ticks: { stroke: theme.grid, width: 1 },
					font: `11px ${theme.font}`,
					size: 32,
					values: (u, splits) => {
						const min = u.scales.x.min ?? 0;
						const max = u.scales.x.max ?? 1;
						const span = Math.max(1, max - min);
						return splits.map((value) => formatAxisTime(value, span));
					}
				},
				{
					stroke: theme.axis,
					grid: { stroke: theme.grid, width: 1 },
					ticks: { stroke: theme.grid, width: 1 },
					font: `11px ${theme.font}`,
					size: 56,
					values: (_u, splits) => splits.map((value) => formatAxisValue(value))
				}
			],
			hooks: {
				setScale: [
					(u, key) => {
						if (key !== 'x') return;
						const min = u.scales.x.min;
						const max = u.scales.x.max;
						if (min == null || max == null) return;
						const next: [number, number] = [min, max];
						const full = seriesRange(series);
						const stillFull = isFullRange(next, full);
						if (!redecimating && !sameRange(plotRange, next)) {
							// A drag-select zoom changes the window without going through
							// applyRange; re-decimate so the zoomed window is as sharp as
							// a wheel or keyboard zoom.
							redecimating = true;
							try {
								const plot = plotFor(next);
								u.setData([plot.timestamps, plot.values], false);
							} finally {
								redecimating = false;
							}
						}
						range = next;
						zoomed = !stillFull;
						scheduleSettle(stillFull ? null : next);
					}
				]
			}
		};
	}

	function currentRange(): [number, number] {
		const fallback = seriesRange(series);
		if (!chart) return fallback;
		const min = chart.scales.x.min;
		const max = chart.scales.x.max;
		if (min == null || max == null) return fallback;
		return [min, max];
	}

	function sameRange(a: [number, number] | null, b: [number, number]): boolean {
		if (!a) return false;
		const tolerance = Math.max(0.5, (b[1] - b[0]) * 1e-6);
		return Math.abs(a[0] - b[0]) <= tolerance && Math.abs(a[1] - b[1]) <= tolerance;
	}

	/**
	 * Decimate the visible window for drawing. The budget is ~2 points per CSS
	 * pixel: enough to keep every spike, small enough to redraw at frame rate.
	 */
	function plotFor(range: [number, number]): PlotSeries {
		const width = Math.max(320, host?.clientWidth || 0);
		// ~1 drawn point per CSS pixel (min+max per bucket), which keeps spikes
		// while the path stays cheap to rasterize.
		const budget = Math.max(200, Math.round(width / 2));
		if (!plotBuffer || plotBudget < budget) {
			plotBuffer = {
				timestamps: new Float64Array(budget * 2),
				values: new Float64Array(budget * 2)
			};
			plotBudget = budget;
		}
		plotRange = range;
		const plot = decimate(series, range, budget, plotBuffer);
		drawnPoints = plot.drawnPoints;
		return plot;
	}

	function applyRange(next: [number, number], reason: RangeReason) {
		if (!chart) return;
		const plot = plotFor(next);
		chart.setData([plot.timestamps, plot.values], false);
		chart.setScale('x', { min: next[0], max: next[1] });
		const full = seriesRange(series);
		const stillFull = isFullRange(next, full);
		zoomed = reason !== 'reset' && !stillFull;
		range = stillFull ? full : next;
		tooltip = null;
		pendingAnnouncement = `${reasonLabel[reason]}: ${formatRangeLabel(next)}.`;
		scheduleSettle(stillFull ? null : next);
	}

	/** Debounce the table/stats recompute and the announcement so interaction frames stay cheap. */
	function scheduleSettle(next: [number, number] | null) {
		if (settleTimer) clearTimeout(settleTimer);
		settleTimer = setTimeout(() => {
			settledRange = next;
			if (pendingAnnouncement) {
				announcement = pendingAnnouncement;
				pendingAnnouncement = '';
			}
		}, 120);
	}

	function zoom(factor: number) {
		applyRange(zoomRange(currentRange(), factor, seriesRange(series)), 'zoom');
	}

	function pan(direction: -1 | 1) {
		applyRange(panRange(currentRange(), direction, seriesRange(series)), 'pan');
	}

	function reset() {
		applyRange(seriesRange(series), 'reset');
	}

	function jumpToLatest() {
		applyRange(latestRange(seriesRange(series)), 'latest');
	}

	function handlePointerMove(event: MouseEvent) {
		if (!chart) return;
		const xs = chart.data[0] as Float64Array;
		const ys = chart.data[1] as Float64Array;
		const index = chart.posToIdx(event.offsetX);
		if (index == null || index < 0 || index >= xs.length) {
			tooltip = null;
			return;
		}
		const rawTime = Number(xs[index]);
		const rawValue = Number(ys[index]);
		const overLeft = chart.over.offsetLeft || 0;
		const overTop = chart.over.offsetTop || 0;
		const width = host?.clientWidth ?? 0;
		const left = Math.min(Math.max(event.offsetX + overLeft, 96), Math.max(96, width - 96));
		tooltip = {
			left,
			top: event.offsetY + overTop,
			time: formatTimestamp(rawTime),
			value: formatValue(rawValue, unit),
			rawTime,
			rawValue
		};
	}

	function handlePointerLeave() {
		tooltip = null;
	}

	function handleWheel(event: WheelEvent) {
		if (!chart) return;
		event.preventDefault();
		const full = seriesRange(series);
		const current = currentRange();
		if (event.shiftKey) {
			pan(event.deltaY > 0 ? 1 : -1);
			return;
		}
		const anchor = chart.posToVal(event.offsetX, 'x');
		const factor = event.deltaY > 0 ? 1.25 : 0.8;
		applyRange(zoomRangeAt(current, factor, anchor, full), 'zoom');
	}

	function handleKeydown(event: KeyboardEvent) {
		if (!chart) return;
		const full = seriesRange(series);
		const current = currentRange();
		switch (event.key) {
			case 'ArrowLeft':
				event.preventDefault();
				applyRange(panRange(current, -1, full), 'pan');
				break;
			case 'ArrowRight':
				event.preventDefault();
				applyRange(panRange(current, 1, full), 'pan');
				break;
			case '+':
			case '=':
				event.preventDefault();
				applyRange(zoomRange(current, 0.6, full), 'zoom');
				break;
			case '-':
			case '_':
				event.preventDefault();
				applyRange(zoomRange(current, 1.6, full), 'zoom');
				break;
			case '0':
			case 'Home':
				event.preventDefault();
				applyRange(full, 'reset');
				break;
			case 'End':
				event.preventDefault();
				applyRange(latestRange(full), 'latest');
				break;
			case 'Escape':
				tooltip = null;
				break;
			default:
				break;
		}
	}

	onMount(() => {
		let disposed = false;
		let observer: ResizeObserver | undefined;

		async function mountChart() {
			try {
				// Dynamic import keeps uPlot out of the initial bundle and out of SSR.
				const { default: UPlot } = await import('uplot');
				if (disposed || !host) return;
				const theme = readChartTheme();
				const width = Math.max(320, host.clientWidth || 0);
				const initial = plotFor(seriesRange(series));
				const started = performance.now();
				const instance = new UPlot(
					buildOptions(width, theme),
					[initial.timestamps, initial.values],
					host
				);
				const elapsed = performance.now() - started;
				chart = instance;
				instances += 1;
				renderMs = Math.round(elapsed * 10) / 10;
				requestAnimationFrame(() => {
					if (disposed) return;
					paintMs = Math.round((performance.now() - started) * 10) / 10;
				});
				status = 'ready';
				instance.over.addEventListener('mousemove', handlePointerMove);
				instance.over.addEventListener('mouseleave', handlePointerLeave);
				instance.over.addEventListener('wheel', handleWheel, { passive: false });
				onready?.(instance);
				if (typeof ResizeObserver !== 'undefined') {
					observer = new ResizeObserver(() => {
						if (!chart || !host) return;
						chart.setSize({ width: Math.max(320, host.clientWidth || 0), height });
					});
					observer.observe(host);
				}
			} catch (error) {
				status = 'error';
				errorMessage = error instanceof Error ? error.message : 'The chart failed to load.';
			}
		}

		void mountChart();

		return () => {
			disposed = true;
			if (settleTimer) clearTimeout(settleTimer);
			observer?.disconnect();
			chart?.destroy();
			chart = undefined;
		};
	});

	$effect(() => {
		// Runs only when the parent bumps `revision`; the plot is reused and the
		// same typed arrays are handed back to uPlot, so nothing is recreated.
		void revision;
		if (!chart) return;
		const instance = chart;
		untrack(() => {
			const started = performance.now();
			// Keep a user zoom while live points arrive; follow the latest point
			// when the chart is showing the full range.
			const window = zoomed ? currentRange() : seriesRange(series);
			const plot = plotFor(window);
			instance.setData([plot.timestamps, plot.values], !zoomed);
			if (!zoomed) settledRange = null;
			const elapsed = performance.now() - started;
			updateMs = Math.round(elapsed * 100) / 100;
			updates += 1;
			onupdate?.(elapsed);
		});
	});
</script>

<section
	class="border-border bg-surface-1 overflow-hidden rounded-lg border"
	aria-labelledby="{chartId}-title"
>
	<header class="border-border flex h-11 items-center justify-between gap-3 border-b px-3">
		<div class="flex min-w-0 items-baseline gap-2">
			<h3 id="{chartId}-title" class="truncate text-sm font-semibold">{label}</h3>
			<span class="text-foreground-muted font-mono text-[11px]">
				{points.toLocaleString('en-US')} points · {unit}
			</span>
			{#if live}
				<span
					class="border-success/40 bg-success/10 text-success inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 font-mono text-[11px] leading-none"
					data-chart-live="true"
				>
					<span class="bg-success h-1.5 w-1.5 animate-pulse rounded-full" aria-hidden="true"></span>
					Live 1 Hz
				</span>
			{/if}
		</div>
		<div class="flex items-center gap-1" role="group" aria-label="Chart controls">
			<button
				type="button"
				class="border-border-strong bg-surface-1 hover:bg-surface-3 focus-visible:outline-primary rounded-md border px-2 py-1 font-mono text-xs focus-visible:outline-2 focus-visible:outline-offset-2"
				aria-label="Pan earlier"
				title="Pan earlier (←)"
				onclick={() => pan(-1)}>←</button
			>
			<button
				type="button"
				class="border-border-strong bg-surface-1 hover:bg-surface-3 focus-visible:outline-primary rounded-md border px-2 py-1 font-mono text-xs focus-visible:outline-2 focus-visible:outline-offset-2"
				aria-label="Pan later"
				title="Pan later (→)"
				onclick={() => pan(1)}>→</button
			>
			<button
				type="button"
				class="border-border-strong bg-surface-1 hover:bg-surface-3 focus-visible:outline-primary rounded-md border px-2 py-1 font-mono text-xs focus-visible:outline-2 focus-visible:outline-offset-2"
				aria-label="Zoom out"
				title="Zoom out (-)"
				onclick={() => zoom(1.6)}>−</button
			>
			<button
				type="button"
				class="border-border-strong bg-surface-1 hover:bg-surface-3 focus-visible:outline-primary rounded-md border px-2 py-1 font-mono text-xs focus-visible:outline-2 focus-visible:outline-offset-2"
				aria-label="Zoom in"
				title="Zoom in (+)"
				onclick={() => zoom(0.6)}>+</button
			>
			<button
				type="button"
				class="border-border-strong bg-surface-1 hover:bg-surface-3 focus-visible:outline-primary rounded-md border px-2 py-1 text-xs focus-visible:outline-2 focus-visible:outline-offset-2"
				aria-label="Jump to the latest 10 percent"
				title="Jump to latest (End)"
				onclick={jumpToLatest}>Latest</button
			>
			<button
				type="button"
				class="border-border-strong bg-surface-1 hover:bg-surface-3 focus-visible:outline-primary rounded-md border px-2 py-1 text-xs focus-visible:outline-2 focus-visible:outline-offset-2"
				aria-label="Reset to the full range"
				title="Full range (0)"
				onclick={reset}>Reset</button
			>
		</div>
	</header>

	<div class="px-3 pt-2">
		<!-- The plot area is a custom keyboard-operable widget: it owns a focus
		     ring, arrow/zoom keys and a described help text, so it is focusable
		     even though the wrapper itself is not a native control. -->
		<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<div
			class="focus-visible:outline-primary relative focus-visible:outline-2 focus-visible:outline-offset-2"
			style:height="{height}px"
			role="group"
			aria-label="{label} time series, {points.toLocaleString('en-US')} points"
			aria-describedby="{chartId}-help"
			tabindex="0"
			bind:this={host}
			data-chart-ready={status === 'ready' ? 'true' : 'false'}
			data-chart-render-ms={renderMs ?? ''}
			data-chart-paint-ms={paintMs ?? ''}
			data-chart-updates={updates}
			data-chart-instances={instances}
			data-chart-drawn={drawnPoints}
			data-chart-range-min={visibleRange[0]}
			data-chart-range-max={visibleRange[1]}
			data-chart-data-min={fullRange[0]}
			data-chart-data-max={fullRange[1]}
			data-chart-zoomed={zoomed ? 'true' : 'false'}
			onkeydown={handleKeydown}
		>
			{#if status === 'loading'}
				<div class="absolute inset-0 space-y-2" aria-hidden="true">
					<div class="bg-surface-3 h-3 w-1/3 animate-pulse rounded-sm"></div>
					<div class="bg-surface-3 h-2/3 w-full animate-pulse rounded-sm"></div>
				</div>
			{:else if status === 'error'}
				<p class="text-danger text-xs" role="alert">Chart failed to load: {errorMessage}</p>
			{/if}
			{#if tooltip}
				<div
					class="border-border-strong bg-surface-2 pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-[130%] rounded-md border px-2 py-1 shadow-lg"
					style:left="{tooltip.left}px"
					style:top="{tooltip.top}px"
					data-chart-tooltip="true"
					data-chart-tooltip-time={tooltip.rawTime}
					data-chart-tooltip-value={tooltip.rawValue}
					aria-hidden="true"
				>
					<p class="text-foreground-muted font-mono text-[10px]">{tooltip.time}</p>
					<p class="font-mono text-xs font-semibold">{tooltip.value}</p>
				</div>
			{/if}
		</div>
		<p id="{chartId}-help" class="sr-only">
			Use the left and right arrow keys to pan, plus and minus to zoom, zero to reset and End to
			jump to the latest points. Drag on the plot to select a range, scroll to zoom and shift-scroll
			to pan. The table below lists the same data in buckets.
		</p>
		<p class="sr-only" role="status" aria-live="polite">{announcement}</p>
		<p class="text-foreground-muted mt-1 font-mono text-[11px]">
			{formatRangeLabel(tableRange)} · min {formatValue(stats.min, unit)} · max {formatValue(
				stats.max,
				unit
			)} · avg {formatValue(stats.avg, unit)} · latest {formatValue(stats.last, unit)}
		</p>
	</div>

	<footer
		class="text-foreground-subtle flex flex-wrap items-center gap-x-4 gap-y-1 px-3 py-2 font-mono text-[11px]"
	>
		<span data-chart-render-label="true">First paint {paintMs ?? '—'} ms</span>
		<span>Plot {renderMs ?? '—'} ms</span>
		<span>Drawn {drawnPoints.toLocaleString('en-US')} of {points.toLocaleString('en-US')}</span>
		<span>Last update {updateMs ?? '—'} ms</span>
		<span>Plot instances {instances}</span>
		<span>Times in {zone}</span>
	</footer>

	<details class="border-border border-t px-3 py-2">
		<summary class="text-foreground-muted cursor-pointer text-xs"
			>View as table (12 buckets)</summary
		>
		<div class="mt-2 overflow-x-auto">
			<table class="w-full border-collapse text-left font-mono text-[11px]">
				<caption class="text-foreground-muted mb-1 text-left text-[11px]">
					Downsampled summary of the visible range: {points.toLocaleString('en-US')} points aggregated
					into {buckets.length} buckets. Values are {unit}.
				</caption>
				<thead>
					<tr class="border-border border-b">
						<th scope="col" class="py-1 pr-3 font-medium">From</th>
						<th scope="col" class="py-1 pr-3 font-medium">To</th>
						<th scope="col" class="py-1 pr-3 text-right font-medium">Min</th>
						<th scope="col" class="py-1 pr-3 text-right font-medium">Max</th>
						<th scope="col" class="py-1 pr-3 text-right font-medium">Avg</th>
						<th scope="col" class="py-1 text-right font-medium">Last</th>
					</tr>
				</thead>
				<tbody>
					{#each buckets as bucket, index (index)}
						<tr class="border-border/60 border-b">
							<td class="py-1 pr-3">{formatTimestamp(bucket.from)}</td>
							<td class="py-1 pr-3">{formatTimestamp(bucket.to)}</td>
							<td class="py-1 pr-3 text-right"
								>{bucket.min === null ? '—' : formatValue(bucket.min)}</td
							>
							<td class="py-1 pr-3 text-right"
								>{bucket.max === null ? '—' : formatValue(bucket.max)}</td
							>
							<td class="py-1 pr-3 text-right"
								>{bucket.avg === null ? '—' : formatValue(bucket.avg)}</td
							>
							<td class="py-1 text-right"
								>{bucket.last === null ? '—' : formatValue(bucket.last)}</td
							>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</details>
</section>
