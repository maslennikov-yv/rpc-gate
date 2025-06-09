package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
	// Удалим неиспользуемый импорт
	// "streaming-server/pkg/types"
)

// HealthChecker интерфейс для проверки здоровья компонентов
type HealthChecker interface {
	Check(ctx context.Context) error
	Name() string
}

// HealthService управляет проверками здоровья
type HealthService struct {
	checkers []HealthChecker
}

// NewHealthService создает новый сервис здоровья
func NewHealthService() *HealthService {
	return &HealthService{
		checkers: make([]HealthChecker, 0),
	}
}

// AddChecker добавляет проверку здоровья
func (hs *HealthService) AddChecker(checker HealthChecker) {
	hs.checkers = append(hs.checkers, checker)
}

// HealthStatus представляет статус здоровья
type HealthStatus struct {
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Service   string                 `json:"service"`
	Version   string                 `json:"version"`
	Checks    map[string]CheckResult `json:"checks"`
}

// CheckResult результат проверки здоровья
type CheckResult struct {
	Status  string        `json:"status"`
	Message string        `json:"message,omitempty"`
	Latency time.Duration `json:"latency"`
}

// Check выполняет все проверки здоровья
func (hs *HealthService) Check(ctx context.Context) HealthStatus {
	status := HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Service:   "streaming-server",
		Version:   "1.0.0",
		Checks:    make(map[string]CheckResult),
	}

	for _, checker := range hs.checkers {
		start := time.Now()
		err := checker.Check(ctx)
		latency := time.Since(start)

		result := CheckResult{
			Status:  "healthy",
			Latency: latency,
		}

		if err != nil {
			result.Status = "unhealthy"
			result.Message = err.Error()
			status.Status = "unhealthy"
		}

		status.Checks[checker.Name()] = result
	}

	return status
}

// HTTPHandler возвращает HTTP обработчик для проверки здоровья
func (hs *HealthService) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		health := hs.Check(ctx)

		w.Header().Set("Content-Type", "application/json")

		if health.Status == "healthy" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(health)
	}
}
