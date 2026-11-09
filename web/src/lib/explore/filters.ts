/**
 * Explore state for the Phase 0 mock (Story 6.4). Everything here is pure so
 * the signal/time-range/service persistence and the query-bar autocomplete can
 * be tested without a browser.
 *
 * The state mirrors what a real investigation keeps: the signal tab, the
 * sticky time range, the service filter and the query text. It is encoded in
 * the URL (EXPERIENCE.md) so a copied link reproduces the view.
 */
import { LOG_SERVICES } from '#lib/table/logs';

export type Signal = 'metrics' | 'logs' | 'traces';

export const SIGNALS: readonly { id: Signal; label: string; noun: string }[] = [
	{ id: 'metrics', label: 'Metrics', noun: 'metrics' },
	{ id: 'logs', label: 'Logs', noun: 'logs' },
	{ id: 'traces', label: 'Traces', noun: 'traces' }
];

export function isSignal(value: unknown): value is Signal {
	return SIGNALS.some((signal) => signal.id === value);
}

export function signalLabel(signal: Signal): string {
	return SIGNALS.find((entry) => entry.id === signal)?.label ?? signal;
}

export function signalNoun(signal: Signal): string {
	return SIGNALS.find((entry) => entry.id === signal)?.noun ?? signal;
}

export type TimeRangeId = '5m' | '15m' | '1h' | '6h' | '24h';

export type TimeRange = { id: TimeRangeId; label: string; seconds: number };

/** Sticky range presets from DESIGN.md (5m, 15m, 1h, 6h, 24h, 7d, 30d). */
export const TIME_RANGES: readonly TimeRange[] = [
	{ id: '5m', label: '5m', seconds: 300 },
	{ id: '15m', label: '15m', seconds: 900 },
	{ id: '1h', label: '1h', seconds: 3_600 },
	{ id: '6h', label: '6h', seconds: 21_600 },
	{ id: '24h', label: '24h', seconds: 86_400 }
];

export const DEFAULT_RANGE: TimeRangeId = '1h';

export function isTimeRange(value: unknown): value is TimeRangeId {
	return TIME_RANGES.some((range) => range.id === value);
}

export function rangeById(id: TimeRangeId): TimeRange {
	return TIME_RANGES.find((range) => range.id === id) ?? TIME_RANGES[2];
}

/** `[` / `]` step the preset without opening a picker (EXPERIENCE.md). */
export function stepRange(id: TimeRangeId, direction: -1 | 1): TimeRangeId {
	const index = TIME_RANGES.findIndex((range) => range.id === id);
	const next = Math.min(TIME_RANGES.length - 1, Math.max(0, index + direction));
	return TIME_RANGES[next].id;
}

export const ALL_SERVICES = 'all';

export const SERVICE_OPTIONS: readonly string[] = [ALL_SERVICES, ...LOG_SERVICES];

export type ExploreState = {
	signal: Signal;
	range: TimeRangeId;
	service: string;
	query: string;
};

export const DEFAULT_EXPLORE_STATE: ExploreState = {
	signal: 'logs',
	range: DEFAULT_RANGE,
	service: ALL_SERVICES,
	query: ''
};

const MAX_QUERY_LENGTH = 256;

function normalizeService(value: string | null): string {
	if (!value) return ALL_SERVICES;
	return SERVICE_OPTIONS.includes(value) ? value : ALL_SERVICES;
}

/**
 * Parse the query string without trusting it: unknown signals/ranges/services
 * fall back to the defaults instead of breaking the page.
 */
export function parseExploreSearch(search: string): ExploreState {
	const params = new URLSearchParams(search.startsWith('?') ? search.slice(1) : search);
	const signal = params.get('signal');
	const range = params.get('range');
	const query = (params.get('q') ?? '').trim().slice(0, MAX_QUERY_LENGTH);

	return {
		signal: isSignal(signal) ? signal : DEFAULT_EXPLORE_STATE.signal,
		range: isTimeRange(range) ? range : DEFAULT_EXPLORE_STATE.range,
		service: normalizeService(params.get('service')),
		query
	};
}

/** Serialize the state so the URL is reproducible (signal, range, service, q). */
export function toExploreSearch(state: ExploreState): string {
	const params = new URLSearchParams();
	params.set('signal', state.signal);
	params.set('range', state.range);
	params.set('service', state.service);
	if (state.query.trim()) params.set('q', state.query.trim());
	return `?${params.toString()}`;
}

export type SuggestionKind = 'metric' | 'field' | 'value' | 'function';

export type Suggestion = {
	/** Full token inserted into the query bar. */
	value: string;
	kind: SuggestionKind;
	detail: string;
};

export type MetricDef = {
	name: string;
	unit: string;
	baseline: number;
	amplitude: number;
	noise: number;
	detail: string;
};

/** Mock metric catalog; the first entry is the default Explore chart. */
export const METRICS: readonly MetricDef[] = [
	{
		name: 'http_requests_total',
		unit: 'req/s',
		baseline: 1_200,
		amplitude: 420,
		noise: 55,
		detail: 'Request rate by service'
	},
	{
		name: 'http_request_duration_seconds',
		unit: 's',
		baseline: 0.24,
		amplitude: 0.08,
		noise: 0.02,
		detail: 'Request latency histogram'
	},
	{
		name: 'process_cpu_seconds_total',
		unit: 'cores',
		baseline: 0.62,
		amplitude: 0.18,
		noise: 0.05,
		detail: 'Process CPU by pod'
	},
	{
		name: 'kafka_consumer_lag',
		unit: 'msgs',
		baseline: 320,
		amplitude: 120,
		noise: 90,
		detail: 'Consumer lag for the ingest topic'
	},
	{
		name: 'otelcol_exporter_queue_size',
		unit: 'items',
		baseline: 2_400,
		amplitude: 900,
		noise: 320,
		detail: 'Collector export queue depth'
	}
];

