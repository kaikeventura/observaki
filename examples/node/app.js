// Send one trace, two logs and two metrics to Observaki over OTLP/HTTP.
//
// This is the Node.js sample behind the Story 5.3 quickstart.
//
//   npm install
//   npm start
//
// Environment:
//   OBSERVAKI_OTLP_ENDPOINT  base OTLP/HTTP endpoint (default http://127.0.0.1:4318)
//   OBSERVAKI_INGEST_TOKEN   bearer ingest token (default the local dev token)
//   OTEL_SERVICE_NAME        service.name resource attribute

import { SpanKind, trace, metrics } from "@opentelemetry/api";
import { SeverityNumber, logs } from "@opentelemetry/api-logs";
import { OTLPLogExporter } from "@opentelemetry/exporter-logs-otlp-http";
import { OTLPMetricExporter } from "@opentelemetry/exporter-metrics-otlp-http";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";
import { resourceFromAttributes } from "@opentelemetry/resources";
import { BatchLogRecordProcessor, LoggerProvider } from "@opentelemetry/sdk-logs";
import { PeriodicExportingMetricReader } from "@opentelemetry/sdk-metrics";
import { NodeSDK } from "@opentelemetry/sdk-node";
import {
	ATTR_DEPLOYMENT_ENVIRONMENT_NAME,
	ATTR_SERVICE_NAME,
	ATTR_SERVICE_VERSION,
} from "@opentelemetry/semantic-conventions";

const DEFAULT_ENDPOINT = "http://127.0.0.1:4318";
const DEFAULT_TOKEN = "observaki-local-dev-token";
const DEFAULT_SERVICE = "observaki-quickstart";

function envOr(key, fallback) {
	const value = (process.env[key] ?? "").trim();
	return value || fallback;
}

function signalUrl(base, path) {
	return `${base.replace(/\/+$/, "")}${path}`;
}

async function main() {
	const endpoint = envOr("OBSERVAKI_OTLP_ENDPOINT", DEFAULT_ENDPOINT);
	const token = envOr("OBSERVAKI_INGEST_TOKEN", DEFAULT_TOKEN);
	const serviceName = envOr("OTEL_SERVICE_NAME", DEFAULT_SERVICE);

	if (!/^https?:\/\//.test(endpoint)) {
		throw new Error(
			`invalid OBSERVAKI_OTLP_ENDPOINT ${JSON.stringify(endpoint)}: use a full URL such as ${DEFAULT_ENDPOINT}`,
		);
	}

	const headers = { Authorization: `Bearer ${token}` };
	const resource = resourceFromAttributes({
		[ATTR_SERVICE_NAME]: serviceName,
		[ATTR_SERVICE_VERSION]: "0.1.0",
		[ATTR_DEPLOYMENT_ENVIRONMENT_NAME]: "local",
	});

	// Register the log provider before NodeSDK.start(): the OpenTelemetry logs
	// API only accepts the first global provider. Creating it here keeps the
	// Authorization header on the log exporter (the SDK's built-in log pipeline
	// did not propagate the provider in the tested 0.301 release).
	const logProvider = new LoggerProvider({
		resource,
		processors: [
			new BatchLogRecordProcessor({
				exporter: new OTLPLogExporter({
					url: signalUrl(endpoint, "/v1/logs"),
					headers,
				}),
			}),
		],
	});
	logs.setGlobalLoggerProvider(logProvider);

	const sdk = new NodeSDK({
		resource,
		traceExporter: new OTLPTraceExporter({
			url: signalUrl(endpoint, "/v1/traces"),
			headers,
		}),
		metricReaders: [
			new PeriodicExportingMetricReader({
				exporter: new OTLPMetricExporter({
					url: signalUrl(endpoint, "/v1/metrics"),
					headers,
				}),
				exportIntervalMillis: 2000,
			}),
		],
	});
	sdk.start();

	const tracer = trace.getTracer("observaki-quickstart");
	tracer.startActiveSpan(
		"POST /checkout",
		{
			kind: SpanKind.SERVER,
			attributes: {
				"http.request.method": "POST",
				"http.route": "/checkout",
			},
		},
		(span) => {
			tracer.startActiveSpan(
				"SELECT cart_items",
				{
					kind: SpanKind.CLIENT,
					attributes: {
						"db.system.name": "postgresql",
						"db.operation.name": "SELECT",
					},
				},
				(child) => child.end(),
			);
			span.end();
		},
	);

	const logger = logs.getLogger("observaki-quickstart");
	logger.emit({
		severityNumber: SeverityNumber.INFO,
		severityText: "INFO",
		body: "checkout completed",
		attributes: {
			"http.route": "/checkout",
			"http.response.status_code": 200,
		},
	});
	logger.emit({
		severityNumber: SeverityNumber.WARN,
		severityText: "WARN",
		body: "payment retry scheduled",
		attributes: { "http.route": "/checkout", "retry.attempt": 1 },
	});

	const meter = metrics.getMeter("observaki-quickstart");
	const counter = meter.createCounter("observaki_quickstart_requests_total", {
		description: "Sample checkout requests handled by the quickstart app.",
		unit: "1",
	});
	const histogram = meter.createHistogram(
		"observaki_quickstart_request_duration_seconds",
		{
			description: "Sample checkout latency recorded by the quickstart app.",
			unit: "s",
		},
	);
	const attributes = { "http.route": "/checkout" };
	counter.add(1, attributes);
	histogram.record(0.042, attributes);

	await sdk.shutdown();
	await logProvider.forceFlush();
	await logProvider.shutdown();
	console.log(
		`sent 2 spans, 2 log records and 2 metric points to ${endpoint} (service ${serviceName})`,
	);
}

main().catch((error) => {
	console.error(`quickstart: ${error.message}`);
	process.exitCode = 1;
});
