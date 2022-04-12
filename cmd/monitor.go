package cmd

import (
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Daemon to monitor validators",
	Long:  "Monitors validators and pushes alerts to Discord using the configuration in config.yaml",
	Run: func(cmd *cobra.Command, args []string) {
		configFile, _ := cmd.Flags().GetString("file")
		dat, err := os.ReadFile(configFile)
		if err != nil {
			log.Fatalf("Error reading config.yaml: %v", err)
		}
		config := HalfLifeConfig{}
		err = yaml.Unmarshal(dat, &config)
		if err != nil {
			log.Fatalf("Error parsing config.yaml: %v", err)
		}

		if config.Notifications == nil {
			panic("Notifications configuration is not present in config.yaml")
		}

		writeConfigMutex := sync.Mutex{}
		// TODO implement more notification services e.g. slack, email
		var notificationService NotificationService
		switch config.Notifications.Service {
		case "discord":
			if config.Notifications.Discord == nil {
				panic("Discord configuration not present in config.yaml")
			}
			notificationService = NewDiscordNotificationService(config.Notifications.Discord.Webhook.ID, config.Notifications.Discord.Webhook.Token)
		default:
			if config.Notifications.Service == "" {
				panic("Notification service not configured in config.yaml")
			}
			panic(fmt.Sprintf("Notification service not supported: %s", config.Notifications.Service))
		}

		alertState := make(map[string]*ValidatorAlertState)
		for i, vm := range config.Validators {
			alertState[vm.Name] = &ValidatorAlertState{
				AlertTypeCounts:            make(map[AlertType]int64),
				SentryGRPCErrorCounts:      make(map[string]int64),
				SentryOutOfSyncErrorCounts: make(map[string]int64),
				SentryHaltErrorCounts:      make(map[string]int64),
				SentryLatestHeight:         make(map[string]int64),
			}
			alertStateLock := sync.Mutex{}
			if i == len(config.Validators)-1 {
				runMonitor(notificationService, alertState[vm.Name], &alertStateLock, configFile, &config, vm, &writeConfigMutex)
			} else {
				go runMonitor(notificationService, alertState[vm.Name], &alertStateLock, configFile, &config, vm, &writeConfigMutex)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(monitorCmd)
	monitorCmd.Flags().StringP("file", "f", configFilePath, "File path to config yaml")
}
