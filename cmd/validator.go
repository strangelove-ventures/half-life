package cmd

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/DisgoOrg/disgo/webhook"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/tendermint/tendermint/libs/bytes"
)

const (
	rpcErrorRetries = 5
)

func formattedTime(t time.Time) string {
	return fmt.Sprintf("%d-%02d-%02d %02d:%02d:%02d UTC",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second())
}

func monitorValidator(vm *ValidatorMonitor) (stats ValidatorStats, errs []error) {
	stats.LastSignedBlockHeight = -1
	fmt.Printf("Monitoring validator: %s\n", vm.Name)
	client, err := getCosmosClient(vm)
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
		for i := stats.Height; i > stats.Height-recentBlocksToCheck; i-- {
			block, err := node.Block(context.Background(), &i)
			if err != nil {
				// generic RPC error for this one so it will be included in the generic RPC error retry
				errs = append(errs, newGenericRPCError(newBlockFetchError(i, vm.RPC).Error()))
				continue
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
				for i := stats.Height - recentBlocksToCheck; stats.LastSignedBlockHeight == -1 && i > (stats.Height-slashingPeriod); i-- {
					block, err := node.Block(context.Background(), &i)
					if err != nil {
						errs = append(errs, newBlockFetchError(i, vm.RPC))
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

func runMonitor(
	wg *sync.WaitGroup,
	alertState *map[string]*ValidatorAlertState,
	discordClient *webhook.Client,
	config *HalfLifeConfig,
	vm *ValidatorMonitor,
	writeConfigMutex *sync.Mutex,
) {
	var stats ValidatorStats
	var errs []error
	for i := 0; i < rpcErrorRetries; i++ {
		stats, errs = monitorValidator(vm)
		if errs == nil {
			fmt.Printf("No errors found for validator: %s\n", vm.Name)
			break
		}
		fmt.Printf("Got validator errors: +%v\n", errs)
		foundNonRPCError := false
		for _, err := range errs {
			if _, ok := err.(*GenericRPCError); !ok {
				foundNonRPCError = true
				break
			}
		}
		if foundNonRPCError {
			break
		}
		if i < rpcErrorRetries-1 {
			fmt.Println("Found only RPC errors, retrying")
		}
		// loop again up to n times if we are hitting only generic RPC errors
	}
	sendDiscordAlert(vm, stats, alertState, discordClient, config, errs, writeConfigMutex)
	wg.Done()
}
