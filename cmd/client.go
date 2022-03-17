package cmd

import (
	"context"
	"os"
	"time"

	cosmosClient "github.com/cosmos/cosmos-sdk/client"
	tmservice "github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	libclient "github.com/tendermint/tendermint/rpc/jsonrpc/client"
	"google.golang.org/grpc"
)

const (
	sentryGRPCTimeoutSeconds = 5
	RPCTimeoutSeconds        = 5
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

func getCosmosClient(rpcAddress string, chainID string) (*cosmosClient.Context, error) {
	client, err := newClient(rpcAddress)
	if err != nil {
		return nil, err
	}
	return &cosmosClient.Context{
		Client:       client,
		ChainID:      chainID,
		Input:        os.Stdin,
		Output:       os.Stdout,
		OutputFormat: "json",
	}, nil
}

func getSlashingInfo(client *cosmosClient.Context) (*slashingtypes.QueryParamsResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*RPCTimeoutSeconds))
	defer cancel()
	return slashingtypes.NewQueryClient(client).Params(ctx, &slashingtypes.QueryParamsRequest{})
}

func getSigningInfo(client *cosmosClient.Context, address string) (*slashingtypes.QuerySigningInfoResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*RPCTimeoutSeconds))
	defer cancel()
	return slashingtypes.NewQueryClient(client).SigningInfo(ctx, &slashingtypes.QuerySigningInfoRequest{
		ConsAddress: address,
	})
}

func getSentryInfo(grpcAddr string) (*tmservice.GetNodeInfoResponse, *tmservice.GetLatestBlockResponse, error) {
	conn, err := grpc.Dial(grpcAddr, grpc.WithInsecure())
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()
	serviceClient := tmservice.NewServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*sentryGRPCTimeoutSeconds))
	defer cancel()
	nodeInfo, err := serviceClient.GetNodeInfo(ctx, &tmservice.GetNodeInfoRequest{})
	if err != nil {
		return nil, nil, err
	}
	syncingInfo, err := serviceClient.GetLatestBlock(ctx, &tmservice.GetLatestBlockRequest{})
	if err != nil {
		return nil, nil, err
	}
	return nodeInfo, syncingInfo, nil
}
