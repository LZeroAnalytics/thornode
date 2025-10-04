package aggregator

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"

	thorchaintypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func aggregateThorchain(ws []KVWrite) (map[string]any, bool) {
	var pools []map[string]any
	var mimirs []map[string]any
	var nodes []map[string]any
	var vaults []map[string]any
	var lps []map[string]any
	var outboundFees []map[string]any
	changed := false

	for _, w := range ws {
		if w.Store != thorchaintypes.StoreKey {
			continue
		}
		kb, err := hex.DecodeString(w.Key)
		if err != nil {
			continue
		}
		key := string(kb)

		switch {
		case strings.HasPrefix(key, "pool/"):
			if w.Op == "delete" || w.Value == "" {
				changed = true
				continue
			}
			bz, err := base64.StdEncoding.DecodeString(w.Value)
			if err != nil {
				continue
			}
			var pool thorchaintypes.Pool
			if err := appCodec.Unmarshal(bz, &pool); err != nil {
				continue
			}
			b, err := appCodec.MarshalJSON(&pool)
			if err != nil {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				continue
			}
			if m != nil {
				pools = append(pools, m)
				changed = true
			}

		case strings.HasPrefix(key, "mimir/"):
			if w.Op == "delete" || w.Value == "" {
				changed = true
				continue
			}
			rawKey := strings.TrimPrefix(key, "mimir/")
			if rawKey == "" {
				continue
			}
			bz, err := base64.StdEncoding.DecodeString(w.Value)
			if err != nil {
				continue
			}
			var v thorchaintypes.ProtoInt64
			if err := appCodec.Unmarshal(bz, &v); err != nil {
				continue
			}
			mimirs = append(mimirs, map[string]any{
				"key":   rawKey,
				"value": v.GetValue(),
			})
			changed = true

		case strings.HasPrefix(key, "node_account/"):
			if w.Op == "delete" || w.Value == "" {
				changed = true
				continue
			}
			bz, err := base64.StdEncoding.DecodeString(w.Value)
			if err != nil {
				continue
			}
			var na thorchaintypes.NodeAccount
			if err := appCodec.Unmarshal(bz, &na); err != nil {
				continue
			}
			b, err := appCodec.MarshalJSON(&na)
			if err != nil {
				continue
			}
			var m map[string]any
			if json.Unmarshal(b, &m) == nil {
				nodes = append(nodes, m)
				changed = true
			}

		case strings.HasPrefix(key, "vault/"):
			if w.Op == "delete" || w.Value == "" {
				changed = true
				continue
			}
			bz, err := base64.StdEncoding.DecodeString(w.Value)
			if err != nil {
				continue
			}
			var v thorchaintypes.Vault
			if err := appCodec.Unmarshal(bz, &v); err != nil {
				continue
			}
			b, err := appCodec.MarshalJSON(&v)
			if err != nil {
				continue
			}
			var m map[string]any
			if json.Unmarshal(b, &m) == nil {
				vaults = append(vaults, m)
				changed = true
			}

		case strings.HasPrefix(key, "liquidity_provider/"):
			if w.Op == "delete" || w.Value == "" {
				changed = true
				continue
			}
			bz, err := base64.StdEncoding.DecodeString(w.Value)
			if err != nil {
				continue
			}
			var lp thorchaintypes.LiquidityProvider
			if err := appCodec.Unmarshal(bz, &lp); err != nil {
				continue
			}
			b, err := appCodec.MarshalJSON(&lp)
			if err != nil {
				continue
			}
			var m map[string]any
			if json.Unmarshal(b, &m) == nil {
				lps = append(lps, m)
				changed = true
			}


		case strings.HasPrefix(key, "outbound_fee/"):
			if w.Op == "delete" || w.Value == "" {
				changed = true
				continue
			}
			denom := strings.TrimPrefix(key, "outbound_fee/")
			if denom == "" {
				continue
			}
			bz, err := base64.StdEncoding.DecodeString(w.Value)
			if err != nil {
				continue
			}
			var v thorchaintypes.ProtoInt64
			if err := appCodec.Unmarshal(bz, &v); err != nil {
				continue
			}
			outboundFees = append(outboundFees, map[string]any{
				"denom": denom,
				"value": v.GetValue(),
			})
			changed = true

		default:
			continue
		}
	}

	if !changed {
		return nil, false
	}
	out := make(map[string]any)
	if len(pools) > 0 {
		out["pools"] = pools
	}
	if len(mimirs) > 0 {
		out["mimirs"] = mimirs
	}
	if len(nodes) > 0 {
		out["nodes"] = nodes
	}
	if len(vaults) > 0 {
		out["vaults"] = vaults
	}
	if len(lps) > 0 {
		out["liquidity_providers"] = lps
	}
	if len(outboundFees) > 0 {
		out["outbound_fees"] = outboundFees
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
