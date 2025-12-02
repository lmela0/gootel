package gootel

import (
	"fmt"

	"github.com/lmela0/gootel/gonfigurator"
)

// embed basic gootel configuration
type GOOtelConfig struct {
	ServiceName     string `map:"OTEL_SERVICE_NAME"`
	ServiceVersion  string `map:"OTEL_SERVICE_VERSION"`
	LogsEndpoint    string `map:"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"`
	TracesEndpoint  string `map:"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"`
	MetricsEndpoint string `map:"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"`
}

func (cfg *GOOtelConfig) LoadWithDefaults(opts gonfigurator.Options) {
	localGrpcEndpoint := "localhost:4317"
	if err := gonfigurator.LoadConfig(cfg, opts); err != nil {
		fmt.Println("error loading config")
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "default-service"
	}
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = "0.0"
	}
	if cfg.LogsEndpoint == "" {
		cfg.LogsEndpoint = localGrpcEndpoint
	}
	if cfg.TracesEndpoint == "" {
		cfg.TracesEndpoint = localGrpcEndpoint
	}
	if cfg.MetricsEndpoint == "" {
		cfg.MetricsEndpoint = localGrpcEndpoint
	}
}

// embed basic info needed by a metric
type QuantitativeMetric struct {
	Name        string
	Unit        string
	Description string
}
