package monitor

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/staking4all/celestia-monitoring-bot/services"
	"github.com/staking4all/celestia-monitoring-bot/services/cosmos"
	"github.com/staking4all/celestia-monitoring-bot/services/models"
	"go.uber.org/zap"
)

type monitorService struct {
	alertState     map[string]map[int64]*models.ValidatorAlertState
	userState      map[int64]map[string]*models.ValidatorAlertState
	valStats       map[string]models.ValidatorStats
	alertStateLock sync.Mutex
	valStatsLock   sync.Mutex
	ctx            context.Context

	config models.Config
	client services.CosmosClient
	ns     services.NotificationService
	db     services.PersistenceDB
}

func NewMonitorService(config models.Config, ns services.NotificationService, db services.PersistenceDB) (services.MonitorService, error) {
	m := &monitorService{
		alertState:     make(map[string]map[int64]*models.ValidatorAlertState),
		userState:      make(map[int64]map[string]*models.ValidatorAlertState),
		alertStateLock: sync.Mutex{},
		ctx:            context.Background(),

		config: config,
		ns:     ns,
		db:     db,
	}

	ns.SetMonitoManager(m)

	data, err := db.List()
	if err != nil {
		return nil, fmt.Errorf("loading database info: %+v", err)
	}

	for userID, valList := range data {
		for _, val := range valList {
			v := val
			err := m.load(userID, &v)
			if err != nil {
				return nil, fmt.Errorf("loading database info: %+v", err)
			}
		}
	}

	client, err := cosmos.NewCosmosClient(config.ValidatorsMonitor.RPC, config.ValidatorsMonitor.ChainID)
	if err != nil {
		return nil, err
	}

	m.client = client

	return m, nil
}

func (m *monitorService) load(userID int64, v *models.Validator) error {
	validator := v.Copy()

	m.alertStateLock.Lock()
	defer m.alertStateLock.Unlock()

	// validate address
	if !strings.HasPrefix(validator.Address, "celestiavalcons1") {
		return fmt.Errorf("invalid address, should start with `celestiavalcons1`")
	}

	if _, _, err := bech32.DecodeAndConvert(validator.Address); err != nil {
		return fmt.Errorf("invalid address: %+v", err)
	}

	if m.userState[userID] == nil {
		m.userState[userID] = make(map[string]*models.ValidatorAlertState)
	}

	if m.userState[userID][validator.Address] != nil {
		return fmt.Errorf("already registered")
	}

	if m.alertState[validator.Address] == nil {
		m.alertState[validator.Address] = make(map[int64]*models.ValidatorAlertState)
	}

	m.alertState[validator.Address][userID] = &models.ValidatorAlertState{
		UserValidator:              validator,
		AlertTypeCounts:            make(map[models.AlertType]int64),
		SentryGRPCErrorCounts:      make(map[string]int64),
		SentryOutOfSyncErrorCounts: make(map[string]int64),
		SentryHaltErrorCounts:      make(map[string]int64),
		SentryLatestHeight:         make(map[string]int64),
	}

	m.userState[userID][validator.Address] = m.alertState[validator.Address][userID]
	return nil
}

func (m *monitorService) Add(userID int64, v *models.Validator) error {
	err := m.load(userID, v)
	if err != nil {
		zap.L().Error("added validator", zap.Int64("userID", userID), zap.Any("validator", v), zap.Error(err))
		return err
	}

	// persist info
	err = m.db.Add(userID, v)
	if err != nil {
		zap.L().Error("added validator", zap.Int64("userID", userID), zap.Any("validator", v), zap.Error(err))
		return err
	}

	zap.L().Debug("added validator", zap.Int64("userID", userID), zap.Any("validator", v))
	return nil
}

