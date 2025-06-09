package dispatcher

import (
	"context"
	"encoding/json"
	"testing"

	"streaming-server/pkg/middleware"
	"streaming-server/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDispatcher(t *testing.T) {
	d := NewDispatcher()
	assert.NotNil(t, d)
	assert.Equal(t, 0, d.HandlerCount())
}

func TestDispatcher_RegisterHandler(t *testing.T) {
	d := NewDispatcher()

	// Test successful registration
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "test",
			ID:      req.ID,
		}, nil
	}

	d.RegisterHandler("test", handler)

	// Verify handler is registered
	methods := d.GetRegisteredMethods()
	assert.Contains(t, methods, "test")

	// Test overwriting existing handler
	newHandler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "new_test",
			ID:      req.ID,
		}, nil
	}

	d.RegisterHandler("test", newHandler)
	methods = d.GetRegisteredMethods()
	assert.Contains(t, methods, "test")
}

func TestDispatcher_SetMiddleware(t *testing.T) {
	d := NewDispatcher()

	// Create middleware chain
	m1 := func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
		return next(req, ctx)
	}
	chain := middleware.NewChain(m1)

	d.SetMiddleware(chain)
	// We can't directly test the middleware field since it's private
	// but we can test that it works in dispatch
}

func TestDispatcher_Dispatch_Success(t *testing.T) {
	d := NewDispatcher()

	// Register test handler
	expectedResult := map[string]interface{}{"message": "success"}
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  expectedResult,
			ID:      req.ID,
		}, nil
	}

	d.RegisterHandler("test", handler)

	// Create test request
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		Params:  json.RawMessage(`{"param": "value"}`),
		ID:      "test-1",
	}

	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	// Dispatch request
	response, err := d.Dispatch(request, ctx)

	// Verify response
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, expectedResult, response.Result)
	assert.Equal(t, "test-1", response.ID)
	assert.Nil(t, response.Error)
}

func TestDispatcher_Dispatch_MethodNotFound(t *testing.T) {
	d := NewDispatcher()

	// Create test request for non-existent method
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "nonexistent",
		ID:      "test-1",
	}

	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	// Dispatch request
	response, err := d.Dispatch(request, ctx)

	// Verify error response
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Nil(t, response.Result)
	assert.NotNil(t, response.Error)
	assert.Equal(t, -32601, response.Error.Code)
	assert.Contains(t, response.Error.Message, "Method not found")
	assert.Equal(t, "test-1", response.ID)
}

func TestDispatcher_Dispatch_HandlerError(t *testing.T) {
	d := NewDispatcher()

	// Register handler that returns error
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return nil, assert.AnError
	}

	d.RegisterHandler("error_test", handler)

	// Create test request
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "error_test",
		ID:      "test-1",
	}

	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	// Dispatch request
	response, err := d.Dispatch(request, ctx)

	// Verify error is returned
	assert.Error(t, err)
	assert.Nil(t, response)
}

func TestDispatcher_Dispatch_HandlerReturnsErrorResponse(t *testing.T) {
	d := NewDispatcher()

	// Register handler that returns error response
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   types.NewInvalidParamsError("Invalid parameters"),
			ID:      req.ID,
		}, nil
	}

	d.RegisterHandler("error_response_test", handler)

	// Create test request
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "error_response_test",
		ID:      "test-1",
	}

	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	// Dispatch request
	response, err := d.Dispatch(request, ctx)

	// Verify error response
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Nil(t, response.Result)
	assert.NotNil(t, response.Error)
	assert.Equal(t, -32602, response.Error.Code)
	assert.Equal(t, "test-1", response.ID)
}

func TestDispatcher_Dispatch_WithMiddleware(t *testing.T) {
	d := NewDispatcher()

	// Create middleware function
	middlewareFunc := func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
		ctx.WithValue("middleware_processed", true)
		return next(req, ctx)
	}

	// Create middleware chain
	chain := middleware.NewChain(middlewareFunc)
	d.SetMiddleware(chain)

	// Register handler that checks middleware processing
	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		processed, _ := ctx.GetValue("middleware_processed")
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  map[string]interface{}{"middleware_processed": processed},
			ID:      req.ID,
		}, nil
	}

	d.RegisterHandler("middleware_test", handler)

	// Test dispatch
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "middleware_test",
		ID:      "test-1",
	}

	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	response, err := d.Dispatch(request, ctx)

	require.NoError(t, err)
	require.NotNil(t, response)
	result, ok := response.Result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, result["middleware_processed"])
}

func TestDispatcher_Dispatch_NilRequest(t *testing.T) {
	d := NewDispatcher()
	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	response, err := d.Dispatch(nil, ctx)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "request cannot be nil")
}

func TestDispatcher_Dispatch_NilContext(t *testing.T) {
	d := NewDispatcher()

	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      "test-1",
	}

	response, err := d.Dispatch(request, nil)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "context cannot be nil")
}

func TestDispatcher_Dispatch_EmptyMethod(t *testing.T) {
	d := NewDispatcher()

	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "",
		ID:      "test-1",
	}

	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	response, err := d.Dispatch(request, ctx)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotNil(t, response.Error)
	assert.Equal(t, -32601, response.Error.Code)
}

// Benchmark tests
func BenchmarkDispatcher_Dispatch(b *testing.B) {
	d := NewDispatcher()

	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "benchmark",
			ID:      req.ID,
		}, nil
	}

	d.RegisterHandler("benchmark", handler)

	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "benchmark",
		ID:      "bench-1",
	}

	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.Dispatch(request, ctx)
	}
}

func BenchmarkDispatcher_RegisterHandler(b *testing.B) {
	d := NewDispatcher()

	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{JSONRPC: "2.0", Result: "test", ID: req.ID}, nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.RegisterHandler("test", handler)
	}
}

func TestDispatcher_GetRegisteredMethods(t *testing.T) {
	dispatcher := NewDispatcher()

	handler1 := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return nil, nil
	}

	handler2 := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return nil, nil
	}

	dispatcher.RegisterHandler("method1", handler1)
	dispatcher.RegisterHandler("method2", handler2)

	methods := dispatcher.GetRegisteredMethods()

	assert.Len(t, methods, 2)
	assert.Contains(t, methods, "method1")
	assert.Contains(t, methods, "method2")
}

func TestDispatcher_HandlerCount(t *testing.T) {
	dispatcher := NewDispatcher()

	assert.Equal(t, 0, dispatcher.HandlerCount())

	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return nil, nil
	}

	dispatcher.RegisterHandler("test1", handler)
	assert.Equal(t, 1, dispatcher.HandlerCount())

	dispatcher.RegisterHandler("test2", handler)
	assert.Equal(t, 2, dispatcher.HandlerCount())

	dispatcher.UnregisterHandler("test1")
	assert.Equal(t, 1, dispatcher.HandlerCount())
}

func TestDispatcher_UnregisterHandler(t *testing.T) {
	dispatcher := NewDispatcher()

	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "test",
			ID:      req.ID,
		}, nil
	}

	dispatcher.RegisterHandler("test", handler)
	assert.Equal(t, 1, dispatcher.HandlerCount())

	dispatcher.UnregisterHandler("test")
	assert.Equal(t, 0, dispatcher.HandlerCount())
}
