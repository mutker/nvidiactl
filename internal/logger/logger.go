package logger

import (
	"os"
	"syscall"
	"time"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"github.com/rs/zerolog"
)

var log zerolog.Logger

type LogLevel int8

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

type LogEvent struct {
	*zerolog.Event
}

func (e *LogEvent) Msg(msg string) {
	e.Event.Msg(msg)
}

func (e *LogEvent) Send() {
	e.Event.Send()
}

// Init initializes the logger based on the given configuration
func Init(debug, verbose, isService bool) {
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	if isService {
		output.TimeFormat = ""
		output.FormatTimestamp = func(_ interface{}) string {
			return ""
		}
	}

	log = zerolog.New(output).With().Timestamp().Logger()

	SetLogLevel(WarnLevel) // Default log level

	if debug {
		SetLogLevel(DebugLevel)
	} else if verbose {
		SetLogLevel(InfoLevel)
	}
}

// SetLogLevel sets the global log level
func SetLogLevel(level LogLevel) {
	zerolog.SetGlobalLevel(zerolog.Level(level))
}

// IsService checks if the application is running as a service
func IsService() bool {
	if _, err := os.Stdin.Stat(); err != nil {
		return true
	}
	if os.Getenv("SERVICE_NAME") != "" || os.Getenv("INVOCATION_ID") != "" {
		return true
	}
	if os.Getppid() == 1 {
		return true
	}

	return syscall.Getpgrp() == syscall.Getpid()
}

// Debug logs a debug message
func Debug() *LogEvent {
	return &LogEvent{log.Debug()}
}

// Info logs an info message
func Info() *LogEvent {
	return &LogEvent{log.Info()}
}

// Warn logs a warning message
func Warn() *LogEvent {
	return &LogEvent{log.Warn()}
}

// Error logs an error message
func Error() *LogEvent {
	return &LogEvent{log.Error()}
}

// ErrorWithCode logs an error message with a specific error code
func ErrorWithCode(err *errors.AppError) *LogEvent {
	return &LogEvent{log.Error().
		Str("error_code", string(err.Code)).
		Str("error_message", err.Error()).
		AnErr("error", err.Err)}
}

// Fatal logs a fatal message and exits the program
func Fatal() *LogEvent {
	return &LogEvent{log.Fatal()}
}

// FatalWithCode logs a fatal message with a specific error code and exits the program
func FatalWithCode(err *errors.AppError) *LogEvent {
	return &LogEvent{log.Fatal().
		Str("error_code", string(err.Code)).
		Str("error_message", err.Error()).
		AnErr("error", err.Err)}
}
