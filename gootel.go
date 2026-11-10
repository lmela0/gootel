package gootel

import (
	"context"
	"errors"
	"fmt"
	stdlog "log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	ometric "go.opentelemetry.io/otel/metric"
	otrace "go.opentelemetry.io/otel/trace"

	otelsdklog "go.opentelemetry.io/otel/sdk/log"
	otelsdkmetric "go.opentelemetry.io/otel/sdk/metric"
	otelsdkresource "go.opentelemetry.io/otel/sdk/resource"
	otelsdktrace "go.opentelemetry.io/otel/sdk/trace"

	otlploggrpc "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otlpmetricgrpc "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otlptracegrpc "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"

	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

const shutdownTimeout = 5 * time.Second

// shutdowner is implemented by all OpenTelemetry providers.
type shutdowner interface {
	Shutdown(context.Context) error
}

// otlpEndpoint is the normalized form of a configured OTLP endpoint.
type otlpEndpoint struct {
	host   string
	secure bool
}

// parseEndpoint accepts both "host:port" and full URLs such as
// "http://collector:4317". Plain host:port endpoints are considered insecure,
// while https URLs enable TLS.
func parseEndpoint(raw string) otlpEndpoint {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return otlpEndpoint{host: DefaultEndpoint}
	}
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
		return otlpEndpoint{
			host:   u.Host,
			secure: strings.EqualFold(u.Scheme, "https"),
		}
	}
	return otlpEndpoint{host: raw}
}

// GOOtel embed OpenTelemetry datastructures to orchestrate telemetry
type GOOtel struct {
	logProvider   *otelsdklog.LoggerProvider
	logger        *stdlog.Logger
	meterProvider *otelsdkmetric.MeterProvider
	meter         ometric.Meter
	traceProvider *otelsdktrace.TracerProvider
	tracer        otrace.Tracer
}

// NewGOOtel builds a GOOtel object using the selected configuration.
// If config is nil, defaults are loaded from the process environment.
func NewGOOtel(ctx context.Context, config *GOOtelConfig) (*GOOtel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if config == nil {
		config = &GOOtelConfig{}
	}
	if err := config.applyEnvDefaults(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConfigLoad, err)
	}

	resource, err := newOTelResource(ctx, config.ServiceName, config.ServiceVersion)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOtelNewResource, err)
	}

	// Track every successfully created provider so a partial initialization
	// can be cleaned up without leaking exporters or background goroutines.
	var created []shutdowner
	cleanup := func(cause error) error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		for i := len(created) - 1; i >= 0; i-- {
			if err := created[i].Shutdown(shutdownCtx); err != nil {
				cause = errors.Join(cause, err)
			}
		}
		return cause
	}

	lp, err := newLogProvider(ctx, resource, config.LogsEndpoint)
	if err != nil {
		return nil, cleanup(fmt.Errorf("%w: %w", ErrOtelNewLogProvider, err))
	}
	created = append(created, lp)

	logger, err := newLogger(config.ServiceName, lp)
	if err != nil {
		return nil, cleanup(fmt.Errorf("%w: %w", ErrOtelNewLogger, err))
	}

	mp, err := newMeterProvider(ctx, resource, config.MetricsEndpoint)
	if err != nil {
		return nil, cleanup(fmt.Errorf("%w: %w", ErrOtelNewMeterProvider, err))
	}
	created = append(created, mp)

	tp, err := newTracerProvider(ctx, resource, config.TracesEndpoint)
	if err != nil {
		return nil, cleanup(fmt.Errorf("%w: %w", ErrOtelNewTracerProvider, err))
	}
	created = append(created, tp)

	// Publish the providers only after the whole setup succeeded.
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetLoggerProvider(lp)

	ot := &GOOtel{
		logProvider:   lp,
		logger:        logger,
		meterProvider: mp,
		meter:         mp.Meter(config.ServiceName),
		traceProvider: tp,
		tracer:        tp.Tracer(config.ServiceName),
	}
	logger.Info("OpenTelemetry service configured and ready",
		stdlog.String("metrics_endpoint", config.MetricsEndpoint),
		stdlog.String("traces_endpoint", config.TracesEndpoint),
		stdlog.String("logs_endpoint", config.LogsEndpoint),
	)
	return ot, nil
}

// newOTelResource creates and initializes an OTel resource. The default
// resource is merged so SDK and OTEL_RESOURCE_ATTRIBUTES metadata are kept.
func newOTelResource(ctx context.Context, serviceName, serviceVersion string) (*otelsdkresource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(serviceVersion),
		attribute.String("application.language", "go"),
	}
	if hostName, err := os.Hostname(); err == nil && hostName != "" {
		attrs = append(attrs, semconv.HostName(hostName))
	}
	return otelsdkresource.Merge(
		otelsdkresource.DefaultWithContext(ctx),
		otelsdkresource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
}

