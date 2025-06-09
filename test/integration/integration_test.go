package integration

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"streaming-server/pkg/handlers"
	"streaming-server/pkg/middleware"
	"streaming-server/pkg/server"
	"streaming-server/pkg/testutil"
	"streaming-server/pkg/types"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// TestEnvironment represents the testing environment configuration
type TestEnvironment struct {
	IsDocker     bool
	IsCI         bool
	QuietMode    bool
	BaseURL      string
	WebSocketURL string
	TCPAddr      string
	HTTPSUrl     string
	TLSAddr      string
	WSSUrl       string
}

// DetectTestEnvironment automatically detects the testing environment
func DetectTestEnvironment() *TestEnvironment {
	env := &TestEnvironment{
		QuietMode: os.Getenv("QUIET_TESTS") == "1",
		IsCI:      os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true",
	}

	// Check for Docker environment indicators
	if os.Getenv("TEST_ISOLATION") == "docker" ||
		os.Getenv("DOCKER_COMPOSE_TEST") == "1" ||
		os.Getenv("TEST_SERVER_HTTP") != "" ||
		fileExists("/.dockerenv") {
		env.IsDocker = true
		env.setupDockerURLs()
	} else {
		env.IsDocker = false
		env.setupLocalURLs()
	}

	return env
}

// fileExists checks if a file exists
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// setupDockerURLs configures URLs for Docker environment
func (env *TestEnvironment) setupDockerURLs() {
	// Use Docker Compose service names for internal networking
	env.BaseURL = getEnvOrDefault("TEST_SERVER_HTTP", "http://streaming-server-test:8080")
	env.WebSocketURL = getEnvOrDefault("TEST_SERVER_WS", "ws://streaming-server-test:8082")
	env.TCPAddr = getEnvOrDefault("TEST_SERVER_TCP", "streaming-server-test:8081")
	env.HTTPSUrl = getEnvOrDefault("TEST_SERVER_HTTPS", "https://streaming-server-test:8443")
	env.TLSAddr = getEnvOrDefault("TEST_SERVER_TLS", "streaming-server-test:8444")
	env.WSSUrl = getEnvOrDefault("TEST_SERVER_WSS", "wss://streaming-server-test:8445")
}

// setupLocalURLs configures URLs for local development environment
func (env *TestEnvironment) setupLocalURLs() {
	// These will be set dynamically when server starts in local mode
	env.BaseURL = ""
	env.WebSocketURL = ""
	env.TCPAddr = ""
	env.HTTPSUrl = ""
	env.TLSAddr = ""
	env.WSSUrl = ""
}

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// IntegrationTestSuite provides a test suite for integration tests
type IntegrationTestSuite struct {
	suite.Suite
	env           *TestEnvironment
	server        *server.Server
	config        server.Config
	logger        *middleware.Logger
	httpClient    *http.Client
	mu            sync.Mutex
	progressGroup *testutil.ProgressGroup
	allocatedPorts []int
}

// SetupSuite initializes the test suite with automatic environment detection
func (suite *IntegrationTestSuite) SetupSuite() {
	// Detect test environment automatically
	suite.env = DetectTestEnvironment()

	// Initialize progress group
	suite.progressGroup = testutil.NewProgressGroup()
	suite.progressGroup.SetQuietMode(suite.env.QuietMode)

	// Create section reporter for setup
	section := testutil.NewSectionReporter("Integration Test Suite Setup")
	section.SetQuietMode(suite.env.QuietMode)
	section.Start()

	// Show environment detection results
	if suite.env.IsDocker {
		section.Status("🐳 Docker environment detected - using container networking")
		section.Status("📡 Service discovery: automatic via Docker Compose")
	} else {
		section.Status("🖥️  Local environment detected - starting embedded server")
		section.Status("🔧 Port management: dynamic allocation")
	}

	// Create logger
	section.Status("Creating test logger")
	logConfig := middleware.LoggingConfig{
		Enabled:        true,
		Destination:    middleware.LogDestinationStdout,
		Format:         middleware.LogFormatJSON,
		Level:          middleware.LogLevelInfo,
		LogSuccessOnly: true,
		ServiceName:    "integration-test-server",
		ServiceVersion: "test-1.0.0",
	}

	var err error
	suite.logger, err = middleware.NewLogger(logConfig)
	require.NoError(suite.T(), err)

	// Setup HTTP client with appropriate configuration
	section.Status("Creating HTTP client")
	suite.httpClient = &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	if suite.env.IsDocker {
		suite.setupDockerEnvironment(section)
	} else {
		suite.setupLocalEnvironment(section)
	}

	section.Status("✅ Setup completed successfully")
	section.Status("Environment: %s", suite.getEnvironmentDescription())
	section.Status("Base URL: %s", suite.env.BaseURL)
	section.Status("WebSocket URL: %s", suite.env.WebSocketURL)
	section.Status("TCP Address: %s", suite.env.TCPAddr)
	section.End()
}

