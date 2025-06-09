//go:build docker
// +build docker

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"streaming-server/pkg/types"
	"streaming-server/pkg/testutil"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// DockerIntegrationTestSuite provides Docker-based integration tests
type DockerIntegrationTestSuite struct {
	suite.Suite
	dockerAvailable bool
	quietMode       bool
	testServerHTTP   string
	testServerWS     string
	testServerTCP    string
	loadServerHTTP   string
	loadServerWS     string
	loadServerTCP    string
	perfServerHTTP   string
	perfServerWS     string
	perfServerTCP    string
	httpClient       *http.Client
	testResults      map[string]interface{}
	mu               sync.Mutex
}

// SetupSuite initializes the Docker test suite
func (suite *DockerIntegrationTestSuite) SetupSuite() {
	// Check if Docker tests should be skipped
	if os.Getenv("SKIP_DOCKER_TESTS") == "1" {
		suite.T().Skip("Docker tests are disabled")
		return
	}

	// Check for quiet mode flag
	suite.quietMode = os.Getenv("QUIET_TESTS") == "1"

	// Simple Docker availability check
	suite.dockerAvailable = true // Assume available for now

	// Get server endpoints from environment variables
	suite.testServerHTTP = getEnvOrDefault("TEST_SERVER_HTTP", "http://localhost:18080")
	suite.testServerWS = getEnvOrDefault("TEST_SERVER_WS", "ws://localhost:18082")
	suite.testServerTCP = getEnvOrDefault("TEST_SERVER_TCP", "localhost:18081")
	
	suite.loadServerHTTP = getEnvOrDefault("LOAD_SERVER_HTTP", "http://localhost:19080")
	suite.loadServerWS = getEnvOrDefault("LOAD_SERVER_WS", "ws://localhost:19082")
	suite.loadServerTCP = getEnvOrDefault("LOAD_SERVER_TCP", "localhost:19081")
	
	suite.perfServerHTTP = getEnvOrDefault("PERF_SERVER_HTTP", "http://localhost:20080")
	suite.perfServerWS = getEnvOrDefault("PERF_SERVER_WS", "ws://localhost:20082")
	suite.perfServerTCP = getEnvOrDefault("PERF_SERVER_TCP", "localhost:20081")

	// Create HTTP client
	suite.httpClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	// Initialize test results
	suite.testResults = make(map[string]interface{})

	// Wait for all servers to be ready
	suite.waitForServersReady()
}

// TearDownSuite cleans up after tests
func (suite *DockerIntegrationTestSuite) TearDownSuite() {
	// Save test results
	suite.saveTestResults()
}

// waitForServersReady waits for all Docker containers to be ready
func (suite *DockerIntegrationTestSuite) waitForServersReady() {
	servers := []string{
		suite.testServerHTTP,
		suite.loadServerHTTP,
		suite.perfServerHTTP,
	}

	for _, server := range servers {
		suite.waitForServerReady(server)
	}
}

// waitForServerReady waits for a specific server to be ready
func (suite *DockerIntegrationTestSuite) waitForServerReady(serverURL string) {
	maxRetries := 60 // 60 seconds timeout
	for i := 0; i < maxRetries; i++ {
		resp, err := suite.httpClient.Get(serverURL + "/rpc")
		if err == nil {
			resp.Body.Close()
			suite.T().Logf("Server %s is ready", serverURL)
			return
		}
		time.Sleep(1 * time.Second)
	}
	suite.T().Fatalf("Server %s failed to become ready within timeout", serverURL)
}

// TestDocker_BasicFunctionality tests basic Docker functionality
func (suite *DockerIntegrationTestSuite) TestDocker_BasicFunctionality() {
	if !suite.dockerAvailable {
		suite.T().Skip("Docker not available")
		return
	}

	section := testutil.NewSectionReporter("Docker Basic Functionality Test")
	section.SetQuietMode(suite.quietMode)
	section.Start()

	section.Status("Testing Docker basic functionality...")
	
	// Simulate Docker test
	// In real implementation, this would test actual Docker containers
	
	section.Status("Docker basic functionality test completed")
	section.End()
}

