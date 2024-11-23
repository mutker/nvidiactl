package errors

// Common error codes
const (
	// System errors
	ErrInternal        ErrorCode = "internal_error"
	ErrInvalidArgument ErrorCode = "invalid_argument"
	ErrNotImplemented  ErrorCode = "not_implemented"
	ErrUnavailable     ErrorCode = "service_unavailable"

	// Configuration errors
	ErrInvalidConfig   ErrorCode = "invalid_configuration"
	ErrMissingConfig   ErrorCode = "missing_configuration"
	ErrBindFlags       ErrorCode = "bind_flags_failed"
	ErrReadConfig      ErrorCode = "read_config_failed"
	ErrInvalidInterval ErrorCode = "invalid_interval"

	// Logging errors
	ErrInvalidLogLevel ErrorCode = "invalid_log_level"

	// Initialization errors
	ErrInitFailed     ErrorCode = "initialization_failed"
	ErrShutdownFailed ErrorCode = "shutdown_failed"

	// Resource errors
	ErrResourceBusy      ErrorCode = "resource_busy"
	ErrResourceNotFound  ErrorCode = "resource_not_found"
	ErrResourceExhausted ErrorCode = "resource_exhausted"

	// Application errors
	ErrInitApp         ErrorCode = "init_app_failed"
	ErrMainLoop        ErrorCode = "main_loop_failed"
	ErrSetGPUState     ErrorCode = "get_gpu_state_failed"
	ErrGetGPUState     ErrorCode = "get_gpu_state_failed"
	ErrShutdownGPU     ErrorCode = "shutdown_gpu_failed"
	ErrResetPowerLimit ErrorCode = "reset_power_limit_failed"
	ErrEnableAutoFan   ErrorCode = "enable_auto_fan_failed"

	// Operation errors
	ErrOperationFailed  ErrorCode = "operation_failed"
	ErrTimeout          ErrorCode = "operation_timeout"
	ErrInvalidOperation ErrorCode = "invalid_operation"

	// Metrics errors
	ErrInitMetrics    ErrorCode = "init_metrics_failed"
	ErrCollectMetrics ErrorCode = "collect_metrics_failed"
	ErrCloseMetrics   ErrorCode = "close_metrics_failed"
)

// Common error messages
var errorMessages = map[ErrorCode]string{
	ErrInternal:          "Internal error occurred",
	ErrInvalidArgument:   "Invalid argument provided",
	ErrNotImplemented:    "Operation not implemented",
	ErrUnavailable:       "Service unavailable",
	ErrInvalidConfig:     "Invalid configuration",
	ErrMissingConfig:     "Missing configuration",
	ErrBindFlags:         "Failed to bind flags",
	ErrReadConfig:        "Failed to read configuration",
	ErrInitFailed:        "Initialization failed",
	ErrShutdownFailed:    "Shutdown failed",
	ErrResourceBusy:      "Resource is busy",
	ErrResourceNotFound:  "Resource not found",
	ErrResourceExhausted: "Resource exhausted",
	ErrOperationFailed:   "Operation failed",
	ErrTimeout:           "Operation timed out",
	ErrInvalidOperation:  "Invalid operation",
	ErrInvalidInterval:   "Invalid interval value",
	ErrInitMetrics:       "Failed to initialize metrics",
	ErrCollectMetrics:    "Failed to collect metrics data",
	ErrCloseMetrics:      "Failed to close metrics connection",
	ErrInitApp:           "Failed to initialize application",
	ErrMainLoop:          "Error in main loop",
	ErrGetGPUState:       "Failed to get GPU state",
	ErrShutdownGPU:       "Failed to shutdown GPU",
	ErrResetPowerLimit:   "Failed to reset power limit",
	ErrEnableAutoFan:     "Failed to enable auto fan control",
}

// GetErrorMessage returns the message for a given error code
func GetErrorMessage(code ErrorCode) string {
	if msg, ok := errorMessages[code]; ok {
		return msg
	}

	return string(code)
}
