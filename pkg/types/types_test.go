package types

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test JSONRPCRequest
func TestJSONRPCRequest_IsNotification(t *testing.T) {
	tests := []struct {
		name     string
		request  JSONRPCRequest
		expected bool
	}{
		{
			name: "Request with string ID",
			request: JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "test",
				ID:      "test-1",
			},
			expected: false,
		},
		{
			name: "Request with number ID",
			request: JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "test",
				ID:      123,
			},
			expected: false,
		},
		{
			name: "Request with nil ID (notification)",
			request: JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "test",
				ID:      nil,
			},
			expected: true,
		},
		{
			name: "Request without ID field (notification)",
			request: JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "test",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.request.IsNotification()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJSONRPCRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request JSONRPCRequest
		valid   bool
	}{
		{
			name: "Valid request",
			request: JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "test",
				Params:  json.RawMessage(`{"param": "value"}`),
				ID:      "test-1",
			},
			valid: true,
		},
		{
			name: "Valid notification",
			request: JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "test",
				Params:  json.RawMessage(`{"param": "value"}`),
			},
			valid: true,
		},
		{
			name: "Invalid JSON-RPC version",
			request: JSONRPCRequest{
				JSONRPC: "1.0",
				Method:  "test",
				ID:      "test-1",
			},
			valid: false,
		},
		{
			name: "Missing method",
			request: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      "test-1",
			},
			valid: false,
		},
		{
			name: "Empty method",
			request: JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "",
				ID:      "test-1",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling/unmarshaling
			data, err := json.Marshal(tt.request)
			require.NoError(t, err)

			var unmarshaled JSONRPCRequest
			err = json.Unmarshal(data, &unmarshaled)
			require.NoError(t, err)

			// Basic validation checks
			if tt.valid {
				assert.Equal(t, "2.0", unmarshaled.JSONRPC)
				assert.NotEmpty(t, unmarshaled.Method)
			}
		})
	}
}

// Test JSONRPCResponse
func TestJSONRPCResponse_Validation(t *testing.T) {
	tests := []struct {
		name     string
		response JSONRPCResponse
		valid    bool
	}{
		{
			name: "Valid success response",
			response: JSONRPCResponse{
				JSONRPC: "2.0",
				Result:  "success",
				ID:      "test-1",
			},
			valid: true,
		},
		{
			name: "Valid error response",
			response: JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   NewInvalidRequestError("test error"),
				ID:      "test-1",
			},
			valid: true,
		},
		{
			name: "Invalid - both result and error",
			response: JSONRPCResponse{
				JSONRPC: "2.0",
				Result:  "success",
				Error:   NewInvalidRequestError("test error"),
				ID:      "test-1",
			},
			valid: false,
		},
		{
			name: "Invalid - neither result nor error",
			response: JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      "test-1",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling/unmarshaling
			data, err := json.Marshal(tt.response)
			require.NoError(t, err)

			var unmarshaled JSONRPCResponse
			err = json.Unmarshal(data, &unmarshaled)
			require.NoError(t, err)

			assert.Equal(t, "2.0", unmarshaled.JSONRPC)
			assert.Equal(t, tt.response.ID, unmarshaled.ID)

			if tt.valid {
				// Valid response should have either result or error, but not both
				hasResult := unmarshaled.Result != nil
				hasError := unmarshaled.Error != nil
				assert.True(t, hasResult != hasError, "Response should have either result or error, but not both")
			}
		})
	}
}

