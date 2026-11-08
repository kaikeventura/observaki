import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Page from './+page.svelte';
import type { HealthOverview } from '#lib/api';

const fetchMock = vi.fn();

function mockOverview(body: HealthOverview, ok = true, status = 200) {
	fetchMock.mockResolvedValue({
		ok,
		status,
		json: async () => body
	});
}

const overview: HealthOverview = {
	generated_at: '2026-11-08T12:00:00Z',
	status: 'degraded',
	summary: ['ingestion lag p95 is above 10s'],
	ingestion: {
		status: 'degraded',
		lag_p95_seconds: 0.8,
		pending_messages: 3,
		dlq_messages: 0,
		written_events: 150,
		writer_connected: true,
		observed_at: '2026-11-08T11:59:58Z',
		source: 'writer /stats + gateway /metrics',
		detail: 'ingestion lag p95 is above the 10s threshold'
	},
	query: {
		status: 'healthy',
		requests: 4,
		errors: 0,
		client_errors: 0,
		error_rate: 0,
		since: '2026-11-08T12:00:00Z'
	},
	storage: {
		status: 'healthy',
		clickhouse_bytes: 4096,
		clickhouse_parts: 3,
		victoriametrics_bytes: 2048,
		victoriametrics_series: 7,
		observed_at: '2026-11-08T11:59:59Z'
	},
	components: [
		{ name: 'api', status: 'healthy', latency_ms: 0.2, observed_at: '2026-11-08T12:00:00Z' },
		{ name: 'gateway', status: 'healthy', latency_ms: 1.4, observed_at: '2026-11-08T12:00:00Z' },
		{
			name: 'writer',
			status: 'degraded',
			latency_ms: 2.1,
			detail: 'slow',
			observed_at: '2026-11-08T12:00:00Z'
		}
	],
	alerting: {
		status: 'healthy',
		enabled: true,
		evaluation_age_seconds: 3,
		stale_after_seconds: 45,
		firing: 1,
		pending: 0,
		notifications_pending: 0
	}
};

beforeEach(() => {
	fetchMock.mockReset();
	vi.stubGlobal('fetch', fetchMock);
});

describe('health page', () => {
	it('shows ingestion lag, query error rate, storage usage and component health', async () => {
		mockOverview(overview);
		render(Page);

		expect(await screen.findByRole('heading', { name: 'Ingestion lag' })).toBeInTheDocument();
		expect(screen.getByRole('heading', { name: 'System health' })).toBeInTheDocument();
		expect(screen.getByRole('heading', { name: 'Query error rate' })).toBeInTheDocument();
		expect(screen.getByRole('heading', { name: 'Storage usage' })).toBeInTheDocument();
		expect(screen.getByRole('heading', { name: 'Components' })).toBeInTheDocument();

		expect(screen.getByText('800 ms')).toBeInTheDocument();
		expect(screen.getByText('0.00%')).toBeInTheDocument();
		expect(screen.getByText(/4\.0 KiB/)).toBeInTheDocument();
		expect(screen.getByText(/7 series/)).toBeInTheDocument();
		expect(screen.getByRole('rowheader', { name: 'gateway' })).toBeInTheDocument();
		expect(screen.getByRole('rowheader', { name: 'writer' })).toBeInTheDocument();
		expect(screen.getAllByText('Degraded').length).toBeGreaterThan(0);
		expect(screen.getByText('Firing')).toBeInTheDocument();
		expect(fetchMock).toHaveBeenCalledWith('/api/v1/health/overview');
	});

	it('announces an unavailable overview and keeps retrying', async () => {
		mockOverview(overview, false, 503);
		render(Page);

		const alert = await screen.findByRole('alert');
		expect(alert).toHaveTextContent('Health overview unavailable');
		expect(alert).toHaveTextContent('HTTP 503');
		expect(alert).toHaveTextContent('Retrying every 10 seconds');
	});
});
