package types

import "time"

// Clock интерфейс позволяет создавать моки для операций со временем в тестах
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
	Sleep(d time.Duration)
	After(d time.Duration) <-chan time.Time
}

// RealClock реализует Clock используя стандартный пакет time
type RealClock struct{}

// Now возвращает текущее время
func (c *RealClock) Now() time.Time {
	return time.Now()
}

// Since возвращает время, прошедшее с момента t
func (c *RealClock) Since(t time.Time) time.Duration {
	return time.Since(t)
}

// Sleep приостанавливает текущую горутину как минимум на время d
func (c *RealClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

// After ожидает истечения указанного времени и затем отправляет текущее время в возвращаемый канал
func (c *RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// MockClock реализует Clock для тестирования с контролируемым временем
type MockClock struct {
	currentTime time.Time
	sleepCalls  []time.Duration
	afterChans  []chan time.Time
}

// NewMockClock создает новый MockClock с заданным начальным временем
func NewMockClock(initialTime time.Time) *MockClock {
	return &MockClock{
		currentTime: initialTime,
		sleepCalls:  make([]time.Duration, 0),
		afterChans:  make([]chan time.Time, 0),
	}
}

// Now возвращает текущее мок-время
func (m *MockClock) Now() time.Time {
	return m.currentTime
}

// Since возвращает продолжительность с момента t, используя мок-время
func (m *MockClock) Since(t time.Time) time.Duration {
	return m.currentTime.Sub(t)
}

// Sleep записывает продолжительность сна, но на самом деле не спит
func (m *MockClock) Sleep(d time.Duration) {
	m.sleepCalls = append(m.sleepCalls, d)
	m.currentTime = m.currentTime.Add(d)
}

// After создает канал, который получит время после продвижения
func (m *MockClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	m.afterChans = append(m.afterChans, ch)
	return ch
}

// Advance продвигает мок-часы на указанную продолжительность
func (m *MockClock) Advance(d time.Duration) {
	m.currentTime = m.currentTime.Add(d)

	// Активировать любые ожидающие каналы After
	for _, ch := range m.afterChans {
		select {
		case ch <- m.currentTime:
		default:
		}
	}
	m.afterChans = nil
}

// GetSleepCalls возвращает все записанные продолжительности сна
func (m *MockClock) GetSleepCalls() []time.Duration {
	return m.sleepCalls
}

// SetTime устанавливает текущее мок-время
func (m *MockClock) SetTime(t time.Time) {
	m.currentTime = t
}

// Reset сбрасывает состояние мок-часов
func (m *MockClock) Reset() {
	m.sleepCalls = nil
	m.afterChans = nil
}
