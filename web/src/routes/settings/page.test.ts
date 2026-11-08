import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Page from './+page.svelte';
import { activeProject, projects } from '#lib/projects';

const fetchMock = vi.fn();

beforeEach(() => {
	fetchMock.mockReset();
	vi.stubGlobal('fetch', fetchMock);
	localStorage.clear();
	activeProject.select(projects[0].id);
});

describe('settings page', () => {
	it('shows the active project and the API version', async () => {
		fetchMock.mockResolvedValue({
			ok: true,
			status: 200,
			json: async () => ({ status: 'ok', version: '0.1.0-test' })
		});

		render(Page);

		expect(screen.getByRole('heading', { level: 1, name: 'Settings' })).toBeInTheDocument();
		expect(screen.getByText('Acme')).toBeInTheDocument();
		expect(screen.getByText('Production')).toBeInTheDocument();
		expect(screen.getByText('project-prod')).toBeInTheDocument();
		expect(await screen.findByText('0.1.0-test')).toBeInTheDocument();
	});

	it('offers a retry when the API is unavailable', async () => {
		fetchMock
			.mockResolvedValueOnce({ ok: false, status: 503, json: async () => ({}) })
			.mockResolvedValueOnce({
				ok: true,
				status: 200,
				json: async () => ({ status: 'ok', version: '0.1.0-recovered' })
			});

		render(Page);

		const alert = await screen.findByRole('alert');
		expect(alert).toHaveTextContent('The API is unavailable.');

		await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));

		expect(await screen.findByText('0.1.0-recovered')).toBeInTheDocument();
		expect(screen.queryByRole('alert')).not.toBeInTheDocument();
	});

	it('reflects the project selected in the switcher store', async () => {
		fetchMock.mockResolvedValue({
			ok: true,
			status: 200,
			json: async () => ({ status: 'ok' })
		});

		render(Page);
		expect(screen.getByText('project-prod')).toBeInTheDocument();

		activeProject.select('project-staging');

		expect(await screen.findByText('project-staging')).toBeInTheDocument();
		expect(screen.getAllByText('staging').length).toBeGreaterThan(0);
	});
});
