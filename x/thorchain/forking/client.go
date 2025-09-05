package forking

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"encoding/binary"
	"encoding/hex"

	storepb "cosmossdk.io/api/cosmos/store/v1beta1"
	sdkmath "cosmossdk.io/math"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protowire"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/types/query"

	"google.golang.org/grpc/status"
	"google.golang.org/grpc/codes"
)

type remoteClient struct {
	grpcConn    *grpc.ClientConn
	queryClient types.QueryClient
	wasmClient  wasmtypes.QueryClient
	config      RemoteConfig
	codec       codec.Codec
}

func NewRemoteClient(config RemoteConfig, cdc codec.Codec) (RemoteClient, error) {
	target := strings.TrimSpace(config.GRPC)

	useTLS := false
	hostForTLS := ""
	normalized := target

	if strings.HasPrefix(target, "grpcs://") {
		useTLS = true
		normalized = strings.TrimPrefix(target, "grpcs://")
	} else if strings.HasPrefix(target, "https://") {
		useTLS = true
		normalized = strings.TrimPrefix(target, "https://")
	}

	if !useTLS {
		if h, p, err := net.SplitHostPort(normalized); err == nil {
			if p == "443" {
				useTLS = true
				hostForTLS = h
			}
		}
	}

	var dialOpt grpc.DialOption
	if useTLS {
		if hostForTLS == "" {
			if h, _, err := net.SplitHostPort(normalized); err == nil {
				hostForTLS = h
			} else {
				hostForTLS = normalized
			}
		}
		tlsCfg := &tls.Config{
			ServerName: hostForTLS,
			MinVersion: tls.VersionTLS12,
		}
		creds := credentials.NewTLS(tlsCfg)
		dialOpt = grpc.WithTransportCredentials(creds)
	} else {
		dialOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	conn, err := grpc.Dial(normalized, dialOpt)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	client := types.NewQueryClient(conn)
	wq := wasmtypes.NewQueryClient(conn)

	cli := &remoteClient{
		grpcConn:    conn,
		queryClient: client,
		wasmClient:  wq,
		config:      config,
		codec:       cdc,
	}
	return cli, nil
}

func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	if status.Code(err) == codes.NotFound {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "not found") || strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "doesnt exist") {
		return true
	}
	return false
}
func shouldRetryWithoutHeight(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "invalid height") {
		return true
	}
	if strings.Contains(msg, "version mismatch") {
		return true
	}
	if strings.Contains(msg, "pruned") {
		return true
	}
	return false
}


func (c *remoteClient) ctxWithHeight(ctx context.Context, height int64) context.Context {
	safe := height
	if height > 0 {
		if latest, err := c.GetLatestHeight(ctx); err == nil && latest > 0 && height > latest {
			safe = latest
		}
	}
	if safe > 0 {
		md := metadata.Pairs("x-cosmos-block-height", fmt.Sprintf("%d", safe))
		return metadata.NewOutgoingContext(ctx, md)
	}
	return ctx
}

func (c *remoteClient) GetWithProof(ctx context.Context, storeKey string, key []byte, height int64) ([]byte, error) {
	return c.fetchViaGRPC(ctx, storeKey, key, height)
}

