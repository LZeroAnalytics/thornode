package aggregator

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"unicode/utf8"

	sdk "github.com/cosmos/cosmos-sdk/types"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
)

func tryUvarint(b []byte) (uint64, bool) {
	var x uint64
	var shift uint
	for i := 0; i < len(b) && i < 10; i++ {
		bt := b[i]
		x |= uint64(bt&0x7F) << shift
		if (bt & 0x80) == 0 {
			return x, true
		}
		shift += 7
	}
	return 0, false
}

func isPrintableUTF8(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, r := range string(b) {
		if r == '\u0000' {
			return false
		}
	}
	return true
}

func aggregateWasm(ws []KVWrite) (map[string]any, bool) {
	const storeKey = wasmtypes.StoreKey

	codes := make([]map[string]any, 0)
	contracts := make([]map[string]any, 0)
	sequences := make(map[string]any)
	contractStates := make(map[string][]map[string]any)

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
				if x, ok := tryUvarint(vb); ok {
					sequences["last_code_id"] = x
					changed = true
					continue
				}
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

		kb, err := hex.DecodeString(w.Key)
		if err != nil || len(kb) < 2 {
			continue
		}
		if kb[0] == 0x06 {
			addrLen := int(kb[1])
			if addrLen > 0 && len(kb) >= 2+addrLen {
				addrB := kb[2 : 2+addrLen]
				addr, err := sdk.Bech32ifyAddressBytes("thor", addrB)
				if err != nil || addr == "" {
					continue
				}
				valAny := any(hex.EncodeToString(vb))
				if len(vb) <= 10 {
					if n, ok := tryUvarint(vb); ok {
						valAny = n
					}
				}
				if s := isPrintableUTF8(vb); s {
					valAny = string(vb)
				}
				keyHex := hex.EncodeToString(kb[2+addrLen:])
				kv := map[string]any{
					"key_hex": keyHex,
					"value":   valAny,
				}
				contractStates[addr] = append(contractStates[addr], kv)
				changed = true
				continue
			}
		}
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
	if len(contractStates) > 0 {
		arr := make([]map[string]any, 0, len(contractStates))
		for addr, kvs := range contractStates {
			arr = append(arr, map[string]any{
				"contract": addr,
				"kv":       kvs,
			})
		}
		out["contract_states"] = arr
	}

	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
