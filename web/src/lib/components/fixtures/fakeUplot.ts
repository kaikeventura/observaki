/**
 * Minimal uPlot stand-in for jsdom tests. It records construction and update
 * calls so the chart component can be tested for "reuse the plot" and for the
 * tooltip/range behaviour without a canvas.
 */

export type FakeAlignedData = [Float64Array, Float64Array];

export type FakeChartOptions = {
	width: number;
	height: number;
	hooks?: {
		setScale?: Array<(chart: FakeUPlot, key: string) => void>;
	};
};

export class FakeUPlot {
	static readonly created: FakeUPlot[] = [];

	readonly opts: FakeChartOptions;
	data: FakeAlignedData;
	readonly root: HTMLDivElement;
	readonly over: HTMLDivElement;
	scales: { x: { min: number; max: number }; y: Record<string, number> };
	readonly setDataCalls: Array<boolean | undefined> = [];
	readonly setScaleCalls: Array<{ min: number; max: number }> = [];
	readonly setSizeCalls: Array<{ width: number; height: number }> = [];
	destroyed = false;

	constructor(opts: FakeChartOptions, data: FakeAlignedData, target: HTMLElement) {
		this.opts = opts;
		this.data = data;
		this.root = document.createElement('div');
		this.over = document.createElement('div');
		this.root.append(this.over);
		target.append(this.root);

		const xs = data[0];
		this.scales = { x: { min: xs[0], max: xs[xs.length - 1] }, y: {} };
		FakeUPlot.created.push(this);
		opts.hooks?.setScale?.[0]?.(this, 'x');
	}

	posToVal(pos: number, _key: string): number {
		const { min, max } = this.scales.x;
		return min + (pos / Math.max(1, this.opts.width)) * (max - min);
	}

	posToIdx(pos: number): number {
		const xs = this.data[0];
		const target = this.posToVal(pos, 'x');
		let low = 0;
		let high = xs.length - 1;
		while (low < high) {
			const mid = Math.floor((low + high) / 2);
			if (xs[mid] < target) low = mid + 1;
			else high = mid;
		}
		if (low > 0 && Math.abs(xs[low - 1] - target) < Math.abs(xs[low] - target)) return low - 1;
		return low;
	}

	setData(data: FakeAlignedData, resetScales?: boolean): void {
		this.data = data;
		this.setDataCalls.push(resetScales);
		if (resetScales !== false) {
			const xs = data[0];
			this.scales.x = { min: xs[0], max: xs[xs.length - 1] };
			this.opts.hooks?.setScale?.[0]?.(this, 'x');
		}
	}

	setScale(_key: string, range: { min: number; max: number }): void {
		this.scales.x = { min: range.min, max: range.max };
		this.setScaleCalls.push(range);
		this.opts.hooks?.setScale?.[0]?.(this, 'x');
	}

	setSize(size: { width: number; height: number }): void {
		this.setSizeCalls.push(size);
	}

	destroy(): void {
		this.destroyed = true;
	}
}

export function resetFakeUPlot(): void {
	FakeUPlot.created.length = 0;
}
