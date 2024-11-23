package gpu

import (
	"codeberg.org/mutker/nvidiactl/internal/errors"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

const (
	// Initialization and Lifecycle Errors
	ErrNotInitialized   = errors.ErrorCode("gpu_not_initialized")
	ErrInitFailed       = errors.ErrorCode("gpu_init_failed")
	ErrDeviceNotFound   = errors.ErrorCode("gpu_device_not_found")
	ErrShutdownFailed   = errors.ErrorCode("gpu_shutdown_failed")
	ErrDeviceInfoFailed = errors.ErrorCode("gpu_device_info_failed")

	// Temperature Errors
	ErrTemperatureReadFailed = errors.ErrorCode("gpu_temperature_read_failed")

	// Fan Control Errors
	ErrFanControlFailed   = errors.ErrorCode("gpu_fan_control_failed")
	ErrFanCountFailed     = errors.ErrorCode("gpu_fan_count_failed")
	ErrGetFanSpeedFailed  = errors.ErrorCode("gpu_fan_speed_failed")
	ErrGetFanLimitsFailed = errors.ErrorCode("gpu_fan_limits_failed")
	ErrSetFanSpeed        = errors.ErrorCode("gpu_set_fan_speed_failed")
	ErrEnableAutoFan      = errors.ErrorCode("gpu_enable_auto_fan_failed")
	ErrDisableAutoFan     = errors.ErrorCode("gpu_disable_auto_fan_failed")

	// Power Management Errors
	ErrPowerManagementFailed = errors.ErrorCode("gpu_power_management_failed")
	ErrPowerLimitFailed      = errors.ErrorCode("gpu_power_limit_failed")
	ErrPowerLimitsFailed     = errors.ErrorCode("gpu_power_limits_failed")
	ErrSetPowerLimit         = errors.ErrorCode("gpu_set_power_limit_failed")

	// Device Discovery Errors
	ErrDeviceCountFailed = errors.ErrorCode("gpu_device_count_failed")
	ErrDeviceUUIDFailed  = errors.ErrorCode("gpu_device_uuid_failed")
)

// nvmlError represents an NVML-specific error
type nvmlError struct {
	ret nvml.Return
}

func (e nvmlError) Error() string {
	return nvml.ErrorString(e.ret)
}

// newNVMLError creates an error from an NVML return code
func newNVMLError(ret nvml.Return) error {
	if ret == nvml.SUCCESS {
		return nil
	}
	return &nvmlError{ret: ret}
}

// IsNVMLSuccess checks if a Return value indicates success
func IsNVMLSuccess(ret nvml.Return) bool {
	return ret == nvml.SUCCESS
}
