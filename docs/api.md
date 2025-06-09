# API Documentation

## Пакеты


### streaming-server/pkg/dispatcher

\`\`\`go
package dispatcher // import "streaming-server/pkg/dispatcher"


TYPES

type Dispatcher struct {
	// Has unexported fields.
}
    Dispatcher обрабатывает JSON-RPC запросы и направляет их к соответствующим
    обработчикам

func NewDispatcher() *Dispatcher
    NewDispatcher создает новый экземпляр диспетчера

func (d *Dispatcher) Dispatch(request *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error)
    Dispatch обрабатывает JSON-RPC запрос и возвращает ответ

func (d *Dispatcher) GetRegisteredMethods() []string
    GetRegisteredMethods возвращает список зарегистрированных методов

func (d *Dispatcher) HandlerCount() int
    HandlerCount возвращает количество зарегистрированных обработчиков

func (d *Dispatcher) RegisterHandler(method string, handler types.Handler)
    RegisterHandler регистрирует обработчик для указанного метода

func (d *Dispatcher) SetMiddleware(chain *middleware.Chain)
    SetMiddleware устанавливает middleware chain для диспетчера

func (d *Dispatcher) UnregisterHandler(method string)
    UnregisterHandler удаляет обработчик для указанного метода

\`\`\`

### streaming-server/pkg/handlers

\`\`\`go
package handlers // import "streaming-server/pkg/handlers"


FUNCTIONS

func CalculateHandler(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error)
    CalculateHandler performs basic arithmetic operations

func EchoHandler(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error)
    EchoHandler echoes back the received message with timestamp

func StatusHandler(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error)
    StatusHandler returns server status information

func TestSlowHandler(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error)
    TestSlowHandler simulates a slow operation for testing timeouts

func TimeHandler(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error)
    TimeHandler returns current server time

\`\`\`

### streaming-server/pkg/middleware

\`\`\`go
package middleware // import "streaming-server/pkg/middleware"


FUNCTIONS

func AuthenticationMiddleware() types.Middleware
    AuthenticationMiddleware предоставляет базовый промежуточный слой
    аутентификации

func HandlerSelectionMiddleware(handlerMap map[string]string) types.Middleware
    HandlerSelectionMiddleware облегчает выбор обработчика через контекст

func LoggingMiddleware(logger *Logger) types.Middleware
    LoggingMiddleware создает промежуточный слой логирования с указанной
    конфигурацией

func TracingMiddleware() types.Middleware
    TracingMiddleware добавляет поддержку распределенной трассировки


TYPES

type AsyncProcessor interface {
	Process(ctx context.Context, fn func()) error
	ProcessWithTimeout(ctx context.Context, fn func(), timeout time.Duration) error
	Shutdown(ctx context.Context) error
}
    AsyncProcessor интерфейс для обработки асинхронных операций

type Chain struct {
	// Has unexported fields.
}
    Chain represents a chain of middleware functions

func NewChain(middlewares ...types.Middleware) *Chain
    NewChain creates a new middleware chain

func (c *Chain) Add(middleware types.Middleware) *Chain
    Add appends middleware to the chain

func (c *Chain) Execute(req *types.JSONRPCRequest, ctx *types.RequestContext, finalHandler types.Handler) (*types.JSONRPCResponse, error)
    Execute executes the middleware chain with the final handler

type DefaultAsyncProcessor struct {
	// Has unexported fields.
}
    DefaultAsyncProcessor реализует AsyncProcessor с использованием горутин

func NewDefaultAsyncProcessor() *DefaultAsyncProcessor
    NewDefaultAsyncProcessor создает новый DefaultAsyncProcessor

func (p *DefaultAsyncProcessor) Process(ctx context.Context, fn func()) error
    Process выполняет функцию асинхронно

func (p *DefaultAsyncProcessor) ProcessWithTimeout(ctx context.Context, fn func(), timeout time.Duration) error
    ProcessWithTimeout выполняет функцию асинхронно с таймаутом

func (p *DefaultAsyncProcessor) Shutdown(ctx context.Context) error
    Shutdown корректно завершает работу процессора

type KafkaLogWriter struct {
	// Has unexported fields.
}
    KafkaLogWriter реализует LogWriter для Kafka

func NewKafkaLogWriter(config LoggingConfig) (*KafkaLogWriter, error)
    NewKafkaLogWriter создает новый писатель журнала Kafka

func (k *KafkaLogWriter) Close() error
    Close закрывает писатель Kafka

func (k *KafkaLogWriter) Flush() error
    Flush сбрасывает все ожидающие сообщения

func (k *KafkaLogWriter) Write(entry LogEntry) error
    Write записывает запись журнала в Kafka

type LogDestination string
    LogDestination представляет, куда должны отправляться журналы

const (
	LogDestinationKafka  LogDestination = "kafka"
	LogDestinationStdout LogDestination = "stdout"
	LogDestinationFile   LogDestination = "file"
)
type LogEntry struct {
	// Идентификация запроса
	RequestID string `json:"request_id"`
	TraceID   string `json:"trace_id,omitempty"`
	SpanID    string `json:"span_id,omitempty"`

	// Детали запроса
	Method     string `json:"method"`
	Transport  string `json:"transport"`
	RemoteAddr string `json:"remote_addr"`
	UserAgent  string `json:"user_agent,omitempty"`

	// Информация о времени
	Timestamp time.Time `json:"timestamp"`
	Duration  int64     `json:"duration_ms"`
	StartTime time.Time `json:"start_time"`

	// Информация об ответе
	Success   bool    `json:"success"`
	ErrorCode *int    `json:"error_code,omitempty"`
	ErrorMsg  *string `json:"error_message,omitempty"`

	// Информация об обработчике
	Handler string `json:"handler,omitempty"`

	// Информация о сервисе
	ServiceName    string `json:"service_name"`
	ServiceVersion string `json:"service_version"`

	// Метаданные журнала
	Level LogLevel `json:"level"`

	// Дополнительный контекст
	RequestData map[string]interface{} `json:"request_data,omitempty"`
	Headers     map[string]string      `json:"headers,omitempty"`
	ExtraFields map[string]string      `json:"extra_fields,omitempty"`
}
    LogEntry представляет структурированную запись журнала

type LogFormat string
    LogFormat представляет формат записей журнала

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)
type LogLevel string
    LogLevel представляет уровень серьезности записей журнала

const (
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelDebug LogLevel = "debug"
)
type LogWriter interface {
	Write(entry LogEntry) error
	Close() error
	Flush() error
}
    LogWriter интерфейс для различных направлений журнала

type Logger struct {
	// Has unexported fields.
}
    Logger обрабатывает операции логирования с асинхронной обработкой

func NewKafkaLogger(config LoggingConfig) *Logger
    NewKafkaLogger создает новый логгер Kafka (обратная совместимость) Устарело:
    Используйте NewLogger с LogDestinationKafka вместо этого

func NewLogger(config LoggingConfig) (*Logger, error)
    NewLogger создает новый логгер с указанной конфигурацией

func NewLoggerWithDependencies(config LoggingConfig, asyncProcessor AsyncProcessor, clock types.Clock) (*Logger, error)
    NewLoggerWithDependencies создает новый логгер с внедряемыми зависимостями

func (l *Logger) Close() error
    Close закрывает логгер и его писатель

func (l *Logger) Flush() error
    Flush сбрасывает все ожидающие записи журнала

type LoggingConfig struct {
	// Конфигурация Kafka
	KafkaBrokers []string `json:"kafka_brokers"`
	Topic        string   `json:"topic"`

	// Общая конфигурация
	Enabled     bool           `json:"enabled"`
	Level       LogLevel       `json:"level"`
	Format      LogFormat      `json:"format"`
	Destination LogDestination `json:"destination"`

	// Опции фильтрации
	LogSuccessOnly bool     `json:"log_success_only"`
	ExcludeMethods []string `json:"exclude_methods"`
	IncludeMethods []string `json:"include_methods"`

	// Опции производительности
	BufferSize    int           `json:"buffer_size"`
	FlushInterval time.Duration `json:"flush_interval"`

	// Логирование в файл (если destination - file)
	FilePath string `json:"file_path"`

	// Дополнительные метаданные
	ServiceName    string            `json:"service_name"`
	ServiceVersion string            `json:"service_version"`
	ExtraFields    map[string]string `json:"extra_fields"`
}
    LoggingConfig содержит конфигурацию для промежуточного слоя логирования

func DefaultLoggingConfig() LoggingConfig
    DefaultLoggingConfig возвращает конфигурацию логирования по умолчанию

type MockAsyncProcessor struct {
	// Has unexported fields.
}
    MockAsyncProcessor реализует AsyncProcessor для тестирования

func NewMockAsyncProcessor() *MockAsyncProcessor
    NewMockAsyncProcessor создает новый MockAsyncProcessor

func (m *MockAsyncProcessor) ExecuteProcessedFunctions()
    ExecuteProcessedFunctions выполняет все записанные функции синхронно

func (m *MockAsyncProcessor) GetProcessedFunctionCount() int
    GetProcessedFunctionCount возвращает количество обработанных функций

func (m *MockAsyncProcessor) Process(ctx context.Context, fn func()) error
    Process записывает функцию для тестирования и опционально выполняет ее

func (m *MockAsyncProcessor) ProcessWithTimeout(ctx context.Context, fn func(), timeout time.Duration) error
    ProcessWithTimeout записывает функцию для тестирования

func (m *MockAsyncProcessor) Reset()
    Reset очищает все записанное состояние

func (m *MockAsyncProcessor) SetProcessErrors(errors ...error)
    SetProcessErrors устанавливает ошибки, которые будут возвращены вызовами
    Process

func (m *MockAsyncProcessor) SetShutdownError(err error)
    SetShutdownError устанавливает ошибку, которая будет возвращена методом
    Shutdown

func (m *MockAsyncProcessor) Shutdown(ctx context.Context) error
    Shutdown возвращает настроенную ошибку завершения

type StdoutLogWriter struct {
	// Has unexported fields.
}
    StdoutLogWriter реализует LogWriter для stdout

func NewStdoutLogWriter(config LoggingConfig) *StdoutLogWriter
    NewStdoutLogWriter создает новый писатель журнала stdout

func (s *StdoutLogWriter) Close() error
    Close является пустой операцией для писателя stdout

func (s *StdoutLogWriter) Flush() error
    Flush является пустой операцией для писателя stdout

func (s *StdoutLogWriter) Write(entry LogEntry) error
    Write записывает запись журнала в stdout

\`\`\`

### streaming-server/pkg/server

\`\`\`go
package server // import "streaming-server/pkg/server"


TYPES

type Config struct {
	HTTPAddr     string
	HTTPSAddr    string
	TCPAddr      string
	TLSAddr      string
	WSAddr       string
	WSSAddr      string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	TLSConfig    *tls.Config
	ServiceName  string
	Version      string
}
    Config содержит конфигурацию сервера

type JSONRPCProcessor struct {
	// Has unexported fields.
}
    JSONRPCProcessor обрабатывает JSON-RPC запросы

func NewJSONRPCProcessor(dispatcher *dispatcher.Dispatcher, logger *middleware.Logger) *JSONRPCProcessor
    NewJSONRPCProcessor создает новый процессор JSON-RPC

func (p *JSONRPCProcessor) ProcessBatchRequest(data []byte, ctx ProcessingContext) interface{}
    ProcessBatchRequest обрабатывает пакетный JSON-RPC запрос

func (p *JSONRPCProcessor) ProcessSingleRequest(data []byte, ctx ProcessingContext) *types.JSONRPCResponse
    ProcessSingleRequest обрабатывает одиночный JSON-RPC запрос

type ProcessingContext struct {
	Transport      string
	RemoteAddr     string
	HTTPRequest    *http.Request
	ServiceName    string
	ServiceVersion string
	Headers        http.Header
	UserAgent      string
}
    ProcessingContext содержит контекст обработки запроса

type Server struct {
	// Has unexported fields.
}
    Server представляет JSON-RPC сервер

func NewServer(config Config, logger *middleware.Logger) *Server
    NewServer создает новый экземпляр сервера

func (s *Server) GetDispatcher() *dispatcher.Dispatcher
    GetDispatcher возвращает диспетчер сервера

func (s *Server) RegisterHandler(method string, handler types.Handler)
    RegisterHandler регистрирует обработчик для указанного метода

func (s *Server) Start() error
    Start starts all configured server protocols

func (s *Server) Stop() error
    Stop gracefully stops the server

\`\`\`

### streaming-server/pkg/testutil

\`\`\`go
package testutil // import "streaming-server/pkg/testutil"


CONSTANTS

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[1;31m"
	ColorGreen  = "\033[1;32m"
	ColorYellow = "\033[1;33m"
	ColorBlue   = "\033[1;34m"
	ColorPurple = "\033[1;35m"
	ColorCyan   = "\033[1;36m"
	ColorWhite  = "\033[1;37m"
	ColorGray   = "\033[0;37m"

	// Background colors
	BgBlack   = "\033[40m"
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
	BgWhite   = "\033[47m"
)
    Color codes for terminal output


FUNCTIONS

func FormatFooter() string
    FormatFooter formats a footer for test output

func FormatHeader(title string) string
    FormatHeader formats a header for test output


TYPES

type ProgressGroup struct {
	// Has unexported fields.
}
    ProgressGroup manages multiple progress reporters

func NewProgressGroup() *ProgressGroup
    NewProgressGroup creates a new progress group

func (pg *ProgressGroup) AddReporter(name string, reporter *ProgressReporter)
    AddReporter adds a reporter to the group

func (pg *ProgressGroup) GetReporter(name string) *ProgressReporter
    GetReporter gets a reporter by name

func (pg *ProgressGroup) SetQuietMode(quiet bool)
    SetQuietMode sets quiet mode for all reporters

func (pg *ProgressGroup) StopAll()
    StopAll stops all reporters in the group

type ProgressReporter struct {
	// Has unexported fields.
}
    ProgressReporter provides real-time progress updates for long-running tests

func NewProgressReporter(testName string, total int) *ProgressReporter

func (p *ProgressReporter) FinishProgress(successCount, errorCount int)
    FinishProgress completes the progress and shows final status

func (p *ProgressReporter) GetCurrent() int
    GetCurrent returns the current progress count

func (p *ProgressReporter) GetTotal() int
    GetTotal returns the total progress count

func (p *ProgressReporter) Increment()
    Increment increases the current progress count

func (p *ProgressReporter) IncrementBy(n int)
    IncrementBy increases the current progress count by the specified amount

func (p *ProgressReporter) IsComplete() bool
    IsComplete returns true if progress is complete

func (p *ProgressReporter) Message(format string, args ...interface{})
    Message displays a message without affecting the progress bar

func (p *ProgressReporter) SetHideRequests(hide bool)
    NewProgressReporter creates a new progress reporter for a test

func (p *ProgressReporter) SetQuietMode(quiet bool)
    SetQuietMode enables or disables quiet mode (minimal output)

func (p *ProgressReporter) SetSuppressDetails(suppress bool)
    SetSuppressDetails enables or disables detailed message suppression for mass
    operations

func (p *ProgressReporter) SetTotal(total int)
    SetTotal updates the total number of items

func (p *ProgressReporter) Start()
    Start begins the progress reporting

func (p *ProgressReporter) Stop()
    Stop ends the progress reporting

func (p *ProgressReporter) UpdateProgress(current, total int, message string)
    UpdateProgress updates progress with a custom message without creating new
    lines

type SectionReporter struct {
	// Has unexported fields.
}
    SectionReporter reports progress for a test section

func NewSectionReporter(name string) *SectionReporter
    NewSectionReporter creates a new section reporter

func (s *SectionReporter) End()
    End completes the section

func (s *SectionReporter) Fail(err error)
    Fail marks the section as failed

func (s *SectionReporter) SetHideDetails(hide bool)
    SetHideDetails sets whether to hide detailed status messages

func (s *SectionReporter) SetQuietMode(quiet bool)
    SetQuietMode enables or disables quiet mode

func (s *SectionReporter) Start()
    Start begins the section

func (s *SectionReporter) Status(format string, args ...interface{})
    Status reports a status update for the section

type TestProgressLogger struct {
	// Has unexported fields.
}
    TestProgressLogger provides a simple logger for test progress

func NewTestProgressLogger(testName string) *TestProgressLogger
    NewTestProgressLogger creates a new test progress logger

func (l *TestProgressLogger) Debugf(format string, args ...interface{})
    Debugf logs a debug message

func (l *TestProgressLogger) Errorf(format string, args ...interface{})
    Errorf logs an error message

func (l *TestProgressLogger) Infof(format string, args ...interface{})
    Infof logs an informational message

func (l *TestProgressLogger) SetHideDetails(hide bool)
    SetHideDetails sets whether to hide detailed log messages

func (l *TestProgressLogger) SetQuietMode(quiet bool)
    SetQuietMode enables or disables quiet mode

func (l *TestProgressLogger) Successf(format string, args ...interface{})
    Successf logs a success message

func (l *TestProgressLogger) Warnf(format string, args ...interface{})
    Warnf logs a warning message

\`\`\`

### streaming-server/pkg/types

\`\`\`go
package types // import "streaming-server/pkg/types"


CONSTANTS

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
    Стандартные коды ошибок JSON-RPC 2.0


TYPES

type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
	Sleep(d time.Duration)
	After(d time.Duration) <-chan time.Time
}
    Clock интерфейс позволяет создавать моки для операций со временем в тестах

var GlobalClock Clock = &RealClock{}
    Глобальный экземпляр часов - может быть заменен для тестирования

type DefaultIDGenerator struct{}
    DefaultIDGenerator реализует IDGenerator с использованием crypto/rand

func (g *DefaultIDGenerator) Generate() string
    Generate создает криптографически безопасный случайный ID

type Handler func(*JSONRPCRequest, *RequestContext) (*JSONRPCResponse, error)
    Handler представляет обработчик метода JSON-RPC

type IDGenerator interface {
	Generate() string
}
    IDGenerator интерфейс для генерации идентификаторов запросов

var GlobalIDGenerator IDGenerator = &DefaultIDGenerator{}
    Глобальный генератор ID - может быть заменен для тестирования

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
}
    JSONRPCRequest представляет запрос JSON-RPC 2.0

func (r *JSONRPCRequest) IsNotification() bool
    IsNotification проверяет, является ли запрос уведомлением (без ID)

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}
    JSONRPCResponse представляет ответ JSON-RPC 2.0

type Middleware func(*JSONRPCRequest, *RequestContext, Handler) (*JSONRPCResponse, error)
    Middleware представляет функцию промежуточного слоя

type MockClock struct {
	// Has unexported fields.
}
    MockClock реализует Clock для тестирования с контролируемым временем

func NewMockClock(initialTime time.Time) *MockClock
    NewMockClock создает новый MockClock с заданным начальным временем

func (m *MockClock) Advance(d time.Duration)
    Advance продвигает мок-часы на указанную продолжительность

func (m *MockClock) After(d time.Duration) <-chan time.Time
    After создает канал, который получит время после продвижения

func (m *MockClock) GetSleepCalls() []time.Duration
    GetSleepCalls возвращает все записанные продолжительности сна

func (m *MockClock) Now() time.Time
    Now возвращает текущее мок-время

func (m *MockClock) Reset()
    Reset сбрасывает состояние мок-часов

func (m *MockClock) SetTime(t time.Time)
    SetTime устанавливает текущее мок-время

func (m *MockClock) Since(t time.Time) time.Duration
    Since возвращает продолжительность с момента t, используя мок-время

func (m *MockClock) Sleep(d time.Duration)
    Sleep записывает продолжительность сна, но на самом деле не спит

type MockIDGenerator struct {
	// Has unexported fields.
}
    MockIDGenerator реализует IDGenerator для тестирования

func NewMockIDGenerator(ids ...string) *MockIDGenerator
    NewMockIDGenerator создает новый MockIDGenerator с предопределенными ID

func (m *MockIDGenerator) Generate() string
    Generate возвращает следующий предопределенный ID

func (m *MockIDGenerator) Reset()
    Reset сбрасывает генератор для начала с начала

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
    RPCError представляет ошибку JSON-RPC 2.0

func NewInternalError(data interface{}) *RPCError
    NewInternalError создает внутреннюю ошибку

func NewInvalidParamsError(data interface{}) *RPCError
    NewInvalidParamsError создает ошибку неверных параметров

func NewInvalidRequestError(data interface{}) *RPCError
    NewInvalidRequestError создает ошибку неверного запроса

func NewMethodNotFoundError(method string) *RPCError
    NewMethodNotFoundError создает ошибку "метод не найден"

func NewParseError(data interface{}) *RPCError
    NewParseError создает ошибку парсинга

type RealClock struct{}
    RealClock реализует Clock используя стандартный пакет time

func (c *RealClock) After(d time.Duration) <-chan time.Time
    After ожидает истечения указанного времени и затем отправляет текущее время
    в возвращаемый канал

func (c *RealClock) Now() time.Time
    Now возвращает текущее время

func (c *RealClock) Since(t time.Time) time.Duration
    Since возвращает время, прошедшее с момента t

func (c *RealClock) Sleep(d time.Duration)
    Sleep приостанавливает текущую горутину как минимум на время d

type RequestContext struct {
	RequestID       string
	Transport       string
	RemoteAddr      string
	StartTime       time.Time
	UserAgent       string
	Headers         map[string]string
	Data            map[string]interface{}
	Span            interface{} // Используем interface{} чтобы избежать зависимости импорта
	HTTPRequest     *http.Request
	SelectedHandler string
	// Has unexported fields.
}
    RequestContext содержит данные и метаданные, специфичные для запроса

func NewRequestContext(ctx context.Context, transport, remoteAddr string) *RequestContext
    NewRequestContext создает новый контекст запроса

func NewRequestContextWithClock(ctx context.Context, transport, remoteAddr string, clock Clock) *RequestContext
    NewRequestContextWithClock создает новый контекст запроса с определенными
    часами

func (rc *RequestContext) Context() context.Context
    Context возвращает базовый context.Context

func (rc *RequestContext) Duration() time.Duration
    Duration возвращает время, прошедшее с начала запроса

func (rc *RequestContext) GetValue(key string) (interface{}, bool)
    GetValue извлекает значение из данных контекста запроса

func (rc *RequestContext) WithValue(key string, value interface{})
    WithValue добавляет пару ключ-значение в данные контекста запроса

\`\`\`
