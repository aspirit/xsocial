package otelutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aspirit/xsocial/pkg/logger"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	metricSdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	// semconv v1.26.0 ensures stable attributes for OTLP and Prometheus exporters.
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
)

// Constants for telemetry provider names and exporter types.
const (
	NoopProviderName   = "noop"
	ExporterOTLPgRPC   = "grpc"
	ExporterNone       = "none"
	ExporterPrometheus = "prometheus"
)

// OtelProvider manages OpenTelemetry trace, metric, and log.
type OtelProvider struct {
	logger        *logger.Logger
	traceProvider *trace.TracerProvider
	meterProvider *metricSdk.MeterProvider
	prometheusSrv *http.Server
	Tracer        oteltrace.Tracer
	Meter         metric.Meter
}

// OtelInitConfig holds telemetry configuration.
type OtelInitConfig struct {
	ServiceName    string  // required
	Environment    string  // optional, e.g., dev, prod
	SampleRatio    float64 // 0.0 - 1.0
	EnableTrace    bool
	EnableMetric   bool
	EnableLog      bool
	TraceExporter  string // ExporterOTLPgRPC | ExporterNone
	TraceEndpoint  string
	MetricExporter string // ExporterOTLPgRPC | ExporterPrometheus | ExporterNone
	MetricEndpoint string
	LogExporter    string         // ExporterOTLPgRPC | ExporterNone
	PrometheusPort int            // required if MetricExporter=ExporterPrometheus
	PrometheusHost string         // bind host for /metrics; empty = all interfaces. Set "127.0.0.1" to restrict to loopback.
	Logger         *logger.Logger // optional injected logger
	LogLevel       string         // info, debug, etc.
	Timeout        time.Duration  // exporter init timeout
}

// Exporter configuration errors.
var (
	ErrUnsupportedTraceExporter  = errors.New("otel: unsupported trace exporter")
	ErrUnsupportedMetricExporter = errors.New("otel: unsupported metric exporter")
	ErrUnsupportedLogExporter    = errors.New("otel: unsupported log exporter")
)

var DefaultTelemetry *OtelProvider

// Default returns the global TelemetryProvider instance, or nil if not initialized.
func Default() *OtelProvider {
	return DefaultTelemetry
}

// Reset clears the global TelemetryProvider for testing.
func Reset() {
	DefaultTelemetry = nil
}

// Count increments a named business counter on the global telemetry provider.
// It is safe to call when telemetry is uninitialized (no-op), so domains can be
// instrumented without taking a hard dependency on metrics being configured.
func Count(ctx context.Context, name string, value int64) {
	if DefaultTelemetry != nil {
		DefaultTelemetry.RecordCounter(ctx, name, value)
	}
}

// Middleware creates a Gin middleware for OpenTelemetry tracing.
func Middleware(serviceName string) gin.HandlerFunc {
	opts := []otelgin.Option{}

	return otelgin.Middleware(serviceName, opts...)
}

// normalizeConfig sets default values for Config.
func normalizeConfig(cfg OtelInitConfig) OtelInitConfig {
	if cfg.SampleRatio < 0.0 || cfg.SampleRatio > 1.0 {
		cfg.SampleRatio = 1.0
	}
	if cfg.TraceExporter == "" {
		cfg.TraceExporter = ExporterOTLPgRPC
	}
	if cfg.MetricExporter == "" {
		cfg.MetricExporter = ExporterOTLPgRPC
	}
	if cfg.LogExporter == "" {
		cfg.LogExporter = ExporterNone
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}

	return cfg
}

// setupLogger initializes the zap logger.
func setupLogger(cfg OtelInitConfig) *logger.Logger {
	if cfg.Logger != nil {
		return cfg.Logger
	}

	return nil
}

// setupResource creates the OpenTelemetry resource.
func setupResource(ctx context.Context, cfg OtelInitConfig) (*resource.Resource, error) {
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	return res, nil
}

// setupTrace initializes the trace provider.
func setupTrace(ctx context.Context, cfg OtelInitConfig, res *resource.Resource) (*trace.TracerProvider, oteltrace.Tracer, error) {
	if !cfg.EnableTrace || cfg.TraceExporter == ExporterNone {
		return nil, otel.Tracer(NoopProviderName), nil
	}
	var traceExporter trace.SpanExporter
	switch cfg.TraceExporter {
	case ExporterOTLPgRPC:
		var err error
		traceExporter, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.TraceEndpoint),
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithTimeout(cfg.Timeout),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create trace exporter: %w", err)
		}
	default:
		return nil, nil, fmt.Errorf("%w: %s", ErrUnsupportedTraceExporter, cfg.TraceExporter)
	}
	traceProvider := trace.NewTracerProvider(
		trace.WithSampler(trace.TraceIDRatioBased(cfg.SampleRatio)),
		trace.WithBatcher(traceExporter),
		trace.WithResource(res),
	)
	otel.SetTracerProvider(traceProvider)

	return traceProvider, traceProvider.Tracer(cfg.ServiceName), nil
}

