package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"streaming-server/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEchoHandler(t *testing.T) {
	tests := []struct {
		name           string
		params         json.RawMessage
		expectedResult map[string]interface{}
		expectError    bool
	}{
		{
			name:   "Valid echo with message",
			params: json.RawMessage(`{"message": "hello world"}`),
			expectedResult: map[string]interface{}{
				"echo": map[string]interface{}{"message": "hello world"},
			},
			expectError: false,
		},
		{
			name:   "Echo with multiple fields",
			params: json.RawMessage(`{"message": "test", "user": "john", "count": 42}`),
			expectedResult: map[string]interface{}{
				"echo": map[string]interface{}{
					"message": "test",
					"user":    "john",
					"count":   float64(42), // JSON numbers become float64
				},
			},
			expectError: false,
		},
		{
			name:   "Echo with nil params",
			params: nil,
			expectedResult: map[string]interface{}{
				"echo": map[string]interface{}(nil),
			},
			expectError: false,
		},
		{
			name:   "Echo with empty object",
			params: json.RawMessage(`{}`),
			expectedResult: map[string]interface{}{
				"echo": map[string]interface{}{},
			},
			expectError: false,
		},
		{
			name:        "Invalid JSON params",
			params:      json.RawMessage(`{"invalid": json}`),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  tt.params,
				ID:      "test-1",
			}

			ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

			response, err := EchoHandler(request, ctx)

			if tt.expectError {
				require.NotNil(t, response)
				assert.NotNil(t, response.Error)
				assert.Equal(t, -32700, response.Error.Code) // Parse error
				return
			}

			require.NoError(t, err)
			require.NotNil(t, response)
			assert.Equal(t, "2.0", response.JSONRPC)
			assert.Equal(t, "test-1", response.ID)
			assert.Nil(t, response.Error)

			result, ok := response.Result.(map[string]interface{})
			require.True(t, ok)

			// Check echo field
			expectedEcho := tt.expectedResult["echo"]
			if expectedEcho == nil {
				assert.Nil(t, result["echo"])
			} else {
				assert.Equal(t, expectedEcho, result["echo"])
			}

			// Check additional fields
			assert.Contains(t, result, "request_id")
			assert.Contains(t, result, "transport")
			assert.Contains(t, result, "timestamp")
			assert.Equal(t, ctx.RequestID, result["request_id"])
		})
	}
}

func TestCalculateHandler(t *testing.T) {
	tests := []struct {
		name           string
		params         json.RawMessage
		expectedResult float64
		expectError    bool
		expectedCode   int
	}{
		{
			name:           "Valid addition",
			params:         json.RawMessage(`{"operation": "add", "a": 5, "b": 3}`),
			expectedResult: 8,
			expectError:    false,
		},
		{
			name:           "Valid subtraction",
			params:         json.RawMessage(`{"operation": "subtract", "a": 10, "b": 4}`),
			expectedResult: 6,
			expectError:    false,
		},
		{
			name:           "Valid multiplication",
			params:         json.RawMessage(`{"operation": "multiply", "a": 6, "b": 7}`),
			expectedResult: 42,
			expectError:    false,
		},
		{
			name:           "Valid division",
			params:         json.RawMessage(`{"operation": "divide", "a": 15, "b": 3}`),
			expectedResult: 5,
			expectError:    false,
		},
		{
			name:           "Addition with + operator",
			params:         json.RawMessage(`{"operation": "+", "a": 2, "b": 3}`),
			expectedResult: 5,
			expectError:    false,
		},
		{
			name:           "Subtraction with - operator",
			params:         json.RawMessage(`{"operation": "-", "a": 10, "b": 3}`),
			expectedResult: 7,
			expectError:    false,
		},
		{
			name:           "Multiplication with * operator",
			params:         json.RawMessage(`{"operation": "*", "a": 4, "b": 5}`),
			expectedResult: 20,
			expectError:    false,
		},
		{
			name:           "Division with / operator",
			params:         json.RawMessage(`{"operation": "/", "a": 20, "b": 4}`),
			expectedResult: 5,
			expectError:    false,
		},
		{
			name:         "Division by zero",
			params:       json.RawMessage(`{"operation": "divide", "a": 10, "b": 0}`),
			expectError:  true,
			expectedCode: -32602, // Invalid params
		},
		{
			name:         "Invalid operation",
			params:       json.RawMessage(`{"operation": "modulo", "a": 10, "b": 3}`),
			expectError:  true,
			expectedCode: -32602, // Invalid params
		},
		{
			name:         "Missing operation",
			params:       json.RawMessage(`{"a": 5, "b": 3}`),
			expectError:  true,
			expectedCode: -32602, // Invalid params
		},
		{
			name:         "Missing operands",
			params:       json.RawMessage(`{"operation": "add"}`),
			expectError:  true,
			expectedCode: -32602, // Invalid params
		},
		{
			name:         "Missing a operand",
			params:       json.RawMessage(`{"operation": "add", "b": 3}`),
			expectError:  true,
			expectedCode: -32602, // Invalid params
		},
		{
			name:         "Missing b operand",
			params:       json.RawMessage(`{"operation": "add", "a": 5}`),
			expectError:  true,
			expectedCode: -32602, // Invalid params
		},
		{
			name:         "String operands",
			params:       json.RawMessage(`{"operation": "add", "a": "five", "b": "three"}`),
			expectError:  true,
			expectedCode: -32602, // Invalid params
		},
		{
			name:         "Nil params",
			params:       nil,
			expectError:  true,
			expectedCode: -32602, // Invalid params
		},
		{
			name:         "Invalid JSON",
			params:       json.RawMessage(`{"operation": "add", "a": 5, "b":}`),
			expectError:  true,
			expectedCode: -32700, // Parse error
		},
		{
			name:         "Empty operation",
			params:       json.RawMessage(`{"operation": "", "a": 5, "b": 3}`),
			expectError:  true,
			expectedCode: -32602, // Invalid params
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "calculate",
				Params:  tt.params,
				ID:      "test-1",
			}

			ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

			response, err := CalculateHandler(request, ctx)

			require.NoError(t, err)
			require.NotNil(t, response)
			assert.Equal(t, "2.0", response.JSONRPC)
			assert.Equal(t, "test-1", response.ID)

			if tt.expectError {
				assert.NotNil(t, response.Error)
				assert.Equal(t, tt.expectedCode, response.Error.Code)
				assert.Nil(t, response.Result)
				return
			}

			assert.Nil(t, response.Error)
			assert.NotNil(t, response.Result)

			result, ok := response.Result.(map[string]interface{})
			require.True(t, ok)

			// Check calculation result
			assert.Equal(t, tt.expectedResult, result["result"])
			assert.Contains(t, result, "operation")
			assert.Contains(t, result, "operands")
			assert.Contains(t, result, "request_id")
			assert.Equal(t, ctx.RequestID, result["request_id"])

			// Verify operands array
			operands, ok := result["operands"].([]float64)
			require.True(t, ok)
			assert.Len(t, operands, 2)
		})
	}
}

