package middleware

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultAsyncProcessor_Process(t *testing.T) {
	processor := NewDefaultAsyncProcessor()
	defer processor.Shutdown(context.Background())

	executed := false
	var mu sync.Mutex

	fn := func() {
		mu.Lock()
		executed = true
		mu.Unlock()
	}

	err := processor.Process(context.Background(), fn)
	assert.NoError(t, err)

	// Wait a bit for async execution
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	assert.True(t, executed)
	mu.Unlock()
}

func TestDefaultAsyncProcessor_ProcessWithTimeout(t *testing.T) {
	processor := NewDefaultAsyncProcessor()
	defer processor.Shutdown(context.Background())

	executed := false
	var mu sync.Mutex

	fn := func() {
		mu.Lock()
		executed = true
		mu.Unlock()
	}

	err := processor.ProcessWithTimeout(context.Background(), fn, 100*time.Millisecond)
	assert.NoError(t, err)

	// Wait a bit for async execution
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	assert.True(t, executed)
	mu.Unlock()
}

func TestDefaultAsyncProcessor_ProcessWithTimeout_Timeout(t *testing.T) {
	processor := NewDefaultAsyncProcessor()
	defer processor.Shutdown(context.Background())

	slowFn := func() {
		time.Sleep(200 * time.Millisecond)
	}

	err := processor.ProcessWithTimeout(context.Background(), slowFn, 50*time.Millisecond)
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestDefaultAsyncProcessor_Shutdown(t *testing.T) {
	processor := NewDefaultAsyncProcessor()

	// Start some work
	for i := 0; i < 5; i++ {
		processor.Process(context.Background(), func() {
			time.Sleep(10 * time.Millisecond)
		})
	}

	// Shutdown should wait for all work to complete
	err := processor.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestDefaultAsyncProcessor_ShutdownWithTimeout(t *testing.T) {
	processor := NewDefaultAsyncProcessor()

	// Start some slow work
	processor.Process(context.Background(), func() {
		time.Sleep(200 * time.Millisecond)
	})

	// Shutdown with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := processor.Shutdown(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestMockAsyncProcessor_Process(t *testing.T) {
	processor := NewMockAsyncProcessor()

	executed := false
	fn := func() {
		executed = true
	}

	err := processor.Process(context.Background(), fn)
	assert.NoError(t, err)

	// Function should not be executed yet
	assert.False(t, executed)
	assert.Equal(t, 1, processor.GetProcessedFunctionCount())

	// Execute manually
	processor.ExecuteProcessedFunctions()
	assert.True(t, executed)
}

func TestMockAsyncProcessor_ProcessWithErrors(t *testing.T) {
	processor := NewMockAsyncProcessor()

	expectedError := errors.New("test error")
	processor.SetProcessErrors(expectedError)

	fn := func() {}

	err := processor.Process(context.Background(), fn)
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)

	// Function should still be recorded
	assert.Equal(t, 1, processor.GetProcessedFunctionCount())
}

func TestMockAsyncProcessor_ProcessWithTimeout(t *testing.T) {
	processor := NewMockAsyncProcessor()

	fn := func() {}

	err := processor.ProcessWithTimeout(context.Background(), fn, 100*time.Millisecond)
	assert.NoError(t, err)
	assert.Equal(t, 1, processor.GetProcessedFunctionCount())
}

func TestMockAsyncProcessor_Shutdown(t *testing.T) {
	processor := NewMockAsyncProcessor()

	expectedError := errors.New("shutdown error")
	processor.SetShutdownError(expectedError)

	err := processor.Shutdown(context.Background())
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestMockAsyncProcessor_Reset(t *testing.T) {
	processor := NewMockAsyncProcessor()

	// Add some functions and errors
	processor.Process(context.Background(), func() {})
	processor.SetProcessErrors(errors.New("test"))
	processor.SetShutdownError(errors.New("shutdown"))

	assert.Equal(t, 1, processor.GetProcessedFunctionCount())

	// Reset should clear everything
	processor.Reset()

	assert.Equal(t, 0, processor.GetProcessedFunctionCount())

	// Should not return errors after reset
	err := processor.Process(context.Background(), func() {})
	assert.NoError(t, err)

	err = processor.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestMockAsyncProcessor_ExecuteProcessedFunctions_WithNilFunction(t *testing.T) {
	processor := NewMockAsyncProcessor()

	// Manually add a nil function to test robustness
	processor.processedFunctions = append(processor.processedFunctions, nil)

	// Should not panic
	require.NotPanics(t, func() {
		processor.ExecuteProcessedFunctions()
	})
}

func TestMockAsyncProcessor_ConcurrentAccess(t *testing.T) {
	processor := NewMockAsyncProcessor()

	const numGoroutines = 10
	const functionsPerGoroutine = 100

	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < functionsPerGoroutine; j++ {
				processor.Process(context.Background(), func() {})
			}
		}()
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < functionsPerGoroutine; j++ {
				processor.GetProcessedFunctionCount()
			}
		}()
	}

	wg.Wait()

	// Should have processed all functions
	assert.Equal(t, numGoroutines*functionsPerGoroutine, processor.GetProcessedFunctionCount())
}

// Benchmark tests
func BenchmarkDefaultAsyncProcessor_Process(b *testing.B) {
	processor := NewDefaultAsyncProcessor()
	defer processor.Shutdown(context.Background())

	fn := func() {
		// Minimal work
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		processor.Process(context.Background(), fn)
	}
}

func BenchmarkMockAsyncProcessor_Process(b *testing.B) {
	processor := NewMockAsyncProcessor()

	fn := func() {
		// Minimal work
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		processor.Process(context.Background(), fn)
	}
}
