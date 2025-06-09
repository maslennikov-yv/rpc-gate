package types

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Глобальный экземпляр часов - может быть заменен для тестирования
var GlobalClock Clock = &RealClock{}

// JSONRPCRequest представляет запрос JSON-RPC 2.0
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
}

// IsNotification проверяет, является ли запрос уведомлением (без ID)
func (r *JSONRPCRequest) IsNotification() bool {
	return r.ID == nil
}

// JSONRPCResponse представляет ответ JSON-RPC 2.0
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// RPCError представляет ошибку JSON-RPC 2.0
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Стандартные коды ошибок JSON-RPC 2.0
const (
	// Предопределенные коды ошибок
	ParseError     = -32700 // Неверный JSON
	InvalidRequest = -32600 // JSON не является валидным объектом запроса
	MethodNotFound = -32601 // Метод не найден
	InvalidParams  = -32602 // Неверные параметры
	InternalError  = -32603 // Внутренняя ошибка
	
	// Коды ошибок сервера (зарезервированы для реализации)
	ServerErrorStart = -32099
	ServerErrorEnd   = -32000
)

// NewParseError создает ошибку парсинга
func NewParseError(data interface{}) *RPCError {
	return &RPCError{
		Code:    ParseError,
		Message: "Parse error",
		Data:    data,
	}
}

// NewInvalidRequestError создает ошибку неверного запроса
func NewInvalidRequestError(data interface{}) *RPCError {
	return &RPCError{
		Code:    InvalidRequest,
		Message: "Invalid Request",
		Data:    data,
	}
}

// NewMethodNotFoundError создает ошибку "метод не найден"
func NewMethodNotFoundError(method string) *RPCError {
	return &RPCError{
		Code:    MethodNotFound,
		Message: "Method not found",
		Data:    method,
	}
}

// NewInvalidParamsError создает ошибку неверных параметров
func NewInvalidParamsError(data interface{}) *RPCError {
	message := "Invalid params"
	if data != nil {
		if str, ok := data.(string); ok && str != "" {
			message = fmt.Sprintf("Invalid params: %s", str)
		}
	}
	return &RPCError{
		Code:    InvalidParams,
		Message: message,
		Data:    data,
	}
}

// NewInternalError создает внутреннюю ошибку
func NewInternalError(data interface{}) *RPCError {
	return &RPCError{
		Code:    InternalError,
		Message: "Internal error",
		Data:    data,
	}
}

// RequestContext содержит данные и метаданные, специфичные для запроса
type RequestContext struct {
	ctx           context.Context
	RequestID     string
	Transport     string
	RemoteAddr    string
	StartTime     time.Time
	UserAgent     string
	Headers       map[string]string
	Data          map[string]interface{}
	Span          interface{} // Используем interface{} чтобы избежать зависимости импорта
	HTTPRequest   *http.Request
	SelectedHandler string
	clock         Clock // Внедряемые часы для тестирования
}

// NewRequestContext создает новый контекст запроса
func NewRequestContext(ctx context.Context, transport, remoteAddr string) *RequestContext {
	return NewRequestContextWithClock(ctx, transport, remoteAddr, GlobalClock)
}

// NewRequestContextWithClock создает новый контекст запроса с определенными часами
func NewRequestContextWithClock(ctx context.Context, transport, remoteAddr string, clock Clock) *RequestContext {
	return &RequestContext{
		ctx:        ctx,
		RequestID:  generateRequestID(),
		Transport:  transport,
		RemoteAddr: remoteAddr,
		StartTime:  clock.Now(),
		Headers:    make(map[string]string),
		Data:       make(map[string]interface{}),
		clock:      clock,
	}
}

// Context возвращает базовый context.Context
func (rc *RequestContext) Context() context.Context {
	return rc.ctx
}

// WithValue добавляет пару ключ-значение в данные контекста запроса
func (rc *RequestContext) WithValue(key string, value interface{}) {
	rc.Data[key] = value
}

// GetValue извлекает значение из данных контекста запроса
func (rc *RequestContext) GetValue(key string) (interface{}, bool) {
	value, exists := rc.Data[key]
	return value, exists
}

// Duration возвращает время, прошедшее с начала запроса
func (rc *RequestContext) Duration() time.Duration {
	if rc.clock != nil {
		return rc.clock.Since(rc.StartTime)
	}
	return time.Since(rc.StartTime)
}

// Handler представляет обработчик метода JSON-RPC
type Handler func(*JSONRPCRequest, *RequestContext) (*JSONRPCResponse, error)

// Middleware представляет функцию промежуточного слоя
type Middleware func(*JSONRPCRequest, *RequestContext, Handler) (*JSONRPCResponse, error)

// IDGenerator интерфейс для генерации идентификаторов запросов
type IDGenerator interface {
	Generate() string
}

// DefaultIDGenerator реализует IDGenerator с использованием crypto/rand
type DefaultIDGenerator struct{}

// Generate создает криптографически безопасный случайный ID
func (g *DefaultIDGenerator) Generate() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Запасной вариант с ID на основе времени, если crypto/rand не работает
		return GlobalClock.Now().Format("20060102150405") + "-fallback"
	}
	return hex.EncodeToString(bytes)
}

// Глобальный генератор ID - может быть заменен для тестирования
var GlobalIDGenerator IDGenerator = &DefaultIDGenerator{}

// generateRequestID генерирует уникальный идентификатор запроса
func generateRequestID() string {
	return GlobalIDGenerator.Generate()
}

// MockIDGenerator реализует IDGenerator для тестирования
type MockIDGenerator struct {
	ids []string
	idx int
}

// NewMockIDGenerator создает новый MockIDGenerator с предопределенными ID
func NewMockIDGenerator(ids ...string) *MockIDGenerator {
	return &MockIDGenerator{
		ids: ids,
		idx: 0,
	}
}

// Generate возвращает следующий предопределенный ID
func (m *MockIDGenerator) Generate() string {
	if m.idx >= len(m.ids) {
		return "mock-id-overflow"
	}
	id := m.ids[m.idx]
	m.idx++
	return id
}

// Reset сбрасывает генератор для начала с начала
func (m *MockIDGenerator) Reset() {
	m.idx = 0
}
