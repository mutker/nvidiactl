package logger

import (
	"io"
	"os"
	"syscall"
	"time"

	"github.com/rs/zerolog"
)

var log zerolog.Logger

// Init initializes the logger based on the given configuration
func Init(debug, verbose bool, isService bool) {
	var output io.Writer

	if isService {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
		output = os.Stdout
	} else {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	log = zerolog.New(output).With().Timestamp().Logger()

	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else if verbose {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	}
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
	if syscall.Getpgrp() == syscall.Getpid() {
		return true
	}
	return false
}

// Debug logs a debug message
func Debug() *zerolog.Event {
	return log.Debug()
}

// Info logs an info message
func Info() *zerolog.Event {
	return log.Info()
}

// Warn logs a warning message
func Warn() *zerolog.Event {
	return log.Warn()
}

// Error logs an error message
func Error() *zerolog.Event {
	return log.Error()
}

// Fatal logs a fatal message and exits the program
func Fatal() *zerolog.Event {
	return log.Fatal()
}