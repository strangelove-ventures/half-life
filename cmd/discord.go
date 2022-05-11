package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DisgoOrg/disgo/discord"
	"github.com/DisgoOrg/disgo/rest"
	"github.com/DisgoOrg/disgo/webhook"
	"github.com/DisgoOrg/snowflake"
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

type DiscordNotificationService struct {
	webhookID    string
	webhookToken string
	postMutex    *sync.Mutex
}

func formattedTime(t time.Time) string {
	return fmt.Sprintf("<t:%d:R>", t.Unix())
}

func NewDiscordNotificationService(webhookID, webhookToken string) *DiscordNotificationService {
	return &DiscordNotificationService{
		webhookID:    webhookID,
		webhookToken: webhookToken,
		postMutex:    &sync.Mutex{},
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
	var uptime string
	if stats.SlashingPeriodUptime == 0 {
		uptime = "N/A"
	} else {
		uptime = fmt.Sprintf("%.02f", stats.SlashingPeriodUptime)
	}

	title := fmt.Sprintf("%s (%s%% up)", vm.Name, uptime)

	var description string
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

					var height string
					if sentryStats.Height == 0 {
						height = "N/A"
					} else {
						height = fmt.Sprint(sentryStats.Height)
					}
					var version string
					if sentryStats.Version == "" {
						version = "N/A"
					} else {
						version = sentryStats.Version
					}

					sentryString += fmt.Sprintf("\n%s **%s** - Height **%s** - Version **%s**", statusIcon, sentryStats.Name, height, version)
					sentryFound = true
					break
				}
			}
			if !sentryFound {
				sentryString += fmt.Sprintf("\n%s **%s** - Height **N/A** - Version **N/A**", iconError, vmSentry.Name)
			}
		}
	}

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
			if stats.RecentMissedBlockAlertLevel >= alertLevelHigh {
				recentSignedBlocksIcon = iconError
			} else if stats.RecentMissedBlockAlertLevel == alertLevelWarning {
				recentSignedBlocksIcon = iconWarning
			} else {
				recentSignedBlocksIcon = iconGood
			}
			recentSignedBlocks = fmt.Sprintf("%s Latest Blocks Signed: **%d/%d**", recentSignedBlocksIcon, recentBlocksToCheck-stats.RecentMissedBlocks, recentBlocksToCheck)

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

	color := getColorForAlertLevel(stats.AlertLevel)

	return discord.Embed{
		Title:       title,
		Description: description,
		Color:       color,
	}
}

func (service *DiscordNotificationService) client() *webhook.Client {
	return webhook.NewClient(snowflake.Snowflake(service.webhookID), service.webhookToken)
}

// implements NotificationService interface
func (service *DiscordNotificationService) UpdateValidatorRealtimeStatus(
	configFile string,
	config *HalfLifeConfig,
	vm *ValidatorMonitor,
	stats ValidatorStats,
	writeConfigMutex *sync.Mutex,
) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*4))
	defer cancel()
	client := service.client()
	defer client.Close(ctx)
	if vm.DiscordStatusMessageID != nil {
		service.postMutex.Lock()
		_, err := client.UpdateMessage(snowflake.Snowflake(*vm.DiscordStatusMessageID), discord.WebhookMessageUpdate{
			Embeds: &[]discord.Embed{
				getCurrentStatsEmbed(stats, vm),
			},
		}, rest.WithCtx(ctx))
		service.postMutex.Unlock()
		if err != nil {
			fmt.Printf("Error updating discord message: %v\n", err)
			return
		}
	} else {
		service.postMutex.Lock()
		message, err := client.CreateMessage(discord.WebhookMessageCreate{
			Username: config.Notifications.Discord.Username,
			Embeds: []discord.Embed{
				getCurrentStatsEmbed(stats, vm),
			},
		}, rest.WithCtx(ctx))
		service.postMutex.Unlock()
		if err != nil {
			fmt.Printf("Error sending discord message: %v\n", err)
			return
		}
		messageID := string(message.ID)
		vm.DiscordStatusMessageID = &messageID
		fmt.Printf("Saved message ID: %s\n", messageID)
		saveConfig(configFile, config, writeConfigMutex)
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
	for _, userID := range config.Notifications.Discord.AlertUserIDs {
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*4))
		defer cancel()
		client := service.client()
		defer client.Close(ctx)
		service.postMutex.Lock()
		_, err := client.CreateMessage(discord.WebhookMessageCreate{
			Username: config.Notifications.Discord.Username,
			Content:  toNotify,
			Embeds: []discord.Embed{
				discord.Embed{
					Title:       embedTitle,
					Description: fmt.Sprintf("**Errors:**\n%s", strings.Trim(alertString, "\n")),
					Color:       alertColor,
				},
			},
		}, rest.WithCtx(ctx))
		service.postMutex.Unlock()
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*4))
		defer cancel()
		client := service.client()
		defer client.Close(ctx)
		service.postMutex.Lock()
		_, err := client.CreateMessage(discord.WebhookMessageCreate{
			Username: config.Notifications.Discord.Username,
			Content:  toNotify,
			Embeds: []discord.Embed{
				discord.Embed{
					Title:       embedTitle,
					Description: fmt.Sprintf("**Errors cleared:**\n%s", strings.Trim(clearedAlertsString, "\n")),
					Color:       colorGood,
				},
			},
		}, rest.WithCtx(ctx))
		service.postMutex.Unlock()
		if err != nil {
			fmt.Printf("Error sending discord message: %v\n", err)
		}
	}
}
