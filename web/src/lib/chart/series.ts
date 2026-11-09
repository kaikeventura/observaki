/**
 * Deterministic time-series data and range math for the Phase 0 chart spike
 * (Story 6.2). Everything here is pure so the render budget and the range
 * behaviour can be tested without a canvas.
 *
 * X values are epoch **seconds** because that is what uPlot's `time` scale
 * expects; Y values are metric values in the series unit.
 */

/** 100k points is the acceptance budget for Story 6.2. */
export const DEFAULT_POINTS = 100_000;
export const DEFAULT_STEP_SECONDS = 1;

export type TimeSeries = {
	timestamps: Float64Array;
	values: Float64Array;
	stepSeconds: number;
};

export type SeriesShape = {
	baseline: number;
	amplitude: number;
	noise: number;
};

export type SeriesOptions = Partial<SeriesShape> & {
	points?: number;
	stepSeconds?: number;
	/** Epoch seconds of the last point. Defaults to now. */
	endSeconds?: number;
};

export type VisibleStats = {
	count: number;
	min: number;
	max: number;
	avg: number;
	last: number;
};

export type Bucket = {
	from: number;
	to: number;
	count: number;
	min: number | null;
	max: number | null;
	avg: number | null;
	last: number | null;
};

const DEFAULT_SHAPE: SeriesShape = { baseline: 1_200, amplitude: 420, noise: 55 };

/** Small deterministic PRNG so the spike data is identical across runs. */
function hash01(n: number): number {
	let x = Math.imul(n ^ 0x9e3779b9, 0x85ebca6b);
	x = Math.imul(x ^ (x >>> 13), 0xc2b2ae35);
	x ^= x >>> 16;
	return (x >>> 0) / 4_294_967_296;
}

/**
 * Sample the synthetic metric at a given epoch second: a 6-hour cycle, a daily
 * cycle and deterministic noise. Live points continue the same curve.
 */
export function sampleValue(timestamp: number, shape: Partial<SeriesShape> = {}): number {
	const { baseline, amplitude, noise } = { ...DEFAULT_SHAPE, ...shape };
	const sixHours = Math.sin((timestamp / 21_600) * Math.PI * 2);
	const daily = Math.sin((timestamp / 86_400) * Math.PI * 2 - 1.1);
	const jitter = (hash01(timestamp) - 0.5) * 2 * noise;
	const value = baseline + amplitude * (0.6 * sixHours + 0.4 * daily) + jitter;
	return Math.round(Math.max(0, value) * 100) / 100;
}

export function generateSeries(options: SeriesOptions = {}): TimeSeries {
	const points = Math.max(1, Math.floor(options.points ?? DEFAULT_POINTS));
	const stepSeconds = options.stepSeconds ?? DEFAULT_STEP_SECONDS;
	const endSeconds = Math.floor(options.endSeconds ?? Date.now() / 1000);
	const shape: SeriesShape = {
		baseline: options.baseline ?? DEFAULT_SHAPE.baseline,
		amplitude: options.amplitude ?? DEFAULT_SHAPE.amplitude,
		noise: options.noise ?? DEFAULT_SHAPE.noise
	};
	const timestamps = new Float64Array(points);
	const values = new Float64Array(points);
	const start = endSeconds - (points - 1) * stepSeconds;

	for (let i = 0; i < points; i += 1) {
		const timestamp = start + i * stepSeconds;
		timestamps[i] = timestamp;
		values[i] = sampleValue(timestamp, shape);
	}

	return { timestamps, values, stepSeconds };
}

/**
 * Append a point to a fixed-size window. When the window is full the oldest
 * point is dropped, so the typed arrays never grow and memory stays flat.
 * Returns false for out-of-order timestamps (uPlot needs a sorted x array).
 */
export function appendPoint(series: TimeSeries, timestamp: number, value: number): boolean {
	const { timestamps, values } = series;
	const length = timestamps.length;
	if (length === 0) return false;
	if (!Number.isFinite(timestamp) || !Number.isFinite(value)) return false;
	if (timestamp <= timestamps[length - 1]) return false;

	if (length > 1) {
		timestamps.copyWithin(0, 1);
		values.copyWithin(0, 1);
	}
	timestamps[length - 1] = timestamp;
	values[length - 1] = value;
	return true;
}

