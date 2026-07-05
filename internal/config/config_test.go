package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultsAndRequired(t *testing.T) {
	unsetEnv(t, KeyAppEnv)
	unsetEnv(t, KeyLogLevel)

	t.Setenv(KeyTelegramToken, "token")
	t.Setenv(KeyBotOwner, "12345")
	t.Setenv(KeyMongoURI, "mongodb://localhost:27017")
	t.Setenv(KeyMongoDB, "telegram_bot")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.AppEnv != DefaultAppEnv {
		t.Fatalf("expected app env %s, got %s", DefaultAppEnv, cfg.AppEnv)
	}
	if cfg.BotOwnerID != 12345 {
		t.Fatalf("expected bot owner id to be parsed, got %d", cfg.BotOwnerID)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("expected default log level %s, got %s", DefaultLogLevel, cfg.LogLevel)
	}
}

func TestLoadFailsOnMissingRequired(t *testing.T) {
	unsetEnv(t, KeyAppEnv)

	unsetEnv(t, KeyTelegramToken)
	t.Setenv(KeyBotOwner, "999")
	t.Setenv(KeyMongoURI, "mongodb://localhost:27017")
	t.Setenv(KeyMongoDB, "telegram_bot")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected missing required env to error")
	}
	if !strings.Contains(err.Error(), KeyTelegramToken) {
		t.Fatalf("expected error to mention missing %s, got %v", KeyTelegramToken, err)
	}
}

func TestLoadValidatesOwnerID(t *testing.T) {
	unsetEnv(t, KeyAppEnv)

	t.Setenv(KeyTelegramToken, "token")
	t.Setenv(KeyBotOwner, "abc")
	t.Setenv(KeyMongoURI, "mongodb://localhost:27017")
	t.Setenv(KeyMongoDB, "telegram_bot")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for invalid %s", KeyBotOwner)
	}
	if !strings.Contains(err.Error(), KeyBotOwner) {
		t.Fatalf("expected error to mention %s, got %v", KeyBotOwner, err)
	}
}

func TestLoadUsesDotEnvInDevelopment(t *testing.T) {
	tmpDir := t.TempDir()
	dotenvContent := []byte(`
APP_ENV=development
TELEGRAM_TOKEN=dotenv-token
BOT_OWNER=77
MONGO_URI=mongodb://from-dotenv
MONGO_DB=telegram_bot_dev
LOG_LEVEL=debug
`)

	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), dotenvContent, 0o644); err != nil {
		t.Fatalf("failed to write dotenv: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	unsetEnv(t, KeyAppEnv)
	unsetEnv(t, KeyTelegramToken)
	unsetEnv(t, KeyBotOwner)
	unsetEnv(t, KeyMongoURI)
	unsetEnv(t, KeyMongoDB)
	unsetEnv(t, KeyLogLevel)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected dotenv-backed config to load, got error: %v", err)
	}

	if cfg.AppEnv != EnvDevelopment {
		t.Fatalf("expected development env from dotenv, got %s", cfg.AppEnv)
	}
	if cfg.TelegramToken != "dotenv-token" {
		t.Fatalf("expected token from dotenv, got %s", cfg.TelegramToken)
	}
	if cfg.BotOwnerID != 77 {
		t.Fatalf("expected owner id 77 from dotenv, got %d", cfg.BotOwnerID)
	}
	if cfg.MongoURI != "mongodb://from-dotenv" {
		t.Fatalf("expected mongo uri from dotenv, got %s", cfg.MongoURI)
	}
	if cfg.MongoDB != "telegram_bot_dev" {
		t.Fatalf("expected mongo db from dotenv, got %s", cfg.MongoDB)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected log level from dotenv, got %s", cfg.LogLevel)
	}
}

func TestLoadValidatesMongoURIFormat(t *testing.T) {
	unsetEnv(t, KeyAppEnv)

	t.Setenv(KeyTelegramToken, "token")
	t.Setenv(KeyBotOwner, "123")
	t.Setenv(KeyMongoURI, "http://localhost:27017")
	t.Setenv(KeyMongoDB, "telegram_bot")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected invalid mongo uri to error")
	}
	if !strings.Contains(err.Error(), KeyMongoURI) {
		t.Fatalf("expected error to mention %s, got %v", KeyMongoURI, err)
	}
}

func TestLoadValidatesAppEnv(t *testing.T) {
	t.Setenv(KeyAppEnv, "staging")
	t.Setenv(KeyTelegramToken, "token")
	t.Setenv(KeyBotOwner, "123")
	t.Setenv(KeyMongoURI, "mongodb://localhost:27017")
	t.Setenv(KeyMongoDB, "telegram_bot")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected invalid app env to error")
	}
	if !strings.Contains(err.Error(), KeyAppEnv) {
		t.Fatalf("expected error to mention %s, got %v", KeyAppEnv, err)
	}
}

func TestFormatRedactedMasksSecrets(t *testing.T) {
	cfg := Config{
		AppEnv:        EnvDevelopment,
		TelegramToken: "123456789:secret",
		BotOwnerID:    42,
		MongoURI:      "mongodb://user:pass@localhost:27017",
		MongoDB:       "telegram_bot_dev",
		LogLevel:      "debug",
	}

	out := FormatRedacted(cfg)
	if strings.Contains(out, "secret") || strings.Contains(out, "user:pass") {
		t.Fatalf("expected secrets to be redacted, got %q", out)
	}
	for _, expected := range []string{
		"app_env: development",
		"bot_owner: 42",
		"telegram_token: 1234...redacted",
		"mongo_uri: mongodb://localhost:27017",
		"mongo_db: telegram_bot_dev",
		"log_level: debug",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q in output %q", expected, out)
		}
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	_ = os.Unsetenv(key)
}
