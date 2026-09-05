package main

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// setupTracing wires up OpenTelemetry tracing when OTEL_EXPORTER_OTLP_ENDPOINT
// is set. If it's unset, tracing stays disabled: otel's default no-op tracer
// is used and this makes no network calls. The returned shutdown func should
// be called (best-effort) before the process exits so buffered spans flush.
func setupTracing(ctx context.Context) (shutdown func(context.Context) error, err error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	serviceName := envDefault("OTEL_SERVICE_NAME", "tasting-journals-app")

	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
	if os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") != "false" {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(opts...))
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	slog.Default().Info("tracing enabled", "endpoint", endpoint, "service_name", serviceName)
	return provider.Shutdown, nil
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