// setupMetric initializes the metric provider and optional Prometheus server.
func setupMetric(ctx context.Context, cfg OtelInitConfig, res *resource.Resource) (*metricSdk.MeterProvider, metric.Meter, *http.Server, error) {
	if !cfg.EnableMetric || cfg.MetricExporter == ExporterNone {
		return nil, otel.Meter(NoopProviderName), nil, nil
	}
	var mp *metricSdk.MeterProvider
	switch cfg.MetricExporter {
	case ExporterOTLPgRPC:
		metricExporter, err := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithEndpoint(cfg.MetricEndpoint),
			otlpmetricgrpc.WithInsecure(),
			otlpmetricgrpc.WithTimeout(cfg.Timeout),
		)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create metric exporter: %w", err)
		}
		mp = metricSdk.NewMeterProvider(
			metricSdk.WithReader(metricSdk.NewPeriodicReader(metricExporter)),
			metricSdk.WithResource(res),
		)
	case ExporterPrometheus:
		promExporter, err := prometheus.New()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create prometheus exporter: %w", err)
		}
		mp = metricSdk.NewMeterProvider(
			metricSdk.WithReader(promExporter),
			metricSdk.WithResource(res),
		)
		srv := &http.Server{
			// PrometheusHost defaults to "" (all interfaces) for backward compat;
			// set it to 127.0.0.1 (or restrict via firewall/NetworkPolicy) so the
			// metrics endpoint is not reachable from the public internet.
			Addr:              fmt.Sprintf("%s:%d", cfg.PrometheusHost, cfg.PrometheusPort),
			ReadHeaderTimeout: 30 * time.Second,
		}
		http.Handle("/metrics", promhttp.Handler())
		go func() {
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				// Log error asynchronously to avoid blocking
				zap.L().Error("Prometheus server failed", zap.Error(err))
			}
		}()

		otel.SetMeterProvider(mp)

		return mp, mp.Meter(cfg.ServiceName), srv, nil
	default:
		return nil, nil, nil, fmt.Errorf("%w: %s", ErrUnsupportedMetricExporter, cfg.MetricExporter)
	}
	otel.SetMeterProvider(mp)

	return mp, mp.Meter(cfg.ServiceName), nil, nil
}

// setupLog configures the log exporter (placeholder for OTLP).
func setupLog(cfg OtelInitConfig, logger *logger.Logger) error {
	if cfg.EnableLog && cfg.LogExporter != ExporterNone {
		if cfg.LogExporter == ExporterOTLPgRPC {
			// TODO: OTLP log exporter implementation is pending due to missing OpenTelemetry Zap bridge.
			// Currently logs a warning and falls back to zap logger.
			if logger == nil {
				return nil
			}
			logger.Warn("OTLP log exporter not implemented, using zap")
		} else {
			return fmt.Errorf("%w: %s", ErrUnsupportedLogExporter, cfg.LogExporter)
		}
	}

	return nil
}

// NewOtelProvider initializes telemetry based on config.
func NewOtelProvider(ctx context.Context, cfg OtelInitConfig) (*OtelProvider, error) {
	cfg = normalizeConfig(cfg)
	tp := &OtelProvider{}

	logger := setupLogger(cfg)
	tp.logger = logger

	res, err := setupResource(ctx, cfg)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	traceProvider, tracer, err := setupTrace(ctx, cfg, res)
	if err != nil {
		return nil, err
	}
	tp.traceProvider = traceProvider
	tp.Tracer = tracer

	meterProvider, meter, prometheusSrv, err := setupMetric(ctx, cfg, res)
	if err != nil {
		return nil, err
	}
	tp.meterProvider = meterProvider
	tp.Meter = meter
	tp.prometheusSrv = prometheusSrv

	if err := setupLog(cfg, logger); err != nil {
		return nil, err
	}

	return tp, nil
}

// StartSpan starts a new span if tracing is enabled.
func (tp *OtelProvider) StartSpan(ctx context.Context, name string) (context.Context, func()) {
	if tp.Tracer == otel.Tracer(NoopProviderName) {
		return ctx, func() {}
	}
	ctx, span := tp.Tracer.Start(ctx, name)

	return ctx, func() { span.End() }
}

func (tp *OtelProvider) SpanFromContext(ctx context.Context) oteltrace.Span {
	span := oteltrace.SpanFromContext(ctx)
	if !span.IsRecording() && tp.logger != nil {
		tp.logger.Debug("Retrieved no-op or invalid span from context")
	}

	return span
}

// RecordCounter increments a counter metric if metrics are enabled.
func (tp *OtelProvider) RecordCounter(ctx context.Context, name string, value int64) {
	if tp.Meter == otel.Meter(NoopProviderName) {
		return
	}
	counter, err := tp.Meter.Int64Counter(name)
	if err != nil {
		tp.logger.Error("Failed to create counter", zap.Error(err))

		return
	}
	counter.Add(ctx, value)
}

// LogInfo logs a message if logging is enabled.
func (tp *OtelProvider) LogInfo(ctx context.Context, msg string, fields ...zap.Field) {
	if tp.logger == nil {
		return
	}
	tp.logger.Info(msg, fields...)
}

// Shutdown gracefully shuts down telemetry providers.
func (tp *OtelProvider) Shutdown(ctx context.Context) {
	if tp.logger != nil {
		_ = tp.logger.Sync()
	}
	if tp.traceProvider != nil {
		_ = tp.traceProvider.Shutdown(ctx)
	}
	if tp.meterProvider != nil {
		_ = tp.meterProvider.Shutdown(ctx)
	}
	if tp.prometheusSrv != nil {
		_ = tp.prometheusSrv.Close()
	}
}
