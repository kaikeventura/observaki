/**
 * Deterministic log data and window math for the Phase 0 virtualized table
 * spike (Story 6.3). Everything here is pure so sorting, column resizing and
 * the visible-row window can be tested without a browser layout.
 *
 * Timestamps are epoch **seconds** (fractional, so milliseconds survive) to
 * match the chart spike and the OTLP data model.
 */

/** 10k rows is the acceptance budget for Story 6.3. */
export const DEFAULT_LOG_ROWS = 10_000;
export const DEFAULT_ROW_HEIGHT = 40;
export const DEFAULT_HEADER_HEIGHT = 40;
export const DEFAULT_OVERSCAN = 6;

export const LOG_SEVERITIES = ['DEBUG', 'INFO', 'WARN', 'ERROR'] as const;
export type LogSeverity = (typeof LOG_SEVERITIES)[number];

export type LogRow = {
	id: string;
	timestamp: number;
	severity: LogSeverity;
	service: string;
	message: string;
	durationMs: number;
	traceId: string;
	spanId: string;
};

export type LogSortKey = 'timestamp' | 'severity' | 'service' | 'message' | 'duration' | 'trace';
export type SortDirection = 'asc' | 'desc';
export type LogSort = { key: LogSortKey; direction: SortDirection };
export type TableStatus = 'loading' | 'ready' | 'empty' | 'error';

export type LogColumn = {
	key: LogSortKey;
	label: string;
	width: number;
	minWidth: number;
	maxWidth: number;
	align?: 'left' | 'right';
};

export const LOG_COLUMNS: readonly LogColumn[] = [
	{ key: 'timestamp', label: 'Time', width: 116, minWidth: 96, maxWidth: 200 },
	{ key: 'severity', label: 'Level', width: 96, minWidth: 80, maxWidth: 140 },
	{ key: 'service', label: 'Service', width: 150, minWidth: 100, maxWidth: 320 },
	{ key: 'message', label: 'Message', width: 480, minWidth: 220, maxWidth: 920 },
	{
		key: 'duration',
		label: 'Duration',
		width: 100,
		minWidth: 84,
		maxWidth: 180,
		align: 'right'
	},
	{ key: 'trace', label: 'Trace', width: 150, minWidth: 110, maxWidth: 260 }
];

export const DEFAULT_LOG_SORT: LogSort = { key: 'timestamp', direction: 'desc' };

export const DEFAULT_LOG_WIDTHS = Object.fromEntries(
	LOG_COLUMNS.map((column) => [column.key, column.width])
) as Record<LogSortKey, number>;

const SEVERITY_RANK: Record<LogSeverity, number> = {
	DEBUG: 0,
	INFO: 1,
	WARN: 2,
	ERROR: 3
};

const SERVICES = [
	'checkout',
	'payments',
	'cart',
	'search',
	'auth',
	'inventory',
	'notifications',
	'gateway'
] as const;

// Deterministic weights: most rows are INFO, enough WARN/ERROR to make the
// severity filter and the colored left border visible in the spike.
const LEVEL_WEIGHTS: LogSeverity[] = [
	'DEBUG',
	'INFO',
	'INFO',
	'INFO',
	'INFO',
	'INFO',
	'WARN',
	'WARN',
	'ERROR'
];

/** Small deterministic PRNG so the spike data is identical across runs. */
function hash01(n: number): number {
	let x = Math.imul(n ^ 0x9e3779b9, 0x85ebca6b);
	x = Math.imul(x ^ (x >>> 13), 0xc2b2ae35);
	x ^= x >>> 16;
	return (x >>> 0) / 4_294_967_296;
}

function pick<T>(values: readonly T[], n: number): T {
	return values[Math.min(values.length - 1, Math.floor(hash01(n) * values.length))];
}

function hex(value: number, length: number): string {
	return value.toString(16).padStart(length, '0').slice(-length);
}

function messageFor(severity: LogSeverity, service: string, n: number): string {
	const number = 100 + Math.floor(hash01(n * 7 + 3) * 9_000);
	switch (severity) {
		case 'DEBUG':
			return pick(
				[
					`span exported to the collector in ${number % 90} ms`,
					`config reloaded (${number % 40} keys, generation ${number % 9})`,
					`cache lookup hit for tenant-a/${service} key ${number}`
				],
				n * 11
			);
		case 'INFO':
			return pick(
				[
					`GET /api/v1/${service} 200 in ${number % 400} ms`,
					`request completed for ${service} in ${number % 900} ms`,
					`processed ${number} events from the ${service} queue`,
					`${service} cache hit ratio ${80 + (number % 19)}%`
				],
				n * 13
			);
		case 'WARN':
			return pick(
				[
					`upstream 503 from ${service}, retry ${1 + (number % 5)} of 5`,
					`slow query on ${service} took ${300 + (number % 2_400)} ms`,
					`queue depth ${number} above the warn threshold`,
					`${service} connection pool at ${85 + (number % 14)}%`
				],
				n * 17
			);
		case 'ERROR':
			return pick(
				[
					`upstream timeout after ${1_000 + (number % 4_000)} ms`,
					`failed to write ${service} batch: connection reset by peer`,
					`payment authorization declined (code ${number % 90})`,
					`panic recovered in ${service} handler, request aborted`
				],
				n * 19
			);
	}
}

/**
 * Generate a deterministic, newest-first log set. The default span is roughly
 * six hours at 10k rows, which is the kind of range an Explore query returns.
 */
