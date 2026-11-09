<script lang="ts">
	import { onMount } from 'svelte';
	import {
		clampColumnWidth,
		DEFAULT_HEADER_HEIGHT,
		DEFAULT_LOG_SORT,
		DEFAULT_LOG_WIDTHS,
		DEFAULT_OVERSCAN,
		DEFAULT_ROW_HEIGHT,
		formatDurationMs,
		formatLogClock,
		formatLogTimestamp,
		LOG_COLUMNS,
		nextSort,
		shortenId,
		sortLogs,
		virtualWindow,
		type LogColumn,
		type LogRow,
		type LogSort,
		type LogSortKey,
		type SortDirection,
		type TableStatus
	} from '#lib/table/logs';
	import StateMessage from './StateMessage.svelte';
	import StatusPill from './StatusPill.svelte';

	let {
		rows = [],
		status = 'ready',
		label = 'Logs',
		caption = 'Log results',
		height = 544,
		onretry,
		emptyTitle = 'No logs match this filter.',
		emptyBody = 'Adjust the filters or widen the time range, then retry. The query is kept.',
		errorTitle,
		errorBody,
		requestId
	}: {
		rows?: LogRow[];
		status?: TableStatus;
		label?: string;
		caption?: string;
		height?: number;
		onretry?: () => void;
		emptyTitle?: string;
		emptyBody?: string;
		errorTitle?: string;
		errorBody?: string;
		requestId?: string;
	} = $props();

	const tableId = $props.id();
	const numberFormat = new Intl.NumberFormat('en-US');
	const ROW_HEIGHT = DEFAULT_ROW_HEIGHT;
	const HEADER_HEIGHT = DEFAULT_HEADER_HEIGHT;

	const effectiveStatus = $derived<TableStatus>(
		status === 'ready' && rows.length === 0 ? 'empty' : status
	);

	let widths = $state<Record<LogSortKey, number>>({ ...DEFAULT_LOG_WIDTHS });
	let sort = $state<LogSort>({ ...DEFAULT_LOG_SORT });
	let resizeCount = $state(0);

	let selectedId = $state<string | null>(null);
	let detailOpen = $state(false);
	let copyNotice = $state('');
	let announcement = $state('');

	let scrollHost = $state<HTMLDivElement>();
	let viewport = $state(0);
	let scrollTop = $state(0);
	let scrollFrame = 0;

	const sorted = $derived(sortLogs(rows, sort));
	const totalWidth = $derived(LOG_COLUMNS.reduce((sum, column) => sum + widths[column.key], 0));
	const selectedIndex = $derived(sorted.findIndex((row) => row.id === selectedId));
	const selectedRow = $derived(selectedIndex >= 0 ? sorted[selectedIndex] : null);

	const effectiveViewport = $derived(viewport || height);
	const bodyViewport = $derived(Math.max(ROW_HEIGHT, effectiveViewport - HEADER_HEIGHT));
	// The sticky header occupies the first HEADER_HEIGHT pixels of the scroller,
	// so the body window starts after it.
	const bodyScrollTop = $derived(Math.max(0, scrollTop - HEADER_HEIGHT));
	const view = $derived(
		virtualWindow(bodyScrollTop, bodyViewport, ROW_HEIGHT, sorted.length, DEFAULT_OVERSCAN)
	);
	const visibleRows = $derived(sorted.slice(view.start, view.end));

	const sortLabels: Record<LogSortKey, string> = {
		timestamp: 'Time',
		severity: 'Level',
		service: 'Service',
		message: 'Message',
		duration: 'Duration',
		trace: 'Trace'
	};

	const severityTone: Record<
		'DEBUG' | 'INFO' | 'WARN' | 'ERROR',
		'neutral' | 'info' | 'warning' | 'danger'
	> = {
		DEBUG: 'neutral',
		INFO: 'info',
		WARN: 'warning',
		ERROR: 'danger'
	};

	function directionWord(direction: SortDirection): string {
		return direction === 'asc' ? 'ascending' : 'descending';
	}

	function ariaSort(key: LogSortKey): 'ascending' | 'descending' | 'none' {
		if (sort.key !== key) return 'none';
		return sort.direction === 'asc' ? 'ascending' : 'descending';
	}

	function sortIndicator(key: LogSortKey): string {
		if (sort.key !== key) return '';
		return sort.direction === 'asc' ? '▲' : '▼';
	}

	function toggleSort(key: LogSortKey) {
		sort = nextSort(sort, key);
		announcement = `Sorted by ${sortLabels[key]}, ${directionWord(sort.direction)}.`;
	}

	function resizeTo(column: LogColumn, width: number) {
		const next = clampColumnWidth(column, width);
		if (next === widths[column.key]) return;
		widths[column.key] = next;
		resizeCount += 1;
		announcement = `${column.label} column is ${next} pixels wide.`;
	}

	function handleResizeKey(event: KeyboardEvent, column: LogColumn) {
		const step = event.shiftKey ? 32 : 8;
		if (event.key === 'ArrowLeft') {
			event.preventDefault();
			resizeTo(column, widths[column.key] - step);
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			resizeTo(column, widths[column.key] + step);
		} else if (event.key === 'Home') {
			event.preventDefault();
			resizeTo(column, column.width);
		}
	}

	function startResize(event: PointerEvent, column: LogColumn) {
		event.preventDefault();
		const startX = event.clientX;
		const startWidth = widths[column.key];
		const handle = event.currentTarget as HTMLElement;
		handle.setPointerCapture?.(event.pointerId);

		function move(moveEvent: PointerEvent) {
			resizeTo(column, startWidth + moveEvent.clientX - startX);
		}
		function stop() {
			window.removeEventListener('pointermove', move);
			window.removeEventListener('pointerup', stop);
			window.removeEventListener('pointercancel', stop);
		}

		window.addEventListener('pointermove', move);
		window.addEventListener('pointerup', stop);
		window.addEventListener('pointercancel', stop);
	}

	function handleScroll(event: Event) {
		const target = event.currentTarget as HTMLDivElement;
		if (scrollFrame) return;
		// Coalesce scroll events to one state update per frame; replacing ~26
		// rows is cheap, but doing it on every wheel tick is not.
		scrollFrame = requestAnimationFrame(() => {
			scrollFrame = 0;
			scrollTop = target.scrollTop;
		});
	}

	function scrollRowIntoView(index: number) {
		if (!scrollHost) return;
		const rowTop = HEADER_HEIGHT + index * ROW_HEIGHT;
		const rowBottom = rowTop + ROW_HEIGHT;
		if (rowTop < scrollHost.scrollTop) {
			scrollHost.scrollTop = rowTop;
		} else if (rowBottom > scrollHost.scrollTop + effectiveViewport) {
			scrollHost.scrollTop = rowBottom - effectiveViewport;
		}
		scrollTop = scrollHost.scrollTop;
	}

	function selectRow(row: LogRow, index: number) {
		selectedId = row.id;
		detailOpen = true;
		announcement = `Selected row ${index + 1} of ${sorted.length}: ${row.severity}, ${row.service}, ${formatLogClock(row.timestamp)}.`;
	}

	function moveSelection(delta: -1 | 1) {
		if (sorted.length === 0) return;
		const current = selectedIndex >= 0 ? selectedIndex : delta > 0 ? -1 : sorted.length;
		const next = Math.min(sorted.length - 1, Math.max(0, current + delta));
		const row = sorted[next];
		selectedId = row.id;
		detailOpen = false;
		scrollRowIntoView(next);
		announcement = `Row ${next + 1} of ${sorted.length}: ${row.severity}, ${row.service}, ${formatLogClock(row.timestamp)}.`;
	}

	function closeDetail() {
		detailOpen = false;
		announcement = 'Detail panel closed.';
		scrollHost?.focus();
	}

	function handleKeydown(event: KeyboardEvent) {
		if (effectiveStatus !== 'ready') return;
		switch (event.key) {
			case 'j':
			case 'ArrowDown':
				event.preventDefault();
				moveSelection(1);
				break;
			case 'k':
			case 'ArrowUp':
				event.preventDefault();
				moveSelection(-1);
				break;
			case 'Enter':
				if (selectedRow) {
					event.preventDefault();
					detailOpen = true;
					announcement = `Detail panel opened for ${selectedRow.id}.`;
				}
				break;
			case 'Escape':
				if (detailOpen) {
					event.preventDefault();
					closeDetail();
				}
				break;
			default:
				break;
		}
	}

	function handleWindowKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && detailOpen) closeDetail();
	}

	async function copyTrace(traceId: string) {
		try {
			if (!navigator.clipboard) throw new Error('clipboard unavailable');
			await navigator.clipboard.writeText(traceId);
			copyNotice = 'Trace ID copied.';
		} catch {
			copyNotice = 'Copy failed; select the value manually.';
		}
	}

	$effect(() => {
		if (effectiveStatus !== 'ready' && detailOpen) detailOpen = false;
	});

	$effect(() => {
		if (detailOpen && !selectedRow) detailOpen = false;
	});

	onMount(() => {
		return () => {
			if (scrollFrame) cancelAnimationFrame(scrollFrame);
		};
	});