func (c *remoteClient) fetchViaGRPC(ctx context.Context, storeKey string, key []byte, height int64) ([]byte, error) {
	keyStr := string(key)
	lkey := strings.ToLower(keyStr)
	lstore := strings.ToLower(storeKey)
	if lstore == strings.ToLower(wasmtypes.StoreKey) {
		if len(key) == 0 {
			return nil, nil
		}
		switch key[0] {
		case 0x02: // ContractInfo: 0x02 | addr (no length prefix)
			b := key[1:]
			addr, ok := c.parseWasmContractAddrStrict(b)
			if !ok {
				fmt.Printf("[forking][wasm][0x02] failed to parse addr from key=%s\n", hex.EncodeToString(key))
				return nil, nil
			}
			fmt.Printf("[forking][wasm][0x02] parsed addr=%s from key=%s\n", addr, hex.EncodeToString(key))
			resp, err := c.wasmClient.ContractInfo(c.ctxWithHeight(ctx, height), &wasmtypes.QueryContractInfoRequest{Address: addr})
			if err != nil {
				if shouldRetryWithoutHeight(err) {
					resp, err = c.wasmClient.ContractInfo(ctx, &wasmtypes.QueryContractInfoRequest{Address: addr})
				}
			}
			if err != nil {
				low := strings.ToLower(err.Error())
				if isNotFoundErr(err) || strings.Contains(low, "no such contract") {
					fmt.Printf("[forking][wasm][0x02] remote miss for addr=%s err=%v\n", addr, err)
					return nil, nil
				}
				fmt.Printf("[forking][wasm][0x02] remote error for addr=%s err=%v\n", addr, err)
				return nil, fmt.Errorf("wasm ContractInfo: %w", err)
			}
			if resp == nil {
				fmt.Printf("[forking][wasm][0x02] empty response for addr=%s\n", addr)
				return nil, nil
			}
			fmt.Printf("[forking][wasm][0x02] remote success for addr=%s code_id=%d\n", addr, resp.ContractInfo.CodeID)
			return c.codec.Marshal(&resp.ContractInfo)
		case 0x01: // CodeInfo: 0x01 | codeID(8 bytes, big-endian)
			if codeID, ok := c.parseWasmCodeID(key[1:]); ok {
				resp, err := c.wasmClient.Code(c.ctxWithHeight(ctx, height), &wasmtypes.QueryCodeRequest{CodeId: codeID})
				if err != nil {
					if shouldRetryWithoutHeight(err) {
						resp, err = c.wasmClient.Code(ctx, &wasmtypes.QueryCodeRequest{CodeId: codeID})
					}
				}
				if err != nil {
					if isNotFoundErr(err) || strings.Contains(strings.ToLower(err.Error()), "no such code") {
						return nil, nil
					}
					return nil, fmt.Errorf("wasm CodeInfo: %w", err)
				}
				if resp == nil {
					return nil, nil
				}
				ci := wasmtypes.CodeInfo{
					CodeHash:          resp.DataHash,
					Creator:           resp.Creator,
					InstantiateConfig: resp.InstantiatePermission,
				}
				return c.codec.Marshal(&ci)
			}
			return nil, nil
		case 0x03:
			if addr, suffix, ok := c.parseWasmContractStoreKeyNoLen(key[1:]); ok {
				if len(suffix) == 0 {
					return nil, nil
				}
				resp, err := c.wasmClient.RawContractState(c.ctxWithHeight(ctx, height), &wasmtypes.QueryRawContractStateRequest{
					Address:   addr,
					QueryData: suffix,
				})
				if err != nil {
					if shouldRetryWithoutHeight(err) {
						resp, err = c.wasmClient.RawContractState(ctx, &wasmtypes.QueryRawContractStateRequest{
							Address:   addr,
							QueryData: suffix,
						})
					}
				}
				if err != nil {
					low := strings.ToLower(err.Error())
					if isNotFoundErr(err) || strings.Contains(low, "no such contract") {
						items, aerr := c.fetchAllContractState(ctx, addr, height, key[0])
						if aerr == nil && len(items) > 0 {
							for _, kv := range items {
								if bytes.Equal(kv.Key, key) {
									return kv.Value, nil
								}
							}
						}
						return nil, nil
					}
					return nil, fmt.Errorf("wasm RawContractState: %w", err)
				}
				if resp == nil || len(resp.Data) == 0 {
					items, aerr := c.fetchAllContractState(ctx, addr, height, key[0])
					if aerr == nil && len(items) > 0 {
						for _, kv := range items {
							if bytes.Equal(kv.Key, key) {
								return kv.Value, nil
							}
						}
					}
					return nil, nil
				}
				return resp.Data, nil
			}
			if codeID, ok := c.parseWasmCodeID(key[1:]); ok {
				resp, err := c.wasmClient.Code(c.ctxWithHeight(ctx, height), &wasmtypes.QueryCodeRequest{CodeId: codeID})
				if err != nil {
					if shouldRetryWithoutHeight(err) {
						resp, err = c.wasmClient.Code(ctx, &wasmtypes.QueryCodeRequest{CodeId: codeID})
					}
				}
				if err != nil {
					if isNotFoundErr(err) || strings.Contains(strings.ToLower(err.Error()), "no such code") {
						return nil, nil
					}
					return nil, fmt.Errorf("wasm CodeBytes: %w", err)
				}
				if resp == nil || len(resp.Data) == 0 {
					return nil, nil
				}
				return resp.Data, nil
			}
			return nil, nil
		case 0x05: // ContractStore: 0x05 | addr | key...
			if addr, suffix, ok := c.parseWasmContractStoreKeyNoLen(key[1:]); ok {
				if len(suffix) == 0 {
					return nil, nil
				}
				resp, err := c.wasmClient.RawContractState(c.ctxWithHeight(ctx, height), &wasmtypes.QueryRawContractStateRequest{
					Address:   addr,
					QueryData: suffix,
				})
				if err != nil {
					if shouldRetryWithoutHeight(err) {
						resp, err = c.wasmClient.RawContractState(ctx, &wasmtypes.QueryRawContractStateRequest{
							Address:   addr,
							QueryData: suffix,
						})
					}
				}
				if err != nil {
					low := strings.ToLower(err.Error())
					if isNotFoundErr(err) || strings.Contains(low, "no such contract") {
						items, aerr := c.fetchAllContractState(ctx, addr, height, key[0])
						if aerr == nil && len(items) > 0 {
							for _, kv := range items {
								if bytes.Equal(kv.Key, key) {
									return kv.Value, nil
								}
							}
						}
						return nil, nil
					}
					return nil, fmt.Errorf("wasm RawContractState: %w", err)
				}
				if resp == nil || len(resp.Data) == 0 {
					items, aerr := c.fetchAllContractState(ctx, addr, height, key[0])
					if aerr == nil && len(items) > 0 {
						for _, kv := range items {
							if bytes.Equal(kv.Key, key) {
								return kv.Value, nil
							}
						}
					}
					return nil, nil
				}
				return resp.Data, nil
			}
			return nil, nil
		default:
			return nil, nil
		}
	}


	switch {

	case strings.Contains(lkey, "mimir//"):
		return c.fetchMimirData(ctx, keyStr, height)
	case strings.Contains(lkey, "ragnarok"):
		return c.fetchRagnarokData(ctx, height)
	case strings.Contains(lkey, "pool/") || strings.Contains(lstore, "pool"):
		return c.fetchPoolData(ctx, keyStr, height)
	case strings.Contains(lkey, "account") || strings.Contains(lstore, "account"):
		return c.fetchAccountData(ctx, keyStr, height)
	case strings.Contains(lkey, "balance") || strings.Contains(lstore, "bank"):
		return c.fetchBalanceData(ctx, keyStr, height)
	case strings.Contains(lkey, "node_account") || strings.Contains(lstore, "node"):
		return c.fetchNodeData(ctx, keyStr, height)
	case strings.Contains(lkey, "lp/") || strings.Contains(lstore, "lp"):
		return c.fetchLPData(ctx, keyStr, height)
	case strings.Contains(lkey, "loan/") || strings.Contains(lstore, "loan"):
		return c.fetchBorrowerData(ctx, keyStr, height)
	case strings.Contains(lkey, "saver/") || strings.Contains(lstore, "saver"):
		return c.fetchSaverData(ctx, keyStr, height)
	default:
		return nil, nil
	}
}

