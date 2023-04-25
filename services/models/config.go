package models

const (
	DefaultSlashingPeriodUptimeWarningThreshold float64 = 99.80 // 20 of the last 10,000 blocks missed
	DefaultSlashingPeriodUptimeErrorThreshold   float64 = 98    // 200 of the last 10,000 blocks missed
	DefaultRecentBlocksToCheck                  int64   = 20
	DefaultNotifyEvery                          int64   = 20 // check runs every ~30 seconds, so will notify for continued errors and rollup stats every ~10 mins
	DefaultRecentMissedBlocksNotifyThreshold    int64   = 10

	DefaultSlashingPeriod            = int64(10000)
	DefaultHaltThresholdNanoseconds  = 3e11 // if nodes are stuck for > 5 minutes, will be considered halt
	DefaultMaxNbConcurrentGoroutines = 10
)

type NotificationsConfig struct {
	Telegram *TelegramConfig `yaml:"telegram"`
}

type TelegramConfig struct {
	APIToken  string `yaml:"api_token"`
	MasterIDs []int  `yaml:"master_ids"`
}

type AlertConfig struct {
	IgnoreAlerts []*AlertType `yaml:"ignore_alerts"`
}

func (at *AlertConfig) AlertActive(alert AlertType) bool {
	for _, a := range at.IgnoreAlerts {
		if *a == alert {
			return false
		}
	}
	return true
}

type Config struct {
	AlertConfig       AlertConfig             `yaml:"alerts"`
	Notifications     NotificationsConfig     `yaml:"notifications"`
	ValidatorsMonitor ValidatorsMonitorConfig `yaml:"validators_monitor"`
}

func (c *Config) LoadDefault() {
	if c.ValidatorsMonitor.SlashingPeriodUptimeWarningThreshold == 0 {
		c.ValidatorsMonitor.SlashingPeriodUptimeWarningThreshold = DefaultSlashingPeriodUptimeWarningThreshold
	}
	if c.ValidatorsMonitor.SlashingPeriodUptimeErrorThreshold == 0 {
		c.ValidatorsMonitor.SlashingPeriodUptimeErrorThreshold = DefaultSlashingPeriodUptimeErrorThreshold
	}
	if c.ValidatorsMonitor.RecentBlocksToCheck == 0 {
		c.ValidatorsMonitor.RecentBlocksToCheck = DefaultRecentBlocksToCheck
	}
	if c.ValidatorsMonitor.NotifyEvery == 0 {
		c.ValidatorsMonitor.NotifyEvery = DefaultNotifyEvery
	}
	if c.ValidatorsMonitor.RecentMissedBlocksNotifyThreshold == 0 {
		c.ValidatorsMonitor.RecentMissedBlocksNotifyThreshold = DefaultRecentMissedBlocksNotifyThreshold
	}
	if c.ValidatorsMonitor.SlashingPeriod == 0 {
		c.ValidatorsMonitor.SlashingPeriod = DefaultSlashingPeriod
	}
	if c.ValidatorsMonitor.HaltThresholdNanoseconds == 0 {
		c.ValidatorsMonitor.HaltThresholdNanoseconds = DefaultHaltThresholdNanoseconds
	}
	if c.ValidatorsMonitor.MaxNbConcurrentGoroutines == 0 {
		c.ValidatorsMonitor.MaxNbConcurrentGoroutines = DefaultMaxNbConcurrentGoroutines
	}
}

type ValidatorsMonitorConfig struct {
	RPC        string `yaml:"rpc"`
	ChainID    string `yaml:"chain_id"`
	RPCRetries int    `yaml:"rpc_retries"`

	NotifyEvery                          int64   `yaml:"notify_every"`
	SlashingPeriodUptimeWarningThreshold float64 `yaml:"slashing_warn_threshold"`
	SlashingPeriodUptimeErrorThreshold   float64 `yaml:"slashing_error_threshold"`
	RecentBlocksToCheck                  int64   `yaml:"recent_blocks_to_check"`
	MissedBlocksThreshold                int64   `yaml:"missed_blocks_threshold"`
	RecentMissedBlocksNotifyThreshold    int64   `yaml:"recent_missed_blocks_notify_threshold"`
	SlashingPeriod                       int64   `yaml:"slashing_period"`
	HaltThresholdNanoseconds             int64   `yaml:"halt_threshold_nanoseconds"`
	MaxNbConcurrentGoroutines            int     `yaml:"max_nb_concurrent_goroutines"`
}