</script>

<svelte:window onkeydown={handleWindowKeydown} />

<section
	class="border-border bg-surface-1 overflow-hidden rounded-lg border"
	aria-labelledby="{tableId}-title"
	data-table-component="true"
>
	<header class="border-border flex h-11 items-center justify-between gap-3 border-b px-3">
		<div class="flex min-w-0 items-baseline gap-2">
			<h2 id="{tableId}-title" class="truncate text-sm font-semibold">{label}</h2>
			<span class="text-foreground-muted font-mono text-[11px]">
				{numberFormat.format(rows.length)} rows
			</span>
			<span class="text-foreground-subtle font-mono text-[11px]">
				{effectiveStatus === 'ready' ? 'virtualized' : effectiveStatus}
			</span>
		</div>
		<span class="text-foreground-subtle font-mono text-[11px]">j/k select · Enter opens</span>
	</header>

	{#if effectiveStatus === 'ready'}
		<div class="flex flex-col lg:flex-row">
			<div class="min-w-0 flex-1">
				<!-- The scroller is the keyboard surface for row selection: j/k move,
				     Enter opens and Escape closes the detail panel. -->
				<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
				<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
				<div
					class="focus-visible:outline-primary overflow-auto focus-visible:outline-2 focus-visible:outline-offset-[-2px]"
					style:height="{height}px"
					role="group"
					aria-label="Log results, {numberFormat.format(sorted.length)} rows"
					aria-describedby="{tableId}-help"
					tabindex="0"
					bind:this={scrollHost}
					bind:clientHeight={viewport}
					onscroll={handleScroll}
					onkeydown={handleKeydown}
					data-table-scroll="true"
					data-table-ready="true"
					data-table-total={sorted.length}
					data-table-rendered={view.rendered}
					data-table-start={view.start}
					data-table-end={view.end}
					data-table-scroll-top={scrollTop}
					data-table-sort="{sort.key}:{sort.direction}"
					data-table-selected={selectedId ?? ''}
					data-table-selected-index={selectedIndex}
					data-table-detail={detailOpen ? 'true' : 'false'}
					data-table-row-height={ROW_HEIGHT}
					data-table-resizes={resizeCount}
				>
					<table
						class="border-collapse text-left"
						style="width: max(100%, {totalWidth}px); table-layout: fixed;"
					>
						<caption class="sr-only">
							{caption}. {numberFormat.format(sorted.length)} rows, sorted by {sortLabels[
								sort.key
							]},
							{directionWord(sort.direction)}. Only the visible rows are rendered; the rest is
							reserved height. Press j or k to move the selection, Enter to open a row and Escape to
							close it.
						</caption>
						<colgroup>
							{#each LOG_COLUMNS as column (column.key)}
								<col
									data-column={column.key}
									data-width={widths[column.key]}
									style:width="{widths[column.key]}px"
								/>
							{/each}
						</colgroup>
						<thead>
							<tr>
								{#each LOG_COLUMNS as column (column.key)}
									<th
										scope="col"
										data-column={column.key}
										aria-sort={ariaSort(column.key)}
										class="bg-surface-2 border-border relative border-b p-0"
										style:height="{HEADER_HEIGHT}px"
									>
										<button
											type="button"
											class="text-foreground-muted hover:text-foreground focus-visible:outline-primary flex h-full w-full items-center gap-1 px-2 text-[11px] font-medium tracking-wide uppercase focus-visible:outline-2 focus-visible:outline-offset-[-2px] {column.align ===
											'right'
												? 'justify-end'
												: 'justify-between'}"
											title="Sort by {column.label}"
											onclick={() => toggleSort(column.key)}
										>
											<span class="truncate">{column.label}</span>
											<span class="text-foreground-subtle" aria-hidden="true"
												>{sortIndicator(column.key)}</span
											>
										</button>
										<span
											role="separator"
											aria-orientation="vertical"
											aria-label="Resize {column.label} column"
											aria-valuemin={column.minWidth}
											aria-valuemax={column.maxWidth}
											aria-valuenow={widths[column.key]}
											tabindex="0"
											data-resizer={column.key}
											class="focus-visible:outline-primary absolute top-0 -right-1 z-20 h-full w-2 cursor-col-resize touch-none focus-visible:outline-2 focus-visible:outline-offset-2"
											onkeydown={(event) => handleResizeKey(event, column)}
											onpointerdown={(event) => startResize(event, column)}
										>
											<span
												class="bg-border-strong pointer-events-none absolute top-1 right-1 h-[calc(100%-0.5rem)] w-px"
												aria-hidden="true"
											></span>
										</span>
									</th>
								{/each}
							</tr>
						</thead>
						<tbody>
							<tr aria-hidden="true" data-table-spacer="top" style:height="{view.offsetTop}px">
								<td colspan={LOG_COLUMNS.length} class="p-0" style:line-height="0"></td>
							</tr>
							{#each visibleRows as row, index (row.id)}
								<!-- svelte-ignore a11y_click_events_have_key_events -->
								<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
								<tr
									class="border-border/60 h-10 cursor-pointer border-b {selectedId === row.id
										? 'bg-surface-3'
										: 'hover:bg-surface-2'}"
									aria-current={selectedId === row.id ? 'true' : undefined}
									data-row-id={row.id}
									data-row-index={view.start + index}
									data-row-timestamp={row.timestamp}
									data-row-duration={row.durationMs}
									data-row-severity={row.severity}
									data-row-service={row.service}
									onclick={() => selectRow(row, view.start + index)}
								>
									<td
										class="text-foreground-muted border-l-2 px-2 py-0 font-mono text-[11px] whitespace-nowrap {selectedId ===
										row.id
											? 'border-l-primary'
											: 'border-l-transparent'}"
									>
										{formatLogClock(row.timestamp)}
									</td>
									<td class="px-2 py-0 whitespace-nowrap">
										<StatusPill label={row.severity} tone={severityTone[row.severity]} />
									</td>
									<td class="px-2 py-0 text-xs whitespace-nowrap">{row.service}</td>
									<td class="overflow-hidden px-2 py-0 text-xs">
										<span class="block truncate" title={row.message}>{row.message}</span>
									</td>
									<td
										class="px-2 py-0 text-right font-mono text-[11px] whitespace-nowrap tabular-nums"
									>
										{formatDurationMs(row.durationMs)}
									</td>
									<td
										class="text-foreground-muted overflow-hidden px-2 py-0 font-mono text-[11px] whitespace-nowrap"
									>
										<span class="block truncate" title={row.traceId}
											>{shortenId(row.traceId, 6)}</span
										>
									</td>
								</tr>
							{/each}
							<tr
								aria-hidden="true"
								data-table-spacer="bottom"
								style:height="{view.offsetBottom}px"
							>
								<td colspan={LOG_COLUMNS.length} class="p-0" style:line-height="0"></td>
							</tr>
						</tbody>
					</table>
				</div>
				<p id="{tableId}-help" class="sr-only">
					Use j and k (or the arrow keys) to move the selected row, Enter to open the detail panel
					and Escape to close it. Click a row to select and open it. Drag a column edge or focus the
					resizer and use the arrow keys to resize it. Click a column header to sort; click again to
					reverse.
				</p>
				<p class="sr-only" role="status" aria-live="polite">{announcement}</p>
				<footer
					class="text-foreground-subtle flex flex-wrap items-center gap-x-4 gap-y-1 px-3 py-2 font-mono text-[11px]"
				>
					<span>Visible window + {DEFAULT_OVERSCAN} rows overscan</span>
					<span>Sorted by {sortLabels[sort.key]} {directionWord(sort.direction)}</span>
					<span>Click a row to open its detail</span>
				</footer>
			</div>

			{#if detailOpen && selectedRow}
				<aside
					class="border-border bg-surface-2 overflow-auto border-t lg:w-80 lg:shrink-0 lg:border-t-0 lg:border-l"
					style:max-height="{height}px"
					aria-label="Log detail"
					data-table-detail-panel="true"
				>
					<header
						class="border-border bg-surface-2 sticky top-0 flex items-center justify-between gap-2 border-b px-3 py-2"
					>
						<h3 class="text-sm font-semibold">Log detail</h3>
						<button
							type="button"
							class="border-border-strong bg-surface-1 hover:bg-surface-3 focus-visible:outline-primary rounded-md border px-2 py-1 text-xs focus-visible:outline-2 focus-visible:outline-offset-2"
							aria-label="Close detail panel"
							onclick={closeDetail}>Close</button
						>
					</header>
					<dl class="space-y-3 p-3 text-xs" data-table-detail-id={selectedRow.id}>
						<div>
							<dt class="text-foreground-subtle font-mono text-[10px] uppercase">Time</dt>
							<dd class="font-mono">{formatLogTimestamp(selectedRow.timestamp)}</dd>
						</div>
						<div class="flex flex-wrap items-center gap-2">
							<div>
								<dt class="text-foreground-subtle font-mono text-[10px] uppercase">Level</dt>
								<dd class="mt-1">
									<StatusPill
										label={selectedRow.severity}
										tone={severityTone[selectedRow.severity]}
									/>
								</dd>
							</div>
							<div>
								<dt class="text-foreground-subtle font-mono text-[10px] uppercase">Service</dt>
								<dd class="mt-1">{selectedRow.service}</dd>
							</div>
							<div>
								<dt class="text-foreground-subtle font-mono text-[10px] uppercase">Duration</dt>
								<dd class="mt-1 font-mono">{formatDurationMs(selectedRow.durationMs)}</dd>
							</div>
						</div>
						<div>
							<dt class="text-foreground-subtle font-mono text-[10px] uppercase">Message</dt>
							<dd class="mt-1 break-words">{selectedRow.message}</dd>
						</div>
						<div>
							<dt class="text-foreground-subtle font-mono text-[10px] uppercase">Trace ID</dt>
							<dd class="mt-1 flex items-start gap-2">
								<code class="min-w-0 flex-1 font-mono text-[10px] break-all"
									>{selectedRow.traceId}</code
								>
								<button
									type="button"
									class="border-border-strong bg-surface-1 hover:bg-surface-3 focus-visible:outline-primary shrink-0 rounded-md border px-2 py-1 text-[11px] focus-visible:outline-2 focus-visible:outline-offset-2"
									onclick={() => copyTrace(selectedRow.traceId)}>Copy</button
								>
							</dd>
						</div>
						<div>
							<dt class="text-foreground-subtle font-mono text-[10px] uppercase">Span ID</dt>
							<dd class="mt-1 font-mono text-[10px] break-all">{selectedRow.spanId}</dd>
						</div>
						<div>
							<dt class="text-foreground-subtle font-mono text-[10px] uppercase">Row ID</dt>
							<dd class="mt-1 font-mono text-[10px]">{selectedRow.id}</dd>
						</div>
						<p class="text-foreground-muted" role="status" aria-live="polite">{copyNotice}</p>
					</dl>
				</aside>
			{/if}
		</div>
	{:else}
		<div
			class="flex flex-col justify-center p-4"
			style:min-height="{height + 32}px"
			data-table-state-frame={effectiveStatus}
		>
			<StateMessage
				variant={effectiveStatus === 'error'
					? 'error'
					: effectiveStatus === 'loading'
						? 'loading'
						: 'empty'}
				title={effectiveStatus === 'error'
					? errorTitle
					: effectiveStatus === 'empty'
						? emptyTitle
						: undefined}
				body={effectiveStatus === 'error'
					? errorBody
					: effectiveStatus === 'empty'
						? emptyBody
						: undefined}
				requestId={effectiveStatus === 'error' ? requestId : undefined}
				primaryLabel={effectiveStatus === 'loading' ? undefined : 'Retry query'}
				onprimary={effectiveStatus === 'loading' ? undefined : onretry}
			/>
		</div>
	{/if}
</section>
