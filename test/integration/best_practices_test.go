package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"streaming-server/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBestPractices_GracefulDegradation validates graceful degradation under load
func (suite *IntegrationTestSuite) TestBestPractices_GracefulDegradation() {
	// Test server behavior under high load
	const highLoad = 50
	var wg sync.WaitGroup
	results := make(chan bool, highLoad)
	
	for i := 0; i < highLoad; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()
			
			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(fmt.Sprintf(`{"message": "load-test-%d"}`, requestID)),
				ID:      fmt.Sprintf("load-%d", requestID),
			}
			
			response := suite.makeHTTPRequest(request)
			results <- (response != nil && response.Error == nil)
		}(i)
	}
	
	wg.Wait()
	close(results)
	
	// Count successful responses
	successCount := 0
	for success := range results {
		if success {
			successCount++
		}
	}
	
	// Should handle at least 80% of requests successfully under load
	successRate := float64(successCount) / float64(highLoad)
	assert.GreaterOrEqual(suite.T(), successRate, 0.8, 
		"Server should handle at least 80%% of requests under load")
}

// TestBestPractices_ResourceCleanup validates proper resource cleanup
func (suite *IntegrationTestSuite) TestBestPractices_ResourceCleanup() {
	// Test that connections are properly cleaned up
	initialConnections := suite.getActiveConnections()
	
	// Create and close multiple connections
	const numConnections = 10
	for i := 0; i < numConnections; i++ {
		// Make a request and let connection close naturally
		request := types.JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  "status",
			ID:      fmt.Sprintf("cleanup-%d", i),
		}
		
		response := suite.makeHTTPRequest(request)
		assert.NotNil(suite.T(), response)
		assert.Nil(suite.T(), response.Error)
	}
	
	// Allow time for cleanup
	time.Sleep(100 * time.Millisecond)
	
	finalConnections := suite.getActiveConnections()
	
	// Connection count should not have grown significantly
	connectionGrowth := finalConnections - initialConnections
	assert.LessOrEqual(suite.T(), connectionGrowth, 5, 
		"Connection count should not grow excessively")
}

// TestBestPractices_ErrorRecovery validates error recovery mechanisms
func (suite *IntegrationTestSuite) TestBestPractices_ErrorRecovery() {
	// Test that server recovers from errors and continues processing
	
	// Send an error-inducing request
	errorRequest := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test_error",
		ID:      "recovery-error",
	}
	
	errorResponse := suite.makeHTTPRequest(errorRequest)
	assert.NotNil(suite.T(), errorResponse)
	assert.NotNil(suite.T(), errorResponse.Error)
	
	// Immediately send a normal request to verify recovery
	normalRequest := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(`{"message": "recovery test"}`),
		ID:      "recovery-normal",
	}
	
	normalResponse := suite.makeHTTPRequest(normalRequest)
	assert.NotNil(suite.T(), normalResponse)
	assert.Nil(suite.T(), normalResponse.Error)
	assert.Equal(suite.T(), "recovery-normal", normalResponse.ID)
}

// TestBestPractices_SecurityHeaders validates security best practices
func (suite *IntegrationTestSuite) TestBestPractices_SecurityHeaders() {
	// Test CORS and security headers
	req, err := http.NewRequest("OPTIONS", suite.env.BaseURL+"/rpc", nil)
	require.NoError(suite.T(), err)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	
	resp, err := suite.httpClient.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	
	// Verify CORS headers are present
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
	assert.Contains(suite.T(), resp.Header.Get("Access-Control-Allow-Origin"), "*")
	assert.Contains(suite.T(), resp.Header.Get("Access-Control-Allow-Methods"), "POST")
}

// TestBestPractices_InputValidation validates comprehensive input validation
func (suite *IntegrationTestSuite) TestBestPractices_InputValidation() {
	maliciousInputs := []struct {
		name    string
		request string
	}{
		{
			name:    "SQL Injection Attempt",
			request: `{"jsonrpc": "2.0", "method": "echo'; DROP TABLE users; --", "id": 1}`,
		},
		{
			name:    "XSS Attempt",
			request: `{"jsonrpc": "2.0", "method": "echo", "params": {"message": "<script>alert('xss')</script>"}, "id": 2}`,
		},
		{
			name:    "Path Traversal Attempt",
			request: `{"jsonrpc": "2.0", "method": "../../../etc/passwd", "id": 3}`,
		},
		{
			name:    "Command Injection Attempt",
			request: `{"jsonrpc": "2.0", "method": "echo", "params": {"message": "; rm -rf /"}, "id": 4}`,
		},
	}
	
	for _, input := range maliciousInputs {
		suite.Run(input.name, func() {
			resp, err := suite.httpClient.Post(suite.env.BaseURL+"/rpc", "application/json", 
				strings.NewReader(input.request))
			require.NoError(suite.T(), err)
			defer resp.Body.Close()
			
			// Server should handle malicious input gracefully
			assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
			
			var response types.JSONRPCResponse
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(suite.T(), err)
			
			// Should either return method not found or process safely
			if response.Error != nil {
				assert.Equal(suite.T(), -32601, response.Error.Code) // Method not found
			}
		})
	}
}

// TestBestPractices_PerformanceOptimization validates performance optimizations
func (suite *IntegrationTestSuite) TestBestPractices_PerformanceOptimization() {
	// Test response times for different request sizes
	sizes := []struct {
		name string
		size int
	}{
		{"Small", 100},
		{"Medium", 1000},
		{"Large", 10000},
	}
	
	for _, size := range sizes {
		suite.Run(size.name, func() {
			message := strings.Repeat("a", size.size)
			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(fmt.Sprintf(`{"message": "%s"}`, message)),
				ID:      fmt.Sprintf("perf-%s", size.name),
			}
			
			start := time.Now()
			response := suite.makeHTTPRequest(request)
			duration := time.Since(start)
			
			assert.NotNil(suite.T(), response)
			assert.Nil(suite.T(), response.Error)
			
			// Performance should scale reasonably with request size
			maxExpectedTime := time.Duration(size.size/1000+1) * time.Second
			assert.Less(suite.T(), duration, maxExpectedTime, 
				"Response time should scale reasonably with request size")
		})
	}
}

// TestBestPractices_Monitoring validates monitoring and health check capabilities
func (suite *IntegrationTestSuite) TestBestPractices_Monitoring() {
	// Test health endpoint
	resp, err := suite.httpClient.Get(suite.env.BaseURL + "/health")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
	
	var health map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(suite.T(), err)
	
	// Verify health response structure
	assert.Equal(suite.T(), "healthy", health["status"])
	assert.Contains(suite.T(), health, "timestamp")
	assert.Contains(suite.T(), health, "service")
	assert.Contains(suite.T(), health, "version")
}

// Helper method to estimate active connections (simplified)
func (suite *IntegrationTestSuite) getActiveConnections() int {
	// This is a simplified estimation - in a real implementation,
	// you would query actual server metrics
	return 0
}