// Test RPCError
func TestRPCError_StandardErrors(t *testing.T) {
	tests := []struct {
		name         string
		errorFunc    func(interface{}) *RPCError
		expectedCode int
		data         interface{}
	}{
		{
			name:         "Parse Error",
			errorFunc:    NewParseError,
			expectedCode: -32700,
			data:         "Invalid JSON",
		},
		{
			name:         "Invalid Request Error",
			errorFunc:    NewInvalidRequestError,
			expectedCode: -32600,
			data:         "Invalid request structure",
		},
		{
			name:         "Method Not Found Error",
			errorFunc:    func(data interface{}) *RPCError { return NewMethodNotFoundError(data.(string)) },
			expectedCode: -32601,
			data:         "Unknown method",
		},
		{
			name:         "Invalid Params Error",
			errorFunc:    NewInvalidParamsError,
			expectedCode: -32602,
			data:         "Invalid parameters",
		},
		{
			name:         "Internal Error",
			errorFunc:    NewInternalError,
			expectedCode: -32603,
			data:         "Internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.errorFunc(tt.data)

			assert.NotNil(t, err)
			assert.Equal(t, tt.expectedCode, err.Code)
			assert.NotEmpty(t, err.Message)
			assert.Equal(t, tt.data, err.Data)

			// Test JSON marshaling
			data, jsonErr := json.Marshal(err)
			require.NoError(t, jsonErr)

			var unmarshaled RPCError
			jsonErr = json.Unmarshal(data, &unmarshaled)
			require.NoError(t, jsonErr)

			assert.Equal(t, err.Code, unmarshaled.Code)
			assert.Equal(t, err.Message, unmarshaled.Message)
		})
	}
}

func TestRPCError_CustomError(t *testing.T) {
	customErr := &RPCError{
		Code:    -32000,
		Message: "Custom application error",
		Data:    map[string]interface{}{"details": "Custom error details"},
	}

	// Test JSON marshaling/unmarshaling
	data, err := json.Marshal(customErr)
	require.NoError(t, err)

	var unmarshaled RPCError
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, customErr.Code, unmarshaled.Code)
	assert.Equal(t, customErr.Message, unmarshaled.Message)
	assert.Equal(t, customErr.Data, unmarshaled.Data)
}

// Test RequestContext
func TestNewRequestContext(t *testing.T) {
	ctx := context.Background()
	transport := "test-service"
	remoteAddr := "127.0.0.1:8080"

	reqCtx := NewRequestContext(ctx, transport, remoteAddr)

	assert.NotNil(t, reqCtx)
	assert.Equal(t, transport, reqCtx.Transport)
	assert.Equal(t, remoteAddr, reqCtx.RemoteAddr)
	assert.NotEmpty(t, reqCtx.RequestID)
	assert.WithinDuration(t, time.Now(), reqCtx.StartTime, time.Second)
	assert.NotNil(t, reqCtx.Context())
}

func TestRequestContext_WithValue(t *testing.T) {
	ctx := NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	// Test setting and getting values
	ctx.WithValue("key1", "value1")
	ctx.WithValue("key2", 123)
	ctx.WithValue("key3", map[string]interface{}{"nested": "value"})

	value1, exists1 := ctx.GetValue("key1")
	assert.True(t, exists1)
	assert.Equal(t, "value1", value1)

	value2, exists2 := ctx.GetValue("key2")
	assert.True(t, exists2)
	assert.Equal(t, 123, value2)

	value3, exists3 := ctx.GetValue("key3")
	assert.True(t, exists3)
	assert.Equal(t, map[string]interface{}{"nested": "value"}, value3)

	_, exists := ctx.GetValue("nonexistent")
	assert.False(t, exists)
}

func TestRequestContext_Duration(t *testing.T) {
	ctx := NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	// Wait a small amount of time
	time.Sleep(10 * time.Millisecond)

	duration := ctx.Duration()
	assert.True(t, duration >= 10*time.Millisecond)
	assert.True(t, duration < time.Second) // Should be reasonable
}

// Test Handler type
func TestHandlerFunc(t *testing.T) {
	// Test handler function signature
	handler := func(req *JSONRPCRequest, ctx *RequestContext) (*JSONRPCResponse, error) {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "test result",
			ID:      req.ID,
		}, nil
	}

	request := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      "test-1",
	}

	context := NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	response, err := handler(request, context)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, "test result", response.Result)
	assert.Equal(t, "test-1", response.ID)
}

// Benchmark tests
func BenchmarkRequestContext_WithValue(b *testing.B) {
	ctx := NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.WithValue("benchmark", i)
	}
}

func BenchmarkRequestContext_GetValue(b *testing.B) {
	ctx := NewRequestContext(context.Background(), "test-service", "127.0.0.1")
	ctx.WithValue("benchmark", "value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ctx.GetValue("benchmark")
	}
}

func BenchmarkJSONRPCRequest_IsNotification(b *testing.B) {
	request := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      nil,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = request.IsNotification()
	}
}
