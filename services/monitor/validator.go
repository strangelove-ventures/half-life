package monitor

import (
	"fmt"
	"reflect"
	"time"

	"github.com/cometbft/cometbft/libs/bytes"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/staking4all/celestia-monitoring-bot/services/models"
	"go.uber.org/zap"
)

const (
	RecentBlocksToCheck   = int64(20)
	missedBlocksThreshold = int64(1)
)

func (m *monitorService) getData(
	address string,
	slashingInfo *slashingtypes.QueryParamsResponse,
	nodeStatus *coretypes.ResultStatus,
) (*models.ValidatorStats, error) {
	zap.L().Debug("retriving data", zap.String("address", address))

	stats := &models.ValidatorStats{
		LastSignedBlockHeight: -1,
		Errs:                  make([]models.IgnorableError, 0),
	}

	_, hexAddress, err := bech32.DecodeAndConvert(address)
	if err != nil {
		stats.AddIgnorableError(models.NewIgnorableError(err))
		return nil, err
	}

	var valInfo *slashingtypes.QuerySigningInfoResponse

	// TODO: move retrier to client
	for i := 0; i < m.config.ValidatorsMonitor.RPCRetries; i++ {
		valInfo, err = m.client.GetSigningInfo(address)
		if err == nil {
			break
		}

		zap.L().Debug("getting info", zap.Int("retry", i), zap.Error(err))

		if i < m.config.ValidatorsMonitor.RPCRetries-1 {
			time.Sleep(time.Duration((i*i)+1) * time.Second) // exponential backoff retry
		} else {
			return nil, err
		}
	}

	signingInfo := valInfo.ValSigningInfo
	stats.Tombstoned = signingInfo.Tombstoned
	if signingInfo.Tombstoned {
		stats.AddIgnorableError(models.NewTombstonedError())
	}
	stats.JailedUntil = signingInfo.JailedUntil
	if signingInfo.JailedUntil.After(time.Now()) {
		stats.AddIgnorableError(models.NewJailedError(signingInfo.JailedUntil))
	}
	slashingPeriod := slashingInfo.Params.SignedBlocksWindow
	stats.SlashingPeriodUptime = 100.0 - 100.0*(float64(signingInfo.MissedBlocksCounter)/float64(slashingPeriod))

	if stats.SlashingPeriodUptime < m.config.ValidatorsMonitor.SlashingPeriodUptimeErrorThreshold {
		stats.AddIgnorableError(models.NewSlashingSLAError(stats.SlashingPeriodUptime, m.config.ValidatorsMonitor.SlashingPeriodUptimeErrorThreshold))
	}

	stats.Height = nodeStatus.SyncInfo.LatestBlockHeight
	stats.Timestamp = nodeStatus.SyncInfo.LatestBlockTime
	stats.RecentMissedBlocks = 0

	for i := stats.Height; i > stats.Height-RecentBlocksToCheck && i > 0; i-- {
		block, err := m.client.GetNodeBlock(i)
		if err != nil {
			return nil, err
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

	if stats.RecentMissedBlocks > missedBlocksThreshold {
		stats.AddIgnorableError(models.NewMissedRecentBlocksError(stats.RecentMissedBlocks, m.config.ValidatorsMonitor.RecentBlocksToCheck))
		// Go back to find last signed block
		if stats.LastSignedBlockHeight == -1 {
			for i := stats.Height - RecentBlocksToCheck; stats.LastSignedBlockHeight == -1 && i > (stats.Height-slashingPeriod) && i > 0; i-- {
				block, err := m.client.GetNodeBlock(i)
				if err != nil {
					return nil, err
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

	return stats, nil
}

func (m *monitorService) GetCurrentState(userID int64, address string) (stats models.ValidatorStatsRegister, err error) {
	m.alertStateLock.Lock()
	defer m.alertStateLock.Unlock()
	m.valStatsLock.Lock()
	defer m.valStatsLock.Unlock()

	if m.userState[userID] != nil && m.userState[userID][address] != nil {
		stats.Validator = *m.userState[userID][address].UserValidator
		stats.ValidatorStats = m.valStats[address]
	} else {
		err = fmt.Errorf("validator not registered for user: %s", address)
	}

	return
}

func (m *monitorService) List(userID int64) ([]models.ValidatorStatsRegister, error) {
	m.alertStateLock.Lock()
	defer m.alertStateLock.Unlock()
	m.valStatsLock.Lock()
	defer m.valStatsLock.Unlock()

	list := make([]models.ValidatorStatsRegister, 0)
	for addr, as := range m.userState[userID] {
		list = append(list, models.ValidatorStatsRegister{
			ValidatorStats: m.valStats[addr],
			Validator:      *as.UserValidator,
		})
	}

	return list, nil
}
