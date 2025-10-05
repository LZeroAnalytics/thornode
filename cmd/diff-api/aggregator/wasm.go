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
	"encoding/binary"
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

func readBEU64(b []byte) (uint64, bool) {
	if len(b) < 8 {
		return 0, false
	}
	return binary.BigEndian.Uint64(b[len(b)-8:]), true
}

func parseCodeID(b []byte) (uint64, bool) {
	if id, ok := tryUvarint(b); ok {
		return id, true
	}
	if id, ok := readBEU64(b); ok {
		return id, true
	}
	return 0, false
}
func parseAddrFromKey(kb []byte, start int) (string, bool) {
	if len(kb) >= start+32 {
		if a, err := sdk.Bech32ifyAddressBytes("thor", kb[start:start+32]); err == nil && a != "" {
			return a, true
		}
	}
	if len(kb) >= start+1 {
		l := int(kb[start])
		if l > 0 && len(kb) >= start+1+l {
			if a, err := sdk.Bech32ifyAddressBytes("thor", kb[start+1:start+1+l]); err == nil && a != "" {
				return a, true
			}
		}
	}
	return "", false
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


		kb, err := hex.DecodeString(w.Key)
		if err != nil || len(kb) == 0 {
			continue
		}
		prefix := kb[0]


		switch prefix {
		case 0x02:
			addr, ok := parseAddrFromKey(kb, 1)
			if !ok || addr == "" {
				break
			}
			ci := new(wasmtypes.ContractInfo)
			var mm map[string]any
			if appCodec.Unmarshal(vb, ci) == nil {
				if jb, err := appCodec.MarshalJSON(ci); err == nil {
					_ = json.Unmarshal(jb, &mm)
				}
			}
			if mm == nil && len(vb) > 0 && (vb[0] == '{' || vb[0] == '[') {
				_ = json.Unmarshal(vb, &mm)
			}
			if mm != nil && len(mm) > 0 {
				agg := contractsByAddr[addr]
				if agg == nil {
					agg = &contractAgg{}
					contractsByAddr[addr] = agg
				}
				agg.info = mm
				changed = true
			}
			break

		case 0x03:
			if len(kb) < 1+32 {
				break
			}
			addrB := kb[1:33]
			addr, err := sdk.Bech32ifyAddressBytes("thor", addrB)
			if err != nil || addr == "" {
				break
			}
			suffix := kb[33:]
			if bytes.Equal(suffix, []byte("contract_info")) {
				ci := new(wasmtypes.ContractInfo)
				var mm map[string]any
				if appCodec.Unmarshal(vb, ci) == nil {
					if jb, err := appCodec.MarshalJSON(ci); err == nil {
						_ = json.Unmarshal(jb, &mm)
					}
				}
				if mm == nil && len(vb) > 0 && (vb[0] == '{' || vb[0] == '[') {
					_ = json.Unmarshal(vb, &mm)
				}
				if mm != nil && len(mm) > 0 {
					agg := contractsByAddr[addr]
					if agg == nil {
						agg = &contractAgg{}
						contractsByAddr[addr] = agg
					}
					agg.info = mm
					changed = true
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
			break

		case 0x07:
			if len(kb) >= 2 {
				if id, ok := readBEU64(kb[1:]); ok {
					agg := codesByID[id]
					if agg == nil {
						agg = &codeAgg{}
						codesByID[id] = agg
					}
					agg.pin = true
					changed = true
				} else if id, ok := tryUvarint(kb[1:]); ok {
					agg := codesByID[id]
					if agg == nil {
						agg = &codeAgg{}
						codesByID[id] = agg
					}
					agg.pin = true
					changed = true
				}
			}

		case 0x01:
			if len(kb) >= 2 {
				if id, ok := readBEU64(kb[1:]); ok {
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
				} else if id, ok := tryUvarint(kb[1:]); ok {
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
			}

		case 0x04:
			suffix := kb[1:]
			switch string(suffix) {
			case "lastCodeId":
				if x, ok := tryUvarint(vb); ok {
					sequences["last_code_id"] = x
					changed = true
				} else if x, ok := readBEU64(vb); ok {
					sequences["last_code_id"] = x
					changed = true
				}
			case "lastContractId":
				if x, ok := tryUvarint(vb); ok {
					sequences["last_contract_id"] = x
					changed = true
				} else if x, ok := readBEU64(vb); ok {
					sequences["last_contract_id"] = x
					changed = true
				}
			}
		case 0x0a:
			break

		case 0x05:
			if len(kb) >= 1+32+8 {
				addrB := kb[1 : 1+32]
				addr, err := sdk.Bech32ifyAddressBytes("thor", addrB)
				if err == nil && addr != "" {
					h := new(wasmtypes.ContractCodeHistoryEntry)
					var mm map[string]any
					if appCodec.Unmarshal(vb, h) == nil {
						if jb, err := appCodec.MarshalJSON(h); err == nil {
							_ = json.Unmarshal(jb, &mm)
						}
					}
					if mm == nil || len(mm) == 0 {
						_ = json.Unmarshal(vb, &mm)
					}
					if mm != nil && len(mm) > 0 {
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

		case 0x09:
			break
		case 0x10:
			pm := new(wasmtypes.Params)
			if appCodec.Unmarshal(vb, pm) == nil {
				if jb, err := appCodec.MarshalJSON(pm); err == nil {
					var mm map[string]any
					if json.Unmarshal(jb, &mm) == nil && len(mm) > 0 {
						for k, v := range mm {
							paramsOut[k] = v
						}
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
	out["params"] = paramsOut
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
				"contract_address": addr,
				"contract_info":    agg.info,
			}
			if len(agg.state) > 0 {
				rec["contract_state"] = agg.state
			} else {
				rec["contract_state"] = []any{}
			}
			if len(agg.history) > 0 {
				rec["contract_code_history"] = agg.history
			} else {
				rec["contract_code_history"] = []any{}
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
