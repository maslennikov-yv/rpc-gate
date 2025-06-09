package integration

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"streaming-server/pkg/types"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComprehensive_JSONRPCCompliance validates full JSON-RPC 2.0 specification compliance
func (suite *IntegrationTestSuite) TestComprehensive_JSONRPCCompliance() {
	testCases := []struct {
		name     string
		request  interface{}
		expected func(*types.JSONRPCResponse) bool
	}{
		{
			name: "Valid Request",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(`{"message": "test"}`),
				ID:      1,
			},
			expected: func(resp *types.JSONRPCResponse) bool {
				return resp.Error == nil && resp.Result != nil && resp.ID == float64(1)
			},
		},
		{
			name: "Notification (no ID)",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(`{"message": "notification"}`),
			},
			expected: func(resp *types.JSONRPCResponse) bool {
				return resp == nil // No response for notifications
			},
		},
		{
			name: "Invalid JSON-RPC version",
			request: types.JSONRPCRequest{
				JSONRPC: "1.0",
				Method:  "echo",
				ID:      2,
			},
			expected: func(resp *types.JSONRPCResponse) bool {
				return resp.Error != nil && resp.Error.Code == -32600
			},
		},
		{
			name: "Method not found",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "nonexistent",
				ID:      3,
			},
			expected: func(resp *types.JSONRPCResponse) bool {
				return resp.Error != nil && resp.Error.Code == -32601
			},
		},
		{
			name: "Invalid params",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "calculate",
				Params:  json.RawMessage(`{"invalid": "params"}`),
				ID:      4,
			},
			expected: func(resp *types.JSONRPCResponse) bool {
				return resp.Error != nil && resp.Error.Code == -32602
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			response := suite.makeHTTPRequest(tc.request.(types.JSONRPCRequest))
			assert.True(suite.T(), tc.expected(response), "Test case %s failed", tc.name)
		})
	}
}

// TestComprehensive_AllProtocolSupport validates all supported protocols
func (suite *IntegrationTestSuite) TestComprehensive_AllProtocolSupport() {
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "status",
		ID:      "protocol-test",
	}

	protocols := []struct {
		name string
		test func() *types.JSONRPCResponse
	}{
		{
			name: "HTTP",
			test: func() *types.JSONRPCResponse {
				return suite.makeHTTPRequest(request)
			},
		},
		{
			name: "HTTPS",
			test: func() *types.JSONRPCResponse {
				return suite.makeHTTPSRequest(request)
			},
		},
		{
			name: "WebSocket",
			test: func() *types.JSONRPCResponse {
				return suite.makeWebSocketRequest(request)
			},
		},
		{
			name: "TCP",
			test: func() *types.JSONRPCResponse {
				return suite.makeTCPRequest(request)
			},
		},
		{
			name: "TLS",
			test: func() *types.JSONRPCResponse {
				return suite.makeTLSRequest(request)
			},
		},
	}

	for _, protocol := range protocols {
		suite.Run(protocol.name, func() {
			response := protocol.test()
			assert.NotNil(suite.T(), response, "Protocol %s should return a response", protocol.name)
			assert.Nil(suite.T(), response.Error, "Protocol %s should not return an error", protocol.name)
			assert.NotNil(suite.T(), response.Result, "Protocol %s should return a result", protocol.name)
			assert.Equal(suite.T(), "protocol-test", response.ID, "Protocol %s should preserve request ID", protocol.name)
		})
	}
}

// TestComprehensive_PersistentConnections validates persistent connection handling
func (suite *IntegrationTestSuite) TestComprehensive_PersistentConnections() {
	suite.Run("TCP_Persistent", func() {
		conn, err := net.Dial("tcp", suite.env.TCPAddr)
		require.NoError(suite.T(), err)
		defer conn.Close()

		encoder := json.NewEncoder(conn)
		decoder := json.NewDecoder(conn)

		// Send multiple requests on the same connection
		for i := 0; i < 5; i++ {
			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(fmt.Sprintf(`{"message": "persistent-%d"}`, i)),
				ID:      fmt.Sprintf("persistent-%d", i),
			}

			err = encoder.Encode(request)
			require.NoError(suite.T(), err)

			var response types.JSONRPCResponse
			err = decoder.Decode(&response)
			require.NoError(suite.T(), err)

			assert.Nil(suite.T(), response.Error)
			assert.Equal(suite.T(), fmt.Sprintf("persistent-%d", i), response.ID)
		}
	})

	suite.Run("WebSocket_Persistent", func() {
		dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
		conn, _, err := dialer.Dial(suite.env.WebSocketURL+"/ws", nil)
		require.NoError(suite.T(), err)
		defer conn.Close()

		// Send multiple requests on the same WebSocket connection
		for i := 0; i < 5; i++ {
			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "time",
				ID:      fmt.Sprintf("ws-persistent-%d", i),
			}

			err = conn.WriteJSON(request)
			require.NoError(suite.T(), err)

			var response types.JSONRPCResponse
			err = conn.ReadJSON(&response)
			require.NoError(suite.T(), err)

			assert.Nil(suite.T(), response.Error)
			assert.Equal(suite.T(), fmt.Sprintf("ws-persistent-%d", i), response.ID)
		}
	})
}

