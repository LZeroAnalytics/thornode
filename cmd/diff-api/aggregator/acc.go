package aggregator

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
)

func aggregateAcc(ws []KVWrite) (map[string]any, bool) {
	const storeKey = authtypes.StoreKey

	accMap := make(map[string]map[string]any)
	rawState := make([]map[string]string, 0)
	changed := false

	for _, w := range ws {
		if w.Store != storeKey || w.Op == "delete" || w.Value == "" {
			continue
		}
		bz, err := base64.StdEncoding.DecodeString(w.Value)
		if err != nil {
			rawState = append(rawState, map[string]string{"key_hex": w.Key, "value_b64": w.Value})
			changed = true
			continue
		}

		var ai authtypes.AccountI

		var anyMsg codectypes.Any
		if err := appCodec.Unmarshal(bz, &anyMsg); err == nil {
			var unpack authtypes.AccountI
			if err := appCodec.UnpackAny(&anyMsg, &unpack); err == nil && unpack != nil {
				ai = unpack
			} else if len(anyMsg.Value) > 0 && ai == nil {
				var ba authtypes.BaseAccount
				if err := appCodec.Unmarshal(anyMsg.Value, &ba); err == nil {
					ai = &ba
				}
				if ai == nil {
					var va vestingtypes.BaseVestingAccount
					if err := appCodec.Unmarshal(anyMsg.Value, &va); err == nil {
						ai = &va
					}
				}
				if ai == nil {
					var ca vestingtypes.ContinuousVestingAccount
					if err := appCodec.Unmarshal(anyMsg.Value, &ca); err == nil {
						ai = &ca
					}
				}
				if ai == nil {
					var da vestingtypes.DelayedVestingAccount
					if err := appCodec.Unmarshal(anyMsg.Value, &da); err == nil {
						ai = &da
					}
				}
				if ai == nil {
					var pa vestingtypes.PeriodicVestingAccount
					if err := appCodec.Unmarshal(anyMsg.Value, &pa); err == nil {
						ai = &pa
					}
				}
			}
		}

		if ai == nil {
			var ba authtypes.BaseAccount
			if err := appCodec.Unmarshal(bz, &ba); err == nil {
				ai = &ba
			}
		}
		if ai == nil {
			var va vestingtypes.BaseVestingAccount
			if err := appCodec.Unmarshal(bz, &va); err == nil {
				ai = &va
			}
		}
		if ai == nil {
			var ca vestingtypes.ContinuousVestingAccount
			if err := appCodec.Unmarshal(bz, &ca); err == nil {
				ai = &ca
			}
		}
		if ai == nil {
			var da vestingtypes.DelayedVestingAccount
			if err := appCodec.Unmarshal(bz, &da); err == nil {
				ai = &da
			}
		}
		if ai == nil {
			var pa vestingtypes.PeriodicVestingAccount
			if err := appCodec.Unmarshal(bz, &pa); err == nil {
				ai = &pa
			}
		}
		if ai == nil {
			rawState = append(rawState, map[string]string{"key_hex": w.Key, "value_b64": w.Value})
			changed = true
			continue
		}

		addr := strings.TrimSpace(ai.GetAddress().String())
		if addr == "" {
			continue
		}
		jb, err := appCodec.MarshalJSON(ai)
		if err != nil {
			rawState = append(rawState, map[string]string{"key_hex": w.Key, "value_b64": w.Value})
			changed = true
			continue
		}
		var mm map[string]any
		if json.Unmarshal(jb, &mm) != nil {
			rawState = append(rawState, map[string]string{"key_hex": w.Key, "value_b64": w.Value})
			changed = true
			continue
		}
		accMap[addr] = mm
		changed = true
	}

	if !changed {
		return nil, false
	}

	accounts := make([]map[string]any, 0, len(accMap))
	for _, v := range accMap {
		accounts = append(accounts, v)
	}
	out := map[string]any{"accounts": accounts}
	if len(rawState) > 0 {
		out["raw_state"] = rawState
	}
	return out, true
}
