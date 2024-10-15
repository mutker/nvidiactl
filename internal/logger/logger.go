package logger

import (
	"os"
	"syscall"
	"time"

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

// Init initializes the logger based on the given configuration
func Init(debug, verbose, isService bool) {
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	if isService {
		output.TimeFormat = ""
		output.FormatTimestamp = func(i interface{}) string {
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
func Debug() *zerolog.Event { return log.Debug() }

// Info logs an info message
func Info() *zerolog.Event { return log.Info() }

// Warn logs a warning message
func Warn() *zerolog.Event { return log.Warn() }

// Error logs an error message
func Error() *zerolog.Event { return log.Error() }

// Fatal logs a fatal message and exits the program
func Fatal() *zerolog.Event { return log.Fatal() }
