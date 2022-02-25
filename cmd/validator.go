package cmd

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/tendermint/tendermint/libs/bytes"
)

const (
	rpcErrorRetries    = 5
	outOfSyncThreshold = 5
)

func formattedTime(t time.Time) string {
	return fmt.Sprintf("<t:%d:R>", t.Unix())
}

func monitorValidator(
	vm *ValidatorMonitor,
	stats *ValidatorStats,
) (errs []error) {
	stats.LastSignedBlockHeight = -1
	fmt.Printf("Monitoring validator: %s\n", vm.Name)
	client, err := getCosmosClient(vm.RPC, vm.ChainID)
	if err != nil {
		errs = append(errs, newGenericRPCError(err.Error()))
		return
	}
	_, hexAddress, err := bech32.DecodeAndConvert(vm.Address)
	if err != nil {
		errs = append(errs, err)
		return
	}

	valInfo, err := getSigningInfo(client, vm.Address)
	slashingPeriod := int64(10000)
	if err != nil {
		errs = append(errs, newGenericRPCError(err.Error()))
	} else {
		signingInfo := valInfo.ValSigningInfo
		if signingInfo.Tombstoned {
			errs = append(errs, newTombstonedError())
		}
		if signingInfo.JailedUntil.After(time.Now()) {
			errs = append(errs, newJailedError(signingInfo.JailedUntil))
		}
		slashingInfo, err := getSlashingInfo(client)
		if err != nil {
			errs = append(errs, newGenericRPCError(err.Error()))
		} else {
			slashingPeriod = slashingInfo.Params.SignedBlocksWindow
			stats.SlashingPeriodUptime = 100.0 - 100.0*(float64(signingInfo.MissedBlocksCounter)/float64(slashingPeriod))
		}
	}
	node, err := client.GetNode()
	if err != nil {
		errs = append(errs, newGenericRPCError(err.Error()))
		return
	}
	status, err := node.Status(context.Background())
	if err != nil {
		errs = append(errs, newGenericRPCError(err.Error()))
	} else {
		if status.SyncInfo.CatchingUp {
			errs = append(errs, newOutOfSyncError(vm.RPC))
		}
		stats.Height = status.SyncInfo.LatestBlockHeight
		stats.Timestamp = formattedTime(status.SyncInfo.LatestBlockTime)
		stats.RecentMissedBlocks = 0
		for i := stats.Height; i > stats.Height-recentBlocksToCheck && i > 0; i-- {
			block, err := node.Block(context.Background(), &i)
			if err != nil {
				// generic RPC error for this one so it will be included in the generic RPC error retry
				errs = append(errs, newGenericRPCError(newBlockFetchError(i, vm.RPC).Error()))
				continue
			}
			if i == 1 {
				break
			}
			found := false
			for _, voter := range block.Block.LastCommit.Signatures {
				if reflect.DeepEqual(voter.ValidatorAddress, bytes.HexBytes(hexAddress)) {
					if block.Block.Height > stats.LastSignedBlockHeight {
						stats.LastSignedBlockHeight = block.Block.Height
						stats.LastSignedBlockTimestamp = formattedTime(block.Block.Time)
					}
					found = true
					break
				}
			}
			if !found {
				stats.RecentMissedBlocks++
			}
		}

		if stats.RecentMissedBlocks > 0 {
			errs = append(errs, newMissedRecentBlocksError(stats.RecentMissedBlocks))
			// Go back to find last signed block
			if stats.LastSignedBlockHeight == -1 {
				for i := stats.Height - recentBlocksToCheck; stats.LastSignedBlockHeight == -1 && i > (stats.Height-slashingPeriod) && i > 0; i-- {
					block, err := node.Block(context.Background(), &i)
					if err != nil {
						errs = append(errs, newBlockFetchError(i, vm.RPC))
						break
					}
					if i == 1 {
						break
					}
					for _, voter := range block.Block.LastCommit.Signatures {
						if reflect.DeepEqual(voter.ValidatorAddress, bytes.HexBytes(hexAddress)) {
							stats.LastSignedBlockHeight = block.Block.Height
							stats.LastSignedBlockTimestamp = formattedTime(block.Block.Time)
							break
						}
					}
					if stats.LastSignedBlockHeight != -1 {
						break
					}
				}
			}
		}
	}

	return
}

