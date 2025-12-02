package gootel

import (
	"context"
	"os"

	slog "log/slog"

	otelslog "go.opentelemetry.io/contrib/bridges/otelslog"
	otelsdklog "go.opentelemetry.io/otel/sdk/log"
)

// handler collector
type MultiHandler struct {
	handlers []slog.Handler
}

// newMultiHandler builds a MultiHandler
func newMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

func (h *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle the record using every enabled handler in the MultiHandler
func (h *MultiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, record.Level) {
			if err := handler.Handle(ctx, record); err != nil {
				return err
			}
		}
	}
	return nil
}

// WithAttrs add attrs to all handlers
func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return newMultiHandler(newHandlers...)
}

// WithGroup adds a group to all handlers
func (h *MultiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return newMultiHandler(newHandlers...)
}

func newLogger(serviceName string, lp *otelsdklog.LoggerProvider) (*slog.Logger, error) {
	// log > OTEL collector
	otelHandler := otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(lp))
	// JSON > stdout
	stdoutHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelInfo,
	})
	// combine handlers
	multiHandler := newMultiHandler(otelHandler, stdoutHandler)
	// wrap handler
	logger := slog.New(multiHandler)
	return logger, nil
}