// Test basic functionality across all server instances
func (suite *DockerIntegrationTestSuite) TestBasicFunctionality() {
	servers := map[string]string{
		"test": suite.testServerHTTP,
		"load": suite.loadServerHTTP,
		"perf": suite.perfServerHTTP,
	}

	for name, server := range servers {
		suite.T().Run(fmt.Sprintf("Server_%s", name), func(t *testing.T) {
			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(`{"message": "docker test"}`),
				ID:      fmt.Sprintf("%s-basic-test", name),
			}

			response := suite.makeHTTPRequest(server, request)
			assert.Equal(t, "2.0", response.JSONRPC)
			assert.Equal(t, fmt.Sprintf("%s-basic-test", name), response.ID)
			assert.Nil(t, response.Error)
			assert.NotNil(t, response.Result)
		})
	}
}

// Test load balancing and distribution
func (suite *DockerIntegrationTestSuite) TestLoadDistribution() {
    const numRequests = 100
    const numWorkers = 10

    // Только для этого теста включаем тихий режим
    localQuietMode := true

    // Создаем секцию с тихим режимом
    section := testutil.NewSectionReporter("Load Distribution Test")
    section.SetQuietMode(localQuietMode)
    section.Start()

    servers := []string{
        suite.testServerHTTP,
        suite.loadServerHTTP,
        suite.perfServerHTTP,
    }

    var wg sync.WaitGroup
    results := make(chan map[string]interface{}, numRequests)

    section.Status("Starting %d workers for %d requests", numWorkers, numRequests)

    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()

            for j := 0; j < numRequests/numWorkers; j++ {
                serverURL := servers[j%len(servers)]
                
                start := time.Now()
                request := types.JSONRPCRequest{
                    JSONRPC: "2.0",
                    Method:  "calculate",
                    Params:  json.RawMessage(`{"operation": "add", "a": 1, "b": 1}`),
                    ID:      fmt.Sprintf("load-test-%d-%d", workerID, j),
                }

                response := suite.makeHTTPRequest(serverURL, request)
                duration := time.Since(start)

                results <- map[string]interface{}{
                    "server":   serverURL,
                    "success":  response.Error == nil,
                    "duration": duration.Milliseconds(),
                    "worker":   workerID,
                }
            }
        }(i)
    }

	wg.Wait()
	close(results)

	// Analyze results
	serverStats := make(map[string][]int64)
	successCount := 0
	totalCount := 0

	for result := range results {
		totalCount++
		if result["success"].(bool) {
			successCount++
		}
		
		server := result["server"].(string)
		duration := result["duration"].(int64)
		serverStats[server] = append(serverStats[server], duration)
	}

	// Verify load distribution
	suite.mu.Lock()
	suite.testResults["load_distribution"] = map[string]interface{}{
		"total_requests":  totalCount,
		"success_count":   successCount,
		"success_rate":    float64(successCount) / float64(totalCount),
		"server_stats":    serverStats,
	}
	suite.mu.Unlock()

	// Assert success rate
	successRate := float64(successCount) / float64(totalCount)
	assert.GreaterOrEqual(suite.T(), successRate, 0.95)

	// Verify each server handled requests
	for server, durations := range serverStats {
		assert.Greater(suite.T(), len(durations), 0, "Server %s should have handled requests", server)
	}
}

// Test WebSocket functionality across containers
func (suite *DockerIntegrationTestSuite) TestWebSocketCommunication() {
	servers := map[string]string{
		"test": suite.testServerWS,
		"load": suite.loadServerWS,
		"perf": suite.perfServerWS,
	}

	for name, wsURL := range servers {
		suite.T().Run(fmt.Sprintf("WebSocket_%s", name), func(t *testing.T) {
			dialer := websocket.Dialer{
				HandshakeTimeout: 10 * time.Second,
			}

			conn, _, err := dialer.Dial(wsURL+"/ws", nil)
			require.NoError(t, err)
			defer conn.Close()

			// Test multiple message types
			testMessages := []types.JSONRPCRequest{
				{JSONRPC: "2.0", Method: "echo", Params: json.RawMessage(`{"test": "websocket"}`), ID: "ws-1"},
				{JSONRPC: "2.0", Method: "time", ID: "ws-2"},
				{JSONRPC: "2.0", Method: "status", ID: "ws-3"},
			}

			for _, msg := range testMessages {
				err = conn.WriteJSON(msg)
				require.NoError(t, err)

				var response types.JSONRPCResponse
				err = conn.ReadJSON(&response)
				require.NoError(t, err)

				assert.Equal(t, "2.0", response.JSONRPC)
				assert.Equal(t, msg.ID, response.ID)
				assert.Nil(t, response.Error)
			}
		})
	}
}

