package metrics

import (
	"context"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const meterName = "github.com/constell/constell/backend/pkg/metrics"

// HTTPMiddleware records HTTP request QPS / latency / error rate.
func HTTPMiddleware(handler http.Handler) http.Handler {
	meter := otel.Meter(meterName)
	requestTotal, _ := meter.Int64Counter(
		"http_request_total",
		metric.WithDescription("Total HTTP requests"),
	)
	requestDuration, _ := meter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("HTTP request duration in seconds"),
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		handler.ServeHTTP(wrapped, r)

		duration := time.Since(start).Seconds()
		attrs := metric.WithAttributes(
			attribute.String("method", r.Method),
			attribute.String("path", r.URL.Path),
			attribute.Int("status", wrapped.statusCode),
		)

		requestTotal.Add(r.Context(), 1, attrs)
		requestDuration.Record(r.Context(), duration, attrs)
	})
}

// ConnectRPCInterceptor records Connect-RPC call QPS / latency / error rate.
func ConnectRPCInterceptor() connect.UnaryInterceptorFunc {
	meter := otel.Meter(meterName)
	callTotal, _ := meter.Int64Counter(
		"connect_rpc_call_total",
		metric.WithDescription("Total Connect-RPC calls"),
	)

	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			procedure := req.Spec().Procedure

			code := "ok"
			if err != nil {
				if connectErr := new(connect.Error); connectErr != nil {
					code = connectErr.Code().String()
				} else {
					code = "unknown"
				}
			}

			callTotal.Add(ctx, 1,
				metric.WithAttributes(
					attribute.String("procedure", procedure),
					attribute.String("code", code),
				),
			)

			return resp, err
		}
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
