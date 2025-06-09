package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"streaming-server/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockLogWriter - это мок-реализация LogWriter для тестирования
type MockLogWriter struct {
	mock.Mock
	entries []LogEntry
}

func (m *MockLogWriter) Write(entry LogEntry) error {
	args := m.Called(entry)
	m.entries = append(m.entries, entry)
	return args.Error(0)
}

func (m *MockLogWriter) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockLogWriter) Flush() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockLogWriter) GetEntries() []LogEntry {
	return m.entries
}

func (m *MockLogWriter) Reset() {
	m.entries = nil
}

func TestDefaultLoggingConfig(t *testing.T) {
	config := DefaultLoggingConfig()

	assert.True(t, config.Enabled)
	assert.Equal(t, LogLevelInfo, config.Level)
	assert.Equal(t, LogFormatJSON, config.Format)
	assert.Equal(t, LogDestinationKafka, config.Destination)
	assert.True(t, config.LogSuccessOnly)
	assert.Equal(t, 1000, config.BufferSize)
	assert.Equal(t, 5*time.Second, config.FlushInterval)
	assert.Equal(t, "streaming-server", config.ServiceName)
	assert.Equal(t, "1.0.0", config.ServiceVersion)
	assert.NotNil(t, config.ExtraFields)
}

func TestNewKafkaLogWriter(t *testing.T) {
	tests := []struct {
		name        string
		config      LoggingConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "валидная конфигурация",
			config: LoggingConfig{
				KafkaBrokers: []string{"localhost:9092"},
				Topic:        "test-topic",
				BufferSize:   100,
				FlushInterval: time.Second,
			},
			expectError: false,
		},
		{
			name: "отсутствуют брокеры",
			config: LoggingConfig{
				Topic: "test-topic",
			},
			expectError: true,
			errorMsg:    "не настроены брокеры kafka",
		},
		{
			name: "отсутствует тема",
			config: LoggingConfig{
				KafkaBrokers: []string{"localhost:9092"},
			},
			expectError: true,
			errorMsg:    "не настроена тема kafka",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer, err := NewKafkaLogWriter(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, writer)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, writer)
				assert.NotNil(t, writer.writer)

				// Очистка
				writer.Close()
			}
		})
	}
}

func TestStdoutLogWriter(t *testing.T) {
	config := LoggingConfig{
		Format: LogFormatJSON,
	}

	writer := NewStdoutLogWriter(config)
	assert.NotNil(t, writer)

	entry := LogEntry{
		RequestID: "test-123",
		Method:    "test",
		Transport: "HTTP",
		Timestamp: time.Now(),
		Success:   true,
		Level:     LogLevelInfo,
	}

	err := writer.Write(entry)
	assert.NoError(t, err)

	// Тест текстового формата
	config.Format = LogFormatText
	writer = NewStdoutLogWriter(config)
	err = writer.Write(entry)
	assert.NoError(t, err)

	// Тест close и flush (должны быть пустыми операциями)
	assert.NoError(t, writer.Close())
	assert.NoError(t, writer.Flush())
}

func TestLogger_shouldLog(t *testing.T) {
	tests := []struct {
		name       string
		config     LoggingConfig
		req        *types.JSONRPCRequest
		success    bool
		hasError   bool
		shouldLog  bool
	}{
		{
			name: "отключенный логгер",
			config: LoggingConfig{
				Enabled: false,
			},
			req:       &types.JSONRPCRequest{Method: "test"},
			success:   true,
			hasError:  false,
			shouldLog: false,
		},
		{
			name: "только успешные - успешный случай",
			config: LoggingConfig{
				Enabled:        true,
				LogSuccessOnly: true,
			},
			req:       &types.JSONRPCRequest{Method: "test"},
			success:   true,
			hasError:  false,
			shouldLog: true,
		},
		{
			name: "только успешные - случай с ошибкой",
			config: LoggingConfig{
				Enabled:        true,
				LogSuccessOnly: true,
			},
			req:       &types.JSONRPCRequest{Method: "test"},
			success:   false,
			hasError:  true,
			shouldLog: false,
		},
		{
			name: "включенные методы - включен",
			config: LoggingConfig{
				Enabled:        true,
				IncludeMethods: []string{"test", "echo"},
			},
			req:       &types.JSONRPCRequest{Method: "test"},
			success:   true,
			hasError:  false,
			shouldLog: true,
		},
		{
			name: "включенные методы - не включен",
			config: LoggingConfig{
				Enabled:        true,
				IncludeMethods: []string{"echo", "status"},
			},
			req:       &types.JSONRPCRequest{Method: "test"},
			success:   true,
			hasError:  false,
			shouldLog: false,
		},
		{
			name: "исключенные методы - исключен",
			config: LoggingConfig{
				Enabled:        true,
				ExcludeMethods: []string{"test", "debug"},
			},
			req:       &types.JSONRPCRequest{Method: "test"},
			success:   true,
			hasError:  false,
			shouldLog: false,
		},
		{
			name: "исключенные методы - не исключен",
			config: LoggingConfig{
				Enabled:        true,
				ExcludeMethods: []string{"debug", "internal"},
			},
			req:       &types.JSONRPCRequest{Method: "test"},
			success:   true,
			hasError:  false,
			shouldLog: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &Logger{config: tt.config}
			result := logger.shouldLog(tt.req, tt.success, tt.hasError)
			assert.Equal(t, tt.shouldLog, result)
		})
	}
}