func monitorSentry(
	wg *sync.WaitGroup,
	errs *[]error,
	errsLock *sync.Mutex,
	sentry Sentry,
	stats *ValidatorStats,
	vm *ValidatorMonitor,
) {
	nodeInfo, syncInfo, err := getSentryInfo(sentry.GRPC)
	var errToAdd error
	sentryStats := SentryStats{Name: sentry.Name, SentryAlertType: sentryAlertTypeNone}
	if err != nil {
		errToAdd = newSentryGRPCError(sentry.Name, err.Error())
		sentryStats.SentryAlertType = sentryAlertTypeGRPCError
	} else {
		sentryStats.Height = syncInfo.Block.Header.Height
		sentryStats.Version = nodeInfo.ApplicationVersion.GetVersion()
		if stats.Height-syncInfo.Block.Header.Height > outOfSyncThreshold {
			errToAdd = newSentryOutOfSyncError(sentry.Name, fmt.Sprintf("Height: %d not in sync with RPC Height: %d", syncInfo.Block.Header.Height, stats.Height))
			sentryStats.SentryAlertType = sentryAlertTypeOutOfSyncError
		}
	}
	errsLock.Lock()
	stats.SentryStats = append(stats.SentryStats, sentryStats)
	if err != nil {
		*errs = append(*errs, errToAdd)
	}
	errsLock.Unlock()
	wg.Done()
}

func monitorSentries(
	stats *ValidatorStats,
	vm *ValidatorMonitor,
) []error {
	errs := make([]error, 0)
	wg := sync.WaitGroup{}
	errsLock := sync.Mutex{}
	sentries := *vm.Sentries
	wg.Add(len(sentries))
	for _, sentry := range sentries {
		go monitorSentry(&wg, &errs, &errsLock, sentry, stats, vm)
	}
	wg.Wait()
	return errs
}

func runMonitor(
	notificationService NotificationService,
	alertState *map[string]*ValidatorAlertState,
	config *HalfLifeConfig,
	vm *ValidatorMonitor,
	writeConfigMutex *sync.Mutex,
) {
	for {
		stats := ValidatorStats{}
		var valErrs []error
		var sentryErrs []error

		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			var rpcRetries int
			if vm.RPCRetries != nil {
				rpcRetries = *vm.RPCRetries
			} else {
				rpcRetries = rpcErrorRetries
			}

			for i := 0; i < rpcRetries; i++ {
				valErrs = monitorValidator(vm, &stats)
				if len(valErrs) == 0 {
					fmt.Printf("No errors found for validator: %s\n", vm.Name)
					break
				}
				fmt.Printf("Got validator errors: +%v\n", valErrs)
				foundNonRPCError := false
				for _, err := range valErrs {
					if _, ok := err.(*GenericRPCError); !ok {
						foundNonRPCError = true
						break
					}
				}
				if foundNonRPCError {
					break
				}
				if i < rpcRetries-1 {
					fmt.Println("Found only RPC errors, retrying")
					time.Sleep(time.Duration((i*i)+1) * time.Second) // exponential backoff retry
				}
				// loop again up to n times if we are hitting only generic RPC errors
			}
			wg.Done()
		}()

		if vm.Sentries != nil {
			wg.Add(1)
			go func() {
				sentryErrs = monitorSentries(&stats, vm)
				if len(sentryErrs) == 0 {
					fmt.Printf("No errors found for validator sentries: %s\n", vm.Name)
				} else {
					fmt.Printf("Got validator sentry errors: +%v\n", sentryErrs)
				}
				wg.Done()
			}()
		}

		wg.Wait()

		errs := []error{}
		if len(valErrs) > 0 {
			errs = append(errs, valErrs...)
		}
		if len(sentryErrs) > 0 {
			errs = append(errs, sentryErrs...)
		}

		stats.determineCurrentAlertLevel()

		notification := getAlertNotification(vm, stats, alertState, errs)

		if notification != nil {
			notificationService.SendValidatorAlertNotification(config, vm, stats, notification)
		}

		notificationService.UpdateValidatorRealtimeStatus(config, vm, stats, writeConfigMutex)

		time.Sleep(30 * time.Second)
	}
}

