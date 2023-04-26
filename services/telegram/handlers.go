package telegram

import (
	"bytes"

	"github.com/kyokomi/emoji/v2"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	"gopkg.in/telebot.v3/middleware"
)

func (t *telegramNotificationService) RegisterHandlers() {
	// Global-scoped middleware:
	t.bot.Use(middleware.AutoRespond())

	t.bot.Handle("/help", t.helpHandler, OnlyDM())
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

func (t *telegramNotificationService) helpHandler(c telebot.Context) error {
	zap.L().Info("User triggered Telegram command",
		zap.String("Command", "/menu"),
		zap.String("Username", c.Chat().Username),
		zap.Int64("UserID", c.Chat().ID),
	)

	return t.showHelp(c)
}

func (t *telegramNotificationService) showHelp(c telebot.Context) error {

	var buffer bytes.Buffer
	buffer.WriteString("Celestia Validators Monitoring - Skating4All community delegate\n\nHere are the list of commands avaliable\n")
	buffer.WriteString(emoji.Sprintf("\nDirect Messages:"))
	buffer.WriteString(emoji.Sprintf("\n==============="))
	buffer.WriteString(emoji.Sprintf("\n:question: help - show help menu"))
	buffer.WriteString(emoji.Sprintf("\n/help"))
	buffer.WriteString(emoji.Sprintf("\n\n:arrow_forward: start - start an account with us"))
	buffer.WriteString(emoji.Sprintf("\n/start"))
	buffer.WriteString(emoji.Sprintf("\n\n:floppy_disk: add - add validator to monitoring list"))
	buffer.WriteString(emoji.Sprintf("\n/add Name Address(like celestiavalcons1XXXXXXX)"))
	buffer.WriteString(emoji.Sprintf("\n\n:wastebasket: remove - remove validator from monitoring list"))
	buffer.WriteString(emoji.Sprintf("\n/remove Address(like celestiavalcons1XXXXXXX)"))
	buffer.WriteString(emoji.Sprintf("\n\n:bookmark_tabs: list - list all my validators registered with user"))
	buffer.WriteString(emoji.Sprintf("\n/list"))
	buffer.WriteString(emoji.Sprintf("\n\n:bar_chart: status - show validator node status"))
	buffer.WriteString(emoji.Sprintf("\n/status Address(like celestiavalcons1XXXXXXX)"))

	t.Send(c.Chat(), buffer.String())
	return t.showMenu(c)
}

func (t *telegramNotificationService) Stop() {
	t.bot.Stop()
}
