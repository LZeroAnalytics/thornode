package forking

import (
	"context"

	storetypes "cosmossdk.io/core/store"
)

type forkingKVStore struct {
	parent       storetypes.KVStore
	remoteClient RemoteClient
	cache        Cache
	config       RemoteConfig
	storeKey     string
	service      *forkingKVStoreService
	gasMeter     GasMeter
}

func NewForkingKVStore(
	parent storetypes.KVStore,
	remoteClient RemoteClient,
	cache Cache,
	config RemoteConfig,
	storeKey string,
	service *forkingKVStoreService,
	gasMeter GasMeter,
) ForkingKVStore {
	return &forkingKVStore{
		parent:       parent,
		remoteClient: remoteClient,
		cache:        cache,
		config:       config,
		storeKey:     storeKey,
		service:      service,
		gasMeter:     gasMeter,
	}
}

func (f *forkingKVStore) Get(key []byte) ([]byte, error) {
	if value, err := f.parent.Get(key); err == nil && value != nil {
		return value, nil
	}
	
	if f.config.CacheEnabled {
		if cached := f.cache.Get(key); cached != nil {
			f.service.updateStats(false, true, 0, false)
			return cached, nil
		}
	}
	
	if f.remoteClient == nil {
		return nil, nil
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), f.config.Timeout)
	defer cancel()
	
	height := f.service.GetPinnedHeight()
	if height == 0 {
		var err error
		height, err = f.remoteClient.GetLatestHeight(ctx)
		if err != nil {
			if f.gasMeter != nil {
				f.gasMeter.ConsumeGas(f.config.GasCostPerFetch, "forking_remote_fetch_failed")
			}
		f.service.updateStats(true, false, f.config.GasCostPerFetch, true)
		return nil, err
		}
	}
	
	value, err := f.remoteClient.GetWithProof(ctx, f.storeKey, key, height)
	if err != nil {
		if f.gasMeter != nil {
			f.gasMeter.ConsumeGas(f.config.GasCostPerFetch, "forking_remote_fetch_failed")
		}
		f.service.updateStats(true, false, f.config.GasCostPerFetch, true)
		return nil, err
	}
	
	if f.config.CacheEnabled && value != nil {
		f.cache.Set(key, value)
	}
	
	if value != nil {
		f.parent.Set(key, value)
	}
	
	if f.gasMeter != nil {
		f.gasMeter.ConsumeGas(f.config.GasCostPerFetch, "forking_remote_fetch_success")
	}
	f.service.updateStats(true, false, f.config.GasCostPerFetch, false)
	
	return value, nil
}

func (f *forkingKVStore) Has(key []byte) (bool, error) {
	value, err := f.Get(key)
	if err != nil {
		return false, err
	}
	return value != nil, nil
}

func (f *forkingKVStore) Set(key []byte, value []byte) error {
	if err := f.parent.Set(key, value); err != nil {
		return err
	}
	if f.config.CacheEnabled {
		f.cache.Set(key, value)
	}
	return nil
}

func (f *forkingKVStore) Delete(key []byte) error {
	if err := f.parent.Delete(key); err != nil {
		return err
	}
	if f.config.CacheEnabled {
		f.cache.Delete(key)
	}
	return nil
}

func (f *forkingKVStore) Iterator(start, end []byte) storetypes.Iterator {
	iter, _ := f.parent.Iterator(start, end)
	return iter
}

func (f *forkingKVStore) ReverseIterator(start, end []byte) storetypes.Iterator {
	iter, _ := f.parent.ReverseIterator(start, end)
	return iter
}

func (f *forkingKVStore) GetStats() ForkingStats {
	return f.service.GetStats()
}
