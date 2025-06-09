package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/gorilla/websocket"
)

// JSONRPCRequest представляет JSON-RPC запрос
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      interface{} `json:"id,omitempty"`
}

// JSONRPCResponse представляет JSON-RPC ответ
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// JSONRPCError представляет ошибку JSON-RPC
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ClientConfig содержит конфигурацию клиента
type ClientConfig struct {
	Protocol string
	Host     string
	Port     int
	TLS      bool
	Timeout  time.Duration
	Debug    bool
}

// Client представляет JSON-RPC клиент
type Client struct {
	config ClientConfig
	client *http.Client
}

// HistoryManager управляет историей команд
type HistoryManager struct {
	historyFile string
	commands    []string
	maxSize     int
}

// NewHistoryManager создает новый менеджер истории
func NewHistoryManager() *HistoryManager {
	homeDir, _ := os.UserHomeDir()
	historyFile := filepath.Join(homeDir, ".jsonrpc_client_history")
	
	hm := &HistoryManager{
		historyFile: historyFile,
		commands:    make([]string, 0),
		maxSize:     1000, // Максимум 1000 команд в истории
	}
	
	hm.loadHistory()
	return hm
}

// loadHistory загружает историю из файла
func (hm *HistoryManager) loadHistory() {
	file, err := os.Open(hm.historyFile)
	if err != nil {
		return // Файл не существует, это нормально
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			hm.commands = append(hm.commands, line)
		}
	}
}

// saveHistory сохраняет историю в файл
func (hm *HistoryManager) saveHistory() error {
	file, err := os.Create(hm.historyFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Сохраняем только последние maxSize команд
	start := 0
	if len(hm.commands) > hm.maxSize {
		start = len(hm.commands) - hm.maxSize
	}

	for i := start; i < len(hm.commands); i++ {
		if _, err := fmt.Fprintln(file, hm.commands[i]); err != nil {
			return err
		}
	}

	return nil
}

// addCommand добавляет команду в историю
func (hm *HistoryManager) addCommand(command string) {
	command = strings.TrimSpace(command)
	if command == "" {
		return
	}

	// Избегаем дублирования последней команды
	if len(hm.commands) > 0 && hm.commands[len(hm.commands)-1] == command {
		return
	}

	hm.commands = append(hm.commands, command)
	
	// Ограничиваем размер истории
	if len(hm.commands) > hm.maxSize {
		hm.commands = hm.commands[1:]
	}
}

// getCommands возвращает все команды для автодополнения
func (hm *HistoryManager) getCommands() []string {
	return hm.commands
}

// CommandCompleter предоставляет автодополнение команд
type CommandCompleter struct {
	commands []string
}

// NewCommandCompleter создает новый автодополнитель команд
func NewCommandCompleter() *CommandCompleter {
	return &CommandCompleter{
		commands: []string{
			"echo", "calc", "calculate", "status", "time", "notify", "raw", 
			"debug", "help", "quit", "exit", "history", "clear",
		},
	}
}

// Do выполняет автодополнение
func (cc *CommandCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	lineStr := string(line)
	fields := strings.Fields(lineStr)
	
	if len(fields) == 0 || (len(fields) == 1 && pos == len(line)) {
		// Автодополнение команд
		prefix := ""
		if len(fields) == 1 {
			prefix = fields[0]
		}
		
		var suggestions [][]rune
		for _, cmd := range cc.commands {
			if strings.HasPrefix(cmd, prefix) {
				suggestions = append(suggestions, []rune(cmd[len(prefix):]))
			}
		}
		return suggestions, len(prefix)
	}
	
	return nil, 0
}

// NewClient создает новый клиент
func NewClient(config ClientConfig) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Только для тестирования
		},
	}

	return &Client{
		config: config,
		client: &http.Client{
			Transport: transport,
			Timeout:   config.Timeout,
		},
	}
}

// makeRequest создает JSON-RPC запрос
func makeRequest(method string, params interface{}, id interface{}) *JSONRPCRequest {
	return &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}
}

