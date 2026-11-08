"""Send one trace, two logs and two metrics to Observaki over OTLP/HTTP.

This is the Python sample behind the Story 5.3 quickstart.

    python3 -m venv .venv && . .venv/bin/activate
    pip install -r requirements.txt
    python app.py

Environment:
    OBSERVAKI_OTLP_ENDPOINT  base OTLP/HTTP endpoint (default http://127.0.0.1:4318)
    OBSERVAKI_INGEST_TOKEN   bearer ingest token (default the local dev token)
    OTEL_SERVICE_NAME        service.name resource attribute
"""

from __future__ import annotations

import logging
import os
import sys

from opentelemetry import metrics, trace
from opentelemetry._logs import set_logger_provider
from opentelemetry.exporter.otlp.proto.http._log_exporter import OTLPLogExporter
from opentelemetry.exporter.otlp.proto.http.metric_exporter import OTLPMetricExporter
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk._logs import LoggerProvider, LoggingHandler
from opentelemetry.sdk._logs.export import BatchLogRecordProcessor
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import PeriodicExportingMetricReader
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

DEFAULT_ENDPOINT = "http://127.0.0.1:4318"
DEFAULT_TOKEN = "observaki-local-dev-token"
DEFAULT_SERVICE = "observaki-quickstart"
EXPORT_TIMEOUT_SECONDS = 10


def env_or(key: str, fallback: str) -> str:
    value = os.environ.get(key, "").strip()
    return value or fallback


def signal_endpoint(base: str, path: str) -> str:
    return base.rstrip("/") + path


def build_resource(service_name: str) -> Resource:
    return Resource.create(
        {
            "service.name": service_name,
            "service.version": "0.1.0",
            "deployment.environment.name": "local",
        }
    )


def main() -> int:
    endpoint = env_or("OBSERVAKI_OTLP_ENDPOINT", DEFAULT_ENDPOINT)
    token = env_or("OBSERVAKI_INGEST_TOKEN", DEFAULT_TOKEN)
    service_name = env_or("OTEL_SERVICE_NAME", DEFAULT_SERVICE)

    if not endpoint.startswith(("http://", "https://")):
        print(
            f"invalid OBSERVAKI_OTLP_ENDPOINT {endpoint!r}: use a full URL such as {DEFAULT_ENDPOINT}",
            file=sys.stderr,
        )
        return 2

    headers = {"Authorization": f"Bearer {token}"}
    resource = build_resource(service_name)

    trace_provider = TracerProvider(resource=resource)
    trace_provider.add_span_processor(
        BatchSpanProcessor(
            OTLPSpanExporter(
                endpoint=signal_endpoint(endpoint, "/v1/traces"),
                headers=headers,
                timeout=EXPORT_TIMEOUT_SECONDS,
            )
        )
    )
    trace.set_tracer_provider(trace_provider)

    log_provider = LoggerProvider(resource=resource)
    log_provider.add_log_record_processor(
        BatchLogRecordProcessor(
            OTLPLogExporter(
                endpoint=signal_endpoint(endpoint, "/v1/logs"),
                headers=headers,
                timeout=EXPORT_TIMEOUT_SECONDS,
            )
        )
    )
    set_logger_provider(log_provider)

    metric_reader = PeriodicExportingMetricReader(
        OTLPMetricExporter(
            endpoint=signal_endpoint(endpoint, "/v1/metrics"),
            headers=headers,
            timeout=EXPORT_TIMEOUT_SECONDS,
        ),
        export_interval_millis=2000,
    )
    meter_provider = MeterProvider(resource=resource, metric_readers=[metric_reader])
    metrics.set_meter_provider(meter_provider)

    logger = logging.getLogger("observaki-quickstart")
    logger.setLevel(logging.INFO)
    logger.propagate = False
    handler = LoggingHandler(level=logging.INFO, logger_provider=log_provider)
    logger.addHandler(handler)

    tracer = trace_provider.get_tracer("observaki-quickstart")
    with tracer.start_as_current_span(
        "POST /checkout",
        kind=trace.SpanKind.SERVER,
        attributes={"http.request.method": "POST", "http.route": "/checkout"},
    ):
        with tracer.start_as_current_span(
            "SELECT cart_items",
            kind=trace.SpanKind.CLIENT,
            attributes={"db.system.name": "postgresql", "db.operation.name": "SELECT"},
        ):
            pass

    logger.info(
        "checkout completed",
        extra={"http.route": "/checkout", "http.response.status_code": 200},
    )
    logger.warning(
        "payment retry scheduled",
        extra={"http.route": "/checkout", "retry.attempt": 1},
    )

    meter = meter_provider.get_meter("observaki-quickstart")
    counter = meter.create_counter(
        "observaki_quickstart_requests_total",
        unit="1",
        description="Sample checkout requests handled by the quickstart app.",
    )
    histogram = meter.create_histogram(
        "observaki_quickstart_request_duration_seconds",
        unit="s",
        description="Sample checkout latency recorded by the quickstart app.",
    )
    attributes = {"http.route": "/checkout"}
    counter.add(1, attributes)
    histogram.record(0.042, attributes)

    errors: list[str] = []
    if not trace_provider.force_flush(timeout_millis=EXPORT_TIMEOUT_SECONDS * 1000):
        errors.append("trace flush timed out")
    if not log_provider.force_flush(timeout_millis=EXPORT_TIMEOUT_SECONDS * 1000):
        errors.append("log flush timed out")
    if not meter_provider.force_flush(timeout_millis=EXPORT_TIMEOUT_SECONDS * 1000):
        errors.append("metric flush timed out")

    trace_provider.shutdown()
    log_provider.shutdown()
    meter_provider.shutdown()

    if errors:
        for error in errors:
            print(f"quickstart: {error}", file=sys.stderr)
        return 1

    print(
        f"sent 2 spans, 2 log records and 2 metric points to {endpoint} "
        f"(service {service_name})"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