export function seriesRange(series: TimeSeries): [number, number] {
	const { timestamps } = series;
	if (timestamps.length === 0) return [0, 1];
	const last = timestamps.length - 1;
	return [timestamps[0], timestamps[last]];
}

/** Index of the first timestamp >= value. */
export function lowerBound(timestamps: Float64Array, value: number): number {
	let low = 0;
	let high = timestamps.length;
	while (low < high) {
		const mid = (low + high) >>> 1;
		if (timestamps[mid] < value) low = mid + 1;
		else high = mid;
	}
	return low;
}

export function visibleStats(
	series: TimeSeries,
	range: [number, number] = seriesRange(series)
): VisibleStats {
	const { timestamps, values } = series;
	const start = lowerBound(timestamps, range[0]);
	let count = 0;
	let min = Number.POSITIVE_INFINITY;
	let max = Number.NEGATIVE_INFINITY;
	let sum = 0;
	let last = 0;

	for (let i = start; i < timestamps.length; i += 1) {
		if (timestamps[i] > range[1]) break;
		const value = values[i];
		if (value < min) min = value;
		if (value > max) max = value;
		sum += value;
		last = value;
		count += 1;
	}

	if (count === 0) return { count: 0, min: 0, max: 0, avg: 0, last: 0 };
	return { count, min, max, avg: sum / count, last };
}

/**
 * Bucket the visible range for the accessible table. The chart renders 100k
 * points; the table shows one row per bucket with min/max/avg/last so a screen
 * reader gets the shape without 100k rows in the DOM.
 */
export function summarize(
	series: TimeSeries,
	buckets = 12,
	range: [number, number] = seriesRange(series)
): Bucket[] {
	const { timestamps, values } = series;
	const count = Math.max(1, Math.floor(buckets));
	const from = range[0];
	const to = Math.max(range[0] + 1e-6, range[1]);
	const width = (to - from) / count;
	const result: Bucket[] = Array.from({ length: count }, (_, index) => ({
		from: from + index * width,
		to: from + (index + 1) * width,
		count: 0,
		min: null,
		max: null,
		avg: null,
		last: null
	}));

	const start = lowerBound(timestamps, from);
	for (let i = start; i < timestamps.length; i += 1) {
		const timestamp = timestamps[i];
		if (timestamp > to) break;
		const index = Math.min(count - 1, Math.floor((timestamp - from) / width));
		const bucket = result[index];
		const value = values[i];
		bucket.count += 1;
		bucket.min = bucket.min === null ? value : Math.min(bucket.min, value);
		bucket.max = bucket.max === null ? value : Math.max(bucket.max, value);
		bucket.avg = bucket.avg === null ? value : bucket.avg + value;
		bucket.last = value;
	}

	for (const bucket of result) {
		if (bucket.avg !== null) bucket.avg = bucket.avg / bucket.count;
	}
	return result;
}

export function clampRange(range: [number, number], dataRange: [number, number]): [number, number] {
	const dataSpan = Math.max(1e-6, dataRange[1] - dataRange[0]);
	const minSpan = Math.min(dataSpan, 20);
	const span = Math.min(dataSpan, Math.max(minSpan, range[1] - range[0]));
	const center = (range[0] + range[1]) / 2;
	let min = center - span / 2;
	let max = min + span;

	if (min < dataRange[0]) {
		min = dataRange[0];
		max = min + span;
	}
	if (max > dataRange[1]) {
		max = dataRange[1];
		min = max - span;
	}
	if (min < dataRange[0]) min = dataRange[0];
	return [min, Math.max(max, min + Math.min(minSpan, dataSpan))];
}

/** Zoom by `factor` (< 1 zooms in) keeping `anchor` at the same ratio. */
export function zoomRangeAt(
	range: [number, number],
	factor: number,
	anchor: number,
	dataRange: [number, number]
): [number, number] {
	const span = Math.max(1e-6, range[1] - range[0]);
	const ratio = Math.min(1, Math.max(0, (anchor - range[0]) / span));
	const nextSpan = Math.min(dataRange[1] - dataRange[0], Math.max(20, span * factor));
	const min = anchor - ratio * nextSpan;
	return clampRange([min, min + nextSpan], dataRange);
}

export function zoomRange(
	range: [number, number],
	factor: number,
	dataRange: [number, number]
): [number, number] {
	return zoomRangeAt(range, factor, (range[0] + range[1]) / 2, dataRange);
}