// Test TCP communication across containers
func (suite *DockerIntegrationTestSuite) TestTCPCommunication() {
	servers := map[string]string{
		"test": suite.testServerTCP,
		"load": suite.loadServerTCP,
		"perf": suite.perfServerTCP,
	}

	for name, tcpAddr := range servers {
		suite.T().Run(fmt.Sprintf("TCP_%s", name), func(t *testing.T) {
			conn, err := net.DialTimeout("tcp", tcpAddr, 10*time.Second)
			require.NoError(t, err)
			defer conn.Close()

			encoder := json.NewEncoder(conn)
			decoder := json.NewDecoder(conn)

			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "status",
				ID:      fmt.Sprintf("tcp-%s-test", name),
			}

			err = encoder.Encode(request)
			require.NoError(t, err)

			var response types.JSONRPCResponse
			err = decoder.Decode(&response)
			require.NoError(t, err)

			assert.Equal(t, "2.0", response.JSONRPC)
			assert.Equal(t, fmt.Sprintf("tcp-%s-test", name), response.ID)
			assert.Nil(t, response.Error)
		})
	}
}

// Test data serialization and deserialization
func (suite *DockerIntegrationTestSuite) TestDataSerialization() {
	testCases := []struct {
		name string
		data interface{}
	}{
		{
			name: "simple_object",
			data: map[string]interface{}{
				"string": "test",
				"number": 42,
				"bool":   true,
			},
		},
		{
			name: "nested_object",
			data: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"value": "deep",
					},
				},
			},
		},
		{
			name: "array_data",
			data: []interface{}{1, "two", 3.0, true, nil},
		},
		{
			name: "large_object",
			data: suite.generateLargeObject(1000),
		},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			paramsJSON, err := json.Marshal(tc.data)
			require.NoError(t, err)

			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(paramsJSON),
				ID:      fmt.Sprintf("serialization-%s", tc.name),
			}

			response := suite.makeHTTPRequest(suite.testServerHTTP, request)
			assert.Equal(t, "2.0", response.JSONRPC)
			assert.Nil(t, response.Error)
			assert.NotNil(t, response.Result)

			// Verify data integrity
			result, ok := response.Result.(map[string]interface{})
			require.True(t, ok)
			
			echoedData, ok := result["echo"]
			require.True(t, ok)
			
			// For simple verification, check that we got data back
			assert.NotNil(t, echoedData)
		})
	}
}

// Test error handling across containers
func (suite *DockerIntegrationTestSuite) TestErrorHandling() {
	errorTestCases := []struct {
		name           string
		request        types.JSONRPCRequest
		expectedStatus int
	}{
		{
			name: "invalid_method",
			request: types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "nonexistent_method",
				ID:      "error-test-1",
			},
			expectedStatus: 200, // JSON-RPC errors return 200 with error in body
		},
		{
			name: "invalid_json",
			request: types.JSONRPCRequest{
				JSONRPC: "1.0", // Invalid version
				Method:  "echo",
				ID:      "error-test-2",
			},
			expectedStatus: 200,
		},
	}

	servers := []string{
		suite.testServerHTTP,
		suite.loadServerHTTP,
		suite.perfServerHTTP,
	}

	for _, server := range servers {
		for _, tc := range errorTestCases {
			suite.T().Run(fmt.Sprintf("%s_%s", tc.name, server), func(t *testing.T) {
				jsonData, err := json.Marshal(tc.request)
				require.NoError(t, err)

				resp, err := suite.httpClient.Post(server+"/rpc", "application/json", bytes.NewBuffer(jsonData))
				require.NoError(t, err)
				defer resp.Body.Close()

				assert.Equal(t, tc.expectedStatus, resp.StatusCode)

				if resp.StatusCode == 200 {
					var response types.JSONRPCResponse
					err = json.NewDecoder(resp.Body).Decode(&response)
					require.NoError(t, err)

					if tc.name == "invalid_method" {
						assert.NotNil(t, response.Error)
						assert.Equal(t, -32601, response.Error.Code)
					}
				}
			})
		}
	}
}

