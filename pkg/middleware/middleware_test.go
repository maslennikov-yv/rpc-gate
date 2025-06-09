package middleware

import (
	"context"
	"testing"

	"streaming-server/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChain(t *testing.T) {
	// Test empty chain
	chain := NewChain()
	assert.NotNil(t, chain)
	assert.Empty(t, chain.middlewares)

	// Test chain with middlewares
	m1 := func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
		return next(req, ctx)
	}
	m2 := func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
		return next(req, ctx)
	}
	chain = NewChain(m1, m2)
	assert.Len(t, chain.middlewares, 2)
}

func TestChain_Execute_EmptyChain(t *testing.T) {
	chain := NewChain()
	
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      "test-1",
	}
	
	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")
	
	// Handler that should be called directly
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "direct_call",
			ID:      req.ID,
		}, nil
	}
	
	response, err := chain.Execute(request, ctx, handler)
	
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "direct_call", response.Result)
}

func TestChain_Execute_SingleMiddleware(t *testing.T) {
	m1 := func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
		ctx.WithValue("m1_processed", true)
		return next(req, ctx)
	}
	
	chain := NewChain(m1)
	
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      "test-1",
	}
	
	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")
	
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		processed, exists := ctx.GetValue("m1_processed")
		assert.True(t, exists)
		assert.Equal(t, true, processed)
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  processed,
			ID:      req.ID,
		}, nil
	}
	
	response, err := chain.Execute(request, ctx, handler)
	
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, true, response.Result)
}

func TestChain_Execute_MultipleMiddlewares(t *testing.T) {
	var executionOrder []string
	
	m1 := func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
		executionOrder = append(executionOrder, "m1_before")
		response, err := next(req, ctx)
		executionOrder = append(executionOrder, "m1_after")
		return response, err
	}
	
	m2 := func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
		executionOrder = append(executionOrder, "m2_before")
		response, err := next(req, ctx)
		executionOrder = append(executionOrder, "m2_after")
		return response, err
	}
	
	chain := NewChain(m1, m2)
	
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      "test-1",
	}
	
	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")
	
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		executionOrder = append(executionOrder, "handler")
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "success",
			ID:      req.ID,
		}, nil
	}
	
	response, err := chain.Execute(request, ctx, handler)
	
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "success", response.Result)
	
	// Verify execution order: m1_before -> m2_before -> handler -> m2_after -> m1_after
	expectedOrder := []string{"m1_before", "m2_before", "handler", "m2_after", "m1_after"}
	assert.Equal(t, expectedOrder, executionOrder)
}

func TestChain_Execute_MiddlewareError(t *testing.T) {
	m1 := func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   types.NewInternalError("middleware error"),
			ID:      req.ID,
		}, nil
	}
	
	chain := NewChain(m1)
	
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      "test-1",
	}
	
	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")
	
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		t.Error("Handler should not be called when middleware returns error")
		return nil, nil
	}
	
	response, err := chain.Execute(request, ctx, handler)
	
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotNil(t, response.Error)
	assert.Equal(t, -32603, response.Error.Code)
}

func TestChain_Execute_MiddlewareSkipsNext(t *testing.T) {
	m1 := func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
		// Skip calling next and return directly
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "middleware_response",
			ID:      req.ID,
		}, nil
	}
	
	chain := NewChain(m1)
	
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      "test-1",
	}
	
	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")
	
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		t.Error("Handler should not be called when middleware skips next")
		return nil, nil
	}
	
	response, err := chain.Execute(request, ctx, handler)
	
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "middleware_response", response.Result)
}

func TestLoggingMiddleware(t *testing.T) {
	logConfig := LoggingConfig{
		Enabled:     true,
		Destination: LogDestinationStdout,
		Format:      LogFormatJSON,
		Level:       LogLevelInfo,
	}
	logger, err := NewLogger(logConfig)
	require.NoError(t, err)
	
	middleware := LoggingMiddleware(logger)
	
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      "test-1",
	}
	
	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")
	
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "success",
			ID:      req.ID,
		}, nil
	}
	
	response, err := middleware(request, ctx, handler)
	
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "success", response.Result)
}

func TestLoggingMiddleware_WithError(t *testing.T) {
	logConfig := LoggingConfig{
		Enabled:     true,
		Destination: LogDestinationStdout,
		Format:      LogFormatJSON,
		Level:       LogLevelInfo,
	}
	logger, err := NewLogger(logConfig)
	require.NoError(t, err)
	
	middleware := LoggingMiddleware(logger)
	
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      "test-1",
	}
	
	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")
	
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   types.NewInternalError("test error"),
			ID:      req.ID,
		}, nil
	}
	
	response, err := middleware(request, ctx, handler)
	
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotNil(t, response.Error)
}

// Benchmark tests
func BenchmarkChain_Execute_EmptyChain(b *testing.B) {
	chain := NewChain()
	
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      "bench-1",
	}
	
	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")
	
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "benchmark",
			ID:      req.ID,
		}, nil
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chain.Execute(request, ctx, handler)
	}
}

func BenchmarkChain_Execute_WithMiddleware(b *testing.B) {
	m1 := func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
		return next(req, ctx)
	}
	chain := NewChain(m1)
	
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      "bench-1",
	}
	
	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")
	
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "benchmark",
			ID:      req.ID,
		}, nil
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chain.Execute(request, ctx, handler)
	}
}
