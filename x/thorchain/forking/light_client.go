package forking

import (
	"context"
	"fmt"
	"time"

	"github.com/tendermint/tendermint/light"
	lighthttp "github.com/tendermint/tendermint/light/provider/http"
	dbs "github.com/tendermint/tendermint/light/store/db"
	"github.com/tendermint/tendermint/types"
	dbm "github.com/tendermint/tm-db"
)

type LightClient struct {
	client *light.Client
	config RemoteConfig
}

func NewLightClient(config RemoteConfig) (*LightClient, error) {
	primary, err := lighthttp.New(config.ChainID, config.RPC)
	if err != nil {
		return nil, fmt.Errorf("failed to create light client provider: %w", err)
	}
	
	db := dbm.NewMemDB()
	store := dbs.New(db)
	
	client, err := light.NewClient(
		context.Background(),
		config.ChainID,
		light.TrustOptions{
			Period: config.TrustingPeriod,
			Height: config.TrustHeight,
			Hash:   []byte(config.TrustHash),
		},
		primary,
		[]light.Provider{}, // No witnesses for now
		store,
		light.Logger(nil), // Use default logger
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create light client: %w", err)
	}
	
	return &LightClient{
		client: client,
		config: config,
	}, nil
}

func (lc *LightClient) VerifyHeaderAtHeight(ctx context.Context, height int64) (*types.SignedHeader, error) {
	header, err := lc.client.VerifyHeaderAtHeight(ctx, height, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to verify header at height %d: %w", height, err)
	}
	
	return header, nil
}

func (lc *LightClient) GetTrustedHeader() (*types.SignedHeader, error) {
	return lc.client.TrustedHeader(0)
}

func (lc *LightClient) Update(ctx context.Context) (*types.SignedHeader, error) {
	return lc.client.Update(ctx, time.Now())
}

func (lc *LightClient) Close() error {
	lc.client.Cleanup()
	return nil
}
