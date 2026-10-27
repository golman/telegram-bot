// Package testutil содержит хелперы, общие для тестов всех пакетов.
package testutil

import (
	"bytes"
	"log"
	"log/slog"
	"testing"

	"github.com/golman/telegram-bot/internal/logging"
)

// CaptureLog перенаправляет логи slog в буфер до конца теста. Формат тот же,
// что в проде (key=value, время в UTC), уровень debug. После теста
// восстанавливаются slog.Default и вывод пакета log.
func CaptureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	// slog.SetDefault перенаправляет и пакет log; его вывод восстанавливается отдельно.
	prevOut, prevFlags := log.Writer(), log.Flags()
	slog.SetDefault(slog.New(logging.NewHandler(&buf, slog.LevelDebug)))
	t.Cleanup(func() {
		slog.SetDefault(prev)
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	return &buf
}
