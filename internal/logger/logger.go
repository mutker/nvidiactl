package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/journald"
)

type LogLevel int8

var log zerolog.Logger

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

func Init(debug, verbose bool) {
	var output io.Writer

	if IsService() {
		output = journald.NewJournalDWriter()
	} else {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	var level zerolog.Level
	if debug {
		level = zerolog.DebugLevel
	} else if verbose {
		level = zerolog.InfoLevel
	} else {
		level = zerolog.WarnLevel
	}

	log = zerolog.New(output).With().Timestamp().Logger().Level(level)

	// Add a test log message
	log.Info().Msg("Logger initialized")
}

// SetLogLevel sets the global log level
func SetLogLevel(level LogLevel) {
	switch level {
	case DebugLevel:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case InfoLevel:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case WarnLevel:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case ErrorLevel:
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case FatalLevel:
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	}
}

// IsService checks if the application is running as a service
func IsService() bool {
	return os.Getenv("JOURNAL_STREAM") != ""
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
