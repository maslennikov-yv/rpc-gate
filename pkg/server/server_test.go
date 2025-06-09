package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"streaming-server/pkg/middleware"
	"streaming-server/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestServer(t *testing.T) (*Server, *middleware.Logger) {
	// Create a test logger
	logConfig := middleware.LoggingConfig{
		Enabled:     true,
		Destination: middleware.LogDestinationStdout,
		Format:      middleware.LogFormatJSON,
		Level:       middleware.LogLevelInfo,
	}
	logger, err := middleware.NewLogger(logConfig)
	require.NoError(t, err)

	// Create a minimal config
	config := Config{
		HTTPAddr:     ":0", // Use system-assigned port
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
		ServiceName:  "integration-test-server",
		Version:      "test-1.0.0",
	}

	// Create server
	server := NewServer(config, logger)
	return server, logger
}

func TestNewServer(t *testing.T) {
	config := Config{
		HTTPAddr:     ":8080",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logConfig := middleware.LoggingConfig{
		Enabled:     true,
		Destination: middleware.LogDestinationStdout,
		Format:      middleware.LogFormatJSON,
		Level:       middleware.LogLevelInfo,
	}
	logger, err := middleware.NewLogger(logConfig)
	require.NoError(t, err)

	server := NewServer(config, logger)

	assert.NotNil(t, server)
	assert.NotNil(t, server.dispatcher)
	assert.NotNil(t, server.processor)
	assert.Equal(t, config, server.config)
	assert.Equal(t, logger, server.logger)
}

func TestServer_RegisterHandler(t *testing.T) {
	server, logger := setupTestServer(t)

	handler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "test",
			ID:      req.ID,
		}, nil
	}

	server.RegisterHandler("test", handler)

	// Verify handler is registered by checking dispatcher
	dispatcher := server.GetDispatcher()
	assert.NotNil(t, dispatcher)
	assert.NotNil(t, logger)
}

func TestJSONRPCProcessor_ProcessSingleRequest_ValidRequest(t *testing.T) {
	server, _ := setupTestServer(t)

	requestData := `{"jsonrpc":"2.0","method":"echo","params":{"message":"test"},"id":"test-1"}`

	ctx := ProcessingContext{
		Transport:      "HTTP",
		RemoteAddr:     "127.0.0.1",
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
	}

	response := server.processor.ProcessSingleRequest([]byte(requestData), ctx)

	require.NotNil(t, response)
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, "test-1", response.ID)
	assert.Nil(t, response.Error)
	assert.NotNil(t, response.Result)
}

func TestJSONRPCProcessor_ProcessSingleRequest_InvalidJSON(t *testing.T) {
	server, _ := setupTestServer(t)

	requestData := `{"jsonrpc":"2.0","method":"echo","params":{"message":"test"},"id":}`

	ctx := ProcessingContext{
		Transport:   "HTTP",
		RemoteAddr:  "127.0.0.1",
		ServiceName: "test-service",
	}

	response := server.processor.ProcessSingleRequest([]byte(requestData), ctx)

	require.NotNil(t, response)
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Nil(t, response.ID)
	assert.NotNil(t, response.Error)
	assert.Equal(t, -32700, response.Error.Code) // Parse error
}

func TestJSONRPCProcessor_ProcessSingleRequest_InvalidVersion(t *testing.T) {
	server, _ := setupTestServer(t)

	requestData := `{"jsonrpc":"1.0","method":"echo","id":"test-1"}`

	ctx := ProcessingContext{
		Transport:   "HTTP",
		RemoteAddr:  "127.0.0.1",
		ServiceName: "test-service",
	}

	response := server.processor.ProcessSingleRequest([]byte(requestData), ctx)

	require.NotNil(t, response)
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, "test-1", response.ID)
	assert.NotNil(t, response.Error)
	assert.Equal(t, -32600, response.Error.Code) // Invalid request
}

func TestJSONRPCProcessor_ProcessSingleRequest_Notification(t *testing.T) {
	server, _ := setupTestServer(t)

	requestData := `{"jsonrpc":"2.0","method":"echo","params":{"message":"notification"}}`

	ctx := ProcessingContext{
		Transport:   "HTTP",
		RemoteAddr:  "127.0.0.1",
		ServiceName: "test-service",
	}

	response := server.processor.ProcessSingleRequest([]byte(requestData), ctx)

	// Notifications should return nil response
	assert.Nil(t, response)
}

