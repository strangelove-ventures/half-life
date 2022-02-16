package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/DisgoOrg/disgo/discord"
	"github.com/DisgoOrg/disgo/webhook"
	"github.com/DisgoOrg/snowflake"
	cosmosClient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/bytes"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	libclient "github.com/tendermint/tendermint/rpc/jsonrpc/client"
	"gopkg.in/yaml.v2"
)

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

type ValidatorMonitor struct {
	Name                   string  `yaml:"name"`
	RPC                    string  `yaml:"rpc"`
	Address                string  `yaml:"address"`
	ChainID                string  `yaml:"chain-id"`
	Bech32Prefix           string  `yaml:"bech32-prefix"`
	DiscordStatusMessageID *string `yaml:"discord-status-message-id"`
}

type ValidatorStats struct {
	Timestamp                string
	Height                   int64
	RecentMissedBlocks       int64
	LastSignedBlockHeight    int64
	LastSignedBlockTimestamp string
	SlashingPeriodUptime     float64
}

type ValidatorAlertState struct {
	AlertTypeCounts           map[int8]int64
	RecentMissedBlocksCounter int64
}

const (
	configFilePath      = "./config.yaml"
	recentBlocksToCheck = 20
	notifyEvery         = 20 // check runs every ~30 seconds, so will notify for continued errors and rollup stats every ~10 mins
)

type JailedError struct{ msg string }

func (e *JailedError) Error() string { return e.msg }
func newJailedError(until time.Time) *JailedError {
	return &JailedError{fmt.Sprintf("validator is jailed until %s", until.String())}
}

type TombstonedError struct{ msg string }

func (e *TombstonedError) Error() string { return e.msg }
func newTombstonedError() *TombstonedError {
	return &TombstonedError{"validator is tombstoned"}
}

type OutOfSyncError struct{ msg string }

func (e *OutOfSyncError) Error() string { return e.msg }
func newOutOfSyncError(address string) *OutOfSyncError {
	return &OutOfSyncError{fmt.Sprintf("rpc server %s out of sync, cannot get up to date information", address)}
}

type BlockFetchError struct{ msg string }

func (e *BlockFetchError) Error() string { return e.msg }
func newBlockFetchError(height int64, address string) *BlockFetchError {
	return &BlockFetchError{fmt.Sprintf("error fetching block %d from rpc server %s", height, address)}
}

type MissedRecentBlocksError struct{ msg string }

func (e *MissedRecentBlocksError) Error() string { return e.msg }
func newMissedRecentBlocksError(missed int64) *MissedRecentBlocksError {
	return &MissedRecentBlocksError{fmt.Sprintf("missed %d/%d most recent blocks", missed, recentBlocksToCheck)}
}

func newClient(addr string) (rpcclient.Client, error) {
	httpClient, err := libclient.DefaultHTTPClient(addr)
	if err != nil {
		return nil, err
	}

	httpClient.Timeout = 10 * time.Second
	rpcClient, err := rpchttp.NewWithClient(addr, "/websocket", httpClient)
	if err != nil {
		return nil, err
	}

	return rpcClient, nil
}

func getCosmosClient(vm *ValidatorMonitor) (*cosmosClient.Context, error) {
	client, err := newClient(vm.RPC)
	if err != nil {
		return nil, err
	}
	return &cosmosClient.Context{
		Client:       client,
		ChainID:      vm.ChainID,
		Input:        os.Stdin,
		Output:       os.Stdout,
		OutputFormat: "json",
	}, nil
}

func getSlashingInfo(client *cosmosClient.Context) (*slashingtypes.QueryParamsResponse, error) {
	return slashingtypes.NewQueryClient(client).Params(context.Background(), &slashingtypes.QueryParamsRequest{})
}

func getSigningInfo(client *cosmosClient.Context, address string) (*slashingtypes.QuerySigningInfoResponse, error) {
	return slashingtypes.NewQueryClient(client).SigningInfo(context.Background(), &slashingtypes.QuerySigningInfoRequest{
		ConsAddress: address,
	})
}

