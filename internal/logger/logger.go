package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Logger wraps zerolog.Logger with additional context methods
type Logger struct {
	zerolog.Logger
}

// Config contains logger configuration
type Config struct {
	Level  string // debug, info, warn, error
	Format string // json, console
	Output io.Writer
}

// New creates a new logger with the given configuration
func New(cfg Config) *Logger {
	// Set log level
	level := parseLevel(cfg.Level)
	zerolog.SetGlobalLevel(level)

	// Configure output
	var output io.Writer = os.Stdout
	if cfg.Output != nil {
		output = cfg.Output
	}

	// Configure format
	var logger zerolog.Logger
	if cfg.Format == "console" {
		output = zerolog.ConsoleWriter{
			Out:        output,
			TimeFormat: time.RFC3339,
			NoColor:    false,
		}
	}

	logger = zerolog.New(output).
		With().
		Timestamp().
		Caller().
		Logger()

	// Set as global logger
	log.Logger = logger

	return &Logger{Logger: logger}
}

// parseLevel converts string level to zerolog.Level
func parseLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

// WithRequestID adds a request ID to the logger context
func (l *Logger) WithRequestID(requestID string) *Logger {
	logger := l.Logger.With().Str("request_id", requestID).Logger()
	return &Logger{Logger: logger}
}

// WithSessionID adds a session ID to the logger context
func (l *Logger) WithSessionID(sessionID string) *Logger {
	logger := l.Logger.With().Str("session_id", sessionID).Logger()
	return &Logger{Logger: logger}
}

// WithMessageID adds a message ID to the logger context
func (l *Logger) WithMessageID(msgID string) *Logger {
	logger := l.Logger.With().Str("msg_id", msgID).Logger()
	return &Logger{Logger: logger}
}

// WithUser adds a user identifier to the logger context
func (l *Logger) WithUser(user string) *Logger {
	logger := l.Logger.With().Str("user", user).Logger()
	return &Logger{Logger: logger}
}

// WithClientIP adds a client IP to the logger context
func (l *Logger) WithClientIP(ip string) *Logger {
	logger := l.Logger.With().Str("client_ip", ip).Logger()
	return &Logger{Logger: logger}
}

// WithComponent adds a component name to the logger context
func (l *Logger) WithComponent(component string) *Logger {
	logger := l.Logger.With().Str("component", component).Logger()
	return &Logger{Logger: logger}
}

// WithStage adds a pipeline stage to the logger context
func (l *Logger) WithStage(stage string) *Logger {
	logger := l.Logger.With().Str("stage", stage).Logger()
	return &Logger{Logger: logger}
}

// WithRecipient adds a recipient to the logger context
func (l *Logger) WithRecipient(recipient string) *Logger {
	logger := l.Logger.With().Str("recipient", recipient).Logger()
	return &Logger{Logger: logger}
}

// WithError adds an error to the logger context
func (l *Logger) WithError(err error) *Logger {
	logger := l.Logger.With().Err(err).Logger()
	return &Logger{Logger: logger}
}

// WithFields adds multiple fields to the logger context
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	logger := l.Logger.With().Fields(fields).Logger()
	return &Logger{Logger: logger}
}

// Global returns the global logger instance
func Global() *Logger {
	return &Logger{Logger: log.Logger}
}

// Debug logs a debug message
func (l *Logger) Debug(msg string) {
	l.Logger.Debug().Msg(msg)
}

// Info logs an info message
func (l *Logger) Info(msg string) {
	l.Logger.Info().Msg(msg)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string) {
	l.Logger.Warn().Msg(msg)
}

// Error logs an error message
func (l *Logger) Error(msg string) {
	l.Logger.Error().Msg(msg)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string) {
	l.Logger.Fatal().Msg(msg)
}

// Debugf logs a debug message with formatting
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.Logger.Debug().Msgf(format, args...)
}

// Infof logs an info message with formatting
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Logger.Info().Msgf(format, args...)
}

// Warnf logs a warning message with formatting
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.Logger.Warn().Msgf(format, args...)
}

// Errorf logs an error message with formatting
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Logger.Error().Msgf(format, args...)
}

// Fatalf logs a fatal message with formatting and exits
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.Logger.Fatal().Msgf(format, args...)
}
