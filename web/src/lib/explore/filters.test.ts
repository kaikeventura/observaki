import { describe, expect, it } from 'vitest';
import {
	ALL_SERVICES,
	QUERY_SUGGESTIONS,
	applySuggestion,
	currentToken,
	DEFAULT_EXPLORE_STATE,
	metricForQuery,
	parseExploreSearch,
	queryFitsSignal,
	rangeById,
	signalLabel,
	stepRange,
	suggestionsFor,
	switchSignal,
	toExploreSearch,
	type ExploreState
} from './filters';

const base: ExploreState = {
	signal: 'logs',
	range: '1h',
	service: 'checkout',
	query: 'severity:error'
};

describe('explore search state', () => {
	it('round-trips signal, range, service and query through the URL', () => {
		const search = toExploreSearch(base);
		expect(search).toBe('?signal=logs&range=1h&service=checkout&q=severity%3Aerror');
		expect(parseExploreSearch(search)).toEqual(base);
	});

	it('falls back to defaults for garbage and drops an empty query', () => {
		expect(parseExploreSearch('?signal=nope&range=1y&service=ghost')).toEqual({
			...DEFAULT_EXPLORE_STATE,
			service: ALL_SERVICES
		});
		expect(toExploreSearch({ ...base, query: '   ' })).toBe(
			'?signal=logs&range=1h&service=checkout'
		);
	});

	it('steps the range presets without leaving the catalog', () => {
		expect(stepRange('5m', -1)).toBe('5m');
		expect(stepRange('5m', 1)).toBe('15m');
		expect(stepRange('24h', 1)).toBe('24h');
		expect(rangeById('24h').seconds).toBe(86_400);
	});
});

describe('signal switching', () => {
	it('keeps the time range and the service filter and says so', () => {
		const result = switchSignal(base, 'traces');
		expect(result.state.range).toBe('1h');
		expect(result.state.service).toBe('checkout');
		expect(result.state.query).toBe('');
		expect(result.announcement).toContain('Traces.');
		expect(result.announcement).toContain('the 1h time range and the checkout service');
		expect(result.announcement).toContain('Dropped the query "severity:error"');
	});

	it('keeps a query that applies to the new signal', () => {
		const result = switchSignal({ ...base, query: 'service:checkout' }, 'traces');
		expect(result.state.query).toBe('service:checkout');
		expect(result.announcement).toContain('Kept the query.');
		expect(result.announcement).not.toContain('Dropped');
	});

	it('announces all services when no service filter is set', () => {
		const result = switchSignal({ ...base, service: ALL_SERVICES }, 'metrics');
		expect(result.announcement).toContain('all services');
		expect(result.state.signal).toBe('metrics');
	});

	it('is a no-op on the same signal', () => {
		expect(switchSignal(base, 'logs')).toEqual({ state: base, announcement: '' });
	});
});

describe('query vocabulary', () => {
	it('accepts only fields and functions that belong to the signal', () => {
		expect(queryFitsSignal('logs', 'severity:error service:checkout')).toBe(true);
		expect(queryFitsSignal('logs', 'rate(http_requests_total)')).toBe(false);
		expect(queryFitsSignal('traces', 'status:error duration:>500ms')).toBe(true);
		expect(queryFitsSignal('metrics', 'sum(rate(http_requests_total[5m]))')).toBe(true);
		expect(queryFitsSignal('metrics', 'severity:error')).toBe(false);
		expect(queryFitsSignal('metrics', '')).toBe(true);
	});

	it('prefix-matches suggestions for the trailing token', () => {
		const logs = suggestionsFor('logs', 'sev');
		expect(logs.length).toBeGreaterThan(0);
		expect(logs.every((suggestion) => suggestion.value.startsWith('sev'))).toBe(true);

		const metrics = suggestionsFor('metrics', 'http_');
		expect(metrics.map((suggestion) => suggestion.value)).toEqual(
			expect.arrayContaining(['http_requests_total', 'http_request_duration_seconds'])
		);

		expect(suggestionsFor('logs', 'nothing-matches')).toEqual([]);
	});

	it('caps the suggestion list and replaces only the trailing token', () => {
		expect(suggestionsFor('logs', '', 3)).toHaveLength(3);
		const suggestion = QUERY_SUGGESTIONS.logs.find((entry) => entry.value === 'severity:error');
		expect(suggestion).toBeDefined();
		expect(applySuggestion('service:checkout sev', suggestion!)).toBe(
			'service:checkout severity:error'
		);
		expect(currentToken('service:checkout sev')).toBe('sev');
	});

	it('resolves the metric named in the query', () => {
		expect(metricForQuery('rate(kafka_consumer_lag)').name).toBe('kafka_consumer_lag');
		expect(metricForQuery('').name).toBe('http_requests_total');
		expect(signalLabel('traces')).toBe('Traces');
	});
});