// setupDockerEnvironment configures the test suite for Docker environment
func (suite *IntegrationTestSuite) setupDockerEnvironment(section *testutil.SectionReporter) {
	section.Status("Configuring Docker environment")
	
	// Wait for services to be ready with extended timeout for Docker
	section.Status("Waiting for Docker services to be ready...")
	suite.waitForDockerServices(section)
	
	section.Status("Docker services are ready")
}

// setupLocalEnvironment configures the test suite for local development
func (suite *IntegrationTestSuite) setupLocalEnvironment(section *testutil.SectionReporter) {
	section.Status("Configuring local development environment")
	
	// Allocate ports using the optimized port manager
	section.Status("Allocating ports for local server")
	ports, err := GetTestPortRange(6)
	require.NoError(suite.T(), err, "Failed to allocate ports for test server")
	suite.allocatedPorts = ports
	
	section.Status("Allocated ports: HTTP=%d, HTTPS=%d, TCP=%d, TLS=%d, WS=%d, WSS=%d", 
		ports[0], ports[1], ports[2], ports[3], ports[4], ports[5])

	// Generate TLS config for local testing
	section.Status("Generating test TLS certificates")
	tlsConfig := suite.generateTestTLSConfig()

	// Configure server
	suite.config = server.Config{
		HTTPAddr:     fmt.Sprintf(":%d", ports[0]),
		HTTPSAddr:    fmt.Sprintf(":%d", ports[1]),
		TCPAddr:      fmt.Sprintf(":%d", ports[2]),
		TLSAddr:      fmt.Sprintf(":%d", ports[3]),
		WSAddr:       fmt.Sprintf(":%d", ports[4]),
		WSSAddr:      fmt.Sprintf(":%d", ports[5]),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
		TLSConfig:    tlsConfig,
	}

	// Update environment URLs with allocated ports
	suite.env.BaseURL = fmt.Sprintf("http://localhost:%d", ports[0])
	suite.env.HTTPSUrl = fmt.Sprintf("https://localhost:%d", ports[1])
	suite.env.TCPAddr = fmt.Sprintf("localhost:%d", ports[2])
	suite.env.TLSAddr = fmt.Sprintf("localhost:%d", ports[3])
	suite.env.WebSocketURL = fmt.Sprintf("ws://localhost:%d", ports[4])
	suite.env.WSSUrl = fmt.Sprintf("wss://localhost:%d", ports[5])

	// Create and start server
	section.Status("Creating and starting local server")
	suite.server = server.NewServer(suite.config, suite.logger)
	suite.registerTestHandlers()
	
	err = suite.server.Start()
	require.NoError(suite.T(), err)

	// Wait for local server to be ready
	section.Status("Waiting for local server to be ready")
	suite.waitForServerReady(suite.env.BaseURL, 30*time.Second)
}

// waitForDockerServices waits for Docker Compose services to be ready
func (suite *IntegrationTestSuite) waitForDockerServices(section *testutil.SectionReporter) {
	maxWait := 60 * time.Second
	checkInterval := 2 * time.Second
	timeout := time.Now().Add(maxWait)

	services := []struct {
		name string
		url  string
	}{
		{"HTTP Server", suite.env.BaseURL + "/health"},
		{"WebSocket Server", suite.env.BaseURL}, // Basic connectivity check
	}

	for _, service := range services {
		section.Status("Checking %s availability...", service.name)
		
		for time.Now().Before(timeout) {
			resp, err := suite.httpClient.Get(service.url)
			if err == nil {
				resp.Body.Close()
				section.Status("✅ %s is ready", service.name)
				break
			}
			
			if time.Now().Add(checkInterval).After(timeout) {
				suite.T().Fatalf("❌ %s failed to become ready within %v. Last error: %v", 
					service.name, maxWait, err)
			}
			
			time.Sleep(checkInterval)
		}
	}
}

