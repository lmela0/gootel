package gootel

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lmela0/gootel/gonfigurator"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	ometric "go.opentelemetry.io/otel/metric"
	otrace "go.opentelemetry.io/otel/trace"

	stdlog "log/slog"

	otelsdklog "go.opentelemetry.io/otel/sdk/log"
	otelsdkmetric "go.opentelemetry.io/otel/sdk/metric"
	otelsdkresource "go.opentelemetry.io/otel/sdk/resource"
	otelsdktrace "go.opentelemetry.io/otel/sdk/trace"

	otlploggrpc "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otlpmetricgrpc "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otlptracegrpc "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"

	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// GOOtel embed OpenTelemetry datastructures to orchestrate telemetry
type GOOtel struct {
	logProvider   *otelsdklog.LoggerProvider
	logger        *stdlog.Logger
	meterProvider *otelsdkmetric.MeterProvider
	meter         ometric.Meter
	traceProvider *otelsdktrace.TracerProvider
	tracer        otrace.Tracer
}

// NewGOOtel build a GOOtel object using selected configuration
func NewGOOtel(ctx context.Context, config *GOOtelConfig) (*GOOtel, error) {
	if config == nil {
		config = &GOOtelConfig{}
		config.LoadWithDefaults(gonfigurator.EnvOption{})
	}
	resource, err := newOTelResource(config.ServiceName, config.ServiceVersion)
	if err != nil {
		// handle
		return nil, ErrOtelNewResource
	}
	lp, err := newLogProvider(ctx, resource, config.LogsEndpoint)
	if err != nil {
		// handle
		return nil, ErrOtelNewLogProvider

	}
	logger, err := newLogger(config.ServiceName, lp)
	if err != nil {
		// handle
		return nil, ErrOtelNewLogger
	}

	mp, err := newMeterProvider(ctx, resource, config.MetricsEndpoint)
	if err != nil {
		// handle
		return nil, ErrOtelNewMeterProvider

	}
	metricOpts := []ometric.MeterOption{}
	meter := mp.Meter(config.ServiceName, metricOpts...)

	tp, err := newTracerProvider(ctx, resource, config.TracesEndpoint)
	if err != nil {
		// handle
		return nil, ErrOtelNewTracerProvider
	}
	tracerOpts := []otrace.TracerOption{}
	tracer := tp.Tracer(config.ServiceName, tracerOpts...)
	otel := GOOtel{
		logProvider:   lp,
		logger:        logger,
		meterProvider: mp,
		meter:         meter,
		traceProvider: tp,
		tracer:        tracer,
	}
	fmt.Printf("OpenTelemetry service configured and ready.\nEndpoints:\n\tMetrics:%s\n\tTraces:%s\n\tLogs:%s\n",
		config.MetricsEndpoint, config.TracesEndpoint, config.LogsEndpoint,
	)
	return &otel, nil
}

// newOTelResource create and initialize an otel Resource
func newOTelResource(serviceName, serviceVersion string) (*otelsdkresource.Resource, error) {
	hostName, _ := os.Hostname()
	res := otelsdkresource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(serviceVersion),
		semconv.HostName(hostName),
		attribute.String("application.language", "go"),
	)
	return res, nil
}

// newLogProvider create and initialize an otel LoggerProvider
func newLogProvider(ctx context.Context, res *otelsdkresource.Resource, endpoint string) (*otelsdklog.LoggerProvider, error) {
	logExporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(endpoint),
		otlploggrpc.WithInsecure())
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
	clientOpts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(), // Usa l'insecure per default
	}
	traceExporter, err := otlptracegrpc.New(ctx, clientOpts...)
	if err != nil {
		return nil, ErrTraceExporter(err)
	}
	tp := otelsdktrace.NewTracerProvider(
		otelsdktrace.WithBatcher(traceExporter, otelsdktrace.WithExportTimeout(time.Second)),
		otelsdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}

// newMeterProvider create and initialize an otel MeterProvider
func newMeterProvider(ctx context.Context, res *otelsdkresource.Resource, endpoint string) (*otelsdkmetric.MeterProvider, error) {
	metricClientOpts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithInsecure(),
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
	otel.SetMeterProvider(mp)
	return mp, nil
}

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

func (ot *GOOtel) Shutdown() {
	// max 5 sec timeout
	ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	// Shutdown del Tracer Provider
	if err := ot.traceProvider.Shutdown(ctxTimeout); err != nil {
		fmt.Printf("error shutting down tracer provider: %v", err)
	}
	// Shutdown del Meter Provider
	if err := ot.meterProvider.Shutdown(ctxTimeout); err != nil {
		fmt.Printf("error shutting down meter provider: %v", err)
	}
	// Shutdown del Log Provider
	if err := ot.logProvider.Shutdown(ctxTimeout); err != nil {
		fmt.Printf("error shutting down log provider: %v", err)
	}
	fmt.Println("providers correctly closed.")
}

func (ot *GOOtel) LogWarn(msg string, args ...any) {
	ot.logger.Warn(msg, args...)
}

func (ot *GOOtel) LogInfo(msg string, args ...any) {
	ot.logger.Info(msg, args...)
}

func (ot *GOOtel) LogError(msg string, args ...any) {
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
