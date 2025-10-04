package aggregator

import (
	"encoding/base64"
	"encoding/json"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
)

func aggregateWasm(ws []KVWrite) (map[string]any, bool) {
	const storeKey = wasmtypes.StoreKey

	codes := make([]map[string]any, 0)
	contracts := make([]map[string]any, 0)
	sequences := make(map[string]any)

	paramsOut := make(map[string]any)
	changed := false

	for _, w := range ws {
		if w.Store != storeKey {
			continue
		}
		if w.Op == "delete" || w.Value == "" {
			continue
		}
		if w.Key == "08" {
			vb, err := base64.StdEncoding.DecodeString(w.Value)
			if err == nil && len(vb) > 0 {
				var x uint64
				var shift uint
				for i := 0; i < len(vb); i++ {
					b := vb[i]
					x |= uint64(b&0x7F) << shift
					if (b & 0x80) == 0 {
						break
					}
					shift += 7
				}
				sequences["last_code_id"] = x
				changed = true
				continue
			}
			continue
		}

		vb, err := base64.StdEncoding.DecodeString(w.Value)
		if err != nil || len(vb) == 0 {
			continue
		}

		if ci := new(wasmtypes.CodeInfo); appCodec.Unmarshal(vb, ci) == nil {
			if jb, err := appCodec.MarshalJSON(ci); err == nil {
				var mm map[string]any
				if json.Unmarshal(jb, &mm) == nil && len(mm) > 0 {
					codes = append(codes, mm)
					changed = true
					continue
				}
			}
		}

		if cti := new(wasmtypes.ContractInfo); appCodec.Unmarshal(vb, cti) == nil {
			if jb, err := appCodec.MarshalJSON(cti); err == nil {
				var mm map[string]any
				if json.Unmarshal(jb, &mm) == nil && len(mm) > 0 {
					contracts = append(contracts, mm)
					changed = true
					continue
				}
			}
		}

		if pm := new(wasmtypes.Params); appCodec.Unmarshal(vb, pm) == nil {
			if jb, err := appCodec.MarshalJSON(pm); err == nil {
				var mm map[string]any
				if json.Unmarshal(jb, &mm) == nil {
					for k, v := range mm {
						paramsOut[k] = v
					}
					changed = true
					continue
				}
			}
		}

		continue
	}

	if !changed {
		return nil, false
	}
	out := make(map[string]any)
	if len(paramsOut) > 0 {
		out["params"] = paramsOut
	}
	if len(codes) > 0 {
		out["codes"] = codes
	}
	if len(contracts) > 0 {
		out["contracts"] = contracts
	}
	if len(sequences) > 0 {
		out["sequences"] = sequences
	}

	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
