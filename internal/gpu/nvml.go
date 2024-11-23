package gpu

import (
	"codeberg.org/mutker/nvidiactl/internal/errors"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// nvmlController abstracts NVML operations for testing
type nvmlController interface {
	Initialize() error
	Shutdown() error
	GetDeviceCount() (int, error)
	GetDevice(index int) (nvml.Device, error)
	GetDeviceByUUID(uuid string) (nvml.Device, error)
}

type nvmlWrapper struct {
	initialized bool
}

func (w *nvmlWrapper) Initialize() error {
	errFactory := errors.New()
	if w.initialized {
		return nil
	}

	ret := nvml.Init()
	if !IsNVMLSuccess(ret) {
		return errFactory.Wrap(ErrInitFailed, newNVMLError(ret))
	}

	w.initialized = true

	return nil
}

func (w *nvmlWrapper) Shutdown() error {
	errFactory := errors.New()
	if !w.initialized {
		return nil
	}

	ret := nvml.Shutdown()
	if !IsNVMLSuccess(ret) {
		return errFactory.Wrap(ErrShutdownFailed, newNVMLError(ret))
	}

	w.initialized = false

	return nil
}

func (w *nvmlWrapper) GetDeviceCount() (int, error) {
	errFactory := errors.New()
	if !w.initialized {
		return 0, errFactory.New(ErrNotInitialized)
	}

	count, ret := nvml.DeviceGetCount()
	if !IsNVMLSuccess(ret) {
		return 0, errFactory.Wrap(ErrDeviceCountFailed, newNVMLError(ret))
	}

	return count, nil
}

func (w *nvmlWrapper) GetDevice(index int) (nvml.Device, error) {
	errFactory := errors.New()
	if !w.initialized {
		return nil, errFactory.New(ErrNotInitialized)
	}

	device, ret := nvml.DeviceGetHandleByIndex(index)
	if !IsNVMLSuccess(ret) {
		return nil, errFactory.Wrap(ErrDeviceNotFound, newNVMLError(ret))
	}

	return device, nil
}

func (w *nvmlWrapper) GetDeviceByUUID(uuid string) (nvml.Device, error) {
	errFactory := errors.New()
	if !w.initialized {
		return nil, errFactory.New(ErrNotInitialized)
	}

	device, ret := nvml.DeviceGetHandleByUUID(uuid)
	if !IsNVMLSuccess(ret) {
		return nil, errFactory.Wrap(ErrDeviceNotFound, newNVMLError(ret))
	}

	return device, nil
}