func (c *remoteClient) fetchPoolData(ctx context.Context, key string, height int64) ([]byte, error) {
	assetStr := c.extractAssetFromPoolKey(key)
	if assetStr != "" {
		if a, err := common.NewAsset(assetStr); err == nil {
			if strings.EqualFold(a.Chain.String(), "THOR") {
				return nil, nil
			}
			req := &types.QueryPoolRequest{
				Asset:  assetStr,
				Height: fmt.Sprintf("%d", height),
			}
			single, err := c.queryClient.Pool(ctx, req)
			if err != nil {
				if isNotFoundErr(err) {
					return nil, nil
				}
				return nil, fmt.Errorf("gRPC pool query failed: %w", err)
			}

			asset, err := common.NewAsset(single.Asset)
			if err != nil {
				return nil, fmt.Errorf("invalid asset in pool response: %w", err)
			}

			br := sdkmath.NewUintFromString(single.BalanceRune)
			ba := sdkmath.NewUintFromString(single.BalanceAsset)
			lpu := sdkmath.NewUintFromString(single.LPUnits)
			su := sdkmath.NewUintFromString(single.SynthUnits)
			pir := sdkmath.NewUintFromString(single.PendingInboundRune)
			pia := sdkmath.NewUintFromString(single.PendingInboundAsset)

			var status types.PoolStatus
			switch strings.ToLower(single.Status) {
			case "available":
				status = types.PoolStatus_Available
			case "staged":
				status = types.PoolStatus_Staged
			case "suspended":
				status = types.PoolStatus_Suspended
			default:
				status = types.PoolStatus_UnknownPoolStatus
			}

			record := types.Pool{
				BalanceRune:         br,
				BalanceAsset:        ba,
				Asset:               asset,
				LPUnits:             lpu,
				Status:              status,
				StatusSince:         0,
				Decimals:            single.Decimals,
				SynthUnits:          su,
				PendingInboundRune:  pir,
				PendingInboundAsset: pia,
			}

			return c.codec.Marshal(&record)
		}
		return nil, nil
	}

	reqPools := &types.QueryPoolsRequest{
		Height: fmt.Sprintf("%d", height),
	}
	respPools, err := c.queryClient.Pools(ctx, reqPools)
	if err != nil {
		return nil, fmt.Errorf("gRPC pools query failed: %w", err)
	}
	return c.codec.Marshal(respPools)

}

func (c *remoteClient) fetchAccountData(ctx context.Context, key string, height int64) ([]byte, error) {
	address := c.extractAddressFromKey(key)
	if address == "" {
		return nil, nil
	}
	req := &types.QueryAccountRequest{
		Address: address,
	}
	resp, err := c.queryClient.Account(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC account query failed: %w", err)
	}
	return c.codec.Marshal(resp)
}

func (c *remoteClient) fetchLPData(ctx context.Context, key string, height int64) ([]byte, error) {
	assetStr, addr := c.extractLPFromKey(key)
	if assetStr == "" || addr == "" {
		return nil, nil
	}
	req := &types.QueryLiquidityProviderRequest{
		Asset:   assetStr,
		Address: addr,
		Height:  fmt.Sprintf("%d", height),
	}
	lpResp, err := c.queryClient.LiquidityProvider(ctx, req)
	if err != nil || lpResp == nil || lpResp.Asset == "" {
		lpsReq := &types.QueryLiquidityProvidersRequest{
			Asset:  assetStr,
			Height: fmt.Sprintf("%d", height),
		}
		lpsResp, lerr := c.queryClient.LiquidityProviders(ctx, lpsReq)
		if lerr != nil {
			if isNotFoundErr(lerr) {
				return nil, nil
			}
			return nil, fmt.Errorf("gRPC liquidity providers fallback failed: %w", lerr)
		}
		ua := strings.ToUpper(addr)
		for _, it := range lpsResp.LiquidityProviders {
			if strings.ToUpper(it.RuneAddress) == ua || strings.ToUpper(it.AssetAddress) == ua {
				lpResp = it
				break
			}
		}
		if lpResp == nil || lpResp.Asset == "" {
			return nil, nil
		}
	}
	asset, err := common.NewAsset(lpResp.Asset)
	if err != nil {
		return nil, nil
	}
	var runeAddr, assetAddr common.Address
	if lpResp.RuneAddress != "" {
		runeAddr, _ = common.NewAddress(lpResp.RuneAddress)
	}
	if lpResp.AssetAddress != "" {
		assetAddr, _ = common.NewAddress(lpResp.AssetAddress)
	}
	record := types.LiquidityProvider{
		Asset:              asset,
		RuneAddress:        runeAddr,
		AssetAddress:       assetAddr,
		LastAddHeight:      lpResp.LastAddHeight,
		LastWithdrawHeight: lpResp.LastWithdrawHeight,
		Units:              sdkmath.NewUintFromString(lpResp.Units),
		PendingRune:        sdkmath.NewUintFromString(lpResp.PendingRune),
		PendingAsset:       sdkmath.NewUintFromString(lpResp.PendingAsset),
		RuneDepositValue:   sdkmath.NewUintFromString(lpResp.RuneDepositValue),
		AssetDepositValue:  sdkmath.NewUintFromString(lpResp.AssetDepositValue),
	}
	return c.codec.Marshal(&record)
}

func (c *remoteClient) fetchSaverData(ctx context.Context, key string, height int64) ([]byte, error) {
	assetStr, addr := c.extractSaverFromKey(key)
	if assetStr == "" || addr == "" {
		return nil, nil
	}
	req := &types.QuerySaverRequest{
		Asset:   assetStr,
		Address: addr,
		Height:  fmt.Sprintf("%d", height),
	}
	resp, err := c.queryClient.Saver(ctx, req)
	if err != nil || resp == nil || resp.Asset == "" {
		listReq := &types.QuerySaversRequest{
			Asset:  assetStr,
			Height: fmt.Sprintf("%d", height),
		}
		listResp, lerr := c.queryClient.Savers(ctx, listReq)
		if lerr != nil {
			if isNotFoundErr(lerr) {
				return nil, nil
			}
			return nil, fmt.Errorf("gRPC savers fallback failed: %w", lerr)
		}
		ua := strings.ToUpper(addr)
		var found *types.QuerySaverResponse
		for _, s := range listResp.Savers {
			if strings.ToUpper(s.AssetAddress) == ua {
				found = s
				break
			}
		}
		if found == nil {
			return nil, nil
		}
		return c.codec.Marshal(found)
	}
	return c.codec.Marshal(resp)
}

func (c *remoteClient) extractSaverFromKey(key string) (string, string) {
	lower := strings.ToLower(key)
	idx := strings.Index(lower, "saver/")
	if idx == -1 {
		return "", ""
	}
	raw := strings.TrimLeft(key[idx+len("saver/"):], "/")
	if raw == "" {
		return "", ""
	}
	last := strings.LastIndex(raw, "/")
	if last == -1 {
		return "", ""
	}
	assetPart := raw[:last]
	addr := raw[last+1:]
	assetPart = c.normalizeAssetFromKeyAsset(assetPart)
	return assetPart, addr
}