func TestJSONRPCProcessor_ProcessSingleRequest_MethodNotFound(t *testing.T) {
	server, _ := setupTestServer(t)

	requestData := `{"jsonrpc":"2.0","method":"nonexistent","id":"test-1"}`

	ctx := ProcessingContext{
		Transport:   "HTTP",
		RemoteAddr:  "127.0.0.1",
		ServiceName: "test-service",
	}

	response := server.processor.ProcessSingleRequest([]byte(requestData), ctx)

	require.NotNil(t, response)
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, "test-1", response.ID)
	assert.NotNil(t, response.Error)
	assert.Equal(t, -32601, response.Error.Code) // Method not found
}

func TestJSONRPCProcessor_ProcessBatchRequest_ValidBatch(t *testing.T) {
	server, _ := setupTestServer(t)

	requestData := `[
		{"jsonrpc":"2.0","method":"echo","params":{"message":"test1"},"id":"1"},
		{"jsonrpc":"2.0","method":"echo","params":{"message":"test2"},"id":"2"}
	]`

	ctx := ProcessingContext{
		Transport:   "HTTP",
		RemoteAddr:  "127.0.0.1",
		ServiceName: "test-service",
	}

	result := server.processor.ProcessBatchRequest([]byte(requestData), ctx)

	require.NotNil(t, result)
	responses, ok := result.([]*types.JSONRPCResponse)
	require.True(t, ok)
	assert.Len(t, responses, 2)

	for i, response := range responses {
		assert.Equal(t, "2.0", response.JSONRPC)
		assert.Equal(t, string(rune('1'+i)), response.ID)
		assert.Nil(t, response.Error)
	}
}

func TestJSONRPCProcessor_ProcessBatchRequest_EmptyBatch(t *testing.T) {
	server, _ := setupTestServer(t)

	requestData := `[]`

	ctx := ProcessingContext{
		Transport:   "HTTP",
		RemoteAddr:  "127.0.0.1",
		ServiceName: "test-service",
	}

	result := server.processor.ProcessBatchRequest([]byte(requestData), ctx)

	require.NotNil(t, result)
	response, ok := result.(*types.JSONRPCResponse)
	require.True(t, ok)
	assert.NotNil(t, response.Error)
	assert.Equal(t, -32600, response.Error.Code) // Invalid request
}

func TestJSONRPCProcessor_ProcessBatchRequest_AllNotifications(t *testing.T) {
	server, _ := setupTestServer(t)

	requestData := `[
		{"jsonrpc":"2.0","method":"echo","params":{"message":"notification1"}},
		{"jsonrpc":"2.0","method":"echo","params":{"message":"notification2"}}
	]`

	ctx := ProcessingContext{
		Transport:   "HTTP",
		RemoteAddr:  "127.0.0.1",
		ServiceName: "test-service",
	}

	result := server.processor.ProcessBatchRequest([]byte(requestData), ctx)

	// All notifications should return nil
	assert.Nil(t, result)
}

