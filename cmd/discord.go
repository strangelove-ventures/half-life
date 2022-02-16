package cmd

import (
	"fmt"
	"strings"
	"sync"

	"github.com/DisgoOrg/disgo/discord"
	"github.com/DisgoOrg/disgo/webhook"
	"github.com/DisgoOrg/snowflake"
)

func getCurrentStatsEmbed(stats ValidatorStats, vm *ValidatorMonitor) discord.Embed {
	var color int
	if stats.Height == stats.LastSignedBlockHeight {
		if stats.RecentMissedBlocks == 0 && stats.SlashingPeriodUptime > 75 {
			color = 0x00FF00
		} else {
			color = 0xFFAC1C
		}

		return discord.Embed{
			Title: fmt.Sprintf("%s (%.02f%% up)", vm.Name, stats.SlashingPeriodUptime),
			Description: fmt.Sprintf("Latest Timestamp: %s\nLatest Height: %d\nMost Recent Signed Blocks: %d/%d",
				stats.Timestamp, stats.Height, recentBlocksToCheck-stats.RecentMissedBlocks, recentBlocksToCheck),
			Color: color,
		}
	}

	if stats.RecentMissedBlocks < recentBlocksToCheck && stats.SlashingPeriodUptime > 75 {
		if stats.RecentMissedBlocks > recentMissedBlocksNotifyThreshold {
			color = 0xFFAC1C
		} else {
			color = 0x00FF00
		}
	} else {
		color = 0xFF0000
	}

	return discord.Embed{
		Title: fmt.Sprintf("%s (%.02f%% up)", vm.Name, stats.SlashingPeriodUptime),
		Description: fmt.Sprintf("Latest Timestamp: %s\nLatest Height: %d\nLast Signed Height: %d\nLast Signed Timestamp: %s\nMost Recent Signed Blocks: %d/%d",
			stats.Timestamp, stats.Height, stats.LastSignedBlockHeight, stats.LastSignedBlockTimestamp, recentBlocksToCheck-stats.RecentMissedBlocks, recentBlocksToCheck),
		Color: color,
	}
}

