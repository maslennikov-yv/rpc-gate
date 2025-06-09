package middleware

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "sync"
    "time"
    
    "streaming-server/pkg/types"
    "github.com/segmentio/kafka-go"
)

// LogLevel представляет уровень серьезности записей журнала
type LogLevel string

const (
    LogLevelInfo  LogLevel = "info"
    LogLevelWarn  LogLevel = "warn"
    LogLevelError LogLevel = "error"
    LogLevelDebug LogLevel = "debug"
)

// LogFormat представляет формат записей журнала
type LogFormat string

const (
    LogFormatJSON LogFormat = "json"
    LogFormatText LogFormat = "text"
)

// LogDestination представляет, куда должны отправляться журналы
type LogDestination string

const (
    LogDestinationKafka  LogDestination = "kafka"
    LogDestinationStdout LogDestination = "stdout"
    LogDestinationFile   LogDestination = "file"
)

// LoggingConfig содержит конфигурацию для промежуточного слоя логирования
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

// DefaultLoggingConfig возвращает конфигурацию логирования по умолчанию
func DefaultLoggingConfig() LoggingConfig {
    return LoggingConfig{
        Enabled:        true,
        Level:          LogLevelInfo,
        Format:         LogFormatJSON,
        Destination:    LogDestinationKafka,
        LogSuccessOnly: true,
        BufferSize:     1000,
        FlushInterval:  5 * time.Second,
        ServiceName:    "streaming-server",
        ServiceVersion: "1.0.0",
        ExtraFields:    make(map[string]string),
    }
}

// LogEntry представляет структурированную запись журнала
type LogEntry struct {
    // Идентификация запроса
    RequestID   string      `json:"request_id"`
    TraceID     string      `json:"trace_id,omitempty"`
    SpanID      string      `json:"span_id,omitempty"`
    
    // Детали запроса
    Method      string      `json:"method"`
    Transport   string      `json:"transport"`
    RemoteAddr  string      `json:"remote_addr"`
    UserAgent   string      `json:"user_agent,omitempty"`
    
    // Информация о времени
    Timestamp   time.Time   `json:"timestamp"`
    Duration    int64       `json:"duration_ms"`
    StartTime   time.Time   `json:"start_time"`
    
    // Информация об ответе
    Success     bool        `json:"success"`
    ErrorCode   *int        `json:"error_code,omitempty"`
    ErrorMsg    *string     `json:"error_message,omitempty"`
    
    // Информация об обработчике
    Handler     string      `json:"handler,omitempty"`
    
    // Информация о сервисе
    ServiceName    string `json:"service_name"`
    ServiceVersion string `json:"service_version"`
    
    // Метаданные журнала
    Level       LogLevel    `json:"level"`
    
    // Дополнительный контекст
    RequestData map[string]interface{} `json:"request_data,omitempty"`
    Headers     map[string]string      `json:"headers,omitempty"`
    ExtraFields map[string]string      `json:"extra_fields,omitempty"`
}

// LogWriter интерфейс для различных направлений журнала
type LogWriter interface {
    Write(entry LogEntry) error
    Close() error
    Flush() error
}

// KafkaLogWriter реализует LogWriter для Kafka
type KafkaLogWriter struct {
    writer *kafka.Writer
    config LoggingConfig
    mu     sync.RWMutex
}

// NewKafkaLogWriter создает новый писатель журнала Kafka
func NewKafkaLogWriter(config LoggingConfig) (*KafkaLogWriter, error) {
    if len(config.KafkaBrokers) == 0 {
        return nil, fmt.Errorf("не настроены брокеры kafka")
    }
    
    if config.Topic == "" {
        return nil, fmt.Errorf("не настроена тема kafka")
    }
    
    writer := &kafka.Writer{
        Addr:         kafka.TCP(config.KafkaBrokers...),
        Topic:        config.Topic,
        Balancer:     &kafka.LeastBytes{},
        RequiredAcks: kafka.RequireOne,
        Async:        true,
        BatchSize:    config.BufferSize,
        BatchTimeout: config.FlushInterval,
    }
    
    return &KafkaLogWriter{
        writer: writer,
        config: config,
    }, nil
}

