package observability

import (
	// Удалим неиспользуемый импорт
	// "context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"streaming-server/pkg/types"
	"time"
)

var (
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "jsonrpc_requests_total",
			Help: "Total number of JSON-RPC requests",
		},
		[]string{"method", "transport", "status"},
	)

	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "jsonrpc_request_duration_seconds",
			Help: "Duration of JSON-RPC requests",
		},
		[]string{"method", "transport"},
	)

	activeConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "jsonrpc_active_connections",
			Help: "Number of active connections",
		},
		[]string{"transport"},
	)
)

// MetricsMiddleware добавляет сбор метрик
func MetricsMiddleware() types.Middleware {
	return func(req *types.JSONRPCRequest, ctx *types.RequestContext, next types.Handler) (*types.JSONRPCResponse, error) {
		start := time.Now()

		response, err := next(req, ctx)

		duration := time.Since(start)
		status := "success"

		if err != nil {
			status = "error"
		} else if response != nil && response.Error != nil {
			status = "rpc_error"
		}

		requestsTotal.WithLabelValues(req.Method, ctx.Transport, status).Inc()
		requestDuration.WithLabelValues(req.Method, ctx.Transport).Observe(duration.Seconds())

		return response, err
	}
}

// ConnectionTracker отслеживает активные соединения
type ConnectionTracker struct {
	transport string
}

func NewConnectionTracker(transport string) *ConnectionTracker {
	return &ConnectionTracker{transport: transport}
}

func (ct *ConnectionTracker) OnConnect() {
	activeConnections.WithLabelValues(ct.transport).Inc()
}

func (ct *ConnectionTracker) OnDisconnect() {
	activeConnections.WithLabelValues(ct.transport).Dec()
}