// TestComprehensive_ConcurrentHandling validates concurrent request processing
func (suite *IntegrationTestSuite) TestComprehensive_ConcurrentHandling() {
	const numWorkers = 10
	const requestsPerWorker = 5
	totalRequests := numWorkers * requestsPerWorker

	var wg sync.WaitGroup
	results := make(chan *types.JSONRPCResponse, totalRequests)
	errors := make(chan error, totalRequests)

	// Launch concurrent workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < requestsPerWorker; j++ {
				request := types.JSONRPCRequest{
					JSONRPC: "2.0",
					Method:  "calculate",
					Params:  json.RawMessage(`{"operation": "add", "a": 10, "b": 5}`),
					ID:      fmt.Sprintf("worker-%d-req-%d", workerID, j),
				}

				response := suite.makeHTTPRequest(request)
				if response != nil {
					results <- response
				} else {
					errors <- fmt.Errorf("nil response from worker %d request %d", workerID, j)
				}
			}
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	// Validate results
	successCount := 0
	for response := range results {
		if response.Error == nil {
			successCount++
			// Verify calculation result
			if result, ok := response.Result.(float64); ok {
				assert.Equal(suite.T(), float64(15), result, "Calculation should return 15")
			}
		}
	}

	errorCount := len(errors)
	assert.Equal(suite.T(), 0, errorCount, "Should have no errors")
	assert.Equal(suite.T(), totalRequests, successCount, "All requests should succeed")
}

// TestComprehensive_MiddlewareChaining validates middleware functionality
func (suite *IntegrationTestSuite) TestComprehensive_MiddlewareChaining() {
	// Test that middleware is properly applied by checking logging behavior
	// and request processing order

	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(`{"message": "middleware test"}`),
		ID:      "middleware-test",
	}

	response := suite.makeHTTPRequest(request)

	// Verify the request was processed through middleware
	assert.NotNil(suite.T(), response)
	assert.Nil(suite.T(), response.Error)
	assert.NotNil(suite.T(), response.Result)

	// The response should contain the echoed message
	result, ok := response.Result.(map[string]interface{})
	require.True(suite.T(), ok)

	echo, ok := result["echo"].(map[string]interface{})
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), "middleware test", echo["message"])
}

// TestComprehensive_ErrorHandlingAndRecovery validates comprehensive error handling
func (suite *IntegrationTestSuite) TestComprehensive_ErrorHandlingAndRecovery() {
	errorScenarios := []struct {
		name     string
		request  types.JSONRPCRequest
		expected int // Expected error code
	}{
		{
			name: "Parse Error",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(`{invalid json}`),
				ID:      "parse-error",
			},
			expected: -32700,
		},
		{
			name: "Invalid Request",
			request: types.JSONRPCRequest{
				JSONRPC: "1.0", // Wrong version
				Method:  "echo",
				ID:      "invalid-request",
			},
			expected: -32600,
		},
		{
			name: "Method Not Found",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "nonexistent_method",
				ID:      "method-not-found",
			},
			expected: -32601,
		},
		{
			name: "Invalid Params",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "calculate",
				Params:  json.RawMessage(`{"operation": "divide", "a": 10, "b": 0}`),
				ID:      "invalid-params",
			},
			expected: -32602,
		},
	}

	for _, scenario := range errorScenarios {
		suite.Run(scenario.name, func() {
			// For parse errors, we need to send malformed JSON directly
			if scenario.name == "Parse Error" {
				resp, err := suite.httpClient.Post(suite.env.BaseURL+"/rpc", "application/json",
					bytes.NewBufferString(`{"jsonrpc": "2.0", "method": "echo", "params": {invalid json}, "id": "parse-error"}`))
				require.NoError(suite.T(), err)
				defer resp.Body.Close()

				var response types.JSONRPCResponse
				err = json.NewDecoder(resp.Body).Decode(&response)
				require.NoError(suite.T(), err)

				assert.NotNil(suite.T(), response.Error)
				assert.Equal(suite.T(), scenario.expected, response.Error.Code)
			} else {
				response := suite.makeHTTPRequest(scenario.request)
				assert.NotNil(suite.T(), response)
				assert.NotNil(suite.T(), response.Error)
				assert.Equal(suite.T(), scenario.expected, response.Error.Code)
			}
		})
	}
}