func (c *remoteClient) fetchBorrowerData(ctx context.Context, key string, height int64) ([]byte, error) {
	assetStr, addr := c.extractBorrowerFromKey(key)
	if assetStr == "" || addr == "" {
		return nil, nil
	}
	req := &types.QueryBorrowerRequest{
		Asset:   assetStr,
		Address: addr,
		Height:  fmt.Sprintf("%d", height),
	}
	bResp, err := c.queryClient.Borrower(ctx, req)
	if err != nil || bResp == nil || bResp.Asset == "" {
		listReq := &types.QueryBorrowersRequest{
			Asset:  assetStr,
			Height: fmt.Sprintf("%d", height),
		}
		listResp, lerr := c.queryClient.Borrowers(ctx, listReq)
		if lerr != nil {
			if isNotFoundErr(lerr) {
				return nil, nil
			}
			return nil, fmt.Errorf("gRPC borrowers fallback failed: %w", lerr)
		}
		ua := strings.ToUpper(addr)
		for _, b := range listResp.Borrowers {
			if strings.ToUpper(b.Owner) == ua {
				bResp = b
				break
			}
		}
		if bResp == nil || bResp.Asset == "" {
			return nil, nil
		}
	}
	asset, err := common.NewAsset(bResp.Asset)
	if err != nil {
		return nil, nil
	}
	var owner common.Address
	if bResp.Owner != "" {
		owner, _ = common.NewAddress(bResp.Owner)
	}
	record := types.Loan{
		Owner:               owner,
		Asset:               asset,
		DebtIssued:          sdkmath.NewUintFromString(bResp.DebtIssued),
		DebtRepaid:          sdkmath.NewUintFromString(bResp.DebtRepaid),
		LastOpenHeight:      bResp.LastOpenHeight,
		CollateralDeposited: sdkmath.NewUintFromString(bResp.CollateralDeposited),
		CollateralWithdrawn: sdkmath.NewUintFromString(bResp.CollateralWithdrawn),
	}
	return c.codec.Marshal(&record)
}

func (c *remoteClient) extractLPFromKey(key string) (string, string) {
	lower := strings.ToLower(key)
	idx := strings.Index(lower, "lp/")
	if idx == -1 {
		return "", ""
	}
	raw := strings.TrimLeft(key[idx+len("lp/"):], "/")
	if raw == "" {
		return "", ""
	}
	last := strings.LastIndex(raw, "/")
	if last == -1 {
		return "", ""
	}
	assetPart := raw[:last]
	addr := raw[last+1:]
	assetPart = c.normalizeAssetFromKeyAsset(assetPart)
	return assetPart, addr
}

func (c *remoteClient) extractBorrowerFromKey(key string) (string, string) {
	lower := strings.ToLower(key)
	idx := strings.Index(lower, "loan/")
	if idx == -1 {
		return "", ""
	}
	raw := strings.TrimLeft(key[idx+len("loan/"):], "/")
	if raw == "" {
		return "", ""
	}
	last := strings.LastIndex(raw, "/")
	if last == -1 {
		return "", ""
	}
	assetPart := raw[:last]
	addr := raw[last+1:]
	assetPart = c.normalizeAssetFromKeyAsset(assetPart)
	return assetPart, addr
}

func (c *remoteClient) normalizeAssetFromKeyAsset(s string) string {
	if strings.Contains(s, "/") {
		parts := strings.SplitN(s, "/", 2)
		left := parts[0]
		right := parts[1]
		if strings.HasPrefix(right, "0x") || strings.HasPrefix(right, "0X") {
			right = "0X" + strings.ToUpper(right[2:])
		}
		return left + "." + right
	}
	return s
}


func (c *remoteClient) fetchBalanceData(ctx context.Context, key string, height int64) ([]byte, error) {
	address := c.extractAddressFromKey(key)
	if address == "" {
		return nil, nil
	}
	
	req := &types.QueryBalancesRequest{
		Address: address,
	}
	
	resp, err := c.queryClient.Balances(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC balances query failed: %w", err)
	}
	
	return c.codec.Marshal(resp)
}

func (c *remoteClient) fetchNodeData(ctx context.Context, key string, height int64) ([]byte, error) {
	lower := strings.ToLower(key)
	if idx := strings.Index(lower, "node_account/"); idx != -1 {
		rest := strings.TrimPrefix(key[idx+len("node_account/"):], "/")
		if rest != "" && !strings.Contains(rest, "/") {
			req := &types.QueryNodeRequest{
				Address: rest,
				Height:  fmt.Sprintf("%d", height),
			}
			single, err := c.queryClient.Node(ctx, req)
			if err != nil {
				if isNotFoundErr(err) {
					return nil, nil
				}
				return nil, fmt.Errorf("gRPC node query failed: %w", err)
			}
			naAddr, err := common.NewAddress(single.NodeAddress)
			if err != nil {
				return nil, nil
			}
			accAddr, err := cosmos.AccAddressFromBech32(single.NodeAddress)
			if err != nil {
				return nil, nil
			}
			var status types.NodeStatus
			switch strings.ToLower(single.Status) {
			case "whitelisted":
				status = types.NodeStatus_Whitelisted
			case "standby":
				status = types.NodeStatus_Standby
			case "ready":
				status = types.NodeStatus_Ready
			case "active":
				status = types.NodeStatus_Active
			case "disabled":
				status = types.NodeStatus_Disabled
			default:
				status = types.NodeStatus_Unknown
			}
			bond := sdkmath.NewUintFromString(single.TotalBond)
			bondAddr, err := common.NewAddress(single.NodeOperatorAddress)
			if err != nil {
				bondAddr = common.NoAddress
			}
			record := types.NodeAccount{
				NodeAddress:         accAddr,
				Status:              status,
				PubKeySet:           single.PubKeySet,
				ValidatorConsPubKey: single.ValidatorConsPubKey,
				Bond:                bond,
				ActiveBlockHeight:   single.ActiveBlockHeight,
				BondAddress:         bondAddr,
				StatusSince:         single.StatusSince,
				SignerMembership:    single.SignerMembership,
				RequestedToLeave:    single.RequestedToLeave,
				ForcedToLeave:       single.ForcedToLeave,
				IPAddress:           single.IpAddress,
				Version:             single.Version,
				MissingBlocks:       uint64(single.MissingBlocks),
				Maintenance:         single.Maintenance,
			}
			_ = naAddr
			return c.codec.Marshal(&record)
		}
	}
	req := &types.QueryNodesRequest{
		Height: fmt.Sprintf("%d", height),
	}
	resp, err := c.queryClient.Nodes(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC nodes query failed: %w", err)
	}
	return c.codec.Marshal(resp)
}

