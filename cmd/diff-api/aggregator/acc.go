package aggregator

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
)

func aggregateAcc(ws []KVWrite) (map[string]any, bool) {
	const storeKey = authtypes.StoreKey

	accMap := make(map[string]map[string]any)
	changed := false

	for _, w := range ws {
		if w.Store != storeKey {
			continue
		}
		if w.Op == "delete" || w.Value == "" {
			continue
		}
		bz, err := base64.StdEncoding.DecodeString(w.Value)
		if err != nil {
			continue
		}

		var addr string
		var m map[string]any

		if ba := new(authtypes.BaseAccount); appCodec.Unmarshal(bz, ba) == nil && ba.Address != "" {
			if jb, err := appCodec.MarshalJSON(ba); err == nil {
				var mm map[string]any
				if json.Unmarshal(jb, &mm) == nil {
					m = mm
					addr = strings.TrimSpace(ba.Address)
				}
			}
		} else {
			switch {
			case func() bool {
				cv := new(vestingtypes.ContinuousVestingAccount)
				if appCodec.Unmarshal(bz, cv) != nil {
					return false
				}
				jb, err := appCodec.MarshalJSON(cv)
				if err != nil {
					return false
				}
				var mm map[string]any
				if json.Unmarshal(jb, &mm) != nil {
					return false
				}
				if bva, ok := mm["base_vesting_account"].(map[string]any); ok {
					if ba, ok := bva["base_account"].(map[string]any); ok {
						if a, ok := ba["address"].(string); ok {
							addr = strings.TrimSpace(a)
							m = mm
							return addr != ""
						}
					}
				}
				return false
			}():
			case func() bool {
				dv := new(vestingtypes.DelayedVestingAccount)
				if appCodec.Unmarshal(bz, dv) != nil {
					return false
				}
				jb, err := appCodec.MarshalJSON(dv)
				if err != nil {
					return false
				}
				var mm map[string]any
				if json.Unmarshal(jb, &mm) != nil {
					return false
				}
				if bva, ok := mm["base_vesting_account"].(map[string]any); ok {
					if ba, ok := bva["base_account"].(map[string]any); ok {
						if a, ok := ba["address"].(string); ok {
							addr = strings.TrimSpace(a)
							m = mm
							return addr != ""
						}
					}
				}
				return false
			}():
			case func() bool {
				pv := new(vestingtypes.PeriodicVestingAccount)
				if appCodec.Unmarshal(bz, pv) != nil {
					return false
				}
				jb, err := appCodec.MarshalJSON(pv)
				if err != nil {
					return false
				}
				var mm map[string]any
				if json.Unmarshal(jb, &mm) != nil {
					return false
				}
				if bva, ok := mm["base_vesting_account"].(map[string]any); ok {
					if ba, ok := bva["base_account"].(map[string]any); ok {
						if a, ok := ba["address"].(string); ok {
							addr = strings.TrimSpace(a)
							m = mm
							return addr != ""
						}
					}
				}
				return false
			}():
			default:
				// Fallback: try generic JSON unmarshal path if structure resembles BaseAccount
				var ba authtypes.BaseAccount
				if appCodec.Unmarshal(bz, &ba) == nil && ba.Address != "" {
					if jb, err := appCodec.MarshalJSON(&ba); err == nil {
						var mm map[string]any
						if json.Unmarshal(jb, &mm) == nil {
							m = mm
							addr = strings.TrimSpace(ba.Address)
						}
					}
				}
			}
		}

		if addr == "" || m == nil {
			continue
		}
		accMap[addr] = m
		changed = true
	}

	if !changed {
		return nil, false
	}

	accounts := make([]map[string]any, 0, len(accMap))
	for _, v := range accMap {
		accounts = append(accounts, v)
	}
	out := map[string]any{
		"accounts": accounts,
	}
	return out, true
}
