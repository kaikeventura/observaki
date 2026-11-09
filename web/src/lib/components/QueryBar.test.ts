import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import QueryBar from './QueryBar.svelte';

function queryInput(): HTMLInputElement {
	return document.querySelector('[data-query-input]') as HTMLInputElement;
}

describe('QueryBar', () => {
	it('shows mock suggestions while typing and inserts the active one', async () => {
		const onrun = vi.fn();
		render(QueryBar, { value: '', signal: 'logs', onrun });

		const input = queryInput();
		await fireEvent.focus(input);
		await fireEvent.input(input, { target: { value: 'body' } });

		const list = screen.getByRole('listbox', { name: 'Logs suggestions' });
		expect(list).toBeInTheDocument();
		expect(screen.getAllByRole('option').length).toBeGreaterThan(0);
		expect(input).toHaveAttribute('aria-expanded', 'true');

		await fireEvent.keyDown(input, { key: 'ArrowDown' });
		expect(input).toHaveAttribute('aria-activedescendant');
		await fireEvent.keyDown(input, { key: 'Enter' });

		expect(input.value).toBe('body:');
		expect(onrun).not.toHaveBeenCalled();
	});

	it('runs with Ctrl/Meta+Enter and closes the list with Escape', async () => {
		const onrun = vi.fn();
		render(QueryBar, { value: 'service:checkout', signal: 'traces', onrun });

		const input = queryInput();
		await fireEvent.focus(input);
		await fireEvent.input(input, { target: { value: 'ser' } });
		expect(screen.getByRole('listbox')).toBeInTheDocument();

		await fireEvent.keyDown(input, { key: 'Escape' });
		await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument());

		await fireEvent.keyDown(input, { key: 'Enter', metaKey: true });
		expect(onrun).toHaveBeenCalledWith('ser');
	});

	it('announces the suggestion count for screen readers', async () => {
		render(QueryBar, { value: '', signal: 'metrics', onrun: () => {} });
		const input = queryInput();
		await fireEvent.focus(input);
		await fireEvent.input(input, { target: { value: 'http_' } });

		expect(screen.getByText(/suggestions available for Metrics/)).toBeInTheDocument();
	});
});