func (c *remoteClient) fetchMimirData(ctx context.Context, key string, height int64) ([]byte, error) {
	mimirKey := c.extractMimirKeyFromPath(key)
	if mimirKey == "" {
		return nil, nil
	}
	
	req := &types.QueryMimirWithKeyRequest{
		Key:    mimirKey,
		Height: fmt.Sprintf("%d", height),
	}
	
	resp, err := c.queryClient.MimirWithKey(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC mimir query failed: %w", err)
	}
	
	return c.codec.Marshal(resp)
}

func (c *remoteClient) fetchRagnarokData(ctx context.Context, height int64) ([]byte, error) {
	req := &types.QueryRagnarokRequest{
		Height: fmt.Sprintf("%d", height),
	}
	
	resp, err := c.queryClient.Ragnarok(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC ragnarok query failed: %w", err)
	}
	
	return c.codec.Marshal(resp)
}

func (c *remoteClient) extractMimirKeyFromPath(key string) string {
	if strings.HasPrefix(key, "mimir//") {
		return strings.TrimPrefix(key, "mimir//")
	}
	return ""
}

func (c *remoteClient) extractAssetFromPoolKey(key string) string {
	lower := strings.ToLower(key)
	idx := strings.Index(lower, "pool/")
	if idx == -1 {
		return ""
	}
	raw := key[idx+len("pool/"):]
	raw = strings.TrimLeft(raw, "/")
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "/") {
		parts := strings.SplitN(raw, "/", 2)
		if parts[0] == "" || parts[1] == "" {
			return ""
		}
		right := parts[1]
		if strings.HasPrefix(right, "0x") || strings.HasPrefix(right, "0X") {
			right = "0X" + strings.ToUpper(right[2:])
		}
		return parts[0] + "." + right
	}
	return raw
}