// TestComprehensive_BatchProcessing validates batch request handling
func (suite *IntegrationTestSuite) TestComprehensive_BatchProcessing() {
	suite.Run("Mixed_Batch", func() {
		batchJSON := `[
			{"jsonrpc": "2.0", "method": "echo", "params": {"message": "test1"}, "id": 1},
			{"jsonrpc": "2.0", "method": "echo", "params": {"message": "test2"}},
			{"jsonrpc": "2.0", "method": "time", "id": 3},
			{"jsonrpc": "2.0", "method": "nonexistent", "id": 4}
		]`

		resp, err := suite.httpClient.Post(suite.env.BaseURL+"/rpc", "application/json", bytes.NewBufferString(batchJSON))
		require.NoError(suite.T(), err)
		defer resp.Body.Close()

		var responses []types.JSONRPCResponse
		err = json.NewDecoder(resp.Body).Decode(&responses)
		require.NoError(suite.T(), err)

		// Should have 3 responses (excluding notification)
		assert.Len(suite.T(), responses, 3)

		// Verify response IDs and results
		responseMap := make(map[interface{}]*types.JSONRPCResponse)
		for i := range responses {
			responseMap[responses[i].ID] = &responses[i]
		}

		// Check successful requests
		assert.Contains(suite.T(), responseMap, float64(1))
		assert.Contains(suite.T(), responseMap, float64(3))
		assert.Contains(suite.T(), responseMap, float64(4))

		assert.Nil(suite.T(), responseMap[float64(1)].Error)
		assert.Nil(suite.T(), responseMap[float64(3)].Error)
		assert.NotNil(suite.T(), responseMap[float64(4)].Error) // Method not found
	})
}

// TestComprehensive_ResourceManagement validates proper resource handling
func (suite *IntegrationTestSuite) TestComprehensive_ResourceManagement() {
	// Test connection limits and cleanup
	const maxConnections = 20
	var connections []net.Conn

	// Open multiple TCP connections
	for i := 0; i < maxConnections; i++ {
		conn, err := net.Dial("tcp", suite.env.TCPAddr)
		if err != nil {
			// If we can't open more connections, that's expected behavior
			break
		}
		connections = append(connections, conn)
	}

	// Verify we can still make requests on existing connections
	if len(connections) > 0 {
		encoder := json.NewEncoder(connections[0])
		decoder := json.NewDecoder(connections[0])

		request := types.JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  "status",
			ID:      "resource-test",
		}

		err := encoder.Encode(request)
		require.NoError(suite.T(), err)

		var response types.JSONRPCResponse
		err = decoder.Decode(&response)
		require.NoError(suite.T(), err)

		assert.Nil(suite.T(), response.Error)
	}

	// Clean up connections
	for _, conn := range connections {
		conn.Close()
	}
}

// Helper method for HTTPS requests
func (suite *IntegrationTestSuite) makeHTTPSRequest(request types.JSONRPCRequest) *types.JSONRPCResponse {
	httpsURL := suite.env.HTTPSUrl

	jsonData, err := json.Marshal(request)
	require.NoError(suite.T(), err)

	resp, err := suite.httpClient.Post(httpsURL+"/rpc", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	if request.IsNotification() {
		return nil
	}

	var response types.JSONRPCResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(suite.T(), err)

	return &response
}

// Helper method for TLS requests
func (suite *IntegrationTestSuite) makeTLSRequest(request types.JSONRPCRequest) *types.JSONRPCResponse {
	tlsAddr := suite.env.TLSAddr

	conn, err := tls.Dial("tcp", tlsAddr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(suite.T(), err)
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	err = encoder.Encode(request)
	require.NoError(suite.T(), err)

	if request.IsNotification() {
		return nil
	}

	var response types.JSONRPCResponse
	err = decoder.Decode(&response)
	require.NoError(suite.T(), err)

	return &response
}