// Write записывает запись журнала в Kafka
func (k *KafkaLogWriter) Write(entry LogEntry) error {
    k.mu.RLock()
    defer k.mu.RUnlock()
    
    if k.writer == nil {
        return fmt.Errorf("писатель kafka не инициализирован")
    }
    
    var data []byte
    var err error
    
    switch k.config.Format {
    case LogFormatJSON:
        data, err = json.Marshal(entry)
    case LogFormatText:
        data = []byte(k.formatTextEntry(entry))
    default:
        data, err = json.Marshal(entry)
    }
    
    if err != nil {
        return fmt.Errorf("не удалось отформатировать запись журнала: %w", err)
    }
    
    message := kafka.Message{
        Key:   []byte(entry.RequestID),
        Value: data,
        Time:  entry.Timestamp,
        Headers: []kafka.Header{
            {Key: "service", Value: []byte(entry.ServiceName)},
            {Key: "version", Value: []byte(entry.ServiceVersion)},
            {Key: "transport", Value: []byte(entry.Transport)},
            {Key: "method", Value: []byte(entry.Method)},
        },
    }
    
    return k.writer.WriteMessages(context.Background(), message)
}

// formatTextEntry форматирует запись журнала как обычный текст
func (k *KafkaLogWriter) formatTextEntry(entry LogEntry) string {
    status := "УСПЕХ"
    if !entry.Success {
        status = "ОШИБКА"
    }
    
    return fmt.Sprintf("[%s] %s %s %s %s %dмс - %s (ID: %s)",
        entry.Timestamp.Format(time.RFC3339),
        entry.Level,
        entry.Transport,
        entry.Method,
        status,
        entry.Duration,
        entry.Handler,
        entry.RequestID,
    )
}

// Close закрывает писатель Kafka
func (k *KafkaLogWriter) Close() error {
    k.mu.Lock()
    defer k.mu.Unlock()
    
    if k.writer != nil {
        return k.writer.Close()
    }
    return nil
}

// Flush сбрасывает все ожидающие сообщения
func (k *KafkaLogWriter) Flush() error {
    // Писатель Kafka автоматически обрабатывает пакетирование
    return nil
}

// StdoutLogWriter реализует LogWriter для stdout
type StdoutLogWriter struct {
    config LoggingConfig
}

// NewStdoutLogWriter создает новый писатель журнала stdout
func NewStdoutLogWriter(config LoggingConfig) *StdoutLogWriter {
    return &StdoutLogWriter{config: config}
}

// Write записывает запись журнала в stdout
func (s *StdoutLogWriter) Write(entry LogEntry) error {
    var output string
    
    switch s.config.Format {
    case LogFormatJSON:
        data, jsonErr := json.Marshal(entry)
        if jsonErr != nil {
            return fmt.Errorf("не удалось сериализовать запись журнала: %w", jsonErr)
        }
        output = string(data)
    case LogFormatText:
        output = s.formatTextEntry(entry)
    default:
        data, jsonErr := json.Marshal(entry)
        if jsonErr != nil {
            return fmt.Errorf("не удалось сериализовать запись журнала: %w", jsonErr)
        }
        output = string(data)
    }
    
    log.Println(output)
    return nil
}

// formatTextEntry форматирует запись журнала как обычный текст
func (s *StdoutLogWriter) formatTextEntry(entry LogEntry) string {
    status := "УСПЕХ"
    if !entry.Success {
        status = "ОШИБКА"
    }
    
    return fmt.Sprintf("[%s] %s %s %s %s %dмс - %s (ID: %s)",
        entry.Timestamp.Format(time.RFC3339),
        entry.Level,
        entry.Transport,
        entry.Method,
        status,
        entry.Duration,
        entry.Handler,
        entry.RequestID,
    )
}

// Close является пустой операцией для писателя stdout
func (s *StdoutLogWriter) Close() error {
    return nil
}

// Flush является пустой операцией для писателя stdout
func (s *StdoutLogWriter) Flush() error {
    return nil
}

