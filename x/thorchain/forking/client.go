package forking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/ics23/go"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cometbft/cometbft/rpc/client"
	"github.com/cometbft/cometbft/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	thorchaintypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type PoolsAPIResponse []Pool

type Pool struct {
	Asset               string `json:"asset"`
	Status              string `json:"status"`
	BalanceRune         string `json:"balance_rune"`
	BalanceAsset        string `json:"balance_asset"`
	LPUnits             string `json:"LP_units"`
	SynthUnits          string `json:"synth_units"`
	PendingInboundRune  string `json:"pending_inbound_rune"`
	PendingInboundAsset string `json:"pending_inbound_asset"`
}

type BalancesAPIResponse struct {
	Result []Amount `json:"result"`
}

type Amount struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

type remoteClient struct {
	rpcClient client.Client
	httpClient *http.Client
	config    RemoteConfig
	codec     codec.Codec
	
	trustedHeight int64
	trustedHash   []byte
}

func NewRemoteClient(config RemoteConfig, codec codec.Codec) (RemoteClient, error) {
	rpcClient, err := rpchttp.New(config.RPC, "/websocket")
	if err != nil {
		return nil, fmt.Errorf("failed to create RPC client: %w", err)
	}
	
	httpClient := &http.Client{
		Timeout: config.Timeout,
	}
	
	client := &remoteClient{
		rpcClient:  rpcClient,
		httpClient: httpClient,
		config:     config,
		codec:      codec,
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
	
	fmt.Printf("DEBUG: GetWithProof called with storeKey=%s, key=%q, height=%d, path=%s\n", storeKey, string(key), height, path)
	
	result, err := c.rpcClient.ABCIQueryWithOptions(ctx, path, key, client.ABCIQueryOptions{
		Height: height,
		Prove:  false,
	})
	if err != nil {
		fmt.Printf("DEBUG: ABCI query failed: %v\n", err)
		return nil, fmt.Errorf("ABCI query failed: %w", err)
	}
	
	fmt.Printf("DEBUG: ABCI query result - Code=%d, Log=%s, ValueLen=%d\n", result.Response.Code, result.Response.Log, len(result.Response.Value))
	
	if result.Response.Code != 0 {
		fmt.Printf("DEBUG: ABCI query returned error code %d: %s\n", result.Response.Code, result.Response.Log)
		return nil, fmt.Errorf("ABCI query returned error code %d: %s", 
			result.Response.Code, result.Response.Log)
	}
	
	if len(result.Response.Value) == 0 {
		fmt.Printf("DEBUG: ABCI query returned empty value\n")
		return nil, nil
	}
	
	fmt.Printf("DEBUG: ABCI query successful, returning %d bytes\n", len(result.Response.Value))
	return result.Response.Value, nil
}

func (c *remoteClient) GetLatestHeight(ctx context.Context) (int64, error) {
	result, err := c.rpcClient.Status(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get status: %w", err)
	}
	
	return result.SyncInfo.LatestBlockHeight, nil
}

func (c *remoteClient) verifyProof(proofOps *cmtproto.ProofOps, storeKey string, key, value []byte, height int64) error {
	if proofOps == nil {
		return fmt.Errorf("no proof provided")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
	defer cancel()
	
	result, err := c.rpcClient.Block(ctx, &height)
	if err != nil {
		return fmt.Errorf("failed to get block at height %d: %w", height, err)
	}
	
	if err := c.verifyHeaderChain(&result.Block.Header, height); err != nil {
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
		if !ics23.VerifyMembership(spec, []byte(root), ics23Proofs[0], key, value) {
			return fmt.Errorf("membership verification failed")
		}
	} else {
		if !ics23.VerifyNonMembership(spec, []byte(root), ics23Proofs[0], key) {
			return fmt.Errorf("non-membership verification failed")
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
		if !bytes.Equal(header.Hash(), c.trustedHash) {
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

func (c *remoteClient) GetRange(ctx context.Context, storeKey string, start, end []byte, height int64) ([]KeyValue, error) {
	var results []KeyValue
	
	fmt.Printf("DEBUG: GetRange called with storeKey=%s, start=%q, end=%q, height=%d\n", storeKey, string(start), string(end), height)
	
	if len(start) == 0 {
		fmt.Printf("DEBUG: GetRange returning empty - start is empty\n")
		return results, nil
	}
	
	startStr := string(start)
	
	if startStr == "pool/" || (len(start) >= 5 && string(start[:5]) == "pool/") {
		fmt.Printf("DEBUG: GetRange detected pool prefix query, calling fetchPoolsFromAPI\n")
		fmt.Printf("DEBUG: API config value: %q\n", c.config.API)
		if c.config.API != "" {
			pools, err := c.fetchPoolsFromAPI(ctx, height)
			if err != nil {
				fmt.Printf("DEBUG: API pool fetch failed: %v, returning partial data\n", err)
				return results, nil // Return partial data on API failure
			}
			fmt.Printf("DEBUG: fetchPoolsFromAPI returned %d pools\n", len(pools))
			return pools, nil
		} else {
			fmt.Printf("DEBUG: API config is empty, skipping API call\n")
		}
	}
	
	if len(start) >= 4 && string(start[:4]) == "bal/" {
		fmt.Printf("DEBUG: GetRange detected balance prefix query\n")
		if c.config.API != "" {
			parts := strings.Split(startStr, "/")
			if len(parts) >= 2 {
				address := parts[1]
				balances, err := c.fetchBalanceFromAPI(ctx, address, height)
				if err != nil {
					fmt.Printf("DEBUG: API balance fetch failed: %v, returning partial data\n", err)
					return results, nil // Return partial data on API failure
				}
				fmt.Printf("DEBUG: fetchBalanceFromAPI returned %d balances\n", len(balances))
				return balances, nil
			}
		}
	}
	
	fmt.Printf("DEBUG: GetRange doing individual key fetch for %q\n", startStr)
	value, err := c.GetWithProof(ctx, storeKey, start, height)
	if err == nil && value != nil {
		results = append(results, KeyValue{
			Key:   start,
			Value: value,
		})
	}
	fmt.Printf("DEBUG: Individual key fetch returned %d results, err=%v\n", len(results), err)
	
	return results, nil
}


func (c *remoteClient) fetchPoolsFromAPI(ctx context.Context, height int64) ([]KeyValue, error) {
	var results []KeyValue
	
	fmt.Printf("DEBUG: fetchPoolsFromAPI called with height=%d\n", height)
	
	if c.config.API == "" {
		fmt.Printf("DEBUG: API endpoint not configured, returning empty results\n")
		return results, nil
	}
	
	url := fmt.Sprintf("%s/thorchain/pools", c.config.API)
	
	fmt.Printf("DEBUG: Making API request to %s\n", url)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		fmt.Printf("DEBUG: Failed to create request: %v\n", err)
		return results, nil // Return partial data on API failure
	}
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		fmt.Printf("DEBUG: API request failed: %v, returning partial data\n", err)
		return results, nil // Return partial data on API failure
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("DEBUG: API returned status %d, returning partial data\n", resp.StatusCode)
		return results, nil // Return partial data on API failure
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("DEBUG: Failed to read response: %v, returning partial data\n", err)
		return results, nil // Return partial data on API failure
	}
	
	var pools PoolsAPIResponse
	if err := json.Unmarshal(body, &pools); err != nil {
		fmt.Printf("DEBUG: Failed to unmarshal pools: %v, returning partial data\n", err)
		return results, nil // Return partial data on API failure
	}
	
	fmt.Printf("DEBUG: Successfully fetched %d pools from API\n", len(pools))
	
	poolPrefix := []byte("pool/")
	for _, pool := range pools {
		key := append(poolPrefix, []byte(pool.Asset)...)
		value, err := c.convertPoolToProtobuf(pool)
		if err != nil {
			fmt.Printf("DEBUG: Failed to convert pool %s to protobuf: %v\n", pool.Asset, err)
			continue
		}
		results = append(results, KeyValue{
			Key:   key,
			Value: value,
		})
	}
	
	fmt.Printf("DEBUG: fetchPoolsFromAPI returning %d pools\n", len(results))
	return results, nil
}

func (c *remoteClient) fetchBalanceFromAPI(ctx context.Context, address string, height int64) ([]KeyValue, error) {
	var results []KeyValue
	
	fmt.Printf("DEBUG: fetchBalanceFromAPI called with address=%s, height=%d\n", address, height)
	
	if c.config.API == "" {
		fmt.Printf("DEBUG: API endpoint not configured, returning empty results\n")
		return results, nil
	}
	
	url := fmt.Sprintf("%s/cosmos/bank/v1beta1/balances/%s", c.config.API, address)
	
	fmt.Printf("DEBUG: Making API request to %s\n", url)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		fmt.Printf("DEBUG: Failed to create request: %v\n", err)
		return results, nil // Return partial data on API failure
	}
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		fmt.Printf("DEBUG: API request failed: %v, returning partial data\n", err)
		return results, nil // Return partial data on API failure
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("DEBUG: API returned status %d, returning partial data\n", resp.StatusCode)
		return results, nil // Return partial data on API failure
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("DEBUG: Failed to read response: %v, returning partial data\n", err)
		return results, nil // Return partial data on API failure
	}
	
	var balances BalancesAPIResponse
	if err := json.Unmarshal(body, &balances); err != nil {
		fmt.Printf("DEBUG: Failed to unmarshal balances: %v, returning partial data\n", err)
		return results, nil // Return partial data on API failure
	}
	
	fmt.Printf("DEBUG: Successfully fetched %d balances from API\n", len(balances.Result))
	
	for _, balance := range balances.Result {
		key := []byte(fmt.Sprintf("bal/%s/%s", address, balance.Denom))
		value, err := c.convertBalanceToProtobuf(balance)
		if err != nil {
			fmt.Printf("DEBUG: Failed to convert balance %s to protobuf: %v\n", balance.Denom, err)
			continue
		}
		results = append(results, KeyValue{
			Key:   key,
			Value: value,
		})
	}
	
	fmt.Printf("DEBUG: fetchBalanceFromAPI returning %d balances\n", len(results))
	return results, nil
}

func (c *remoteClient) convertPoolToProtobuf(pool Pool) ([]byte, error) {
	fmt.Printf("DEBUG: convertPoolToProtobuf called for pool %s\n", pool.Asset)
	
	asset, err := common.NewAsset(pool.Asset)
	if err != nil {
		fmt.Printf("DEBUG: failed to parse asset %s: %v\n", pool.Asset, err)
		return nil, fmt.Errorf("failed to parse asset %s: %w", pool.Asset, err)
	}
	
	status := thorchaintypes.PoolStatus_Available
	switch strings.ToLower(pool.Status) {
	case "available":
		status = thorchaintypes.PoolStatus_Available
	case "staged":
		status = thorchaintypes.PoolStatus_Staged
	case "suspended":
		status = thorchaintypes.PoolStatus_Suspended
	default:
		status = thorchaintypes.PoolStatus_Available
	}
	
	balanceRune := cosmos.ZeroUint()
	if pool.BalanceRune != "" {
		balanceRune = cosmos.NewUintFromString(pool.BalanceRune)
	}
	
	balanceAsset := cosmos.ZeroUint()
	if pool.BalanceAsset != "" {
		balanceAsset = cosmos.NewUintFromString(pool.BalanceAsset)
	}
	
	lpUnits := cosmos.ZeroUint()
	if pool.LPUnits != "" {
		lpUnits = cosmos.NewUintFromString(pool.LPUnits)
	}
	
	synthUnits := cosmos.ZeroUint()
	if pool.SynthUnits != "" {
		synthUnits = cosmos.NewUintFromString(pool.SynthUnits)
	}
	
	pendingInboundRune := cosmos.ZeroUint()
	if pool.PendingInboundRune != "" {
		pendingInboundRune = cosmos.NewUintFromString(pool.PendingInboundRune)
	}
	
	pendingInboundAsset := cosmos.ZeroUint()
	if pool.PendingInboundAsset != "" {
		pendingInboundAsset = cosmos.NewUintFromString(pool.PendingInboundAsset)
	}
	
	thorPool := &thorchaintypes.Pool{
		BalanceRune:         balanceRune,
		BalanceAsset:        balanceAsset,
		Asset:               asset,
		LPUnits:             lpUnits,
		SynthUnits:          synthUnits,
		PendingInboundRune:  pendingInboundRune,
		PendingInboundAsset: pendingInboundAsset,
		Status:              status,
		Decimals:            8, // Default decimals for most assets
	}
	
	data, err := c.codec.Marshal(thorPool)
	if err != nil {
		fmt.Printf("DEBUG: failed to marshal pool %s: %v\n", pool.Asset, err)
		return nil, fmt.Errorf("failed to marshal pool %s: %w", pool.Asset, err)
	}
	
	fmt.Printf("DEBUG: convertPoolToProtobuf returning %d bytes for pool %s\n", len(data), pool.Asset)
	return data, nil
}

func (c *remoteClient) convertBalanceToProtobuf(balance Amount) ([]byte, error) {
	fmt.Printf("DEBUG: convertBalanceToProtobuf called for balance %s:%s\n", balance.Denom, balance.Amount)
	
	data, err := json.Marshal(balance)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal balance %s: %w", balance.Denom, err)
	}
	
	fmt.Printf("DEBUG: convertBalanceToProtobuf returning %d bytes for balance %s\n", len(data), balance.Denom)
	return data, nil
}

func (c *remoteClient) Close() error {
	if c.rpcClient != nil {
		return c.rpcClient.Stop()
	}
	return nil
}
