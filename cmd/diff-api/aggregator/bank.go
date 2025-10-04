package aggregator

type BankPatch struct {
	Balances []map[string]any
	Supply   []map[string]any
	Params   map[string]any
	Metadata []map[string]any
}

func aggregateBank(ws []KVWrite) (map[string]any, bool) {
	changed := false
	out := make(map[string]any)
	_ = ws
	return out, changed
}