// sendHTTPRequest отправляет HTTP запрос
func (c *Client) sendHTTPRequest(req *JSONRPCRequest) (*JSONRPCResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if c.config.Debug {
		fmt.Printf("🔍 DEBUG Request: %s\n", string(data))
	}

	scheme := "http"
	if c.config.TLS {
		scheme = "https"
	}

	url := fmt.Sprintf("%s://%s:%d/rpc", scheme, c.config.Host, c.config.Port)
	
	resp, err := c.client.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if c.config.Debug {
		fmt.Printf("🔍 DEBUG Response: %s\n", string(body))
	}

	// Для уведомлений (без ID) ответ может быть пустым
	if len(body) == 0 {
		return nil, nil
	}

	var response JSONRPCResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &response, nil
}

// sendWebSocketRequest отправляет WebSocket запрос
func (c *Client) sendWebSocketRequest(req *JSONRPCRequest) (*JSONRPCResponse, error) {
	scheme := "ws"
	if c.config.TLS {
		scheme = "wss"
	}

	u := url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", c.config.Host, c.config.Port),
		Path:   "/ws",
	}

	if c.config.Debug {
		fmt.Printf("🔍 DEBUG WebSocket URL: %s\n", u.String())
	}

	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	data, err := json.Marshal(req)
	if c.config.Debug {
		fmt.Printf("🔍 DEBUG WebSocket Request: %s\n", string(data))
	}

	if err := conn.WriteJSON(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Для уведомлений не ожидаем ответа
	if req.ID == nil {
		return nil, nil
	}

	var response JSONRPCResponse
	if err := conn.ReadJSON(&response); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if c.config.Debug {
		respData, _ := json.Marshal(response)
		fmt.Printf("🔍 DEBUG WebSocket Response: %s\n", string(respData))
	}

	return &response, nil
}

// sendTCPRequest отправляет TCP запрос
func (c *Client) sendTCPRequest(req *JSONRPCRequest) (*JSONRPCResponse, error) {
	address := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
	
	if c.config.Debug {
		fmt.Printf("🔍 DEBUG TCP Address: %s\n", address)
	}

	var conn net.Conn
	var err error

	if c.config.TLS {
		conn, err = tls.Dial("tcp", address, &tls.Config{
			InsecureSkipVerify: true,
		})
	} else {
		conn, err = net.Dial("tcp", address)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if c.config.Debug {
		fmt.Printf("🔍 DEBUG TCP Request: %s\n", string(data))
	}

	// Отправляем запрос с переводом строки
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Для уведомлений не ожидаем ответа
	if req.ID == nil {
		return nil, nil
	}

	// Читаем ответ
	reader := bufio.NewReader(conn)
	line, _, err := reader.ReadLine()
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if c.config.Debug {
		fmt.Printf("🔍 DEBUG TCP Response: %s\n", string(line))
	}

	var response JSONRPCResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &response, nil
}

// SendRequest отправляет запрос в зависимости от протокола
func (c *Client) SendRequest(req *JSONRPCRequest) (*JSONRPCResponse, error) {
	switch strings.ToLower(c.config.Protocol) {
	case "http", "https":
		return c.sendHTTPRequest(req)
	case "ws", "wss", "websocket":
		return c.sendWebSocketRequest(req)
	case "tcp", "tls":
		return c.sendTCPRequest(req)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", c.config.Protocol)
	}
}

// printResponse выводит ответ в удобном формате
func printResponse(response *JSONRPCResponse, err error) {
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		return
	}

	if response == nil {
		fmt.Printf("✅ Notification sent successfully (no response expected)\n")
		return
	}

	if response.Error != nil {
		fmt.Printf("❌ JSON-RPC Error [%d]: %s\n", response.Error.Code, response.Error.Message)
		if response.Error.Data != nil {
			fmt.Printf("   Data: %v\n", response.Error.Data)
		}
		return
	}

	fmt.Printf("✅ Success (ID: %v)\n", response.ID)
	if response.Result != nil {
		resultJSON, _ := json.MarshalIndent(response.Result, "   ", "  ")
		fmt.Printf("   Result: %s\n", string(resultJSON))
	}
}

// showHistory показывает историю команд
func showHistory(history *HistoryManager) {
	commands := history.getCommands()
	if len(commands) == 0 {
		fmt.Println("📜 History is empty")
		return
	}

	fmt.Printf("📜 Command History (last %d commands):\n", len(commands))
	start := 0
	if len(commands) > 20 {
		start = len(commands) - 20
		fmt.Printf("   ... (showing last 20 of %d commands)\n", len(commands))
	}

	for i := start; i < len(commands); i++ {
		fmt.Printf("   %3d: %s\n", i+1, commands[i])
	}
}

