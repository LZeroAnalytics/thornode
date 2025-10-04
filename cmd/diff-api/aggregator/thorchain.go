package aggregator

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	thorchaintypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func aggregateThorchain(ws []KVWrite) (map[string]any, bool) {
	var pools []map[string]any
	var mimirs []map[string]any
	var rawState []map[string]string
	changed := false

	for _, w := range ws {
		if w.Store != thorchaintypes.StoreKey {
			continue
		}
		key := strings.ToLower(w.Key)
		switch {
		case strings.HasPrefix(key, "node_account/"), strings.HasPrefix(key, "vault/"):
			continue
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
				rawState = append(rawState, map[string]string{"key_hex": w.Key, "value_b64": w.Value})
				changed = true
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
			rawKey := strings.TrimPrefix(w.Key, "mimir/")
			if rawKey == "" {
				continue
			}
			bz, err := base64.StdEncoding.DecodeString(w.Value)
			if err != nil {
				continue
			}
			var v thorchaintypes.ProtoInt64
			if err := appCodec.Unmarshal(bz, &v); err != nil {
				rawState = append(rawState, map[string]string{"key_hex": w.Key, "value_b64": w.Value})
				changed = true
				continue
			}
			mimirs = append(mimirs, map[string]any{
				"key":   rawKey,
				"value": v.GetValue(),
			})
			changed = true
		default:
			if w.Op != "delete" && w.Value != "" {
				rawState = append(rawState, map[string]string{"key_hex": w.Key, "value_b64": w.Value})
				changed = true
			}
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
	if len(rawState) > 0 {
		out["raw_state"] = rawState
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
