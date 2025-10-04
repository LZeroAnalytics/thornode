package aggregator

import (
	"encoding/base64"
	"encoding/hex"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func aggregateBank(ws []KVWrite) (map[string]any, bool) {
	const storeKey = banktypes.StoreKey

	var supply []map[string]any
	changed := false

	for _, w := range ws {
		if w.Store != storeKey {
			continue
		}
		if w.Op == "delete" || w.Value == "" {
			continue
		}
		kb, err := hex.DecodeString(w.Key)
		if err != nil || len(kb) == 0 {
			continue
		}
		switch kb[0] {
		case 0x00:
			if len(kb) < 2 {
				continue
			}
			denom := string(kb[1:])
			vb, err := base64.StdEncoding.DecodeString(w.Value)
			if err != nil || len(vb) == 0 {
				continue
			}
			amount := string(vb)
			supply = append(supply, map[string]any{
				"denom":  denom,
				"amount": amount,
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
	if len(supply) > 0 {
		out["supply"] = supply
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
