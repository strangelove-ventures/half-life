package telegram

import (
	"gopkg.in/telebot.v3"
)

func OnlyDM() telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			if msg := c.Message(); msg != nil && !msg.Private() {
				return nil
			}
			return next(c)
		}
	}
}
