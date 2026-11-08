// Command quickstart sends one trace, two logs and two metrics to Observaki
// over OTLP/HTTP. It is the Go sample behind the Story 5.3 quickstart.
//
//	go run .
//	OBSERVAKI_OTLP_ENDPOINT=http://127.0.0.1:4318 \
//	OBSERVAKI_INGEST_TOKEN=observaki-local-dev-token go run .
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otlplog "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otlpmetric "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otlptrace "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	apilog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultEndpoint = "http://127.0.0.1:4318"
	defaultToken    = "observaki-local-dev-token"
	defaultService  = "observaki-quickstart"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("quickstart: %v", err)
	}
}

func run() error {
	endpoint := envOr("OBSERVAKI_OTLP_ENDPOINT", defaultEndpoint)
	token := envOr("OBSERVAKI_INGEST_TOKEN", defaultToken)
	serviceName := envOr("OTEL_SERVICE_NAME", defaultService)

	base, err := url.Parse(endpoint)
	if err != nil || base.Host == "" {
		return fmt.Errorf("invalid OBSERVAKI_OTLP_ENDPOINT %q: use a full URL such as %s", endpoint, defaultEndpoint)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", serviceName),
		attribute.String("service.version", "0.1.0"),
		attribute.String("deployment.environment.name", "local"),
	))
	if err != nil {
		return fmt.Errorf("build resource: %w", err)
	}

	headers := map[string]string{"Authorization": "Bearer " + token}
	secure := base.Scheme == "https"
	commonTraceOpts := []otlptrace.Option{
		otlptrace.WithEndpoint(base.Host),
		otlptrace.WithHeaders(headers),
	}
	if !secure {
		commonTraceOpts = append(commonTraceOpts, otlptrace.WithInsecure())
	}
	traceExporter, err := otlptrace.New(ctx, commonTraceOpts...)
	if err != nil {
		return fmt.Errorf("create trace exporter: %w", err)
	}
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter),
	)

	commonMetricOpts := []otlpmetric.Option{
		otlpmetric.WithEndpoint(base.Host),
		otlpmetric.WithHeaders(headers),
	}
	if !secure {
		commonMetricOpts = append(commonMetricOpts, otlpmetric.WithInsecure())
	}
	metricExporter, err := otlpmetric.New(ctx, commonMetricOpts...)
	if err != nil {
		return fmt.Errorf("create metric exporter: %w", err)
	}
	metricProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(2*time.Second))),
	)

	commonLogOpts := []otlplog.Option{
		otlplog.WithEndpoint(base.Host),
		otlplog.WithHeaders(headers),
	}
	if !secure {
		commonLogOpts = append(commonLogOpts, otlplog.WithInsecure())
	}
	logExporter, err := otlplog.New(ctx, commonLogOpts...)
	if err != nil {
		return fmt.Errorf("create log exporter: %w", err)
	}
	logProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
	)

	otel.SetTracerProvider(traceProvider)
	emitTrace(ctx, traceProvider)
	emitLogs(ctx, logProvider)
	if err := emitMetrics(ctx, metricProvider); err != nil {
		return err
	}

	flushCtx, flushCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer flushCancel()
	var errs []error
	errs = append(errs, traceProvider.ForceFlush(flushCtx))
	errs = append(errs, logProvider.ForceFlush(flushCtx))
	errs = append(errs, metricProvider.ForceFlush(flushCtx))
	errs = append(errs, traceProvider.Shutdown(flushCtx))
	errs = append(errs, logProvider.Shutdown(flushCtx))
	errs = append(errs, metricProvider.Shutdown(flushCtx))
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("flush telemetry: %w", err)
	}

	fmt.Printf("sent 2 spans, 2 log records and 2 metric points to %s (service %s)\n", endpoint, serviceName)
	return nil
}

func emitTrace(ctx context.Context, provider *sdktrace.TracerProvider) {
	tracer := provider.Tracer("observaki-quickstart")
	ctx, span := tracer.Start(ctx, "POST /checkout",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.request.method", "POST"),
			attribute.String("http.route", "/checkout"),
		),
	)
	_, child := tracer.Start(ctx, "SELECT cart_items",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system.name", "postgresql"),
			attribute.String("db.operation.name", "SELECT"),
		),
	)
	child.End()
	span.End()
}

func emitLogs(ctx context.Context, provider *sdklog.LoggerProvider) {
	logger := provider.Logger("observaki-quickstart")
	info := apilog.Record{}
	info.SetTimestamp(time.Now())
	info.SetSeverity(apilog.SeverityInfo)
	info.SetBody(attribute.StringValue("checkout completed"))
	info.AddAttributes(
		attribute.String("http.route", "/checkout"),
		attribute.Int("http.response.status_code", 200),
	)
	logger.Emit(ctx, info)

	warn := apilog.Record{}
	warn.SetTimestamp(time.Now())
	warn.SetSeverity(apilog.SeverityWarn)
	warn.SetBody(attribute.StringValue("payment retry scheduled"))
	warn.AddAttributes(
		attribute.String("http.route", "/checkout"),
		attribute.Int("retry.attempt", 1),
	)
	logger.Emit(ctx, warn)
}

func emitMetrics(ctx context.Context, provider *sdkmetric.MeterProvider) error {
	meter := provider.Meter("observaki-quickstart")
	counter, err := meter.Int64Counter("observaki_quickstart_requests_total",
		metric.WithDescription("Sample checkout requests handled by the quickstart app."),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("create counter: %w", err)
	}
	histogram, err := meter.Float64Histogram("observaki_quickstart_request_duration_seconds",
		metric.WithDescription("Sample checkout latency recorded by the quickstart app."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("create histogram: %w", err)
	}
	attrs := metric.WithAttributes(attribute.String("http.route", "/checkout"))
	counter.Add(ctx, 1, attrs)
	histogram.Record(ctx, 0.042, attrs)
	return nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