// waitForServerReady waits for a server to be ready with configurable timeout
func (suite *IntegrationTestSuite) waitForServerReady(url string, timeout time.Duration) {
	checkInterval := 100 * time.Millisecond
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		client := &http.Client{
			Timeout: 1 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
		
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}

		time.Sleep(checkInterval)
	}

	suite.T().Fatalf("Server failed to start within %v. URL: %s", timeout, url)
}

// registerTestHandlers registers handlers for local testing
func (suite *IntegrationTestSuite) registerTestHandlers() {
	suite.server.RegisterHandler("echo", handlers.EchoHandler)
	suite.server.RegisterHandler("time", handlers.TimeHandler)
	suite.server.RegisterHandler("status", handlers.StatusHandler)
	suite.server.RegisterHandler("calculate", handlers.CalculateHandler)
	suite.server.RegisterHandler("test_error", suite.errorHandler)
	suite.server.RegisterHandler("test_slow", suite.slowHandler)
}

// generateTestTLSConfig creates a self-signed certificate for testing
func (suite *IntegrationTestSuite) generateTestTLSConfig() *tls.Config {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(suite.T(), err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Test Organization"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test City"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(suite.T(), err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(suite.T(), err)

	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}
}

// TearDownSuite cleans up after tests
func (suite *IntegrationTestSuite) TearDownSuite() {
	section := testutil.NewSectionReporter("Integration Test Suite Teardown")
	section.SetQuietMode(suite.env.QuietMode)
	section.Start()

	suite.mu.Lock()
	defer suite.mu.Unlock()

	// Stop progress reporters
	if suite.progressGroup != nil {
		section.Status("Stopping progress reporters")
		suite.progressGroup.StopAll()
	}

	if suite.env.IsDocker {
		section.Status("🐳 Docker environment - skipping server shutdown")
		section.Status("Docker Compose will handle service cleanup")
	} else {
		section.Status("🖥️  Local environment - performing cleanup")
		
		if suite.server != nil {
			section.Status("Stopping local server")
			done := make(chan error, 1)
			go func() {
				done <- suite.server.Stop()
			}()

			select {
			case err := <-done:
				if err != nil {
					section.Status("Server stop error: %v", err)
				} else {
					section.Status("✅ Server stopped successfully")
				}
			case <-time.After(3 * time.Second):
				section.Status("⚠️  Server stop timeout - forcing cleanup")
			}
		}

		// Release allocated ports
		if len(suite.allocatedPorts) > 0 {
			section.Status("Releasing allocated ports")
			for _, port := range suite.allocatedPorts {
				ReleaseTestPort(port)
			}
			suite.allocatedPorts = nil
			
			stats := GetPortStats()
			section.Status("Final port utilization: %.1f%%", stats["utilization_pct"])
		}
	}

	if suite.logger != nil {
		section.Status("Closing logger")
		suite.logger.Close()
	}

	section.Status("✅ Cleanup completed")
	section.End()
}

// getEnvironmentDescription returns a human-readable environment description
func (suite *IntegrationTestSuite) getEnvironmentDescription() string {
	if suite.env.IsDocker {
		if suite.env.IsCI {
			return "Docker + CI"
		}
		return "Docker Compose"
	}
	if suite.env.IsCI {
		return "CI (Local)"
	}
	return "Local Development"
}

// Test handlers for integration testing
func (suite *IntegrationTestSuite) errorHandler(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
	return nil, fmt.Errorf("intentional test error")
}

func (suite *IntegrationTestSuite) slowHandler(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
	time.Sleep(2 * time.Second)
	return &types.JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  "slow response",
		ID:      req.ID,
	}, nil
}

// HTTP Integration Tests
func (suite *IntegrationTestSuite) TestHTTP_BasicRequest() {
	section := testutil.NewSectionReporter("HTTP Basic Request Test")
	section.SetQuietMode(suite.env.QuietMode)
	section.Start()

	section.Status("Creating test request")
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(`{"message": "integration test"}`),
		ID:      1,
	}

	section.Status("Sending HTTP request to %s", suite.env.BaseURL)
	response := suite.makeHTTPRequest(request)

	section.Status("Validating response")
	assert.Equal(suite.T(), "2.0", response.JSONRPC)
	assert.Equal(suite.T(), float64(1), response.ID)
	assert.Nil(suite.T(), response.Error)
	assert.NotNil(suite.T(), response.Result)

	result, ok := response.Result.(map[string]interface{})
	require.True(suite.T(), ok)

	echo, ok := result["echo"].(map[string]interface{})
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), "integration test", echo["message"])

	section.End()
}

