package logging

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"

	"bot/internal/config"
)

func TestSetupUsesJSONFormatterInProduction(t *testing.T) {
	resetLogger()

	entry, err := Setup(config.Config{AppEnv: config.EnvProduction, LogLevel: "info"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jsonFormatter, ok := entry.Logger.Formatter.(*logrus.JSONFormatter)
	if !ok {
		t.Fatalf("expected JSON formatter, got %T", entry.Logger.Formatter)
	}

	if jsonFormatter.FieldMap[logrus.FieldKeyTime] != "ts" {
		t.Fatalf("expected ts field for timestamps, got %q", jsonFormatter.FieldMap[logrus.FieldKeyTime])
	}
	if entry.Data["service"] != serviceName {
		t.Fatalf("expected service field, got %v", entry.Data["service"])
	}
	if entry.Data["env"] != config.EnvProduction {
		t.Fatalf("expected env field to be %q, got %v", config.EnvProduction, entry.Data["env"])
	}
}

func TestSetupUsesJSONFormatterInDevelopment(t *testing.T) {
	resetLogger()

	entry, err := Setup(config.Config{AppEnv: config.EnvDevelopment, LogLevel: "debug"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := entry.Logger.Formatter.(*logrus.JSONFormatter); !ok {
		t.Fatalf("expected JSON formatter, got %T", entry.Logger.Formatter)
	}
	if entry.Data["env"] != config.EnvDevelopment {
		t.Fatalf("expected env field to be %q, got %v", config.EnvDevelopment, entry.Data["env"])
	}
}

func TestSetupRejectsInvalidLogLevel(t *testing.T) {
	resetLogger()

	if _, err := Setup(config.Config{AppEnv: config.EnvDevelopment, LogLevel: "loud"}); err == nil {
		t.Fatalf("expected error for invalid log level")
	}

	if baseLogger != nil {
		t.Fatalf("base logger should remain unset after failure")
	}
}

func TestLoggingHelpersIncludeContextAndLevels(t *testing.T) {
	resetLogger()

	logger, hook := test.NewNullLogger()
	logger.SetFormatter(defaultFormatter())
	baseLogger = logger.WithFields(logrus.Fields{
		"service": serviceName,
		"env":     config.EnvDevelopment,
	})

	Info("hello world", logrus.Fields{"event": "startup"})
	Warn("careful now", nil)
	Error("boom", logrus.Fields{"error": "fail"})

	entries := hook.AllEntries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 log entries, got %d", len(entries))
	}

	if entries[0].Level != logrus.InfoLevel || entries[0].Data["event"] != "startup" {
		t.Fatalf("expected info level with startup event, got level=%s data=%v", entries[0].Level, entries[0].Data)
	}
	if entries[1].Level != logrus.WarnLevel {
		t.Fatalf("expected warn level, got %s", entries[1].Level)
	}
	if entries[2].Level != logrus.ErrorLevel || entries[2].Data["error"] != "fail" {
		t.Fatalf("expected error level with error field, got level=%s data=%v", entries[2].Level, entries[2].Data)
	}

	ctxEntry := WithContext(Context{UserID: 42, ChatID: -1001, Event: "ping"})
	ctxEntry.Info("ctx log")

	last := hook.LastEntry()
	if last.Data["user_id"] != int64(42) || last.Data["chat_id"] != int64(-1001) || last.Data["event"] != "ping" {
		t.Fatalf("expected context fields, got %v", last.Data)
	}
	if last.Data["service"] != serviceName || last.Data["env"] != config.EnvDevelopment {
		t.Fatalf("expected base fields preserved, got %v", last.Data)
	}
}
