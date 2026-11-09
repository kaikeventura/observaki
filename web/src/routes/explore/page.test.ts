import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it } from 'vitest';
import Page from './+page.svelte';
import { activeProject, projects } from '#lib/projects';

beforeEach(() => {
	localStorage.clear();
	activeProject.select(projects[0].id);
});

async function tableReady() {
	return waitFor(() => {
		expect(document.querySelector('[data-table-ready="true"]')).toBeTruthy();
	});
}

describe('explore page', () => {
	it('shows the virtualized 10k-row table with the active project', async () => {
		const { container } = render(Page);

		expect(screen.getByRole('heading', { level: 1, name: 'Explore' })).toBeInTheDocument();
		expect(screen.getByText(/Acme \/ Production/)).toBeInTheDocument();
		await tableReady();

		const scroller = container.querySelector('[data-table-ready="true"]') as HTMLElement;
		expect(scroller).toHaveAttribute('data-table-total', '10000');
		expect(Number(scroller.getAttribute('data-table-rendered'))).toBeLessThan(40);
		expect(container.querySelectorAll('tr[data-row-id]').length).toBeLessThan(40);
	});

	it('shows the empty state and recovers when the user retries', async () => {
		render(Page);
		await tableReady();

		await fireEvent.click(screen.getByRole('button', { name: 'Empty result' }));

		expect(screen.getByText('No logs match this filter.')).toBeInTheDocument();
		expect(document.querySelector('[data-table-state-frame="empty"]')).toBeTruthy();

		await fireEvent.click(screen.getByRole('button', { name: 'Retry query' }));
		await tableReady();
		expect(document.querySelector('[data-table-ready="true"]')).toHaveAttribute(
			'data-table-total',
			'10000'
		);
	});

	it('shows the error state with a request id and retries', async () => {
		render(Page);
		await tableReady();

		await fireEvent.click(screen.getByRole('button', { name: 'Query error' }));

		expect(screen.getByRole('alert')).toHaveTextContent('The query failed.');
		expect(screen.getByText(/Request ID req_01J8Z3K4/)).toBeInTheDocument();

		await fireEvent.click(screen.getByRole('button', { name: 'Retry query' }));
		await tableReady();
	});
});
