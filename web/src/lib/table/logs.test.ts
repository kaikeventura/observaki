import { describe, expect, it } from 'vitest';
import {
	clampColumnWidth,
	DEFAULT_LOG_SORT,
	DEFAULT_LOG_ROWS,
	formatDurationMs,
	formatLogClock,
	formatLogTimestamp,
	generateLogs,
	LOG_COLUMNS,
	nextSort,
	shortenId,
	sortLogs,
	virtualWindow,
	type LogRow
} from './logs';

const END = 1_700_000_000;

function row(overrides: Partial<LogRow> & Pick<LogRow, 'id' | 'timestamp'>): LogRow {
	return {
		severity: 'INFO',
		service: 'checkout',
		message: 'request completed',
		durationMs: 10,
		traceId: 'a'.repeat(32),
		spanId: 'b'.repeat(16),
		...overrides
	};
}

describe('generateLogs', () => {
	it('generates a deterministic newest-first log set', () => {
		const first = generateLogs({ rows: 1_000, endSeconds: END });
		const second = generateLogs({ rows: 1_000, endSeconds: END });

		expect(first).toEqual(second);
		expect(first).toHaveLength(1_000);
		expect(new Set(first.map((entry) => entry.id)).size).toBe(1_000);
		expect(first[0].timestamp).toBeGreaterThan(first[999].timestamp);
		expect(first[0].timestamp).toBeLessThanOrEqual(END);
		expect(first[0].id).toBe('log-000001');
		expect(first[999].id).toBe('log-001000');
	});

	it('fills 10k rows by default with valid OTLP-shaped fields', () => {
		const rows = generateLogs({ endSeconds: END });

		expect(rows).toHaveLength(DEFAULT_LOG_ROWS);
		for (const entry of rows.slice(0, 200)) {
			expect(['DEBUG', 'INFO', 'WARN', 'ERROR']).toContain(entry.severity);
			expect(entry.service).toBeTruthy();
			expect(entry.message.length).toBeGreaterThan(8);
			expect(entry.durationMs).toBeGreaterThan(0);
			expect(entry.traceId).toMatch(/^[0-9a-f]{32}$/);
			expect(entry.spanId).toMatch(/^[0-9a-f]{16}$/);
		}
		expect(new Set(rows.map((entry) => entry.severity)).size).toBe(4);
	});

	it('handles an empty result without inventing rows', () => {
		expect(generateLogs({ rows: 0 })).toEqual([]);
	});
});

describe('sortLogs', () => {
	it('sorts by duration in both directions without mutating the source', () => {
		const rows = generateLogs({ rows: 500, endSeconds: END });
		const original = rows.map((entry) => entry.id);

		const asc = sortLogs(rows, { key: 'duration', direction: 'asc' });
		const desc = sortLogs(rows, { key: 'duration', direction: 'desc' });

		for (let i = 1; i < asc.length; i += 1) {
			expect(asc[i].durationMs).toBeGreaterThanOrEqual(asc[i - 1].durationMs);
			expect(desc[i].durationMs).toBeLessThanOrEqual(desc[i - 1].durationMs);
		}
		expect(rows.map((entry) => entry.id)).toEqual(original);
	});

	it('orders severity by rank, not alphabetically', () => {
		const rows = [
			row({ id: 'log-3', timestamp: 3, severity: 'ERROR' }),
			row({ id: 'log-1', timestamp: 1, severity: 'DEBUG' }),
			row({ id: 'log-2', timestamp: 2, severity: 'WARN' })
		];

		const sorted = sortLogs(rows, { key: 'severity', direction: 'asc' });

		expect(sorted.map((entry) => entry.severity)).toEqual(['DEBUG', 'WARN', 'ERROR']);
	});

	it('uses the row id as a stable tie-breaker', () => {
		const rows = [
			row({ id: 'log-b', timestamp: 1, durationMs: 10 }),
			row({ id: 'log-a', timestamp: 2, durationMs: 10 }),
			row({ id: 'log-c', timestamp: 3, durationMs: 10 })
		];

		expect(sortLogs(rows, { key: 'duration', direction: 'desc' }).map((entry) => entry.id)).toEqual(
			['log-a', 'log-b', 'log-c']
		);
	});
});

describe('nextSort', () => {
	it('starts a new column ascending and toggles the current one', () => {
		expect(nextSort(DEFAULT_LOG_SORT, 'duration')).toEqual({ key: 'duration', direction: 'asc' });
		expect(nextSort({ key: 'duration', direction: 'asc' }, 'duration')).toEqual({
			key: 'duration',
			direction: 'desc'
		});
		expect(nextSort({ key: 'duration', direction: 'desc' }, 'timestamp')).toEqual({
			key: 'timestamp',
			direction: 'desc'
		});
	});
});

describe('virtualWindow', () => {
	it('renders only the visible slice plus overscan at the top', () => {
		const window = virtualWindow(0, 544, 40, 10_000);

		expect(window.start).toBe(0);
		expect(window.end).toBe(20); // 14 visible rows + 6 overscan
		expect(window.offsetTop).toBe(0);
		expect(window.offsetBottom).toBe((10_000 - 20) * 40);
		expect(window.totalHeight).toBe(400_000);
		expect(window.rendered).toBeLessThan(40);
	});

	it('moves the window with the scroll position and clamps at the end', () => {
		const middle = virtualWindow(4_000, 544, 40, 10_000);
		expect(middle.start).toBe(94);
		expect(middle.end).toBeGreaterThan(100);
		expect(middle.offsetTop).toBe(94 * 40);
		expect(middle.rendered).toBeLessThan(40);

		const end = virtualWindow(Number.POSITIVE_INFINITY, 544, 40, 10_000);
		expect(end.end).toBe(10_000);
		expect(end.offsetBottom).toBe(0);
		expect(end.rendered).toBeLessThan(40);
	});

	it('returns an empty window for an empty result set', () => {
		expect(virtualWindow(0, 544, 40, 0)).toMatchObject({
			start: 0,
			end: 0,
			offsetTop: 0,
			offsetBottom: 0,
			totalHeight: 0,
			rendered: 0
		});
	});
});

describe('columns and formatting', () => {
	it('clamps column widths to the declared bounds', () => {
		const service = LOG_COLUMNS.find((column) => column.key === 'service')!;
		expect(clampColumnWidth(service, 10)).toBe(100);
		expect(clampColumnWidth(service, 200)).toBe(200);
		expect(clampColumnWidth(service, 5_000)).toBe(320);
		expect(clampColumnWidth(service, Number.NaN)).toBe(service.width);
	});

	it('formats clock, full timestamp, duration and short ids', () => {
		expect(formatLogClock(END)).toMatch(/^\d{2}:\d{2}:\d{2}\.\d{3}$/);
		expect(formatLogTimestamp(END)).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}$/);
		expect(formatDurationMs(42)).toBe('42 ms');
		expect(formatDurationMs(1_250)).toBe('1.25 s');
		expect(formatDurationMs(12_500)).toBe('12.5 s');
		expect(shortenId('0123456789abcdef0123456789abcdef')).toBe('012345…abcdef');
		expect(shortenId('short')).toBe('short');
	});
});
