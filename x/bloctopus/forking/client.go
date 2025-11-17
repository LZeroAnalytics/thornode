package forking

import (
	"context"
	"crypto/tls"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type RemoteClient struct {
	endpoint string
	conn     *grpc.ClientConn
	bankQ    banktypes.QueryClient
}

func NewRemoteClient(endpoint string) (*RemoteClient, error) {
	creds := credentials.NewTLS(&tls.Config{
		ServerName: "grpc.thor.pfc.zone",
	})
	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}
	return &RemoteClient{
		endpoint: endpoint,
		conn:     conn,
		bankQ:    banktypes.NewQueryClient(conn),
	}, nil
}

func (c *RemoteClient) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *RemoteClient) DenomsMetadata(ctx context.Context) (*banktypes.QueryDenomsMetadataResponse, error) {
	return c.bankQ.DenomsMetadata(ctx, &banktypes.QueryDenomsMetadataRequest{})
}
