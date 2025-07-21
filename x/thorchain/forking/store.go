package forking

import (
	"context"
	"fmt"

	storetypes "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/types"
)

type forkingKVStore struct {
	parent       storetypes.KVStore
	remoteClient RemoteClient
	cache        Cache
	config       RemoteConfig
	storeKey     string
	service      *forkingKVStoreService
}

func NewForkingKVStore(
	parent storetypes.KVStore,
	remoteClient RemoteClient,
	cache Cache,
	config RemoteConfig,
	storeKey string,
	service *forkingKVStoreService,
) ForkingKVStore {
	return &forkingKVStore{
		parent:       parent,
		remoteClient: remoteClient,
		cache:        cache,
		config:       config,
		storeKey:     storeKey,
		service:      service,
	}
}

func (f *forkingKVStore) Get(key []byte) []byte {
	if value := f.parent.Get(key); value != nil {
		return value
	}
	
	if f.config.CacheEnabled {
		if cached := f.cache.Get(key); cached != nil {
			f.service.updateStats(false, true, 0, false)
			return cached
		}
	}
	
	if f.remoteClient == nil {
		return nil
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), f.config.Timeout)
	defer cancel()
	
	height := f.service.GetRemoteHeight()
	if height == 0 {
		var err error
		height, err = f.remoteClient.GetLatestHeight(ctx)
		if err != nil {
			f.service.updateStats(true, false, f.config.GasCostPerFetch, true)
			return nil
		}
	}
	
	value, err := f.remoteClient.GetWithProof(ctx, f.storeKey, key, height)
	if err != nil {
		f.service.updateStats(true, false, f.config.GasCostPerFetch, true)
		return nil
	}
	
	if f.config.CacheEnabled && value != nil {
		f.cache.Set(key, value)
	}
	
	if value != nil {
		f.parent.Set(key, value)
	}
	
	f.service.updateStats(true, false, f.config.GasCostPerFetch, false)
	
	return value
}

func (f *forkingKVStore) Has(key []byte) bool {
	return f.Get(key) != nil
}

func (f *forkingKVStore) Set(key []byte, value []byte) {
	f.parent.Set(key, value)
	if f.config.CacheEnabled {
		f.cache.Set(key, value)
	}
}

func (f *forkingKVStore) Delete(key []byte) {
	f.parent.Delete(key)
	if f.config.CacheEnabled {
		f.cache.Delete(key)
	}
}

func (f *forkingKVStore) Iterator(start, end []byte) storetypes.Iterator {
	return f.parent.Iterator(start, end)
}

func (f *forkingKVStore) ReverseIterator(start, end []byte) storetypes.Iterator {
	return f.parent.ReverseIterator(start, end)
}

func (f *forkingKVStore) GetStats() ForkingStats {
	return f.service.GetStats()
}
