package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"streaming-server/pkg/dispatcher"
	"streaming-server/pkg/handlers"
	"streaming-server/pkg/middleware"
	"streaming-server/pkg/types"
)

// Server представляет JSON-RPC сервер
type Server struct {
	config     Config
	dispatcher *dispatcher.Dispatcher
	processor  *JSONRPCProcessor
	logger     *middleware.Logger
	httpServer *http.Server
	upgrader   websocket.Upgrader
	// Другие поля...
}

// Config содержит конфигурацию сервера
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

// ProcessingContext содержит контекст обработки запроса
type ProcessingContext struct {
	Transport      string
	RemoteAddr     string
	HTTPRequest    *http.Request
	ServiceName    string
	ServiceVersion string
	Headers        http.Header
	UserAgent      string
}

// NewServer создает новый экземпляр сервера
func NewServer(config Config, logger *middleware.Logger) *Server {
	dispatcher := dispatcher.NewDispatcher()

	// Set up middleware chain
	chain := middleware.NewChain(
		middleware.LoggingMiddleware(logger),
	)
	dispatcher.SetMiddleware(chain)

	// Register default handlers
	registerDefaultHandlers(dispatcher)

	processor := NewJSONRPCProcessor(dispatcher, logger)

	return &Server{
		config:     config,
		dispatcher: dispatcher,
		processor:  processor,
		logger:     logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for testing
			},
		},
	}
}

// registerDefaultHandlers registers the default JSON-RPC handlers
func registerDefaultHandlers(d *dispatcher.Dispatcher) {
	d.RegisterHandler("echo", handlers.EchoHandler)
	d.RegisterHandler("calculate", handlers.CalculateHandler)
	d.RegisterHandler("status", handlers.StatusHandler)
	d.RegisterHandler("time", handlers.TimeHandler)
	d.RegisterHandler("test_slow", handlers.TestSlowHandler)

	// Test error handler for integration tests
	d.RegisterHandler("test_error", func(req *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
		return nil, fmt.Errorf("intentional test error")
	})
}

// RegisterHandler регистрирует обработчик для указанного метода
func (s *Server) RegisterHandler(method string, handler types.Handler) {
	s.dispatcher.RegisterHandler(method, handler)
}