func TestLogger_createLogEntry_WithMockClock(t *testing.T) {
	// Используем мок-часы для детерминированного тестирования
	fixedTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	mockClock := types.NewMockClock(fixedTime)

	config := LoggingConfig{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		ExtraFields:    map[string]string{"env": "test"},
	}

	logger := &Logger{
		config: config,
		clock:  mockClock,
	}

	req := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      1,
	}

	// Создаем контекст с мок-часами
	ctx := types.NewRequestContextWithClock(context.Background(), "HTTP", "127.0.0.1:8080", mockClock)
	ctx.UserAgent = "test-agent"
	ctx.SelectedHandler = "test_handler"
	ctx.WithValue("test_key", "test_value")
	ctx.Headers["Content-Type"] = "application/json"

	// Продвигаем часы для симуляции длительности запроса
	mockClock.Advance(100 * time.Millisecond)

	tests := []struct {
		name     string
		response *types.JSONRPCResponse
		err      error
		expected func(entry LogEntry)
	}{
		{
			name: "успешный запрос",
			response: &types.JSONRPCResponse{
				JSONRPC: "2.0",
				Result:  "success",
				ID:      1,
			},
			err: nil,
			expected: func(entry LogEntry) {
				assert.True(t, entry.Success)
				assert.Equal(t, LogLevelInfo, entry.Level)
				assert.Nil(t, entry.ErrorCode)
				assert.Nil(t, entry.ErrorMsg)
				assert.Equal(t, int64(100), entry.Duration) // 100мс в мок-часах
			},
		},
		{
			name:     "запрос с ошибкой",
			response: nil,
			err:      errors.New("тестовая ошибка"),
			expected: func(entry LogEntry) {
				assert.False(t, entry.Success)
				assert.Equal(t, LogLevelError, entry.Level)
				assert.NotNil(t, entry.ErrorMsg)
				assert.Equal(t, "тестовая ошибка", *entry.ErrorMsg)
				assert.Equal(t, int64(100), entry.Duration)
			},
		},
		{
			name: "запрос с RPC ошибкой",
			response: &types.JSONRPCResponse{
				JSONRPC: "2.0",
				Error: &types.RPCError{
					Code:    -32601,
					Message: "Метод не найден",
				},
				ID: 1,
			},
			err: nil,
			expected: func(entry LogEntry) {
				assert.False(t, entry.Success)
				assert.Equal(t, LogLevelWarn, entry.Level)
				assert.NotNil(t, entry.ErrorCode)
				assert.Equal(t, -32601, *entry.ErrorCode)
				assert.NotNil(t, entry.ErrorMsg)
				assert.Equal(t, "Метод не найден", *entry.ErrorMsg)
				assert.Equal(t, int64(100), entry.Duration)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := logger.createLogEntry(req, ctx, tt.response, tt.err)

			// Общие проверки
			assert.Equal(t, ctx.RequestID, entry.RequestID)
			assert.Equal(t, req.Method, entry.Method)
			assert.Equal(t, ctx.Transport, entry.Transport)
			assert.Equal(t, ctx.RemoteAddr, entry.RemoteAddr)
			assert.Equal(t, ctx.UserAgent, entry.UserAgent)
			assert.Equal(t, ctx.SelectedHandler, entry.Handler)
			assert.Equal(t, config.ServiceName, entry.ServiceName)
			assert.Equal(t, config.ServiceVersion, entry.ServiceVersion)
			assert.Equal(t, fixedTime.Add(100*time.Millisecond), entry.Timestamp)
			assert.Contains(t, entry.Headers, "Content-Type")
			assert.Contains(t, entry.RequestData, "test_key")
			assert.Contains(t, entry.ExtraFields, "env")

			// Специфичные для теста проверки
			tt.expected(entry)
		})
	}
}