// processCommand обрабатывает команду и возвращает JSON-RPC запрос
func processCommand(line string, requestID *int) (*JSONRPCRequest, bool, string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, false, ""
	}

	parts := strings.Fields(line)
	command := strings.ToLower(parts[0])

	switch command {
	case "quit", "exit", "q":
		return nil, false, "quit"

	case "help", "h":
		return nil, false, "help"

	case "history":
		return nil, false, "history"

	case "clear":
		return nil, false, "clear"

	case "echo":
		if len(parts) < 2 {
			fmt.Println("Usage: echo <message>")
			return nil, false, ""
		}
		message := strings.Join(parts[1:], " ")
		
		// Убираем кавычки если они есть
		if strings.HasPrefix(message, "\"") && strings.HasSuffix(message, "\"") {
			message = strings.Trim(message, "\"")
		}
		
		req := makeRequest("echo", map[string]interface{}{
			"message":   message,
			"timestamp": time.Now().Unix(),
		}, *requestID)
		*requestID++
		return req, true, ""

	case "calc", "calculate":
		if len(parts) != 4 {
			fmt.Println("Usage: calc <a> <op> <b>")
			fmt.Println("Example: calc 10 + 5")
			return nil, false, ""
		}

		a, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			fmt.Printf("Invalid number: %s\n", parts[1])
			return nil, false, ""
		}

		b, err := strconv.ParseFloat(parts[3], 64)
		if err != nil {
			fmt.Printf("Invalid number: %s\n", parts[3])
			return nil, false, ""
		}

		req := makeRequest("calculate", map[string]interface{}{
			"a":         a,
			"b":         b,
			"operation": parts[2],
		}, *requestID)
		*requestID++
		return req, true, ""

	case "status":
		req := makeRequest("status", nil, *requestID)
		*requestID++
		return req, true, ""

	case "time":
		req := makeRequest("time", nil, *requestID)
		*requestID++
		return req, true, ""

	case "notify":
		if len(parts) < 2 {
			fmt.Println("Usage: notify <method> [params]")
			return nil, false, ""
		}

		var params interface{}
		if len(parts) > 2 {
			paramsStr := strings.Join(parts[2:], " ")
			if err := json.Unmarshal([]byte(paramsStr), &params); err != nil {
				// Если не JSON, используем как строку
				params = paramsStr
			}
		}

		req := makeRequest(parts[1], params, nil) // nil ID для уведомления
		return req, true, ""

	case "raw":
		if len(parts) < 2 {
			fmt.Println("Usage: raw <json>")
			return nil, false, ""
		}

		jsonStr := strings.Join(parts[1:], " ")
		req := &JSONRPCRequest{}
		if err := json.Unmarshal([]byte(jsonStr), req); err != nil {
			fmt.Printf("Invalid JSON: %v\n", err)
			return nil, false, ""
		}

		if req.ID != nil {
			*requestID++
		}
		return req, true, ""

	default:
		fmt.Printf("Unknown command: %s. Type 'help' for available commands.\n", command)
		return nil, false, ""
	}
}

