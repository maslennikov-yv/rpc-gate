package integration

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	// Оптимизированный диапазон портов для тестирования
	// Используем узкий диапазон для быстрого выделения
	DefaultTestPortStart = 45000
	DefaultTestPortEnd   = 45100 // Всего 100 портов для быстрой проверки

	// Максимальное количество попыток выделения порта
	MaxAllocationAttempts = 20

	// Задержка между попытками выделения
	AllocationRetryDelay = 5 * time.Millisecond
)

// OptimizedPortAllocator - оптимизированный менеджер портов для тестов
type OptimizedPortAllocator struct {
	mu             sync.RWMutex
	usedPorts      map[int]bool
	startPort      int
	endPort        int
	nextPort       int // Указатель на следующий порт для проверки
	poolSize       int
	allocatedCount int
}

// NewOptimizedPortAllocator создает новый оптимизированный аллокатор портов
func NewOptimizedPortAllocator() *OptimizedPortAllocator {
	startPort := DefaultTestPortStart
	endPort := DefaultTestPortEnd

	// Проверяем переменные окружения для кастомизации диапазона
	if envStart := os.Getenv("TEST_PORT_START"); envStart != "" {
		if port, err := strconv.Atoi(envStart); err == nil && port > 1024 {
			startPort = port
		}
	}

	if envEnd := os.Getenv("TEST_PORT_END"); envEnd != "" {
		if port, err := strconv.Atoi(envEnd); err == nil && port > startPort {
			endPort = port
		}
	}

	poolSize := endPort - startPort

	return &OptimizedPortAllocator{
		usedPorts:      make(map[int]bool, poolSize),
		startPort:      startPort,
		endPort:        endPort,
		nextPort:       startPort,
		poolSize:       poolSize,
		allocatedCount: 0,
	}
}

// AllocatePort быстро находит и резервирует свободный порт
func (opa *OptimizedPortAllocator) AllocatePort() (int, error) {
	opa.mu.Lock()
	defer opa.mu.Unlock()

	// Проверяем, не исчерпан ли пул портов
	if opa.allocatedCount >= opa.poolSize {
		return 0, fmt.Errorf("port pool exhausted: %d/%d ports allocated", opa.allocatedCount, opa.poolSize)
	}

	// Быстрый поиск свободного порта, начиная с nextPort
	for attempts := 0; attempts < MaxAllocationAttempts; attempts++ {
		port := opa.getNextPortCandidate()

		// Проверяем, не используется ли порт нашим аллокатором
		if opa.usedPorts[port] {
			continue
		}

		// Быстрая проверка доступности порта
		if opa.isPortAvailableFast(port) {
			opa.usedPorts[port] = true
			opa.allocatedCount++
			return port, nil
		}

		// Небольшая задержка только при неудаче
		time.Sleep(AllocationRetryDelay)
	}

	return 0, fmt.Errorf("failed to allocate port after %d attempts in range %d-%d",
		MaxAllocationAttempts, opa.startPort, opa.endPort)
}

// getNextPortCandidate возвращает следующий кандидат для проверки
func (opa *OptimizedPortAllocator) getNextPortCandidate() int {
	port := opa.nextPort
	opa.nextPort++

	// Циклический переход к началу диапазона
	if opa.nextPort > opa.endPort {
		opa.nextPort = opa.startPort
	}

	return port
}

// isPortAvailableFast быстро проверяет доступность порта
func (opa *OptimizedPortAllocator) isPortAvailableFast(port int) bool {
	// Используем короткий таймаут для быстрой проверки
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}

	// Немедленно закрываем listener
	listener.Close()

	// Дополнительная проверка - пытаемся подключиться
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 50*time.Millisecond)
	if err != nil {
		// Если подключение не удалось, порт свободен
		return true
	}

	conn.Close()
	return false
}

// ReleasePort освобождает порт
func (opa *OptimizedPortAllocator) ReleasePort(port int) {
	opa.mu.Lock()
	defer opa.mu.Unlock()

	if opa.usedPorts[port] {
		delete(opa.usedPorts, port)
		opa.allocatedCount--
	}
}

// AllocatePorts выделяет несколько портов оптимизированным способом
func (opa *OptimizedPortAllocator) AllocatePorts(count int) ([]int, error) {
	if count <= 0 {
		return nil, fmt.Errorf("invalid port count: %d", count)
	}

	if count > opa.poolSize {
		return nil, fmt.Errorf("requested %d ports, but pool size is only %d", count, opa.poolSize)
	}

	opa.mu.RLock()
	available := opa.poolSize - opa.allocatedCount
	opa.mu.RUnlock()

	if count > available {
		return nil, fmt.Errorf("requested %d ports, but only %d available", count, available)
	}

	ports := make([]int, 0, count)

	for i := 0; i < count; i++ {
		port, err := opa.AllocatePort()
		if err != nil {
			// Освобождаем уже выделенные порты в случае ошибки
			for _, p := range ports {
				opa.ReleasePort(p)
			}
			return nil, fmt.Errorf("failed to allocate port %d/%d: %w", i+1, count, err)
		}
		ports = append(ports, port)
	}

	return ports, nil
}

// GetStats возвращает статистику использования портов
func (opa *OptimizedPortAllocator) GetStats() map[string]interface{} {
	opa.mu.RLock()
	defer opa.mu.RUnlock()

	return map[string]interface{}{
		"pool_size":       opa.poolSize,
		"allocated_count": opa.allocatedCount,
		"available_count": opa.poolSize - opa.allocatedCount,
		"utilization_pct": float64(opa.allocatedCount) / float64(opa.poolSize) * 100,
		"port_range":      fmt.Sprintf("%d-%d", opa.startPort, opa.endPort),
		"next_port":       opa.nextPort,
	}
}

// Reset сбрасывает состояние аллокатора
func (opa *OptimizedPortAllocator) Reset() {
	opa.mu.Lock()
	defer opa.mu.Unlock()

	opa.usedPorts = make(map[int]bool, opa.poolSize)
	opa.nextPort = opa.startPort
	opa.allocatedCount = 0
}

// GetPortRange возвращает диапазон портов
func (opa *OptimizedPortAllocator) GetPortRange() (int, int) {
	return opa.startPort, opa.endPort
}

// Глобальный оптимизированный аллокатор портов
var globalOptimizedPortAllocator = NewOptimizedPortAllocator()

// GetTestPort возвращает доступный порт для тестов (оптимизированная версия)
func GetTestPort() (int, error) {
	return globalOptimizedPortAllocator.AllocatePort()
}

// ReleaseTestPort освобождает тестовый порт (оптимизированная версия)
func ReleaseTestPort(port int) {
	globalOptimizedPortAllocator.ReleasePort(port)
}

// GetTestPortRange возвращает диапазон портов для тестов (оптимизированная версия)
func GetTestPortRange(count int) ([]int, error) {
	return globalOptimizedPortAllocator.AllocatePorts(count)
}

// GetPortStats возвращает статистику использования портов
func GetPortStats() map[string]interface{} {
	return globalOptimizedPortAllocator.GetStats()
}

// ResetPortAllocator сбрасывает состояние глобального аллокатора
func ResetPortAllocator() {
	globalOptimizedPortAllocator.Reset()
}
