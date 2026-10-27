// Команда telegram-bot запускает бота: читает конфиг из окружения,
// настраивает логи, подключается к Bot API и работает до SIGINT/SIGTERM.
package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	tgbotapi "github.com/mymmrac/telego"

	"github.com/golman/telegram-bot/internal/bot"
	"github.com/golman/telegram-bot/internal/config"
	"github.com/golman/telegram-bot/internal/logging"
	"github.com/golman/telegram-bot/internal/tgapi"
)

// run запускает бота и возвращает код выхода процесса: 0 после остановки
// по сигналу, 1 при ошибке запуска или работы. Конфиг читается через getenv,
// логи пишутся в stdout и в файл.
func run(getenv func(string) string, stdout io.Writer) int {
	logging.Bootstrap(stdout)
	cfg, warnings, loadErr := config.Load(getenv)
	setupErr := logging.Setup(cfg.Log, stdout)
	// Предупреждения пишутся после открытия файла логов, чтобы попасть и туда;
	// при ошибке настройки файла — хотя бы в stdout (туда логи направил
	// logging.Bootstrap).
	for _, w := range warnings {
		slog.Warn("некорректная переменная окружения, используется значение по умолчанию",
			"var", w.Var, "value", w.Value, "default", w.Default)
	}
	if setupErr != nil {
		slog.Error("не удалось настроить логирование", "err", setupErr)
		return 1
	}
	if loadErr != nil { // config.ErrNoToken или config.ErrNoChannel
		slog.Error(loadErr.Error())
		return 1
	}

	opts := append(logging.BotOptions(cfg.Token, cfg.Log.Debug), tgbotapi.WithAPICaller(tgapi.NewCaller()))
	client, err := tgbotapi.NewBot(cfg.Token, opts...)
	if err != nil {
		slog.Error("не удалось создать клиент Bot API", "err", err)
		return 1
	}

	// SIGINT/SIGTERM отменяют ctx: telego прекращает long polling и закрывает
	// канал обновлений, после чего Run дообрабатывает буфер и выходит.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		// Повторный сигнал снова завершает процесс сразу, если остановка зависла.
		stop()
	}()

	updates, err := client.UpdatesViaLongPolling(ctx, &tgbotapi.GetUpdatesParams{Timeout: tgapi.LongPollTimeoutSeconds})
	if err != nil {
		slog.Error("не удалось запустить long polling", "err", err)
		return 1
	}

	if err := bot.New(client, cfg).Run(ctx, updates); err != nil {
		// Обработку сигналов возвращаем Go до записи ошибки, чтобы повторный
		// сигнал сразу завершал процесс.
		stop()
		slog.Error("бот остановился с ошибкой", "err", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Getenv, os.Stdout))
}
