package integration

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"streaming-server/pkg/types"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProtocol_CrossProtocolCompatibility tests that the same request works across all protocols
func (suite *IntegrationTestSuite) TestProtocol_CrossProtocolCompatibility() {
	// Create a standard request
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(`{"message": "cross-protocol test"}`),
		ID:      "cross-protocol-test",
	}

	// Test HTTP
	httpResponse := suite.makeHTTPRequest(request)
	assert.Nil(suite.T(), httpResponse.Error)
	assert.NotNil(suite.T(), httpResponse.Result)

	// Test WebSocket
	wsResponse := suite.makeWebSocketRequest(request)
	assert.Nil(suite.T(), wsResponse.Error)
	assert.NotNil(suite.T(), wsResponse.Result)

	// Test TCP
	tcpResponse := suite.makeTCPRequest(request)
	assert.Nil(suite.T(), tcpResponse.Error)
	assert.NotNil(suite.T(), tcpResponse.Result)

	// Compare results across protocols
	httpResult, ok := httpResponse.Result.(map[string]interface{})
	require.True(suite.T(), ok)
	wsResult, ok := wsResponse.Result.(map[string]interface{})
	require.True(suite.T(), ok)
	tcpResult, ok := tcpResponse.Result.(map[string]interface{})
	require.True(suite.T(), ok)

	// The echo field should be the same across all protocols
	httpEcho, ok := httpResult["echo"].(map[string]interface{})
	require.True(suite.T(), ok)
	wsEcho, ok := wsResult["echo"].(map[string]interface{})
	require.True(suite.T(), ok)
	tcpEcho, ok := tcpResult["echo"].(map[string]interface{})
	require.True(suite.T(), ok)

	assert.Equal(suite.T(), "cross-protocol test", httpEcho["message"])
	assert.Equal(suite.T(), "cross-protocol test", wsEcho["message"])
	assert.Equal(suite.T(), "cross-protocol test", tcpEcho["message"])
}

// TestProtocol_HTTPSSupport tests HTTPS support
func (suite *IntegrationTestSuite) TestProtocol_HTTPSSupport() {
	// Skip if HTTPS is not configured
	if suite.config.HTTPSAddr == "" {
		suite.T().Skip("HTTPS not configured")
	}

	// Create a request
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(`{"message": "https test"}`),
		ID:      "https-test",
	}

	// Extract HTTPS port from config
	httpsURL := fmt.Sprintf("https://localhost%s", suite.config.HTTPSAddr)
	
	// Send HTTPS request
	jsonData, err := json.Marshal(request)
	require.NoError(suite.T(), err)

	resp, err := suite.httpClient.Post(httpsURL+"/rpc", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(suite.T(), err)

	var response types.JSONRPCResponse
	err = json.Unmarshal(body, &response)
	require.NoError(suite.T(), err)

	assert.Nil(suite.T(), response.Error)
	assert.NotNil(suite.T(), response.Result)

	result, ok := response.Result.(map[string]interface{})
	require.True(suite.T(), ok)
	echo, ok := result["echo"].(map[string]interface{})
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), "https test", echo["message"])
}

// TestProtocol_WSSSupport tests WSS (WebSocket Secure) support
func (suite *IntegrationTestSuite) TestProtocol_WSSSupport() {
	// Skip if WSS is not configured
	if suite.config.WSSAddr == "" {
		suite.T().Skip("WSS not configured")
	}

	// Create a request
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(`{"message": "wss test"}`),
		ID:      "wss-test",
	}

	// Extract WSS port from config
	wssURL := fmt.Sprintf("wss://localhost%s", suite.config.WSSAddr)
	
	// Connect to WSS
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.Dial(wssURL+"/wss", nil)
	require.NoError(suite.T(), err)
	defer conn.Close()

	// Send request
	err = conn.WriteJSON(request)
	require.NoError(suite.T(), err)

	// Read response
	var response types.JSONRPCResponse
	err = conn.ReadJSON(&response)
	require.NoError(suite.T(), err)

	assert.Nil(suite.T(), response.Error)
	assert.NotNil(suite.T(), response.Result)

	result, ok := response.Result.(map[string]interface{})
	require.True(suite.T(), ok)
	echo, ok := result["echo"].(map[string]interface{})
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), "wss test", echo["message"])
}

