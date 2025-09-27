package keeper

import (
	"context"
	"crypto/tls"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

type RemoteClient struct {
	conn        *grpc.ClientConn
	bankQuery   banktypes.QueryClient
	denomQuery  banktypes.QueryClient
	endpoint    string
	dialed      bool
	tlsInsecure bool
}

func NewRemoteClient(endpoint string) (*RemoteClient, error) {
	rc := &RemoteClient{endpoint: endpoint}
	err := rc.ensureConn()
	if err != nil {
		return nil, err
	}
	return rc, nil
}

func (c *RemoteClient) ensureConn() error {
	if c.dialed {
		return nil
	}
	creds := credentials.NewTLS(&tls.Config{})
	conn, err := grpc.Dial(c.endpoint, grpc.WithTransportCredentials(creds))
	if err != nil {
		return err
	}
	c.conn = conn
	c.bankQuery = banktypes.NewQueryClient(conn)
	c.dialed = true
	return nil
}

func (c *RemoteClient) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *RemoteClient) RemoteBalances(ctx context.Context, addr string) (*banktypes.QueryAllBalancesResponse, error) {
	if err := c.ensureConn(); err != nil {
		return nil, err
	}
	resp, err := c.bankQuery.AllBalances(ctx, &banktypes.QueryAllBalancesRequest{Address: addr})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *RemoteClient) RemoteDenomsMetadata(ctx context.Context) (*banktypes.QueryDenomsMetadataResponse, error) {
	if err := c.ensureConn(); err != nil {
		return nil, err
	}
	resp, err := c.bankQuery.DenomsMetadata(ctx, &banktypes.QueryDenomsMetadataRequest{})
	if err != nil {
		return nil, err
	}
	return resp, nil
}
