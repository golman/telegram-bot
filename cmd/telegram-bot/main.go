// Команда telegram-bot запускает бота: читает конфиг из окружения,
// настраивает логи, подключается к Bot API и работает до SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/mymmrac/telego"

	"github.com/golman/telegram-bot/internal/bot"
	"github.com/golman/telegram-bot/internal/config"
	"github.com/golman/telegram-bot/internal/logging"
	"github.com/golman/telegram-bot/internal/tgapi"
)

// run запускает бота и возвращает код выхода процесса: 0 после остановки
// по сигналу, 1 при ошибке запуска или работы. Конфиг читается через getenv,
// логи пишутся в stdout и в файл.
func run(getenv func(string) string, stdout io.Writer) int {
	return runWith(context.Background(), getenv, stdout)
}

// runWith — run, который останавливается и при отмене parent, а клиенту
// telego передаёт ещё и extra. Тесты задают через них фейковый Bot API
// (telego.WithAPIServer) и остановку без сигнала.
func runWith(parent context.Context, getenv func(string) string, stdout io.Writer, extra ...telego.BotOption) int {
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

	opts := append(logging.BotOptions(cfg.Token, cfg.Log.Debug), telego.WithAPICaller(tgapi.NewCaller()))
	opts = append(opts, extra...)
	client, err := telego.NewBot(cfg.Token, opts...)
	if err != nil {
		slog.Error("не удалось создать клиент Bot API", "err", err)
		return 1
	}
	// Неверный или отозванный токен проверяется сразу: в long polling telego
	// повторял бы ошибку 401 каждые 8 секунд, и бот «работал» бы вечно.
	me, err := client.GetMe(context.Background())
	if err != nil {
		slog.Error("Telegram не принял токен бота", "err", err)
		return 1
	}
	slog.Info("бот подключён к Telegram", "username", me.Username)
	// Без прав в канале бот работал бы, но каждый автор получал бы отказ:
	// ошибка в CHANNEL_ID (например, без -100) видна сразу, а не в жалобах.
	if err := checkChannel(client, cfg.ChannelID, me.ID); err != nil {
		slog.Error("бот не может публиковать в канал", "chat_id", cfg.ChannelID, "err", err)
		return 1
	}
	// Пока настроен webhook, getUpdates отвечает 409, и бот работал бы,
	// не получая обновлений. Сам он webhook не снимает: тот может быть
	// нужен другому деплою, решает человек (подсказка в записи и в README).
	webhook, err := client.GetWebhookInfo(context.Background())
	if err != nil {
		slog.Error("не удалось проверить webhook", "err", err)
		return 1
	}
	if webhook.URL != "" {
		slog.Error("у бота настроен webhook, обновления через long polling не придут",
			"webhook_host", webhookHost(webhook.URL), "pending_update_count", webhook.PendingUpdateCount, "fix", webhookFix)
		return 1
	}

	// SIGINT/SIGTERM отменяют ctx: telego прекращает long polling и закрывает
	// канал обновлений, после чего Run дообрабатывает буфер и выходит.
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		// Повторный сигнал снова завершает процесс сразу, если остановка зависла.
		stop()
	}()

	updates, err := client.UpdatesViaLongPolling(ctx, &telego.GetUpdatesParams{
		Timeout: tgapi.LongPollTimeoutSeconds,
		// Явно: без списка Telegram берёт сохранённый с прошлого раза, и если его
		// когда-то сузили, нажатия кнопок (callback_query) перестали бы приходить.
		AllowedUpdates: []string{"message", "callback_query"},
	})
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

// errNoPostRight — бот не администратор канала с правом публикации сообщений.
var errNoPostRight = errors.New("бот не администратор канала с правом «Публикация сообщений»")

// checkChannel проверяет, что бот botID — администратор канала channelID
// с правом публикации. Ошибка API (канал не найден, бот не в канале)
// возвращается как есть.
func checkChannel(client *telego.Bot, channelID, botID int64) error {
	member, err := client.GetChatMember(context.Background(), &telego.GetChatMemberParams{
		ChatID: telego.ChatID{ID: channelID},
		UserID: botID,
	})
	if err != nil {
		return err
	}
	if admin, ok := member.(*telego.ChatMemberAdministrator); ok && admin.CanPostMessages {
		return nil
	}
	return errNoPostRight
}

// webhookFix — подсказка в записи о настроенном webhook. Токена в ней нет:
// команда берёт его из переменной окружения.
const webhookFix = `снимите webhook: curl -s "https://api.telegram.org/bot$BOT_TOKEN/deleteWebhook", подробности — в README, «Проверка и типичные сбои»`

// webhookHost возвращает хост адреса webhook для лога: путь может быть
// секретным, поэтому целиком адрес не пишется.
func webhookHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "?"
	}
	return u.Host
}

func main() {
	os.Exit(run(os.Getenv, os.Stdout))
}
