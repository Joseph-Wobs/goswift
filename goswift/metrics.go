// go-swift/goswift/metrics.go
package goswift

import (
	"sync"
	"time"
)

// RouteMetrics holds metrics for a specific route.
type RouteMetrics struct {
	RequestCount int           `json:"request_count"`
	TotalLatency time.Duration `json:"total_latency_ns"` // Total latency in nanoseconds
	AvgLatency   time.Duration `json:"avg_latency_ns"`   // Average latency in nanoseconds
}

// MetricsManager collects and provides simple application metrics.
type MetricsManager struct {
	mu      sync.RWMutex
	metrics map[string]*RouteMetrics // map[routePath]*RouteMetrics
}

// NewMetricsManager creates and initializes a new MetricsManager.
func NewMetricsManager() *MetricsManager {
	return &MetricsManager{
		metrics: make(map[string]*RouteMetrics),
	}
}

// RecordRequest updates metrics for a given route path.
func (mm *MetricsManager) RecordRequest(routePath string, duration time.Duration) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, ok := mm.metrics[routePath]; !ok {
		mm.metrics[routePath] = &RouteMetrics{}
	}

	mm.metrics[routePath].RequestCount++
	mm.metrics[routePath].TotalLatency += duration
	mm.metrics[routePath].AvgLatency = mm.metrics[routePath].TotalLatency / time.Duration(mm.metrics[routePath].RequestCount)
}

// GetMetrics returns a copy of the collected metrics.
func (mm *MetricsManager) GetMetrics() map[string]RouteMetrics {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	// Create a deep copy to prevent external modification
	copiedMetrics := make(map[string]RouteMetrics, len(mm.metrics))
	for path, metrics := range mm.metrics {
		copiedMetrics[path] = *metrics // Copy the struct value
	}
	return copiedMetrics
}