func sendDiscordAlert(
	vm *ValidatorMonitor,
	stats ValidatorStats,
	alertState *map[string]*ValidatorAlertState,
	discordClient *webhook.Client,
	config *HalfLifeConfig,
	errs []error,
	writeConfigMutex *sync.Mutex,
) {
	if (*alertState)[vm.Name] == nil {
		(*alertState)[vm.Name] = &ValidatorAlertState{
			AlertTypeCounts: make(map[int8]int64),
		}
	}
	var foundAlertTypes []int8
	alertString := ""
	alertLevel := int8(0)
	clearedAlertsString := ""

	for _, err := range errs {
		switch err.(type) {
		case *JailedError:
			foundAlertTypes = append(foundAlertTypes, alertTypeJailed)
			if (*alertState)[vm.Name].AlertTypeCounts[alertTypeJailed]%notifyEvery == 0 {
				alertString += "• " + err.Error() + "\n"
				if alertLevel < alertLevelHigh {
					alertLevel = alertLevelHigh
				}
			}
			(*alertState)[vm.Name].AlertTypeCounts[alertTypeJailed]++
		case *TombstonedError:
			foundAlertTypes = append(foundAlertTypes, alertTypeTombstoned)
			if (*alertState)[vm.Name].AlertTypeCounts[alertTypeTombstoned]%notifyEvery == 0 {
				alertString += "• " + err.Error() + "\n"
				if alertLevel < alertLevelCritical {
					alertLevel = alertLevelCritical
				}
			}
			(*alertState)[vm.Name].AlertTypeCounts[alertTypeTombstoned]++
		case *OutOfSyncError:
			foundAlertTypes = append(foundAlertTypes, alertTypeOutOfSync)
			if (*alertState)[vm.Name].AlertTypeCounts[alertTypeOutOfSync]%notifyEvery == 0 {
				alertString += "• " + err.Error() + "\n"
				if alertLevel < alertLevelWarning {
					alertLevel = alertLevelWarning
				}
			}
			(*alertState)[vm.Name].AlertTypeCounts[alertTypeOutOfSync]++
		case *BlockFetchError:
			foundAlertTypes = append(foundAlertTypes, alertTypeBlockFetch)
			if (*alertState)[vm.Name].AlertTypeCounts[alertTypeBlockFetch]%notifyEvery == 0 {
				alertString += "• " + err.Error() + "\n"
				if alertLevel < alertLevelWarning {
					alertLevel = alertLevelWarning
				}
			}
			(*alertState)[vm.Name].AlertTypeCounts[alertTypeBlockFetch]++
		case *MissedRecentBlocksError:
			foundAlertTypes = append(foundAlertTypes, alertTypeMissedRecentBlocks)
			if (*alertState)[vm.Name].AlertTypeCounts[alertTypeMissedRecentBlocks]%notifyEvery == 0 || stats.RecentMissedBlocks != (*alertState)[vm.Name].RecentMissedBlocksCounter {
				alertString += "• " + err.Error() + "\n"
				if stats.RecentMissedBlocks > (*alertState)[vm.Name].RecentMissedBlocksCounter {
					if stats.RecentMissedBlocks > recentMissedBlocksNotifyThreshold {
						if alertLevel < alertLevelHigh {
							alertLevel = alertLevelHigh
						}
					} else {
						if alertLevel < alertLevelWarning {
							alertLevel = alertLevelWarning
						}
					}
				} else {
					if alertLevel < alertLevelWarning {
						alertLevel = alertLevelWarning
					}
				}
			}
			(*alertState)[vm.Name].RecentMissedBlocksCounter = stats.RecentMissedBlocks
			if stats.RecentMissedBlocks > (*alertState)[vm.Name].RecentMissedBlocksCounterMax {
				(*alertState)[vm.Name].RecentMissedBlocksCounterMax = stats.RecentMissedBlocks
			}
			(*alertState)[vm.Name].AlertTypeCounts[alertTypeMissedRecentBlocks]++
		default:
			alertString += "• " + err.Error() + "\n"
			if alertLevel < alertLevelWarning {
				alertLevel = alertLevelWarning
			}
		}
	}

	notifyForClear := false
	// iterate through all error types
	for i := alertTypeJailed; i <= alertTypeMissedRecentBlocks; i++ {
		alertTypeFound := false
		for _, alertType := range foundAlertTypes {
			if i == alertType {
				alertTypeFound = true
				break
			}
		}
		// reset alert type if we didn't see it this time
		if !alertTypeFound && (*alertState)[vm.Name].AlertTypeCounts[i] > 0 {
			(*alertState)[vm.Name].AlertTypeCounts[i] = 0
			switch i {
			case alertTypeJailed:
				clearedAlertsString += "• jailed\n"
				notifyForClear = true
			case alertTypeTombstoned:
				clearedAlertsString += "• tombstoned\n"
				notifyForClear = true
			case alertTypeOutOfSync:
				clearedAlertsString += "• out of sync\n"
			case alertTypeBlockFetch:
				clearedAlertsString += "• block fetch error\n"
			case alertTypeMissedRecentBlocks:
				clearedAlertsString += "• missed recent blocks\n"
				if (*alertState)[vm.Name].RecentMissedBlocksCounterMax > recentMissedBlocksNotifyThreshold {
					notifyForClear = true
				}
				(*alertState)[vm.Name].RecentMissedBlocksCounter = 0
				(*alertState)[vm.Name].RecentMissedBlocksCounterMax = 0
			default:
			}
		}
	}
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

	if alertString != "" {
		var alertColor int
		toNotify := strings.Trim(tagUser, " ")
		switch alertLevel {
		case alertLevelWarning:
			alertColor = 0xFFAC1C
			toNotify = ""
		case alertLevelCritical:
			alertColor = 0x964B00
		case alertLevelHigh:
			fallthrough
		default:
			alertColor = 0xFF0000
		}
		_, err := discordClient.CreateMessage(discord.WebhookMessageCreate{
			Username: config.Discord.Username,
			Content:  toNotify,
			Embeds: []discord.Embed{
				discord.Embed{
					Title:       embedTitle,
					Description: fmt.Sprintf("Errors:\n%s", strings.Trim(alertString, "\n")),
					Color:       alertColor,
				},
			},
		})
		if err != nil {
			fmt.Printf("Error sending discord message: %v\n", err)
		}
	}
	if clearedAlertsString != "" {
		toNotify := ""
		if notifyForClear {
			toNotify = strings.Trim(tagUser, " ")
		}
		_, err := discordClient.CreateMessage(discord.WebhookMessageCreate{
			Username: config.Discord.Username,
			Content:  toNotify,
			Embeds: []discord.Embed{
				discord.Embed{
					Title:       embedTitle,
					Description: fmt.Sprintf("Errors cleared:\n%s", strings.Trim(clearedAlertsString, "\n")),
					Color:       0x00ff00,
				},
			},
		})
		if err != nil {
			fmt.Printf("Error sending discord message: %v\n", err)
		}
	}
	if vm.DiscordStatusMessageID != nil {
		_, err := discordClient.UpdateMessage(snowflake.Snowflake(*vm.DiscordStatusMessageID), discord.WebhookMessageUpdate{
			Embeds: &[]discord.Embed{
				getCurrentStatsEmbed(stats, vm),
			},
		})
		if err != nil {
			fmt.Printf("Error updating discord message: %v\n", err)
		}
	} else {
		message, err := discordClient.CreateMessage(discord.WebhookMessageCreate{
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
