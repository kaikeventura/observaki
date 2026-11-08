import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it } from 'vitest';
import Harness from './fixtures/AppShellHarness.svelte';
import { activeProject, projects } from '#lib/projects';

beforeEach(() => {
	localStorage.clear();
	activeProject.select(projects[0].id);
});

describe('AppShell', () => {
	it('shows navigation for Explore, Dashboards, Alerts and Settings', () => {
		render(Harness, { activePath: '/explore' });

		const nav = screen.getByRole('navigation', { name: 'Primary' });
		for (const label of ['Explore', 'Dashboards', 'Alerts', 'Settings']) {
			expect(screen.getByRole('link', { name: label })).toBeInTheDocument();
		}

		expect(screen.getByRole('link', { name: 'Explore' })).toHaveAttribute('aria-current', 'page');
		expect(screen.getByRole('link', { name: 'Dashboards' })).not.toHaveAttribute('aria-current');
		expect(nav).toHaveTextContent('State gallery');
		expect(nav).toHaveTextContent('System health');
	});

	it('puts the project switcher and the content in the shell', () => {
		render(Harness, { activePath: '/' });

		expect(screen.getByRole('button', { name: /Acme \/ Production/ })).toBeInTheDocument();
		expect(screen.getByText('Shell content')).toBeInTheDocument();
		expect(screen.getByRole('main')).toHaveAttribute('id', 'main-content');
	});

	it('starts keyboard navigation at a skip link and keeps every control native', () => {
		const { container } = render(Harness, { activePath: '/settings' });

		const skip = screen.getByRole('link', { name: 'Skip to main content' });
		expect(skip).toHaveAttribute('href', '#main-content');

		const interactive = Array.from(
			container.querySelectorAll<HTMLElement>('a[href], button, input, select, textarea')
		);
		expect(interactive.length).toBeGreaterThan(6);
		for (const element of interactive) {
			expect(element.getAttribute('tabindex')).not.toBe('-1');
			expect(element.className).toContain('focus-visible:outline');
		}
	});
});
