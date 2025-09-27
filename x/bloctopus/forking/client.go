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
		if isNotFoundErr(err) {
			return nil, nil
		}
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
			jailed := false

			record := types.NodeAccount{
				NodeAddress:         naAddr,
				Status:              status,
				PubKeySet:           types.PubKeySet{},
				ValidatorConsPubKey: accAddr.String(),
				ActiveBlockHeight:   0,
				Bond:                bond,
				Rewards:             sdkmath.ZeroUint(),
				SlashPoints:         0,
				SignerMembership:    nil,
				RequestedToLeave:    false,
				ForcedToLeave:       false,
				LeaveHeight:         0,
				IPAddress:           "",
				Version:             single.Version,
				Resolver:            "",
				Jailed:              jailed,
				ObserveChains:       nil,
				PreflightStatus:     "",
				StatusSince:         0,
				BondProviders:       types.BondProviders{},
				CurrentAward:        sdkmath.ZeroUint(),
				SlashAmount:         sdkmath.ZeroUint(),
				Signer:              false,
				RequestedWidthdraw:  sdkmath.ZeroUint(),
			}
			return c.codec.Marshal(&record)
		}
	}
	return nil, nil
}

func (c *remoteClient) fetchMimirData(ctx context.Context, key string, height int64) ([]byte, error) {
	return nil, nil
}

func (c *remoteClient) fetchRagnarokData(ctx context.Context, height int64) ([]byte, error) {
	resp := &storepb.ProofOp{
		Type: "ragnarok",
		Key:  []byte("ragnarok"),
	}
	return c.codec.Marshal(resp)
}

func (c *remoteClient) extractMimirKeyFromPath(path string) string {
	return path
}

func (c *remoteClient) extractAssetFromPoolKey(key string) string {
	lower := strings.ToLower(key)
	idx := strings.Index(lower, "pool/")
	if idx == -1 {
		return ""
	}
	raw := strings.TrimLeft(key[idx+len("pool/"):], "/")
	if raw == "" {
		return ""
	}
	return c.normalizeAssetFromKeyAsset(raw)
}

func (c *remoteClient) extractAddressFromKey(key string) string {
	lower := strings.ToLower(key)
	prefixes := []string{
		"balance/",
		"account/",
	}
	for _, p := range prefixes {
		if idx := strings.Index(lower, p); idx != -1 {
			raw := strings.TrimLeft(key[idx+len(p):], "/")
			return raw
		}
	}
	return ""
}

func (c *remoteClient) GetLatestHeight(ctx context.Context) (int64, error) {
	return 0, nil
}

func (c *remoteClient) GetRange(ctx context.Context, storeKey string, start, end []byte, height int64) ([]KeyValue, error) {
	switch strings.ToLower(storeKey) {
	case "pool", "node", "bank", "account":
	default:
		return nil, nil
	}
	var kvs []KeyValue
	return kvs, nil
}

func (c *remoteClient) getRangeViaPoolsGRPC(ctx context.Context, start, end []byte, height int64) ([]KeyValue, error) {
	return nil, nil
}

func (c *remoteClient) getRangeViaNodesGRPC(ctx context.Context, start, end []byte, height int64) ([]KeyValue, error) {
	return nil, nil
}

func decodeStoreKVPairs(bz []byte) ([]KeyValue, error) {
	var res []KeyValue
	for len(bz) > 0 {
		fieldNum, typ, n := protowire.ConsumeTag(bz)
		if n <= 0 {
			return nil, fmt.Errorf("invalid protobuf data")
		}
		bz = bz[n:]
		if typ != protowire.BytesType {
			return nil, fmt.Errorf("unexpected type: %v", typ)
		}
		v, m := protowire.ConsumeBytes(bz)
		if m <= 0 {
			return nil, fmt.Errorf("invalid bytes field")
		}
		res = append(res, KeyValue{
			Key:   []byte(fmt.Sprintf("%d", fieldNum)),
			Value: v,
		})
		bz = bz[m:]
	}
	return res, nil
}

func (c *remoteClient) Close() error {
	if c.grpcConn != nil {
		return c.grpcConn.Close()
	}
	return nil
}
