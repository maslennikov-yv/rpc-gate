package integration

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"streaming-server/pkg/types"

	"github.com/stretchr/testify/assert"
)

// TestObservability_RequestTracing validates request tracing capabilities
func (suite *IntegrationTestSuite) TestObservability_RequestTracing() {
	// Test that requests are properly traced through the system
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(`{"message": "tracing test"}`),
		ID:      "trace-test-001",
	}

	startTime := time.Now()
	response := suite.makeHTTPRequest(request)
	duration := time.Since(startTime)

	// Verify response
	assert.NotNil(suite.T(), response)
	assert.Nil(suite.T(), response.Error)
	assert.Equal(suite.T(), "trace-test-001", response.ID)

	// Verify timing is reasonable (should be fast for echo)
	assert.Less(suite.T(), duration, 5*time.Second, "Request should complete quickly")

	// Verify response contains timestamp (from echo handler)
	result, ok := response.Result.(map[string]interface{})
	assert.True(suite.T(), ok)
	assert.Contains(suite.T(), result, "timestamp")
}

// TestObservability_ConcurrentRequestTracing validates tracing under load
func (suite *IntegrationTestSuite) TestObservability_ConcurrentRequestTracing() {
	const numRequests = 20
	var wg sync.WaitGroup
	results := make(chan *types.JSONRPCResponse, numRequests)
	timings := make(chan time.Duration, numRequests)

	// Launch concurrent requests
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()

			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(fmt.Sprintf(`{"message": "concurrent-%d"}`, requestID)),
				ID:      fmt.Sprintf("trace-concurrent-%d", requestID),
			}

			start := time.Now()
			response := suite.makeHTTPRequest(request)
			duration := time.Since(start)

			results <- response
			timings <- duration
		}(i)
	}

	wg.Wait()
	close(results)
	close(timings)

	// Analyze results
	successCount := 0
	var totalDuration time.Duration
	maxDuration := time.Duration(0)

	for response := range results {
		if response != nil && response.Error == nil {
			successCount++
		}
	}

	for duration := range timings {
		totalDuration += duration
		if duration > maxDuration {
			maxDuration = duration
		}
	}

	// Validate performance and success rate
	assert.Equal(suite.T(), numRequests, successCount, "All requests should succeed")
	
	avgDuration := totalDuration / time.Duration(numRequests)
	assert.Less(suite.T(), avgDuration, 2*time.Second, "Average response time should be reasonable")
	assert.Less(suite.T(), maxDuration, 5*time.Second, "Max response time should be reasonable")
}

// TestObservability_ErrorTracking validates error tracking and reporting
func (suite *IntegrationTestSuite) TestObservability_ErrorTracking() {
	errorRequests := []struct {
		name    string
		request types.JSONRPCRequest
	}{
		{
			name: "Method Not Found",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "nonexistent_method",
				ID:      "error-trace-1",
			},
		},
		{
			name: "Invalid Params",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "calculate",
				Params:  json.RawMessage(`{"invalid": "params"}`),
				ID:      "error-trace-2",
			},
		},
		{
			name: "Server Error",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "test_error",
				ID:      "error-trace-3",
			},
		},
	}

	for _, errorReq := range errorRequests {
		suite.Run(errorReq.name, func() {
			startTime := time.Now()
			response := suite.makeHTTPRequest(errorReq.request)
			duration := time.Since(startTime)

			// Verify error response structure
			assert.NotNil(suite.T(), response)
			assert.NotNil(suite.T(), response.Error)
			assert.Equal(suite.T(), errorReq.request.ID, response.ID)

			// Verify timing is still reasonable even for errors
			assert.Less(suite.T(), duration, 3*time.Second, "Error responses should be fast")

			// Verify error has proper structure
			assert.NotEmpty(suite.T(), response.Error.Message)
			assert.NotZero(suite.T(), response.Error.Code)
		})
	}
}

// TestObservability_PerformanceMetrics validates performance monitoring
func (suite *IntegrationTestSuite) TestObservability_PerformanceMetrics() {
	// Test different handler performance characteristics
	handlers := []struct {
		name     string
		method   string
		expected time.Duration
	}{
		{
			name:     "Fast Handler (echo)",
			method:   "echo",
			expected: 100 * time.Millisecond,
		},
		{
			name:     "Medium Handler (calculate)",
			method:   "calculate",
			expected: 200 * time.Millisecond,
		},
		{
			name:     "Status Handler",
			method:   "status",
			expected: 100 * time.Millisecond,
		},
	}

	for _, handler := range handlers {
		suite.Run(handler.name, func() {
			var params json.RawMessage
			switch handler.method {
			case "echo":
				params = json.RawMessage(`{"message": "performance test"}`)
			case "calculate":
				params = json.RawMessage(`{"operation": "add", "a": 1, "b": 2}`)
			default:
				params = nil
			}

			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  handler.method,
				Params:  params,
				ID:      fmt.Sprintf("perf-%s", handler.method),
			}

			start := time.Now()
			response := suite.makeHTTPRequest(request)
			duration := time.Since(start)

			// Verify successful response
			assert.NotNil(suite.T(), response)
			assert.Nil(suite.T(), response.Error)

			// Verify performance is within expected bounds
			assert.Less(suite.T(), duration, handler.expected*5, 
				"Handler %s took too long: %v", handler.method, duration)
		})
	}
}

// TestObservability_ProtocolMetrics validates metrics across different protocols
func (suite *IntegrationTestSuite) TestObservability_ProtocolMetrics() {
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "time",
		ID:      "protocol-metrics",
	}

	protocols := []struct {
		name string
		test func() (time.Duration, *types.JSONRPCResponse)
	}{
		{
			name: "HTTP",
			test: func() (time.Duration, *types.JSONRPCResponse) {
				start := time.Now()
				resp := suite.makeHTTPRequest(request)
				return time.Since(start), resp
			},
		},
		{
			name: "WebSocket",
			test: func() (time.Duration, *types.JSONRPCResponse) {
				start := time.Now()
				resp := suite.makeWebSocketRequest(request)
				return time.Since(start), resp
			},
		},
		{
			name: "TCP",
			test: func() (time.Duration, *types.JSONRPCResponse) {
				start := time.Now()
				resp := suite.makeTCPRequest(request)
				return time.Since(start), resp
			},
		},
	}

	for _, protocol := range protocols {
		suite.Run(protocol.name, func() {
			duration, response := protocol.test()

			// Verify successful response
			assert.NotNil(suite.T(), response)
			assert.Nil(suite.T(), response.Error)
			assert.Equal(suite.T(), "protocol-metrics", response.ID)

			// Verify reasonable performance
			assert.Less(suite.T(), duration, 2*time.Second, 
				"Protocol %s should respond quickly", protocol.name)

			// Verify response contains time data
			result, ok := response.Result.(map[string]interface{})
			assert.True(suite.T(), ok)
			assert.Contains(suite.T(), result, "time")
		})
	}
}
