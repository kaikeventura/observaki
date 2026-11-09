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
	resetFakeUPlot();
});

describe('dashboards page', () => {
	it('renders the chart spike with the active project and the panel notes', async () => {
		render(Page);

		expect(screen.getByRole('heading', { level: 1, name: 'Dashboards' })).toBeInTheDocument();
		expect(screen.getByText(/Acme \/ Production/)).toBeInTheDocument();
		expect(screen.getByRole('heading', { name: 'What this panel proves' })).toBeInTheDocument();
		await waitFor(() =>
			expect(screen.getByRole('heading', { name: 'Request rate' })).toBeInTheDocument()
		);
	});

	it('pauses and resumes the 1 Hz live updates', async () => {
		render(Page);

		const pause = await screen.findByRole('button', { name: 'Pause live' });
		expect(pause).toHaveAttribute('aria-pressed', 'true');

		await fireEvent.click(pause);

		const resume = screen.getByRole('button', { name: 'Resume live' });
		expect(resume).toHaveAttribute('aria-pressed', 'false');
		expect(screen.queryByText('Live 1 Hz')).toBeNull();
	});

	it('appends a point on demand', async () => {
		const { container } = render(Page);

		await screen.findByRole('button', { name: 'Append one point' });
		await waitFor(() => {
			expect(container.querySelector('[data-chart-ready="true"]')).toBeTruthy();
		});

		await fireEvent.click(screen.getByRole('button', { name: 'Append one point' }));

		await waitFor(() => {
			expect(container.querySelector('[data-chart-updates="1"]')).toBeTruthy();
		});
	});
});