// Logger обрабатывает операции логирования с асинхронной обработкой
type Logger struct {
    config         LoggingConfig
    writer         LogWriter
    asyncProcessor AsyncProcessor
    clock          types.Clock
    mu             sync.RWMutex
}

// NewLogger создает новый логгер с указанной конфигурацией
func NewLogger(config LoggingConfig) (*Logger, error) {
    return NewLoggerWithDependencies(config, NewDefaultAsyncProcessor(), types.GlobalClock)
}

// NewLoggerWithDependencies создает новый логгер с внедряемыми зависимостями
func NewLoggerWithDependencies(config LoggingConfig, asyncProcessor AsyncProcessor, clock types.Clock) (*Logger, error) {
    if !config.Enabled {
        return &Logger{
            config:         config,
            asyncProcessor: asyncProcessor,
            clock:          clock,
        }, nil
    }
    
    var writer LogWriter
    var err error
    
    switch config.Destination {
    case LogDestinationKafka:
        writer, err = NewKafkaLogWriter(config)
    case LogDestinationStdout:
        writer = NewStdoutLogWriter(config)
    default:
        return nil, fmt.Errorf("неподдерживаемое назначение журнала: %s", config.Destination)
    }
    
    if err != nil {
        return nil, fmt.Errorf("не удалось создать писатель журнала: %w", err)
    }
    
    return &Logger{
        config:         config,
        writer:         writer,
        asyncProcessor: asyncProcessor,
        clock:          clock,
    }, nil
}

// shouldLog определяет, должен ли запрос быть залогирован на основе конфигурации
func (l *Logger) shouldLog(req *types.JSONRPCRequest, success bool, hasError bool) bool {
    if !l.config.Enabled {
        return false
    }
    
    // Проверка фильтра только успешных
    if l.config.LogSuccessOnly && (!success || hasError) {
        return false
    }
    
    // Проверка включения/исключения метода
    if len(l.config.IncludeMethods) > 0 {
        included := false
        for _, method := range l.config.IncludeMethods {
            if method == req.Method {
                included = true
                break
            }
        }
        if !included {
            return false
        }
    }
    
    for _, method := range l.config.ExcludeMethods {
        if method == req.Method {
            return false
        }
    }
    
    return true
}

// createLogEntry создает структурированную запись журнала из данных запроса
func (l *Logger) createLogEntry(req *types.JSONRPCRequest, ctx *types.RequestContext, response *types.JSONRPCResponse, err error) LogEntry {
    now := l.clock.Now()
    
    entry := LogEntry{
        RequestID:      ctx.RequestID,
        Method:         req.Method,
        Transport:      ctx.Transport,
        RemoteAddr:     ctx.RemoteAddr,
        UserAgent:      ctx.UserAgent,
        Timestamp:      now,
        Duration:       ctx.Duration().Milliseconds(),
        StartTime:      ctx.StartTime,
        Handler:        ctx.SelectedHandler,
        ServiceName:    l.config.ServiceName,
        ServiceVersion: l.config.ServiceVersion,
        Level:          LogLevelInfo,
        RequestData:    make(map[string]interface{}),
        Headers:        make(map[string]string),
        ExtraFields:    make(map[string]string),
    }
    
    // Определение успеха и информации об ошибке
    entry.Success = err == nil && (response == nil || response.Error == nil)
    
    if err != nil {
        entry.Level = LogLevelError
        errMsg := err.Error()
        entry.ErrorMsg = &errMsg
    } else if response != nil && response.Error != nil {
        entry.Level = LogLevelWarn
        entry.Success = false
        entry.ErrorCode = &response.Error.Code
        entry.ErrorMsg = &response.Error.Message
    }
    
    // Копирование заголовков (ограничение для предотвращения больших нагрузок)
    headerCount := 0
    for key, value := range ctx.Headers {
        if headerCount >= 10 { // Ограничение заголовков для предотвращения больших записей журнала
            break
        }
        entry.Headers[key] = value
        headerCount++
    }
    
    // Копирование данных запроса (ограничение для предотвращения больших нагрузок)
    dataCount := 0
    for key, value := range ctx.Data {
        if dataCount >= 10 { // Ограничение полей данных
            break
        }
        entry.RequestData[key] = value
        dataCount++
    }
    
    // Копирование дополнительных полей
    for key, value := range l.config.ExtraFields {
        entry.ExtraFields[key] = value
    }
    
    return entry
}

