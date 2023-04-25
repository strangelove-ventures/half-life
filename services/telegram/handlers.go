package telegram

import (
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	"gopkg.in/telebot.v3/middleware"
)

func (t *telegramNotificationService) RegisterHandlers() {
	// Global-scoped middleware:
	t.bot.Use(middleware.AutoRespond())

	t.bot.Handle("/add", t.addValidatorHandler, OnlyDM())
	t.bot.Handle("/remove", t.removeValidatorHandler, OnlyDM())
	t.bot.Handle("/list", t.listValidatorHandler, OnlyDM())
	t.bot.Handle("/status", t.statusHandler, OnlyDM())
	t.bot.Handle("/menu", t.menuHandler, OnlyDM())

	t.bot.Handle(telebot.OnText, t.handleText, OnlyDM())

	t.buttonHandler()

	zap.L().Info("Starting Telegram Controller", zap.String("user", t.bot.Me.Username))

	go t.bot.Start()
}

func (t *telegramNotificationService) handleText(_ telebot.Context) error {
	// nothing here...
	return nil
}

func (t *telegramNotificationService) Stop() {
	t.bot.Stop()
}