// newLogProvider create and initialize an otel LoggerProvider
func newLogProvider(ctx context.Context, res *otelsdkresource.Resource, endpoint string) (*otelsdklog.LoggerProvider, error) {
	ep := parseEndpoint(endpoint)
	opts := []otlploggrpc.Option{otlploggrpc.WithEndpoint(ep.host)}
	if !ep.secure {
		opts = append(opts, otlploggrpc.WithInsecure())
	}
	logExporter, err := otlploggrpc.New(ctx, opts...)
	if err != nil {
		return nil, ErrLogExporter(err)
	}
	loggerProvider := otelsdklog.NewLoggerProvider(
		otelsdklog.WithProcessor(
			otelsdklog.NewBatchProcessor(logExporter),
		),
		otelsdklog.WithResource(res),
	)
	return loggerProvider, nil
}

// newTracerProvider create and initialize an otel TracerProvider
func newTracerProvider(ctx context.Context, res *otelsdkresource.Resource, endpoint string) (*otelsdktrace.TracerProvider, error) {
	ep := parseEndpoint(endpoint)
	clientOpts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(ep.host)}
	if !ep.secure {
		clientOpts = append(clientOpts, otlptracegrpc.WithInsecure())
	}
	traceExporter, err := otlptracegrpc.New(ctx, clientOpts...)
	if err != nil {
		return nil, ErrTraceExporter(err)
	}
	tp := otelsdktrace.NewTracerProvider(
		otelsdktrace.WithBatcher(traceExporter),
		otelsdktrace.WithResource(res),
	)
	return tp, nil
}

// newMeterProvider create and initialize an otel MeterProvider
func newMeterProvider(ctx context.Context, res *otelsdkresource.Resource, endpoint string) (*otelsdkmetric.MeterProvider, error) {
	ep := parseEndpoint(endpoint)
	metricClientOpts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(ep.host)}
	if !ep.secure {
		metricClientOpts = append(metricClientOpts, otlpmetricgrpc.WithInsecure())
	}
	metricExporter, err := otlpmetricgrpc.New(ctx, metricClientOpts...)
	if err != nil {
		return nil, ErrMetricExporter(err)
	}
	// how often
	readingPeriod := 15 * time.Second
	mp := otelsdkmetric.NewMeterProvider(
		otelsdkmetric.WithReader(
			otelsdkmetric.NewPeriodicReader(
				metricExporter,
				otelsdkmetric.WithInterval(readingPeriod),
			),
		),
		otelsdkmetric.WithResource(res),
	)
	return mp, nil
}

// concatOptions prepends a description and a unit to the provided options.
// T must be one of the otel/metric option interfaces; the values returned by
// WithDescription and WithUnit implement all of them.
func concatOptions[T any](descr, unit string, options ...T) []T {
	opts := []T{
		ometric.WithDescription(descr).(T),
		ometric.WithUnit(unit).(T),
	}
	opts = append(opts, options...)
	return opts
}

// ------------------------------------------------------------------------------------------------------------------------------
// ------------------------------------------------------------------------------------------------------------------------------
// ------------------------------------------------------------------------------------------------------------------------------
// ------------------------------------------------------------------------------------------------------------------------------
// ------------------------------------------------------------------------------------------------------------------------------

