package telegram

import (
	"fmt"

	"github.com/staking4all/celestia-monitoring-bot/services/models"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

func (t *telegramNotificationService) addValidatorHandler(c telebot.Context) error {
	if len(c.Args()) != 2 {
		_, _ = t.Send(c.Chat(), "Please try `/add ValidatorName celestiavalcons1XXXXXXX`")
		return nil
	}

	zap.L().Info("add validator", zap.Int64("userID", c.Sender().ID), zap.String("userName", c.Sender().Username), zap.String("name", c.Args()[0]), zap.String("address", c.Args()[1]))
	err := t.mm.Add(c.Sender().ID, models.NewValidator(c.Args()[0], c.Args()[1]))
	if err != nil {
		zap.L().Warn("addHandler", zap.Int64("userID", c.Sender().ID), zap.String("userName", c.Sender().Username), zap.String("name", c.Args()[0]), zap.String("address", c.Args()[1]), zap.Error(err))
		_, _ = t.Send(c.Chat(), err.Error())
		return err
	}

	_, _ = t.Send(c.Chat(), fmt.Sprintf("validator added to monitor list *%s*", c.Args()[1]))

	return nil
}

func (t *telegramNotificationService) removeValidatorHandler(c telebot.Context) error {
	if len(c.Args()) != 1 {
		// TODO: list validators regitered to user
		_, _ = t.Send(c.Chat(), "Please try `/remove celestiavalcons1XXXXXXX`")
		return nil
	}

	zap.L().Info("remove validator", zap.Int64("userID", c.Sender().ID), zap.String("userName", c.Sender().Username), zap.String("address", c.Args()[0]))
	err := t.mm.Remove(c.Sender().ID, c.Args()[0])
	if err != nil {
		zap.L().Warn("removeHandler", zap.Int64("userID", c.Sender().ID), zap.String("userName", c.Sender().Username), zap.String("address", c.Args()[0]), zap.Error(err))
		_, _ = t.Send(c.Chat(), err.Error())
		return err
	}

	_, _ = t.Send(c.Chat(), fmt.Sprintf("validator removed from monitor list *%s*", c.Args()[0]))

	return nil
}

func (t *telegramNotificationService) statusHandler(c telebot.Context) error {
	if len(c.Args()) != 1 {
		// TODO: list validators regitered to user
		_, _ = t.Send(c.Chat(), "Please try `/status celestiavalcons1XXXXXXX`")
		return nil
	}

	zap.L().Info("status validator", zap.Int64("userID", c.Sender().ID), zap.String("userName", c.Sender().Username), zap.String("address", c.Args()[0]))
	stats, err := t.mm.GetCurrentState(c.Sender().ID, c.Args()[0])
	if err != nil {
		zap.L().Warn("statusHandler", zap.Int64("userID", c.Sender().ID), zap.String("userName", c.Sender().Username), zap.String("address", c.Args()[0]), zap.Error(err))
		_, _ = t.Send(c.Chat(), err.Error())
		return err
	}

	result := t.GetCurrentStatsEmbed(stats)

	_, _ = t.Send(c.Chat(), result)

	return nil
}

func (t *telegramNotificationService) listValidatorHandler(c telebot.Context) error {
	list, err := t.mm.List(c.Sender().ID)
	if err != nil {
		zap.L().Warn("listValidator", zap.Int64("userID", c.Sender().ID), zap.String("userName", c.Sender().Username), zap.String("address", c.Args()[0]), zap.Error(err))
		_, _ = t.Send(c.Chat(), err.Error())
		return err
	}

	_, _ = t.Send(c.Chat(), t.List(list))
	if len(list) == 0 {
		_, _ = t.Send(c.Chat(), "*Empty List*")
	}

	return nil
}
