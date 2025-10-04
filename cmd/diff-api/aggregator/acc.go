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
	changed := false

	for _, w := range ws {
		if w.Store != storeKey || w.Op == "delete" || w.Value == "" {
			continue
		}
		bz, err := base64.StdEncoding.DecodeString(w.Value)
		if err != nil {
			continue
		}

		var ai authtypes.AccountI

		var anyMsg codectypes.Any
		if err := appCodec.Unmarshal(bz, &anyMsg); err == nil {
			var unpack authtypes.AccountI
			if err := appCodec.UnpackAny(&anyMsg, &unpack); err == nil && unpack != nil {
				ai = unpack
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
			continue
		}

		addr := strings.TrimSpace(ai.GetAddress().String())
		if addr == "" {
			continue
		}
		jb, err := appCodec.MarshalJSON(ai)
		if err != nil {
			continue
		}
		var mm map[string]any
		if json.Unmarshal(jb, &mm) != nil {
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
	return map[string]any{"accounts": accounts}, true
}
