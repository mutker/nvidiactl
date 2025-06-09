package metrics

import (
	"context"
	"time"
)

// MetricsCollector defines the core domain interface
type MetricsCollector interface {
	Record(ctx context.Context, snapshot *MetricsSnapshot) error
	Close() error
	IsReadOnly() bool
}

// Repository defines the interface for metrics data storage
type MetricsRepository interface {
	Record(snapshot *MetricsSnapshot) error
	Close() error
}

// MetricsSnapshot represents domain entities
type MetricsSnapshot struct {
	Timestamp   time.Time
	FanSpeed    FanMetrics
	Temperature TempMetrics
	PowerLimit  PowerMetrics
	SystemState StateMetrics
}

// Domain value objects
type FanMetrics struct {
	Current int
	Target  int
}

type TempMetrics struct {
	Current int
	Average int
}

type PowerMetrics struct {
	Current int
	Target  int
	Average int
}

type StateMetrics struct {
	AutoFanControl  bool
	PerformanceMode bool
}