// runInteractiveMode запускает интерактивный режим с расширенными возможностями
func runInteractiveMode(client *Client) {
	fmt.Println("🚀 Enhanced Interactive JSON-RPC Client")
	fmt.Println("Features:")
	fmt.Println("  • Command history navigation (↑/↓ arrows)")
	fmt.Println("  • Line editing (←/→ arrows, backspace, delete)")
	fmt.Println("  • Tab completion for commands")
	fmt.Println("  • Persistent history across sessions")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  echo <message>           - Echo message")
	fmt.Println("  calc <a> <op> <b>        - Calculate (op: +, -, *, /)")
	fmt.Println("  status                   - Get server status")
	fmt.Println("  time                     - Get server time")
	fmt.Println("  notify <method> [params] - Send notification")
	fmt.Println("  raw <json>               - Send raw JSON-RPC request")
	fmt.Println("  history                  - Show command history")
	fmt.Println("  clear                    - Clear screen")
	fmt.Println("  help                     - Show this help")
	fmt.Println("  quit                     - Exit")
	fmt.Println()
	fmt.Println("Navigation:")
	fmt.Println("  ↑/↓ arrows              - Browse command history")
	fmt.Println("  ←/→ arrows              - Move cursor in line")
	fmt.Println("  Tab                     - Auto-complete commands")
	fmt.Println("  Ctrl+C                  - Exit")
	fmt.Println()

	// Инициализируем менеджер истории
	history := NewHistoryManager()
	defer func() {
		if err := history.saveHistory(); err != nil {
			fmt.Printf("Warning: Failed to save history: %v\n", err)
		}
	}()

	// Настраиваем readline
	completer := NewCommandCompleter()
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "jsonrpc> ",
		HistoryFile:     history.historyFile,
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		HistorySearchFold: true,
	})
	if err != nil {
		fmt.Printf("Failed to initialize readline: %v\n", err)
		return
	}
	defer rl.Close()

	requestID := 1

	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				if len(line) == 0 {
					fmt.Println("\n👋 Goodbye! (Use 'quit' or Ctrl+D to exit)")
					break
				} else {
					continue
				}
			} else if err == io.EOF {
				fmt.Println("\n👋 Goodbye!")
				break
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Добавляем команду в историю
		history.addCommand(line)

		// Обрабатываем специальные команды
		req, shouldSend, action := processCommand(line, &requestID)

		switch action {
		case "quit":
			fmt.Println("👋 Goodbye!")
			return

		case "help":
			fmt.Println("Available commands:")
			fmt.Println("  echo <message>           - Echo message")
			fmt.Println("  calc <a> <op> <b>        - Calculate (op: +, -, *, /)")
			fmt.Println("  status                   - Get server status")
			fmt.Println("  time                     - Get server time")
			fmt.Println("  notify <method> [params] - Send notification")
			fmt.Println("  raw <json>               - Send raw JSON-RPC request")
			fmt.Println("  history                  - Show command history")
			fmt.Println("  clear                    - Clear screen")
			fmt.Println("  help                     - Show this help")
			fmt.Println("  quit                     - Exit")
			continue

		case "history":
			showHistory(history)
			continue

		case "clear":
			fmt.Print("\033[2J\033[H") // ANSI escape codes для очистки экрана
			continue
		}

		// Отправляем запрос если нужно
		if shouldSend && req != nil {
			fmt.Printf("📤 Sending: %s\n", req.Method)
			
			// Переключаем режим отладки если команда содержит debug
			if strings.Contains(line, "debug") {
				client.config.Debug = !client.config.Debug
				fmt.Printf("🔍 Debug mode: %v\n", client.config.Debug)
				continue
			}

			response, err := client.SendRequest(req)
			printResponse(response, err)
			fmt.Println()
		}
	}
}

// runBenchmark запускает бенчмарк
func runBenchmark(client *Client, requests int, concurrent int) {
	fmt.Printf("🏃 Running benchmark: %d requests with %d concurrent workers\n", requests, concurrent)
	
	start := time.Now()
	
	// Канал для задач
	jobs := make(chan int, requests)
	results := make(chan error, requests)
	
	// Запускаем воркеры
	for w := 0; w < concurrent; w++ {
		go func() {
			for range jobs {
				req := makeRequest("status", nil, time.Now().UnixNano())
				_, err := client.SendRequest(req)
				results <- err
			}
		}()
	}
	
	// Отправляем задачи
	for i := 0; i < requests; i++ {
		jobs <- i
	}
	close(jobs)
	
	// Собираем результаты
	var errors int
	for i := 0; i < requests; i++ {
		if err := <-results; err != nil {
			errors++
		}
	}
	
	duration := time.Since(start)
	rps := float64(requests) / duration.Seconds()
	
	fmt.Printf("📊 Benchmark Results:\n")
	fmt.Printf("   Total requests: %d\n", requests)
	fmt.Printf("   Successful: %d\n", requests-errors)
	fmt.Printf("   Errors: %d\n", errors)
	fmt.Printf("   Duration: %v\n", duration)
	fmt.Printf("   Requests/sec: %.2f\n", rps)
}

