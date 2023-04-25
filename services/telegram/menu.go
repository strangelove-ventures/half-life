package telegram

import (
	"bytes"

	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

func (t *telegramNotificationService) buttonHandler() {
	t.bot.Handle(&telebot.InlineButton{
		Unique: "show_menu",
	}, func(c telebot.Context) error {
		_ = t.bot.Delete(c.Message())
		return t.showMenu(c)
	})

	t.bot.Handle(&telebot.InlineButton{
		Unique: "add_menu",
	}, func(c telebot.Context) error {
		_ = t.bot.Delete(c.Message())
		return t.addValidatorHandler(c)
	})
}

func (t *telegramNotificationService) menuHandler(c telebot.Context) error {
	zap.L().Info("User triggered Telegram command",
		zap.String("Command", "/menu"),
		zap.String("Username", c.Chat().Username),
		zap.Int64("UserID", c.Chat().ID),
	)

	var buffer bytes.Buffer
	if len(c.Args()) > 0 {
		buffer.WriteString("Invalid parameter! try: /menu")
		_, _ = t.bot.Send(c.Chat(), buffer.String())
		return nil
	}

	return t.showMenu(c)
}

func (t *telegramNotificationService) showMenu(c telebot.Context) error {
	var buffer bytes.Buffer
	buffer.WriteString("Please select an option")
	addBtn := telebot.InlineButton{
		Unique: "add_menu",
		Text:   "\U0001f516 Add",
	}
	listBtn := telebot.InlineButton{
		Unique: "list_menu",
		Text:   "\U0001F4d1 List",
		Data:   "",
	}
	statusBtn := telebot.InlineButton{
		Unique: "my_status",
		Text:   "\U00002699 Status",
	}

	inlineKeys := [][]telebot.InlineButton{
		{listBtn, addBtn},
		{statusBtn},
	}

	opt := &telebot.ReplyMarkup{
		InlineKeyboard: inlineKeys,
	}

	_, _ = t.bot.Send(c.Chat(), buffer.String(), opt)

	return nil
}
