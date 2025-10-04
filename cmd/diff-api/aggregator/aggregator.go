package aggregator

type KVWrite struct {
	Store string
	Op    string
	Key   string
	Value string
}

type AppState = map[string]any

func AggregateAppState(ws []KVWrite) (AppState, error) {
	out := make(AppState)
	if bank, ok := aggregateBank(ws); ok {
		out["bank"] = bank
	}
	if wasm, ok := aggregateWasm(ws); ok {
		out["wasm"] = wasm
	}
	if thor, ok := aggregateThorchain(ws); ok {
		out["thorchain"] = thor
	}
	return out, nil
}
