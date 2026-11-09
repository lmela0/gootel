package gootel

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/lmela0/gootel/gonfigurator"
)

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantHost   string
		wantSecure bool
	}{
		{"empty", "", DefaultEndpoint, false},
		{"host and port", "localhost:4317", "localhost:4317", false},
		{"spaces", "  collector:4317  ", "collector:4317", false},
		{"http url", "http://collector:4317", "collector:4317", false},
		{"https url", "https://collector:4317", "collector:4317", true},
		{"ipv6", "[::1]:4317", "[::1]:4317", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEndpoint(tt.raw)
			if got.host != tt.wantHost {
				t.Errorf("host = %q, want %q", got.host, tt.wantHost)
			}
			if got.secure != tt.wantSecure {
				t.Errorf("secure = %v, want %v", got.secure, tt.wantSecure)
			}
		})
	}
}

func clearOTelEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OTEL_SERVICE_NAME",
		"OTEL_SERVICE_VERSION",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
	} {
		t.Setenv(key, "")
	}
}

func TestGOOtelConfigLoadWithDefaults(t *testing.T) {
	clearOTelEnv(t)

	cfg := GOOtelConfig{}
	if err := cfg.LoadWithDefaults(gonfigurator.EnvOption{}); err != nil {
		t.Fatalf("LoadWithDefaults() error = %v", err)
	}
	if cfg.ServiceName != DefaultServiceName {
		t.Errorf("ServiceName = %q, want %q", cfg.ServiceName, DefaultServiceName)
	}
	if cfg.ServiceVersion != DefaultServiceVersion {
		t.Errorf("ServiceVersion = %q, want %q", cfg.ServiceVersion, DefaultServiceVersion)
	}
	if cfg.LogsEndpoint != DefaultEndpoint ||
		cfg.TracesEndpoint != DefaultEndpoint ||
		cfg.MetricsEndpoint != DefaultEndpoint {
		t.Errorf("endpoints = %q/%q/%q, want %q",
			cfg.LogsEndpoint, cfg.TracesEndpoint, cfg.MetricsEndpoint, DefaultEndpoint)
	}
}

func TestGOOtelConfigKeepsExplicitValues(t *testing.T) {
	clearOTelEnv(t)
	t.Setenv("OTEL_SERVICE_NAME", "env-service")

	cfg := GOOtelConfig{ServiceName: "explicit-service"}
	if err := cfg.applyEnvDefaults(); err != nil {
		t.Fatalf("applyEnvDefaults() error = %v", err)
	}
	if cfg.ServiceName != "explicit-service" {
		t.Errorf("ServiceName = %q, want explicit value to win", cfg.ServiceName)
	}
}

func TestGOOtelConfigEnvFallback(t *testing.T) {
	clearOTelEnv(t)
	t.Setenv("OTEL_SERVICE_NAME", "env-service")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4317")

	cfg := GOOtelConfig{}
	if err := cfg.applyEnvDefaults(); err != nil {
		t.Fatalf("applyEnvDefaults() error = %v", err)
	}
	if cfg.ServiceName != "env-service" {
		t.Errorf("ServiceName = %q, want env-service", cfg.ServiceName)
	}
	if cfg.LogsEndpoint != "http://collector:4317" ||
		cfg.TracesEndpoint != "http://collector:4317" ||
		cfg.MetricsEndpoint != "http://collector:4317" {
		t.Errorf("endpoints = %q/%q/%q, want generic endpoint fallback",
			cfg.LogsEndpoint, cfg.TracesEndpoint, cfg.MetricsEndpoint)
	}
}

func TestGOOtelConfigSpecificEndpointWins(t *testing.T) {
	clearOTelEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4317")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "http://logs:4317")

	cfg := GOOtelConfig{}
	if err := cfg.applyEnvDefaults(); err != nil {
		t.Fatalf("applyEnvDefaults() error = %v", err)
	}
	if cfg.LogsEndpoint != "http://logs:4317" {
		t.Errorf("LogsEndpoint = %q, want specific endpoint", cfg.LogsEndpoint)
	}
	if cfg.TracesEndpoint != "http://collector:4317" {
		t.Errorf("TracesEndpoint = %q, want generic endpoint", cfg.TracesEndpoint)
	}
}

func TestShutdownOnNilAndZeroValue(t *testing.T) {
	var nilOtel *GOOtel
	if err := nilOtel.Shutdown(); err != nil {
		t.Errorf("nil Shutdown() error = %v", err)
	}
	if err := (&GOOtel{}).Shutdown(); err != nil {
		t.Errorf("zero value Shutdown() error = %v", err)
	}
}

type recordingHandler struct {
	handlers int
	err      error
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) Handle(context.Context, slog.Record) error {
	h.handlers++
	return h.err
}
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func TestMultiHandlerDispatchesToAllHandlers(t *testing.T) {
	first := &recordingHandler{}
	second := &recordingHandler{err: errors.New("boom")}
	handler := NewMultiHandler(first, second)

	if err := handler.Handle(context.Background(), slog.Record{}); err == nil {
		t.Fatal("Handle() error = nil, want aggregated error")
	}
	if first.handlers != 1 {
		t.Errorf("first handler calls = %d, want 1", first.handlers)
	}
	if second.handlers != 1 {
		t.Errorf("second handler calls = %d, want 1", second.handlers)
	}
}
