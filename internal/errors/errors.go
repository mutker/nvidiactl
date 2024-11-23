package errors

import (
	"errors"
	"fmt"
)

// Basic error check functions from standard library
var (
	Is     = errors.Is
	As     = errors.As
	Unwrap = errors.Unwrap
)

// appError implements the Error interface
type appError struct {
	code    ErrorCode
	message string
	err     error
	data    any
}

func (e *appError) Error() string {
	if e.message == "" {
		e.message = GetErrorMessage(e.code)
	}

	if e.data != nil {
		return fmt.Sprintf("%s: %v", e.message, e.data)
	}

	if e.err != nil {
		return fmt.Sprintf("%s: %v", e.message, e.err)
	}

	return e.message
}

func (e *appError) Code() ErrorCode {
	return e.code
}

func (e *appError) WithMessage(msg string) Error {
	return &appError{
		code:    e.code,
		message: msg,
		err:     e.err,
		data:    e.data,
	}
}

func (e *appError) WithData(data any) Error {
	return &appError{
		code:    e.code,
		message: e.message,
		err:     e.err,
		data:    data,
	}
}

func (e *appError) GetData() any {
	return e.data
}

func (e *appError) Unwrap() error {
	return e.err
}

type defaultFactory struct{}

func (*defaultFactory) New(code ErrorCode) Error {
	return &appError{
		code: code,
	}
}

func (*defaultFactory) Wrap(code ErrorCode, err error) Error {
	return &appError{
		code: code,
		err:  err,
	}
}

func (*defaultFactory) WithMessage(code ErrorCode, msg string) Error {
	return &appError{
		code:    code,
		message: msg,
	}
}

func (*defaultFactory) WithData(code ErrorCode, data any) Error {
	return &appError{
		code: code,
		data: data,
	}
}

// New creates a Factory instance for error creation
func New() Factory {
	return &defaultFactory{}
}

// IsNVMLSuccess checks if the error is a "SUCCESS" message from NVML
func IsNVMLSuccess(err error) bool {
	return err != nil && err.Error() == "SUCCESS"
}
