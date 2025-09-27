package forking

import (
	"context"

	storetypes "cosmossdk.io/core/store"
)

type forkingKVStoreService struct {
	parent storetypes.KVStoreService
}

func NewKVStoreService(parent storetypes.KVStoreService) storetypes.KVStoreService {
	return &forkingKVStoreService{parent: parent}
}

func (s *forkingKVStoreService) OpenStore(ctx context.Context) storetypes.KVStore {
	return s.parent.OpenStore(ctx)
}
