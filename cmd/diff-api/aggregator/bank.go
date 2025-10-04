package aggregator

import (
	"encoding/base64"
	"encoding/hex"

	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func aggregateBank(ws []KVWrite) (map[string]any, bool) {
	const storeKey = banktypes.StoreKey

	var supply []map[string]any
	balancesByAddr := make(map[string]map[string]string)
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
		case 0x02:
			if len(kb) < 3 {
				continue
			}
			addrLen := int(kb[1])
			if addrLen <= 0 || len(kb) < 2+addrLen+1 {
				continue
			}
			addrBytes := kb[2 : 2+addrLen]
			denom := string(kb[2+addrLen:])
			vb, err := base64.StdEncoding.DecodeString(w.Value)
			if err != nil || len(vb) == 0 {
				continue
			}
			amount := string(vb)
			addr, err := sdk.Bech32ifyAddressBytes("thor", addrBytes)
			if err != nil || addr == "" {
				continue
			}
			if _, ok := balancesByAddr[addr]; !ok {
				balancesByAddr[addr] = make(map[string]string)
			}
			balancesByAddr[addr][denom] = amount
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
	if len(balancesByAddr) > 0 {
		bals := make([]map[string]any, 0, len(balancesByAddr))
		for addr, denoms := range balancesByAddr {
			coins := make([]map[string]any, 0, len(denoms))
			for d, a := range denoms {
				coins = append(coins, map[string]any{
					"denom":  d,
					"amount": a,
				})
			}
			bals = append(bals, map[string]any{
				"address": addr,
				"coins":   coins,
			})
		}
		out["balances"] = bals
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