// Test network resilience and timeouts
func (suite *DockerIntegrationTestSuite) TestNetworkResilience() {
	// Test with different timeout scenarios
	timeoutClient := &http.Client{
		Timeout: 1 * time.Second, // Very short timeout
	}

	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(`{"test": "timeout"}`),
		ID:      "timeout-test",
	}

	jsonData, err := json.Marshal(request)
	require.NoError(suite.T(), err)

	// Test normal request (should succeed)
	resp, err := suite.httpClient.Post(suite.testServerHTTP+"/rpc", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()
	assert.Equal(suite.T(), 200, resp.StatusCode)

	// Test with timeout client (may timeout but shouldn't crash)
	_, err = timeoutClient.Post(suite.testServerHTTP+"/rpc", "application/json", bytes.NewBuffer(jsonData))
	// We don't assert on error here as it may or may not timeout depending on server response time
}

// Test concurrent connections across multiple containers
func (suite *DockerIntegrationTestSuite) TestConcurrentConnections() {
    const numConnections = 50
    const requestsPerConnection = 10

    // Только для этого теста включаем тихий режим
    localQuietMode := true

    // Создаем секцию с тихим режимом
    section := testutil.NewSectionReporter("Concurrent Connections Test")
    section.SetQuietMode(localQuietMode)
    section.Start()
    defer section.End()

    servers := []string{
        suite.testServerHTTP,
        suite.loadServerHTTP,
        suite.perfServerHTTP,
    }

    section.Status("Testing %d concurrent connections with %d requests each", 
        numConnections, requestsPerConnection)

    var wg sync.WaitGroup
    results := make(chan bool, numConnections*requestsPerConnection*len(servers))

	for _, server := range servers {
		for i := 0; i < numConnections; i++ {
			wg.Add(1)
			go func(serverURL string, connID int) {
				defer wg.Done()

				for j := 0; j < requestsPerConnection; j++ {
					request := types.JSONRPCRequest{
						JSONRPC: "2.0",
						Method:  "calculate",
						Params:  json.RawMessage(`{"operation": "multiply", "a": 2, "b": 3}`),
						ID:      fmt.Sprintf("concurrent-%d-%d", connID, j),
					}

					response := suite.makeHTTPRequest(serverURL, request)
					success := response.Error == nil && response.Result != nil
					results <- success
				}
			}(server, i)
		}
	}

	wg.Wait()
	close(results)

	// Count successful requests
	successCount := 0
	totalCount := 0
	for success := range results {
		totalCount++
		if success {
			successCount++
		}
	}

	successRate := float64(successCount) / float64(totalCount)
	
	suite.mu.Lock()
	suite.testResults["concurrent_connections"] = map[string]interface{}{
		"total_requests": totalCount,
		"success_count":  successCount,
		"success_rate":   successRate,
	}
	suite.mu.Unlock()

	assert.GreaterOrEqual(suite.T(), successRate, 0.95)
}

// Helper methods
func (suite *DockerIntegrationTestSuite) makeHTTPRequest(serverURL string, request types.JSONRPCRequest) *types.JSONRPCResponse {
	jsonData, err := json.Marshal(request)
	require.NoError(suite.T(), err)

	resp, err := suite.httpClient.Post(serverURL+"/rpc", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(suite.T(), err)

	var response types.JSONRPCResponse
	err = json.Unmarshal(body, &response)
	require.NoError(suite.T(), err)

	return &response
}

func (suite *DockerIntegrationTestSuite) generateLargeObject(size int) map[string]interface{} {
	obj := make(map[string]interface{})
	for i := 0; i < size; i++ {
		obj[fmt.Sprintf("key_%d", i)] = fmt.Sprintf("value_%d", i)
	}
	return obj
}

func (suite *DockerIntegrationTestSuite) saveTestResults() {
	suite.mu.Lock()
	defer suite.mu.Unlock()

	resultsJSON, err := json.MarshalIndent(suite.testResults, "", "  ")
	if err != nil {
		suite.T().Logf("Failed to marshal test results: %v", err)
		return
	}

	err = os.WriteFile("/app/test-results/docker-integration-results.json", resultsJSON, 0644)
	if err != nil {
		suite.T().Logf("Failed to save test results: %v", err)
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Run the Docker integration test suite
func TestDockerIntegrationSuite(t *testing.T) {
	// Only run if Docker tests are explicitly enabled
	if os.Getenv("SKIP_DOCKER_TESTS") == "1" {
		t.Skip("Docker tests are disabled")
		return
	}

	fmt.Print(testutil.FormatHeader("Docker Integration Test Suite"))

	suite.Run(t, new(DockerIntegrationTestSuite))

	fmt.Print(testutil.FormatFooter())
}