const METRIC_FUNCTIONS = ['rate(', 'sum(', 'avg(', 'histogram_quantile('];
const METRIC_LABELS = ['service=', 'status=', 'method=', 'route='];

const LOG_FIELDS = ['service:', 'severity:', 'body:', 'trace_id:', 'span_id:'];
const LOG_VALUES = [
	`severity:error`,
	`severity:warn`,
	`severity:info`,
	...LOG_SERVICES.map((service) => `service:${service}`)
];

const TRACE_FIELDS = ['service:', 'status:', 'duration:', 'span.name:', 'span.kind:', 'trace_id:'];
const TRACE_VALUES = [
	'status:error',
	'status:ok',
	'duration:>500ms',
	'duration:>2s',
	'span.kind:server',
	...LOG_SERVICES.map((service) => `service:${service}`)
];

function metricSuggestions(): Suggestion[] {
	return [
		...METRICS.map((metric) => ({
			value: metric.name,
			kind: 'metric' as const,
			detail: metric.detail
		})),
		...METRIC_FUNCTIONS.map((value) => ({
			value,
			kind: 'function' as const,
			detail: 'MetricsQL function'
		})),
		...METRIC_LABELS.map((value) => ({
			value,
			kind: 'field' as const,
			detail: 'Label matcher'
		}))
	];
}

function fieldSuggestions(fields: readonly string[], values: readonly string[]): Suggestion[] {
	return [
		...fields.map((value) => ({
			value,
			kind: 'field' as const,
			detail: 'Filter field'
		})),
		...values.map((value) => ({
			value,
			kind: 'value' as const,
			detail: 'Mock value'
		}))
	];
}

export const QUERY_SUGGESTIONS: Record<Signal, readonly Suggestion[]> = {
	metrics: metricSuggestions(),
	logs: fieldSuggestions(LOG_FIELDS, LOG_VALUES),
	traces: fieldSuggestions(TRACE_FIELDS, TRACE_VALUES)
};

const VOCABULARY: Record<Signal, ReadonlySet<string>> = {
	metrics: new Set([
		...METRICS.map((metric) => metric.name),
		...METRIC_FUNCTIONS.map((fn) => fn.slice(0, -1)),
		...METRIC_LABELS.map((label) => label.slice(0, -1))
	]),
	logs: new Set([
		...LOG_FIELDS.map((field) => field.slice(0, -1)),
		...LOG_VALUES.map((value) => value.split(':')[0])
	]),
	traces: new Set([
		...TRACE_FIELDS.map((field) => field.slice(0, -1)),
		...TRACE_VALUES.map((value) => value.split(':')[0])
	])
};

const FIELD_TOKEN = /([a-z_][a-z0-9_.]*)\s*[:(]/gi;

/** Fields/functions referenced by a query, lowercased. */
export function queryFields(query: string): string[] {
	const fields: string[] = [];
	for (const match of query.matchAll(FIELD_TOKEN)) fields.push(match[1].toLowerCase());
	return fields;
}

/**
 * A query is valid for a signal when every field/function it references exists
 * in that signal's vocabulary. That is what lets the mock decide whether the
 * query survives a tab switch instead of silently applying a log filter to
 * metrics.
 */
export function queryFitsSignal(signal: Signal, query: string): boolean {
	if (!query.trim()) return true;
	const vocabulary = VOCABULARY[signal];
	return queryFields(query).every((field) => vocabulary.has(field));
}

export type SignalSwitch = { state: ExploreState; announcement: string };

/**
 * Switching signals keeps the time range and the service filter, keeps the
 * query when it is semantically valid and reports exactly what was kept or
 * dropped (EXPERIENCE.md: never silently drop filters).
 */
export function switchSignal(state: ExploreState, next: Signal): SignalSwitch {
	if (state.signal === next) return { state, announcement: '' };

	const kept = [
		`the ${rangeById(state.range).label} time range`,
		state.service === ALL_SERVICES ? 'all services' : `the ${state.service} service`
	];
	const notes = [`${signalLabel(next)}. Kept ${kept.join(' and ')}.`];
	let query = state.query;

	if (query.trim() && !queryFitsSignal(next, query)) {
		notes.push(
			`Dropped the query "${query.trim()}" because it does not apply to ${signalNoun(next)}.`
		);
		query = '';
	} else if (query.trim()) {
		notes.push('Kept the query.');
	}

	return {
		state: { ...state, signal: next, query },
		announcement: notes.join(' ')
	};
}

/** The trailing token the user is typing (used to prefix-match suggestions). */
export function currentToken(query: string): string {
	const match = /(\S*)$/.exec(query);
	return match ? match[1] : '';
}

/**
 * Autocomplete candidates for the current token. Empty token shows the
 * catalog head so the field is discoverable without typing.
 */
export function suggestionsFor(signal: Signal, query: string, limit = 8): Suggestion[] {
	const token = currentToken(query).toLowerCase();
	const matches = QUERY_SUGGESTIONS[signal].filter((suggestion) =>
		suggestion.value.toLowerCase().startsWith(token)
	);
	return matches.slice(0, limit);
}

/** Replace the trailing token with the chosen suggestion. */
export function applySuggestion(query: string, suggestion: Suggestion): string {
	const token = currentToken(query);
	return `${query.slice(0, query.length - token.length)}${suggestion.value}`;
}

/** The metric referenced by the query, falling back to the catalog head. */
export function metricForQuery(query: string): MetricDef {
	const lower = query.toLowerCase();
	return METRICS.find((metric) => lower.includes(metric.name)) ?? METRICS[0];
}
