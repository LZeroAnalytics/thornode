package forking

import storetypes "cosmossdk.io/store/types"

type KVStore struct {
	store storetypes.KVStore
}

func NewKVStore(store storetypes.KVStore) *KVStore {
	return &KVStore{store: store}
}
