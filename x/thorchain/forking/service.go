package forking

import (
	"context"
	"fmt"
	"sync"

	storetypes "cosmossdk.io/core/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type forkingKVStoreService struct {
	parent       storetypes.KVStoreService
	remoteClient RemoteClient
	cache        Cache
	config       RemoteConfig
	storeKey     string
	
	mu           sync.RWMutex
	remoteHeight int64
	pinnedHeight int64
	blockActive  bool
	
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
		pinnedHeight: 0,
		blockActive:  false,
		stats:        ForkingStats{},
	}
}

func (f *forkingKVStoreService) OpenKVStore(ctx context.Context) storetypes.KVStore {
	parentStore := f.parent.OpenKVStore(ctx)
	
	var gasMeter GasMeter
	if sdkCtx, ok := ctx.(sdk.Context); ok {
		gasMeter = NewSDKGasMeter(sdkCtx.GasMeter())
	}
	
	return NewForkingKVStore(parentStore, f.remoteClient, f.cache, f.config, f.storeKey, f, gasMeter)
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

func (f *forkingKVStoreService) BeginBlock(height int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	if f.remoteClient == nil {
		return nil
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), f.config.Timeout)
	defer cancel()
	
	var remoteHeight int64
	var err error
	
	if f.config.ForkHeight > 0 {
		remoteHeight = f.config.ForkHeight
	} else {
		remoteHeight, err = f.remoteClient.GetLatestHeight(ctx)
		if err != nil {
			if f.remoteHeight > 0 {
				f.pinnedHeight = f.remoteHeight
			} else {
				return fmt.Errorf("failed to get remote height and no previous height available: %w", err)
			}
			f.blockActive = true
			return nil
		}
	}
	
	f.pinnedHeight = remoteHeight
	f.remoteHeight = remoteHeight
	
	f.blockActive = true
	return nil
}

func (f *forkingKVStoreService) EndBlock() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	f.blockActive = false
	f.pinnedHeight = 0
	return nil
}

func (f *forkingKVStoreService) GetPinnedHeight() int64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	if f.blockActive && f.pinnedHeight > 0 {
		return f.pinnedHeight
	}
	return f.remoteHeight
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
