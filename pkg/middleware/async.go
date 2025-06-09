package middleware

import (
	"context"
	"sync"
	"time"
)

// AsyncProcessor интерфейс для обработки асинхронных операций
type AsyncProcessor interface {
	Process(ctx context.Context, fn func()) error
	ProcessWithTimeout(ctx context.Context, fn func(), timeout time.Duration) error
	Shutdown(ctx context.Context) error
}

// DefaultAsyncProcessor реализует AsyncProcessor с использованием горутин
type DefaultAsyncProcessor struct {
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// NewDefaultAsyncProcessor создает новый DefaultAsyncProcessor
func NewDefaultAsyncProcessor() *DefaultAsyncProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	return &DefaultAsyncProcessor{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Process выполняет функцию асинхронно
func (p *DefaultAsyncProcessor) Process(ctx context.Context, fn func()) error {
	select {
	case <-p.ctx.Done():
		return p.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	default:
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					// Логируем панику, но не крашим программу
				}
			}()
			fn()
		}()
		return nil
	}
}

// ProcessWithTimeout выполняет функцию асинхронно с таймаутом
func (p *DefaultAsyncProcessor) ProcessWithTimeout(ctx context.Context, fn func(), timeout time.Duration) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan struct{})
	var fnErr error

	err := p.Process(timeoutCtx, func() {
		defer close(done)
		fn()
	})

	if err != nil {
		return err
	}

	select {
	case <-done:
		return fnErr
	case <-timeoutCtx.Done():
		return timeoutCtx.Err()
	}
}

// Shutdown корректно завершает работу процессора
func (p *DefaultAsyncProcessor) Shutdown(ctx context.Context) error {
	p.cancel()

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// MockAsyncProcessor реализует AsyncProcessor для тестирования
type MockAsyncProcessor struct {
	processedFunctions []func()
	processErrors      []error
	shutdownError      error
	mu                 sync.Mutex
}

// NewMockAsyncProcessor создает новый MockAsyncProcessor
func NewMockAsyncProcessor() *MockAsyncProcessor {
	return &MockAsyncProcessor{
		processedFunctions: make([]func(), 0),
		processErrors:      make([]error, 0),
	}
}

// Process записывает функцию для тестирования и опционально выполняет ее
func (m *MockAsyncProcessor) Process(ctx context.Context, fn func()) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.processedFunctions = append(m.processedFunctions, fn)

	if len(m.processErrors) > 0 {
		err := m.processErrors[0]
		m.processErrors = m.processErrors[1:]
		return err
	}

	return nil
}

// ProcessWithTimeout записывает функцию для тестирования
func (m *MockAsyncProcessor) ProcessWithTimeout(ctx context.Context, fn func(), timeout time.Duration) error {
	return m.Process(ctx, fn)
}

// Shutdown возвращает настроенную ошибку завершения
func (m *MockAsyncProcessor) Shutdown(ctx context.Context) error {
	return m.shutdownError
}

// ExecuteProcessedFunctions выполняет все записанные функции синхронно
func (m *MockAsyncProcessor) ExecuteProcessedFunctions() {
	m.mu.Lock()
	functions := make([]func(), len(m.processedFunctions))
	copy(functions, m.processedFunctions)
	m.mu.Unlock()

	for _, fn := range functions {
		if fn != nil {
			fn()
		}
	}
}

// GetProcessedFunctionCount возвращает количество обработанных функций
func (m *MockAsyncProcessor) GetProcessedFunctionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.processedFunctions)
}

// SetProcessErrors устанавливает ошибки, которые будут возвращены вызовами Process
func (m *MockAsyncProcessor) SetProcessErrors(errors ...error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processErrors = errors
}

// SetShutdownError устанавливает ошибку, которая будет возвращена методом Shutdown
func (m *MockAsyncProcessor) SetShutdownError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownError = err
}

// Reset очищает все записанное состояние
func (m *MockAsyncProcessor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processedFunctions = nil
	m.processErrors = nil
	m.shutdownError = nil
}
