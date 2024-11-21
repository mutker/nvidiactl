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
	if w.initialized {
		return nil
	}

	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return errors.Wrap(ErrInitFailed, nvml.ErrorString(ret))
	}

	w.initialized = true

	return nil
}

func (w *nvmlWrapper) Shutdown() error {
	if !w.initialized {
		return nil
	}

	ret := nvml.Shutdown()
	if ret != nvml.SUCCESS {
		return errors.Wrap(ErrShutdownFailed, nvml.ErrorString(ret))
	}

	w.initialized = false

	return nil
}

func (w *nvmlWrapper) GetDeviceCount() (int, error) {
	if !w.initialized {
		return 0, errors.New(ErrNotInitialized)
	}

	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return 0, errors.Wrap(ErrDeviceCountFailed, nvml.ErrorString(ret))
	}

	return count, nil
}

func (w *nvmlWrapper) GetDevice(index int) (nvml.Device, error) {
	if !w.initialized {
		return nil, errors.New(ErrNotInitialized)
	}

	device, ret := nvml.DeviceGetHandleByIndex(index)
	if ret != nvml.SUCCESS {
		return nil, errors.Wrap(ErrDeviceNotFound, nvml.ErrorString(ret))
	}

	return device, nil
}

func (w *nvmlWrapper) GetDeviceByUUID(uuid string) (nvml.Device, error) {
	if !w.initialized {
		return nil, errors.New(ErrNotInitialized)
	}

	device, ret := nvml.DeviceGetHandleByUUID(uuid)
	if ret != nvml.SUCCESS {
		return nil, errors.Wrap(ErrDeviceNotFound, nvml.ErrorString(ret))
	}

	return device, nil
}