func TestStatusHandler(t *testing.T) {
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "status",
		ID:      "test-1",
	}

	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	response, err := StatusHandler(request, ctx)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, "test-1", response.ID)
	assert.Nil(t, response.Error)

	result, ok := response.Result.(map[string]interface{})
	require.True(t, ok)

	// Check required fields
	assert.Equal(t, "healthy", result["status"])
	assert.Contains(t, result, "timestamp")
	assert.Contains(t, result, "transport")
	assert.Contains(t, result, "request_id")
	assert.Contains(t, result, "version")
	assert.Contains(t, result, "uptime")

	// Verify types
	assert.IsType(t, "", result["timestamp"])
	assert.Equal(t, ctx.RequestID, result["request_id"])
	assert.Equal(t, "1.0.0", result["version"])
}

func TestTimeHandler(t *testing.T) {
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "time",
		ID:      "test-1",
	}

	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	response, err := TimeHandler(request, ctx)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, "test-1", response.ID)
	assert.Nil(t, response.Error)

	result, ok := response.Result.(map[string]interface{})
	require.True(t, ok)

	// Check required fields
	assert.Contains(t, result, "time")
	assert.Contains(t, result, "timestamp")
	assert.Contains(t, result, "formatted")
	assert.Contains(t, result, "unix")
	assert.Contains(t, result, "timezone")
	assert.Contains(t, result, "request_id")
	assert.Contains(t, result, "server_time")

	// Verify types and values
	assert.IsType(t, "", result["time"])
	assert.IsType(t, "", result["timestamp"])
	assert.IsType(t, "", result["formatted"])
	assert.IsType(t, int64(0), result["unix"])
	assert.Equal(t, "UTC", result["timezone"])
	assert.Equal(t, ctx.RequestID, result["request_id"])
}

func TestTestSlowHandler(t *testing.T) {
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test_slow",
		ID:      "test-1",
	}

	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	// Measure execution time
	start := ctx.StartTime
	response, err := TestSlowHandler(request, ctx)
	duration := ctx.Duration()

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, "test-1", response.ID)
	assert.Nil(t, response.Error)
	assert.Equal(t, "slow operation completed", response.Result)

	// Verify it actually took time (should be at least 2 seconds)
	assert.True(t, duration.Seconds() >= 2.0, "Handler should take at least 2 seconds")
	_ = start // Use the variable to avoid unused variable error
}

func TestConvertToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected float64
		success  bool
	}{
		{"float64", float64(42.5), 42.5, true},
		{"float32", float32(42.5), 42.5, true},
		{"int", int(42), 42.0, true},
		{"int32", int32(42), 42.0, true},
		{"int64", int64(42), 42.0, true},
		{"uint", uint(42), 42.0, true},
		{"uint32", uint32(42), 42.0, true},
		{"uint64", uint64(42), 42.0, true},
		{"string", "42", 0.0, false},
		{"bool", true, 0.0, false},
		{"nil", nil, 0.0, false},
		{"slice", []int{1, 2, 3}, 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, success := convertToFloat64(tt.input)
			assert.Equal(t, tt.success, success)
			if tt.success {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// Benchmark tests
func BenchmarkEchoHandler(b *testing.B) {
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(`{"message": "benchmark test"}`),
		ID:      "bench-1",
	}

	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = EchoHandler(request, ctx)
	}
}

func BenchmarkCalculateHandler(b *testing.B) {
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "calculate",
		Params:  json.RawMessage(`{"operation": "multiply", "a": 6, "b": 7}`),
		ID:      "bench-1",
	}

	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = CalculateHandler(request, ctx)
	}
}

func BenchmarkStatusHandler(b *testing.B) {
	request := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "status",
		ID:      "bench-1",
	}

	ctx := types.NewRequestContext(context.Background(), "test-service", "127.0.0.1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = StatusHandler(request, ctx)
	}
}