func TestLoggingMiddleware_WithMockAsyncProcessor(t *testing.T) {
	mockWriter := &MockLogWriter{}
	mockWriter.On("Write", mock.AnythingOfType("LogEntry")).Return(nil)

	mockAsyncProcessor := NewMockAsyncProcessor()
	mockClock := types.NewMockClock(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))

	config := LoggingConfig{
		Enabled:        true,
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
	}

	logger := &Logger{
		config:         config,
		writer:         mockWriter,
		asyncProcessor: mockAsyncProcessor,
		clock:          mockClock,
	}

	middleware := LoggingMiddleware(logger)

	req := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      1,
	}

	ctx := types.NewRequestContextWithClock(context.Background(), "HTTP", "127.0.0.1", mockClock)

	nextHandler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "success",
			ID:      req.ID,
		}, nil
	}

	response, err := middleware(req, ctx, nextHandler)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "success", response.Result)

	// Verify that async processing was called
	assert.Equal(t, 1, mockAsyncProcessor.GetProcessedFunctionCount())

	// Execute the processed function to trigger logging
	mockAsyncProcessor.ExecuteProcessedFunctions()

	// Verify that Write was called
	mockWriter.AssertCalled(t, "Write", mock.AnythingOfType("LogEntry"))
}

func TestLoggingMiddleware_WithError_MockAsyncProcessor(t *testing.T) {
	mockWriter := &MockLogWriter{}
	mockWriter.On("Write", mock.AnythingOfType("LogEntry")).Return(nil)

	mockAsyncProcessor := NewMockAsyncProcessor()
	mockClock := types.NewMockClock(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))

	config := LoggingConfig{
		Enabled:        true,
		LogSuccessOnly: false, // Log errors too
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
	}

	logger := &Logger{
		config:         config,
		writer:         mockWriter,
		asyncProcessor: mockAsyncProcessor,
		clock:          mockClock,
	}

	middleware := LoggingMiddleware(logger)

	req := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      1,
	}

	ctx := types.NewRequestContextWithClock(context.Background(), "HTTP", "127.0.0.1", mockClock)

	expectedError := errors.New("test error")
	nextHandler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return nil, expectedError
	}

	response, err := middleware(req, ctx, nextHandler)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, response)

	// Verify that async processing was called
	assert.Equal(t, 1, mockAsyncProcessor.GetProcessedFunctionCount())

	// Execute the processed function to trigger logging
	mockAsyncProcessor.ExecuteProcessedFunctions()

	// Verify that Write was called
	mockWriter.AssertCalled(t, "Write", mock.AnythingOfType("LogEntry"))
}

func TestLoggingMiddleware_FilteredOut_MockAsyncProcessor(t *testing.T) {
	mockWriter := &MockLogWriter{}
	mockAsyncProcessor := NewMockAsyncProcessor()

	config := LoggingConfig{
		Enabled:        true,
		LogSuccessOnly: true,
		ExcludeMethods: []string{"test"},
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
	}

	logger := &Logger{
		config:         config,
		writer:         mockWriter,
		asyncProcessor: mockAsyncProcessor,
		clock:          types.GlobalClock,
	}

	middleware := LoggingMiddleware(logger)

	req := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test", // This method is excluded
		ID:      1,
	}

	ctx := types.NewRequestContext(context.Background(), "HTTP", "127.0.0.1")

	nextHandler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "success",
			ID:      req.ID,
		}, nil
	}

	response, err := middleware(req, ctx, nextHandler)

	assert.NoError(t, err)
	assert.NotNil(t, response)

	// Verify that async processing was NOT called
	assert.Equal(t, 0, mockAsyncProcessor.GetProcessedFunctionCount())

	// Verify that Write was NOT called
	mockWriter.AssertNotCalled(t, "Write", mock.AnythingOfType("LogEntry"))
}

func TestLogger_Close(t *testing.T) {
	mockWriter := &MockLogWriter{}
	mockWriter.On("Close").Return(nil)

	mockAsyncProcessor := NewMockAsyncProcessor()

	logger := &Logger{
		writer:         mockWriter,
		asyncProcessor: mockAsyncProcessor,
	}

	err := logger.Close()
	assert.NoError(t, err)
	mockWriter.AssertCalled(t, "Close")
}

func TestLogger_Flush(t *testing.T) {
	mockWriter := &MockLogWriter{}
	mockWriter.On("Flush").Return(nil)

	logger := &Logger{
		writer: mockWriter,
	}

	err := logger.Flush()
	assert.NoError(t, err)
	mockWriter.AssertCalled(t, "Flush")
}

