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
	return out, nil
}