func (c *remoteClient) extractAddressFromKey(key string) string {
	parts := strings.Split(key, "/")
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func (c *remoteClient) GetLatestHeight(ctx context.Context) (int64, error) {
	req := &types.QueryLastBlocksRequest{}
	resp, err := c.queryClient.LastBlocks(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("failed to get latest height via gRPC: %w", err)
	}
	if len(resp.LastBlocks) > 0 {
		return resp.LastBlocks[0].Thorchain, nil
	}
	return 0, fmt.Errorf("no block data available")
}


func (c *remoteClient) GetRange(ctx context.Context, storeKey string, start, end []byte, height int64) ([]KeyValue, error) {
	if storeKey == "thorchain" {
		if len(start) > 0 {
			s := string(start)
			if strings.HasPrefix(s, "pool/") {
				fmt.Printf("[forking][RANGE][thorchain] pools via gRPC height=%d\n", height)
				return c.getRangeViaPoolsGRPC(ctx, height)
			}
			if strings.HasPrefix(s, "node_account/") {
				fmt.Printf("[forking][RANGE][thorchain] nodes via gRPC height=%d\n", height)
				return c.getRangeViaNodesGRPC(ctx, height)
			}
			if strings.HasPrefix(s, "lp/") {
				fmt.Printf("[forking][RANGE][thorchain] LPs via gRPC height=%d\n", height)
				return c.getRangeViaLPsGRPC(ctx, height)
			}
			if strings.HasPrefix(s, "mimir/") {
				fmt.Printf("[forking][RANGE][thorchain] mimir via gRPC height=%d\n", height)
				return c.getRangeViaMimirGRPC(ctx, height)
			}
		}
		if len(end) > 0 {
			e := string(end)
			if strings.HasPrefix(e, "pool/") {
				fmt.Printf("[forking][RANGE][thorchain] pools via gRPC (end) height=%d\n", height)
				return c.getRangeViaPoolsGRPC(ctx, height)
			}
			if strings.HasPrefix(e, "node_account/") {
				fmt.Printf("[forking][RANGE][thorchain] nodes via gRPC (end) height=%d\n", height)
				return c.getRangeViaNodesGRPC(ctx, height)
			}
			if strings.HasPrefix(e, "lp/") {
				fmt.Printf("[forking][RANGE][thorchain] LPs via gRPC (end) height=%d\n", height)
				return c.getRangeViaLPsGRPC(ctx, height)
			}
			if strings.HasPrefix(e, "mimir/") {
				fmt.Printf("[forking][RANGE][thorchain] mimir via gRPC (end) height=%d\n", height)
				return c.getRangeViaMimirGRPC(ctx, height)
			}
		}
	}
	if strings.EqualFold(storeKey, wasmtypes.StoreKey) {
		if len(start) >= 1 && start[0] == 0x01 && len(end) >= 1 && end[0] == 0x02 {
			var out []KeyValue
			var pageKey []byte
			for {
				resp, err := c.wasmClient.Codes(c.ctxWithHeight(ctx, height), &wasmtypes.QueryCodesRequest{
					Pagination: &query.PageRequest{
						Key:   pageKey,
						Limit: 1000,
					},
				})
				if err != nil {
					if shouldRetryWithoutHeight(err) {
						resp, err = c.wasmClient.Codes(ctx, &wasmtypes.QueryCodesRequest{
							Pagination: &query.PageRequest{
								Key:   pageKey,
								Limit: 1000,
							},
						})
					}
				}
				if err != nil {
					return nil, fmt.Errorf("wasm Codes: %w", err)
				}
				if resp == nil {
					break
				}
				for _, ci := range resp.CodeInfos {
					key := make([]byte, 1+8)
					key[0] = 0x01
					binary.BigEndian.PutUint64(key[1:], ci.CodeID)
					val, merr := c.codec.Marshal(&wasmtypes.CodeInfo{
						CodeHash:          ci.DataHash,
						Creator:           ci.Creator,
						InstantiateConfig: ci.InstantiatePermission,
					})
					if merr == nil {
						out = append(out, KeyValue{Key: key, Value: val})
					}
				}
				if resp.Pagination == nil || len(resp.Pagination.NextKey) == 0 {
					break
				}
				pageKey = resp.Pagination.NextKey
			}
			return out, nil
		}

		// Contract store prefix 0x03/0x05 | addr | key...
		if len(start) >= 2 && (start[0] == 0x05 || start[0] == 0x03) {
			if addr, _, ok := c.parseWasmContractStoreKeyNoLen(start[1:]); ok {
				items, err := c.fetchAllContractState(ctx, addr, height, start[0])
				if err != nil || len(items) == 0 {
					return []KeyValue{}, err
				}
				if len(start) > 0 || len(end) > 0 {
					var filtered []KeyValue
					for _, kv := range items {
						if (len(start) == 0 || bytes.Compare(kv.Key, start) >= 0) && (len(end) == 0 || bytes.Compare(kv.Key, end) < 0) {
							filtered = append(filtered, kv)
						}
					}
					return filtered, nil
				}
				return items, nil
			}
		}

		if len(start) >= 1 && start[0] == 0x05 && len(end) >= 1 && end[0] == 0x06 {
			var out []KeyValue
			resp, err := c.wasmClient.PinnedCodes(c.ctxWithHeight(ctx, height), &wasmtypes.QueryPinnedCodesRequest{})
			if err != nil {
				if shouldRetryWithoutHeight(err) {
					resp, err = c.wasmClient.PinnedCodes(ctx, &wasmtypes.QueryPinnedCodesRequest{})
				}
			}
			if err != nil {
				return nil, fmt.Errorf("wasm PinnedCodes: %w", err)
			}
			if resp != nil {
				for _, id := range resp.CodeIDs {
					key := make([]byte, 1+8)
					key[0] = 0x05
					binary.BigEndian.PutUint64(key[1:], id)
					out = append(out, KeyValue{Key: key, Value: []byte{1}})
				}
			}
			return out, nil
		}
		return []KeyValue{}, nil
	}


	switch storeKey {
	case "pools":
		return c.getRangeViaPoolsGRPC(ctx, height)
	case "nodes":
		return c.getRangeViaNodesGRPC(ctx, height)
	default:
		return []KeyValue{}, nil
	}
}
func (c *remoteClient) fetchAllContractState(ctx context.Context, addr string, height int64, prefixByte byte) ([]KeyValue, error) {
	pg := &query.PageRequest{Limit: 200}
	var out []KeyValue
	for {
		req := &wasmtypes.QueryAllContractStateRequest{
			Address:    addr,
			Pagination: pg,
		}
		resp, err := c.wasmClient.AllContractState(c.ctxWithHeight(ctx, height), req)
		if err != nil && shouldRetryWithoutHeight(err) {
			resp, err = c.wasmClient.AllContractState(ctx, req)
		}
		if err != nil {
			return nil, err
		}
		if resp == nil || len(resp.Models) == 0 {
			break
		}
		prefix := c.makeWasmContractStorePrefix(addr)
		for _, m := range resp.Models {
			k := make([]byte, 0, 1+len(prefix)+len(m.Key))
			k = append(k, prefixByte)
			k = append(k, prefix...)
			k = append(k, m.Key...)
			out = append(out, KeyValue{Key: k, Value: m.Value})
		}
		if resp.Pagination == nil || resp.Pagination.NextKey == nil || len(resp.Pagination.NextKey) == 0 {
			break
		}
		pg.Key = resp.Pagination.NextKey
	}
	return out, nil
}


func (c *remoteClient) getRangeViaPoolsGRPC(ctx context.Context, height int64) ([]KeyValue, error) {
	req := &types.QueryPoolsRequest{
		Height: fmt.Sprintf("%d", height),
	}
	resp, err := c.queryClient.Pools(ctx, req)
	if err != nil {
		fmt.Printf("[forking][RANGE][thorchain] gRPC pools error height=%d err=%v\n", height, err)
		return nil, fmt.Errorf("gRPC pools range query failed: %w", err)
	}

	var kvPairs []KeyValue
	for _, p := range resp.Pools {
		fmt.Printf("[forking][RANGE][thorchain] pool %s status=%s\n", p.Asset, p.Status)
		asset, err := common.NewAsset(p.Asset)
		if err != nil {
			continue
		}

		br := sdkmath.NewUintFromString(p.BalanceRune)
		ba := sdkmath.NewUintFromString(p.BalanceAsset)
		lpu := sdkmath.NewUintFromString(p.LPUnits)
		su := sdkmath.NewUintFromString(p.SynthUnits)
		pir := sdkmath.NewUintFromString(p.PendingInboundRune)
		pia := sdkmath.NewUintFromString(p.PendingInboundAsset)

		var status types.PoolStatus
		switch strings.ToLower(p.Status) {
		case "available":
			status = types.PoolStatus_Available
		case "staged":
			status = types.PoolStatus_Staged
		case "suspended":
			status = types.PoolStatus_Suspended
		default:
			status = types.PoolStatus_UnknownPoolStatus
		}

		record := types.Pool{
			BalanceRune:         br,
			BalanceAsset:        ba,
			Asset:               asset,
			LPUnits:             lpu,
			Status:              status,
			StatusSince:         0,
			Decimals:            p.Decimals,
			SynthUnits:          su,
			PendingInboundRune:  pir,
			PendingInboundAsset: pia,
		}

		key := fmt.Sprintf("pool//%s", strings.ToUpper(asset.String()))
		value, _ := c.codec.Marshal(&record)
		kvPairs = append(kvPairs, KeyValue{Key: []byte(key), Value: value})
	}

	return kvPairs, nil
}

func (c *remoteClient) getRangeViaNodesGRPC(ctx context.Context, height int64) ([]KeyValue, error) {
	req := &types.QueryNodesRequest{
		Height: fmt.Sprintf("%d", height),
	}
	resp, err := c.queryClient.Nodes(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC nodes range query failed: %w", err)
	}

	var kvPairs []KeyValue
	for _, n := range resp.Nodes {
		naAddr, err := common.NewAddress(n.NodeAddress)
		if err != nil {
			continue
		}
		accAddr, err := cosmos.AccAddressFromBech32(n.NodeAddress)
		if err != nil {
			continue
		}
		var status types.NodeStatus
		switch strings.ToLower(n.Status) {
		case "whitelisted":
			status = types.NodeStatus_Whitelisted
		case "standby":
			status = types.NodeStatus_Standby
		case "ready":
			status = types.NodeStatus_Ready
		case "active":
			status = types.NodeStatus_Active
		case "disabled":
			status = types.NodeStatus_Disabled
		default:
			status = types.NodeStatus_Unknown
		}

		bond := sdkmath.NewUintFromString(n.TotalBond)
		bondAddr, err := common.NewAddress(n.NodeOperatorAddress)
		if err != nil {
			bondAddr = common.NoAddress
		}

		record := types.NodeAccount{
			NodeAddress:         accAddr,
			Status:              status,
			PubKeySet:           n.PubKeySet,
			ValidatorConsPubKey: n.ValidatorConsPubKey,
			Bond:                bond,
			ActiveBlockHeight:   n.ActiveBlockHeight,
			BondAddress:         bondAddr,
			StatusSince:         n.StatusSince,
			SignerMembership:    n.SignerMembership,
			RequestedToLeave:    n.RequestedToLeave,
			ForcedToLeave:       n.ForcedToLeave,
			IPAddress:           n.IpAddress,
			Version:             n.Version,
			MissingBlocks:       uint64(n.MissingBlocks),
			Maintenance:         n.Maintenance,
		}

		key := fmt.Sprintf("node_account/%s", naAddr.String())
		value, _ := c.codec.Marshal(&record)
		kvPairs = append(kvPairs, KeyValue{Key: []byte(key), Value: value})
	}

	return kvPairs, nil
}
func (c *remoteClient) getRangeViaLPsGRPC(ctx context.Context, height int64) ([]KeyValue, error) {
	reqPools := &types.QueryPoolsRequest{
		Height: fmt.Sprintf("%d", height),
	}
	poolsResp, err := c.queryClient.Pools(ctx, reqPools)
	if err != nil {
		return nil, fmt.Errorf("gRPC LPs: pools list failed: %w", err)
	}
	var out []KeyValue
	for _, p := range poolsResp.Pools {
		if p.Asset == "" {
			continue
		}
		lpsReq := &types.QueryLiquidityProvidersRequest{
			Asset:  p.Asset,
			Height: fmt.Sprintf("%d", height),
		}
		lpsResp, err := c.queryClient.LiquidityProviders(ctx, lpsReq)
		if err != nil {
			continue
		}
		asset, aerr := common.NewAsset(p.Asset)
		if aerr != nil {
			continue
		}
		for _, it := range lpsResp.LiquidityProviders {
			var runeAddr, assetAddr common.Address
			if it.RuneAddress != "" {
				runeAddr, _ = common.NewAddress(it.RuneAddress)
			}
			if it.AssetAddress != "" {
				assetAddr, _ = common.NewAddress(it.AssetAddress)
			}
			rec := types.LiquidityProvider{
				Asset:              asset,
				RuneAddress:        runeAddr,
				AssetAddress:       assetAddr,
				LastAddHeight:      it.LastAddHeight,
				LastWithdrawHeight: it.LastWithdrawHeight,
				Units:              sdkmath.NewUintFromString(it.Units),
				PendingRune:        sdkmath.NewUintFromString(it.PendingRune),
				PendingAsset:       sdkmath.NewUintFromString(it.PendingAsset),
				RuneDepositValue:   sdkmath.NewUintFromString(it.RuneDepositValue),
				AssetDepositValue:  sdkmath.NewUintFromString(it.AssetDepositValue),
			}
			key := fmt.Sprintf("lp//%s/%s", strings.ToUpper(asset.String()), strings.ToUpper(rec.GetAddress().String()))
			val, _ := c.codec.Marshal(&rec)
			out = append(out, KeyValue{Key: []byte(key), Value: val})
		}
	}
	return out, nil
}

func (c *remoteClient) getRangeViaMimirGRPC(ctx context.Context, height int64) ([]KeyValue, error) {
	req := &types.QueryMimirValuesRequest{
		Height: fmt.Sprintf("%d", height),
	}
	resp, err := c.queryClient.MimirValues(ctx, req)
	if err != nil || resp == nil {
		return []KeyValue{}, err
	}
	var out []KeyValue
	for k, v := range resp.Values {
		key := fmt.Sprintf("mimir//%s", strings.ToUpper(k))
		val, _ := c.codec.Marshal(&keeperv1.ProtoInt64{Value: v})
		out = append(out, KeyValue{Key: []byte(key), Value: val})
	}
	return out, nil
}


func decodeStoreKVPairs(b []byte) ([]*storepb.StoreKVPair, error) {
	pairs := make([]*storepb.StoreKVPair, 0, 64)

	for len(b) > 0 {
		// Outer: tag=1, wire=bytes (length-delimited message)
		fieldNum, wireType, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, fmt.Errorf("consume outer tag failed: %d", n)
		}
		if fieldNum != 1 || wireType != protowire.BytesType {
			return nil, fmt.Errorf("unexpected outer field: num=%d wt=%d", fieldNum, wireType)
		}

		msgBytes, m := protowire.ConsumeBytes(b[n:])
		if m < 0 {
			return nil, fmt.Errorf("consume outer bytes failed")
		}

		kv := &storepb.StoreKVPair{}
		// Parse inner message manually to avoid proto version mismatches (wireType errors)
		for len(msgBytes) > 0 {
			inNum, _, inN := protowire.ConsumeTag(msgBytes)
			if inN < 0 {
				return nil, fmt.Errorf("consume inner tag failed: %d", inN)
			}
			switch inNum {
			case 1: // key (bytes)
				bb, l := protowire.ConsumeBytes(msgBytes[inN:])
				if l < 0 {
					return nil, fmt.Errorf("consume key failed")
				}
				kv.Key = append([]byte(nil), bb...)
				msgBytes = msgBytes[inN+l:]
			case 2: // value (bytes)
				vb, l := protowire.ConsumeBytes(msgBytes[inN:])
				if l < 0 {
					return nil, fmt.Errorf("consume value failed")
				}
				kv.Value = append([]byte(nil), vb...)
				msgBytes = msgBytes[inN+l:]
			case 3: // store_key (string)
				s, l := protowire.ConsumeString(msgBytes[inN:])
				if l < 0 {
					return nil, fmt.Errorf("consume store_key failed")
				}
				kv.StoreKey = s
				msgBytes = msgBytes[inN+l:]
			case 4: // delete (varint -> bool)
				v, l := protowire.ConsumeVarint(msgBytes[inN:])
				if l < 0 {
					return nil, fmt.Errorf("consume delete failed")
				}
				kv.Delete = v != 0
				msgBytes = msgBytes[inN+l:]
			default:
				_, _, l := protowire.ConsumeField(msgBytes[inN:])
				if l < 0 {
					return nil, fmt.Errorf("skip unknown field=%d failed", inNum)
				}
				msgBytes = msgBytes[inN+l:]
			}
		}

		pairs = append(pairs, kv)
		b = b[n+m:]
	}

	return pairs, nil
}
func (c *remoteClient) parseWasmContractAddrCandidates(b []byte) []string {
	var out []string
	if len(b) == 0 {
		return out
	}
	if ln, n := protowire.ConsumeVarint(b); n > 0 && int(ln) == 20 && len(b) >= n+int(ln) {
		addrBz := b[n : n+int(ln)]
		out = append(out, cosmos.AccAddress(addrBz).String())
	}
	for i := 0; i+1+20 <= len(b); i++ {
		if b[i] == 0x14 {
			addrBz := b[i+1 : i+1+20]
			out = append(out, cosmos.AccAddress(addrBz).String())
		}
	}
	for i := 0; i+20 <= len(b); i++ {
		addrBz := b[i : i+20]
		out = append(out, cosmos.AccAddress(addrBz).String())
	}
	if len(b) == 20 {
		out = append(out, cosmos.AccAddress(b).String())
	}
	if len(b) > 20 {
		h := b[:20]
		out = append(out, cosmos.AccAddress(h).String())
		t := b[len(b)-20:]
		out = append(out, cosmos.AccAddress(t).String())
		if len(b) >= 21 {
			midStart := (len(b) - 20) / 2
			out = append(out, cosmos.AccAddress(b[midStart:midStart+20]).String())
		}
	}
	seen := make(map[string]struct{}, len(out))
	dedup := make([]string, 0, len(out))
	for _, a := range out {
		if a == "" {
			continue
		}
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		dedup = append(dedup, a)
	}
	return dedup
}
func (c *remoteClient) parseWasmContractAddrStrict(b []byte) (string, bool) {
	if len(b) == 0 {
		return "", false
	}
	if ln, n := protowire.ConsumeVarint(b); n > 0 && int(ln) == 20 && len(b) >= n+int(ln) {
		return cosmos.AccAddress(b[n : n+int(ln)]).String(), true
	}
	if len(b) == 20 {
		return cosmos.AccAddress(b).String(), true
	}
	if len(b) == 21 && b[0] == 0x14 {
		return cosmos.AccAddress(b[1:21]).String(), true
	}
	if len(b) == 32 {
		return cosmos.AccAddress(b).String(), true
	}
	return "", false
}