// Start starts all configured server protocols
func (s *Server) Start() error {
	// Start HTTP server
	go func() {
		if err := s.startHTTP(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start HTTPS server
	go func() {
		if err := s.startHTTPS(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTPS server error: %v", err)
		}
	}()

	// Start WebSocket server
	go func() {
		if err := s.startWebSocket(); err != nil && err != http.ErrServerClosed {
			log.Printf("WebSocket server error: %v", err)
		}
	}()

	// Start Secure WebSocket server
	go func() {
		if err := s.startSecureWebSocket(); err != nil && err != http.ErrServerClosed {
			log.Printf("Secure WebSocket server error: %v", err)
		}
	}()

	// Start TCP server
	go func() {
		if err := s.startTCP(); err != nil {
			log.Printf("TCP server error: %v", err)
		}
	}()

	// Start TLS server
	go func() {
		if err := s.startTLS(); err != nil {
			log.Printf("TLS server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the server
func (s *Server) Stop() error {
	// Implementation for graceful shutdown would go here
	return nil
}

// GetDispatcher возвращает диспетчер сервера
func (s *Server) GetDispatcher() *dispatcher.Dispatcher {
	return s.dispatcher
}

// handleHTTPRequest обрабатывает HTTP запрос
func (s *Server) handleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	// Обработка CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Чтение тела запроса
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Проверка на пустое тело запроса
	if len(body) == 0 {
		// Для пустого тела возвращаем ошибку Invalid Request
		invalidRequestError := &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   types.NewInvalidRequestError("Request body cannot be empty"),
			ID:      nil,
		}

		responseJSON, _ := json.Marshal(invalidRequestError)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseJSON)
		return
	}

	// Создание контекста обработки
	ctx := ProcessingContext{
		Transport:      "HTTP",
		RemoteAddr:     r.RemoteAddr,
		HTTPRequest:    r,
		ServiceName:    s.config.ServiceName,
		ServiceVersion: s.config.Version,
		Headers:        r.Header,
		UserAgent:      r.UserAgent(),
	}

	// Обработка запроса
	var result interface{}

	// Определяем, является ли запрос пакетным
	if len(body) > 0 && body[0] == '[' {
		result = s.processor.ProcessBatchRequest(body, ctx)
	} else {
		result = s.processor.ProcessSingleRequest(body, ctx)
	}

	// Обработка результата с детальной диагностикой
	if result == nil {
		// Для уведомлений согласно JSON-RPC 2.0 не должно быть никакого ответа
		w.WriteHeader(http.StatusOK)
		return
	}

	// Дополнительная проверка для случаев, когда result не nil, но содержит nil значение
	switch v := result.(type) {
	case *types.JSONRPCResponse:
		if v == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
	case []*types.JSONRPCResponse:
		if len(v) == 0 {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// Сериализация ответа только для валидных результатов
	responseJSON, err := json.Marshal(result)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Отправка ответа
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(responseJSON)
}

// handleHealth обрабатывает запрос проверки здоровья
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   s.config.ServiceName,
		"version":   s.config.Version,
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(responseJSON)
}

// JSONRPCProcessor обрабатывает JSON-RPC запросы
type JSONRPCProcessor struct {
	dispatcher *dispatcher.Dispatcher
	logger     *middleware.Logger
}

// NewJSONRPCProcessor создает новый процессор JSON-RPC
func NewJSONRPCProcessor(dispatcher *dispatcher.Dispatcher, logger *middleware.Logger) *JSONRPCProcessor {
	return &JSONRPCProcessor{
		dispatcher: dispatcher,
		logger:     logger,
	}
}

// ProcessSingleRequest обрабатывает одиночный JSON-RPC запрос
func (p *JSONRPCProcessor) ProcessSingleRequest(data []byte, ctx ProcessingContext) *types.JSONRPCResponse {
	// Step 1: Parse JSON
	var request types.JSONRPCRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   types.NewParseError("Invalid JSON: " + err.Error()),
			ID:      nil, // ID is null when request cannot be parsed
		}
	}

	// Step 2: Validate JSON-RPC 2.0 structure
	if err := p.validateRequest(&request); err != nil {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   err,
			ID:      request.ID, // Use request ID even if request is invalid
		}
	}

	// Step 3: Handle notifications (requests without ID)
	if request.IsNotification() {
		// Process notification but don't return response
		p.processNotification(&request, ctx)
		return nil // No response for notifications per JSON-RPC 2.0 spec
	}

	// Step 4: Process regular request
	return p.processRegularRequest(&request, ctx)
}

// ProcessBatchRequest обрабатывает пакетный JSON-RPC запрос
func (p *JSONRPCProcessor) ProcessBatchRequest(data []byte, ctx ProcessingContext) interface{} {
	// Parse as array of raw messages
	var rawRequests []json.RawMessage
	if err := json.Unmarshal(data, &rawRequests); err != nil {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   types.NewParseError("Invalid JSON in batch request: " + err.Error()),
			ID:      nil,
		}
	}

	// Validate batch is not empty
	if len(rawRequests) == 0 {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   types.NewInvalidRequestError("Batch request cannot be empty"),
			ID:      nil,
		}
	}

	// Process each request in the batch
	var responses []*types.JSONRPCResponse
	for _, rawReq := range rawRequests {
		response := p.ProcessSingleRequest(rawReq, ctx)
		if response != nil { // Only add non-notification responses
			responses = append(responses, response)
		}
	}

	// If all requests were notifications, return nothing
	if len(responses) == 0 {
		return nil
	}

	// Return array of responses
	return responses
}

