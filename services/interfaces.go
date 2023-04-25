package services

import (
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/staking4all/celestia-monitoring-bot/services/models"
)

type CosmosClient interface {
	GetNodeBlock(nonce int64) (*coretypes.ResultBlock, error)
	GetNodeStatus() (*coretypes.ResultStatus, error)
	GetSlashingInfo() (*slashingtypes.QueryParamsResponse, error)
	GetSigningInfo(address string) (*slashingtypes.QuerySigningInfoResponse, error)
}

type MonitorService interface {
	Run() error
	Stop() error
}

type MonitorManager interface {
	Add(userID int64, validator *models.Validator) error
	Remove(userID int64, address string) error
	List(userID int64) ([]models.ValidatorStatsRegister, error)
	GetCurrentState(userID int64, address string) (models.ValidatorStatsRegister, error)
}

type NotificationService interface {
	// send one time alert for validator
	SendValidatorAlertNotification(userID int64, vm *models.Validator, stats models.ValidatorStats, alertNotification *models.ValidatorAlertNotification)
	SetMonitoManager(mm MonitorManager)
}

type PersistenceDB interface {
	Close() error
	Add(userID int64, validator *models.Validator) error
	Remove(userID int64, address string) error
	List() (map[int64][]models.Validator, error)
}
