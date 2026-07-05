// Package logging provides structured logging setup for the bot.
package logging

import (
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"bot/internal/config"
)

const serviceName = "telegram-bot-template"

var baseLogger *logrus.Entry

// Context captures common optional fields to attach to log entries.
type Context struct {
	UserID int64
	ChatID int64
	Event  string
}

// Fields is a shorthand alias for structured log fields.
type Fields = logrus.Fields

// Setup configures the global logger using the provided runtime configuration.
// It applies JSON formatting, log level, and default fields.
func Setup(cfg config.Config) (*logrus.Entry, error) {
	level, err := parseLevel(cfg.LogLevel)
	if err != nil {
		return nil, err
	}

	logger := logrus.New()
	logger.SetLevel(level)
	logger.SetFormatter(defaultFormatter())

	baseLogger = logger.WithFields(logrus.Fields{
		"service": serviceName,
		"env":     cfg.AppEnv,
	})

	return baseLogger, nil
}

// Logger returns the configured base logger, initializing a default one if Setup
// has not been called (useful for early boot errors).
func Logger() *logrus.Entry {
	return ensureLogger()
}

// WithContext returns a logger entry enriched with contextual fields when
// provided. Fields are omitted when zero-valued.
func WithContext(ctx Context) *logrus.Entry {
	fields := logrus.Fields{}

	if ctx.UserID != 0 {
		fields["user_id"] = ctx.UserID
	}
	if ctx.ChatID != 0 {
		fields["chat_id"] = ctx.ChatID
	}
	if strings.TrimSpace(ctx.Event) != "" {
		fields["event"] = strings.TrimSpace(ctx.Event)
	}

	return logWithFields(fields)
}

// Info logs an informational message with optional structured fields.
func Info(msg string, fields logrus.Fields) {
	logWithFields(fields).Info(msg)
}

// Warn logs a warning message with optional structured fields.
func Warn(msg string, fields logrus.Fields) {
	logWithFields(fields).Warn(msg)
}

// Error logs an error message with optional structured fields.
func Error(msg string, fields logrus.Fields) {
	logWithFields(fields).Error(msg)
}

func logWithFields(fields logrus.Fields) *logrus.Entry {
	entry := ensureLogger()
	if len(fields) == 0 {
		return entry
	}

	return entry.WithFields(fields)
}

func ensureLogger() *logrus.Entry {
	if baseLogger != nil {
		return baseLogger
	}

	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(defaultFormatter())

	baseLogger = logger.WithFields(logrus.Fields{
		"service": serviceName,
		"env":     config.DefaultAppEnv,
	})

	return baseLogger
}

func defaultFormatter() logrus.Formatter {
	fieldMap := logrus.FieldMap{
		logrus.FieldKeyTime:  "ts",
		logrus.FieldKeyMsg:   "msg",
		logrus.FieldKeyLevel: "level",
	}

	return &logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
		FieldMap:        fieldMap,
	}
}

func parseLevel(value string) (logrus.Level, error) {
	level, err := logrus.ParseLevel(strings.ToLower(strings.TrimSpace(value)))
	if err != nil {
		return logrus.InfoLevel, fmt.Errorf("invalid log level %q: %w", value, err)
	}

	return level, nil
}

// resetLogger clears the cached logger; used in tests.
func resetLogger() {
	baseLogger = nil
}
