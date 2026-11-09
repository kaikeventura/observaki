import { describe, expect, it } from 'vitest';
import { generateLogs } from '#lib/table/logs';
import {
	filterByService,
	filterLogs,
	generateTraces,
	MAX_CHART_POINTS,
	metricSeries,
	severityFromQuery,
	statusFromQuery,
	TRACE_LIMIT
} from './mock';
import { METRICS, rangeById } from './filters';

const END = 1_700_000_000;

describe('metric series', () => {
	it('caps the chart resolution and ends at the sticky range end', () => {
		const series = metricSeries(METRICS[0], '24h', END);
		expect(series.timestamps.length).toBe(MAX_CHART_POINTS);
		expect(series.timestamps[series.timestamps.length - 1]).toBe(END);

		const span = END - series.timestamps[0];
		expect(span).toBeGreaterThan(rangeById('24h').seconds * 0.95);
		expect(span).toBeLessThanOrEqual(rangeById('24h').seconds);
	});

	it('is deterministic and follows the metric shape', () => {
		const first = metricSeries(METRICS[1], '1h', END);
		const second = metricSeries(METRICS[1], '1h', END);
		expect(Array.from(first.values)).toEqual(Array.from(second.values));
		expect(Math.max(...first.values)).toBeLessThan(2);
	});
});

describe('log filtering', () => {
	const logs = generateLogs({ rows: 500, endSeconds: END });

	it('filters by service and keeps all services when asked', () => {
		expect(filterByService(logs, 'all')).toHaveLength(500);
		const checkout = filterByService(logs, 'checkout');
		expect(checkout.length).toBeGreaterThan(0);
		expect(checkout.every((row) => row.service === 'checkout')).toBe(true);
	});

	it('reads the severity from the query', () => {
		expect(severityFromQuery('severity:error')).toEqual(['ERROR']);
		expect(severityFromQuery('severity:error,warn')).toEqual(['ERROR', 'WARN']);
		expect(severityFromQuery('service:checkout')).toEqual([]);

		const errors = filterLogs(logs, 'severity:error');
		expect(errors.length).toBeGreaterThan(0);
		expect(errors.every((row) => row.severity === 'ERROR')).toBe(true);
		expect(filterLogs(logs, 'service:checkout')).toHaveLength(logs.length);
	});
});

describe('trace preview', () => {
	it('is deterministic and newest first', () => {
		const first = generateTraces({ rows: 400, endSeconds: END, range: '1h' });
		const second = generateTraces({ rows: 400, endSeconds: END, range: '1h' });
		expect(first).toEqual(second);
		expect(first.length).toBeGreaterThan(TRACE_LIMIT);
		expect(first[0].startSeconds).toBe(END);
		for (let i = 1; i < first.length; i += 1) {
			expect(first[i].startSeconds).toBeLessThan(first[i - 1].startSeconds);
		}
	});

	it('honours the service and status filters', () => {
		const traces = generateTraces({ rows: 800, service: 'checkout', endSeconds: END });
		expect(traces.length).toBeGreaterThan(0);
		expect(traces.every((trace) => trace.service === 'checkout')).toBe(true);

		const errors = generateTraces({ rows: 800, status: 'error', endSeconds: END });
		expect(errors.length).toBeGreaterThan(0);
		expect(errors.every((trace) => trace.status === 'error')).toBe(true);
	});

	it('maps status from the query text', () => {
		expect(statusFromQuery('status:error')).toBe('error');
		expect(statusFromQuery('STATUS:OK')).toBe('ok');
		expect(statusFromQuery('service:checkout')).toBe('any');
	});
});
