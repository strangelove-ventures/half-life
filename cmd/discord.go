package cmd

import (
	"fmt"
	"strings"
	"sync"

	"github.com/DisgoOrg/disgo/discord"
	"github.com/DisgoOrg/disgo/webhook"
	"github.com/DisgoOrg/snowflake"
)

const (
	colorGood     = 0x00FF00
	colorWarning  = 0xFFAC1C
	colorError    = 0xFF0000
	colorCritical = 0x964B00

	iconGood  = "🟢" // green circle
	iconError = "🔴" // red circle
)

type DiscordNotificationService struct {
	client *webhook.Client
}

func NewDiscordNotificationService(webhookID, webhookToken string) *DiscordNotificationService {
	return &DiscordNotificationService{
		client: webhook.NewClient(snowflake.Snowflake(webhookID), webhookToken),
	}
}

func getColorForAlertLevel(alertLevel AlertLevel) int {
	switch alertLevel {
	case alertLevelNone:
		return colorGood
	case alertLevelWarning:
		return colorWarning
	case alertLevelCritical:
		return colorCritical
	default:
		return colorError
	}
}

func getCurrentStatsEmbed(stats ValidatorStats, vm *ValidatorMonitor) discord.Embed {
	color := getColorForAlertLevel(stats.AlertLevel)

	var uptime string
	if stats.SlashingPeriodUptime == 0 {
		uptime = "N/A"
	} else {
		uptime = fmt.Sprintf("%.02f%%", stats.SlashingPeriodUptime)
	}

	sentryString := ""
	if vm.Sentries != nil {
		for _, vmSentry := range *vm.Sentries {
			sentryFound := false
			for _, sentryStats := range stats.SentryStats {
				if vmSentry.Name == sentryStats.Name {
					var statusIcon string
					if sentryStats.SentryAlertType == sentryAlertTypeNone {
						statusIcon = iconGood
					} else {
						statusIcon = iconError
					}

					sentryString += fmt.Sprintf("\n%s **%s** - Height **%d** - Version **%s**", statusIcon, sentryStats.Name, sentryStats.Height, sentryStats.Version)
					sentryFound = true
					break
				}
			}
			if !sentryFound {
				sentryString += fmt.Sprintf("\n%s **%s** - Height **N/A** - Version **N/A**", iconError, vmSentry.Name)
			}
		}
	}

	if stats.Height == stats.LastSignedBlockHeight {
		return discord.Embed{
			Title: fmt.Sprintf("%s (%s up)", vm.Name, uptime),
			Description: fmt.Sprintf("Latest Timestamp: **%s**\nLatest Height: **%d**\nMost Recent Signed Blocks: **%d/%d**%s",
				stats.Timestamp, stats.Height, recentBlocksToCheck-stats.RecentMissedBlocks, recentBlocksToCheck, sentryString),
			Color: color,
		}
	}

	return discord.Embed{
		Title: fmt.Sprintf("%s (%s up)", vm.Name, uptime),
		Description: fmt.Sprintf("Latest Timestamp: **%s**\nLatest Height: **%d**\nLast Signed Height: **%d**\nLast Signed Timestamp: **%s**\nMost Recent Signed Blocks: **%d/%d**%s",
			stats.Timestamp, stats.Height, stats.LastSignedBlockHeight, stats.LastSignedBlockTimestamp, recentBlocksToCheck-stats.RecentMissedBlocks, recentBlocksToCheck, sentryString),
		Color: color,
	}
}

// implements NotificationService interface
func (service *DiscordNotificationService) UpdateValidatorRealtimeStatus(
	config *HalfLifeConfig,
	vm *ValidatorMonitor,
	stats ValidatorStats,
	writeConfigMutex *sync.Mutex,
) {
	if vm.DiscordStatusMessageID != nil {
		_, err := service.client.UpdateMessage(snowflake.Snowflake(*vm.DiscordStatusMessageID), discord.WebhookMessageUpdate{
			Embeds: &[]discord.Embed{
				getCurrentStatsEmbed(stats, vm),
			},
		})
		if err != nil {
			fmt.Printf("Error updating discord message: %v\n", err)
		}
	} else {
		message, err := service.client.CreateMessage(discord.WebhookMessageCreate{
			Username: config.Discord.Username,
			Embeds: []discord.Embed{
				getCurrentStatsEmbed(stats, vm),
			},
		})
		if err != nil {
			fmt.Printf("Error sending discord message: %v\n", err)
		}
		messageID := string(message.ID)
		vm.DiscordStatusMessageID = &messageID
		fmt.Printf("Saved message ID: %s\n", messageID)
		saveConfig(config, writeConfigMutex)
	}
}

// implements NotificationService interface
func (service *DiscordNotificationService) SendValidatorAlertNotification(
	config *HalfLifeConfig,
	vm *ValidatorMonitor,
	stats ValidatorStats,
	alertNotification *ValidatorAlertNotification,
) {
	tagUser := ""
	for _, userID := range config.Discord.AlertUserIDs {
		tagUser += fmt.Sprintf("<@%s> ", userID)
	}

	var embedTitle string
	if stats.SlashingPeriodUptime > 0 {
		embedTitle = fmt.Sprintf("%s (%.02f%% up)", vm.Name, stats.SlashingPeriodUptime)
	} else {
		embedTitle = fmt.Sprintf("%s (N/A%% up)", vm.Name)
	}

	if len(alertNotification.Alerts) > 0 {
		alertString := ""
		for _, alert := range alertNotification.Alerts {
			alertString += fmt.Sprintf("\n• %s", alert)
		}
		alertColor := getColorForAlertLevel(alertNotification.AlertLevel)
		toNotify := ""
		if alertNotification.AlertLevel > alertLevelWarning {
			toNotify = strings.Trim(tagUser, " ")
		}
		_, err := service.client.CreateMessage(discord.WebhookMessageCreate{
			Username: config.Discord.Username,
			Content:  toNotify,
			Embeds: []discord.Embed{
				discord.Embed{
					Title:       embedTitle,
					Description: fmt.Sprintf("**Errors:**\n%s", strings.Trim(alertString, "\n")),
					Color:       alertColor,
				},
			},
		})
		if err != nil {
			fmt.Printf("Error sending discord message: %v\n", err)
		}
	}

	if len(alertNotification.ClearedAlerts) > 0 {
		clearedAlertsString := ""
		for _, alert := range alertNotification.ClearedAlerts {
			clearedAlertsString += fmt.Sprintf("\n• %s", alert)
		}
		toNotify := ""
		if alertNotification.NotifyForClear {
			toNotify = strings.Trim(tagUser, " ")
		}
		_, err := service.client.CreateMessage(discord.WebhookMessageCreate{
			Username: config.Discord.Username,
			Content:  toNotify,
			Embeds: []discord.Embed{
				discord.Embed{
					Title:       embedTitle,
					Description: fmt.Sprintf("**Errors cleared:**\n%s", strings.Trim(clearedAlertsString, "\n")),
					Color:       colorGood,
				},
			},
		})
		if err != nil {
			fmt.Printf("Error sending discord message: %v\n", err)
		}
	}
}
