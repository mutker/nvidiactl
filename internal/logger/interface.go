package logger

import "codeberg.org/mutker/nvidiactl/internal/errors"

// Logger defines the interface for logging operations.
type Logger interface {
	Debug() *LogEvent
	Info() *LogEvent
	Warn() *LogEvent
	Error() *LogEvent
	ErrorWithCode(err errors.Error) *LogEvent
	FatalWithCode(err errors.Error) *LogEvent
	ErrorWithContext(err errors.Error, component, operation string) *LogEvent
}