func monitorValidator(vm *ValidatorMonitor) (stats ValidatorStats, errs []error) {
	stats.LastSignedBlockHeight = -1
	fmt.Printf("Monitoring validator: %s\n", vm.Name)
	client, err := getCosmosClient(vm)
	if err != nil {
		errs = append(errs, err)
		return
	}
	address, err := hex.DecodeString(vm.Address)
	if err != nil {
		errs = append(errs, err)
		return
	}
	bech32Address, err := bech32.ConvertAndEncode(vm.Bech32Prefix, address)
	if err != nil {
		errs = append(errs, err)
		return
	}
	valInfo, err := getSigningInfo(client, bech32Address)
	if err != nil {
		errs = append(errs, err)
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
			errs = append(errs, err)
		} else {
			slashingPeriod := slashingInfo.Params.SignedBlocksWindow
			stats.SlashingPeriodUptime = 100.0 - 100.0*(float64(signingInfo.MissedBlocksCounter)/float64(slashingPeriod))
		}
	}
	node, err := client.GetNode()
	if err != nil {
		errs = append(errs, err)
		return
	}
	status, err := node.Status(context.Background())
	if err != nil {
		errs = append(errs, err)
	} else {
		if status.SyncInfo.CatchingUp {
			errs = append(errs, newOutOfSyncError(vm.RPC))
		}
		stats.Height = status.SyncInfo.LatestBlockHeight
		stats.Timestamp = status.SyncInfo.LatestBlockTime.String()
		stats.RecentMissedBlocks = 0
		for i := stats.Height; i > stats.Height-recentBlocksToCheck; i-- {
			block, err := node.Block(context.Background(), &i)
			if err != nil {
				errs = append(errs, newBlockFetchError(i, vm.RPC))
				continue
			}
			found := false
			for _, voter := range block.Block.LastCommit.Signatures {
				if reflect.DeepEqual(voter.ValidatorAddress, bytes.HexBytes(address)) {
					if block.Block.Height > stats.LastSignedBlockHeight {
						stats.LastSignedBlockHeight = block.Block.Height
						stats.LastSignedBlockTimestamp = block.Block.Time.String()
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
				for i := stats.Height - recentBlocksToCheck; stats.LastSignedBlockHeight == -1; i-- {
					block, err := node.Block(context.Background(), &i)
					if err != nil {
						errs = append(errs, newBlockFetchError(i, vm.RPC))
						break
					}
					for _, voter := range block.Block.LastCommit.Signatures {
						if reflect.DeepEqual(voter.ValidatorAddress, bytes.HexBytes(address)) {
							stats.LastSignedBlockHeight = block.Block.Height
							stats.LastSignedBlockTimestamp = block.Block.Time.String()
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

func getCurrentStatsEmbed(stats ValidatorStats, vm *ValidatorMonitor) discord.Embed {
	var color int
	if stats.Height == stats.LastSignedBlockHeight {
		if stats.RecentMissedBlocks == 0 && stats.SlashingPeriodUptime > 75 {
			color = 0x00FF00
		} else {
			color = 0xFFAC1C
		}

		return discord.Embed{
			Title: fmt.Sprintf("%s (%.02f %% up)", vm.Name, stats.SlashingPeriodUptime),
			Description: fmt.Sprintf("Latest Timestamp: %s\nLatest Height: %d\nMost Recent Signed Blocks: %d/%d",
				stats.Timestamp, stats.Height, recentBlocksToCheck-stats.RecentMissedBlocks, recentBlocksToCheck),
			Color: color,
		}
	}

	if stats.RecentMissedBlocks < recentBlocksToCheck && stats.SlashingPeriodUptime > 75 {
		color = 0xFFAC1C
	} else {
		color = 0xFF0000
	}

	return discord.Embed{
		Title: fmt.Sprintf("%s (%.02f %% up)", vm.Name, stats.SlashingPeriodUptime),
		Description: fmt.Sprintf("Latest Timestamp: %s\nLatest Height: %d\nLast Signed Height: %d\nLast Signed Timestamp: %s\nMost Recent Signed Blocks: %d/%d",
			stats.Timestamp, stats.Height, stats.LastSignedBlockHeight, stats.LastSignedBlockTimestamp, recentBlocksToCheck-stats.RecentMissedBlocks, recentBlocksToCheck),
		Color: color,
	}
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

func sendDiscordAlert(
	vm *ValidatorMonitor,
	stats ValidatorStats,
	alertState *map[string]*ValidatorAlertState,
	discordClient *webhook.Client,
	config *HalfLifeConfig,
	errs []error,
	writeConfigMutex *sync.Mutex,
) {
	if (*alertState)[vm.Name] == nil {
		(*alertState)[vm.Name] = &ValidatorAlertState{
			AlertTypeCounts: make(map[int8]int64),
		}
	}
	var foundAlertTypes []int8
	alertString := ""
	clearedAlertsString := ""
	if len(errs) == 0 {
		// reset all alert type counts so they notify immediately if they occur again
		for key := range (*alertState)[vm.Name].AlertTypeCounts {
			(*alertState)[vm.Name].AlertTypeCounts[key] = 0
		}
	} else {
		for _, err := range errs {
			switch err.(type) {
			case *JailedError:
				foundAlertTypes = append(foundAlertTypes, 1)
				if (*alertState)[vm.Name].AlertTypeCounts[1]%notifyEvery == 0 {
					alertString += err.Error() + "\n"
				}
				(*alertState)[vm.Name].AlertTypeCounts[1]++
			case *TombstonedError:
				foundAlertTypes = append(foundAlertTypes, 2)
				if (*alertState)[vm.Name].AlertTypeCounts[2]%notifyEvery == 0 {
					alertString += err.Error() + "\n"
				}
				(*alertState)[vm.Name].AlertTypeCounts[2]++
			case *OutOfSyncError:
				foundAlertTypes = append(foundAlertTypes, 3)
				if (*alertState)[vm.Name].AlertTypeCounts[3]%notifyEvery == 0 {
					alertString += err.Error() + "\n"
				}
				(*alertState)[vm.Name].AlertTypeCounts[3]++
			case *BlockFetchError:
				foundAlertTypes = append(foundAlertTypes, 4)
				if (*alertState)[vm.Name].AlertTypeCounts[4]%notifyEvery == 0 {
					alertString += err.Error() + "\n"
				}
				(*alertState)[vm.Name].AlertTypeCounts[4]++
			case *MissedRecentBlocksError:
				foundAlertTypes = append(foundAlertTypes, 5)
				if (*alertState)[vm.Name].AlertTypeCounts[5]%notifyEvery == 0 || stats.RecentMissedBlocks > (*alertState)[vm.Name].RecentMissedBlocksCounter {
					alertString += err.Error() + "\n"
				}
				(*alertState)[vm.Name].RecentMissedBlocksCounter = stats.RecentMissedBlocks
				(*alertState)[vm.Name].AlertTypeCounts[5]++
			default:
				alertString += err.Error() + "\n"
			}
		}
	}
	// iterate through all error types
	for i := int8(1); i <= 5; i++ {
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
			case 1:
				clearedAlertsString += "jailed\n"
			case 2:
				clearedAlertsString += "tombstoned\n"
			case 3:
				clearedAlertsString += "out of sync\n"
			case 4:
				clearedAlertsString += "block fetch error\n"
			case 5:
				clearedAlertsString += "missed recent blocks\n"
				(*alertState)[vm.Name].RecentMissedBlocksCounter = 0
			default:
			}
		}
	}
	tagUser := ""
	for _, userID := range config.Discord.AlertUserIDs {
		tagUser += fmt.Sprintf("<@%s> ", userID)
	}

	if alertString != "" {
		_, err := discordClient.CreateMessage(discord.WebhookMessageCreate{
			Username: config.Discord.Username,
			Content:  strings.Trim(tagUser, " "),
			Embeds: []discord.Embed{
				discord.Embed{
					Title:       fmt.Sprintf("%s (%.02f %% up)", vm.Name, stats.SlashingPeriodUptime),
					Description: strings.Trim(alertString, "\n"),
					Color:       0xff0000,
				},
			},
		})
		if err != nil {
			fmt.Printf("Error sending discord message: %v\n", err)
		}
	}
	if clearedAlertsString != "" {
		_, err := discordClient.CreateMessage(discord.WebhookMessageCreate{
			Username: config.Discord.Username,
			Content:  tagUser,
			Embeds: []discord.Embed{
				discord.Embed{
					Title:       fmt.Sprintf("%s (%.02f %% up)", vm.Name, stats.SlashingPeriodUptime),
					Description: fmt.Sprintf("Errors cleared: %s\n", strings.Trim(clearedAlertsString, "\n")),
					Color:       0x00ff00,
				},
			},
		})
		if err != nil {
			fmt.Printf("Error sending discord message: %v\n", err)
		}
	}
	if vm.DiscordStatusMessageID != nil {
		_, err := discordClient.UpdateMessage(snowflake.Snowflake(*vm.DiscordStatusMessageID), discord.WebhookMessageUpdate{
			Embeds: &[]discord.Embed{
				getCurrentStatsEmbed(stats, vm),
			},
		})
		if err != nil {
			fmt.Printf("Error updating discord message: %v\n", err)
		}
	} else {
		message, err := discordClient.CreateMessage(discord.WebhookMessageCreate{
			Username: config.Discord.Username,
			Embeds: []discord.Embed{
				getCurrentStatsEmbed(stats, vm),
			},
		})
		if err != nil {
			fmt.Printf("Error sending discord message: %v\n", err)
		}
		messageID := string(message.ID)
		vm.DiscordStatusMessageID = &messageID
		fmt.Printf("Saved message ID: %s\n", messageID)
		saveConfig(config, writeConfigMutex)
	}
}

func runMonitor(
	wg *sync.WaitGroup,
	alertState *map[string]*ValidatorAlertState,
	discordClient *webhook.Client,
	config *HalfLifeConfig,
	vm *ValidatorMonitor,
	writeConfigMutex *sync.Mutex,
) {
	stats, errs := monitorValidator(vm)
	if errs != nil {
		fmt.Printf("Got validator errors: +%v\n", errs)

	} else {
		fmt.Printf("No errors found for validator: %s\n", vm.Name)
	}
	sendDiscordAlert(vm, stats, alertState, discordClient, config, errs, writeConfigMutex)
	wg.Done()
}

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Daemon to monitor validators",
	Long:  "Monitors validators and pushes alerts to Discord using the configuration in config.yaml",
	Run: func(cmd *cobra.Command, args []string) {
		dat, err := os.ReadFile(configFilePath)
		if err != nil {
			log.Fatalf("Error reading config.yaml: %v", err)
		}
		config := HalfLifeConfig{}
		err = yaml.Unmarshal(dat, &config)
		if err != nil {
			log.Fatalf("Error parsing config.yaml: %v", err)
		}
		writeConfigMutex := sync.Mutex{}
		discordClient := webhook.NewClient(snowflake.Snowflake(config.Discord.Webhook.ID), config.Discord.Webhook.Token)
		alertState := make(map[string]*ValidatorAlertState)
		for {
			wg := sync.WaitGroup{}
			wg.Add(len(config.Validators))
			for _, vm := range config.Validators {
				go runMonitor(&wg, &alertState, discordClient, &config, vm, &writeConfigMutex)
			}
			wg.Wait()
			time.Sleep(30 * time.Second)
		}
	},
}

func init() {
	rootCmd.AddCommand(monitorCmd)
}