// TestProtocol_TLSSupport tests TLS support
func (suite *IntegrationTestSuite) TestProtocol_TLSSupport() {
	// Skip if TLS is not configured
	if suite.config.TLSAddr == "" {
		suite.T().Skip("TLS not configured")
	}

	// Create a request
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(`{"message": "tls test"}`),
		ID:      "tls-test",
	}

	// Extract TLS port from config
	tlsAddr := fmt.Sprintf("localhost%s", suite.config.TLSAddr)
	
	// Connect to TLS server
	conn, err := tls.Dial("tcp", tlsAddr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(suite.T(), err)
	defer conn.Close()

	// Send request
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	err = encoder.Encode(request)
	require.NoError(suite.T(), err)

	// Read response
	var response types.JSONRPCResponse
	err = decoder.Decode(&response)
	require.NoError(suite.T(), err)

	assert.Nil(suite.T(), response.Error)
	assert.NotNil(suite.T(), response.Result)

	result, ok := response.Result.(map[string]interface{})
	require.True(suite.T(), ok)
	echo, ok := result["echo"].(map[string]interface{})
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), "tls test", echo["message"])
}

// TestProtocol_ConnectionPersistence tests that connections can handle multiple requests
func (suite *IntegrationTestSuite) TestProtocol_ConnectionPersistence() {
	// Test TCP connection persistence
	conn, err := net.Dial("tcp", suite.env.TCPAddr)
	require.NoError(suite.T(), err)
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	// Send multiple requests on the same connection
	for i := 0; i < 3; i++ {
		request := types.JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  "echo",
			Params:  json.RawMessage(fmt.Sprintf(`{"message": "persistence test %d"}`, i)),
			ID:      fmt.Sprintf("persistence-test-%d", i),
		}

		err = encoder.Encode(request)
		require.NoError(suite.T(), err)

		var response types.JSONRPCResponse
		err = decoder.Decode(&response)
		require.NoError(suite.T(), err)

		assert.Nil(suite.T(), response.Error)
		assert.NotNil(suite.T(), response.Result)
		assert.Equal(suite.T(), fmt.Sprintf("persistence-test-%d", i), response.ID)
	}
}

// TestProtocol_WebSocketPersistence tests WebSocket connection persistence
func (suite *IntegrationTestSuite) TestProtocol_WebSocketPersistence() {
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.Dial(suite.env.WebSocketURL+"/ws", nil)
	require.NoError(suite.T(), err)
	defer conn.Close()

	// Send multiple requests on the same WebSocket connection
	for i := 0; i < 3; i++ {
		request := types.JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  "echo",
			Params:  json.RawMessage(fmt.Sprintf(`{"message": "ws persistence test %d"}`, i)),
			ID:      fmt.Sprintf("ws-persistence-test-%d", i),
		}

		err = conn.WriteJSON(request)
		require.NoError(suite.T(), err)

		var response types.JSONRPCResponse
		err = conn.ReadJSON(&response)
		require.NoError(suite.T(), err)

		assert.Nil(suite.T(), response.Error)
		assert.NotNil(suite.T(), response.Result)
		assert.Equal(suite.T(), fmt.Sprintf("ws-persistence-test-%d", i), response.ID)
	}
}

// TestProtocol_ConcurrentConnections tests multiple concurrent connections
func (suite *IntegrationTestSuite) TestProtocol_ConcurrentConnections() {
	const numConnections = 5
	const requestsPerConnection = 3

	results := make(chan bool, numConnections*requestsPerConnection)

	// Test concurrent TCP connections
	for i := 0; i < numConnections; i++ {
		go func(connID int) {
			conn, err := net.Dial("tcp", suite.env.TCPAddr)
			if err != nil {
				for j := 0; j < requestsPerConnection; j++ {
					results <- false
				}
				return
			}
			defer conn.Close()

			encoder := json.NewEncoder(conn)
			decoder := json.NewDecoder(conn)

			for j := 0; j < requestsPerConnection; j++ {
				request := types.JSONRPCRequest{
					JSONRPC: "2.0",
					Method:  "echo",
					Params:  json.RawMessage(fmt.Sprintf(`{"message": "concurrent test %d-%d"}`, connID, j)),
					ID:      fmt.Sprintf("concurrent-test-%d-%d", connID, j),
				}

				err = encoder.Encode(request)
				if err != nil {
					results <- false
					continue
				}

				var response types.JSONRPCResponse
				err = decoder.Decode(&response)
				if err != nil {
					results <- false
					continue
				}

				success := response.Error == nil && response.Result != nil
				results <- success
			}
		}(i)
	}

	// Collect results
	successCount := 0
	totalCount := numConnections * requestsPerConnection
	for i := 0; i < totalCount; i++ {
		if <-results {
			successCount++
		}
	}

	// Expect at least 80% success rate for concurrent connections
	successRate := float64(successCount) / float64(totalCount)
	assert.GreaterOrEqual(suite.T(), successRate, 0.8)
}
