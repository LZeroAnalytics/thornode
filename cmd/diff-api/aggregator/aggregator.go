package aggregator

import (
	"github.com/cosmos/cosmos-sdk/codec"
)

type KVWrite struct {
	Store string
	Op    string
	Key   string
	Value string
}

type AppState = map[string]any

var appCodec codec.Codec

func init() {
	ec := makeLocalEncodingConfig()
	appCodec = ec.Codec
}

func AggregateAppState(ws []KVWrite) (AppState, error) {
	out := make(AppState)

	if acc, ok := aggregateAcc(ws); ok && len(acc) > 0 {
		auth, _ := out["auth"].(map[string]any)
		if auth == nil {
			auth = make(map[string]any)
			out["auth"] = auth
		}
		if accs, ok := acc["accounts"]; ok {
			auth["accounts"] = accs
		}
		if rs, ok := acc["raw_state"]; ok {
			auth["raw_state"] = rs
		}
	}

	if bank, ok := aggregateBank(ws); ok && len(bank) > 0 {
		out["bank"] = bank
	}
	if wasm, ok := aggregateWasm(ws); ok && len(wasm) > 0 {
		out["wasm"] = wasm
	}
	if thor, ok := aggregateThorchain(ws); ok && len(thor) > 0 {
		out["thorchain"] = thor
	}
	return out, nil
}
