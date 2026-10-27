package logging

import "log/slog"

// Доступ внешних тестов (package logging_test) к неэкспортированному:
// тесты в самом package logging не могут импортировать testutil — тот
// импортирует logging, и получился бы цикл.

type TelegoLogger = telegoLogger

var NewTelegoLogger = newTelegoLogger

// ResetLevel возвращает уровень логов к info после теста Setup с Debug.
func ResetLevel() { logLevel.Set(slog.LevelInfo) }
