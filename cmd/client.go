package cmd

import (
	"context"
	"os"
	"time"

	cosmosClient "github.com/cosmos/cosmos-sdk/client"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	libclient "github.com/tendermint/tendermint/rpc/jsonrpc/client"
)

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
