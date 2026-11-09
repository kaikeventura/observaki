/**
 * Deterministic mock results for the Phase 0 Explore screen (Story 6.4).
 *
 * The Query Service arrives in Phase 1 (Story 8.3). Until then the screen
 * renders the same shaped data the real one will: a metric series, a
 * virtualized log result and a trace list, all filtered by the sticky time
 * range, the service filter and (where meaningful) the query text.
 */
import { generateSeries, type TimeSeries } from '#lib/chart/series';
import type { LogRow } from '#lib/table/logs';
import { ALL_SERVICES, rangeById, type MetricDef, type TimeRangeId } from '#lib/explore/filters';

/** Cap the chart at a readable resolution: one point every few seconds. */
export const MAX_CHART_POINTS = 1_800;

/** The trace list is a preview, not a paginated result. */
export const TRACE_LIMIT = 50;

export function metricSeries(
	metric: MetricDef,
	range: TimeRangeId,
	endSeconds: number
): TimeSeries {
	const seconds = rangeById(range).seconds;
	const points = Math.min(MAX_CHART_POINTS, Math.max(2, Math.round(seconds / 2)));
	const stepSeconds = Math.max(1, Math.round(seconds / (points - 1)));
	return generateSeries({
		points,
		stepSeconds,
		endSeconds,
		baseline: metric.baseline,
		amplitude: metric.amplitude,
		noise: metric.noise
	});
}

export function filterByService<T extends { service: string }>(
	rows: readonly T[],
	service: string
): T[] {
	if (service === ALL_SERVICES) return [...rows];
	return rows.filter((row) => row.service === service);
}

/** `severity:error` / `severity:error,info` narrows the log result. */
export function severityFromQuery(query: string): string[] {
	const match = /severity:([a-z,|]+)/i.exec(query);
	if (!match) return [];
	return match[1]
		.toLowerCase()
		.split(/[,|]/)
		.map((value) => value.trim())
		.filter(Boolean)
		.map((value) => value.toUpperCase());
}

export function filterLogs(rows: readonly LogRow[], query: string): LogRow[] {
	const severities = severityFromQuery(query);
	if (severities.length === 0) return [...rows];
	return rows.filter((row) => severities.includes(row.severity));
}

export type TraceStatus = 'ok' | 'error';

export type TraceRow = {
	id: string;
	rootSpan: string;
	service: string;
	status: TraceStatus;
	durationMs: number;
	spans: number;
	startSeconds: number;
};

const TRACE_ROOTS = [
	'POST /checkout',
	'GET /cart',
	'POST /payments/authorize',
	'GET /search',
	'POST /auth/token',
	'PUT /inventory/reserve',
	'GET /gateway/health'
];

const TRACE_SERVICES = [
	'checkout',
	'payments',
	'cart',
	'search',
	'auth',
	'inventory',
	'notifications',
	'gateway'
];

function hash01(n: number): number {
	let x = Math.imul(n ^ 0x9e3779b9, 0x85ebca6b);
	x = Math.imul(x ^ (x >>> 13), 0xc2b2ae35);
	x ^= x >>> 16;
	return (x >>> 0) / 4_294_967_296;
}

function hex(value: number, length: number): string {
	return Math.floor(value).toString(16).padStart(length, '0').slice(-length);
}

/**
 * Deterministic trace preview: newest first, spread across the sticky range,
 * with roughly one in six requests failing.
 */
export function generateTraces(
	options: {
		rows?: number;
		service?: string;
		range?: TimeRangeId;
		endSeconds?: number;
		status?: TraceStatus | 'any';
	} = {}
): TraceRow[] {
	const total = Math.max(0, Math.floor(options.rows ?? TRACE_LIMIT * 4));
	const rangeSeconds = rangeById(options.range ?? '1h').seconds;
	const endSeconds = options.endSeconds ?? 1_700_000_000;
	const service = options.service && options.service !== ALL_SERVICES ? options.service : undefined;
	const rows: TraceRow[] = [];

	for (let i = 0; i < total; i += 1) {
		const pickedService = TRACE_SERVICES[Math.floor(hash01(i * 13 + 3) * TRACE_SERVICES.length)];
		const status: TraceStatus = hash01(i * 17 + 5) > 0.84 ? 'error' : 'ok';
		if (service && pickedService !== service) continue;
		if (options.status && options.status !== 'any' && status !== options.status) continue;

		const base = status === 'error' ? 1_900 : 180;
		const jitter = hash01(i * 23 + 7);
		rows.push({
			id: `${hex(hash01(i * 29 + 1) * 4_294_967_296, 8)}${hex(hash01(i * 29 + 2) * 4_294_967_296, 8)}`,
			rootSpan: TRACE_ROOTS[Math.floor(hash01(i * 31 + 9) * TRACE_ROOTS.length)],
			service: pickedService,
			status,
			durationMs: Math.round(base + jitter * base * 3),
			spans: 1 + Math.floor(hash01(i * 37 + 11) * 24),
			startSeconds: endSeconds - (i * rangeSeconds) / Math.max(1, total)
		});
	}

	return rows;
}

export function statusFromQuery(query: string): TraceStatus | 'any' {
	if (/status:error/i.test(query)) return 'error';
	if (/status:ok/i.test(query)) return 'ok';
	return 'any';
}
