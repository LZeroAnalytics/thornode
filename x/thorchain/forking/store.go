package forking

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	storetypes "cosmossdk.io/core/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type forkingKVStore struct {
	parent       storetypes.KVStore
	remoteClient RemoteClient
	cache        Cache
	config       RemoteConfig
	storeKey     string
	service      *forkingKVStoreService
	gasMeter     GasMeter
	sdkCtx       *sdk.Context
}

func NewForkingKVStore(
	parent storetypes.KVStore,
	remoteClient RemoteClient,
	cache Cache,
	config RemoteConfig,
	storeKey string,
	service *forkingKVStoreService,
	gasMeter GasMeter,
	sdkCtx *sdk.Context,
) ForkingKVStore {
	return &forkingKVStore{
		parent:       parent,
		remoteClient: remoteClient,
		cache:        cache,
		config:       config,
		storeKey:     storeKey,
		service:      service,
		gasMeter:     gasMeter,
		sdkCtx:       sdkCtx,
	}
}

func (f *forkingKVStore) shouldAllowRemoteFetch() bool {
	if f.service.IsGenesisMode() {
		return false
	}

	if f.remoteClient == nil {
		return false
	}

	if f.service.IsBlockProcessing() {
		return false
	}

	if f.sdkCtx != nil && (f.sdkCtx.IsCheckTx() || f.sdkCtx.IsReCheckTx()) {
		return false
	}

	return true
}

func (f *forkingKVStore) Get(key []byte) ([]byte, error) {
	if v, err := f.parent.Get(key); err == nil && v != nil {
		return v, nil
	}

	if f.config.CacheEnabled {
		if cached := f.cache.Get(key); cached != nil {
			fmt.Printf("[forking][GET] cache-hit store=%s key=%s\n", f.storeKey, hex.EncodeToString(key))
			f.service.updateStats(false, true, 0, false)
			return cached, nil
		}
	}

	if !f.shouldAllowRemoteFetch() {
		return nil, nil
	}

	height := f.service.GetPinnedHeight()
	fmt.Printf("[forking][GET] pinned-height=%d store=%s key=%s\n", height, f.storeKey, hex.EncodeToString(key))
	if height == 0 {
		fmt.Printf("[forking][GET] remote-disabled(height=0) store=%s key=%s\n", f.storeKey, hex.EncodeToString(key))
		return nil, nil
	}

	fmt.Printf("[forking][GET] remote-fetch store=%s key=%s height=%d\n", f.storeKey, hex.EncodeToString(key), height)
	ctx, cancel := context.WithTimeout(context.Background(), f.config.Timeout)
	defer cancel()

	startTime := time.Now()
	v, err := f.remoteClient.GetWithProof(ctx, f.storeKey, key, height)
	duration := time.Since(startTime)
	
	if err != nil {
		fmt.Printf("[forking][GET] remote-error store=%s key=%s height=%d duration=%v err=%v\n", f.storeKey, hex.EncodeToString(key), height, duration, err)
		if f.gasMeter != nil && f.config.GasCostPerFetch > 0 {
			f.gasMeter.ConsumeGas(f.config.GasCostPerFetch, "forking_remote_fetch_failed")
		}
		f.service.updateStats(true, false, f.config.GasCostPerFetch, true)
		return nil, err
	}

	if v == nil {
		fmt.Printf("[forking][GET] remote-miss store=%s key=%s height=%d duration=%v\n", f.storeKey, hex.EncodeToString(key), height, duration)
	} else {
		fmt.Printf("[forking][GET] remote-success store=%s key=%s height=%d duration=%v size=%d bytes\n", f.storeKey, hex.EncodeToString(key), height, duration, len(v))
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
		fmt.Printf("[forking][ITER] local-err store=%s err=%v\n", f.storeKey, err)
		return nil, err
	}

	if !f.shouldAllowRemoteFetch() {
		return localIter, nil
	}

	remoteIter, rerr := f.getRemoteIterator(start, end)
	if rerr != nil {
		return localIter, nil
	}
	return NewMergedIterator(localIter, remoteIter), nil
}

func (f *forkingKVStore) ReverseIterator(start, end []byte) (storetypes.Iterator, error) {
	localIter, err := f.parent.ReverseIterator(start, end)
	if err != nil {
		fmt.Printf("[forking][RITER] local-err store=%s err=%v\n", f.storeKey, err)
		return nil, err
	}

	if !f.shouldAllowRemoteFetch() {
		return localIter, nil
	}

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
	if !f.shouldAllowRemoteFetch() {
		return &EmptyIterator{}, nil
	}

	height := f.service.GetPinnedHeight()
	if height == 0 || f.remoteClient == nil {
		return &EmptyIterator{}, nil
	}

	fmt.Printf("[forking][RANGE] remote-fetch store=%s start=%s end=%s height=%d\n", f.storeKey, hex.EncodeToString(start), hex.EncodeToString(end), height)
	
	ctx, cancel := context.WithTimeout(context.Background(), f.config.Timeout)
	defer cancel()

	startTime := time.Now()
	items, err := f.remoteClient.GetRange(ctx, f.storeKey, start, end, height)
	duration := time.Since(startTime)
	
	if err != nil {
		fmt.Printf("[forking][RANGE] remote-error store=%s start=%s end=%s height=%d duration=%v err=%v\n", f.storeKey, hex.EncodeToString(start), hex.EncodeToString(end), height, duration, err)
		return &EmptyIterator{}, err
	}
	
	fmt.Printf("[forking][RANGE] remote-success store=%s start=%s end=%s height=%d duration=%v items=%d\n", f.storeKey, hex.EncodeToString(start), hex.EncodeToString(end), height, duration, len(items))

	if f.config.CacheEnabled {
		for _, kv := range items {
			f.cache.Set(kv.Key, kv.Value)
		}
	}

	return NewRemoteIterator(items, reverse), nil
}