func TestServer_handleHTTPRequest_ValidRequest(t *testing.T) {
	server, _ := setupTestServer(t)

	requestBody := `{"jsonrpc":"2.0","method":"echo","params":{"message":"test"},"id":"test-1"}`

	req := httptest.NewRequest("POST", "/rpc", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	server.handleHTTPRequest(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response types.JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, "test-1", response.ID)
	assert.Nil(t, response.Error)
}

func TestServer_handleHTTPRequest_InvalidMethod(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/rpc", nil)
	w := httptest.NewRecorder()

	server.handleHTTPRequest(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestServer_handleHTTPRequest_EmptyBody(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/rpc", strings.NewReader(""))
	w := httptest.NewRecorder()

	server.handleHTTPRequest(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response types.JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.NotNil(t, response.Error)
	// Проверяем, что код ошибки соответствует Invalid Request (-32600)
	assert.Equal(t, -32600, response.Error.Code, "Пустое тело должно вызывать ошибку Invalid Request")
}

func TestServer_handleHTTPRequest_Notification(t *testing.T) {
	// Создаем конфигурацию логгера
	logConfig := middleware.LoggingConfig{
		Enabled:     true,
		Destination: middleware.LogDestinationStdout,
		Format:      middleware.LogFormatJSON,
		Level:       middleware.LogLevelInfo,
	}

	// Создаем логгер
	logger, err := middleware.NewLogger(logConfig)
	require.NoError(t, err)

	// Создаем сервер с правильной конфигурацией
	server := NewServer(Config{
		ServiceName: "integration-test-server",
		Version:     "test-1.0.0",
	}, logger)

	// Создаем запрос-уведомление (без ID)
	requestBody := `{"jsonrpc":"2.0","method":"echo","params":{"message":"test notification"}}`

	// Создаем HTTP запрос
	req := httptest.NewRequest("POST", "/rpc", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	// Создаем ResponseRecorder для записи ответа
	w := httptest.NewRecorder()

	// Обрабатываем запрос
	server.handleHTTPRequest(w, req)

	// Проверяем, что статус ответа 200 OK
	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем, что тело ответа пустое (для уведомлений не должно быть ответа)
	responseBody := w.Body.String()
	t.Logf("Response body content: %q (length: %d)", responseBody, len(responseBody))
	t.Logf("Response body bytes: %v", w.Body.Bytes())

	// Проверяем, что тело ответа действительно пустое
	assert.Equal(t, "", responseBody, "Response body should be completely empty for notifications")
	assert.Equal(t, 0, w.Body.Len(), "Response body length should be 0 for notifications")
}

func TestServer_handleHTTPRequest_BatchRequest(t *testing.T) {
	server, _ := setupTestServer(t)

	requestBody := `[
		{"jsonrpc":"2.0","method":"echo","params":{"message":"test1"},"id":"1"},
		{"jsonrpc":"2.0","method":"echo","params":{"message":"test2"},"id":"2"}
	]`

	req := httptest.NewRequest("POST", "/rpc", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	server.handleHTTPRequest(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responses []*types.JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &responses)
	require.NoError(t, err)

	assert.Len(t, responses, 2)
}

func TestServer_handleHTTPRequest_CORS(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest("OPTIONS", "/rpc", nil)
	w := httptest.NewRecorder()

	server.handleHTTPRequest(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "POST, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type", w.Header().Get("Access-Control-Allow-Headers"))
}

func TestServer_handleHealth(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response["status"])
	assert.Contains(t, response, "timestamp")
	assert.Equal(t, "integration-test-server", response["service"])
	assert.Equal(t, "test-1.0.0", response["version"])
}

func TestConfig_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		valid  bool
	}{
		{
			name: "Valid config",
			config: Config{
				HTTPAddr:     ":8080",
				HTTPSAddr:    ":8443",
				TCPAddr:      ":9090",
				TLSAddr:      ":9443",
				WSAddr:       ":8081",
				WSSAddr:      ":8444",
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
				IdleTimeout:  60 * time.Second,
			},
			valid: true,
		},
		{
			name:   "Empty config",
			config: Config{},
			valid:  true, // Should work with defaults
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logConfig := middleware.LoggingConfig{
				Enabled:     true,
				Destination: middleware.LogDestinationStdout,
				Format:      middleware.LogFormatJSON,
				Level:       middleware.LogLevelInfo,
			}
			logger, err := middleware.NewLogger(logConfig)
			require.NoError(t, err)

			server := NewServer(tt.config, logger)

			if tt.valid {
				assert.NotNil(t, server)
				assert.Equal(t, tt.config, server.config)
			}
		})
	}
}

// Benchmark tests
func BenchmarkJSONRPCProcessor_ProcessSingleRequest(b *testing.B) {
	config := Config{}
	logConfig := middleware.LoggingConfig{
		Enabled:     true,
		Destination: middleware.LogDestinationStdout,
		Format:      middleware.LogFormatJSON,
		Level:       middleware.LogLevelInfo,
	}
	logger, _ := middleware.NewLogger(logConfig)
	server := NewServer(config, logger)

	requestData := []byte(`{"jsonrpc":"2.0","method":"echo","params":{"message":"benchmark"},"id":"bench-1"}`)

	ctx := ProcessingContext{
		Transport:   "HTTP",
		RemoteAddr:  "127.0.0.1",
		ServiceName: "test-service",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = server.processor.ProcessSingleRequest(requestData, ctx)
	}
}

func BenchmarkServer_handleHTTPRequest(b *testing.B) {
	config := Config{}
	logConfig := middleware.LoggingConfig{
		Enabled:     true,
		Destination: middleware.LogDestinationStdout,
		Format:      middleware.LogFormatJSON,
		Level:       middleware.LogLevelInfo,
	}
	logger, _ := middleware.NewLogger(logConfig)
	server := NewServer(config, logger)

	requestBody := `{"jsonrpc":"2.0","method":"echo","params":{"message":"benchmark"},"id":"bench-1"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/rpc", strings.NewReader(requestBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleHTTPRequest(w, req)
	}
}
