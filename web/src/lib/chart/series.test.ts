import { describe, expect, it } from 'vitest';
import {
	appendPoint,
	clampRange,
	decimate,
	generateSeries,
	isFullRange,
	latestRange,
	lowerBound,
	panRange,
	sampleValue,
	seriesRange,
	summarize,
	visibleStats,
	zoomRange,
	zoomRangeAt,
	type PlotBuffer
} from './series';

const END = 1_700_000_000;

function small() {
	return generateSeries({ points: 1_000, endSeconds: END });
}

describe('generateSeries', () => {
	it('is deterministic for the same end time and seed-free shape', () => {
		const first = generateSeries({ points: 10, endSeconds: END });
		const second = generateSeries({ points: 10, endSeconds: END });

		expect(Array.from(first.timestamps)).toEqual(Array.from(second.timestamps));
		expect(Array.from(first.values)).toEqual(Array.from(second.values));
	});

	it('produces the requested number of points at 1 s resolution ending at endSeconds', () => {
		const series = generateSeries({ points: 1_000, endSeconds: END });

		expect(series.timestamps).toHaveLength(1_000);
		expect(series.timestamps[999]).toBe(END);
		expect(series.timestamps[998]).toBe(END - 1);
		expect(series.timestamps[0]).toBe(END - 999);
		expect(series.stepSeconds).toBe(1);
	});

	it('generates 100k points by default', () => {
		expect(generateSeries({ endSeconds: END }).timestamps).toHaveLength(100_000);
	});

	it('keeps values finite and non-negative', () => {
		const series = generateSeries({ points: 5_000, endSeconds: END });
		for (const value of series.values) {
			expect(Number.isFinite(value)).toBe(true);
			expect(value).toBeGreaterThanOrEqual(0);
		}
	});
});

describe('sampleValue', () => {
	it('continues the same curve for live points', () => {
		expect(sampleValue(END)).toBe(sampleValue(END));
		expect(sampleValue(END)).not.toBe(sampleValue(END + 1));
	});
});

describe('appendPoint', () => {
	it('slides a full window so memory stays bounded', () => {
		const series = small();
		const capacity = series.timestamps.length;
		const first = series.timestamps[0];

		expect(appendPoint(series, END + 1, 42)).toBe(true);

		expect(series.timestamps).toHaveLength(capacity);
		expect(series.timestamps[0]).toBe(first + 1);
		expect(series.timestamps[capacity - 1]).toBe(END + 1);
		expect(series.values[capacity - 1]).toBe(42);
	});

	it('rejects out-of-order and non-finite points', () => {
		const series = small();

		expect(appendPoint(series, END, 1)).toBe(false);
		expect(appendPoint(series, END - 10, 1)).toBe(false);
		expect(appendPoint(series, Number.NaN, 1)).toBe(false);
		expect(appendPoint(series, END + 1, Number.POSITIVE_INFINITY)).toBe(false);
		expect(series.timestamps[series.timestamps.length - 1]).toBe(END);
	});
});

describe('range math', () => {
	const dataRange: [number, number] = [0, 1_000];

	it('zooms around an anchor and keeps the anchor in place', () => {
		const next = zoomRangeAt([0, 1_000], 0.5, 250, dataRange);

		expect(next[1] - next[0]).toBeCloseTo(500, 5);
		expect((250 - next[0]) / (next[1] - next[0])).toBeCloseTo(0.25, 2);
	});

	it('never zooms past 20 s or outside the data', () => {
		const tooSmall = zoomRange([0, 1_000], 0.0001, dataRange);
		expect(tooSmall[1] - tooSmall[0]).toBeCloseTo(20, 5);
		expect(tooSmall[0]).toBeGreaterThanOrEqual(0);
		expect(tooSmall[1]).toBeLessThanOrEqual(1_000);

		const outside = zoomRangeAt([0, 100], 2, 0, dataRange);
		expect(outside[0]).toBeGreaterThanOrEqual(0);
		expect(outside[1]).toBeLessThanOrEqual(1_000);
	});

	it('pans by a fraction of the visible span and clamps at the edges', () => {
		expect(panRange([400, 600], 1, dataRange)).toEqual([450, 650]);
		expect(panRange([0, 100], -1, dataRange)).toEqual([0, 100]);
		expect(panRange([900, 1_000], 1, dataRange)).toEqual([900, 1_000]);
	});

	it('keeps the span when clamping', () => {
		const clamped = clampRange([-100, 100], dataRange);
		expect(clamped[0]).toBe(0);
		expect(clamped[1] - clamped[0]).toBeCloseTo(200, 5);
	});

	it('jumps to the last 10% of the data', () => {
		expect(latestRange(dataRange)).toEqual([900, 1_000]);
	});

	it('detects the full range with a small tolerance', () => {
		expect(isFullRange([0, 1_000], dataRange)).toBe(true);
		expect(isFullRange([0, 999.5], dataRange)).toBe(true);
		expect(isFullRange([0, 800], dataRange)).toBe(false);
	});
});

