package aggregator

import (
	appparams "gitlab.com/thorchain/thornode/v3/app/params"
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
	ec := appparams.MakeEncodingConfig()
	appCodec = ec.Codec
}

func AggregateAppState(ws []KVWrite) (AppState, error) {
	out := make(AppState)
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
