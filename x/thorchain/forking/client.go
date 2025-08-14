package forking

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"

	storepb "cosmossdk.io/api/cosmos/store/v1beta1"
	sdkmath "cosmossdk.io/math"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protowire"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/codes"
)

type remoteClient struct {
	grpcConn    *grpc.ClientConn
	queryClient types.QueryClient
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

	cli := &remoteClient{
		grpcConn:    conn,
		queryClient: client,
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

func (c *remoteClient) GetWithProof(ctx context.Context, storeKey string, key []byte, height int64) ([]byte, error) {
	return c.fetchViaGRPC(ctx, storeKey, key, height)
}

func (c *remoteClient) fetchViaGRPC(ctx context.Context, storeKey string, key []byte, height int64) ([]byte, error) {
	keyStr := string(key)
	lkey := strings.ToLower(keyStr)
	lstore := strings.ToLower(storeKey)

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
	if err != nil {
		if isNotFoundErr(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("gRPC liquidity provider query failed: %w", err)
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
	if err != nil {
		if isNotFoundErr(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("gRPC saver query failed: %w", err)
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
	if err != nil {
		if isNotFoundErr(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("gRPC borrower query failed: %w", err)
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
			if strings.HasPrefix(string(start), "pool/") {
				return c.getRangeViaPoolsGRPC(ctx, height)
			}
			if strings.HasPrefix(string(start), "node_account/") {
				return c.getRangeViaNodesGRPC(ctx, height)
			}
		}
		if len(end) > 0 {
			if strings.HasPrefix(string(end), "pool/") {
				return c.getRangeViaPoolsGRPC(ctx, height)
			}
			if strings.HasPrefix(string(end), "node_account/") {
				return c.getRangeViaNodesGRPC(ctx, height)
			}
		}
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

func (c *remoteClient) getRangeViaPoolsGRPC(ctx context.Context, height int64) ([]KeyValue, error) {
	req := &types.QueryPoolsRequest{
		Height: fmt.Sprintf("%d", height),
	}
	resp, err := c.queryClient.Pools(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC pools range query failed: %w", err)
	}

	var kvPairs []KeyValue
	for _, p := range resp.Pools {
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

func (c *remoteClient) Close() error {
	if c.grpcConn != nil {
		return c.grpcConn.Close()
	}
	return nil
}
