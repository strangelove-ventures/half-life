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
		_ = t.addValidatorHandler(c)
		return t.menuHandler(c)
	})

	t.bot.Handle(&telebot.InlineButton{
		Unique: "rm_menu",
	}, func(c telebot.Context) error {
		_ = t.bot.Delete(c.Message())
		_ = t.removeValidatorHandler(c)
		return t.menuHandler(c)
	})

	t.bot.Handle(&telebot.InlineButton{
		Unique: "list_menu",
	}, func(c telebot.Context) error {
		_ = t.bot.Delete(c.Message())
		_ = t.listValidatorHandler(c)
		return t.menuHandler(c)
	})

	t.bot.Handle(&telebot.InlineButton{
		Unique: "my_status",
	}, func(c telebot.Context) error {
		_ = t.bot.Delete(c.Message())
		_ = t.statusHandler(c)
		return t.menuHandler(c)
	})
}

func (t *telegramNotificationService) menuHandler(c telebot.Context) error {
	zap.L().Info("User triggered Telegram command",
		zap.String("Command", "/menu"),
		zap.String("Username", c.Chat().Username),
		zap.Int64("UserID", c.Chat().ID),
	)

	return t.showMenu(c)
}

func (t *telegramNotificationService) showMenu(c telebot.Context) error {
	var buffer bytes.Buffer
	buffer.WriteString("Please select an option")
	addBtn := telebot.InlineButton{
		Unique: "add_menu",
		Text:   "\U0001f4be Add",
	}
	rmBtn := telebot.InlineButton{
		Unique: "rm_menu",
		Text:   "\U0001F5D1 Remove",
	}
	listBtn := telebot.InlineButton{
		Unique: "list_menu",
		Text:   "\U0001f4d1 List",
		Data:   "",
	}
	statusBtn := telebot.InlineButton{
		Unique: "my_status",
		Text:   "\U0001f4ca Status",
	}

	inlineKeys := [][]telebot.InlineButton{
		{addBtn, rmBtn},
		{listBtn, statusBtn},
	}

	opt := &telebot.ReplyMarkup{
		InlineKeyboard: inlineKeys,
	}

	_, _ = t.bot.Send(c.Chat(), buffer.String(), opt)

	return nil
}
