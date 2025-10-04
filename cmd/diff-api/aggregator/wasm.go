package aggregator

func aggregateWasm(ws []KVWrite) (map[string]any, bool) {
	changed := false
	out := make(map[string]any)
	_ = ws
	return out, changed
}
