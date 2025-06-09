package dispatcher

import (
	"errors"
	"fmt"
	"sync"
	
	"streaming-server/pkg/middleware"
	"streaming-server/pkg/types"
)

// Dispatcher обрабатывает JSON-RPC запросы и направляет их к соответствующим обработчикам
type Dispatcher struct {
	handlers       map[string]types.Handler
	middlewareChain *middleware.Chain
	mu             sync.RWMutex
}

// NewDispatcher создает новый экземпляр диспетчера
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		handlers:       make(map[string]types.Handler),
		middlewareChain: middleware.NewChain(),
	}
}

// RegisterHandler регистрирует обработчик для указанного метода
func (d *Dispatcher) RegisterHandler(method string, handler types.Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[method] = handler
}

// UnregisterHandler удаляет обработчик для указанного метода
func (d *Dispatcher) UnregisterHandler(method string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.handlers, method)
}

// SetMiddleware устанавливает middleware chain для диспетчера
func (d *Dispatcher) SetMiddleware(chain *middleware.Chain) {
	d.middlewareChain = chain
}

// Dispatch обрабатывает JSON-RPC запрос и возвращает ответ
func (d *Dispatcher) Dispatch(request *types.JSONRPCRequest, ctx *types.RequestContext) (*types.JSONRPCResponse, error) {
	// Проверяем, что запрос не nil
	if request == nil {
		return nil, errors.New("request cannot be nil")
	}
	
	// Проверяем, что контекст не nil
	if ctx == nil {
		return nil, errors.New("context cannot be nil")
	}

	// Получаем обработчик для метода
	d.mu.RLock()
	handler, exists := d.handlers[request.Method]
	d.mu.RUnlock()

	if !exists {
		return &types.JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   types.NewMethodNotFoundError(fmt.Sprintf("Method not found: %s", request.Method)),
			ID:      request.ID,
		}, nil
	}

	// Используем middleware chain для обработки запроса
	return d.middlewareChain.Execute(request, ctx, handler)
}

// GetRegisteredMethods возвращает список зарегистрированных методов
func (d *Dispatcher) GetRegisteredMethods() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	methods := make([]string, 0, len(d.handlers))
	for method := range d.handlers {
		methods = append(methods, method)
	}
	
	return methods
}

// HandlerCount возвращает количество зарегистрированных обработчиков
func (d *Dispatcher) HandlerCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.handlers)
}