export function generateLogs(options: { rows?: number; endSeconds?: number } = {}): LogRow[] {
	const total = Math.max(0, Math.floor(options.rows ?? DEFAULT_LOG_ROWS));
	const endSeconds = options.endSeconds ?? Date.now() / 1000;
	const rows: LogRow[] = new Array(total);

	for (let i = 0; i < total; i += 1) {
		const severity = pick(LEVEL_WEIGHTS, i * 3 + 1);
		const service = pick(SERVICES, i * 5 + 2);
		const jitter = hash01(i * 23 + 7);
		const baseDuration = severity === 'ERROR' ? 1_500 : severity === 'WARN' ? 700 : 120;
		const traceId = [
			hex(Math.floor(hash01(i * 29 + 1) * 4_294_967_296), 8),
			hex(Math.floor(hash01(i * 29 + 2) * 4_294_967_296), 8),
			hex(Math.floor(hash01(i * 29 + 3) * 4_294_967_296), 8),
			hex(Math.floor(hash01(i * 29 + 4) * 4_294_967_296), 8)
		].join('');

		rows[i] = {
			id: `log-${String(i + 1).padStart(6, '0')}`,
			// ~6 hours across 10k rows, with sub-second jitter.
			timestamp: endSeconds - i * 2.16 - jitter * 0.9,
			severity,
			service,
			message: messageFor(severity, service, i),
			durationMs: Math.round(baseDuration + jitter * baseDuration * 2),
			traceId,
			spanId: hex(Math.floor(hash01(i * 31 + 5) * 4_294_967_296), 8) + hex(i + 1, 8)
		};
	}

	return rows;
}

function compareStrings(a: string, b: string): number {
	if (a < b) return -1;
	if (a > b) return 1;
	return 0;
}

export function compareLogRows(a: LogRow, b: LogRow, key: LogSortKey): number {
	switch (key) {
		case 'timestamp':
			return a.timestamp - b.timestamp;
		case 'severity':
			return SEVERITY_RANK[a.severity] - SEVERITY_RANK[b.severity];
		case 'service':
			return compareStrings(a.service, b.service);
		case 'message':
			return compareStrings(a.message, b.message);
		case 'duration':
			return a.durationMs - b.durationMs;
		case 'trace':
			return compareStrings(a.traceId, b.traceId);
	}
}

/**
 * Return a sorted copy. The original order is preserved so the parent can keep
 * the source data immutable; ties fall back to the row id so the order is
 * stable across runs.
 */
export function sortLogs(rows: readonly LogRow[], sort: LogSort): LogRow[] {
	const factor = sort.direction === 'asc' ? 1 : -1;
	return [...rows].sort(
		(a, b) => factor * compareLogRows(a, b, sort.key) || compareStrings(a.id, b.id)
	);
}

/** First click on a column sorts ascending; repeated clicks toggle. */
export function nextSort(current: LogSort, key: LogSortKey): LogSort {
	if (current.key === key) {
		return { key, direction: current.direction === 'asc' ? 'desc' : 'asc' };
	}
	return { key, direction: key === 'timestamp' ? 'desc' : 'asc' };
}

export type VirtualWindow = {
	start: number;
	end: number;
	offsetTop: number;
	offsetBottom: number;
	totalHeight: number;
	rendered: number;
};

/**
 * Compute the rows that intersect the scroll viewport plus an overscan. The
 * table renders only this slice and reserves the rest with spacer rows, so the
 * DOM stays proportional to the viewport, not to the result set.
 */
export function virtualWindow(
	scrollTop: number,
	viewportHeight: number,
	rowHeight: number,
	totalRows: number,
	overscan = DEFAULT_OVERSCAN
): VirtualWindow {
	const safeRow = Math.max(1, Math.floor(rowHeight));
	const safeViewport = Math.max(safeRow, Math.floor(viewportHeight));
	const rows = Math.max(0, Math.floor(totalRows));
	const maxScroll = Math.max(0, rows * safeRow - safeViewport);
	const top = Math.min(Math.max(0, scrollTop), maxScroll);
	const firstVisible = Math.floor(top / safeRow);
	const visibleCount = Math.ceil(safeViewport / safeRow);
	const start = Math.max(0, firstVisible - Math.max(0, Math.floor(overscan)));
	const end = Math.min(rows, firstVisible + visibleCount + Math.max(0, Math.floor(overscan)));

	return {
		start,
		end,
		offsetTop: start * safeRow,
		offsetBottom: (rows - end) * safeRow,
		totalHeight: rows * safeRow,
		rendered: Math.max(0, end - start)
	};
}

export function clampColumnWidth(column: LogColumn, width: number): number {
	if (!Number.isFinite(width)) return column.width;
	return Math.min(column.maxWidth, Math.max(column.minWidth, Math.round(width)));
}

function pad(value: number, length = 2): string {
	return String(value).padStart(length, '0');
}

/** `14:03:21.042` for the table cell; the full date lives in the detail panel. */
export function formatLogClock(seconds: number): string {
	const date = new Date(seconds * 1000);
	return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}.${pad(
		date.getMilliseconds(),
		3
	)}`;
}

/** `2026-11-08 14:03:21.042` for the detail panel and the accessible summary. */
export function formatLogTimestamp(seconds: number): string {
	const date = new Date(seconds * 1000);
	const day = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
	return `${day} ${formatLogClock(seconds)}`;
}

export function formatDurationMs(ms: number): string {
	if (ms >= 10_000) return `${(ms / 1000).toFixed(1)} s`;
	if (ms >= 1_000) return `${(ms / 1000).toFixed(2)} s`;
	return `${Math.round(ms)} ms`;
}

/** Middle-truncated id for narrow cells; the full value stays in the panel. */
export function shortenId(id: string, keep = 6): string {
	if (id.length <= keep * 2 + 1) return id;
	return `${id.slice(0, keep)}…${id.slice(-keep)}`;
}
