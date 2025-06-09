package observability

import (
	// Удалим неиспользуемый импорт
	// "context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"streaming-server/pkg/types"
)

// TracingMiddleware добавляет distributed tracing
func TracingMiddleware() types.Middleware {
	tracer := otel.Tracer("streaming-server")
	
	return func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
		spanCtx, span := tracer.Start(ctx.Context(), "jsonrpc."+req.Method,
			trace.WithAttributes(
				attribute.String("jsonrpc.method", req.Method),
				attribute.String("jsonrpc.version", req.JSONRPC),
				attribute.String("transport", ctx.Transport),
				attribute.String("request_id", ctx.RequestID),
			),
		)
		defer span.End()
		
		// Обновляем контекст с span
		newCtx := types.NewRequestContext(spanCtx, ctx.Transport, ctx.RemoteAddr)
		newCtx.RequestID = ctx.RequestID
		newCtx.Span = span
		
		response, err := next(req, newCtx)
		
		if err != nil {
			span.RecordError(err)
			span.SetAttributes(attribute.Bool("error", true))
		} else if response != nil && response.Error != nil {
			span.SetAttributes(
				attribute.Bool("error", true),
				attribute.Int("error.code", response.Error.Code),
				attribute.String("error.message", response.Error.Message),
			)
		}
		
		return response, err
	}
}
