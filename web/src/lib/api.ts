export type HealthStatus =
	| 'healthy'
	| 'degraded'
	| 'critical'
	| 'no_data'
	| 'idle'
	| 'not_configured'
	| 'disabled'
	| 'unknown';

export type Health = {
	status: string;
	version?: string;
};

export type IngestionHealth = {
	status: HealthStatus;
	lag_p95_seconds?: number;
	pending_messages?: number;
	dlq_messages?: number;
	written_events?: number;
	writer_connected?: boolean;
	gateway_connected?: boolean;
	gateway_published_events?: number;
	observed_at?: string;
	source?: string;
	detail?: string;
};

export type QueryHealth = {
	status: HealthStatus;
	requests: number;
	errors: number;
	client_errors: number;
	error_rate: number;
	since: string;
	detail?: string;
};

export type StorageHealth = {
	status: HealthStatus;
	clickhouse_bytes?: number;
	clickhouse_parts?: number;
	victoriametrics_bytes?: number;
	victoriametrics_series?: number;
	observed_at?: string;
	detail?: string;
};

export type ComponentHealth = {
	name: string;
	status: HealthStatus;
	latency_ms: number;
	detail?: string;
	observed_at: string;
};

export type AlertingHealth = {
	status: HealthStatus;
	enabled: boolean;
	last_evaluation_at?: string;
	evaluation_age_seconds?: number;
	stale_after_seconds?: number;
	firing: number;
	pending: number;
	notifications_pending: number;
	last_error?: string;
	detail?: string;
};

export type HealthOverview = {
	generated_at: string;
	status: HealthStatus;
	summary: string[];
	ingestion: IngestionHealth;
	query: QueryHealth;
	storage: StorageHealth;
	components: ComponentHealth[];
	alerting: AlertingHealth;
};

async function getJSON<T>(path: string): Promise<T> {
	const response = await fetch(path);
	if (!response.ok) {
		throw new Error(`HTTP ${response.status}`);
	}
	return (await response.json()) as T;
}

export function getHealth(): Promise<Health> {
	return getJSON<Health>('/api/v1/health');
}

export function getHealthOverview(): Promise<HealthOverview> {
	return getJSON<HealthOverview>('/api/v1/health/overview');
}
