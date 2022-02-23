package cmd

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v2"
)

const (
	configFilePath                    = "./config.yaml"
	recentBlocksToCheck               = 20
	notifyEvery                       = 20 // check runs every ~30 seconds, so will notify for continued errors and rollup stats every ~10 mins
	recentMissedBlocksNotifyThreshold = 10

	alertLevelWarning  = int8(1)
	alertLevelHigh     = int8(2)
	alertLevelCritical = int8(3)

	alertTypeJailed             = int8(1)
	alertTypeTombstoned         = int8(2)
	alertTypeOutOfSync          = int8(3)
	alertTypeBlockFetch         = int8(4)
	alertTypeMissedRecentBlocks = int8(5)
	alertTypeGenericRPC         = int8(6)
)

type SentryStats struct {
	Name    string
	Version string
	Height  int64
}

type ValidatorStats struct {
	Timestamp                string
	Height                   int64
	RecentMissedBlocks       int64
	LastSignedBlockHeight    int64
	LastSignedBlockTimestamp string
	SlashingPeriodUptime     float64
	SentryStats              []SentryStats
}

type ValidatorAlertState struct {
	AlertTypeCounts              map[int8]int64
	SentryGRPCErrorCounts        map[string]int64
	SentryOutOfSyncErrorCounts   map[string]int64
	RecentMissedBlocksCounter    int64
	RecentMissedBlocksCounterMax int64
}

type HalfLifeConfig struct {
	Discord    *DiscordChannelConfig
	Validators []*ValidatorMonitor `yaml:"validators"`
}

type DiscordWebhookConfig struct {
	ID    string `yaml:"id"`
	Token string `yaml:"token"`
}

type DiscordChannelConfig struct {
	Webhook      DiscordWebhookConfig `yaml:"webhook"`
	AlertUserIDs []string             `yaml:"alert-user-ids"`
	Username     string               `yaml:"username"`
}

type Sentry struct {
	Name string `yaml:"name"`
	GRPC string `yaml:"grpc"`
}

type ValidatorMonitor struct {
	Name                   string    `yaml:"name"`
	RPC                    string    `yaml:"rpc"`
	Address                string    `yaml:"address"`
	ChainID                string    `yaml:"chain-id"`
	DiscordStatusMessageID *string   `yaml:"discord-status-message-id"`
	RPCRetries             *int      `yaml:"rpc-retries"`
	Sentries               *[]Sentry `yaml:"sentries"`
}

func saveConfig(config *HalfLifeConfig, writeConfigMutex *sync.Mutex) {
	writeConfigMutex.Lock()
	defer writeConfigMutex.Unlock()

	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		fmt.Printf("Error during config yaml marshal %v\n", err)
	}

	err = os.WriteFile(configFilePath, yamlBytes, 0644)
	if err != nil {
		fmt.Printf("Error saving config yaml %v\n", err)
	}
}
