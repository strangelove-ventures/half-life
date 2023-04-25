package telegram

import (
	"fmt"
	"strings"
	"time"

	"github.com/staking4all/celestia-monitoring-bot/services"
	"github.com/staking4all/celestia-monitoring-bot/services/models"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

const (
	colorGood     = 0x00FF00
	colorWarning  = 0xFFAC1C
	colorError    = 0xFF0000
	colorCritical = 0x964B00

	iconGood    = "🟢" // green circle
	iconWarning = "🟡" // yellow circle
	iconError   = "🔴" // red circle
)

type telegramNotificationService struct {
	config models.Config
	bot    *telebot.Bot

	mm services.MonitorManager
}

func NewTelegramNotificationService(config models.Config) (services.NotificationService, error) {
	pref := telebot.Settings{
		Token:     config.Notifications.Telegram.APIToken,
		Poller:    &telebot.LongPoller{Timeout: 10 * time.Second},
		ParseMode: telebot.ModeHTML,
	}

	bot, err := telebot.NewBot(pref)
	if err != nil {
		zap.L().Error("initing telegram bot", zap.Error(err))
		return nil, err
	}

	t := &telegramNotificationService{
		config: config,
		bot:    bot,
	}

	t.RegisterHandlers()

	return t, nil
}

func (t *telegramNotificationService) SetMonitoManager(mm services.MonitorManager) {
	t.mm = mm
}

func formattedTime(t time.Time) string {
	return t.Format(time.UnixDate)
}

func getColorForAlertLevel(alertLevel models.AlertLevel) (int, string) {
	switch alertLevel {
	case models.AlertLevelNone:
		return colorGood, iconGood
	case models.AlertLevelWarning:
		return colorWarning, iconWarning
	case models.AlertLevelCritical:
		return colorCritical, iconError
	default:
		return colorError, iconError
	}
}

func (t *telegramNotificationService) GetCurrentStatsEmbed(stats models.ValidatorStatsRegister) string {
	var uptime string
	var title string

	if stats.SlashingPeriodUptime == 0 {
		uptime = "N/A"
	} else {
		uptime = fmt.Sprintf("%.02f", stats.SlashingPeriodUptime)
	}

	title = fmt.Sprintf("%s (%s%% up)", stats.Validator.Name, uptime)

	var description string
	sentryString := ""

	recentSignedBlocks := fmt.Sprintf("%s Latest Blocks Signed: **N/A**", iconWarning)

	var latestBlock string
	if stats.Timestamp.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		latestBlock = fmt.Sprintf("%s Height **N/A**", iconError)
	} else {
		var rpcStatusIcon string
		if stats.RPCError {
			rpcStatusIcon = iconError
		} else {
			rpcStatusIcon = iconGood

			var recentSignedBlocksIcon string
			switch level := stats.RecentMissedBlockAlertLevel; {
			case level >= models.AlertLevelHigh:
				recentSignedBlocksIcon = iconError
			case level == models.AlertLevelWarning:
				recentSignedBlocksIcon = iconWarning
			default:
				recentSignedBlocksIcon = iconGood
			}
			recentSignedBlocks = fmt.Sprintf("%s Latest Blocks Signed: **%d/%d**", recentSignedBlocksIcon, t.config.ValidatorsMonitor.RecentBlocksToCheck-stats.RecentMissedBlocks, t.config.ValidatorsMonitor.RecentBlocksToCheck)
		}
		latestBlock = fmt.Sprintf("%s Height **%s** - **%s**", rpcStatusIcon, fmt.Sprint(stats.Height), formattedTime(stats.Timestamp))
	}

	if stats.Height == stats.LastSignedBlockHeight {
		description = fmt.Sprintf("%s\n%s%s",
			latestBlock, recentSignedBlocks, sentryString)
	} else {
		var lastSignedBlock string
		if stats.LastSignedBlockHeight == -1 {
			lastSignedBlock = fmt.Sprintf("%s Last Signed **N/A**", iconError)
		} else {
			lastSignedBlock = fmt.Sprintf("%s Last Signed **%s** - **%s**", iconError, fmt.Sprint(stats.LastSignedBlockHeight), formattedTime(stats.LastSignedBlockTimestamp))
		}
		description = fmt.Sprintf("%s\n%s\n%s%s",
			latestBlock, lastSignedBlock, recentSignedBlocks, sentryString)
	}

	color, icon := getColorForAlertLevel(stats.AlertLevel)
	_ = color

	return fmt.Sprintf("%s *%s* %s\n\n\n\n%s", icon, title, icon, description)
}

func (t *telegramNotificationService) PushMessage(userID int64, chatType telebot.ChatType, mdText string) (*telebot.Message, error) {
	return t.Send(&telebot.Chat{
		ID:   userID,
		Type: chatType,
	}, mdText)
}

func (t *telegramNotificationService) Send(chat *telebot.Chat, mdText string) (*telebot.Message, error) {
	zap.L().Debug("message", zap.String("message", mdText))
	return t.bot.Send(chat, mdText, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
}

// implements NotificationService interface
func (t *telegramNotificationService) SendValidatorAlertNotification(
	userID int64,
	vm *models.Validator,
	stats models.ValidatorStats,
	alertNotification *models.ValidatorAlertNotification,
) {
	zap.L().Debug("notifying", zap.Int64("userID", userID))
	var embedTitle string

	if stats.SlashingPeriodUptime > 0 {
		embedTitle = fmt.Sprintf("%s `(%.02f%% up)`", vm.Name, stats.SlashingPeriodUptime)
	} else {
		embedTitle = fmt.Sprintf("%s `(N/A%% up)`", vm.Name)
	}

	if len(alertNotification.Alerts) > 0 {
		alertString := ""
		for _, alert := range alertNotification.Alerts {
			alertString += fmt.Sprintf("\n• %s", alert)
		}
		_, icon := getColorForAlertLevel(alertNotification.AlertLevel)
		_, _ = t.PushMessage(userID, telebot.ChatPrivate, fmt.Sprintf("%s *%s*\n\n\n**Errors:**\n%s", icon, embedTitle, strings.Trim(alertString, "\n")))
	}

	if len(alertNotification.ClearedAlerts) > 0 {
		clearedAlertsString := ""
		for _, alert := range alertNotification.ClearedAlerts {
			clearedAlertsString += fmt.Sprintf("\n• %s", alert)
		}

		_, _ = t.PushMessage(userID, telebot.ChatPrivate, fmt.Sprintf("%s *%s*\n\n\n**Errors cleared:**\n%s", iconGood, embedTitle, strings.Trim(clearedAlertsString, "\n")))
	}
}

func (t *telegramNotificationService) List(stats []models.ValidatorStatsRegister) (result string) {
	result = "*Validator Monitor List:*\n"

	for _, vs := range stats {
		var uptime string
		var title string
		if vs.ValidatorStats.SlashingPeriodUptime == 0 {
			uptime = "N/A"
		} else {
			uptime = fmt.Sprintf("%.02f", vs.ValidatorStats.SlashingPeriodUptime)
		}

		title = fmt.Sprintf("%s (%s%% up)", vs.Validator.Name, uptime)

		_, icon := getColorForAlertLevel(vs.ValidatorStats.AlertLevel)

		result += fmt.Sprintf("  - %s *%s* %s\n", icon, title, icon)
	}

	return
}