// isFlagSet проверяет, был ли флаг явно установлен
func isFlagSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func main() {
	var (
		protocol   = flag.String("protocol", "http", "Protocol to use (http, https, ws, wss, tcp, tls)")
		host       = flag.String("host", "localhost", "Server host")
		port       = flag.Int("port", 8080, "Server port")
		useTLS     = flag.Bool("tls", false, "Use TLS/SSL")
		timeout    = flag.Duration("timeout", 30*time.Second, "Request timeout")
		method     = flag.String("method", "", "Method to call")
		params     = flag.String("params", "", "Parameters (JSON)")
		id         = flag.String("id", "", "Request ID (empty for notification)")
		interactive = flag.Bool("interactive", true, "Run in interactive mode (default)")
		benchmark  = flag.Bool("benchmark", false, "Run benchmark")
		requests   = flag.Int("requests", 1000, "Number of requests for benchmark")
		concurrent = flag.Int("concurrent", 10, "Number of concurrent workers for benchmark")
		debug      = flag.Bool("debug", false, "Enable debug mode")
	)
	flag.Parse()

	// Определяем порт по умолчанию для протокола
	if *port == 8080 {
		switch strings.ToLower(*protocol) {
		case "https":
			*port = 8443
		case "ws", "websocket":
			*port = 8082
		case "wss":
			*port = 8445
		case "tcp":
			*port = 8081
		case "tls":
			*port = 8444
		}
	}

	config := ClientConfig{
		Protocol: *protocol,
		Host:     *host,
		Port:     *port,
		TLS:      *useTLS,
		Timeout:  *timeout,
		Debug:    *debug,
	}

	client := NewClient(config)

	fmt.Printf("🔗 Connecting to %s://%s:%d\n", *protocol, *host, *port)

	if *benchmark {
		runBenchmark(client, *requests, *concurrent)
		return
	}

	// Если не указан метод и не отключен интерактивный режим, запускаем интерактивный режим
	if *method == "" && *interactive {
		runInteractiveMode(client)
		return
	}

	if *method == "" {
		fmt.Println("❌ Method is required. Use -method flag or -interactive mode.")
		fmt.Println("\nExamples:")
		fmt.Println("  # Interactive mode (default)")
		fmt.Println("  go run cmd/client/main.go")
		fmt.Println("")
		fmt.Println("  # Simple status check")
		fmt.Println("  go run cmd/client/main.go -method status -interactive=false")
		fmt.Println("")
		fmt.Println("  # Echo with parameters")
		fmt.Println("  go run cmd/client/main.go -method echo -params '{\"message\":\"Hello\"}' -interactive=false")
		fmt.Println("")
		fmt.Println("  # Calculate")
		fmt.Println("  go run cmd/client/main.go -method calculate -params '{\"a\":10,\"b\":5,\"operation\":\"+\"}' -interactive=false")
		fmt.Println("")
		fmt.Println("  # Send notification (no response)")
		fmt.Println("  go run cmd/client/main.go -method echo -params '{\"message\":\"Hello\"}' -id \"\" -interactive=false")
		fmt.Println("")
		fmt.Println("  # Benchmark")
		fmt.Println("  go run cmd/client/main.go -benchmark -requests 1000 -concurrent 10")
		fmt.Println("")
		fmt.Println("  # Different protocols")
		fmt.Println("  go run cmd/client/main.go -protocol ws -method status -interactive=false")
		fmt.Println("  go run cmd/client/main.go -protocol tcp -method status -interactive=false")
		os.Exit(1)
	}

	// Парсим параметры
	var parsedParams interface{}
	if *params != "" {
		if err := json.Unmarshal([]byte(*params), &parsedParams); err != nil {
			fmt.Printf("❌ Invalid JSON parameters: %v\n", err)
			os.Exit(1)
		}
	}

	// Определяем ID запроса
	var requestID interface{}
	if *id != "" {
		requestID = *id
	} else if *id == "" && isFlagSet("id") {
		// Если -id="" явно указан, это уведомление
		requestID = nil
	} else {
		// По умолчанию используем ID
		requestID = 1
	}

	req := makeRequest(*method, parsedParams, requestID)

	fmt.Printf("📤 Sending %s request...\n", *method)
	response, err := client.SendRequest(req)
	printResponse(response, err)
}