// Shutdown flushes and stops every provider created by NewGOOtel.
// It returns the combined errors from all providers.
func (ot *GOOtel) Shutdown() error {
	if ot == nil {
		return nil
	}
	ctxTimeout, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	var errs []error
	if ot.traceProvider != nil {
		if err := ot.traceProvider.Shutdown(ctxTimeout); err != nil {
			errs = append(errs, fmt.Errorf("shutdown tracer provider: %w", err))
		}
	}
	if ot.meterProvider != nil {
		if err := ot.meterProvider.Shutdown(ctxTimeout); err != nil {
			errs = append(errs, fmt.Errorf("shutdown meter provider: %w", err))
		}
	}
	if ot.logProvider != nil {
		if err := ot.logProvider.Shutdown(ctxTimeout); err != nil {
			errs = append(errs, fmt.Errorf("shutdown log provider: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (ot *GOOtel) LogWarn(msg string, args ...any) {
	if ot == nil || ot.logger == nil {
		return
	}
	ot.logger.Warn(msg, args...)
}

func (ot *GOOtel) LogInfo(msg string, args ...any) {
	if ot == nil || ot.logger == nil {
		return
	}
	ot.logger.Info(msg, args...)
}

func (ot *GOOtel) LogError(msg string, args ...any) {
	if ot == nil || ot.logger == nil {
		return
	}
	ot.logger.Error(msg, args...)
}

func (ot *GOOtel) Int64Histogram(qmetric QuantitativeMetric, options ...ometric.Int64HistogramOption) (ometric.Int64Histogram, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)
	histogram, err := ot.meter.Int64Histogram(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricHistogram(err)
	}
	return histogram, nil
}

func (ot *GOOtel) Int64UpDownCounter(qmetric QuantitativeMetric, options ...ometric.Int64UpDownCounterOption) (ometric.Int64UpDownCounter, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)
	counter, err := ot.meter.Int64UpDownCounter(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricCounter(err)
	}
	return counter, nil
}

func (ot *GOOtel) StartTrace(ctx context.Context, spanName string, opt []otrace.SpanStartOption) (context.Context, otrace.Span) {
	return ot.tracer.Start(ctx, spanName, opt...)
}

func (ot *GOOtel) Int64Counter(qmetric QuantitativeMetric, options ...ometric.Int64CounterOption) (ometric.Int64Counter, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)

	counter, err := ot.meter.Int64Counter(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricCounter(err)
	}
	return counter, nil
}

func (ot *GOOtel) Int64Gauge(qmetric QuantitativeMetric, options ...ometric.Int64GaugeOption) (ometric.Int64Gauge, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)
	counter, err := ot.meter.Int64Gauge(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricGauge(err)
	}
	return counter, nil
}

func (ot *GOOtel) Int64ObservableCounter(qmetric QuantitativeMetric, options ...ometric.Int64ObservableCounterOption) (ometric.Int64ObservableCounter, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)
	counter, err := ot.meter.Int64ObservableCounter(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricCounter(err)
	}
	return counter, nil
}

func (ot *GOOtel) Int64ObservableUpDownCounter(
	qmetric QuantitativeMetric,
	options ...ometric.Int64ObservableUpDownCounterOption,
) (ometric.Int64ObservableUpDownCounter, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)
	counter, err := ot.meter.Int64ObservableUpDownCounter(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricCounter(err)
	}
	return counter, nil
}

func (ot *GOOtel) Int64ObservableGauge(qmetric QuantitativeMetric, options ...ometric.Int64ObservableGaugeOption) (ometric.Int64ObservableGauge, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)
	counter, err := ot.meter.Int64ObservableGauge(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricGauge(err)
	}
	return counter, nil
}

func (ot *GOOtel) Float64Counter(qmetric QuantitativeMetric, options ...ometric.Float64CounterOption) (ometric.Float64Counter, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)
	counter, err := ot.meter.Float64Counter(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricCounter(err)
	}
	return counter, nil
}

func (ot *GOOtel) Float64UpDownCounter(qmetric QuantitativeMetric, options ...ometric.Float64UpDownCounterOption) (ometric.Float64UpDownCounter, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)
	counter, err := ot.meter.Float64UpDownCounter(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricCounter(err)
	}
	return counter, nil
}

func (ot *GOOtel) Float64Histogram(qmetric QuantitativeMetric, options ...ometric.Float64HistogramOption) (ometric.Float64Histogram, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)
	counter, err := ot.meter.Float64Histogram(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricHistogram(err)
	}
	return counter, nil
}

func (ot *GOOtel) Float64Gauge(qmetric QuantitativeMetric, options ...ometric.Float64GaugeOption) (ometric.Float64Gauge, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)
	counter, err := ot.meter.Float64Gauge(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricGauge(err)
	}
	return counter, nil
}

func (ot *GOOtel) Float64ObservableCounter(qmetric QuantitativeMetric, options ...ometric.Float64ObservableCounterOption) (ometric.Float64ObservableCounter, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)
	counter, err := ot.meter.Float64ObservableCounter(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricCounter(err)
	}
	return counter, nil
}

func (ot *GOOtel) Float64ObservableUpDownCounter(
	qmetric QuantitativeMetric,
	options ...ometric.Float64ObservableUpDownCounterOption,
) (ometric.Float64ObservableUpDownCounter, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)
	counter, err := ot.meter.Float64ObservableUpDownCounter(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricCounter(err)
	}
	return counter, nil
}

func (ot *GOOtel) Float64ObservableGauge(qmetric QuantitativeMetric, options ...ometric.Float64ObservableGaugeOption) (ometric.Float64ObservableGauge, error) {
	opts := concatOptions(qmetric.Description, qmetric.Unit, options...)
	counter, err := ot.meter.Float64ObservableGauge(
		qmetric.Name,
		opts...,
	)
	if err != nil {
		return nil, ErrMetricGauge(err)
	}
	return counter, nil
}
