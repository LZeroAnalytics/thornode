package forking

import (
	"context"

	storetypes "cosmossdk.io/core/store"
)

type forkingKVStoreService struct {
	parent   storetypes.KVStoreService
	endpoint string
}

func NewKVStoreService(parent storetypes.KVStoreService, endpoint string) storetypes.KVStoreService {
	return &forkingKVStoreService{parent: parent, endpoint: endpoint}
}

func (s *forkingKVStoreService) OpenKVStore(ctx context.Context) storetypes.KVStore {
	under := s.parent.OpenKVStore(ctx)
	return NewKVStore(ctx, under, s.endpoint)
}
