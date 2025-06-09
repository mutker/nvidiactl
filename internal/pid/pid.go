package pid

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"codeberg.org/mutker/nvidiactl/internal/errors"
)

const (
	pidFile = "nvidiactl.pid"
)

// Write writes the current process ID to a PID file.
func Write() error {
	errFactory := errors.New()
	pid := os.Getpid()
	path := filepath.Join(os.TempDir(), pidFile)

	if _, err := os.Stat(path); err == nil {
		// PID file exists, check if the process is running
		bytes, err := os.ReadFile(path)
		if err != nil {
			return errFactory.Wrap(errors.ErrInternal, err)
		}

		pid, err := strconv.Atoi(string(bytes))
		if err != nil {
			return errFactory.Wrap(errors.ErrInternal, err)
		}

		process, err := os.FindProcess(pid)
		if err != nil {
			return errFactory.Wrap(errors.ErrInternal, err)
		}

		err = process.Signal(syscall.Signal(0))
		if err == nil {
			return errFactory.New(errors.ErrAlreadyRunning)
		}
	}

	err := os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o600)
	if err != nil {
		return errFactory.Wrap(errors.ErrInternal, err)
	}

	return nil
}

// Remove removes the PID file.
func Remove() error {
	errFactory := errors.New()
	path := filepath.Join(os.TempDir(), pidFile)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	if err := os.Remove(path); err != nil {
		return errFactory.Wrap(errors.ErrInternal, err)
	}

	return nil
}