func (suite *IntegrationTestSuite) TestHTTP_ErrorHandling() {
	section := testutil.NewSectionReporter("HTTP Error Handling Test")
	section.SetQuietMode(suite.env.QuietMode)
	section.Start()

	section.Status("Creating error-inducing request")
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test_error",
		ID:      2,
	}

	section.Status("Sending request that should cause server error")
	jsonData, err := json.Marshal(request)
	require.NoError(suite.T(), err)

	resp, err := suite.httpClient.Post(suite.env.BaseURL+"/rpc", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	section.Status("Validating error response")
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
	section.End()
}

func (suite *IntegrationTestSuite) TestHTTP_InvalidMethod() {
	section := testutil.NewSectionReporter("HTTP Invalid Method Test")
	section.SetQuietMode(suite.env.QuietMode)
	section.Start()

	section.Status("Creating request with invalid method")
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "nonexistent_method",
		ID:      3,
	}

	section.Status("Sending request")
	response := suite.makeHTTPRequest(request)

	section.Status("Validating method not found error")
	assert.Equal(suite.T(), "2.0", response.JSONRPC)
	assert.Equal(suite.T(), float64(3), response.ID)
	assert.NotNil(suite.T(), response.Error)
	assert.Equal(suite.T(), -32601, response.Error.Code)
	assert.Equal(suite.T(), "Method not found", response.Error.Message)
	section.End()
}

func (suite *IntegrationTestSuite) TestHTTP_InvalidJSON() {
	section := testutil.NewSectionReporter("HTTP Invalid JSON Test")
	section.SetQuietMode(suite.env.QuietMode)
	section.Start()

	section.Status("Sending malformed JSON")
	invalidJSON := `{"jsonrpc": "2.0", "method": "echo", "params": {invalid json}, "id": 1}`

	resp, err := suite.httpClient.Post(suite.env.BaseURL+"/rpc", "application/json", bytes.NewBufferString(invalidJSON))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	section.Status("Validating bad request response")
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
	section.End()
}

func (suite *IntegrationTestSuite) TestHTTP_MethodNotAllowed() {
	section := testutil.NewSectionReporter("HTTP Method Not Allowed Test")
	section.SetQuietMode(suite.env.QuietMode)
	section.Start()

	section.Status("Sending GET request to RPC endpoint")
	resp, err := suite.httpClient.Get(suite.env.BaseURL + "/rpc")
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	section.Status("Validating method not allowed response")
	assert.Equal(suite.T(), http.StatusMethodNotAllowed, resp.StatusCode)
	section.End()
}

// HTTPS Integration Tests
func (suite *IntegrationTestSuite) TestHTTPS_BasicRequest() {
	// Skip HTTPS test in Docker mode unless explicitly configured
	if suite.env.IsDocker && suite.env.HTTPSUrl == "" {
		suite.T().Skip("Skipping HTTPS test in Docker mode - not configured")
	}

	section := testutil.NewSectionReporter("HTTPS Basic Request Test")
	section.SetQuietMode(suite.env.QuietMode)
	section.Start()

	section.Status("Creating test request")
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "echo",
		Params:  json.RawMessage(`{"message": "https test"}`),
		ID:      "https-1",
	}

	section.Status("Sending HTTPS request to %s", suite.env.HTTPSUrl)
	jsonData, err := json.Marshal(request)
	require.NoError(suite.T(), err)

	resp, err := suite.httpClient.Post(suite.env.HTTPSUrl+"/rpc", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(suite.T(), err)

	var response types.JSONRPCResponse
	err = json.Unmarshal(body, &response)
	require.NoError(suite.T(), err)

	section.Status("Validating HTTPS response")
	assert.Equal(suite.T(), "2.0", response.JSONRPC)
	assert.Equal(suite.T(), "https-1", response.ID)
	assert.Nil(suite.T(), response.Error)
	assert.NotNil(suite.T(), response.Result)

	section.End()
}

// WebSocket Integration Tests
func (suite *IntegrationTestSuite) TestWebSocket_BasicCommunication() {
	section := testutil.NewSectionReporter("WebSocket Basic Communication Test")
	section.SetQuietMode(suite.env.QuietMode)
	section.Start()

	section.Status("Establishing WebSocket connection to %s", suite.env.WebSocketURL)
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.Dial(suite.env.WebSocketURL+"/ws", nil)
	require.NoError(suite.T(), err)
	defer conn.Close()

	// Send request
	section.Status("Sending WebSocket request")
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "time",
		ID:      "ws-test-1",
	}

	err = conn.WriteJSON(request)
	require.NoError(suite.T(), err)

	// Read response
	section.Status("Reading WebSocket response")
	var response types.JSONRPCResponse
	err = conn.ReadJSON(&response)
	require.NoError(suite.T(), err)

	section.Status("Validating WebSocket response")
	assert.Equal(suite.T(), "2.0", response.JSONRPC)
	assert.Equal(suite.T(), "ws-test-1", response.ID)
	assert.Nil(suite.T(), response.Error)
	assert.NotNil(suite.T(), response.Result)
	section.End()
}