func (m *monitorService) Remove(userID int64, address string) error {
	err := m.db.Remove(userID, address)
	if err != nil {
		return err
	}

	m.alertStateLock.Lock()
	defer m.alertStateLock.Unlock()

	if m.userState[userID] == nil {
		return fmt.Errorf("not found")
	}

	if m.userState[userID][address] == nil {
		return fmt.Errorf("not found")
	}

	delete(m.userState[userID], address)

	if len(m.userState[userID]) == 0 {
		delete(m.userState, userID)
	}

	if m.alertState[address] != nil {
		delete(m.alertState[address], userID)

		if len(m.alertState[address]) == 0 {
			delete(m.alertState, address)
		}
	}

	return nil
}

func (m *monitorService) Stop() error {
	m.ctx.Done()
	zap.L().Info("validators monitoring stopped")

	return nil
}

const (
	exitCodeErr       = 1
	exitCodeInterrupt = 2
)

func (m *monitorService) Run() error {
	ctx, cancel := context.WithCancel(m.ctx)
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	defer func() {
		signal.Stop(signalChan)
		cancel()
	}()

	go func() {
		select {
		case <-signalChan: // first signal, cancel context
			zap.L().Info("validators monitoring stopping")
			cancel()
		case <-ctx.Done():
		}
		<-signalChan // second signal, hard exit
		os.Exit(exitCodeInterrupt)
	}()

	ticker := time.NewTicker(10 * time.Second)

	concurrentGoroutines := make(chan struct{}, m.config.ValidatorsMonitor.MaxNbConcurrentGoroutines)

	zap.L().Info("starting node monitoring")
	for {
		zap.L().Info("checking data")

		slashingInfo, err := m.client.GetSlashingInfo()
		if err != nil {
			zap.L().Error("error retriving slashing info", zap.Error(err))
			continue
		}

		status, err := m.client.GetNodeStatus()
		if err != nil {
			zap.L().Error("error retriving node status", zap.Error(err))
			continue
		}

		// check node data
		if status.SyncInfo.CatchingUp {
			zap.L().Error("error retriving node status: node out of sync")
			continue
		}

		timeSinceLastBlock := time.Now().UnixNano() - status.SyncInfo.LatestBlockTime.UnixNano()
		if timeSinceLastBlock > m.config.ValidatorsMonitor.HaltThresholdNanoseconds {
			zap.L().Error("error retriving node status: chain halt", zap.Int64("time", timeSinceLastBlock))
			continue
		}

		newValStatsMap := make(map[string]models.ValidatorStats)
		statsLock := sync.Mutex{}
		wg := sync.WaitGroup{}

		zap.L().Debug("getting validators to monitor")
		m.alertStateLock.Lock()
		// load map keys
		keys := make([]string, 0, len(m.alertState))
		for k := range m.alertState {
			keys = append(keys, k)
		}
		m.alertStateLock.Unlock()
		zap.L().Debug("getting validators to monitor", zap.Any("list", keys))

		for _, addr := range keys {
			zap.L().Debug("checking node", zap.String("address", addr))
			wg.Add(1)
			go func(addr string) {
				defer func() {
					wg.Done()
					<-concurrentGoroutines
				}()
				concurrentGoroutines <- struct{}{}
				valStats, err := m.getData(addr, slashingInfo, status)
				if err != nil {
					zap.L().Error("error getting validator info", zap.Error(err))
					return
				}
				valStats.DetermineAggregatedErrorsAndAlertLevel(m.config.ValidatorsMonitor)
				statsLock.Lock()
				newValStatsMap[addr] = *valStats
				statsLock.Unlock()
			}(addr)
		}
		wg.Wait()

		// copy to internal
		m.valStatsLock.Lock()
		m.valStats = make(map[string]models.ValidatorStats)
		for addr, stats := range newValStatsMap {
			m.valStats[addr] = stats
		}
		m.valStatsLock.Unlock()

		m.alertStateLock.Lock()
		for addr, stats := range newValStatsMap {
			// get users subscribed
			for userID, val := range m.alertState[addr] {
				notification := stats.GetAlertNotification(val, stats.Errs, m.config.ValidatorsMonitor)
				if notification != nil {
					m.ns.SendValidatorAlertNotification(userID, val.UserValidator, stats, notification)
				}
			}
		}
		m.alertStateLock.Unlock()

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
