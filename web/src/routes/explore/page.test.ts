import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Page from './+page.svelte';
import { activeProject, projects } from '#lib/projects';
import { resetFakeUPlot } from '#lib/components/fixtures/fakeUplot';

vi.mock('uplot', async () => {
	const { FakeUPlot: Fake } = await import('#lib/components/fixtures/fakeUplot');
	return { default: Fake };
});

beforeEach(() => {
	localStorage.clear();
	activeProject.select(projects[0].id);
	history.replaceState(null, '', '/explore');
	resetFakeUPlot();
});

async function tableReady() {
	return waitFor(() => {
		expect(document.querySelector('[data-table-ready="true"]')).toBeTruthy();
	});
}

function queryInput(): HTMLInputElement {
	return document.querySelector('[data-query-input]') as HTMLInputElement;
}

describe('explore mock', () => {
	it('renders the signal tabs, the sticky range and the log result', async () => {
		const { container } = render(Page);

		expect(screen.getByRole('heading', { level: 1, name: 'Explore' })).toBeInTheDocument();
		expect(screen.getByRole('tablist', { name: 'Signal' })).toBeInTheDocument();
		expect(screen.getByRole('tab', { name: 'Logs' })).toHaveAttribute('aria-selected', 'true');
		expect(screen.getByRole('button', { name: '1h' })).toHaveAttribute('aria-pressed', 'true');

		await tableReady();
		const scroller = container.querySelector('[data-table-ready="true"]') as HTMLElement;
		expect(scroller).toHaveAttribute('data-table-total', '10000');
	});

	it('keeps the time range and service filter across signals and announces it', async () => {
		render(Page);
		await tableReady();

		await fireEvent.click(screen.getByRole('button', { name: '6h' }));
		await fireEvent.change(screen.getByLabelText('Service'), { target: { value: 'checkout' } });

		await fireEvent.click(screen.getByRole('tab', { name: 'Traces' }));

		expect(screen.getByRole('tab', { name: 'Traces' })).toHaveAttribute('aria-selected', 'true');
		expect(screen.getByRole('button', { name: '6h' })).toHaveAttribute('aria-pressed', 'true');
		expect(screen.getByLabelText('Service')).toHaveValue('checkout');
		expect(screen.getByText(/Kept the 6h time range and the checkout service/)).toBeInTheDocument();
		expect(document.querySelector('[data-trace-total]')).toBeTruthy();
		expect(window.location.search).toContain('signal=traces');
		expect(window.location.search).toContain('range=6h');
		expect(window.location.search).toContain('service=checkout');
	});

	it('drops a query that does not apply to the new signal and keeps a valid one', async () => {
		render(Page);
		await tableReady();

		const input = queryInput();
		await fireEvent.input(input, { target: { value: 'severity:error' } });
		await fireEvent.keyDown(input, { key: 'Enter' });

		await fireEvent.click(screen.getByRole('tab', { name: 'Metrics' }));
		expect(screen.getByText(/Dropped the query "severity:error"/)).toBeInTheDocument();
		expect(queryInput()).toHaveValue('');

		await fireEvent.input(queryInput(), { target: { value: 'service:checkout' } });
		await fireEvent.keyDown(queryInput(), { key: 'Enter' });
		await fireEvent.click(screen.getByRole('tab', { name: 'Traces' }));
		expect(screen.getByText(/Kept the query\./)).toBeInTheDocument();
		expect(queryInput()).toHaveValue('service:checkout');
	});

	it('autocompletes from mock data and restores the view from the URL', async () => {
		const first = render(Page);
		await tableReady();

		const input = queryInput();
		await fireEvent.focus(input);
		await fireEvent.input(input, { target: { value: 'kafka' } });
		await fireEvent.click(screen.getByRole('tab', { name: 'Metrics' }));
		const metricsInput = queryInput();
		await fireEvent.focus(metricsInput);
		await fireEvent.input(metricsInput, { target: { value: 'kafka' } });

		const options = screen.getAllByRole('option');
		expect(options.map((option) => option.textContent)).toEqual(
			expect.arrayContaining([expect.stringContaining('kafka_consumer_lag')])
		);
		await fireEvent.keyDown(metricsInput, { key: 'ArrowDown' });
		await fireEvent.keyDown(metricsInput, { key: 'Enter' });
		expect(queryInput().value).toBe('kafka_consumer_lag');

		first.unmount();

		history.replaceState(
			null,
			'',
			'/explore?signal=traces&range=24h&service=cart&q=status%3Aerror'
		);
		render(Page);

		await waitFor(() => {
			expect(screen.getByRole('tab', { name: 'Traces' })).toHaveAttribute('aria-selected', 'true');
		});
		expect(screen.getByRole('button', { name: '24h' })).toHaveAttribute('aria-pressed', 'true');
		expect(screen.getByLabelText('Service')).toHaveValue('cart');
		expect(queryInput()).toHaveValue('status:error');
		expect(screen.getByText(/Showing the newest/)).toBeInTheDocument();
	});
});