// validateRequest validates a JSON-RPC 2.0 request structure
func (p *JSONRPCProcessor) validateRequest(req *types.JSONRPCRequest) *types.RPCError {
	// Validate JSON-RPC version
	if req.JSONRPC != "2.0" {
		return types.NewInvalidRequestError("JSON-RPC version must be '2.0'")
	}

	// Validate method is present and non-empty
	if req.Method == "" {
		return types.NewInvalidRequestError("Method is required and cannot be empty")
	}

	// Validate method name format (should not start with "rpc." unless it's a reserved method)
	if strings.HasPrefix(req.Method, "rpc.") {
		return types.NewMethodNotFoundError(req.Method + " (reserved method prefix)")
	}

	return nil
}

// processNotification processes a notification request (no response expected)
func (p *JSONRPCProcessor) processNotification(req *types.JSONRPCRequest, ctx ProcessingContext) {
	// Create request context
	requestCtx := p.createRequestContext(req, ctx)

	// Process through dispatcher (ignore response and errors for notifications)
	if p.dispatcher != nil {
		_, _ = p.dispatcher.Dispatch(req, requestCtx)
	}
}

// processRegularRequest processes a regular request (response expected)
func (p *JSONRPCProcessor) processRegularRequest(req *types.JSONRPCRequest, ctx ProcessingContext) *types.JSONRPCResponse {
	// Create request context
	requestCtx := p.createRequestContext(req, ctx)

	// Process through dispatcher
	response, err := p.dispatcher.Dispatch(req, requestCtx)
	if err != nil {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   types.NewInternalError("Dispatcher error: " + err.Error()),
			ID:      req.ID,
		}
	}

	// Ensure response has correct JSON-RPC version and ID
	if response != nil {
		response.JSONRPC = "2.0"
		response.ID = req.ID
	}

	return response
}

// createRequestContext creates a request context from processing context
func (p *JSONRPCProcessor) createRequestContext(req *types.JSONRPCRequest, ctx ProcessingContext) *types.RequestContext {
	var requestCtx *types.RequestContext

	if ctx.HTTPRequest != nil {
		requestCtx = types.NewRequestContext(ctx.HTTPRequest.Context(), ctx.ServiceName, ctx.RemoteAddr)
	} else {
		requestCtx = types.NewRequestContext(context.Background(), ctx.ServiceName, ctx.RemoteAddr)
	}

	requestCtx.WithValue("transport", ctx.Transport)
	requestCtx.WithValue("service_version", ctx.ServiceVersion)
	requestCtx.WithValue("method", req.Method)

	if ctx.HTTPRequest != nil {
		requestCtx.WithValue("headers", ctx.HTTPRequest.Header)
		requestCtx.WithValue("user_agent", ctx.HTTPRequest.UserAgent())
		requestCtx.HTTPRequest = ctx.HTTPRequest
	}

	return requestCtx
}

// HTTP Server Implementation

// startHTTP starts the HTTP server
func (s *Server) startHTTP() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", s.handleHTTPRequest)
	mux.HandleFunc("/health", s.handleHealth)

	server := &http.Server{
		Addr:         s.config.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	log.Printf("Starting HTTP server on %s", s.config.HTTPAddr)
	return server.ListenAndServe()
}

// startHTTPS starts the HTTPS server
func (s *Server) startHTTPS() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", s.handleHTTPRequest)
	mux.HandleFunc("/health", s.handleHealth)

	server := &http.Server{
		Addr:         s.config.HTTPSAddr,
		Handler:      mux,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
		TLSConfig:    s.config.TLSConfig,
	}

	log.Printf("Starting HTTPS server on %s", s.config.HTTPSAddr)
	return server.ListenAndServeTLS("", "") // TLS config is already set
}

// WebSocket Server Implementation

