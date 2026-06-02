package logging

import (
	"context"
	"io"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const loggerKey contextKey = "constell-logger"

// multiHandler fans out slog records to multiple handlers.
type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if err := h.Handle(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

// Init initializes slog logger with JSON format and OTel bridge.
func Init(serviceName, env string) *slog.Logger {
	return InitWithWriter(serviceName, env, os.Stdout)
}

// InitWithWriter initializes slog logger writing to the given writer.
func InitWithWriter(serviceName, env string, w io.Writer) *slog.Logger {
	jsonHandler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// Bridge to OTel LoggerProvider
	otelHandler := otelslog.NewHandler(serviceName)

	handler := &multiHandler{
		handlers: []slog.Handler{jsonHandler, otelHandler},
	}

	logger := slog.New(handler).With(
		"service", serviceName,
		"env", env,
	)
	return logger
}

// FromContext extracts logger from context. Returns default logger if not found.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// NewContext creates a context with the given logger.
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// WithTraceID returns a logger with trace_id from the OTel span.
func WithTraceID(logger *slog.Logger, ctx context.Context) *slog.Logger {
	spanCtx := spanContextFromContext(ctx)
	if spanCtx.HasTraceID() {
		return logger.With("trace_id", spanCtx.TraceID())
	}
	return logger
}

func spanContextFromContext(ctx context.Context) trace.SpanContext {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return trace.SpanContext{}
	}
	return span.SpanContext()
}
