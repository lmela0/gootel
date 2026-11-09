package gootel

import (
	"fmt"
	"os"

	"github.com/lmela0/gootel/gonfigurator"
)

const (
	// DefaultServiceName is used when OTEL_SERVICE_NAME is not configured.
	DefaultServiceName = "default-service"
	// DefaultServiceVersion is used when OTEL_SERVICE_VERSION is not configured.
	DefaultServiceVersion = "0.0"
	// DefaultEndpoint is the standard local OTLP/gRPC endpoint.
	DefaultEndpoint = "localhost:4317"
)

// embed basic gootel configuration
type GOOtelConfig struct {
	ServiceName     string `map:"OTEL_SERVICE_NAME"`
	ServiceVersion  string `map:"OTEL_SERVICE_VERSION"`
	LogsEndpoint    string `map:"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"`
	TracesEndpoint  string `map:"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"`
	MetricsEndpoint string `map:"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"`
}

// LoadWithDefaults loads the configuration from opts and fills the missing
// values with OpenTelemetry environment defaults.
func (cfg *GOOtelConfig) LoadWithDefaults(opts gonfigurator.Options) error {
	if cfg == nil {
		return gonfigurator.ErrStructNotValid
	}
	if err := gonfigurator.LoadConfig(cfg, opts); err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	cfg.applyDefaults()
	return nil
}

// applyEnvDefaults fills only the fields that are still empty using the
// standard OpenTelemetry environment variables. Explicit values always win.
func (cfg *GOOtelConfig) applyEnvDefaults() error {
	if cfg == nil {
		return gonfigurator.ErrStructNotValid
	}
	envCfg := GOOtelConfig{}
	if err := gonfigurator.LoadConfig(&envCfg, gonfigurator.EnvOption{}); err != nil {
		return fmt.Errorf("load environment configuration: %w", err)
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = envCfg.ServiceName
	}
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = envCfg.ServiceVersion
	}
	if cfg.LogsEndpoint == "" {
		cfg.LogsEndpoint = envCfg.LogsEndpoint
	}
	if cfg.TracesEndpoint == "" {
		cfg.TracesEndpoint = envCfg.TracesEndpoint
	}
	if cfg.MetricsEndpoint == "" {
		cfg.MetricsEndpoint = envCfg.MetricsEndpoint
	}
	cfg.applyDefaults()
	return nil
}

// applyDefaults fills the remaining empty fields with library defaults.
func (cfg *GOOtelConfig) applyDefaults() {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = DefaultServiceName
	}
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = DefaultServiceVersion
	}
	if cfg.LogsEndpoint == "" {
		cfg.LogsEndpoint = endpoint
	}
	if cfg.TracesEndpoint == "" {
		cfg.TracesEndpoint = endpoint
	}
	if cfg.MetricsEndpoint == "" {
		cfg.MetricsEndpoint = endpoint
	}
}

// embed basic info needed by a metric
type QuantitativeMetric struct {
	Name        string
	Unit        string
	Description string
}