// startWebSocket starts the WebSocket server
func (s *Server) startWebSocket() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)

	server := &http.Server{
		Addr:         s.config.WSAddr,
		Handler:      mux,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	log.Printf("Starting WebSocket server on %s", s.config.WSAddr)
	return server.ListenAndServe()
}

// startSecureWebSocket starts the secure WebSocket server
func (s *Server) startSecureWebSocket() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/wss", s.handleSecureWebSocket)

	server := &http.Server{
		Addr:         s.config.WSSAddr,
		Handler:      mux,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
		TLSConfig:    s.config.TLSConfig,
	}

	log.Printf("Starting Secure WebSocket server on %s", s.config.WSSAddr)
	return server.ListenAndServeTLS("", "")
}

// handleWebSocket handles WebSocket connections
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	s.handleWebSocketConnection(conn, r, "WebSocket")
}

// handleSecureWebSocket handles secure WebSocket connections
func (s *Server) handleSecureWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Secure WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	s.handleWebSocketConnection(conn, r, "Secure WebSocket")
}

// handleWebSocketConnection handles WebSocket message processing with JSON-RPC 2.0 compliance
func (s *Server) handleWebSocketConnection(conn *websocket.Conn, r *http.Request, transport string) {
	ctx := ProcessingContext{
		Transport:      transport,
		RemoteAddr:     r.RemoteAddr,
		HTTPRequest:    r,
		ServiceName:    s.config.ServiceName,
		ServiceVersion: s.config.Version,
	}

	for {
		// Read message
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Process JSON-RPC request
		var result interface{}
		trimmed := strings.TrimSpace(string(message))

		if strings.HasPrefix(trimmed, "[") {
			// Batch request
			result = s.processor.ProcessBatchRequest(message, ctx)
		} else {
			// Single request
			result = s.processor.ProcessSingleRequest(message, ctx)
		}

		// Send response (skip if notification)
		if result != nil {
			if err := conn.WriteJSON(result); err != nil {
				log.Printf("WebSocket write error: %v", err)
				break
			}
		}
	}
}

// TCP Server Implementation

// startTCP starts the TCP server
func (s *Server) startTCP() error {
	listener, err := net.Listen("tcp", s.config.TCPAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("Starting TCP server on %s", s.config.TCPAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("TCP accept error: %v", err)
			continue
		}

		go s.handleTCPConnection(conn, "TCP")
	}
}

// startTLS starts the TLS server
func (s *Server) startTLS() error {
	listener, err := tls.Listen("tcp", s.config.TLSAddr, s.config.TLSConfig)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("Starting TLS server on %s", s.config.TLSAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("TLS accept error: %v", err)
			continue
		}

		go s.handleTCPConnection(conn, "TLS")
	}
}

// handleTCPConnection handles TCP/TLS connections with JSON-RPC 2.0 compliance
func (s *Server) handleTCPConnection(conn net.Conn, transport string) {
	defer conn.Close()

	ctx := ProcessingContext{
		Transport:      transport,
		RemoteAddr:     conn.RemoteAddr().String(),
		HTTPRequest:    nil,
		ServiceName:    s.config.ServiceName,
		ServiceVersion: s.config.Version,
	}

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	for {
		// Read raw JSON message
		var rawMessage json.RawMessage
		if err := decoder.Decode(&rawMessage); err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("TCP decode error: %v", err)
			break
		}

		// Process JSON-RPC request
		var result interface{}
		trimmed := strings.TrimSpace(string(rawMessage))

		if strings.HasPrefix(trimmed, "[") {
			// Batch request
			result = s.processor.ProcessBatchRequest(rawMessage, ctx)
		} else {
			// Single request
			result = s.processor.ProcessSingleRequest(rawMessage, ctx)
		}

		// Send response (skip if notification)
		if result != nil {
			if err := encoder.Encode(result); err != nil {
				log.Printf("TCP encode error: %v", err)
				break
			}
		}
	}
}
