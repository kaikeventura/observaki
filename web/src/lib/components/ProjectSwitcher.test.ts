import { fireEvent, render, screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { beforeEach, describe, expect, it } from 'vitest';
import ProjectSwitcher from './ProjectSwitcher.svelte';
import { PROJECT_STORAGE_KEY, activeProject, projects } from '#lib/projects';

function trigger() {
	return screen.getByRole('button', { name: /Acme \// });
}

beforeEach(() => {
	localStorage.clear();
	activeProject.select(projects[0].id);
});

describe('ProjectSwitcher', () => {
	it('shows the active project and lists every mock project', async () => {
		render(ProjectSwitcher);

		expect(trigger()).toHaveTextContent('Acme / Production');
		expect(trigger()).toHaveAttribute('aria-expanded', 'false');

		await fireEvent.click(trigger());

		expect(trigger()).toHaveAttribute('aria-expanded', 'true');
		expect(
			screen.getByRole('button', { name: 'Switch to Acme Production (production)' })
		).toHaveAttribute('aria-pressed', 'true');
		expect(
			screen.getByRole('button', { name: 'Switch to Acme Staging (staging)' })
		).toHaveAttribute('aria-pressed', 'false');
	});

	it('switches project, announces the change and persists the selection', async () => {
		render(ProjectSwitcher);
		await fireEvent.click(trigger());

		await fireEvent.click(screen.getByRole('button', { name: 'Switch to Acme Staging (staging)' }));

		expect(get(activeProject).id).toBe('project-staging');
		expect(localStorage.getItem(PROJECT_STORAGE_KEY)).toBe('project-staging');
		expect(trigger()).toHaveTextContent('Acme / Staging');
		expect(trigger()).toHaveAttribute('aria-expanded', 'false');
		expect(screen.getByRole('status')).toHaveTextContent('Switched to Acme Staging.');
		expect(
			screen.queryByRole('button', { name: 'Switch to Acme Staging (staging)' })
		).not.toBeInTheDocument();
	});

	it('restores the persisted project after mount', async () => {
		localStorage.setItem(PROJECT_STORAGE_KEY, 'project-staging');

		render(ProjectSwitcher);

		expect(await screen.findByText(/Acme \/ Staging/)).toBeInTheDocument();
	});

	it('closes with Escape and returns focus to the trigger', async () => {
		render(ProjectSwitcher);
		const button = trigger();
		button.focus();
		await fireEvent.click(button);
		expect(
			screen.getByRole('button', { name: 'Switch to Acme Production (production)' })
		).toBeInTheDocument();

		await fireEvent.keyDown(window, { key: 'Escape' });

		expect(
			screen.queryByRole('button', { name: 'Switch to Acme Production (production)' })
		).not.toBeInTheDocument();
		expect(button).toHaveFocus();
	});
});
