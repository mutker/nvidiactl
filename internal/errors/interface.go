package errors

// ErrorCode represents a unique identifier for each error type
type ErrorCode string

// Error represents a domain-specific error with context
type Error interface {
	error
	Code() ErrorCode
	WithMessage(msg string) Error
	WithData(data any) Error
	GetData() any
	Unwrap() error
}

// Factory defines methods for creating domain errors
type Factory interface {
	New(code ErrorCode) Error
	Wrap(code ErrorCode, err error) Error
	WithMessage(code ErrorCode, msg string) Error
	WithData(code ErrorCode, data any) Error
}
