import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import TimeSeriesChart from './TimeSeriesChart.svelte';
import { FakeUPlot, resetFakeUPlot } from './fixtures/fakeUplot';
import { appendPoint, generateSeries } from '#lib/chart/series';
import { formatTimestamp, formatValue } from '#lib/chart/format';

vi.mock('uplot', async () => {
	const { FakeUPlot: Fake } = await import('#lib/components/fixtures/fakeUplot');
	return { default: Fake };
});

const END = 1_700_000_000;

function series() {
	return generateSeries({ points: 400, endSeconds: END });
}

async function ready(container: HTMLElement) {
	await waitFor(() => {
		expect(container.querySelector('[data-chart-ready="true"]')).toBeTruthy();
	});
	return container.querySelector('[data-chart-ready]') as HTMLElement;
}

beforeEach(() => {
	resetFakeUPlot();
});

describe('TimeSeriesChart', () => {
	it('renders one plot for 100k points and reports the first paint', async () => {
		const data = generateSeries({ endSeconds: END });
		const { container } = render(TimeSeriesChart, {
			series: data,
			label: 'Request rate',
			unit: 'req/s'
		});

		const host = await ready(container);

		expect(FakeUPlot.created).toHaveLength(1);
		expect(host).toHaveAttribute('data-chart-instances', '1');
		expect(Number(host.getAttribute('data-chart-render-ms'))).toBeGreaterThanOrEqual(0);
		expect(screen.getByRole('heading', { name: 'Request rate' })).toBeInTheDocument();
		expect(screen.getAllByText(/100,000 points/).length).toBeGreaterThan(0);
		expect(screen.getByText(/Drawn [\d,]+ of 100,000/)).toBeInTheDocument();
		expect(screen.getByText(/First paint/)).toBeInTheDocument();
	});

	it('reuses the plot and the same arrays when live points arrive', async () => {
		const data = series();
		const props = { series: data, label: 'Request rate', unit: 'req/s', revision: 0 };
		const { container, rerender } = render(TimeSeriesChart, props);
		const host = await ready(container);
		const chart = FakeUPlot.created[0];

		expect(appendPoint(data, END + 1, 1_337)).toBe(true);
		await rerender({ ...props, revision: 1 });
		await tick();

		expect(FakeUPlot.created).toHaveLength(1);
		expect(FakeUPlot.created[0]).toBe(chart);
		expect(chart.setDataCalls).toHaveLength(1);
		expect(Array.from(chart.data[0])).toEqual(Array.from(data.timestamps));
		expect(Array.from(chart.data[1])).toEqual(Array.from(data.values));
		expect(host).toHaveAttribute('data-chart-updates', '1');
		expect(screen.getByText(/Last update/)).toBeInTheDocument();
	});

	it('keeps a user zoom when new points arrive', async () => {
		const data = series();
		const props = { series: data, label: 'Request rate', revision: 0 };
		const { container, rerender } = render(TimeSeriesChart, props);
		await ready(container);
		const chart = FakeUPlot.created[0];

		await fireEvent.keyDown(screen.getByRole('group', { name: /time series/ }), { key: '+' });
		appendPoint(data, END + 1, 1_337);
		await rerender({ ...props, revision: 1 });
		await tick();

		expect(chart.setDataCalls.at(-1)).toBe(false);
	});

	it('shows the timestamp and value of the hovered point', async () => {
		const data = series();
		const { container } = render(TimeSeriesChart, {
			series: data,
			label: 'Request rate',
			unit: 'req/s'
		});
		await ready(container);
		const chart = FakeUPlot.created[0];

		const move = new MouseEvent('mousemove', { bubbles: true, clientX: 160 });
		Object.defineProperty(move, 'offsetX', { value: 160 });
		chart.over.dispatchEvent(move);
		await tick();

		const index = chart.posToIdx(160);
		const tooltip = container.querySelector('[data-chart-tooltip]') as HTMLElement;

		expect(tooltip).toBeTruthy();
		expect(tooltip).toHaveTextContent(formatTimestamp(data.timestamps[index]));
		expect(tooltip).toHaveTextContent(formatValue(data.values[index], 'req/s'));
		expect(tooltip).toHaveAttribute('data-chart-tooltip-time', String(data.timestamps[index]));
		expect(tooltip).toHaveAttribute('data-chart-tooltip-value', String(data.values[index]));

		chart.over.dispatchEvent(new MouseEvent('mouseleave', { bubbles: true }));
		await tick();
		expect(container.querySelector('[data-chart-tooltip]')).toBeNull();
	});

	it('zooms, pans and resets with the keyboard and announces the range', async () => {
		const data = series();
		const { container } = render(TimeSeriesChart, { series: data, label: 'Request rate' });
		await ready(container);
		const chart = FakeUPlot.created[0];
		const group = screen.getByRole('group', { name: /time series/ });

		await fireEvent.keyDown(group, { key: '+' });
		const zoomed = chart.setScaleCalls.at(-1);
		expect(zoomed).toBeTruthy();
		expect(zoomed!.max - zoomed!.min).toBeLessThan(END - data.timestamps[0]);
		await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent(/Zoomed:/));

		await fireEvent.keyDown(group, { key: 'ArrowRight' });
		const panned = chart.setScaleCalls.at(-1);
		expect(panned!.min).toBeGreaterThan(zoomed!.min);
		await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent(/Panned:/));

		await fireEvent.keyDown(group, { key: '0' });
		expect(chart.setScaleCalls.at(-1)).toEqual({ min: data.timestamps[0], max: END });
		await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent(/Full range:/));
	});

	it('offers buttons for the same zoom and pan actions', async () => {
		const data = series();
		const { container } = render(TimeSeriesChart, { series: data, label: 'Request rate' });
		await ready(container);
		const chart = FakeUPlot.created[0];

		await fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }));
		expect(chart.setScaleCalls).toHaveLength(1);

		await fireEvent.click(screen.getByRole('button', { name: 'Pan later' }));
		expect(chart.setScaleCalls).toHaveLength(2);

		await fireEvent.click(screen.getByRole('button', { name: 'Jump to the latest 10 percent' }));
		expect(chart.setScaleCalls).toHaveLength(3);

		await fireEvent.click(screen.getByRole('button', { name: 'Reset to the full range' }));
		expect(chart.setScaleCalls.at(-1)).toEqual({ min: data.timestamps[0], max: END });
	});

	it('ships an accessible table with the visible range in buckets', async () => {
		const { container } = render(TimeSeriesChart, {
			series: series(),
			label: 'Request rate',
			unit: 'req/s'
		});
		await ready(container);

		expect(screen.getByText('View as table (12 buckets)')).toBeInTheDocument();
		expect(container.querySelectorAll('tbody tr')).toHaveLength(12);
		expect(screen.getByText(/Downsampled summary of the visible range/)).toBeInTheDocument();
	});

	it('shows a live badge and the visible stats with text, not color alone', async () => {
		const { container } = render(TimeSeriesChart, {
			series: series(),
			label: 'Request rate',
			unit: 'req/s',
			live: true
		});
		await ready(container);

		expect(screen.getByText('Live 1 Hz')).toBeInTheDocument();
		expect(screen.getByText(/min .* req\/s/)).toBeInTheDocument();
	});

	it('destroys the plot on unmount', async () => {
		const { container, unmount } = render(TimeSeriesChart, {
			series: series(),
			label: 'Request rate'
		});
		await ready(container);
		const chart = FakeUPlot.created[0];

		unmount();

		expect(chart.destroyed).toBe(true);
	});
});