// TCP Integration Tests
func (suite *IntegrationTestSuite) TestTCP_BasicCommunication() {
	section := testutil.NewSectionReporter("TCP Basic Communication Test")
	section.SetQuietMode(suite.env.QuietMode)
	section.Start()

	section.Status("Establishing TCP connection to %s", suite.env.TCPAddr)
	conn, err := net.Dial("tcp", suite.env.TCPAddr)
	require.NoError(suite.T(), err)
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	// Send request
	section.Status("Sending TCP request")
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "status",
		ID:      "tcp-test-1",
	}

	err = encoder.Encode(request)
	require.NoError(suite.T(), err)

	// Read response
	section.Status("Reading TCP response")
	var response types.JSONRPCResponse
	err = decoder.Decode(&response)
	require.NoError(suite.T(), err)

	section.Status("Validating TCP response")
	assert.Equal(suite.T(), "2.0", response.JSONRPC)
	assert.Equal(suite.T(), "tcp-test-1", response.ID)
	assert.Nil(suite.T(), response.Error)
	assert.NotNil(suite.T(), response.Result)
	section.End()
}

// TLS Integration Tests
func (suite *IntegrationTestSuite) TestTLS_BasicCommunication() {
	// Skip TLS test in Docker mode unless explicitly configured
	if suite.env.IsDocker && suite.env.TLSAddr == "" {
		suite.T().Skip("Skipping TLS test in Docker mode - not configured")
	}

	section := testutil.NewSectionReporter("TLS Basic Communication Test")
	section.SetQuietMode(suite.env.QuietMode)
	section.Start()

	section.Status("Establishing TLS connection to %s", suite.env.TLSAddr)
	conn, err := tls.Dial("tcp", suite.env.TLSAddr, &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(suite.T(), err)
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	// Send request
	section.Status("Sending TLS request")
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "status",
		ID:      "tls-test-1",
	}

	err = encoder.Encode(request)
	require.NoError(suite.T(), err)

	// Read response
	section.Status("Reading TLS response")
	var response types.JSONRPCResponse
	err = decoder.Decode(&response)
	require.NoError(suite.T(), err)

	section.Status("Validating TLS response")
	assert.Equal(suite.T(), "2.0", response.JSONRPC)
	assert.Equal(suite.T(), "tls-test-1", response.ID)
	assert.Nil(suite.T(), response.Error)
	assert.NotNil(suite.T(), response.Result)
	section.End()
}

// Load Testing
func (suite *IntegrationTestSuite) TestHTTP_ConcurrentRequests() {
	const numGoroutines = 10
	const requestsPerGoroutine = 5
	const totalRequests = numGoroutines * requestsPerGoroutine

	section := testutil.NewSectionReporter("HTTP Concurrent Requests Test")
	section.SetQuietMode(suite.env.QuietMode)
	section.Start()

	// Create a progress reporter for this mass operation
	progressReporter := testutil.NewProgressReporter("Concurrent HTTP Requests", totalRequests)
	progressReporter.SetSuppressDetails(true)
	progressReporter.Start()
	defer progressReporter.Stop()

	section.Status("Starting %d goroutines with %d requests each", numGoroutines, requestsPerGoroutine)

	var wg sync.WaitGroup
	results := make(chan bool, totalRequests)
	completed := 0
	successCount := 0
	errorCount := 0

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				request := types.JSONRPCRequest{
					JSONRPC: "2.0",
					Method:  "calculate",
					Params:  json.RawMessage(`{"operation": "add", "a": 1, "b": 1}`),
					ID:      fmt.Sprintf("%d-%d", workerID, j),
				}

				response := suite.makeHTTPRequest(request)
				success := response.Error == nil && response.Result != nil
				results <- success
			}
		}(i)
	}

	// Monitor progress in a separate goroutine
	go func() {
		for result := range results {
			completed++
			if result {
				successCount++
			} else {
				errorCount++
			}
			
			progressReporter.UpdateProgress(completed, totalRequests,
				fmt.Sprintf("Worker progress: ✓ %d success, ✗ %d errors", successCount, errorCount))
			
			if completed >= totalRequests {
				break
			}
		}
	}()

	wg.Wait()
	close(results)

	// Ensure all results are processed
	for completed < totalRequests {
		time.Sleep(5 * time.Millisecond)
	}

	// Show final results
	progressReporter.FinishProgress(successCount, errorCount)

	// Expect at least 90% success rate
	successRate := float64(successCount) / float64(totalRequests)
	section.Status("Concurrent test results: %d/%d successful (%.2f%%)",
		successCount, totalRequests, successRate*100)

	if successRate >= 0.90 {
		assert.GreaterOrEqual(suite.T(), successRate, 0.90)
		section.End()
	} else {
		section.Fail(fmt.Errorf("success rate too low: %.2f%%", successRate*100))
	}
}