describe('aggregation', () => {
	it('finds the first index at or after a timestamp', () => {
		const { timestamps } = small();
		expect(lowerBound(timestamps, timestamps[0] - 1)).toBe(0);
		expect(lowerBound(timestamps, timestamps[500])).toBe(500);
		expect(lowerBound(timestamps, timestamps[999] + 1)).toBe(1_000);
	});

	it('summarizes the visible range into min, max, avg and last', () => {
		const series = {
			timestamps: new Float64Array([0, 1, 2, 3]),
			values: new Float64Array([10, 30, 20, 40]),
			stepSeconds: 1
		};

		expect(visibleStats(series, [0, 3])).toEqual({ count: 4, min: 10, max: 40, avg: 25, last: 40 });
		expect(visibleStats(series, [1, 2])).toEqual({ count: 2, min: 20, max: 30, avg: 25, last: 20 });
		expect(visibleStats(series, [10, 20]).count).toBe(0);
	});

	it('buckets the range for the accessible table', () => {
		const series = {
			timestamps: new Float64Array([0, 1, 2, 3]),
			values: new Float64Array([10, 30, 20, 40]),
			stepSeconds: 1
		};

		const buckets = summarize(series, 2, [0, 3]);

		expect(buckets).toHaveLength(2);
		expect(buckets[0].count).toBe(2);
		expect(buckets[0].min).toBe(10);
		expect(buckets[0].max).toBe(30);
		expect(buckets[0].avg).toBe(20);
		expect(buckets[1].min).toBe(20);
		expect(buckets[1].last).toBe(40);
	});

	it('leaves empty buckets empty instead of inventing zeroes', () => {
		const series = {
			timestamps: new Float64Array([0, 1]),
			values: new Float64Array([10, 20]),
			stepSeconds: 1
		};

		const buckets = summarize(series, 4, [0, 3]);

		expect(buckets[3].count).toBe(0);
		expect(buckets[3].min).toBeNull();
		expect(buckets[3].avg).toBeNull();
	});

	it('reports the series range', () => {
		expect(seriesRange(small())).toEqual([END - 999, END]);
		expect(
			seriesRange({ timestamps: new Float64Array(), values: new Float64Array(), stepSeconds: 1 })
		).toEqual([0, 1]);
	});
});

describe('decimate', () => {
	function buffer(size: number): PlotBuffer {
		return { timestamps: new Float64Array(size), values: new Float64Array(size) };
	}

	it('copies every point when the window fits the budget', () => {
		const series = small();
		const plot = decimate(series, [END - 99, END], 1_000, buffer(2_000));

		expect(plot.sourcePoints).toBe(100);
		expect(plot.drawnPoints).toBe(100);
		expect(Array.from(plot.timestamps)).toEqual(Array.from(series.timestamps.subarray(900, 1_000)));
		expect(Array.from(plot.values)).toEqual(Array.from(series.values.subarray(900, 1_000)));
	});

	it('keeps the minimum and maximum of the window when it decimates', () => {
		const series = small();
		const range = seriesRange(series);
		const plot = decimate(series, range, 100, buffer(200));

		expect(plot.sourcePoints).toBe(1_000);
		expect(plot.drawnPoints).toBeLessThanOrEqual(200);
		expect(plot.drawnPoints).toBeGreaterThan(0);

		const min = Math.min(...series.values);
		const max = Math.max(...series.values);
		expect(Math.min(...plot.values)).toBe(min);
		expect(Math.max(...plot.values)).toBe(max);
	});

	it('keeps timestamps sorted so uPlot can draw the path', () => {
		const series = small();
		const plot = decimate(series, seriesRange(series), 50, buffer(100));

		for (let i = 1; i < plot.drawnPoints; i += 1) {
			expect(plot.timestamps[i]).toBeGreaterThan(plot.timestamps[i - 1]);
		}
	});

	it('spans the exact visible window so auto-scaling does not inset the axis', () => {
		const series = small();
		const range: [number, number] = [END - 900, END];
		const plot = decimate(series, range, 50, buffer(100));

		expect(plot.timestamps[0]).toBe(range[0]);
		expect(plot.timestamps[plot.drawnPoints - 1]).toBe(range[1]);
	});

	it('returns an empty plot for an empty range', () => {
		const plot = decimate(small(), [END + 10, END + 20], 100, buffer(200));

		expect(plot.sourcePoints).toBe(0);
		expect(plot.drawnPoints).toBe(0);
	});
});