func TestNewLoggerWithDependencies(t *testing.T) {
	tests := []struct {
		name        string
		config      LoggingConfig
		expectError bool
	}{
		{
			name: "disabled logger",
			config: LoggingConfig{
				Enabled: false,
			},
			expectError: false,
		},
		{
			name: "stdout logger",
			config: LoggingConfig{
				Enabled:     true,
				Destination: LogDestinationStdout,
			},
			expectError: false,
		},
		{
			name: "kafka logger - valid config",
			config: LoggingConfig{
				Enabled:      true,
				Destination:  LogDestinationKafka,
				KafkaBrokers: []string{"localhost:9092"},
				Topic:        "test-topic",
			},
			expectError: false,
		},
		{
			name: "kafka logger - invalid config",
			config: LoggingConfig{
				Enabled:     true,
				Destination: LogDestinationKafka,
				// Missing brokers and topic
			},
			expectError: true,
		},
		{
			name: "unsupported destination",
			config: LoggingConfig{
				Enabled:     true,
				Destination: "unsupported",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAsyncProcessor := NewMockAsyncProcessor()
			mockClock := types.NewMockClock(time.Now())

			logger, err := NewLoggerWithDependencies(tt.config, mockAsyncProcessor, mockClock)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, logger)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, logger)

				if logger.writer != nil {
					logger.Close()
				}
			}
		})
	}
}

func TestLogEntry_JSON(t *testing.T) {
	entry := LogEntry{
		RequestID:      "test-123",
		Method:         "echo",
		Transport:      "HTTP",
		RemoteAddr:     "127.0.0.1",
		UserAgent:      "test-agent",
		Timestamp:      time.Now(),
		Duration:       100,
		Success:        true,
		Handler:        "echo_handler",
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Level:          LogLevelInfo,
		RequestData:    map[string]interface{}{"key": "value"},
		Headers:        map[string]string{"Content-Type": "application/json"},
		ExtraFields:    map[string]string{"env": "test"},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var unmarshaled LogEntry
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, entry.RequestID, unmarshaled.RequestID)
	assert.Equal(t, entry.Method, unmarshaled.Method)
	assert.Equal(t, entry.Transport, unmarshaled.Transport)
	assert.Equal(t, entry.Success, unmarshaled.Success)
	assert.Equal(t, entry.Level, unmarshaled.Level)
}

func TestKafkaLogWriter_Write_JSONFormat(t *testing.T) {
	config := LoggingConfig{
		Format: LogFormatJSON,
	}

	// Create a writer without actual Kafka connection for testing
	writer := &KafkaLogWriter{
		config: config,
		writer: nil, // This will cause Write to return an error
	}

	entry := LogEntry{
		RequestID: "test-123",
		Method:    "test",
		Transport: "HTTP",
		Success:   true,
		Level:     LogLevelInfo,
	}

	err := writer.Write(entry)
	assert.Error(t, err) // Should error because writer is nil
	assert.Contains(t, err.Error(), "писатель kafka не инициализирован")
}

func TestKafkaLogWriter_Write_TextFormat(t *testing.T) {
	config := LoggingConfig{
		Format: LogFormatText,
	}

	writer := &KafkaLogWriter{
		config: config,
		writer: nil, // This will cause Write to return an error
	}

	entry := LogEntry{
		RequestID: "test-123",
		Method:    "test",
		Transport: "HTTP",
		Success:   true,
		Level:     LogLevelInfo,
		Timestamp: time.Now(),
		Duration:  100,
		Handler:   "test_handler",
	}

	err := writer.Write(entry)
	assert.Error(t, err) // Should error because writer is nil
	assert.Contains(t, err.Error(), "писатель kafka не инициализирован")
}

// Benchmark tests
func BenchmarkLoggingMiddleware_WithMockAsyncProcessor(b *testing.B) {
	mockWriter := &MockLogWriter{}
	mockWriter.On("Write", mock.AnythingOfType("LogEntry")).Return(nil)

	mockAsyncProcessor := NewMockAsyncProcessor()
	mockClock := types.NewMockClock(time.Now())

	config := LoggingConfig{
		Enabled:        true,
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
	}

	logger := &Logger{
		config:         config,
		writer:         mockWriter,
		asyncProcessor: mockAsyncProcessor,
		clock:          mockClock,
	}

	middleware := LoggingMiddleware(logger)

	req := &types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test",
		ID:      1,
	}

	nextHandler := func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "success",
			ID:      req.ID,
		}, nil
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ctx := types.NewRequestContextWithClock(context.Background(), "HTTP", "127.0.0.1", mockClock)
		middleware(req, ctx, nextHandler)
	}
}
