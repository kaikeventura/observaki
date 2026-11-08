import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it } from 'vitest';
import Page from './+page.svelte';
import { activeProject, projects } from '#lib/projects';

beforeEach(() => {
	localStorage.clear();
	activeProject.select(projects[0].id);
});

describe('explore page', () => {
	it('shows the surface, the active project and an actionable empty state', async () => {
		render(Page);

		expect(screen.getByRole('heading', { level: 1, name: 'Explore' })).toBeInTheDocument();
		expect(screen.getByText(/Acme \/ Production/)).toBeInTheDocument();
		expect(screen.getByText('No traces match this filter.')).toBeInTheDocument();

		await fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }));

		expect(
			screen.getByText(
				'Mock surface: filters cleared. Signals arrive with the Query Service in Phase 1.'
			)
		).toBeInTheDocument();
	});
});
