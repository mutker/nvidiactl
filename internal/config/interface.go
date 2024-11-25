package config

import "context"

// Provider defines the interface for accessing configuration values
// All configuration values are immutable after initial loading unless
// Watch functionality is implemented
type Provider interface {
	// GetInterval returns the update interval in seconds
	GetInterval() int

	// GetTemperature returns the maximum allowed temperature in Celsius
	GetTemperature() int

	// GetFanSpeed returns the maximum allowed fan speed percentage
	GetFanSpeed() int

	// GetHysteresis returns the required temperature change before adjusting fan speed
	GetHysteresis() int

	// IsPerformanceMode returns whether performance mode is enabled
	IsPerformanceMode() bool

	// IsMonitorMode returns whether monitor-only mode is enabled
	IsMonitorMode() bool

	// GetLogLevel returns the configured logging level
	GetLogLevel() string

	// IsMetricsEnabled returns whether metrics collection is enabled
	IsMetricsEnabled() bool

	// GetMetricsDBPath returns the path to the metrics database
	GetMetricsDBPath() string
}

// Loader handles the loading and validation of configuration from
// various sources (files, environment variables, flags)
type Loader interface {
	// Load loads configuration from all sources and validates it
	// Returns a Provider interface for accessing the configuration
	// If loading fails, returns an error with appropriate context
	Load(ctx context.Context, opts ...Option) (Provider, error)

	// Validate checks if the current configuration is valid
	// Returns nil if valid, error with validation details otherwise
	Validate() error
}

// Watcher enables live configuration updates
type Watcher interface {
	// Watch starts watching for configuration changes
	// The callback is called when configuration changes are detected
	Watch(ctx context.Context, callback func(Provider)) error
}

// Option defines a configuration option that can be passed to Load
type Option func(*options) error

// options holds internal configuration options
type options struct {
	configPath string
	envPrefix  string
}

// WithConfigFile specifies an explicit configuration file path
func WithConfigFile(path string) Option {
	return func(o *options) error {
		o.configPath = path
		return nil
	}
}

// WithEnvPrefix specifies a custom environment variable prefix
// Default is "NVIDIACTL"
func WithEnvPrefix(prefix string) Option {
	return func(o *options) error {
		o.envPrefix = prefix
		return nil
	}
}

// LogLevel represents valid logging levels
type LogLevel string

const (
	LogLevelDebug   LogLevel = "debug"
	LogLevelInfo    LogLevel = "info"
	LogLevelWarning LogLevel = "warning"
	LogLevelError   LogLevel = "error"
)

// IsValid returns whether the log level is valid
func (l LogLevel) IsValid() bool {
	switch l {
	case LogLevelDebug, LogLevelInfo, LogLevelWarning, LogLevelError:
		return true
	default:
		return false
	}
}

// String implements the Stringer interface
func (l LogLevel) String() string {
	return string(l)
}

// ValidationError represents a configuration validation error
type ValidationError interface {
	error
	// Field returns the name of the invalid field
	Field() string
	// Value returns the invalid value
	Value() interface{}
	// Reason returns why the value is invalid
	Reason() string
}

// Status represents the current state of the configuration
type Status struct {
	// Valid indicates whether the current configuration is valid
	Valid bool
	// ValidationErrors contains any validation errors if Valid is false
	ValidationErrors []ValidationError
}

// GetStatus returns the current configuration status
func GetStatus() Status {
	return Status{
		Valid:            true,
		ValidationErrors: nil,
	}
}
