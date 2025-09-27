package forking

import (
	"context"

	store "cosmossdk.io/store/types"
)

type forkingKVStoreService struct {
	parent store.KVStoreService
}

func NewKVStoreService(parent store.KVStoreService) store.KVStoreService {
	return &forkingKVStoreService{parent: parent}
}

func (s *forkingKVStoreService) OpenKVStore(ctx context.Context) store.KVStore {
	return s.parent.OpenKVStore(ctx)
}
