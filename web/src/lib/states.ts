/**
 * Shared copy and metadata for the UI states. EXPERIENCE.md owns the treatment;
 * DESIGN.md owns the visual language. Surfaces may override the copy, but every
 * state keeps one sentence of what happened plus an actionable next step.
 */
export const stateVariants = [
	'loading',
	'empty',
	'error',
	'timeout',
	'partial',
	'rate_limited',
	'no_permission'
] as const;

export type StateVariant = (typeof stateVariants)[number];

export type StatePillTone = 'neutral' | 'info' | 'warning' | 'danger';

export type StateCopy = {
	title: string;
	body: string;
	primary?: string;
	secondary?: string;
	pill?: string;
	tone?: StatePillTone;
};

export const stateDefaults: Record<StateVariant, StateCopy> = {
	loading: {
		title: 'Loading data',
		body: 'Fetching the latest signal for this project. You can cancel and keep the previous result; the frame keeps its size so nothing jumps.',
		primary: 'Cancel'
	},
	empty: {
		title: 'No data for this view.',
		body: 'Adjust the filters or widen the time range, then run the query again. The query is kept.',
		primary: 'Clear filters',
		secondary: 'Widen time range'
	},
	error: {
		title: 'The query failed.',
		body: 'The request could not be completed. Retry, or share the request ID with your administrator.',
		primary: 'Retry',
		secondary: 'Open system health',
		tone: 'danger'
	},
	timeout: {
		title: 'Query timed out after 30s.',
		body: 'Narrow the time range or add a filter, then run it again. The previous result stays visible as stale.',
		primary: 'Narrow to 24h',
		secondary: 'Retry',
		pill: 'Timeout',
		tone: 'warning'
	},
	partial: {
		title: 'Showing partial data.',
		body: 'Some signals were sampled or are still loading. Values marked partial may be incomplete.',
		primary: 'Show retained data',
		secondary: 'Open system health',
		pill: 'Partial',
		tone: 'warning'
	},
	rate_limited: {
		title: 'Too many requests. Retry in 30s.',
		body: 'The project query quota is exhausted. The action stays disabled until the window resets.',
		primary: 'Retry in 30s',
		secondary: 'View quota',
		pill: '429',
		tone: 'warning'
	},
	no_permission: {
		title: 'You do not have permission to view this project.',
		body: 'Ask an admin for the viewer role, or switch to a project you can access.',
		primary: 'Request access',
		secondary: 'Switch project',
		pill: '403',
		tone: 'danger'
	}
};

/** Reserved height shared by every state so the page does not shift. */
export const STATE_FRAME_MIN_HEIGHT = '15rem';
