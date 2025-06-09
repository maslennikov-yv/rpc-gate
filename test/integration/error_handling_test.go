package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"streaming-server/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrorHandling_NotFound tests 404 Not Found responses
func (suite *IntegrationTestSuite) TestErrorHandling_NotFound() {
	t := suite.T()

	req, err := http.NewRequest("GET", suite.env.BaseURL+"/nonexistent", nil)
	require.NoError(t, err)

	resp, err := suite.httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestErrorHandling_InvalidJSON tests invalid JSON parsing
func (suite *IntegrationTestSuite) TestErrorHandling_InvalidJSON() {
	t := suite.T()

	invalidJSON := `{"jsonrpc": "2.0", "method": "echo", "params": {invalid json}, "id": 1}`

	resp, err := suite.httpClient.Post(suite.env.BaseURL+"/rpc", "application/json", bytes.NewBufferString(invalidJSON))
	require.NoError(t, err)
	defer resp.Body.Close()

	// The server should return 200 with a JSON-RPC error response
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response types.JSONRPCResponse
	err = json.Unmarshal(body, &response)
	if err == nil {
		// If we can parse it as JSON-RPC response, check for parse error
		assert.NotNil(t, response.Error)
		assert.Equal(t, -32700, response.Error.Code) // Parse error
	}
}

// TestErrorHandling_MissingFields tests missing required fields
func (suite *IntegrationTestSuite) TestErrorHandling_MissingFields() {
	t := suite.T()
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "calculate",
		Params:  json.RawMessage(`{"operation": "add"}`), // Missing 'a' and 'b' fields
		ID:      "missing-fields-test",
	}

	response := suite.makeHTTPRequest(request)
	require.NotNil(t, response)

	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, "missing-fields-test", response.ID)
	assert.NotNil(t, response.Error)
	assert.Equal(t, -32602, response.Error.Code) // Invalid params
	assert.Contains(t, response.Error.Message, "Missing required parameters")
}

// TestErrorHandling_InvalidOperation tests invalid operations
func (suite *IntegrationTestSuite) TestErrorHandling_InvalidOperation() {
	t := suite.T()
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "calculate",
		Params:  json.RawMessage(`{"operation": "invalid", "a": 1, "b": 2}`),
		ID:      "invalid-operation-test",
	}

	response := suite.makeHTTPRequest(request)
	require.NotNil(t, response)

	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, "invalid-operation-test", response.ID)
	assert.NotNil(t, response.Error)
	assert.Equal(t, -32602, response.Error.Code) // Invalid params
	assert.Contains(t, response.Error.Message, "Invalid operation")
}

// TestErrorHandling_DivisionByZero tests division by zero
func (suite *IntegrationTestSuite) TestErrorHandling_DivisionByZero() {
	t := suite.T()
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "calculate",
		Params:  json.RawMessage(`{"operation": "divide", "a": 10, "b": 0}`),
		ID:      "division-by-zero-test",
	}

	response := suite.makeHTTPRequest(request)
	require.NotNil(t, response)

	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, "division-by-zero-test", response.ID)
	assert.NotNil(t, response.Error)
	assert.Equal(t, -32602, response.Error.Code) // Invalid params
	assert.Contains(t, response.Error.Message, "Division by zero")
}

// TestErrorHandling_MethodNotFound tests method not found errors
func (suite *IntegrationTestSuite) TestErrorHandling_MethodNotFound() {
	t := suite.T()
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "nonexistent_method",
		ID:      "method-not-found-test",
	}

	response := suite.makeHTTPRequest(request)
	require.NotNil(t, response)

	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, "method-not-found-test", response.ID)
	assert.NotNil(t, response.Error)
	assert.Equal(t, -32601, response.Error.Code) // Method not found
	assert.Contains(t, response.Error.Message, "Method not found")
}

// TestErrorHandling_Timeout tests request timeout handling
func (suite *IntegrationTestSuite) TestErrorHandling_Timeout() {
	t := suite.T()

	// Create a client with a very short timeout
	shortTimeoutClient := &http.Client{
		Timeout: 500 * time.Millisecond,
		Transport: suite.httpClient.Transport,
	}

	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test_slow", // This handler sleeps for 2 seconds
		ID:      "timeout-test",
	}

	jsonData, err := json.Marshal(request)
	require.NoError(t, err)

	resp, err := shortTimeoutClient.Post(suite.env.BaseURL+"/rpc", "application/json", bytes.NewBuffer(jsonData))

	// Should get a timeout error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
	if resp != nil {
		defer resp.Body.Close()
	}
}

