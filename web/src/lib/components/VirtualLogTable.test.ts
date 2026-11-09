import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import VirtualLogTable from './VirtualLogTable.svelte';
import { generateLogs } from '#lib/table/logs';

const END = 1_700_000_000;
const ROWS = generateLogs({ rows: 10_000, endSeconds: END });

function scroller(container: HTMLElement): HTMLElement {
	return container.querySelector('[data-table-ready="true"]') as HTMLElement;
}

function visibleRows(container: HTMLElement): HTMLElement[] {
	return Array.from(container.querySelectorAll<HTMLElement>('tr[data-row-id]'));
}

describe('VirtualLogTable', () => {
	it('renders 10k rows while keeping only the visible window in the DOM', async () => {
		const { container } = render(VirtualLogTable, { rows: ROWS, label: 'Logs' });
		const host = scroller(container);

		expect(screen.getByRole('table')).toBeInTheDocument();
		expect(screen.getAllByRole('columnheader')).toHaveLength(6);
		expect(host).toHaveAttribute('data-table-total', '10000');
		expect(Number(host.getAttribute('data-table-rendered'))).toBeLessThan(40);
		expect(visibleRows(container).length).toBeGreaterThan(0);
		expect(visibleRows(container).length).toBeLessThan(40);
		expect(host).toHaveAttribute('data-table-sort', 'timestamp:desc');
	});

	it('moves the virtual window with the scroll position', async () => {
		const { container } = render(VirtualLogTable, { rows: ROWS });
		const host = scroller(container);

		host.scrollTop = 4_000;
		await fireEvent.scroll(host);

		await waitFor(() => {
			expect(host.getAttribute('data-table-start')).not.toBe('0');
		});
		expect(Number(host.getAttribute('data-table-start'))).toBeGreaterThan(80);
		expect(visibleRows(container).length).toBeLessThan(40);
		expect(Number(visibleRows(container)[0].getAttribute('data-row-index'))).toBeGreaterThan(80);
	});

	it('sorts explicitly and keeps the column widths', async () => {
		const { container } = render(VirtualLogTable, { rows: ROWS });
		const resizer = screen.getByRole('separator', { name: 'Resize Service column' });

		await fireEvent.keyDown(resizer, { key: 'ArrowRight' });
		expect(resizer).toHaveAttribute('aria-valuenow', '158');

		await fireEvent.click(screen.getByRole('button', { name: 'Duration' }));

		const header = container.querySelector('th[data-column="duration"]');
		expect(header).toHaveAttribute('aria-sort', 'ascending');
		const ascDurations = visibleRows(container).map((row) =>
			Number(row.getAttribute('data-row-duration'))
		);
		for (let i = 1; i < ascDurations.length; i += 1) {
			expect(ascDurations[i]).toBeGreaterThanOrEqual(ascDurations[i - 1]);
		}

		await fireEvent.click(screen.getByRole('button', { name: 'Duration' }));

		expect(header).toHaveAttribute('aria-sort', 'descending');
		const descDurations = visibleRows(container).map((row) =>
			Number(row.getAttribute('data-row-duration'))
		);
		for (let i = 1; i < descDurations.length; i += 1) {
			expect(descDurations[i]).toBeLessThanOrEqual(descDurations[i - 1]);
		}

		expect(resizer).toHaveAttribute('aria-valuenow', '158');
		expect(container.querySelector('col[data-column="service"]')).toHaveAttribute(
			'data-width',
			'158'
		);
	});

	it('selects a row, opens the detail panel and closes it with Escape', async () => {
		const { container } = render(VirtualLogTable, { rows: ROWS });
		const first = visibleRows(container)[0];
		const id = first.getAttribute('data-row-id');

		await fireEvent.click(first);

		expect(first).toHaveAttribute('aria-current', 'true');
		expect(scroller(container)).toHaveAttribute('data-table-selected', id!);
		expect(scroller(container)).toHaveAttribute('data-table-detail', 'true');
		const panel = screen.getByRole('complementary', { name: 'Log detail' });
		expect(panel).toHaveTextContent(id!);
		expect(panel).toHaveTextContent('Trace ID');

		await fireEvent.keyDown(window, { key: 'Escape' });

		expect(screen.queryByRole('complementary', { name: 'Log detail' })).toBeNull();
		expect(scroller(container)).toHaveAttribute('data-table-selected', id!);
	});

	it('moves the selection with j/k and opens with Enter', async () => {
		const { container } = render(VirtualLogTable, { rows: ROWS });
		const host = scroller(container);

		host.focus();
		await fireEvent.keyDown(host, { key: 'j' });
		expect(host).toHaveAttribute('data-table-selected-index', '0');

		await fireEvent.keyDown(host, { key: 'j' });
		expect(host).toHaveAttribute('data-table-selected-index', '1');
		expect(host).toHaveAttribute('data-table-detail', 'false');

		await fireEvent.keyDown(host, { key: 'Enter' });
		expect(host).toHaveAttribute('data-table-detail', 'true');

		await fireEvent.keyDown(host, { key: 'k' });
		expect(host).toHaveAttribute('data-table-selected-index', '0');

		await fireEvent.keyDown(host, { key: 'Escape' });
		expect(host).toHaveAttribute('data-table-detail', 'false');
	});

	it('shows an empty state and offers a retry', async () => {
		const onretry = vi.fn();
		render(VirtualLogTable, { rows: [], status: 'empty', onretry });

		expect(screen.getByText('No logs match this filter.')).toBeInTheDocument();
		expect(document.querySelector('[data-table-state-frame="empty"]')).toBeTruthy();

		await fireEvent.click(screen.getByRole('button', { name: 'Retry query' }));
		expect(onretry).toHaveBeenCalledTimes(1);
	});

	it('shows the error state with a request id and offers a retry', async () => {
		const onretry = vi.fn();
		render(VirtualLogTable, { rows: [], status: 'error', onretry, requestId: 'req_123' });

		const alert = screen.getByRole('alert');
		expect(alert).toHaveTextContent('The query failed.');
		expect(alert).toHaveTextContent('Request ID req_123');

		await fireEvent.click(screen.getByRole('button', { name: 'Retry query' }));
		expect(onretry).toHaveBeenCalledTimes(1);
	});

	it('treats an empty ready result as the empty state', () => {
		render(VirtualLogTable, { rows: [], status: 'ready' });

		expect(screen.getByText('No logs match this filter.')).toBeInTheDocument();
		expect(document.querySelector('[data-table-state-frame="empty"]')).toBeTruthy();
	});
});
