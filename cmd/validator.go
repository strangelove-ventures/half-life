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
	rpcErrorRetries          = 5
	outOfSyncThreshold       = 5
	haltThresholdNanoseconds = 3e11 // if nodes are stuck for > 5 minutes, will be considered halt
)

func monitorValidator(
	vm *ValidatorMonitor,
	stats *ValidatorStats,
) (errs []IgnorableError) {
	stats.LastSignedBlockHeight = -1
	fmt.Printf("Monitoring validator: %s\n", vm.Name)
	client, err := getCosmosClient(vm.RPC, vm.ChainID)
	if err != nil {
		errs = append(errs, newGenericRPCError(err.Error()))
		return
	}
	_, hexAddress, err := bech32.DecodeAndConvert(vm.Address)
	if err != nil {
		errs = append(errs, newIgnorableError(err))
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
	statusCtx, statusCtxCancel := context.WithTimeout(context.Background(), time.Duration(time.Second*RPCTimeoutSeconds))
	status, err := node.Status(statusCtx)
	statusCtxCancel()
	if err != nil {
		errs = append(errs, newGenericRPCError(err.Error()))
	} else {
		if status.SyncInfo.CatchingUp {
			errs = append(errs, newOutOfSyncError(vm.RPC))
		} else {
			timeSinceLastBlock := time.Now().UnixNano() - status.SyncInfo.LatestBlockTime.UnixNano()
			if timeSinceLastBlock > haltThresholdNanoseconds {
				errs = append(errs, newChainHaltError(timeSinceLastBlock))
			}
		}
		stats.Height = status.SyncInfo.LatestBlockHeight
		stats.Timestamp = status.SyncInfo.LatestBlockTime
		stats.RecentMissedBlocks = 0
		for i := stats.Height; i > stats.Height-recentBlocksToCheck && i > 0; i-- {
			blockCtx, blockCtxCancel := context.WithTimeout(context.Background(), time.Duration(time.Second*RPCTimeoutSeconds))
			block, err := node.Block(blockCtx, &i)
			blockCtxCancel()
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
						stats.LastSignedBlockTimestamp = block.Block.Time
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
					blockCtx, blockCtxCancel := context.WithTimeout(context.Background(), time.Duration(time.Second*RPCTimeoutSeconds))
					block, err := node.Block(blockCtx, &i)
					blockCtxCancel()
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
							stats.LastSignedBlockTimestamp = block.Block.Time
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
	alertState *ValidatorAlertState,
	alertStateLock *sync.Mutex,
) {
	nodeInfo, syncInfo, err := getSentryInfo(sentry.GRPC)
	var errsToAdd []error
	sentryStats := SentryStats{Name: sentry.Name, SentryAlertType: sentryAlertTypeNone}
	if err != nil {
		errsToAdd = append(errsToAdd, newSentryGRPCError(sentry.Name, err.Error()))
		sentryStats.SentryAlertType = sentryAlertTypeGRPCError
	} else {
		sentryStats.Height = syncInfo.Block.Header.Height
		sentryStats.Version = nodeInfo.ApplicationVersion.GetVersion()
		alertStateLock.Lock()
		blockDelta := syncInfo.Block.Header.Height - alertState.SentryLatestHeight[sentry.Name]
		alertState.SentryLatestHeight[sentry.Name] = syncInfo.Block.Header.Height
		alertStateLock.Unlock()
		if blockDelta == 0 {
			timeSinceLastBlock := time.Now().UnixNano() - syncInfo.Block.Header.Time.UnixNano()
			if timeSinceLastBlock > haltThresholdNanoseconds {
				errsToAdd = append(errsToAdd, newSentryHaltError(sentry.Name, timeSinceLastBlock))
				sentryStats.SentryAlertType = sentryAlertTypeHalt
			}
		}
	}
	errsLock.Lock()
	stats.SentryStats = append(stats.SentryStats, &sentryStats)
	*errs = append(*errs, errsToAdd...)
	errsLock.Unlock()
	wg.Done()
}

func monitorSentries(
	stats *ValidatorStats,
	vm *ValidatorMonitor,
	alertState *ValidatorAlertState,
	alertStateLock *sync.Mutex,
) []error {
	errs := make([]error, 0)
	wg := sync.WaitGroup{}
	errsLock := sync.Mutex{}
	sentries := *vm.Sentries
	wg.Add(len(sentries))
	for _, sentry := range sentries {
		go monitorSentry(&wg, &errs, &errsLock, sentry, stats, vm, alertState, alertStateLock)
	}
	wg.Wait()
	return errs
}

func runMonitor(
	notificationService NotificationService,
	alertState *ValidatorAlertState,
	alertStateLock *sync.Mutex,
	configFile string,
	config *HalfLifeConfig,
	vm *ValidatorMonitor,
	writeConfigMutex *sync.Mutex,
) {
	for {
		stats := ValidatorStats{}
		var valErrs []IgnorableError
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
				sentryErrs = monitorSentries(&stats, vm, alertState, alertStateLock)
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
			for _, e := range valErrs {
				if e.Active(config.AlertConfig) {
					errs = append(errs, e)
				}
			}
		}
		if len(sentryErrs) > 0 {
			errs = append(errs, sentryErrs...)
		}

		aggregatedErrs := stats.determineAggregatedErrorsAndAlertLevel()
		if len(aggregatedErrs) > 0 {
			errs = append(errs, aggregatedErrs...)
		}

		alertStateLock.Lock()
		notification := getAlertNotification(vm, &stats, alertState, errs)
		alertStateLock.Unlock()

		if notification != nil {
			notificationService.SendValidatorAlertNotification(config, vm, stats, notification)
		}

		notificationService.UpdateValidatorRealtimeStatus(configFile, config, vm, stats, writeConfigMutex)

		time.Sleep(30 * time.Second)
	}
}

func (stats *ValidatorStats) increaseAlertLevel(alertLevel AlertLevel) {
	if stats.AlertLevel < alertLevel {
		stats.AlertLevel = alertLevel
	}
}

// determine alert level and any additional errors now that RPC And sentry checks are complete
func (stats *ValidatorStats) determineAggregatedErrorsAndAlertLevel() (errs []error) {
	sentryErrorCount := 0
	for _, sentryStat := range stats.SentryStats {
		if sentryStat.SentryAlertType != sentryAlertTypeGRPCError {
			if stats.Height-sentryStat.Height > outOfSyncThreshold {
				errs = append(errs, newSentryOutOfSyncError(sentryStat.Name, fmt.Sprintf("Height: %d not in sync with RPC Height: %d", sentryStat.Height, stats.Height)))
				sentryStat.SentryAlertType = sentryAlertTypeOutOfSyncError
			}
		}
		if sentryStat.SentryAlertType != sentryAlertTypeNone {
			// warning for error on single sentry
			stats.increaseAlertLevel(alertLevelWarning)
			sentryErrorCount++
		}
		if stats.Height < sentryStat.Height-outOfSyncThreshold {
			// RPC server is behind sentries
			stats.RPCError = true
			stats.increaseAlertLevel(alertLevelWarning)
		}
	}

	// If all sentries have errors, set overall alert level to high
	if sentryErrorCount == len(stats.SentryStats) {
		stats.increaseAlertLevel(alertLevelHigh)
	}

	if stats.Height == stats.LastSignedBlockHeight {
		if stats.RecentMissedBlocks == 0 {
			if stats.SlashingPeriodUptime > slashingPeriodUptimeWarningThreshold {
				// no recent missed blocks and above warning threshold for slashing period uptime, all good
				return
			} else {
				// Warning for recovering from downtime. Not error because we are currently signing
				stats.increaseAlertLevel(alertLevelWarning)
				return
			}
		} else {
			// Warning for missing recent blocks, but have signed current block
			stats.increaseAlertLevel(alertLevelWarning)
			return
		}
	}

	// past this, we have not signed the most recent block

	if stats.RecentMissedBlocks < recentBlocksToCheck {
		// we have missed some, but not all, of the recent blocks to check
		if stats.SlashingPeriodUptime > slashingPeriodUptimeErrorThreshold {
			stats.increaseAlertLevel(alertLevelWarning)
		} else {
			// we are below slashing period uptime error threshold
			stats.increaseAlertLevel(alertLevelHigh)
		}
	} else {
		// Error, missed all of the recent blocks to check
		stats.increaseAlertLevel(alertLevelHigh)
	}
	return
}

// requires locked alertState
func getAlertNotification(
	vm *ValidatorMonitor,
	stats *ValidatorStats,
	alertState *ValidatorAlertState,
	errs []error,
) *ValidatorAlertNotification {
	var foundAlertTypes []AlertType
	var foundSentryGRPCErrors []string
	var foundSentryOutOfSyncErrors []string
	var foundSentryHaltErrors []string
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
		shouldNotify := alertState.AlertTypeCounts[alertType]%notifyEvery == 0
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
			handleGenericAlert(err, alertTypeJailed, alertLevelHigh)
		case *TombstonedError:
			handleGenericAlert(err, alertTypeTombstoned, alertLevelCritical)
		case *OutOfSyncError:
			handleGenericAlert(err, alertTypeOutOfSync, alertLevelWarning)
			stats.RPCError = true
		case *ChainHaltError:
			fmt.Printf("found chain halt error\n")
			handleGenericAlert(err, alertTypeHalt, alertLevelWarning)
			stats.RPCError = true
		case *BlockFetchError:
			handleGenericAlert(err, alertTypeBlockFetch, alertLevelWarning)
		case *MissedRecentBlocksError:
			addRecentMissedBlocksAlertIfNecessary := func(alertLevel AlertLevel) {
				if shouldNotifyForFoundAlertType(alertTypeMissedRecentBlocks) || stats.RecentMissedBlocks != recentMissedBlocksCounter {
					addAlert(err)
					setAlertLevel(alertLevel)
				}
			}
			if stats.RecentMissedBlocks > recentMissedBlocksCounter {
				if stats.RecentMissedBlocks > recentMissedBlocksNotifyThreshold {
					stats.RecentMissedBlockAlertLevel = alertLevelHigh
					addRecentMissedBlocksAlertIfNecessary(alertLevelHigh)
				} else {
					stats.RecentMissedBlockAlertLevel = alertLevelWarning
					addRecentMissedBlocksAlertIfNecessary(alertLevelWarning)
				}
			} else {
				stats.RecentMissedBlockAlertLevel = alertLevelWarning
				addRecentMissedBlocksAlertIfNecessary(alertLevelWarning)
			}
			alertState.RecentMissedBlocksCounter = stats.RecentMissedBlocks
			if stats.RecentMissedBlocks > alertState.RecentMissedBlocksCounterMax {
				alertState.RecentMissedBlocksCounterMax = stats.RecentMissedBlocks
			}
		case *GenericRPCError:
			handleGenericAlert(err, alertTypeGenericRPC, alertLevelWarning)
			stats.RPCError = true
		case *SentryGRPCError:
			sentryName := err.sentry
			foundSentryGRPCErrors = append(foundSentryGRPCErrors, sentryName)
			if alertState.SentryGRPCErrorCounts[sentryName]%notifyEvery == 0 || alertState.SentryGRPCErrorCounts[sentryName] == sentryGRPCErrorNotifyThreshold {
				addAlert(err)
				if alertState.SentryGRPCErrorCounts[sentryName] >= sentryGRPCErrorNotifyThreshold {
					setAlertLevel(alertLevelHigh)
				} else {
					setAlertLevel(alertLevelWarning)
				}
			}
			alertState.SentryGRPCErrorCounts[sentryName]++
		case *SentryOutOfSyncError:
			sentryName := err.sentry
			foundSentryOutOfSyncErrors = append(foundSentryOutOfSyncErrors, sentryName)
			if alertState.SentryOutOfSyncErrorCounts[sentryName]%notifyEvery == 0 || alertState.SentryOutOfSyncErrorCounts[sentryName] == sentryOutOfSyncErrorNotifyThreshold {
				addAlert(err)
				if alertState.SentryOutOfSyncErrorCounts[sentryName] >= sentryOutOfSyncErrorNotifyThreshold {
					setAlertLevel(alertLevelHigh)
				} else {
					setAlertLevel(alertLevelWarning)
				}
			}
			alertState.SentryOutOfSyncErrorCounts[sentryName]++
		case *SentryHaltError:
			sentryName := err.sentry
			foundSentryHaltErrors = append(foundSentryHaltErrors, sentryName)
			if alertState.SentryHaltErrorCounts[sentryName]%notifyEvery == 0 || alertState.SentryHaltErrorCounts[sentryName] == sentryHaltErrorNotifyThreshold {
				addAlert(err)
				if alertState.SentryHaltErrorCounts[sentryName] >= sentryHaltErrorNotifyThreshold {
					setAlertLevel(alertLevelHigh)
				} else {
					setAlertLevel(alertLevelWarning)
				}
			}
			alertState.SentryHaltErrorCounts[sentryName]++
		default:
			addAlert(err)
			setAlertLevel(alertLevelWarning)
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
		return alertType == alertTypeGenericRPC || alertType == alertTypeOutOfSync
	}
	foundRPCError := hasAlertType(alertTypeOutOfSync) || hasAlertType(alertTypeGenericRPC)

	// iterate through all error types
	for _, i := range alertTypes {
		// reset alert type if we didn't see it this time and it's either an RPC error or there are no RPC errors
		// should only clear jailed, tombstoned, and missed recent blocks errors if there also isn't a generic RPC error or RPC server out of sync error
		if !hasAlertType(i) && alertState.AlertTypeCounts[i] > 0 {
			alertState.AlertTypeCounts[i] = 0
			if isRPCError(i) || !foundRPCError {
				alertState.AlertTypeCounts[i] = 0
				switch i {
				case alertTypeOutOfSync:
					alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "rpc server out of sync")
				case alertTypeGenericRPC:
					alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "generic rpc error")
				case alertTypeJailed:
					alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "jailed")
					alertNotification.NotifyForClear = true
				case alertTypeTombstoned:
					alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "tombstoned")
					alertNotification.NotifyForClear = true
				case alertTypeBlockFetch:
					alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "rpc block fetch error")
				case alertTypeMissedRecentBlocks:
					alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, "missed recent blocks")
					if alertState.RecentMissedBlocksCounterMax > recentMissedBlocksNotifyThreshold {
						alertNotification.NotifyForClear = true
					}
					alertState.RecentMissedBlocksCounter = 0
					alertState.RecentMissedBlocksCounterMax = 0
				default:
				}
			}
		}
	}
	for sentryName := range alertState.SentryGRPCErrorCounts {
		sentryFound := false
		for _, foundSentryName := range foundSentryGRPCErrors {
			if foundSentryName == sentryName {
				sentryFound = true
				break
			}
		}
		if !sentryFound && alertState.SentryGRPCErrorCounts[sentryName] > 0 {
			if alertState.SentryGRPCErrorCounts[sentryName] > sentryGRPCErrorNotifyThreshold {
				alertNotification.NotifyForClear = true
			}
			alertState.SentryGRPCErrorCounts[sentryName] = 0
			alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, fmt.Sprintf("%s grpc error", sentryName))
		}
	}
	for sentryName := range alertState.SentryHaltErrorCounts {
		sentryHasHaltError := false
		for _, foundSentryName := range foundSentryHaltErrors {
			if foundSentryName == sentryName {
				sentryHasHaltError = true
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
		if !sentryHasHaltError && !sentryHasGRPCError && alertState.SentryHaltErrorCounts[sentryName] > 0 {
			if alertState.SentryHaltErrorCounts[sentryName] > sentryHaltErrorNotifyThreshold {
				alertNotification.NotifyForClear = true
			}
			alertState.SentryHaltErrorCounts[sentryName] = 0
			alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, fmt.Sprintf("%s halt error", sentryName))
		}
	}
	for sentryName := range alertState.SentryOutOfSyncErrorCounts {
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
		if !sentryHasOutOfSyncError && !sentryHasGRPCError && alertState.SentryOutOfSyncErrorCounts[sentryName] > 0 {
			if alertState.SentryOutOfSyncErrorCounts[sentryName] > sentryOutOfSyncErrorNotifyThreshold {
				alertNotification.NotifyForClear = true
			}
			alertState.SentryOutOfSyncErrorCounts[sentryName] = 0
			alertNotification.ClearedAlerts = append(alertNotification.ClearedAlerts, fmt.Sprintf("%s out of sync error", sentryName))
		}
	}

	if len(alertNotification.Alerts) == 0 && len(alertNotification.ClearedAlerts) == 0 {
		return nil
	}
	return &alertNotification
}
