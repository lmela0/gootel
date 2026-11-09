package gootel

import (
	"errors"
	"fmt"
)

var (
	ErrConfigLoad            error             = errors.New("error loading OTel configuration")
	ErrOtelNewResource       error             = errors.New("error creating new OTel resource")
	ErrOtelNewLogProvider    error             = errors.New("error creating new Log provider")
	ErrOtelNewLogger         error             = errors.New("error creating new Logger")
	ErrOtelNewMeterProvider  error             = errors.New("error creating new Meter provider")
	ErrOtelNewTracerProvider error             = errors.New("error creating new Tracer provider")
	ErrKeyNotFound           error             = errors.New("key not found")
	ErrOtelDefaultEndpoint   error             = errors.New("OTEL_EXPORTER_OTLP_xxxxx_ENDPOINT/OTEL_EXPORTER_OTLP_ENDPOINT not available, rely on default endpoint")
	ErrLogExporter           func(error) error = func(err error) error {
		return fmt.Errorf("failed to create log exporter: %w", err)
	}
	ErrTraceExporter func(error) error = func(err error) error {
		return fmt.Errorf("failed to create trace exporter: %w", err)
	}
	ErrMetricExporter func(error) error = func(err error) error {
		return fmt.Errorf("failed to create metric exporter: %w", err)
	}
	ErrMetricHistogram func(error) error = func(err error) error {
		return fmt.Errorf("failed to create histogram: %w", err)
	}
	ErrMetricCounter func(error) error = func(err error) error {
		return fmt.Errorf("failed to create counter: %w", err)
	}
	ErrMetricGauge func(error) error = func(err error) error {
		return fmt.Errorf("failed to create gauge: %w", err)
	}
)