func (c *remoteClient) parseWasmContractAddr(b []byte) (string, bool) {
	if len(b) >= 1 {
		ln, n := protowire.ConsumeVarint(b)
		if n > 0 && int(ln) == 20 && len(b) >= n+int(ln) {
			addrBz := b[n : n+int(ln)]
			return cosmos.AccAddress(addrBz).String(), true
		}
	}
	if len(b) == 20 {
		return cosmos.AccAddress(b).String(), true
	}
	if len(b) > 20 {
		return cosmos.AccAddress(b[len(b)-20:]).String(), true
	}
	return "", false
}

func (c *remoteClient) parseWasmCodeID(b []byte) (uint64, bool) {
	if len(b) < 8 {
		return 0, false
	}
	return binary.BigEndian.Uint64(b[:8]), true
}

func (c *remoteClient) parseWasmContractStoreKey(b []byte) (string, []byte, bool) {
	if len(b) >= 1 {
		if ln, n := protowire.ConsumeVarint(b); n > 0 && int(ln) == 20 && len(b) >= n+int(ln) {
			addrBz := b[n : n+int(ln)]
			suffix := b[n+int(ln):]
			return cosmos.AccAddress(addrBz).String(), suffix, true
		}
	}
	for i := 0; i+1+20 <= len(b); i++ {
		if b[i] == 0x14 {
			addrBz := b[i+1 : i+1+20]
			suffix := b[i+1+20:]
			return cosmos.AccAddress(addrBz).String(), suffix, true
		}
	}
	if len(b) >= 20 {
		addrBz := b[:20]
		suffix := b[20:]
		return cosmos.AccAddress(addrBz).String(), suffix, true
	}
	if len(b) > 20 {
		addrBz := b[len(b)-20:]
		suffix := b[:len(b)-20]
		return cosmos.AccAddress(addrBz).String(), suffix, true
	}
	return "", nil, false
}