// logEntry записывает запись журнала с использованием настроенного писателя
func (l *Logger) logEntry(entry LogEntry) {
    if l.writer == nil {
        return
    }
    
    if err := l.writer.Write(entry); err != nil {
        log.Printf("Не удалось записать запись журнала: %v", err)
        
        // Запасной вариант для stdout, если основной писатель не работает
        if l.config.Destination != LogDestinationStdout {
            fallbackWriter := NewStdoutLogWriter(l.config)
            if fallbackErr := fallbackWriter.Write(entry); fallbackErr != nil {
                log.Printf("Запасное логирование также не удалось: %v", fallbackErr)
            }
        }
    }
}

// Close закрывает логгер и его писатель
func (l *Logger) Close() error {
    l.mu.Lock()
    defer l.mu.Unlock()
    
    // Сначала завершаем работу асинхронного процессора
    if l.asyncProcessor != nil {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        l.asyncProcessor.Shutdown(ctx)
    }
    
    if l.writer != nil {
        return l.writer.Close()
    }
    return nil
}

// Flush сбрасывает все ожидающие записи журнала
func (l *Logger) Flush() error {
    l.mu.RLock()
    defer l.mu.RUnlock()
    
    if l.writer != nil {
        return l.writer.Flush()
    }
    return nil
}

// LoggingMiddleware создает промежуточный слой логирования с указанной конфигурацией
func LoggingMiddleware(logger *Logger) types.Middleware {
    return func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
        // Выполнить следующий обработчик
        response, err := next(req, ctx)
        
        // Определить, нужно ли логировать этот запрос
        success := err == nil && (response == nil || response.Error == nil)
        hasError := err != nil || (response != nil && response.Error != nil)
        
        if logger.shouldLog(req, success, hasError) {
            // Создать и залогировать запись асинхронно, чтобы избежать блокировки обработки запроса
            if logger.asyncProcessor != nil {
                logger.asyncProcessor.Process(context.Background(), func() {
                    defer func() {
                        if r := recover(); r != nil {
                            log.Printf("Паника в промежуточном слое логирования: %v", r)
                        }
                    }()
                    
                    entry := logger.createLogEntry(req, ctx, response, err)
                    logger.logEntry(entry)
                })
            } else {
                // Запасной вариант для синхронного логирования
                entry := logger.createLogEntry(req, ctx, response, err)
                logger.logEntry(entry)
            }
        }
        
        return response, err
    }
}

// NewKafkaLogger создает новый логгер Kafka (обратная совместимость)
// Устарело: Используйте NewLogger с LogDestinationKafka вместо этого
func NewKafkaLogger(config LoggingConfig) *Logger {
    config.Destination = LogDestinationKafka
    logger, err := NewLogger(config)
    if err != nil {
        log.Printf("Не удалось создать логгер Kafka: %v", err)
        return &Logger{config: config}
    }
    return logger
}

// TracingMiddleware добавляет поддержку распределенной трассировки
func TracingMiddleware() types.Middleware {
    return func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
        // Здесь была бы реализована логика трассировки
        // Пока просто передаем дальше
        return next(req, ctx)
    }
}

// AuthenticationMiddleware предоставляет базовый промежуточный слой аутентификации
func AuthenticationMiddleware() types.Middleware {
    return func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
        // Здесь была бы реализована логика аутентификации
        // Для демонстрации просто передаем дальше
        return next(req, ctx)
    }
}

// HandlerSelectionMiddleware облегчает выбор обработчика через контекст
func HandlerSelectionMiddleware(handlerMap map[string]string) types.Middleware {
    return func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
        // Установить выбранный обработчик в контексте на основе метода
        if handler, exists := handlerMap[req.Method]; exists {
            ctx.SelectedHandler = handler
        } else {
            ctx.SelectedHandler = "unknown"
        }
        
        return next(req, ctx)
    }
}
