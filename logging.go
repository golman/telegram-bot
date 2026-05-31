package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

const (
	defaultLogFilePath   = "logs/telegram-bot.log"
	defaultLogMaxSizeMB  = 20
	defaultLogMaxBackups = 5
	defaultLogMaxAgeDays = 14
	defaultLogCompress   = true
)

type fileLogConfig struct {
	filePath   string
	maxSizeMB  int
	maxBackups int
	maxAgeDays int
	compress   bool
}

func setupLogging() error {
	cfg := loadLogConfigFromEnv()

	if err := os.MkdirAll(filepath.Dir(cfg.filePath), 0o755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}

	rotatingLog := &lumberjack.Logger{
		Filename:   cfg.filePath,
		MaxSize:    cfg.maxSizeMB,
		MaxBackups: cfg.maxBackups,
		MaxAge:     cfg.maxAgeDays,
		Compress:   cfg.compress,
	}

	log.SetOutput(io.MultiWriter(os.Stdout, rotatingLog))
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.LUTC)
	log.Printf("logging configured: file=%s max_size_mb=%d max_backups=%d max_age_days=%d compress=%t",
		cfg.filePath, cfg.maxSizeMB, cfg.maxBackups, cfg.maxAgeDays, cfg.compress)

	return nil
}

func loadLogConfigFromEnv() fileLogConfig {
	return fileLogConfig{
		filePath:   getEnvOrDefault("LOG_FILE_PATH", defaultLogFilePath),
		maxSizeMB:  getEnvIntOrDefault("LOG_MAX_SIZE_MB", defaultLogMaxSizeMB),
		maxBackups: getEnvIntOrDefault("LOG_MAX_BACKUPS", defaultLogMaxBackups),
		maxAgeDays: getEnvIntOrDefault("LOG_MAX_AGE_DAYS", defaultLogMaxAgeDays),
		compress:   getEnvBoolOrDefault("LOG_COMPRESS", defaultLogCompress),
	}
}

func getEnvOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvIntOrDefault(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		log.Printf("invalid %s=%q, using default %d", key, raw, fallback)
		return fallback
	}
	return value
}

func getEnvBoolOrDefault(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		log.Printf("invalid %s=%q, using default %t", key, raw, fallback)
		return fallback
	}
	return value
}

func getEnvInt64OrDefault(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		log.Printf("invalid %s=%q, using default %d", key, raw, fallback)
		return fallback
	}
	return value
}
