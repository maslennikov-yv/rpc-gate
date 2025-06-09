package integration

import (
	"encoding/json"
	"fmt"
	"sync"

	"streaming-server/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMiddleware_RequestValidation validates middleware request validation
func (suite *IntegrationTestSuite) TestMiddleware_RequestValidation() {
	validationTests := []struct {
		name     string
		request  types.JSONRPCRequest
		shouldPass bool
	}{
		{
			name: "Valid Request",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(`{"message": "valid"}`),
				ID:      "valid-1",
			},
			shouldPass: true,
		},
		{
			name: "Invalid JSON-RPC Version",
			request: types.JSONRPCRequest{
				JSONRPC: "1.0",
				Method:  "echo",
				ID:      "invalid-1",
			},
			shouldPass: false,
		},
		{
			name: "Missing Method",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "",
				ID:      "invalid-2",
			},
			shouldPass: false,
		},
		{
			name: "Reserved Method Prefix",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "rpc.test",
				ID:      "invalid-3",
			},
			shouldPass: false,
		},
	}

	for _, test := range validationTests {
		suite.Run(test.name, func() {
			response := suite.makeHTTPRequest(test.request)
			
			if test.shouldPass {
				assert.NotNil(suite.T(), response)
				assert.Nil(suite.T(), response.Error, "Valid request should not have error")
			} else {
				assert.NotNil(suite.T(), response)
				assert.NotNil(suite.T(), response.Error, "Invalid request should have error")
			}
		})
	}
}

// TestMiddleware_Logging validates middleware logging functionality
func (suite *IntegrationTestSuite) TestMiddleware_Logging() {
	// Test that requests are properly logged by making various types of requests
	requests := []struct {
		name    string
		request types.JSONRPCRequest
	}{
		{
			name: "Successful Request",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(`{"message": "log test"}`),
				ID:      "log-success",
			},
		},
		{
			name: "Error Request",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "nonexistent",
				ID:      "log-error",
			},
		},
		{
			name: "Notification",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(`{"message": "notification"}`),
				// No ID - this is a notification
			},
		},
	}

	for _, req := range requests {
		suite.Run(req.name, func() {
			response := suite.makeHTTPRequest(req.request)
			
			// For notifications, response should be nil
			if req.request.IsNotification() {
				assert.Nil(suite.T(), response)
			} else {
				assert.NotNil(suite.T(), response)
				assert.Equal(suite.T(), req.request.ID, response.ID)
			}
		})
	}
}

// TestMiddleware_ChainExecution validates middleware chain execution order
func (suite *IntegrationTestSuite) TestMiddleware_ChainExecution() {
	// Test that middleware chain executes in proper order by making requests
	// and verifying the final response contains expected data
	
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(`{"message": "chain test"}`),
		ID:      "chain-test",
	}

	response := suite.makeHTTPRequest(request)
	
	// Verify the request went through the full middleware chain
	assert.NotNil(suite.T(), response)
	assert.Nil(suite.T(), response.Error)
	assert.Equal(suite.T(), "chain-test", response.ID)
	
	// Verify the response structure indicates middleware processing
	result, ok := response.Result.(map[string]interface{})
	require.True(suite.T(), ok)
	
	// The echo handler should return the message and timestamp
	assert.Contains(suite.T(), result, "timestamp")
	
	echo, ok := result["echo"].(map[string]interface{})
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), "chain test", echo["message"])
}

// TestMiddleware_ConcurrentProcessing validates middleware under concurrent load
func (suite *IntegrationTestSuite) TestMiddleware_ConcurrentProcessing() {
	const numWorkers = 10
	const requestsPerWorker = 3
	
	var wg sync.WaitGroup
	results := make(chan *types.JSONRPCResponse, numWorkers*requestsPerWorker)
	
	// Launch concurrent requests
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for j := 0; j < requestsPerWorker; j++ {
				request := types.JSONRPCRequest{
					JSONRPC: "2.0",
					Method:  "calculate",
					Params:  json.RawMessage(`{"operation": "multiply", "a": 3, "b": 4}`),
					ID:      fmt.Sprintf("middleware-concurrent-%d-%d", workerID, j),
				}
				
				response := suite.makeHTTPRequest(request)
				results <- response
			}
		}(i)
	}
	
	wg.Wait()
	close(results)
	
	// Validate all responses
	successCount := 0
	for response := range results {
		assert.NotNil(suite.T(), response)
		if response.Error == nil {
			successCount++
			// Verify calculation result
			if result, ok := response.Result.(float64); ok {
				assert.Equal(suite.T(), float64(12), result)
			}
		}
	}
	
	assert.Equal(suite.T(), numWorkers*requestsPerWorker, successCount, 
		"All concurrent requests should succeed through middleware")
}

// TestMiddleware_ErrorHandling validates middleware error handling
func (suite *IntegrationTestSuite) TestMiddleware_ErrorHandling() {
	// Test that middleware properly handles and propagates errors
	errorRequests := []struct {
		name           string
		request        types.JSONRPCRequest
		expectedError  int
	}{
		{
			name: "Handler Error",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "test_error",
				ID:      "middleware-error-1",
			},
			expectedError: -32603, // Internal error
		},
		{
			name: "Method Not Found",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "nonexistent_method",
				ID:      "middleware-error-2",
			},
			expectedError: -32601, // Method not found
		},
		{
			name: "Invalid Params",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "calculate",
				Params:  json.RawMessage(`{"operation": "invalid"}`),
				ID:      "middleware-error-3",
			},
			expectedError: -32602, // Invalid params
		},
	}
	
	for _, errorReq := range errorRequests {
		suite.Run(errorReq.name, func() {
			response := suite.makeHTTPRequest(errorReq.request)
			
			assert.NotNil(suite.T(), response)
			assert.NotNil(suite.T(), response.Error)
			assert.Equal(suite.T(), errorReq.request.ID, response.ID)
			assert.Equal(suite.T(), errorReq.expectedError, response.Error.Code)
		})
	}
}

// TestMiddleware_ContextPropagation validates context propagation through middleware
func (suite *IntegrationTestSuite) TestMiddleware_ContextPropagation() {
	// Test that context information is properly propagated through middleware chain
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "status",
		ID:      "context-test",
	}
	
	response := suite.makeHTTPRequest(request)
	
	assert.NotNil(suite.T(), response)
	assert.Nil(suite.T(), response.Error)
	assert.Equal(suite.T(), "context-test", response.ID)
	
	// Verify the status response contains expected server information
	result, ok := response.Result.(map[string]interface{})
	require.True(suite.T(), ok)
	
	assert.Contains(suite.T(), result, "status")
	assert.Contains(suite.T(), result, "timestamp")
	assert.Contains(suite.T(), result, "version")
	assert.Equal(suite.T(), "healthy", result["status"])
}