/** Pan by a fraction of the visible span; direction -1 is earlier, 1 later. */
export function panRange(
	range: [number, number],
	direction: -1 | 1,
	dataRange: [number, number],
	fraction = 0.25
): [number, number] {
	const span = range[1] - range[0];
	const shift = span * fraction * direction;
	return clampRange([range[0] + shift, range[1] + shift], dataRange);
}

/** Last 10% of the data, used by the "jump to latest" action. */
export function latestRange(dataRange: [number, number]): [number, number] {
	const span = Math.max(20, (dataRange[1] - dataRange[0]) * 0.1);
	return [dataRange[1] - span, dataRange[1]];
}

export function isFullRange(range: [number, number], dataRange: [number, number]): boolean {
	const tolerance = Math.max(1, (dataRange[1] - dataRange[0]) * 0.001);
	return (
		Math.abs(range[0] - dataRange[0]) <= tolerance && Math.abs(range[1] - dataRange[1]) <= tolerance
	);
}

export type PlotBuffer = { timestamps: Float64Array; values: Float64Array };

export type PlotSeries = {
	timestamps: Float64Array;
	values: Float64Array;
	sourcePoints: number;
	drawnPoints: number;
};

/**
 * Min/max decimation for drawing. A 100k-point series over a ~1000 px panel has
 * ~100 samples per pixel; drawing all of them costs frame time and adds
 * sub-pixel noise. Each bucket keeps the lowest and highest sample (in time
 * order), so spikes survive and the path stays faithful. The result is written
 * into `buffer` (which must hold `2 * maxPoints` values) and returned as views,
 * so interaction frames allocate nothing.
 */
export function decimate(
	series: TimeSeries,
	range: [number, number],
	maxPoints: number,
	buffer: PlotBuffer
): PlotSeries {
	const { timestamps, values } = series;
	const start = lowerBound(timestamps, range[0]);
	let end = lowerBound(timestamps, range[1]);
	if (end < timestamps.length && timestamps[end] <= range[1]) end += 1;
	const sourcePoints = Math.max(0, end - start);
	const capacity = Math.min(
		buffer.timestamps.length,
		buffer.values.length,
		Math.max(2, Math.floor(maxPoints) * 2)
	);

	if (sourcePoints === 0) {
		return {
			timestamps: buffer.timestamps.subarray(0, 0),
			values: buffer.values.subarray(0, 0),
			sourcePoints: 0,
			drawnPoints: 0
		};
	}

	if (sourcePoints <= capacity) {
		for (let i = 0; i < sourcePoints; i += 1) {
			buffer.timestamps[i] = timestamps[start + i];
			buffer.values[i] = values[start + i];
		}
		return {
			timestamps: buffer.timestamps.subarray(0, sourcePoints),
			values: buffer.values.subarray(0, sourcePoints),
			sourcePoints,
			drawnPoints: sourcePoints
		};
	}

	const buckets = Math.max(1, Math.floor(capacity / 2));
	const width = sourcePoints / buckets;
	let drawn = 0;
	for (let bucket = 0; bucket < buckets; bucket += 1) {
		const from = start + Math.floor(bucket * width);
		const to = Math.min(end, start + Math.floor((bucket + 1) * width));
		if (from >= to) continue;
		let minIndex = from;
		let maxIndex = from;
		for (let i = from + 1; i < to; i += 1) {
			if (values[i] < values[minIndex]) minIndex = i;
			if (values[i] > values[maxIndex]) maxIndex = i;
		}
		const first = Math.min(minIndex, maxIndex);
		const second = Math.max(minIndex, maxIndex);
		buffer.timestamps[drawn] = timestamps[first];
		buffer.values[drawn] = values[first];
		drawn += 1;
		if (second !== first) {
			buffer.timestamps[drawn] = timestamps[second];
			buffer.values[drawn] = values[second];
			drawn += 1;
		}
	}

	// The drawn path must span the exact visible window. Otherwise uPlot's
	// auto-scale (used when live points reset the scale) would inset the axis by
	// one bucket on each side.
	if (drawn >= 2) {
		buffer.timestamps[0] = timestamps[start];
		buffer.values[0] = values[start];
		buffer.timestamps[drawn - 1] = timestamps[end - 1];
		buffer.values[drawn - 1] = values[end - 1];
	}

	return {
		timestamps: buffer.timestamps.subarray(0, drawn),
		values: buffer.values.subarray(0, drawn),
		sourcePoints,
		drawnPoints: drawn
	};
}
