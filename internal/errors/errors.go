package errors

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	ErrInvalidInterval    ErrorCode = "invalid_interval"
	ErrBindFlags          ErrorCode = "bind_flags_failed"
	ErrReadConfig         ErrorCode = "read_config_failed"
	ErrUninitializedGPU   ErrorCode = "gpu_uninitialized"
	ErrNVMLFailure        ErrorCode = "nvml_operation_failed"
	ErrPowerLimitTooHigh  ErrorCode = "power_limit_too_high"
	ErrPowerLimitTooLow   ErrorCode = "power_limit_too_low"
	ErrGetFanCount        ErrorCode = "get_fan_count_failed"
	ErrInitFanSpeed       ErrorCode = "init_fan_speed_failed"
	ErrInitPowerLimits    ErrorCode = "init_power_limits_failed"
	ErrGetCurrentFanSpeed ErrorCode = "get_current_fan_speed_failed"
	ErrSetDefaultFanSpeed ErrorCode = "set_default_fan_speed_failed"
	ErrGetPowerLimit      ErrorCode = "get_power_limit_failed"
	ErrSetPowerLimit      ErrorCode = "set_power_limit_failed"
	ErrShutdownGPU        ErrorCode = "shutdown_gpu_failed"
	ErrMainLoop           ErrorCode = "main_loop_error"
	ErrGetGPUState        ErrorCode = "get_gpu_state_failed"
	ErrSetGPUState        ErrorCode = "set_gpu_state_failed"
	ErrEnableAutoFan      ErrorCode = "enable_auto_fan_failed"
	ErrResetPowerLimit    ErrorCode = "reset_power_limit_failed"
	ErrInitApp            ErrorCode = "init_app_failed"
	ErrSetFanSpeed        ErrorCode = "set_fan_speed_failed"
	ErrInitTelemetry      ErrorCode = "init_telemetry_failed"
	ErrCollectTelemetry   ErrorCode = "collect_telemetry_failed"
	ErrCloseTelemetry     ErrorCode = "close_telemetry_failed"
)

var errorMessages = map[ErrorCode]string{
	ErrInvalidInterval:    "Invalid interval",
	ErrBindFlags:          "Failed to bind flags",
	ErrReadConfig:         "Failed to read config file",
	ErrUninitializedGPU:   "GPU not initialized",
	ErrNVMLFailure:        "NVML operation failed",
	ErrPowerLimitTooHigh:  "Power limit too high",
	ErrPowerLimitTooLow:   "Power limit too low",
	ErrGetFanCount:        "Failed to get fan count",
	ErrInitFanSpeed:       "Failed to initialize fan speed",
	ErrInitPowerLimits:    "Failed to initialize power limits",
	ErrGetCurrentFanSpeed: "Failed to get current fan speed",
	ErrSetDefaultFanSpeed: "Failed to set default fan speed",
	ErrGetPowerLimit:      "Failed to get power limit",
	ErrSetPowerLimit:      "Failed to set power limit",
	ErrShutdownGPU:        "Error occurred during GPU shutdown",
	ErrMainLoop:           "Error in main loop",
	ErrGetGPUState:        "Failed to get GPU state",
	ErrSetGPUState:        "Failed to set GPU state",
	ErrEnableAutoFan:      "Failed to enable auto fan control",
	ErrResetPowerLimit:    "Failed to reset power limit",
	ErrInitApp:            "Failed to initialize application",
	ErrSetFanSpeed:        "Failed to set fan speed",
	ErrInitTelemetry:      "Failed to initialize telemetry db",
	ErrCollectTelemetry:   "Failed to collect telemetry",
	ErrCloseTelemetry:     "Failed to close telemetry db",
}

type AppError struct {
	Code ErrorCode
	Err  error
	Data interface{}
}

func (e *AppError) Error() string {
	msg := errorMessages[e.Code]
	if e.Data != nil {
		msg = fmt.Sprintf("%s: %v", msg, e.Data)
	}
	if e.Err != nil {
		msg = fmt.Sprintf("%s: %v", msg, e.Err)
	}

	return msg
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// IsNVMLSuccess checks if the error is a "SUCCESS" message from NVML
func IsNVMLSuccess(err error) bool {
	return err != nil && err.Error() == "SUCCESS"
}

// IsAppError checks if the given error is an AppError and returns it if true
func IsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}

	return nil, false
}

func New(code ErrorCode) *AppError {
	return &AppError{
		Code: code,
	}
}

func Wrap(code ErrorCode, err error) *AppError {
	return &AppError{
		Code: code,
		Err:  err,
	}
}

// WithData creates a new AppError with additional data
func WithData(code ErrorCode, data interface{}) *AppError {
	return &AppError{
		Code: code,
		Data: data,
	}
}

// ErrorMessage returns the error message for a given ErrorCode
func ErrorMessage(code ErrorCode) string {
	return errorMessages[code]
}
