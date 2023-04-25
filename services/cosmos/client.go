package cosmos

import (
	"context"
	"os"
	"time"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	cosmosClient "github.com/cosmos/cosmos-sdk/client"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/staking4all/celestia-monitoring-bot/services"
)

const (
	sentryGRPCTimeoutSeconds = 5
	RPCTimeoutSeconds        = 5
)

type client struct {
	cosmosClient *cosmosClient.Context
}

func NewCosmosClient(rpcAddress string, chainID string) (services.CosmosClient, error) {
	httpClient, err := cosmosClient.NewClientFromNode(rpcAddress)
	if err != nil {
		return nil, err
	}

	cl := &cosmosClient.Context{
		Client:       httpClient,
		ChainID:      chainID,
		Input:        os.Stdin,
		Output:       os.Stdout,
		OutputFormat: "json",
	}

	return &client{cosmosClient: cl}, nil
}

func (c *client) GetNodeStatus() (*coretypes.ResultStatus, error) {
	node, err := c.cosmosClient.GetNode()
	if err != nil {
		return nil, err
	}
	statusCtx, statusCtxCancel := context.WithTimeout(context.Background(), time.Duration(time.Second*RPCTimeoutSeconds))
	defer statusCtxCancel()
	return node.Status(statusCtx)
}

func (c *client) GetNodeBlock(nonce int64) (*coretypes.ResultBlock, error) {
	// TODO: create block rotation cache by nonce
	node, err := c.cosmosClient.GetNode()
	if err != nil {
		return nil, err
	}
	blockCtx, blockCtxCancel := context.WithTimeout(context.Background(), time.Duration(time.Second*RPCTimeoutSeconds))
	defer blockCtxCancel()
	return node.Block(blockCtx, &nonce)
}

func (c *client) GetSlashingInfo() (*slashingtypes.QueryParamsResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*RPCTimeoutSeconds))
	defer cancel()
	return slashingtypes.NewQueryClient(c.cosmosClient).Params(ctx, &slashingtypes.QueryParamsRequest{})
}

func (c *client) GetSigningInfo(address string) (*slashingtypes.QuerySigningInfoResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*RPCTimeoutSeconds))
	defer cancel()
	return slashingtypes.NewQueryClient(c.cosmosClient).SigningInfo(ctx, &slashingtypes.QuerySigningInfoRequest{
		ConsAddress: address,
	})
}
