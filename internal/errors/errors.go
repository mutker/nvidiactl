package errors

import (
	"errors"
	"fmt"
)

// ErrorCode represents a unique identifier for each error type
type ErrorCode string

// Basic error check functions
var (
	Is     = errors.Is
	As     = errors.As
	Unwrap = errors.Unwrap
)

// AppError represents a domain-specific error with context
type AppError struct {
	Code    ErrorCode // Unique error identifier
	Message string    // Human-readable error message
	Err     error     // Wrapped error (if any)
	Data    any       // Optional context data
}

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

	// Telemetry errors
	ErrInitTelemetry    ErrorCode = "init_telemetry_failed"
	ErrCollectTelemetry ErrorCode = "collect_telemetry_failed"
	ErrCloseTelemetry   ErrorCode = "close_telemetry_failed"
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
	ErrInitTelemetry:     "Failed to initialize telemetry",
	ErrCollectTelemetry:  "Failed to collect telemetry data",
	ErrCloseTelemetry:    "Failed to close telemetry connection",
	ErrInitApp:           "Failed to initialize application",
	ErrMainLoop:          "Error in main loop",
	ErrGetGPUState:       "Failed to get GPU state",
	ErrShutdownGPU:       "Failed to shutdown GPU",
	ErrResetPowerLimit:   "Failed to reset power limit",
	ErrEnableAutoFan:     "Failed to enable auto fan control",
}

func (e *AppError) Error() string {
	if e.Message == "" {
		e.Message = string(e.Code)
	}

	if e.Data != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Data)
	}

	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}

	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// New creates a basic AppError with just a code
func New(code ErrorCode) *AppError {
	return &AppError{
		Code:    code,
		Message: string(code),
	}
}

// Wrap creates an AppError that wraps another error
func Wrap(code ErrorCode, err any) *AppError {
	return &AppError{
		Code:    code,
		Message: string(code),
		Err:     asError(err),
	}
}

// WithMessage creates an AppError with a custom message
func WithMessage(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// WithData creates an AppError with additional context data
func WithData(code ErrorCode, data any) *AppError {
	return &AppError{
		Code:    code,
		Message: string(code),
		Data:    data,
	}
}

// asError converts various error-like types to error interface
func asError(err any) error {
	switch e := err.(type) {
	case error:
		return e
	default:
		// Use our own WithData function to wrap non-error types
		return WithData(ErrInternal, err).Err
	}
}

// IsNVMLSuccess checks if the error is a "SUCCESS" message from NVML
func IsNVMLSuccess(err error) bool {
	return err != nil && err.Error() == "SUCCESS"
}

// IsAppError checks if an error is an AppError and returns it if true
func IsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}

	return nil, false
}

// GetErrorMessage returns the message for a given error code
func GetErrorMessage(code ErrorCode) string {
	if msg, ok := errorMessages[code]; ok {
		return msg
	}

	return string(code)
}

// ErrorMessage returns the message for a given error code
func ErrorMessage(code ErrorCode) string {
	if msg, ok := errorMessages[code]; ok {
		return msg
	}

	return string(code)
}
