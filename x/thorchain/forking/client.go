package forking

import (
	"bytes"
	"context"
	"fmt"
	"google.golang.org/protobuf/encoding/protowire"

	storepb "cosmossdk.io/api/cosmos/store/v1beta1"
	tmclient "github.com/cometbft/cometbft/rpc/client"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/codec"
)

type remoteClient struct {
	rpcClient tmclient.Client
	config    RemoteConfig
	codec     codec.Codec
}

func NewRemoteClient(config RemoteConfig, cdc codec.Codec) (RemoteClient, error) {
	rpcClient, err := rpchttp.New(config.RPC, "/websocket")
	if err != nil {
		return nil, fmt.Errorf("failed to create RPC client: %w", err)
	}
	cli := &remoteClient{
		rpcClient: rpcClient,
		config:    config,
		codec:     cdc,
	}
	return cli, nil
}

func (c *remoteClient) GetWithProof(ctx context.Context, storeKey string, key []byte, height int64) ([]byte, error) {
	path := fmt.Sprintf("store/%s/key", storeKey)
	res, err := c.rpcClient.ABCIQueryWithOptions(ctx, path, key, tmclient.ABCIQueryOptions{Height: height, Prove: false})
	if err != nil {
		return nil, fmt.Errorf("ABCI query failed: %w", err)
	}
	if res.Response.Code != 0 {
		return nil, fmt.Errorf("ABCI query returned error code %d: %s", res.Response.Code, res.Response.Log)
	}
	if len(res.Response.Value) == 0 {
		return nil, nil
	}
	return res.Response.Value, nil
}

func (c *remoteClient) GetLatestHeight(ctx context.Context) (int64, error) {
	st, err := c.rpcClient.Status(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get status: %w", err)
	}
	return st.SyncInfo.LatestBlockHeight, nil
}

func (c *remoteClient) GetRange(ctx context.Context, storeKey string, start, end []byte, height int64) ([]KeyValue, error) {
	path := fmt.Sprintf("store/%s/subspace", storeKey)
	res, err := c.rpcClient.ABCIQueryWithOptions(ctx, path, start, tmclient.ABCIQueryOptions{Height: height, Prove: false})
	if err != nil {
		return nil, fmt.Errorf("subspace query failed: %w", err)
	}
	if res.Response.Code != 0 || len(res.Response.Value) == 0 {
		return nil, nil
	}

	pairs, err := decodeStoreKVPairs(res.Response.Value)
	if err != nil {
		return nil, fmt.Errorf("decode StoreKVPairs: %w", err)
	}

	out := make([]KeyValue, 0, len(pairs))
	for _, p := range pairs {
		if (len(start) == 0 || bytes.Compare(p.Key, start) >= 0) &&
			(len(end) == 0 || bytes.Compare(p.Key, end) < 0) {
			// copy to detach from proto buffers
			k := append([]byte(nil), p.Key...)
			v := append([]byte(nil), p.Value...)
			out = append(out, KeyValue{Key: k, Value: v})
		}
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

func (c *remoteClient) Close() error {
	if c.rpcClient != nil {
		return c.rpcClient.Stop()
	}
	return nil
}
