import { describe, expect, it } from 'vitest';
import {
	formatAxisTime,
	formatAxisValue,
	formatDuration,
	formatRangeLabel,
	formatTimestamp,
	formatValue,
	timezoneName
} from './format';
import { readChartTheme, withAlpha } from './theme';

const TS = 1_700_000_000; // 2023-11-14T22:13:20Z

describe('format', () => {
	it('formats values with thousands separators and an optional unit', () => {
		expect(formatValue(1_203.456, 'req/s')).toBe('1,203.46 req/s');
		expect(formatValue(1_203)).toBe('1,203');
		expect(formatValue(0, 'req/s')).toBe('0 req/s');
	});

	it('formats axis values compactly', () => {
		expect(formatAxisValue(1_200)).toBe('1.2K');
		expect(formatAxisValue(18_400)).toBe('18.4K');
	});

	it('formats full timestamps as YYYY-MM-DD HH:mm:ss', () => {
		expect(formatTimestamp(TS)).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/);
	});

	it('changes axis precision with the visible span', () => {
		expect(formatAxisTime(TS, 60)).toMatch(/^\d{2}:\d{2}:\d{2}$/);
		expect(formatAxisTime(TS, 3_600)).toMatch(/^\d{2}:\d{2}$/);
		expect(formatAxisTime(TS, 30 * 86_400)).toMatch(/^[A-Za-z]{3} \d{2}$/);
	});

	it('formats durations at human scale', () => {
		expect(formatDuration(45)).toBe('45 s');
		expect(formatDuration(600)).toBe('10 min');
		expect(formatDuration(7_200)).toBe('2 h');
		expect(formatDuration(3 * 86_400)).toBe('3 d');
	});

	it('describes a range with both ends and the span', () => {
		const label = formatRangeLabel([TS, TS + 3_600]);
		expect(label).toContain('→');
		expect(label).toContain('(1 h)');
	});

	it('reports the browser timezone', () => {
		expect(timezoneName()).toBeTruthy();
	});
});

describe('theme', () => {
	it('turns hex tokens into rgba colors', () => {
		expect(withAlpha('#22d3ee', 0.16)).toBe('rgba(34, 211, 238, 0.16)');
		expect(withAlpha('#fff', 1)).toBe('rgba(255, 255, 255, 1)');
		expect(withAlpha('#22d3ee', 2)).toBe('rgba(34, 211, 238, 1)');
	});

	it('passes non-hex colors through untouched', () => {
		expect(withAlpha('oklch(0.7 0.1 200)', 0.2)).toBe('oklch(0.7 0.1 200)');
	});

	it('returns the design-token fallbacks without a document', () => {
		const theme = readChartTheme();
		expect(theme.primary).toBeTruthy();
		expect(theme.grid).toBeTruthy();
	});
});
