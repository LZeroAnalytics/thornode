package forking

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"

	storepb "cosmossdk.io/api/cosmos/store/v1beta1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protowire"

	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
	"github.com/cosmos/cosmos-sdk/codec"
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

func (c *remoteClient) GetWithProof(ctx context.Context, storeKey string, key []byte, height int64) ([]byte, error) {
	return c.fetchViaGRPC(ctx, storeKey, key, height)
}

func (c *remoteClient) fetchViaGRPC(ctx context.Context, storeKey string, key []byte, height int64) ([]byte, error) {
	keyStr := string(key)
	
	switch {
	case strings.Contains(keyStr, "mimir//"):
		return c.fetchMimirData(ctx, keyStr, height)
	case strings.Contains(keyStr, "ragnarok"):
		return c.fetchRagnarokData(ctx, height)
	case strings.Contains(keyStr, "pool") || strings.Contains(storeKey, "pool"):
		return c.fetchPoolData(ctx, keyStr, height)
	case strings.Contains(keyStr, "account") || strings.Contains(storeKey, "account"):
		return c.fetchAccountData(ctx, keyStr, height)
	case strings.Contains(keyStr, "balance") || strings.Contains(storeKey, "bank"):
		return c.fetchBalanceData(ctx, keyStr, height)
	case strings.Contains(keyStr, "node") || strings.Contains(storeKey, "node"):
		return c.fetchNodeData(ctx, keyStr, height)
	default:
		return nil, nil
	}
}

func (c *remoteClient) fetchPoolData(ctx context.Context, key string, height int64) ([]byte, error) {
	req := &types.QueryPoolsRequest{}
	
	resp, err := c.queryClient.Pools(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC pools query failed: %w", err)
	}
	
	return c.codec.Marshal(resp)
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
	req := &types.QueryNodesRequest{}
	
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
	req := &types.QueryPoolsRequest{}
	resp, err := c.queryClient.Pools(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC pools range query failed: %w", err)
	}
	
	var kvPairs []KeyValue
	for _, pool := range resp.Pools {
		key := fmt.Sprintf("pool/%s", pool.Asset)
		value, _ := c.codec.Marshal(pool)
		kvPairs = append(kvPairs, KeyValue{Key: []byte(key), Value: value})
	}
	
	return kvPairs, nil
}

func (c *remoteClient) getRangeViaNodesGRPC(ctx context.Context, height int64) ([]KeyValue, error) {
	req := &types.QueryNodesRequest{}
	resp, err := c.queryClient.Nodes(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC nodes range query failed: %w", err)
	}
	
	var kvPairs []KeyValue
	for _, node := range resp.Nodes {
		key := fmt.Sprintf("node/%s", node.NodeAddress)
		value, _ := c.codec.Marshal(node)
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
