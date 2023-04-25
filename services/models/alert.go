package models

import (
	"fmt"
	"time"
)

type AlertLevel int8

const (
	AlertLevelNone AlertLevel = iota
	AlertLevelWarning
	AlertLevelHigh
	AlertLevelCritical
)

type AlertType string

const (
	AlertTypeJailed             AlertType = "alertTypeJailed"
	AlertTypeTombstoned         AlertType = "alertTypeTombstoned"
	AlertTypeOutOfSync          AlertType = "alertTypeOutOfSync"
	AlertTypeBlockFetch         AlertType = "alertTypeBlockFetch"
	AlertTypeMissedRecentBlocks AlertType = "alertTypeMissedRecentBlocks"
	AlertTypeGenericRPC         AlertType = "alertTypeGenericRPC"
	AlertTypeHalt               AlertType = "alertTypeHalt"
	AlertTypeSlashingSLA        AlertType = "alertTypeSlashingSLA"
)

var AlertTypes = []AlertType{
	AlertTypeJailed,
	AlertTypeTombstoned,
	AlertTypeOutOfSync,
	AlertTypeBlockFetch,
	AlertTypeMissedRecentBlocks,
	AlertTypeGenericRPC,
	AlertTypeHalt,
	AlertTypeSlashingSLA,
}

func (at *AlertType) UnmarshalYAML(unmarshal func(interface{}) error) error {
	alertType := ""
	err := unmarshal(&alertType)
	if err != nil {
		return err
	}

	found := false
	for _, s := range AlertTypes {
		a := AlertType(alertType)
		if s == a {
			found = true
			*at = a
		}
	}

	if !found {
		return fmt.Errorf("invalid alertType")
	}

	return nil
}

type SentryAlertType int8

const (
	SentryAlertTypeNone SentryAlertType = iota
	SentryAlertTypeGRPCError
	SentryAlertTypeOutOfSyncError
	SentryAlertTypeHalt
)

type SentryStats struct {
	Name            string
	Version         string
	Height          int64
	SentryAlertType SentryAlertType
}

type ValidatorStatsRegister struct {
	Validator
	ValidatorStats
}

type ValidatorStats struct {
	Timestamp                   time.Time
	Height                      int64
	RecentMissedBlocks          int64
	LastSignedBlockHeight       int64
	RecentMissedBlockAlertLevel AlertLevel
	LastSignedBlockTimestamp    time.Time
	SlashingPeriodUptime        float64
	SentryStats                 []*SentryStats
	AlertLevel                  AlertLevel
	RPCError                    bool

	Errs []IgnorableError

	Tombstoned  bool
	JailedUntil time.Time
}

func (v *ValidatorStats) AddIgnorableError(err IgnorableError) {
	if v.Errs == nil {
		v.Errs = make([]IgnorableError, 0)
	}
	v.Errs = append(v.Errs, err)
}

type ValidatorAlertState struct {
	AlertTypeCounts              map[AlertType]int64
	SentryGRPCErrorCounts        map[string]int64
	SentryOutOfSyncErrorCounts   map[string]int64
	SentryHaltErrorCounts        map[string]int64
	SentryLatestHeight           map[string]int64
	RecentMissedBlocksCounter    int64
	RecentMissedBlocksCounterMax int64
	LatestBlockChecked           int64
	LatestBlockSigned            int64
	UserValidator                *Validator
}

type ValidatorAlertNotification struct {
	Alerts         []string
	ClearedAlerts  []string
	NotifyForClear bool
	AlertLevel     AlertLevel
}

func (v *ValidatorStats) IncreaseAlertLevel(alertLevel AlertLevel) {
	if v.AlertLevel < alertLevel {
		v.AlertLevel = alertLevel
	}
}

// determine alert level and any additional errors now that RPC checks are complete
func (v *ValidatorStats) DetermineAggregatedErrorsAndAlertLevel(c ValidatorsMonitorConfig) {
	if v.Height == v.LastSignedBlockHeight {
		if v.RecentMissedBlocks == 0 {
			if v.SlashingPeriodUptime > c.SlashingPeriodUptimeWarningThreshold {
				// no recent missed blocks and above warning threshold for slashing period uptime, all good
				return
			}
			// Warning for recovering from downtime. Not error because we are currently signing
			v.IncreaseAlertLevel(AlertLevelWarning)
			return
		}
		// Warning for missing recent blocks, but have signed current block
		v.IncreaseAlertLevel(AlertLevelWarning)
		return
	}

	// past this, we have not signed the most recent block
	if v.RecentMissedBlocks < c.RecentBlocksToCheck {
		// we have missed some, but not all, of the recent blocks to check
		if v.SlashingPeriodUptime > c.SlashingPeriodUptimeErrorThreshold {
			v.IncreaseAlertLevel(AlertLevelWarning)
		} else {
			// we are below slashing period uptime error threshold
			v.IncreaseAlertLevel(AlertLevelHigh)
		}
	} else {
		// Error, missed all of the recent blocks to check
		v.IncreaseAlertLevel(AlertLevelHigh)
	}
}

