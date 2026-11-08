import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import Page from './+page.svelte';
import { stateVariants } from '#lib/states';

describe('state gallery', () => {
	it('triggers every state in the same reserved frame', async () => {
		const { container } = render(Page);

		for (const variant of stateVariants) {
			const label = {
				loading: 'Loading',
				empty: 'Empty',
				error: 'Error',
				timeout: 'Timeout',
				partial: 'Partial data',
				rate_limited: 'Rate limited',
				no_permission: 'No permission'
			}[variant];

			await fireEvent.click(screen.getByRole('button', { name: label }));

			const frame = container.querySelector('[data-state-frame]');
			expect(frame).toHaveAttribute('data-state-frame', variant);
			expect(frame).toHaveAttribute('data-state-height', '15rem');
		}
	});

	it('shows the error state as an alert with a request ID', async () => {
		render(Page);
		await fireEvent.click(screen.getByRole('button', { name: 'Error' }));

		const alert = screen.getByRole('alert');
		expect(alert).toHaveTextContent('The query failed.');
		expect(alert).toHaveTextContent('req_01J8Z3K4');
	});

	it('runs the state action and announces the result', async () => {
		render(Page);
		await fireEvent.click(screen.getByRole('button', { name: 'Timeout' }));

		await fireEvent.click(screen.getByRole('button', { name: 'Narrow to 24h' }));

		expect(
			screen.getByText(/The primary action for the “Timeout” state is handled by the shell/)
		).toBeInTheDocument();
	});

	it('disables the rate-limited retry until the window resets', async () => {
		render(Page);
		await fireEvent.click(screen.getByRole('button', { name: 'Rate limited' }));

		expect(screen.getByRole('button', { name: 'Retry in 30s' })).toBeDisabled();
	});
});
