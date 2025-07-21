package forking

import (
	"context"
	"sync"

	storetypes "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/types"
)

type forkingKVStoreService struct {
	parent       storetypes.KVStoreService
	remoteClient RemoteClient
	cache        Cache
	config       RemoteConfig
	storeKey     string
	
	mu           sync.RWMutex
	remoteHeight int64
	
	stats ForkingStats
}

func NewForkingKVStoreService(
	parent storetypes.KVStoreService,
	remoteClient RemoteClient,
	cache Cache,
	config RemoteConfig,
	storeKey string,
) ForkingKVStoreService {
	return &forkingKVStoreService{
		parent:       parent,
		remoteClient: remoteClient,
		cache:        cache,
		config:       config,
		storeKey:     storeKey,
		remoteHeight: 0,
		stats:        ForkingStats{},
	}
}

func (f *forkingKVStoreService) OpenKVStore(ctx context.Context) storetypes.KVStore {
	parentStore := f.parent.OpenKVStore(ctx)
	return NewForkingKVStore(parentStore, f.remoteClient, f.cache, f.config, f.storeKey, f)
}

func (f *forkingKVStoreService) SetRemoteHeight(height int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.remoteHeight = height
}

func (f *forkingKVStoreService) GetRemoteHeight() int64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.remoteHeight
}

func (f *forkingKVStoreService) GetStats() ForkingStats {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.stats
}

func (f *forkingKVStoreService) updateStats(remoteFetch bool, cacheHit bool, gasUsed uint64, proofFailed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	if remoteFetch {
		f.stats.RemoteFetches++
		f.stats.GasConsumed += gasUsed
	}
	
	if proofFailed {
		f.stats.ProofFailures++
	}
	
	alpha := 0.1 // smoothing factor
	if cacheHit {
		f.stats.CacheHitRatio = alpha*1.0 + (1-alpha)*f.stats.CacheHitRatio
	} else {
		f.stats.CacheHitRatio = alpha*0.0 + (1-alpha)*f.stats.CacheHitRatio
	}
}
