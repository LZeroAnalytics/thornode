package keeper

import (
	"context"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type remoteClient struct {
	conn  *grpc.ClientConn
	qBank banktypes.QueryClient
	to    time.Duration
}

func newRemoteClient(cfg Config) (*remoteClient, error) {
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	cc, err := grpc.Dial(cfg.GRPCEndpoint, dialOpts...)
	if err != nil {
		return nil, err
	}
	return &remoteClient{
		conn:  cc,
		qBank: banktypes.NewQueryClient(cc),
		to:    cfg.Timeout,
	}, nil
}

func (r *remoteClient) AllBalances(ctx context.Context, addr string) (sdk.Coins, error) {
	cctx, cancel := context.WithTimeout(ctx, r.to)
	defer cancel()
	resp, err := r.qBank.AllBalances(cctx, &banktypes.QueryAllBalancesRequest{Address: addr})
	if err != nil {
		return nil, nil
	}
	return resp.Balances, nil
}

func (r *remoteClient) Close() error {
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}
