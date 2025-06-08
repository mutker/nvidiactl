package logger

import (
	"os"
	"syscall"
	"time"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"github.com/rs/zerolog"
)

type LogLevel int8

var logLevelMap = map[string]LogLevel{
	"debug":   DebugLevel,
	"info":    InfoLevel,
	"warning": WarnLevel,
	"error":   ErrorLevel,
}

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

type logger struct {
	log zerolog.Logger
}

func (e *LogEvent) Msg(msg string) {
	e.Event.Msg(msg)
}

func (e *LogEvent) Send() {
	e.Event.Send()
}

// New initializes the logger based on the given configuration
func New(logLevel string, isService bool) Logger {
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

	log := zerolog.New(output).With().Timestamp().Logger()

	// Set log level from string
	if level, ok := logLevelMap[logLevel]; ok {
		log = log.Level(zerolog.Level(level))
	} else {
		log = log.Level(zerolog.WarnLevel) // Fallback to warning if invalid
	}

	return &logger{log}
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
func (l *logger) Debug() *LogEvent {
	return &LogEvent{l.log.Debug()}
}

// Info logs an info message
func (l *logger) Info() *LogEvent {
	return &LogEvent{l.log.Info()}
}

// Warn logs a warning message
func (l *logger) Warn() *LogEvent {
	return &LogEvent{l.log.Warn()}
}

// Error logs an error message
func (l *logger) Error() *LogEvent {
	return &LogEvent{l.log.Error()}
}

// ErrorWithCode logs an error message with a specific error code
func (l *logger) ErrorWithCode(err errors.Error) *LogEvent {
	event := l.log.Error()
	if err != nil {
		event = event.Str("error_code", string(err.Code())).
			Str("error_message", err.Error())

		if unwrapped := err.Unwrap(); unwrapped != nil {
			event = event.AnErr("error", unwrapped)
		}
	}
	return &LogEvent{event}
}

// FatalWithCode logs a fatal message with a specific error code and exits the program
func (l *logger) FatalWithCode(err errors.Error) *LogEvent {
	event := l.log.Fatal()
	if err != nil {
		event = event.Str("error_code", string(err.Code())).
			Str("error_message", err.Error())

		if unwrapped := err.Unwrap(); unwrapped != nil {
			event = event.AnErr("error", unwrapped)
		}
	}
	return &LogEvent{event}
}

func (l *logger) ErrorWithContext(err errors.Error, component, operation string) *LogEvent {
	event := l.log.Error().
		Str("component", component).
		Str("operation", operation)

	if err != nil {
		event = event.
			Str("error_code", string(err.Code())).
			Str("error_message", err.Error())

		if unwrapped := err.Unwrap(); unwrapped != nil {
			event = event.AnErr("error", unwrapped)
		}
	}

	return &LogEvent{event}
}