// TestErrorHandling_LargePayload tests large payload handling
func (suite *IntegrationTestSuite) TestErrorHandling_LargePayload() {
	t := suite.T()
	// Create a moderately large params object (not too large to avoid memory issues)
	largeData := strings.Repeat("x", 1024*100) // 100KB of data
	largeParams := fmt.Sprintf(`{"message": "%s"}`, largeData)

	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(largeParams),
		ID:      "large-payload-test",
	}

	jsonData, err := json.Marshal(request)
	require.NoError(t, err)

	resp, err := suite.httpClient.Post(suite.env.BaseURL+"/rpc", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err)
	defer resp.Body.Close()

	// The server should handle this gracefully
	// Either accept it (200) or reject it with appropriate error
	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var response types.JSONRPCResponse
		err = json.Unmarshal(body, &response)
		require.NoError(t, err)

		assert.Equal(t, "2.0", response.JSONRPC)
		assert.Equal(t, "large-payload-test", response.ID)
	} else {
		// Should be 413 Payload Too Large or similar
		assert.True(t, resp.StatusCode == http.StatusRequestEntityTooLarge ||
			resp.StatusCode == http.StatusBadRequest)
	}
}

// TestErrorHandling_ConcurrentErrors tests concurrent error scenarios
func (suite *IntegrationTestSuite) TestErrorHandling_ConcurrentErrors() {
	t := suite.T()
	const numRequests = 5 // Reduced number to avoid overwhelming the test

	// Create channels for results
	results := make(chan *types.JSONRPCResponse, numRequests)
	errors := make(chan error, numRequests)

	// Send concurrent requests that will cause errors
	for i := 0; i < numRequests; i++ {
		go func(id int) {
			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "nonexistent_method",
				ID:      fmt.Sprintf("concurrent-error-%d", id),
			}

			response := suite.makeHTTPRequest(request)
			if response != nil {
				results <- response
			} else {
				errors <- fmt.Errorf("nil response for request %d", id)
			}
		}(i)
	}

	successCount := 0
	errorCount := 0

	for i := 0; i < numRequests; i++ {
		select {
		case response := <-results:
			successCount++
			assert.Equal(t, "2.0", response.JSONRPC)
			assert.NotNil(t, response.Error)
			assert.Equal(t, -32601, response.Error.Code) // Method not found
		case err := <-errors:
			errorCount++
			t.Logf("Request error: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent responses")
		}
	}

	assert.Equal(t, numRequests, successCount+errorCount)
	assert.GreaterOrEqual(t, successCount, numRequests/2) // At least half should succeed
}

// TestErrorHandling_ConcurrentMixedRequests tests a mix of successful and error requests
func (suite *IntegrationTestSuite) TestErrorHandling_ConcurrentMixedRequests() {
	t := suite.T()
	const numRequests = 10

	// Create channels for results
	results := make(chan *types.JSONRPCResponse, numRequests)
	errors := make(chan error, numRequests)

	// Send a mix of valid and invalid requests
	for i := 0; i < numRequests; i++ {
		go func(id int) {
			var request types.JSONRPCRequest

			// Alternate between valid and invalid requests
			if id%2 == 0 {
				// Valid request
				request = types.JSONRPCRequest{
					JSONRPC: "2.0",
					Method:  "calculate",
					Params:  json.RawMessage(fmt.Sprintf(`{"operation": "add", "a": %d, "b": %d}`, id, id+1)),
					ID:      fmt.Sprintf("mixed-valid-%d", id),
				}
			} else {
				// Invalid request
				request = types.JSONRPCRequest{
					JSONRPC: "2.0",
					Method:  "nonexistent_method",
					ID:      fmt.Sprintf("mixed-invalid-%d", id),
				}
			}

			response := suite.makeHTTPRequest(request)
			if response != nil {
				results <- response
			} else {
				errors <- fmt.Errorf("nil response for request %d", id)
			}
		}(i)
	}

	validCount := 0
	errorCount := 0
	failureCount := 0

	for i := 0; i < numRequests; i++ {
		select {
		case response := <-results:
			if response.Error == nil {
				validCount++
			} else {
				errorCount++
			}
		case err := <-errors:
			failureCount++
			t.Logf("Request error: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent responses")
		}
	}

	// We expect approximately half valid and half error responses
	assert.Equal(t, numRequests, validCount+errorCount+failureCount)
	assert.GreaterOrEqual(t, validCount, numRequests/4) // At least 25% should be valid
	assert.GreaterOrEqual(t, errorCount, numRequests/4) // At least 25% should be expected errors
}
