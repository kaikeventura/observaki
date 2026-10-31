import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Page from './+page.svelte';

const fetchMock = vi.fn();

function mockHealth(body: unknown, ok = true, status = 200) {
	fetchMock.mockResolvedValue({
		ok,
		status,
		json: async () => body
	});
}

beforeEach(() => {
	fetchMock.mockReset();
	vi.stubGlobal('fetch', fetchMock);
});

describe('home page', () => {
	it('renders the product name', () => {
		mockHealth({ status: 'ok' });
		render(Page);
		expect(screen.getByRole('heading', { level: 1, name: 'Observaki' })).toBeInTheDocument();
	});

	it('shows the API status once the health check succeeds', async () => {
		mockHealth({ status: 'ok', version: '0.1.0-test' });
		render(Page);

		expect(await screen.findByText('Operational')).toBeInTheDocument();
		expect(screen.getByText('0.1.0-test')).toBeInTheDocument();
		expect(fetchMock).toHaveBeenCalledWith('/api/v1/health');
	});

	it('shows an error state when the API is unavailable', async () => {
		mockHealth({}, false, 503);
		render(Page);

		const alert = await screen.findByRole('alert');
		expect(alert).toHaveTextContent('API unavailable');
		expect(alert).toHaveTextContent('HTTP 503');
	});
});
