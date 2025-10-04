package aggregator

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
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

	type codeAgg struct {
		info  map[string]any
		bytes string
		pin   bool
	}
	codesByID := make(map[uint64]*codeAgg)

	type contractAgg struct {
		info    map[string]any
		state   []map[string]any
		history []map[string]any
	}
	contractsByAddr := make(map[string]*contractAgg)

	sequences := make(map[string]any)
	paramsOut := make(map[string]any)

	changed := false

	for _, w := range ws {
		if w.Store != storeKey || w.Op == "delete" || w.Value == "" {
			continue
		}

		vb, err := base64.StdEncoding.DecodeString(w.Value)
		if err != nil {
			continue
		}

		if w.Key == "08" {
			if x, ok := tryUvarint(vb); ok {
				sequences["last_code_id"] = x
				changed = true
				continue
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
		if err != nil || len(kb) == 0 {
			continue
		}
		prefix := kb[0]

		switch prefix {
		case 0x02:
			addrLen := 0
			if len(kb) >= 2 {
				addrLen = int(kb[1])
			}
			if addrLen <= 0 || len(kb) < 2+addrLen {
				break
			}
			addrB := kb[2 : 2+addrLen]
			addr, err := sdk.Bech32ifyAddressBytes("thor", addrB)
			if err != nil || addr == "" {
				break
			}
			ci := new(wasmtypes.ContractInfo)
			if appCodec.Unmarshal(vb, ci) != nil {
				break
			}
			if jb, err := appCodec.MarshalJSON(ci); err == nil {
				var mm map[string]any
				if json.Unmarshal(jb, &mm) == nil {
					agg := contractsByAddr[addr]
					if agg == nil {
						agg = &contractAgg{}
						contractsByAddr[addr] = agg
					}
					agg.info = mm
					changed = true
				}
			}
			break

		case 0x03:
			if len(kb) < 1+32 {
				break
			}
			addrB := kb[1:33]
			suffix := kb[33:]
			addr, err := sdk.Bech32ifyAddressBytes("thor", addrB)
			if err != nil || addr == "" {
				break
			}
			if bytes.Equal(suffix, []byte("contract_info")) {
				ci := new(wasmtypes.ContractInfo)
				if appCodec.Unmarshal(vb, ci) != nil {
					break
				}
			if jb, err := appCodec.MarshalJSON(ci); err == nil {
				var mm map[string]any
				if json.Unmarshal(jb, &mm) == nil {
					agg := contractsByAddr[addr]
					if agg == nil {
						agg = &contractAgg{}
						contractsByAddr[addr] = agg
					}
					agg.info = mm
					changed = true
				}
			}
			} else {
				keyHex := hex.EncodeToString(suffix)
				agg := contractsByAddr[addr]
				if agg == nil {
					agg = &contractAgg{}
					contractsByAddr[addr] = agg
				}
				agg.state = append(agg.state, map[string]any{
					"key":   keyHex,
					"value": w.Value,
				})
				changed = true
			}
			break

		case 0x06:
			if len(kb) < 3 {
				break
			}
			addrLen := int(kb[1])
			if addrLen <= 0 || len(kb) < 2+addrLen {
				break
			}
			addrB := kb[2 : 2+addrLen]
			addr, err := sdk.Bech32ifyAddressBytes("thor", addrB)
			if err != nil || addr == "" {
				break
			}
			keyHex := hex.EncodeToString(kb[2+addrLen:])
			agg := contractsByAddr[addr]
			if agg == nil {
				agg = &contractAgg{}
				contractsByAddr[addr] = agg
			}
			agg.state = append(agg.state, map[string]any{
				"key":   keyHex,
				"value": w.Value,
			})
			changed = true

		case 0x07:
			if len(kb) < 3 {
				break
			}
			addrLen := int(kb[1])
			if addrLen <= 0 || len(kb) < 2+addrLen {
				break
			}
			addrB := kb[2 : 2+addrLen]
			addr, err := sdk.Bech32ifyAddressBytes("thor", addrB)
			if err != nil || addr == "" {
				break
			}
			h := new(wasmtypes.ContractCodeHistoryEntry)
			if appCodec.Unmarshal(vb, h) != nil {
				break
			}
			if jb, err := appCodec.MarshalJSON(h); err == nil {
				var mm map[string]any
				if json.Unmarshal(jb, &mm) == nil {
					agg := contractsByAddr[addr]
					if agg == nil {
						agg = &contractAgg{}
						contractsByAddr[addr] = agg
					}
					agg.history = append(agg.history, mm)
					changed = true
				}
			}

		case 0x01:
			if id, ok := tryUvarint(kb[1:]); ok {
				ci := new(wasmtypes.CodeInfo)
				if appCodec.Unmarshal(vb, ci) == nil {
					if jb, err := appCodec.MarshalJSON(ci); err == nil {
						var mm map[string]any
						if json.Unmarshal(jb, &mm) == nil {
							agg := codesByID[id]
							if agg == nil {
								agg = &codeAgg{}
								codesByID[id] = agg
							}
							agg.info = mm
							changed = true
						}
					}
				}
			}

		case 0x04, 0x0a:
			if id, ok := tryUvarint(kb[1:]); ok {
				agg := codesByID[id]
				if agg == nil {
					agg = &codeAgg{}
					codesByID[id] = agg
				}
				agg.bytes = w.Value
				changed = true
			}

		case 0x05:
			if id, ok := tryUvarint(kb[1:]); ok {
				agg := codesByID[id]
				if agg == nil {
					agg = &codeAgg{}
					codesByID[id] = agg
				}
				agg.pin = true
				changed = true
			}

		case 0x09:
			if len(kb) >= 1+32 {
				addrB := kb[len(kb)-32:]
				addr, err := sdk.Bech32ifyAddressBytes("thor", addrB)
				if err == nil && addr != "" {
					h := new(wasmtypes.ContractCodeHistoryEntry)
					if appCodec.Unmarshal(vb, h) == nil {
						if jb, err := appCodec.MarshalJSON(h); err == nil {
							var mm map[string]any
							if json.Unmarshal(jb, &mm) == nil {
								agg := contractsByAddr[addr]
								if agg == nil {
                                    agg = &contractAgg{}
                                    contractsByAddr[addr] = agg
                                }
								agg.history = append(agg.history, mm)
								changed = true
								break
							}
						}
					}
					var mm map[string]any
					if json.Unmarshal(vb, &mm) == nil && len(mm) > 0 {
						agg := contractsByAddr[addr]
						if agg == nil {
							agg = &contractAgg{}
							contractsByAddr[addr] = agg
						}
						agg.history = append(agg.history, mm)
						changed = true
					}
				}
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
	if len(sequences) > 0 {
		out["sequences"] = sequences
	}

	if len(codesByID) > 0 {
		codes := make([]map[string]any, 0, len(codesByID))
		for id, agg := range codesByID {
			rec := map[string]any{
				"code_id":   strconv.FormatUint(id, 10),
				"code_info": agg.info,
				"pinned":    agg.pin,
			}
			if agg.bytes != "" {
				rec["code_bytes"] = agg.bytes
			}
			codes = append(codes, rec)
		}
		out["codes"] = codes
	}

	if len(contractsByAddr) > 0 {
		contracts := make([]map[string]any, 0, len(contractsByAddr))
		for addr, agg := range contractsByAddr {
			rec := map[string]any{
				"contract_address":       addr,
				"contract_info":          agg.info,
			}
			if len(agg.state) > 0 {
				rec["contract_state"] = agg.state
			}
			if len(agg.history) > 0 {
				rec["contract_code_history"] = agg.history
			}
			contracts = append(contracts, rec)
		}
		out["contracts"] = contracts
	}

	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
