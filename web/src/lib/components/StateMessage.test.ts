import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import StateMessage from './StateMessage.svelte';
import { STATE_FRAME_MIN_HEIGHT, stateVariants } from '#lib/states';

describe('StateMessage', () => {
	it('reserves the same frame height for every state so the page does not shift', () => {
		for (const variant of stateVariants) {
			const { container, unmount } = render(StateMessage, { variant });
			const frame = container.querySelector('[data-state-frame]');

			expect(frame).toHaveAttribute('data-state-frame', variant);
			expect(frame).toHaveAttribute('data-state-height', STATE_FRAME_MIN_HEIGHT);
			expect((frame as HTMLElement).style.minHeight).toBe(STATE_FRAME_MIN_HEIGHT);
			unmount();
		}
	});

	it('shows an actionable empty state with both actions wired', async () => {
		const onprimary = vi.fn();
		const onsecondary = vi.fn();
		render(StateMessage, { variant: 'empty', onprimary, onsecondary });

		expect(screen.getByRole('status')).toHaveTextContent('No data for this view.');
		await fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Widen time range' }));
		expect(onprimary).toHaveBeenCalledOnce();
		expect(onsecondary).toHaveBeenCalledOnce();
	});

	it('announces errors as alerts and exposes the request ID', () => {
		render(StateMessage, { variant: 'error', requestId: 'req_01J8Z3K4', onprimary: vi.fn() });

		const alert = screen.getByRole('alert');
		expect(alert).toHaveTextContent('The query failed.');
		expect(alert).toHaveTextContent('Request ID req_01J8Z3K4');
		expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
	});

	it('shows a timeout pill with text, not color alone', () => {
		render(StateMessage, { variant: 'timeout', onprimary: vi.fn() });

		expect(screen.getByText('Timeout')).toBeInTheDocument();
		expect(screen.getByText('Query timed out after 30s.')).toBeInTheDocument();
	});

	it('keeps loading announcements for screen readers without actions', () => {
		render(StateMessage, { variant: 'loading' });

		expect(screen.getByRole('status')).toHaveTextContent('Loading data');
		expect(screen.queryByRole('button')).not.toBeInTheDocument();
	});

	it('offers a cancel action when the loading state can be dismissed', async () => {
		const onprimary = vi.fn();
		render(StateMessage, { variant: 'loading', onprimary });

		await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

		expect(onprimary).toHaveBeenCalledOnce();
	});

	it('disables the retry action while the rate limit window is open', () => {
		render(StateMessage, { variant: 'rate_limited', primaryDisabled: true, onprimary: vi.fn() });

		expect(screen.getByRole('button', { name: 'Retry in 30s' })).toBeDisabled();
		expect(screen.getByText('429')).toBeInTheDocument();
	});
});