// Helper methods
func (suite *IntegrationTestSuite) makeHTTPRequest(request types.JSONRPCRequest) *types.JSONRPCResponse {
	jsonData, err := json.Marshal(request)
	require.NoError(suite.T(), err)

	resp, err := suite.httpClient.Post(suite.env.BaseURL+"/rpc", "application/json", bytes.NewBuffer(jsonData))
	require.NoError(suite.T(), err)
	defer resp.Body.Close()

	// If this is a notification, there should be no response body
	if request.IsNotification() {
		body, err := io.ReadAll(resp.Body)
		require.NoError(suite.T(), err)
		if len(body) == 0 {
			return nil // Correct behavior for notifications
		}
	}

	body, err := io.ReadAll(resp.Body)
	require.NoError(suite.T(), err)

	// If body is empty, return nil
	if len(body) == 0 {
		return nil
	}

	var response types.JSONRPCResponse
	err = json.Unmarshal(body, &response)
	require.NoError(suite.T(), err)

	return &response
}

func (suite *IntegrationTestSuite) makeWebSocketRequest(request types.JSONRPCRequest) *types.JSONRPCResponse {
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.Dial(suite.env.WebSocketURL+"/ws", nil)
	require.NoError(suite.T(), err)
	defer conn.Close()

	err = conn.WriteJSON(request)
	require.NoError(suite.T(), err)

	// If this is a notification, there should be no response
	if request.IsNotification() {
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, _, err := conn.ReadMessage()
		if err != nil {
			return nil // Expected error for notifications
		}
	}

	var response types.JSONRPCResponse
	err = conn.ReadJSON(&response)
	require.NoError(suite.T(), err)

	return &response
}

func (suite *IntegrationTestSuite) makeTCPRequest(request types.JSONRPCRequest) *types.JSONRPCResponse {
	conn, err := net.Dial("tcp", suite.env.TCPAddr)
	require.NoError(suite.T(), err)
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	err = encoder.Encode(request)
	require.NoError(suite.T(), err)

	// If this is a notification, there should be no response
	if request.IsNotification() {
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		var response types.JSONRPCResponse
		err := decoder.Decode(&response)
		if err != nil {
			return nil // Expected error for notifications
		}
	}

	var response types.JSONRPCResponse
	err = decoder.Decode(&response)
	require.NoError(suite.T(), err)

	return &response
}

// Run the integration test suite
func TestIntegrationSuite(t *testing.T) {
	fmt.Print(testutil.FormatHeader("Integration Test Suite"))

	// Detect environment and show information
	env := DetectTestEnvironment()
	
	if !env.QuietMode {
		fmt.Printf("🔍 Environment Detection:\n")
		if env.IsDocker {
			fmt.Printf("   🐳 Docker Compose environment detected\n")
			fmt.Printf("   📡 Using container networking (no port management needed)\n")
		} else {
			fmt.Printf("   🖥️  Local development environment detected\n")
			fmt.Printf("   🔧 Using optimized port allocation\n")
			
			// Show port information for local development
			stats := GetPortStats()
			fmt.Printf("   📊 Port pool: %s (size: %v)\n", 
				stats["port_range"], stats["pool_size"])
		}
		fmt.Printf("   🤫 Tip: Set QUIET_TESTS=1 for minimal output\n")
		fmt.Printf("   🐳 Tip: Set DOCKER_COMPOSE_TEST=1 to force Docker mode\n\n")
	}

	suite.Run(t, new(IntegrationTestSuite))

	fmt.Print(testutil.FormatFooter())
}
