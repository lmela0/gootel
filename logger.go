package gootel

import (
	"context"
	"errors"
	"os"

	slog "log/slog"

	otelslog "go.opentelemetry.io/contrib/bridges/otelslog"
	otelsdklog "go.opentelemetry.io/otel/sdk/log"
)

// MultiHandler dispatches log records to multiple slog handlers.
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler builds a MultiHandler from the provided slog handlers.
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
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

// Handle the record using every enabled handler in the MultiHandler.
// All handlers are always invoked; errors are aggregated.
func (h *MultiHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs []error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, record.Level) {
			if err := handler.Handle(ctx, record); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// WithAttrs add attrs to all handlers
func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return NewMultiHandler(newHandlers...)
}

// WithGroup adds a group to all handlers
func (h *MultiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return NewMultiHandler(newHandlers...)
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
	multiHandler := NewMultiHandler(otelHandler, stdoutHandler)
	// wrap handler
	logger := slog.New(multiHandler)
	return logger, nil
}