func (c *remoteClient) makeWasmContractStorePrefix(addr string) []byte {
	acc, err := cosmos.AccAddressFromBech32(addr)
	if err != nil {
		return nil
	}
	return []byte(acc)
}
func (c *remoteClient) parseWasmContractStoreKeyNoLen(b []byte) (string, []byte, bool) {
	if ln, n := protowire.ConsumeVarint(b); n > 0 && int(ln) == 20 && len(b) >= n+int(ln) {
		addrBz := b[n : n+int(ln)]
		return cosmos.AccAddress(addrBz).String(), b[n+int(ln):], true
	}
	if len(b) >= 21 && b[0] == 0x14 {
		addrBz := b[1:21]
		return cosmos.AccAddress(addrBz).String(), b[21:], true
	}
	if len(b) > 32 {
		addrBz := b[:32]
		if addr, ok := c.parseWasmContractAddrStrict(addrBz); ok {
			return addr, b[32:], true
		}
	}
	if len(b) > 20 {
		addrBz := b[:20]
		if addr, ok := c.parseWasmContractAddrStrict(addrBz); ok {
			return addr, b[20:], true
		}
	}
	return "", nil, false
}



func (c *remoteClient) Close() error {
	if c.grpcConn != nil {
		return c.grpcConn.Close()
	}
	return nil
}
