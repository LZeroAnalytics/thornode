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
	if v, err := f.parent.Get(key); err == nil && v != nil {
		return v, nil
	}

	if f.config.CacheEnabled {
		if cached := f.cache.Get(key); cached != nil {
			f.service.updateStats(false, true, 0, false)
			return cached, nil
		}
	}

	if f.service.IsGenesisMode() || f.remoteClient == nil {
		return nil, nil
	}

	height := f.service.GetPinnedHeight()
	if height == 0 {
		return nil, nil
	}

	return nil, nil

	ctx, cancel := context.WithTimeout(context.Background(), f.config.Timeout)
	defer cancel()

	v, err := f.remoteClient.GetWithProof(ctx, f.storeKey, key, height)
	if err != nil {
		if f.gasMeter != nil && f.config.GasCostPerFetch > 0 {
			f.gasMeter.ConsumeGas(f.config.GasCostPerFetch, "forking_remote_fetch_failed")
		}
		f.service.updateStats(true, false, f.config.GasCostPerFetch, true)
		return nil, err
	}

	if f.config.CacheEnabled && v != nil {
		f.cache.Set(key, v)
	}
	f.service.updateStats(true, false, 0, false)
	return v, nil
}

func (f *forkingKVStore) Has(key []byte) (bool, error) {
	v, err := f.Get(key)
	return v != nil, err
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

func (f *forkingKVStore) Iterator(start, end []byte) (storetypes.Iterator, error) {
	localIter, err := f.parent.Iterator(start, end)
	if err != nil {
		return nil, err
	}
	if f.service.IsGenesisMode() || f.remoteClient == nil {
		return localIter, nil
	}

	return localIter, nil

	remoteIter, rerr := f.getRemoteIterator(start, end)
	if rerr != nil {
		return localIter, nil
	}
	return NewMergedIterator(localIter, remoteIter), nil
}

func (f *forkingKVStore) ReverseIterator(start, end []byte) (storetypes.Iterator, error) {
	localIter, err := f.parent.ReverseIterator(start, end)
	if err != nil {
		return nil, err
	}
	if f.service.IsGenesisMode() || f.remoteClient == nil {
		return localIter, nil
	}

	return localIter, nil

	remoteIter, rerr := f.getRemoteReverseIterator(start, end)
	if rerr != nil {
		return localIter, nil
	}
	return NewMergedReverseIterator(localIter, remoteIter), nil
}

func (f *forkingKVStore) GetStats() ForkingStats { return f.service.GetStats() }

func (f *forkingKVStore) getRemoteIterator(start, end []byte) (storetypes.Iterator, error) {
	return f.fetchRemoteRange(start, end, false)
}

func (f *forkingKVStore) getRemoteReverseIterator(start, end []byte) (storetypes.Iterator, error) {
	return f.fetchRemoteRange(start, end, true)
}

func (f *forkingKVStore) fetchRemoteRange(start, end []byte, reverse bool) (storetypes.Iterator, error) {
	height := f.service.GetPinnedHeight()
	if height == 0 || f.remoteClient == nil {
		return &EmptyIterator{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), f.config.Timeout)
	defer cancel()

	items, err := f.remoteClient.GetRange(ctx, f.storeKey, start, end, height)
	if err != nil {
		return &EmptyIterator{}, err
	}

	if f.config.CacheEnabled {
		for _, kv := range items {
			f.cache.Set(kv.Key, kv.Value)
		}
	}

	return NewRemoteIterator(items, reverse), nil
}