func (stats *ValidatorStats) determineCurrentAlertLevel() {
	if stats.Height == stats.LastSignedBlockHeight {
		if stats.RecentMissedBlocks == 0 {
			if stats.SlashingPeriodUptime > slashingPeriodUptimeWarningThreshold {
				stats.AlertLevel = alertLevelNone
				return
			} else {
				// Warning for recovering from downtime. Not error because we are currently signing
				stats.AlertLevel = alertLevelWarning
				return
			}
		} else {
			// Warning for missing recent blocks, but have signed current block
			stats.AlertLevel = alertLevelWarning
			return
		}
	}

	// past this, we have not signed the most recent block

	if stats.RecentMissedBlocks < recentBlocksToCheck {
		// we have missed some, but not all, of the recent blocks to check
		if stats.SlashingPeriodUptime > slashingPeriodUptimeErrorThreshold {
			stats.AlertLevel = alertLevelWarning
		} else {
			// we are below slashing period uptime error threshold
			stats.AlertLevel = alertLevelHigh
		}
	} else {
		// Error, missed all of the recent blocks to check
		stats.AlertLevel = alertLevelHigh
	}
}

func getAlertNotification(
	vm *ValidatorMonitor,
	stats ValidatorStats,
	alertState *map[string]*ValidatorAlertState,
	errs []error,
) *ValidatorAlertNotification {
	if (*alertState)[vm.Name] == nil {
		(*alertState)[vm.Name] = &ValidatorAlertState{
			AlertTypeCounts:            make(map[AlertType]int64),
			SentryGRPCErrorCounts:      make(map[string]int64),
			SentryOutOfSyncErrorCounts: make(map[string]int64),
		}
	}
	var foundAlertTypes []AlertType
	var foundSentryGRPCErrors []string
	var foundSentryOutOfSyncErrors []string
	alertNotification := ValidatorAlertNotification{AlertLevel: alertLevelNone}

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
		shouldNotify := (*alertState)[vm.Name].AlertTypeCounts[alertType]%notifyEvery == 0
		(*alertState)[vm.Name].AlertTypeCounts[alertType]++
		return shouldNotify
	}

	handleGenericAlert := func(err error, alertType AlertType, alertLevel AlertLevel) {
		if shouldNotifyForFoundAlertType(alertType) {
			addAlert(err)
			setAlertLevel(alertLevel)
		}
	}

	for _, err := range errs {
		switch err := err.(type) {
		case *JailedError:
			handleGenericAlert(err, alertTypeJailed, alertLevelHigh)
		case *TombstonedError:
			handleGenericAlert(err, alertTypeTombstoned, alertLevelCritical)
		case *OutOfSyncError:
			handleGenericAlert(err, alertTypeOutOfSync, alertLevelWarning)
		case *BlockFetchError:
			handleGenericAlert(err, alertTypeBlockFetch, alertLevelWarning)
		case *MissedRecentBlocksError:
			if shouldNotifyForFoundAlertType(alertTypeMissedRecentBlocks) || stats.RecentMissedBlocks != (*alertState)[vm.Name].RecentMissedBlocksCounter {
				addAlert(err)
				if stats.RecentMissedBlocks > (*alertState)[vm.Name].RecentMissedBlocksCounter {
					if stats.RecentMissedBlocks > recentMissedBlocksNotifyThreshold {
						setAlertLevel(alertLevelHigh)
					} else {
						setAlertLevel(alertLevelWarning)
					}
				} else {
					setAlertLevel(alertLevelWarning)
				}
			}
			(*alertState)[vm.Name].RecentMissedBlocksCounter = stats.RecentMissedBlocks
			if stats.RecentMissedBlocks > (*alertState)[vm.Name].RecentMissedBlocksCounterMax {
				(*alertState)[vm.Name].RecentMissedBlocksCounterMax = stats.RecentMissedBlocks
			}
		case *GenericRPCError:
			handleGenericAlert(err, alertTypeGenericRPC, alertLevelWarning)
		case *SentryGRPCError:
			sentryName := err.sentry
			foundSentryGRPCErrors = append(foundSentryGRPCErrors, sentryName)
			if (*alertState)[vm.Name].SentryGRPCErrorCounts[sentryName]%notifyEvery == 0 || (*alertState)[vm.Name].SentryOutOfSyncErrorCounts[sentryName] == sentryGRPCErrorNotifyThreshold {
				addAlert(err)
				if (*alertState)[vm.Name].SentryGRPCErrorCounts[sentryName] >= sentryGRPCErrorNotifyThreshold {
					setAlertLevel(alertLevelHigh)
				} else {
					setAlertLevel(alertLevelWarning)
				}
			}
			(*alertState)[vm.Name].SentryGRPCErrorCounts[sentryName]++
		case *SentryOutOfSyncError:
			sentryName := err.sentry
			foundSentryOutOfSyncErrors = append(foundSentryOutOfSyncErrors, sentryName)
			if (*alertState)[vm.Name].SentryOutOfSyncErrorCounts[sentryName]%notifyEvery == 0 || (*alertState)[vm.Name].SentryOutOfSyncErrorCounts[sentryName] == sentryOutOfSyncErrorNotifyThreshold {
				addAlert(err)
				if (*alertState)[vm.Name].SentryOutOfSyncErrorCounts[sentryName] >= sentryOutOfSyncErrorNotifyThreshold {
					setAlertLevel(alertLevelHigh)
				} else {
					setAlertLevel(alertLevelWarning)
				}
			}
			(*alertState)[vm.Name].SentryOutOfSyncErrorCounts[sentryName]++
		default:
			addAlert(err)
			setAlertLevel(alertLevelWarning)
		}
	}

	// iterate through all error types
	for i := alertTypeJailed; i < alertTypeEnd; i++ {
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
				alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "jailed")
				alertNotification.NotifyForClear = true
			case alertTypeTombstoned:
				alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "tombstoned")
				alertNotification.NotifyForClear = true
			case alertTypeOutOfSync:
				alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "rpc server out of sync")
			case alertTypeBlockFetch:
				alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "rpc block fetch error")
			case alertTypeMissedRecentBlocks:
				alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "missed recent blocks")
				if (*alertState)[vm.Name].RecentMissedBlocksCounterMax > recentMissedBlocksNotifyThreshold {
					alertNotification.NotifyForClear = true
				}
				(*alertState)[vm.Name].RecentMissedBlocksCounter = 0
				(*alertState)[vm.Name].RecentMissedBlocksCounterMax = 0
			case alertTypeGenericRPC:
				alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "generic rpc error")
			default:
			}
		}
	}
	for sentryName := range (*alertState)[vm.Name].SentryGRPCErrorCounts {
		sentryFound := false
		for _, foundSentryName := range foundSentryGRPCErrors {
			if foundSentryName == sentryName {
				sentryFound = true
				break
			}
		}
		if !sentryFound && (*alertState)[vm.Name].SentryGRPCErrorCounts[sentryName] > 0 {
			if (*alertState)[vm.Name].SentryGRPCErrorCounts[sentryName] > sentryGRPCErrorNotifyThreshold {
				alertNotification.NotifyForClear = true
			}
			(*alertState)[vm.Name].SentryGRPCErrorCounts[sentryName] = 0
			alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, fmt.Sprintf("%s grpc error", sentryName))
		}
	}
	for sentryName := range (*alertState)[vm.Name].SentryOutOfSyncErrorCounts {
		sentryHasOutOfSyncError := false
		for _, foundSentryName := range foundSentryOutOfSyncErrors {
			if foundSentryName == sentryName {
				sentryHasOutOfSyncError = true
				break
			}
		}

		sentryHasGRPCError := false
		for _, foundSentryName := range foundSentryGRPCErrors {
			if foundSentryName == sentryName {
				sentryHasGRPCError = true
				break
			}
		}
		if !sentryHasOutOfSyncError && !sentryHasGRPCError && (*alertState)[vm.Name].SentryOutOfSyncErrorCounts[sentryName] > 0 {
			if (*alertState)[vm.Name].SentryOutOfSyncErrorCounts[sentryName] > sentryOutOfSyncErrorNotifyThreshold {
				alertNotification.NotifyForClear = true
			}
			(*alertState)[vm.Name].SentryOutOfSyncErrorCounts[sentryName] = 0
			alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, fmt.Sprintf("%s out of sync error", sentryName))
		}
	}

	if len(alertNotification.Alerts) == 0 && len(alertNotification.ClearedAlerts) == 0 {
		return nil
	}
	return &alertNotification
}
