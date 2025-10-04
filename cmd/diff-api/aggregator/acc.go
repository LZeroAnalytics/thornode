package aggregator

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
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

		var anyMsg codectypes.Any
		if err := appCodec.Unmarshal(bz, &anyMsg); err != nil {
			continue
		}
		var ai authtypes.AccountI
		if err := appCodec.UnpackAny(&anyMsg, &ai); err != nil || ai == nil {
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
