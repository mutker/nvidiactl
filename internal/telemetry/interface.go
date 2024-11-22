package telemetry

import (
	"context"
	"time"
)

// Collector defines the core domain interface
type Collector interface {
	Record(ctx context.Context, snapshot *MetricsSnapshot) error
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
	Current float64
	Average float64
}

type PowerMetrics struct {
	Current int
	Target  int
	Average float64
}

type StateMetrics struct {
	AutoFanControl  bool
	PerformanceMode bool
}