func (v *ValidatorStats) GetAlertNotification(
	alertState *ValidatorAlertState,
	errs []IgnorableError,
	c ValidatorsMonitorConfig,
) *ValidatorAlertNotification {
	var foundAlertTypes []AlertType
	alertNotification := ValidatorAlertNotification{AlertLevel: AlertLevelNone}

	setAlertLevel := func(al AlertLevel) {
		if alertNotification.AlertLevel < al {
			alertNotification.AlertLevel = al
		}
	}

	addAlert := func(err error) {
		alertNotification.Alerts = append(alertNotification.Alerts, err.Error())
	}

	shouldNotifyForFoundAlertType := func(alertType AlertType) bool {
		foundAlertTypes = append(foundAlertTypes, alertType)
		shouldNotify := alertState.AlertTypeCounts[alertType]%alertState.UserValidator.NotifyEvery == 0
		alertState.AlertTypeCounts[alertType]++
		return shouldNotify
	}

	handleGenericAlert := func(err error, alertType AlertType, alertLevel AlertLevel) {
		if shouldNotifyForFoundAlertType(alertType) {
			addAlert(err)
			setAlertLevel(alertLevel)
		}
	}

	recentMissedBlocksCounter := alertState.RecentMissedBlocksCounter

	for _, err := range errs {
		switch err := err.(type) {
		case *JailedError:
			handleGenericAlert(err, AlertTypeJailed, AlertLevelHigh)
		case *TombstonedError:
			handleGenericAlert(err, AlertTypeTombstoned, AlertLevelCritical)
		case *OutOfSyncError:
			handleGenericAlert(err, AlertTypeOutOfSync, AlertLevelWarning)
			v.RPCError = true
		case *ChainHaltError:
			fmt.Printf("found chain halt error\n")
			handleGenericAlert(err, AlertTypeHalt, AlertLevelHigh)
			v.RPCError = true
		case *BlockFetchError:
			handleGenericAlert(err, AlertTypeBlockFetch, AlertLevelWarning)
		case *SlashingSLAError:
			// Because Slashing SLA is a 10,000 block sliding window,
			// we will be alerting for many hours under typical outage scenarios
			// if we alert every ~10 minutes like we do for other AlertTypes.
			//
			// Therefore, we only alert if we haven't already alerted:

			foundAlertTypes = append(foundAlertTypes, AlertTypeSlashingSLA)

			if alertState.AlertTypeCounts[AlertTypeSlashingSLA] == 0 {
				alertState.AlertTypeCounts[AlertTypeSlashingSLA]++
				addAlert(err)
				setAlertLevel(AlertLevelHigh)
			}
		case *MissedRecentBlocksError:
			addRecentMissedBlocksAlertIfNecessary := func(alertLevel AlertLevel) {
				if shouldNotifyForFoundAlertType(AlertTypeMissedRecentBlocks) || v.RecentMissedBlocks != recentMissedBlocksCounter {
					addAlert(err)
					setAlertLevel(alertLevel)
				}
			}
			if v.RecentMissedBlocks > recentMissedBlocksCounter {
				if v.RecentMissedBlocks > c.RecentMissedBlocksNotifyThreshold {
					v.RecentMissedBlockAlertLevel = AlertLevelHigh
					addRecentMissedBlocksAlertIfNecessary(AlertLevelHigh)
				} else {
					v.RecentMissedBlockAlertLevel = AlertLevelWarning
					addRecentMissedBlocksAlertIfNecessary(AlertLevelWarning)
				}
			} else {
				v.RecentMissedBlockAlertLevel = AlertLevelWarning
				addRecentMissedBlocksAlertIfNecessary(AlertLevelWarning)
			}
			alertState.RecentMissedBlocksCounter = v.RecentMissedBlocks
			if v.RecentMissedBlocks > alertState.RecentMissedBlocksCounterMax {
				alertState.RecentMissedBlocksCounterMax = v.RecentMissedBlocks
			}
		case *GenericRPCError:
			handleGenericAlert(err, AlertTypeGenericRPC, AlertLevelWarning)
			v.RPCError = true

		default:
			addAlert(err)
			setAlertLevel(AlertLevelWarning)
		}
	}

	hasAlertType := func(alertType AlertType) bool {
		for _, at := range foundAlertTypes {
			if at == alertType {
				return true
			}
		}
		return false
	}

	isRPCError := func(alertType AlertType) bool {
		return alertType == AlertTypeGenericRPC || alertType == AlertTypeOutOfSync
	}
	foundRPCError := hasAlertType(AlertTypeOutOfSync) || hasAlertType(AlertTypeGenericRPC)

	// iterate through all error types
	for _, i := range AlertTypes {
		// reset alert type if we didn't see it this time and it's either an RPC error or there are no RPC errors
		// should only clear jailed, tombstoned, and missed recent blocks errors if there also isn't a generic RPC error or RPC server out of sync error
		if !hasAlertType(i) && alertState.AlertTypeCounts[i] > 0 {
			alertState.AlertTypeCounts[i] = 0
			if isRPCError(i) || !foundRPCError {
				alertState.AlertTypeCounts[i] = 0
				switch i {
				case AlertTypeOutOfSync:
					alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "rpc server out of sync")
				case AlertTypeGenericRPC:
					alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "generic rpc error")
				case AlertTypeJailed:
					alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "jailed")
					alertNotification.NotifyForClear = true
				case AlertTypeTombstoned:
					alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "tombstoned")
					alertNotification.NotifyForClear = true
				case AlertTypeBlockFetch:
					alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "rpc block fetch error")
				case AlertTypeMissedRecentBlocks:
					alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "missed recent blocks")
					if alertState.RecentMissedBlocksCounterMax > c.RecentMissedBlocksNotifyThreshold {
						alertNotification.NotifyForClear = true
					}
					alertState.RecentMissedBlocksCounter = 0
					alertState.RecentMissedBlocksCounterMax = 0
				case AlertTypeSlashingSLA:
					alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "slashing sla uptime recovered")
					alertNotification.NotifyForClear = true
				default:
				}
			}
		}
	}

	if len(alertNotification.Alerts) == 0 && len(alertNotification.ClearedAlerts) == 0 {
		return nil
	}
	return &alertNotification
}
