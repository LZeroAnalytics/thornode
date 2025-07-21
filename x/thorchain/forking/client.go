package forking

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/store/rootmulti"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	"github.com/cosmos/ics23/go"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	tmhttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
)

type remoteClient struct {
	rpcClient tmclient.Client
	config    RemoteConfig
	codec     codec.Codec
	
	trustedHeight int64
	trustedHash   []byte
}

func NewRemoteClient(config RemoteConfig, codec codec.Codec) (RemoteClient, error) {
	rpcClient, err := tmhttp.New(config.RPC, "/websocket")
	if err != nil {
		return nil, fmt.Errorf("failed to create RPC client: %w", err)
	}
	
	client := &remoteClient{
		rpcClient: rpcClient,
		config:    config,
		codec:     codec,
	}
	
	if err := client.initializeTrustedState(); err != nil {
		return nil, fmt.Errorf("failed to initialize trusted state: %w", err)
	}
	
	return client, nil
}

func (c *remoteClient) initializeTrustedState() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
	defer cancel()
	
	if c.config.TrustHeight > 0 && c.config.TrustHash != "" {
		c.trustedHeight = c.config.TrustHeight
		c.trustedHash = []byte(c.config.TrustHash)
		return nil
	}
	
	result, err := c.rpcClient.Block(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get latest block: %w", err)
	}
	
	c.trustedHeight = result.Block.Height
	c.trustedHash = result.Block.Hash()
	
	return nil
}

func (c *remoteClient) GetWithProof(ctx context.Context, storeKey string, key []byte, height int64) ([]byte, error) {
	path := fmt.Sprintf("store/%s/key", storeKey)
	
	result, err := c.rpcClient.ABCIQueryWithOptions(ctx, path, key, tmclient.ABCIQueryOptions{
		Height: height,
		Prove:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("ABCI query failed: %w", err)
	}
	
	if result.Response.Code != 0 {
		return nil, fmt.Errorf("ABCI query returned error code %d: %s", 
			result.Response.Code, result.Response.Log)
	}
	
	if len(result.Response.Value) == 0 {
		return nil, nil
	}
	
	if err := c.verifyProof(result.Response.ProofOps, storeKey, key, result.Response.Value, height); err != nil {
		return nil, fmt.Errorf("proof verification failed: %w", err)
	}
	
	return result.Response.Value, nil
}

func (c *remoteClient) GetLatestHeight(ctx context.Context) (int64, error) {
	result, err := c.rpcClient.Status(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get status: %w", err)
	}
	
	return result.SyncInfo.LatestBlockHeight, nil
}

func (c *remoteClient) verifyProof(proofOps *types.ProofOps, storeKey string, key, value []byte, height int64) error {
	if proofOps == nil {
		return fmt.Errorf("no proof provided")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
	defer cancel()
	
	result, err := c.rpcClient.Block(ctx, &height)
	if err != nil {
		return fmt.Errorf("failed to get block at height %d: %w", height, err)
	}
	
	if err := c.verifyHeaderChain(result.Block.Header, height); err != nil {
		return fmt.Errorf("header verification failed: %w", err)
	}
	
	ics23Proofs := make([]*ics23.CommitmentProof, len(proofOps.Ops))
	for i, op := range proofOps.Ops {
		ics23Proof := &ics23.CommitmentProof{}
		if err := c.codec.Unmarshal(op.Data, ics23Proof); err != nil {
			return fmt.Errorf("failed to unmarshal proof op %d: %w", i, err)
		}
		ics23Proofs[i] = ics23Proof
	}
	
	root := result.Block.Header.AppHash
	spec := ics23.IavlSpec
	
	if len(value) > 0 {
		err = ics23.VerifyMembership(spec, root, ics23Proofs, key, value)
		if err != nil {
			return fmt.Errorf("membership verification failed: %w", err)
		}
	} else {
		err = ics23.VerifyNonMembership(spec, root, ics23Proofs, key)
		if err != nil {
			return fmt.Errorf("non-membership verification failed: %w", err)
		}
	}
	
	return nil
}

func (c *remoteClient) verifyHeaderChain(header *types.Header, targetHeight int64) error {
	
	if targetHeight < c.trustedHeight {
		return fmt.Errorf("target height %d is less than trusted height %d", 
			targetHeight, c.trustedHeight)
	}
	
	if targetHeight == c.trustedHeight {
		if !header.Hash().Equal(c.trustedHash) {
			return fmt.Errorf("header hash mismatch at trusted height")
		}
		return nil
	}
	
	
	now := time.Now()
	headerTime := header.Time
	if now.Sub(headerTime) > c.config.TrustingPeriod {
		return fmt.Errorf("header is too old: %v > %v", 
			now.Sub(headerTime), c.config.TrustingPeriod)
	}
	
	if headerTime.Sub(now) > c.config.MaxClockDrift {
		return fmt.Errorf("header is too far in the future: %v > %v", 
			headerTime.Sub(now), c.config.MaxClockDrift)
	}
	
	c.trustedHeight = targetHeight
	c.trustedHash = header.Hash()
	
	return nil
}

func (c *remoteClient) Close() error {
	if c.rpcClient != nil {
		return c.rpcClient.Stop()
	}
	return nil
}
